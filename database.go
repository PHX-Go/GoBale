package gobale

import (
	"bufio"
	"encoding/gob"
	"errors"
	"io"
	"os"
	"reflect"
	"sync"
	"time"
)

// Storage defines the abstraction interface for local or remote persistent key-value engines
type Storage interface {
	Set(k string, v any) error
	Get(k string) (any, bool)
	Del(k string) error
	Tx(fn func(store map[string]any)) error
	Close() error
}

// walOp represents a single write-ahead log operation type
type walOp uint8

const (
	walSet walOp = 1
	walDel walOp = 2
)

// walEntry is a single record written to the WAL file
type walEntry struct {
	Op  walOp
	Key string
	Val any
}

// Database wraps local file key-value storage with WAL + periodic snapshot engine
//
// Write path:
//  1. Write is applied to in-memory map immediately (zero latency for reads)
//  2. Entry is appended to WAL file (fast, O(1), no full serialize)
//  3. Background compactor takes a full snapshot every compactEvery writes
//     and truncates the WAL, keeping disk usage bounded
//
// Read path:
//
//	Always served from in-memory map — disk is never touched on reads
//
// Recovery path (startup):
//  1. Load latest snapshot file if it exists
//  2. Replay any WAL entries written after the snapshot
//  3. In-memory state is fully restored
type Database struct {
	mu    sync.RWMutex
	store map[string]any
	path  string // snapshot file path e.g. "data.gob"

	walMu  sync.Mutex
	walF   *os.File // append-only WAL file handle
	walEnc *gob.Encoder

	writeChan    chan struct{}
	closeChan    chan struct{}
	wg           sync.WaitGroup
	writeCount   uint64
	compactEvery uint64 // take a snapshot + truncate WAL every N writes
}

// NewDatabase initializes a WAL-backed key-value store at the given path with common types pre-registered.
// Snapshot → path, WAL → path+".wal"
func NewDatabase(path string) *Database {
	db := &Database{
		store:        make(map[string]any),
		path:         path,
		writeChan:    make(chan struct{}, 1),
		closeChan:    make(chan struct{}),
		compactEvery: 200,
	}

	gob.Register(walEntry{})
	gob.Register(walOp(0))
	gob.Register([]int64{})
	gob.Register([]string{})
	gob.Register(map[string]any{})

	_ = db.recover()
	_ = db.openWAL()

	db.wg.Add(1)
	go db.compactLoop()
	return db
}

// CompactEvery overrides how often (in write ops) a full snapshot is taken (default: 200)
func (db *Database) CompactEvery(n uint64) *Database {
	if n > 0 {
		db.compactEvery = n
	}
	return db
}

// walPath returns the WAL file path derived from the snapshot path
func (db *Database) walPath() string {
	return db.path + ".wal"
}

// openWAL opens or creates the WAL file in append mode
func (db *Database) openWAL() error {
	f, err := os.OpenFile(db.walPath(), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	db.walF = f
	db.walEnc = gob.NewEncoder(f)
	return nil
}

// recover loads the latest snapshot then replays the WAL on top of it
func (db *Database) recover() error {
	// Step 1: load snapshot
	if err := db.loadSnapshot(); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Step 2: replay WAL
	return db.replayWAL()
}

// loadSnapshot reads the GOB snapshot file into memory
func (db *Database) loadSnapshot() error {
	f, err := os.Open(db.path)
	if err != nil {
		return err
	}
	defer f.Close()
	return gob.NewDecoder(f).Decode(&db.store)
}

// replayWAL reads all WAL entries and applies them to the in-memory store.
// Partial/corrupt trailing entries (common on crash) are silently ignored.
func (db *Database) replayWAL() error {
	f, err := os.Open(db.walPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	dec := gob.NewDecoder(bufio.NewReader(f))
	for {
		var entry walEntry
		if err := dec.Decode(&entry); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break // normal end or crash-truncated tail
			}
			break // corrupt entry — stop replay here, data up to this point is safe
		}
		switch entry.Op {
		case walSet:
			db.store[entry.Key] = entry.Val
		case walDel:
			delete(db.store, entry.Key)
		}
	}
	return nil
}

// appendWAL writes a single entry to the WAL file and signals compactor if threshold is reached
func (db *Database) appendWAL(entry walEntry) {
	db.walMu.Lock()
	_ = db.walEnc.Encode(entry)
	_ = db.walF.Sync()
	db.writeCount++
	shouldCompact := db.writeCount >= db.compactEvery
	db.walMu.Unlock()

	if shouldCompact {
		select {
		case db.writeChan <- struct{}{}:
		default:
		}
	}
}

// compactLoop waits for the compaction signal and performs snapshot + WAL truncation
func (db *Database) compactLoop() {
	defer db.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-db.writeChan:
			_ = db.compact()
		case <-ticker.C:
			// Periodic safety compact even if threshold not reached
			db.walMu.Lock()
			dirty := db.writeCount > 0
			db.walMu.Unlock()
			if dirty {
				_ = db.compact()
			}
		case <-db.closeChan:
			return
		}
	}
}

// compact writes a full snapshot and truncates the WAL atomically.
// This is the only place that writes the snapshot file.
//
// Order matters:
//  1. Write snapshot to .tmp file
//  2. fsync + rename (atomic on POSIX)
//  3. Truncate WAL to zero + reset encoder
//  4. Reset write counter
func (db *Database) compact() error {
	// Snapshot current in-memory state
	db.mu.RLock()
	tmp := db.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		db.mu.RUnlock()
		return err
	}
	err = gob.NewEncoder(f).Encode(db.store)
	db.mu.RUnlock()

	if err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	_ = f.Sync()
	_ = f.Close()

	// Atomic replace of snapshot
	if err := os.Rename(tmp, db.path); err != nil {
		return err
	}

	// Truncate WAL now that snapshot is safe
	db.walMu.Lock()
	defer db.walMu.Unlock()

	_ = db.walF.Close()
	f2, err := os.OpenFile(db.walPath(), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	db.walF = f2
	db.walEnc = gob.NewEncoder(f2)
	db.writeCount = 0
	return nil
}

// Set saves a key-value pair to memory and appends to WAL (non-blocking for caller)
func (db *Database) Set(k string, v any) error {
	db.mu.Lock()
	db.store[k] = v
	db.mu.Unlock()
	db.appendWAL(walEntry{Op: walSet, Key: k, Val: v})
	return nil
}

// Get reads a key from in-memory store — disk is never touched
func (db *Database) Get(k string) (any, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	val, exists := db.store[k]
	return val, exists
}

// Del removes a key from memory and appends a delete entry to WAL
func (db *Database) Del(k string) error {
	db.mu.Lock()
	delete(db.store, k)
	db.mu.Unlock()
	db.appendWAL(walEntry{Op: walDel, Key: k})
	return nil
}

// Tx runs a safe isolated transaction on the in-memory store and flushes all changes via WAL
func (db *Database) Tx(fn func(store map[string]any)) error {
	db.mu.Lock()
	before := make(map[string]any, len(db.store))
	for k, v := range db.store {
		before[k] = v
	}

	fn(db.store)

	var changes []walEntry
	for k, v := range db.store {
		if !reflect.DeepEqual(before[k], v) {
			changes = append(changes, walEntry{Op: walSet, Key: k, Val: v})
		}
	}
	db.mu.Unlock()

	for _, entry := range changes {
		db.appendWAL(entry)
	}
	return nil
}

// Close flushes a final snapshot and shuts down the compactor goroutine cleanly
func (db *Database) Close() error {
	close(db.closeChan)
	db.wg.Wait()

	// Execute final compaction
	compactErr := db.compact()

	// Ensure WAL file is closed regardless of compact result
	db.walMu.Lock()
	closeErr := db.walF.Close()
	db.walMu.Unlock()

	if compactErr != nil {
		return compactErr
	}
	return closeErr
}

// DBChain provides a unified fluent state editor on top of abstract Storage methods
type DBChain struct {
	db  Storage
	bot *Bot
	op  string
	key string
	val any
	fn  func(map[string]any)
}

// DB opens the unified Database dot system from the Bot context safely with Singleton Instance
func (b *Bot) DB() *DBChain {
	return &DBChain{db: b.dbInstance, bot: b}
}

// DB opens the unified Database dot system from the Handler context safely with Singleton Instance
func (c *Ctx) DB() *DBChain {
	return &DBChain{db: c.Bot.dbInstance, bot: c.Bot}
}

// Set registers a write instruction on key with given value
func (d *DBChain) Set(k string, v any) *DBChain {
	d.op = "set"
	d.key = k
	d.val = v
	return d
}

// Get prepares a read instruction for the specified key
func (d *DBChain) Get(k string) *DBGetChain {
	return &DBGetChain{dbc: d, key: k}
}

// DBGetChain manages fluent reads ending with terminal Go
type DBGetChain struct {
	dbc *DBChain
	key string
}

// Go executes the read transaction on local DB store and returns value thread-safely
func (dg *DBGetChain) Go() (any, bool) {
	return dg.dbc.db.Get(dg.key)
}

// Del registers a delete instruction on key
func (d *DBChain) Del(k string) *DBChain {
	d.op = "del"
	d.key = k
	return d
}

// Tx prepares a transaction closure block execution on local DB store
func (d *DBChain) Tx(fn func(store map[string]any)) *DBChain {
	d.op = "tx"
	d.fn = fn
	return d
}

// Go executes the database operation with auto error logging
func (d *DBChain) Go() error {
	var err error
	switch d.op {
	case "set":
		err = d.db.Set(d.key, d.val)
	case "del":
		err = d.db.Del(d.key)
	case "tx":
		err = d.db.Tx(d.fn)
	}
	if err != nil {
		logErr(d.bot, "[Database Error] ", err)
	}
	return err
}

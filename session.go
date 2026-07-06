package gobale

import (
	"bytes"
	"encoding/gob"
	"os"
	"strconv"
	"sync"
	"time"
)

// sessionDTO represents an isolated data transfer object for thread-safe GOB encoding
type sessionDTO struct {
	StateValue   string
	DataMap      map[string]any
	LastAccessed time.Time
}

// Session represents a single chat state store
type Session struct {
	mu           sync.RWMutex
	StateValue   string
	DataMap      map[string]any
	LastAccessed time.Time
}

// SessionStore outlines basic interface for session operations
type SessionStore interface {
	Get(chatID int64) *Session
	Clear(chatID int64)
	GetSessionsCount() int
	Close() error
}

// Shard manages thread safe concurrency partition segments
type Shard struct {
	mu       sync.RWMutex
	sessions map[int64]*Session
}

// GOBStore implements sharded session manager writing database GOB formats
type GOBStore struct {
	mu         sync.Mutex
	shards     []*Shard
	shardCount int64
	filePath   string
	stop       chan struct{}
}

// NewGOBStore instantiates sharded session storage engine with safe stop channels
func NewGOBStore(filePath string) *GOBStore {
	count := 32
	m := &GOBStore{
		shards:     make([]*Shard, count),
		shardCount: int64(count),
		filePath:   filePath,
		stop:       make(chan struct{}),
	}
	for i := 0; i < count; i++ {
		m.shards[i] = &Shard{
			sessions: make(map[int64]*Session),
		}
	}

	// Start background cleanup and periodic auto-save loops
	go func() {
		cleanupTicker := time.NewTicker(1 * time.Hour)
		// Auto-save sessions periodically
		saveTicker := time.NewTicker(10 * time.Minute)
		defer cleanupTicker.Stop()
		defer saveTicker.Stop()

		for {
			select {
			case <-cleanupTicker.C:
				now := time.Now()
				for _, s := range m.shards {
					s.mu.Lock()
					for id, sess := range s.sessions {
						sess.mu.RLock()
						if now.Sub(sess.LastAccessed) > 24*time.Hour {
							sess.mu.RUnlock()
							delete(s.sessions, id)
							continue
						}
						sess.mu.RUnlock()
					}
					s.mu.Unlock()
				}
			case <-saveTicker.C:
				// Auto-save active sessions periodically to prevent state loss
				_ = m.Save()
			case <-m.stop:
				return
			}
		}
	}()
	return m
}

// Close terminates GOBStore background cleanup loop and saves active sessions
func (m *GOBStore) Close() error {
	close(m.stop)
	return m.Save()
}

// getShard calculates the partition index for concurrent isolation
func (m *GOBStore) getShard(id int64) *Shard {
	idx := id % m.shardCount
	if idx < 0 {
		idx = -idx
	}
	return m.shards[idx]
}

// Get returns pointer to targeted user Session
func (m *GOBStore) Get(id int64) *Session {
	shard := m.getShard(id)
	shard.mu.Lock()
	if s, ok := shard.sessions[id]; ok {
		s.mu.Lock()
		s.LastAccessed = time.Now()
		s.mu.Unlock()
		shard.mu.Unlock()
		return s
	}
	s := &Session{
		DataMap:      make(map[string]any),
		LastAccessed: time.Now(),
	}
	shard.sessions[id] = s
	shard.mu.Unlock()
	return s
}

// Clear destroys targeted session completely
func (m *GOBStore) Clear(id int64) {
	shard := m.getShard(id)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	delete(shard.sessions, id)
}

// Save dumps active memories safely to database file using deep copied DTOs
func (m *GOBStore) Save() error {
	flat := make(map[int64]sessionDTO)
	for _, s := range m.shards {
		s.mu.RLock()
		for k, v := range s.sessions {
			v.mu.RLock()

			// Deep copy the DataMap under Read Lock to avoid concurrent write modifications
			cp := make(map[string]any)
			for dk, dv := range v.DataMap {
				cp[dk] = dv
			}
			state := v.StateValue
			accessed := v.LastAccessed
			v.mu.RUnlock()

			// Pack the copied values inside a safe, isolated, and non-concurrent DTO
			flat[k] = sessionDTO{
				StateValue:   state,
				DataMap:      cp,
				LastAccessed: accessed,
			}
		}
		s.mu.RUnlock()
	}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(flat); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	tmp := m.filePath + ".tmp"
	file, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, err = file.Write(buf.Bytes())
	_ = file.Sync()
	_ = file.Close()
	return os.Rename(tmp, m.filePath)
}

// Load loads sessions from database file
func (m *GOBStore) Load() error {
	file, err := os.Open(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()
	var flat map[int64]sessionDTO
	if err := gob.NewDecoder(file).Decode(&flat); err != nil {
		return err
	}
	for k, v := range flat {
		shard := m.getShard(k)
		shard.mu.Lock()
		shard.sessions[k] = &Session{
			StateValue:   v.StateValue,
			DataMap:      v.DataMap,
			LastAccessed: v.LastAccessed,
		}
		shard.mu.Unlock()
	}
	return nil
}

// GetSessionsCount returns active records
func (m *GOBStore) GetSessionsCount() int {
	count := 0
	for _, s := range m.shards {
		s.mu.RLock()
		count += len(s.sessions)
		s.mu.RUnlock()
	}
	return count
}

// State opens fluent FSM state editing chain
func (s *Session) State(val ...string) *StateChain {
	return s.chainState(val...)
}

// chainState is helper constructor
func (s *Session) chainState(val ...string) *StateChain {
	return &StateChain{
		sess: s,
		val:  val,
	}
}

// StateChain handles fluent configurations of session state
type StateChain struct {
	sess *Session
	val  []string
}

// Go executes the state transaction
func (sc *StateChain) Go() (string, error) {
	sc.sess.mu.Lock()
	defer sc.sess.mu.Unlock()
	if len(sc.val) > 0 {
		sc.sess.StateValue = sc.val[0]
		sc.sess.LastAccessed = time.Now()
		return sc.val[0], nil
	}
	return sc.sess.StateValue, nil
}

// Data manages user key-value maps inside sessions fluidly
func (s *Session) Data(key string, val ...any) *DataChain {
	return &DataChain{
		sess: s,
		key:  key,
		val:  val,
	}
}

// DataChain configures fluent structures for key-value datasets
type DataChain struct {
	sess *Session
	key  string
	val  []any
}

// Go executes the session database transactional fetch or insert
func (dc *DataChain) Go() (any, error) {
	dc.sess.mu.Lock()
	defer dc.sess.mu.Unlock()
	if len(dc.val) > 0 {
		if dc.sess.DataMap == nil {
			dc.sess.DataMap = make(map[string]any)
		}
		dc.sess.DataMap[dc.key] = dc.val[0]
		dc.sess.LastAccessed = time.Now()
		return dc.val[0], nil
	}
	if dc.sess.DataMap == nil {
		return nil, nil
	}
	v, ok := dc.sess.DataMap[dc.key]
	if !ok {
		return nil, nil
	}
	return v, nil
}

// String retrieves a string value from session safely with an optional fallback default
func (s *Session) String(key string, defaultVal ...string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fallback := ""
	if len(defaultVal) > 0 {
		fallback = defaultVal[0]
	}

	if s.DataMap == nil {
		return fallback
	}
	val, ok := s.DataMap[key]
	if !ok {
		return fallback
	}

	str, ok := val.(string)
	if !ok {
		return fallback
	}
	return str
}

// Int retrieves an integer value supporting safe type-coercion and optional default fallback
func (s *Session) Int(key string, defaultVal ...int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fallback := 0
	if len(defaultVal) > 0 {
		fallback = defaultVal[0]
	}

	if s.DataMap == nil {
		return fallback
	}
	val, ok := s.DataMap[key]
	if !ok {
		return fallback
	}

	switch v := val.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case int32:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case string:
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return fallback
}

// Int64 retrieves an int64 value supporting safe type-coercion and optional default fallback
func (s *Session) Int64(key string, defaultVal ...int64) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var fallback int64
	if len(defaultVal) > 0 {
		fallback = defaultVal[0]
	}

	if s.DataMap == nil {
		return fallback
	}
	val, ok := s.DataMap[key]
	if !ok {
		return fallback
	}

	switch v := val.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			return parsed
		}
	}
	return fallback
}

// Bool retrieves a boolean value supporting safe type-coercion and optional default fallback
func (s *Session) Bool(key string, defaultVal ...bool) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fallback := false
	if len(defaultVal) > 0 {
		fallback = defaultVal[0]
	}

	if s.DataMap == nil {
		return fallback
	}
	val, ok := s.DataMap[key]
	if !ok {
		return fallback
	}

	switch v := val.(type) {
	case bool:
		return v
	case string:
		if parsed, err := strconv.ParseBool(v); err == nil {
			return parsed
		}
	case int:
		return v != 0
	case int64:
		return v != 0
	}
	return fallback
}

// Float64 retrieves a float64 value supporting safe type-coercion and optional default fallback
func (s *Session) Float64(key string, defaultVal ...float64) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var fallback float64
	if len(defaultVal) > 0 {
		fallback = defaultVal[0]
	}

	if s.DataMap == nil {
		return fallback
	}
	val, ok := s.DataMap[key]
	if !ok {
		return fallback
	}

	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			return parsed
		}
	}
	return fallback
}

func init() {
	// Register sessionDTO and slices globally inside GOB engine on startup
	gob.Register(sessionDTO{})
	gob.Register([]string{})
	gob.Register([]int64{})
}

func SessionGet[T any](s *Session, key string) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var zero T
	v, ok := s.DataMap[key]
	if !ok {
		return zero, false
	}
	typed, ok := v.(T)
	return typed, ok
}

func SessionSet[T any](s *Session, key string, val T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.DataMap == nil {
		s.DataMap = make(map[string]any)
	}
	s.DataMap[key] = val
	s.LastAccessed = time.Now()
}

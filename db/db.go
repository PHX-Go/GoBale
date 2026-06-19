package db

import (
	"encoding/gob"
	"os"
	"sync"
)

type LocalDB struct {
	mu       sync.RWMutex
	store    map[string]any
	filePath string
}

func NewLocalDB(filePath string) *LocalDB {
	db := &LocalDB{
		store:    make(map[string]any),
		filePath: filePath,
	}
	_ = db.load()
	return db
}

func (db *LocalDB) Set(key string, val any) error {
	db.mu.Lock()
	db.store[key] = val
	db.mu.Unlock()
	return db.save()
}

func (db *LocalDB) Get(key string) (any, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	val, exists := db.store[key]
	return val, exists
}

func (db *LocalDB) Delete(key string) error {
	db.mu.Lock()
	delete(db.store, key)
	db.mu.Unlock()
	return db.save()
}

func (db *LocalDB) save() error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	tmpPath := db.filePath + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	err = gob.NewEncoder(file).Encode(db.store)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return err
	}

	err = file.Sync()
	_ = file.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, db.filePath)
}

func (db *LocalDB) load() error {
	file, err := os.Open(db.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()
	return gob.NewDecoder(file).Decode(&db.store)
}

func (db *LocalDB) GetKeysCount() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.store)
}
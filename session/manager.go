package session

import (
	"bytes"
	"encoding/gob"
	"os"
	"sync"
	"time"
)

type Session struct {
	mu           sync.RWMutex
	State        string
	Data         map[string]any
	LastAccessed time.Time
}

func (s *Session) SetState(state string) {
	s.mu.Lock()
	s.State = state
	s.LastAccessed = time.Now()
	s.mu.Unlock()
}

func (s *Session) GetState() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

func (s *Session) SetData(key string, val any) {
	s.mu.Lock()
	if s.Data == nil {
		s.Data = make(map[string]any)
	}
	s.Data[key] = val
	s.LastAccessed = time.Now()
	s.mu.Unlock()
}

func (s *Session) GetData(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Data == nil {
		return nil, false
	}
	val, ok := s.Data[key]
	return val, ok
}

type SessionStore interface {
	Get(chatID int64) *Session
	Clear(chatID int64)
}

type Shard struct {
	mu       sync.RWMutex
	sessions makeMap
}

type makeMap map[int64]*Session

type GOBStore struct {
	mu         sync.Mutex
	shards     []*Shard
	shardCount int64
	filePath   string
}

func NewGOBStore(filePath string) *GOBStore {
	shardCount := 32
	m := &GOBStore{
		shards:     make([]*Shard, shardCount),
		shardCount: int64(shardCount),
		filePath:   filePath,
	}
	for i := 0; i < shardCount; i++ {
		m.shards[i] = &Shard{
			sessions: make(makeMap),
		}
	}

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			for _, shard := range m.shards {
				shard.mu.Lock()
				for chatID, s := range shard.sessions {
					s.mu.RLock()
					if now.Sub(s.LastAccessed) > 24*time.Hour {
						s.mu.RUnlock()
						delete(shard.sessions, chatID)
						continue
					}
					s.mu.RUnlock()
				}
				shard.mu.Unlock()
			}
		}
	}()

	return m
}

func (m *GOBStore) getShard(chatID int64) *Shard {
	idx := chatID % m.shardCount
	if idx < 0 {
		idx = -idx
	}
	return m.shards[idx]
}

func (m *GOBStore) Get(chatID int64) *Session {
	shard := m.getShard(chatID)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	s, exists := shard.sessions[chatID]
	if !exists {
		s = &Session{
			State:        "",
			Data:         make(map[string]any),
			LastAccessed: time.Now(),
		}
		shard.sessions[chatID] = s
	} else {
		s.mu.Lock()
		s.LastAccessed = time.Now()
		s.mu.Unlock()
	}
	return s
}

func (m *GOBStore) Clear(chatID int64) {
	shard := m.getShard(chatID)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	delete(shard.sessions, chatID)
}

func (m *GOBStore) Save() error {
	flatData := make(map[int64]*Session)
	for _, shard := range m.shards {
		shard.mu.RLock()
		for k, v := range shard.sessions {
			v.mu.RLock()
			copiedData := make(map[string]any)
			for dk, dv := range v.Data {
				copiedData[dk] = dv
			}
			state := v.State
			v.mu.RUnlock()

			flatData[k] = &Session{
				State:        state,
				Data:         copiedData,
				LastAccessed: v.LastAccessed,
			}
		}
		shard.mu.RUnlock()
	}

	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(flatData)
	if err != nil {
		return err
	}

	go func(dataToSave []byte) {
		m.mu.Lock()
		defer m.mu.Unlock()

		tmpPath := m.filePath + ".tmp"
		file, err := os.OpenFile(tmpPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return
		}

		_, err = file.Write(dataToSave)
		if err != nil {
			_ = file.Close()
			_ = os.Remove(tmpPath)
			return
		}

		err = file.Sync()
		_ = file.Close()
		if err != nil {
			_ = os.Remove(tmpPath)
			return
		}

		_ = os.Rename(tmpPath, m.filePath)
	}(buf.Bytes())

	return nil
}

func (m *GOBStore) Load() error {
	file, err := os.Open(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	var flatData map[int64]*Session
	decoder := gob.NewDecoder(file)
	err = decoder.Decode(&flatData)
	if err != nil {
		return err
	}

	for k, v := range flatData {
		shard := m.getShard(k)
		shard.mu.Lock()
		shard.sessions[k] = v
		shard.mu.Unlock()
	}
	return nil
}

func (m *GOBStore) GetSessionsCount() int {
	count := 0
	for _, shard := range m.shards {
		shard.mu.RLock()
		count += len(shard.sessions)
		shard.mu.RUnlock()
	}
	return count
}

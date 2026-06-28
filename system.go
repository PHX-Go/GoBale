package gobale

import (
	"context"
	"log"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// MemChain manages bot runtime memory and GC configurations fluidly
type MemChain struct {
	bot       *Bot
	limit     int64
	free      bool
	gcPercent int
}

// Mem opens fluent memory configuration dot system
func (b *Bot) Mem() *MemChain {
	return &MemChain{bot: b}
}

// Limit sets max runtime memory allocation limit in Megabytes
func (m *MemChain) Limit(megabytes int64) *MemChain {
	m.limit = megabytes
	return m
}

// GCPercent sets Garbage Collector target percentage dynamically
func (m *MemChain) GCPercent(p int) *MemChain {
	m.gcPercent = p
	return m
}

// Free triggers immediate garbage collection and releases OS memory
func (m *MemChain) Free(val bool) *MemChain {
	m.free = val
	return m
}

// Go executes memory configurations
func (m *MemChain) Go() {
	if m.limit > 0 {
		debug.SetMemoryLimit(m.limit * 1024 * 1024)
	}
	if m.gcPercent > 0 {
		debug.SetGCPercent(m.gcPercent)
	}
	if m.free {
		runtime.GC()
		debug.FreeOSMemory()
	}
}

// ShieldChain provides traffic control services using the unified dot system
type ShieldChain struct {
	bot    *Bot
	ctx    context.Context
	action string
}

// Shield opens the traffic control dot chain from the Bot context
func (b *Bot) Shield() *ShieldChain {
	return &ShieldChain{
		bot: b,
		ctx: context.Background(),
	}
}

// Shield opens the traffic control dot chain from the Handler context
func (c *Ctx) Shield() *ShieldChain {
	return &ShieldChain{
		bot: c.Bot,
		ctx: c.ctx,
	}
}

// Activate configures the shield to trigger dynamic throttling
func (s *ShieldChain) Activate() *ShieldChain {
	s.action = "activate"
	return s
}

// Deactivate configures the shield to restore standard rate limits
func (s *ShieldChain) Deactivate() *ShieldChain {
	s.action = "deactivate"
	return s
}

// IsActive initiates a fluent verification chain of active shield state
func (s *ShieldChain) IsActive() *ShieldActiveChain {
	return &ShieldActiveChain{sc: s}
}

// ShieldActiveChain handles fluent query of active shield mode
type ShieldActiveChain struct {
	sc *ShieldChain
}

// Go executes active shield query and returns boolean status
func (sa *ShieldActiveChain) Go() (bool, error) {
	return atomic.LoadUint32(&sa.sc.bot.shieldMode) == 1, nil
}

// Go executes the shield action configuration on Bot rate limiter
func (s *ShieldChain) Go() error {
	switch s.action {
	case "activate":
		if atomic.CompareAndSwapUint32(&s.bot.shieldMode, 0, 1) {
			log.Println("system shield activated: throttling rate-limiter")
			s.bot.Client.rateLimit = NewRL(10, time.Second)
		}
	case "deactivate":
		if atomic.CompareAndSwapUint32(&s.bot.shieldMode, 1, 0) {
			log.Println("system shield deactivated: restoring standard limits")
			s.bot.Client.rateLimit = NewRL(30, time.Second)
		}
	}
	return nil
}

// MeChain provides a fluent API to fetch bot profile details
type MeChain struct {
	bot *Bot
	ctx context.Context
}

// Me initiates a bot identity query chain from the Bot context
func (b *Bot) Me() *MeChain {
	return &MeChain{
		bot: b,
		ctx: context.Background(),
	}
}

// Me initiates a bot identity query chain from the Handler context
func (c *Ctx) Me() *MeChain {
	return &MeChain{
		bot: c.Bot,
		ctx: c.ctx,
	}
}

// Go executes the query on Bale servers and returns User
func (m *MeChain) Go() (*User, error) {
	var user User
	err := m.bot.BaseRequest(m.ctx, "getMe", nil, &user)
	return &user, err
}

// ReviewChain handles fluent structures of bale system reviews
type ReviewChain struct {
	bot   *Bot
	ctx   context.Context
	user  int64
	delay int
}

// Review opens fluent review dot chain from Bot context
func (b *Bot) Review(userID int64) *ReviewChain {
	return &ReviewChain{
		bot:  b,
		ctx:  context.Background(),
		user: userID,
	}
}

// Review opens fluent review dot chain from Handler context
func (c *Ctx) Review() *ReviewChain {
	return &ReviewChain{
		bot:  c.Bot,
		ctx:  c.ctx,
		user: c.SenderID(),
	}
}

// Delay sets delayed prompt interval seconds
func (r *ReviewChain) Delay(seconds int) *ReviewChain {
	r.delay = seconds
	return r
}

// Go executes the bale system review request
func (r *ReviewChain) Go() (bool, error) {
	var res bool
	err := r.bot.BaseRequest(r.ctx, "askReview", map[string]any{
		"user_id":       r.user,
		"delay_seconds": r.delay,
	}, &res)
	return res, err
}

// TaskChain handles scheduling of background tasks fluently
type TaskChain struct {
	bot *Bot
}

// ScheduledTask provides handles to stop running cron jobs
type ScheduledTask struct {
	stop chan struct{}
}

// Stop terminates the active scheduled task loop
func (t *ScheduledTask) Stop() {
	close(t.stop)
}

// Task opens the background task scheduler dot system from Bot context
func (b *Bot) Task() *TaskChain {
	return &TaskChain{bot: b}
}

// In executes a task once after a given delay duration safely
func (t *TaskChain) In(delay time.Duration, task func()) {
	time.AfterFunc(delay, func() {
		defer func() {
			if r := recover(); r != nil {
				handlePanic(t.bot, r, nil)
			}
		}()
		task()
	})
}

// Every runs a task repeatedly at a fixed interval duration safely and registers into Bot
func (t *TaskChain) Every(interval time.Duration, task func()) *ScheduledTask {
	stop := make(chan struct{})
	st := &ScheduledTask{stop: stop}

	t.bot.muTasks.Lock()
	t.bot.tasks = append(t.bot.tasks, st)
	t.bot.muTasks.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				func() {
					defer func() {
						if r := recover(); r != nil {
							handlePanic(t.bot, r, nil)
						}
					}()
					task()
				}()
			case <-stop:
				return
			}
		}
	}()
	return st
}

// Daily executes a task once every day at specific hour and minute safely
func (t *TaskChain) Daily(hour, minute int, task func()) *ScheduledTask {
	stop := make(chan struct{})
	st := &ScheduledTask{stop: stop}

	t.bot.muTasks.Lock()
	t.bot.tasks = append(t.bot.tasks, st)
	t.bot.muTasks.Unlock()

	go func() {
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
			if next.Before(now) {
				next = next.Add(24 * time.Hour)
			}
			delay := next.Sub(now)
			select {
			case <-time.After(delay):
				func() {
					defer func() {
						if r := recover(); r != nil {
							handlePanic(t.bot, r, nil)
						}
					}()
					task()
				}()
			case <-stop:
				return
			}
		}
	}()
	return st
}

// cacheItem holds a single cached value with expiration
type cacheItem struct {
	value     any
	expiresAt time.Time
}

// BotCache is a thread-safe in-memory TTL cache
type BotCache struct {
	mu    sync.RWMutex
	store map[string]*cacheItem
}

func newBotCache() *BotCache {
	bc := &BotCache{
		store: make(map[string]*cacheItem),
	}
	go bc.cleanup()
	return bc
}

// cleanup removes expired keys every minute in background
func (bc *BotCache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		bc.mu.Lock()
		for k, item := range bc.store {
			if now.After(item.expiresAt) {
				delete(bc.store, k)
			}
		}
		bc.mu.Unlock()
	}
}

// CacheChain provides the fluent cache dot system
type CacheChain struct {
	bot *BotCache
	ctx context.Context
	key string
	val any
	ttl time.Duration
}

// Cache opens the fluent cache dot system from Bot context
func (b *Bot) Cache() *CacheChain {
	return &CacheChain{
		bot: b.cache,
		ctx: context.Background(),
	}
}

// Cache opens the fluent cache dot system from Handler context
func (c *Ctx) Cache() *CacheChain {
	return &CacheChain{
		bot: c.Bot.cache,
		ctx: c.ctx,
	}
}

// Set prepares a write operation with key, value, and TTL
func (cc *CacheChain) Set(key string, val any, ttl time.Duration) *CacheChain {
	cc.key = key
	cc.val = val
	cc.ttl = ttl
	return cc
}

// CacheGetChain handles fluent cache reads
type CacheGetChain struct {
	cc  *CacheChain
	key string
}

// Get prepares a read operation for the given key
func (cc *CacheChain) Get(key string) *CacheGetChain {
	return &CacheGetChain{cc: cc, key: key}
}

// Go executes the read and returns value and existence flag
func (cg *CacheGetChain) Go() (any, bool) {
	cg.cc.bot.mu.RLock()
	defer cg.cc.bot.mu.RUnlock()
	item, ok := cg.cc.bot.store[cg.key]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, false
	}
	return item.value, true
}

// Del removes a key from cache
func (cc *CacheChain) Del(key string) *CacheChain {
	cc.key = key
	cc.val = nil
	cc.ttl = 0
	return cc
}

// Go executes the write or delete operation
func (cc *CacheChain) Go() {
	if cc.val == nil && cc.ttl == 0 {
		cc.bot.mu.Lock()
		delete(cc.bot.store, cc.key)
		cc.bot.mu.Unlock()
		return
	}
	cc.bot.mu.Lock()
	cc.bot.store[cc.key] = &cacheItem{
		value:     cc.val,
		expiresAt: time.Now().Add(cc.ttl),
	}
	cc.bot.mu.Unlock()
}

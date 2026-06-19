package tests

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	gobale "github.com/PHX-Go/GoBale"
	"github.com/PHX-Go/GoBale/db"
	"github.com/PHX-Go/GoBale/models"
)

func BenchmarkGoBaleFullSystem(b *testing.B) {
	bot := gobale.NewBot("123456:ABCdefGhIJKlmNoPQRsTUVwxyZ", 0)
	bot.SetMemoryLimit(32)
	bot.SetGCPercent(50)

	localDB := db.NewLocalDB("bench_heavy_system.db")
	defer os.Remove("bench_heavy_system.db")
	defer os.Remove("bench_heavy_system.db.tmp")

	limiter := gobale.NewRateLimiter(10000000, time.Second)
	cb := gobale.NewCircuitBreaker(10, time.Millisecond)

	ctx := context.Background()

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		h := sha256.New()
		buf := make([]byte, 512)
		var wg sync.WaitGroup

		for pb.Next() {
			idx := rand.Int63n(100000)

			_ = limiter.Wait(ctx)

			cb.RecordFailure()
			if !cb.CanExecute() {
				cb.RecordSuccess()
			}

			key := fmt.Sprintf("bench_key_%d", idx)
			_ = localDB.Set(key, idx)
			_, _ = localDB.Get(key)

			sess := bot.Sessions.Get(idx)
			sess.SetState(fmt.Sprintf("state_%d", idx%10))
			sess.SetData("bench_val", idx)
			_ = sess.GetState()
			_, _ = sess.GetData("bench_val")

			if idx%100 == 0 {
				bot.Sessions.Clear(idx)
			}

			wg.Add(1)
			go func(val int64) {
				defer wg.Done()
				rand.Read(buf)
				h.Write(buf)
				_ = h.Sum(nil)
				h.Reset()
			}(idx)

			var items []models.InlineKeyboardButton
			for k := 0; k < 15; k++ {
				items = append(items, models.NewInlineKeyboardButtonData(
					fmt.Sprintf("Item %d", k),
					fmt.Sprintf("cb_data_%d", k),
				))
			}
			markup := &models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{items},
			}
			_ = markup

			if idx%500 == 0 {
				runtime.GC()
			}

			wg.Wait()
		}
	})
}

package gobale

import (
	"log"
	"sync/atomic"
	"time"
)

type SystemShield struct {
	bot    *Bot
	active uint32
}

func NewSystemShield(b *Bot) *SystemShield {
	return &SystemShield{bot: b}
}

func (s *SystemShield) Activate() {
	if atomic.CompareAndSwapUint32(&s.active, 0, 1) {
		log.Println("🛡️ [System Shield] Activated! Dynamic throttling rate-limiter and tightening anti-spam.")
		s.bot.Client.SetRateLimit(10, time.Second)
	}
}

func (s *SystemShield) Deactivate() {
	if atomic.CompareAndSwapUint32(&s.active, 1, 0) {
		log.Println("🛡️ [System Shield] Deactivated. Restoring standard rate-limiter limits.")
		s.bot.Client.SetRateLimit(30, time.Second)
	}
}

func (s *SystemShield) IsActive() bool {
	return atomic.LoadUint32(&s.active) == 1
}
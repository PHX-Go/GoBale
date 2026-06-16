package gobale

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/PHX-Go/GoBale/models"
)

type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	lastRefill time.Time
	rate       float64
	capacity   float64
}

type ChatLimiterManager struct {
	mu       sync.RWMutex
	limiters map[int64]*tokenBucket
	rate     float64
	capacity float64
}

func (tb *tokenBucket) allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.lastRefill = now

	tb.tokens += elapsed * tb.rate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}
	return false
}

func NewChatLimiterManager(rate float64, capacity float64) *ChatLimiterManager {
	manager := &ChatLimiterManager{
		limiters: make(map[int64]*tokenBucket),
		rate:     rate,
		capacity: capacity,
	}

	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		for range ticker.C {
			manager.mu.Lock()
			now := time.Now()
			for chatID, tb := range manager.limiters {
				tb.mu.Lock()
				if now.Sub(tb.lastRefill) > 10*time.Minute && tb.tokens >= tb.capacity {
					delete(manager.limiters, chatID)
				}
				tb.mu.Unlock()
			}
			manager.mu.Unlock()
		}
	}()

	return manager
}

func (m *ChatLimiterManager) getLimiter(chatID int64) *tokenBucket {
	m.mu.RLock()
	tb, exists := m.limiters[chatID]
	m.mu.RUnlock()

	if exists {
		return tb
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if tb, exists = m.limiters[chatID]; exists {
		return tb
	}

	tb = &tokenBucket{
		tokens:     m.capacity,
		lastRefill: time.Now(),
		rate:       m.rate,
		capacity:   m.capacity,
	}
	m.limiters[chatID] = tb
	return tb
}

func ChatRateLimitMiddleware(rate float64, capacity float64, onLimit HandlerFunc) HandlerFunc {
	manager := NewChatLimiterManager(rate, capacity)

	return func(c *Context) {
		chatID, err := c.DetermineChatID()
		if err != nil {
			c.Next()
			return
		}

		tb := manager.getLimiter(chatID)
		if !tb.allow() {
			if onLimit != nil {
				onLimit(c)
			}
			c.Abort()
			return
		}

		c.Next()
	}
}

func BlacklistMiddleware(blacklist map[int64]bool) HandlerFunc {
	return func(c *Context) {
		if c.Message != nil && c.Message.From != nil {
			if blacklist[c.Message.From.ID] {
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

func MaintenanceMiddleware(enabled *bool, adminID int64, alertText string) HandlerFunc {
	return func(c *Context) {
		if c.Message != nil && c.Message.From != nil {
			if *enabled && c.Message.From.ID != adminID {
				c.Reply(alertText)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

func AntiSpamMiddleware(b *Bot) HandlerFunc {
	return func(c *Context) {
		if b.AntiSpamLimit > 0 && c.Message != nil && c.Message.From != nil {
			userID := c.Message.From.ID
			now := time.Now().UnixNano()

			windowNs := int64(5 * time.Second)
			if b.AntiSpamWindow > 0 {
				windowNs = int64(b.AntiSpamWindow)
			}

			val, _ := b.userLimits.LoadOrStore(userID, &userLimit{})
			ul := val.(*userLimit)

			ul.mu.Lock()
			if now-ul.windowStart < windowNs {
				ul.msgCount++
			} else {
				ul.windowStart = now
				ul.msgCount = 1
			}
			count := ul.msgCount
			ul.mu.Unlock()

			if count > b.AntiSpamLimit {
				if b.OnSpam != nil {
					b.OnSpam(c)
				}
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

func AntiLinkMiddleware() HandlerFunc {
	return func(c *Context) {
		if c.Message != nil && c.Message.Text != "" {
			text := c.Message.Text
			if strings.Contains(text, "http://") || strings.Contains(text, "https://") || strings.Contains(text, "ble.ir/") || strings.Contains(text, "join/") {
				if !c.IsAdmin() {
					c.Delete()
					c.Reply("⚠️ ارسال هرگونه لینک تبلیغاتی در این گروه ممنوع است!")
					c.Abort()
					return
				}
			}
		}
		c.Next()
	}
}

func MandatoryJoinMiddleware(chatID any, alertText string) HandlerFunc {
	return func(c *Context) {
		if c.Message != nil && c.Message.From != nil {
			text := c.Message.Text
			if text == "/start" || strings.HasPrefix(text, "/start ") {
				c.Next()
				return
			}
			userID := c.Message.From.ID
			resolvedID := c.Bot.ResolveChatID(chatID)
			member, err := c.Bot.GetChatMember(resolvedID, userID)
			if err != nil || member.Status == "left" || member.Status == "kicked" {
				c.Reply(alertText)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

func LoggerMiddleware() HandlerFunc {
	return func(c *Context) {
		startTime := time.Now()
		c.Next()
		duration := time.Since(startTime)
		if c.Bot.Logger && c.Message != nil {
			log.Printf("[GoBale] %s | %v | %d | %q",
				time.Now().Format("15:04:05"),
				duration,
				c.Message.From.ID,
				c.Message.Text,
			)
		}
	}
}

func AdminOnly(adminID int64) HandlerFunc {
	return func(c *Context) {
		if c.Message == nil || c.Message.From == nil {
			c.Abort()
			return
		}

		if c.Message.From.ID != adminID {
			c.Reply("⚠️ دسترسی غیرمجاز! این بخش مخصوص مدیریت ربات است.")
			c.Abort()
			return
		}

		c.Next()
	}
}

type commandCooldown struct {
	mu    sync.Mutex
	users map[int64]time.Time
}

func Cooldown(duration time.Duration, alertText string) HandlerFunc {
	cooldowns := &commandCooldown{
		users: make(map[int64]time.Time),
	}

	return func(c *Context) {
		if c.Message == nil || c.Message.From == nil {
			c.Next()
			return
		}

		userID := c.Message.From.ID
		now := time.Now()

		cooldowns.mu.Lock()
		lastRun, exists := cooldowns.users[userID]
		if exists && now.Sub(lastRun) < duration {
			remaining := duration - now.Sub(lastRun)
			cooldowns.mu.Unlock()

			c.Reply(fmt.Sprintf(alertText, remaining.Round(time.Second)))
			c.Abort()
			return
		}

		cooldowns.users[userID] = now
		cooldowns.mu.Unlock()

		c.Next()
	}
}

func SendAction(action string) HandlerFunc {
	return func(c *Context) {
		_, _ = c.SendChatAction(action)
		c.Next()
	}
}

func RequireRole(allowedRole string, alertText string) HandlerFunc {
	return func(c *Context) {
		roleVal, exists := c.GetData("role")

		if !exists || roleVal.(string) != allowedRole {
			c.Reply(alertText)
			c.Abort()
			return
		}

		c.Next()
	}
}

func AdminsOnly() HandlerFunc {
	return func(c *Context) {
		if !c.IsAdmin() {
			c.Reply("⚠️ دسترسی غیرمجاز! این دستور فقط مخصوص مدیران (ادمین‌ها) این گروه یا کانال است.")
			c.Abort()
			return
		}
		c.Next()
	}
}

func MandatoryJoinMulti(alertText string, channels ...string) HandlerFunc {
	return func(c *Context) {
		if c.Message == nil || c.Message.From == nil {
			c.Next()
			return
		}

		userID := c.Message.From.ID
		var missingChannels []string

		for _, channel := range channels {

			checkID := channel
			if strings.Contains(channel, "ble.ir") || strings.Contains(channel, "http") {
				parts := strings.SplitN(channel, ":", 2)
				if len(parts) == 2 {
					checkID = parts[0]
				}
			}

			member, err := c.Bot.GetChatMember(checkID, userID)
			if err != nil || member.Status == "left" || member.Status == "kicked" {
				missingChannels = append(missingChannels, channel)
			}
		}

		if len(missingChannels) == 0 {
			c.Next()
			return
		}

		builder := models.InlineMarkup()
		for _, ch := range missingChannels {

			if strings.Contains(ch, "ble.ir") || strings.Contains(ch, "http") {
				parts := strings.SplitN(ch, ":", 2)
				if len(parts) == 2 {
					joinLink := parts[1]

					builder.Row(models.Btn("📢 عضویت در کانال خصوصی").URL(joinLink))
				}
				continue
			}

			cleanUsername := strings.TrimPrefix(ch, "@")
			linkURL := fmt.Sprintf("https://ble.ir/%s", cleanUsername)

			builder.Row(models.Btn(fmt.Sprintf("📢 عضویت در کانال %s", ch)).URL(linkURL))
		}

		builder.Row(models.Btn("✅ در همه کانال‌ها عضو شدم! تایید مجدد").Callback("check_mandatory_join"))

		markup := builder.Build()

		_, _ = c.Send(alertText, WithKeyboard(markup))
		c.Abort()
	}
}

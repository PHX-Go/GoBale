package gobale

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PHX-Go/GoBale/models"
)

type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	lastRefill time.Time
	rate       float64
	capacity   float64
	lastWarned time.Time
}

type ChatLimiterManager struct {
	shards     []*limiterShard
	shardCount int64
	rate       float64
	capacity   float64
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

func (tb *tokenBucket) shouldWarn(cooldown time.Duration) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	if now.Sub(tb.lastWarned) >= cooldown {
		tb.lastWarned = now
		return true
	}
	return false
}

type limiterShard struct {
	mu       sync.RWMutex
	limiters map[int64]*tokenBucket
}

func NewChatLimiterManager(rate float64, capacity float64) *ChatLimiterManager {
	shardCount := 32
	manager := &ChatLimiterManager{
		shards:     make([]*limiterShard, shardCount),
		shardCount: int64(shardCount),
		rate:       rate,
		capacity:   capacity,
	}

	for i := 0; i < shardCount; i++ {
		manager.shards[i] = &limiterShard{
			limiters: make(map[int64]*tokenBucket),
		}
	}

	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			for _, shard := range manager.shards {
				shard.mu.Lock()
				for chatID, tb := range shard.limiters {
					tb.mu.Lock()
					if now.Sub(tb.lastRefill) > 10*time.Minute && tb.tokens >= tb.capacity {
						delete(shard.limiters, chatID)
					}
					tb.mu.Unlock()
				}
				shard.mu.Unlock()
			}
		}
	}()

	return manager
}

func (m *ChatLimiterManager) getShard(chatID int64) *limiterShard {
	idx := chatID % m.shardCount
	if idx < 0 {
		idx = -idx
	}
	return m.shards[idx]
}

func (m *ChatLimiterManager) getLimiter(chatID int64) *tokenBucket {
	shard := m.getShard(chatID)

	shard.mu.RLock()
	tb, exists := shard.limiters[chatID]
	shard.mu.RUnlock()

	if exists {
		return tb
	}

	shard.mu.Lock()
	defer shard.mu.Unlock()

	if tb, exists = shard.limiters[chatID]; exists {
		return tb
	}

	tb = &tokenBucket{
		tokens:     m.capacity,
		lastRefill: time.Now(),
		rate:       m.rate,
		capacity:   m.capacity,
	}
	shard.limiters[chatID] = tb
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

			_ = c.Delete()

			if tb.shouldWarn(5 * time.Second) {
				if onLimit != nil {
					onLimit(c)
				}
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

func AntiSpamMiddleware(limit int, window time.Duration) HandlerFunc {
	var userLimits sync.Map

	return func(c *Context) {
		if limit > 0 && c.Message != nil && c.Message.From != nil {
			userID := c.Message.From.ID
			now := time.Now().UnixNano()
			windowNs := int64(window)

			val, _ := userLimits.LoadOrStore(userID, &userLimit{})
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

			activeLimit := limit
			if atomic.LoadUint32(&c.Bot.shieldMode) == 1 {
				activeLimit = limit / 3
				if activeLimit < 1 {
					activeLimit = 1
				}
			}

			if count > activeLimit {
				_ = c.Delete()

				if count == activeLimit+1 {
					var mention string
					if c.Message.From.Username != "" {
						mention = "@" + c.Message.From.Username
					} else {
						mention = Bold(c.Message.From.FirstName)
					}

					var warningText string
					if activeLimit < limit {
						warningText = fmt.Sprintf("🛡️ کاربر %s، ربات در حالت سپر دفاعی (ترافیک شدید) قرار دارد! ارسال شما موقتاً مسدود شد.", mention)
					} else {
						warningText = fmt.Sprintf("⚠️ کاربر %s، شما به دلیل ارسال بیش از حد پیام مسدود شدید! پیام‌های شما موقتاً حذف خواهند شد.", mention)
					}

					_, _ = c.SendTemp(warningText, 5*time.Second, WithMarkdown())
				}

				c.Abort()
				return
			}
		}
		c.Next()
	}
}

var DefaultTLDs = []string{"com", "ir", "net", "org", "co", "biz", "info", "me", "club", "xyz", "link", "site", "online", "space", "tech", "website", "gov", "edu", "ble\\.ir"}

func AntiLinkMiddleware(warnDuration time.Duration, customTLDs ...string) HandlerFunc {
	tldsList := append(DefaultTLDs, customTLDs...)

	tldsPattern := strings.Join(tldsList, "|")
	regexPattern := fmt.Sprintf(`(?i)(https?://)?([a-zA-Z0-9-]+\.)+(%s)(/[^\s]*)?`, tldsPattern)
	linkRegex := regexp.MustCompile(regexPattern)

	return func(c *Context) {
		if c.Message != nil && c.Message.Text != "" && c.Message.From != nil {
			text := c.Message.Text

			if linkRegex.MatchString(text) {
				if !c.IsAdmin() {
					var mention string
					if c.Message.From.Username != "" {
						mention = "@" + c.Message.From.Username
					} else {
						mention = Bold(c.Message.From.FirstName)
					}

					warningText := fmt.Sprintf("⚠️ کاربر %s، ارسال هرگونه لینک تبلیغاتی در این گروه ممنوع است!", mention)

					_, _ = c.SendTemp(warningText, warnDuration, WithMarkdown())

					_ = c.Delete()

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

func SuperGroupOnly(alertText string) HandlerFunc {
	return func(c *Context) {
		if !c.IsSuperGroup() {
			c.Reply(alertText)
			c.Abort()
			return
		}
		c.Next()
	}
}

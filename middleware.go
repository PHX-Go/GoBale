package gobale

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// tokenBucket manages request limits for rate limiting thread-safely
type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	lastRefill time.Time
	rate       float64
	cap        float64
	lastWarn   time.Time
}

// allow checks and decrements rate limit token
func (t *tokenBucket) allow() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(t.lastRefill).Seconds()
	t.lastRefill = now
	t.tokens += elapsed * t.rate
	if t.tokens > t.cap {
		t.tokens = t.cap
	}
	if t.tokens >= 1.0 {
		t.tokens -= 1.0
		return true
	}
	return false
}

// shouldWarn checks warning cooldown to avoid alert spamming thread-safely
func (t *tokenBucket) shouldWarn(cooldown time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	// Safely read and update lastWarn under mutual exclusion lock (mu)
	if now.Sub(t.lastWarn) >= cooldown {
		t.lastWarn = now
		return true
	}
	return false
}

// ChatRateLimit restricts chat message intervals using token bucket
func ChatRateLimit(rate, capacity float64, onLimit Handler) Handler {
	var limiters sync.Map
	return func(c *Ctx) {
		id, err := c.ChatID()
		if err != nil {
			c.Next()
			return
		}
		if c.Message != nil && c.Message.From != nil {
			c.Bot.mu.RLock()
			isOwner := c.Message.From.ID == c.Bot.MaintenanceAdminID
			c.Bot.mu.RUnlock()
			isAdmin, _ := c.Chat().IsAdmin().Go()
			if isOwner || isAdmin {
				c.Next()
				return
			}
		}
		val, _ := limiters.LoadOrStore(id, &tokenBucket{
			tokens:     capacity,
			lastRefill: time.Now(),
			rate:       rate,
			cap:        capacity,
		})
		tb := val.(*tokenBucket)
		if !tb.allow() {
			_ = c.Del().Go()
			if tb.shouldWarn(5*time.Second) && onLimit != nil {
				onLimit(c)
			}
			c.Abort()
			return
		}
		c.Next()
	}
}

// userLimit tracks message frequency for antispam
type userLimit struct {
	mu    sync.Mutex
	start int64
	count int
}

// AntiSpam prevents rapid message flood anomalies with customizable auto-deleting alerts
func AntiSpam(limit int, window time.Duration, warnMsg ...string) Handler {
	var tracker sync.Map

	// Default friendly warning text with mention placeholder
	defaultWarn := "⚠️ کاربر عزیز {name}، لطفاً از ارسال پیام‌های پی‌درپی خودداری کنید!"
	if len(warnMsg) > 0 && warnMsg[0] != "" {
		defaultWarn = warnMsg[0]
	}

	return func(c *Ctx) {
		// Bypass callback queries (button clicks)
		if c.Update != nil && c.Update.CallbackQuery != nil {
			c.Next()
			return
		}

		if c.Message == nil || c.Message.From == nil {
			c.Next()
			return
		}
		userID := c.Message.From.ID

		// Bypass global bot owner/administrator
		c.Bot.mu.RLock()
		isOwner := userID == c.Bot.MaintenanceAdminID
		c.Bot.mu.RUnlock()

		// Bypass group administrators
		isAdmin := false
		if c.IsGroup() {
			isAdmin, _ = c.Chat().IsAdmin().Go()
		}

		if isOwner || isAdmin {
			c.Next()
			return
		}

		now := time.Now().UnixNano()
		winNs := int64(window)
		val, _ := tracker.LoadOrStore(userID, &userLimit{})
		ul := val.(*userLimit)
		ul.mu.Lock()
		if now-ul.start < winNs {
			ul.count++
		} else {
			ul.start = now
			ul.count = 1
		}
		count := ul.count
		ul.mu.Unlock()

		activeLimit := limit
		activeShield, _ := c.Bot.Shield().IsActive().Go()
		if activeShield {
			activeLimit = limit / 3
			if activeLimit < 1 {
				activeLimit = 1
			}
		}

		if count > activeLimit {
			_ = c.Del().Go() // Delete the spam message
			if count == activeLimit+1 {
				warn := defaultWarn
				if activeShield {
					warn = "🚨 *[سپر دفاعی فعال]* اسپم متوالی شناسایی شد! پیام‌ها به طور خودکار حذف می‌شوند."
				} else {
					// Replace placeholder with smart mention
					warn = strings.ReplaceAll(warn, "{name}", c.Message.From.Mention())
				}

				// Send a self-destroying warning to keep chat clean
				_, _ = c.Send().
					Text(warn).
					Markdown().
					Temp(5 * time.Second). // Automatically deletes after 5 seconds
					Go()
			}
			c.Abort()
			return
		}
		c.Next()
	}
}

// AntiLink deletes unwanted links and advertisements from chat messages with custom warnings
func AntiLink(warnDuration time.Duration, customMsg string, customTLDs ...string) Handler {
	tlds := []string{"com", "ir", "net", "org", "co", "ble\\.ir"}
	tlds = append(tlds, customTLDs...)
	pattern := fmt.Sprintf(`(?i)(https?://)?([a-zA-Z0-9-]+\.)+(%s)(/[^\s]*)?`, strings.Join(tlds, "|"))
	regex := regexp.MustCompile(pattern)

	// Set default warning text if customMsg is empty
	defaultWarn := "⚠️ کاربر عزیز {name}، ارسال لینک و تبلیغات در این گروه مجاز نیست!"
	if customMsg != "" {
		defaultWarn = customMsg
	}

	return func(c *Ctx) {
		if c.Message != nil && c.Message.Text != "" && c.Message.From != nil {
			c.Bot.mu.RLock()
			isOwner := c.Message.From.ID == c.Bot.MaintenanceAdminID
			c.Bot.mu.RUnlock()
			isAdmin, _ := c.Chat().IsAdmin().Go()
			if isOwner || isAdmin {
				c.Next()
				return
			}
			if regex.MatchString(c.Message.Text) {
				_ = c.Del().Go() // Delete the link message

				warn := strings.ReplaceAll(defaultWarn, "{name}", c.Message.From.Mention())
				_, _ = c.Send().
					Text(warn).
					Temp(warnDuration). // Automatically delete the warning message
					Go()

				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// Cooldown implements delay rules before executing commands again
func Cooldown(dur time.Duration, alert string) Handler {
	var users sync.Map
	return func(c *Ctx) {
		if c.Message == nil || c.Message.From == nil {
			c.Next()
			return
		}
		userID := c.Message.From.ID
		now := time.Now()
		val, loaded := users.LoadOrStore(userID, now)
		if loaded {
			last := val.(time.Time)
			if now.Sub(last) < dur {
				rem := dur - now.Sub(last)
				_, _ = c.Send().Text(fmt.Sprintf(alert, rem.Round(time.Second))).Go()
				c.Abort()
				return
			}
			users.Store(userID, now)
		}
		c.Next()
	}
}

// AntiCrash shields the bot routing channels from combining-mark payloads
func AntiCrash() Handler {
	return func(c *Ctx) {
		if c.Message != nil && c.Message.Text != "" {
			if IsCrashPayload(c.Message.Text) {
				_ = c.Del().Go()
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// IsCrashPayload checks for malicious character overload crashes
func IsCrashPayload(text string) bool {
	runes := []rune(text)
	if len(runes) > 4096 {
		return true
	}
	consec := 0
	total := 0
	for _, r := range runes {
		mark := false
		if r >= 0x0300 && r <= 0x036F {
			mark = true
		}
		if (r >= 0x0610 && r <= 0x061A) || (r >= 0x064B && r <= 0x065F) || (r >= 0x06D6 && r <= 0x06ED) {
			mark = true
		}
		if unicode.Is(unicode.Mn, r) {
			mark = true
		}
		if mark {
			consec++
			total++
			if consec > 5 {
				return true
			}
		} else {
			consec = 0
		}
	}
	if len(runes) > 15 {
		if float64(total)/float64(len(runes)) > 0.35 {
			return true
		}
	}
	return false
}

// AdminsOnly restricts execution of the handler to group administrators and deletes access warning after 5s
func AdminsOnly(customMsg ...string) Handler {
	return func(c *Ctx) {
		isAdmin, err := c.Chat().IsAdmin().Go()
		if err != nil || !isAdmin {
			warn := "⚠️ دسترسی غیرمجاز! این دستور فقط مخصوص مدیران (ادمین‌ها) است."
			if len(customMsg) > 0 && customMsg[0] != "" {
				warn = customMsg[0]
			}

			_, _ = c.Send().
				Text(warn).
				Temp(5 * time.Second).
				Go()

			c.Abort()
			return
		}
		c.Next()
	}
}

// AdminOnly restricts execution to the global bot owner with custom warnings
func AdminOnly(adminID int64, customMsg ...string) Handler {
	// Set default warning text if customMsg is empty
	defaultWarn := "⚠️ دسترسی غیرمجاز! این بخش مخصوص مدیریت کل ربات است."
	if len(customMsg) > 0 && customMsg[0] != "" {
		defaultWarn = customMsg[0]
	}

	return func(c *Ctx) {
		if c.Message == nil || c.Message.From == nil {
			c.Abort()
			return
		}
		if c.Message.From.ID != adminID {
			warn := strings.ReplaceAll(defaultWarn, "{name}", c.Message.From.Mention())
			_, _ = c.Send().
				Text(warn).
				Temp(5 * time.Second). // Clean up the chat
				Go()
			c.Abort()
			return
		}
		c.Next()
	}
}

// SuperGroupOnly restricts execution to supergroups only
func SuperGroupOnly(alert string) Handler {
	return func(c *Ctx) {
		if c.Message == nil || c.Message.Chat.Type != "supergroup" {
			_, _ = c.Send().Text(alert).Go()
			c.Abort()
			return
		}
		c.Next()
	}
}

// ChatGuard is a unified single-pass protection middleware with panic-proof checks and direct terminal logs
func ChatGuard(warnDuration time.Duration, customMsg string, silent bool) Handler {
	defaultWarn := "⚠️ کاربر عزیز {name}، شما اجازه ارسال رسانه از نوع [{type}] را در این چت ندارید!"
	if customMsg != "" {
		defaultWarn = customMsg
	}

	return func(c *Ctx) {
		// Panic-Proof Shield: Bypass immediately if message or sender is nil (e.g. Channel posts)
		if c.Message == nil || c.Message.From == nil {
			c.Next()
			return
		}

		if c.IsPrivate() {
			c.Next()
			return
		}

		chatID, err := c.ChatID()
		if err != nil {
			c.Next()
			return
		}

		senderID := c.SenderID()

		// Bypass global administrators and owner
		c.Bot.mu.RLock()
		isOwner := senderID == c.Bot.MaintenanceAdminID
		c.Bot.mu.RUnlock()
		if isOwner {
			c.Next()
			return
		}

		isAdmin, errAdmin := c.Chat().IsAdmin().Go()
		if errAdmin == nil && isAdmin {
			c.Next()
			return
		}

		db := c.Bot.dbInstance
		if db == nil {
			c.Next()
			return
		}

		// 1. Check if group is locked natively
		lockKey := fmt.Sprintf("group_lock_%d", chatID)
		if val, ok := db.Get(lockKey); ok {
			if locked, okBool := val.(bool); okBool && locked {
				_ = c.Del().Go()
				_, _ = c.Send().Text("⚠️ چت گروه در حال حاضر توسط مدیریت قفل شده است.").Temp(5 * time.Second).Go()
				c.Abort()
				return
			}
		}

		// 2. Check if captcha unverified or muted natively
		captchaKey := fmt.Sprintf("captcha_mute_%d_%d", chatID, senderID)
		if val, ok := db.Get(captchaKey); ok {
			if isMuted, okBool := val.(bool); okBool && isMuted {
				_ = c.Del().Go()
				c.Abort()
				return
			}
		}

		muteKey := fmt.Sprintf("mute_user_%d_%d", chatID, senderID)
		if val, ok := db.Get(muteKey); ok {
			if isMuted, okBool := val.(bool); okBool && isMuted {
				_ = c.Del().Go()
				c.Abort()
				return
			}
		}

		blockedMap := make(map[string]bool)

		// 3. Dynamically map registered group settings directly to media blocks
		c.Bot.mu.RLock()
		for _, setting := range c.Bot.settings { // Fixed: Read from settings instead of groupSettings
			if !setting.IsLocal {
				continue // Skip global settings like maintenance mode
			}
			dbKey := fmt.Sprintf("group_config_%d_%s", chatID, setting.Key)
			if val, ok := db.Get(dbKey); ok {
				if active, okBool := val.(bool); okBool && active {
					switch setting.Key {
					case "g_lock":
						blockedMap[string(MediaAll)] = true
					case "g_lock_photo":
						blockedMap[string(MediaPhoto)] = true
					case "g_lock_voice":
						blockedMap[string(MediaVoice)] = true
					case "g_lock_video":
						blockedMap[string(MediaVideo)] = true
					case "g_lock_sticker":
						blockedMap[string(MediaSticker)] = true
					}
				}
			}
		}
		c.Bot.mu.RUnlock()

		// 4. Also check Group-wide manual command restrictions (e.g. /restrict)
		groupKey := fmt.Sprintf("blocked_media_group_%d", chatID)
		if groupVal, okGroup := db.Get(groupKey); okGroup {
			if blockedSlice, okSlice := groupVal.([]string); okSlice {
				for _, b := range blockedSlice {
					blockedMap[b] = true
				}
			}
		}

		// 5. Also check Individual user manual command restrictions (e.g. /restrict [userID])
		userKey := fmt.Sprintf("blocked_media_%d_%d", chatID, senderID)
		if userVal, okUser := db.Get(userKey); okUser {
			if blockedSlice, okSlice := userVal.([]string); okSlice {
				for _, b := range blockedSlice {
					blockedMap[b] = true
				}
			}
		}

		if len(blockedMap) == 0 {
			c.Next()
			return
		}

		// Detect message media type
		var detected MediaType
		var matchedTypeFarsi string

		if len(c.Message.Photo) > 0 {
			detected = MediaPhoto
			matchedTypeFarsi = "تصویر"
		} else if c.Message.Video != nil {
			detected = MediaVideo
			matchedTypeFarsi = "ویدیو"
		} else if c.Message.Audio != nil {
			detected = MediaAudio
			matchedTypeFarsi = "فایل صوتی"
		} else if c.Message.Document != nil {
			detected = MediaDocument
			matchedTypeFarsi = "سند (فایل)"
		} else if c.Message.Voice != nil {
			detected = MediaVoice
			matchedTypeFarsi = "پیام صوتی (وویس)"
		} else if c.Message.Sticker != nil {
			detected = MediaSticker
			matchedTypeFarsi = "استیکر"
		} else if c.Message.Animation != nil {
			detected = MediaAnimation
			matchedTypeFarsi = "گیف (انیمیشن)"
		} else if c.Message.Location != nil {
			detected = MediaLocation
			matchedTypeFarsi = "موقعیت مکانی"
		} else if c.Message.Contact != nil {
			detected = MediaContact
			matchedTypeFarsi = "مخاطب"
		}

		if detected != "" {
			isBlocked := blockedMap[string(detected)] || blockedMap[string(MediaAll)]
			if isBlocked {
				// Attempt to delete message natively and print error on failure to console
				errDel := c.Del().Go()
				if errDel != nil {
					log.Printf("[DEBUG ERROR] Failed to delete message: %v", errDel)
				}

				if !silent && warnDuration > 0 {
					warn := strings.ReplaceAll(defaultWarn, "{name}", c.Message.From.Mention())
					warn = strings.ReplaceAll(warn, "{type}", matchedTypeFarsi)
					_, _ = c.Send().Text(warn).Temp(warnDuration).Go()
				}
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// handlePanic centrally processes recovered panics, formats errors, and dispatches them to OnError
func handlePanic(b *Bot, r any, c *Ctx) {
	if r == nil {
		return
	}
	err, ok := r.(error)
	if !ok {
		err = fmt.Errorf("recovered panic: %v", r)
	}
	if b.OnError != nil {
		b.OnError(err, c)
	}
}

// Recovery is a global middleware that catches panics in the request processing pipeline safely
func Recovery() Handler {
	return func(c *Ctx) {
		defer func() {
			if r := recover(); r != nil {
				handlePanic(c.Bot, r, c)
			}
		}()
		c.Next()
	}
}

// AntiForward deletes any forwarded messages from non-admin members automatically
func AntiForward(warnDuration time.Duration, customMsg ...string) Handler {
	return func(c *Ctx) {
		if c.Message != nil && (c.Message.ForwardFrom != nil || c.Message.ForwardFromChat != nil) {
			c.Bot.mu.RLock()
			isOwner := c.Message.From.ID == c.Bot.MaintenanceAdminID
			c.Bot.mu.RUnlock()
			isAdmin, _ := c.Chat().IsAdmin().Go()
			if isOwner || isAdmin {
				c.Next()
				return
			}
			_ = c.Del().Go()

			warn := "⚠️ ارسال پیام‌های بازارسال شده (Forward) در این گروه ممنوع است!"
			if len(customMsg) > 0 && customMsg[0] != "" {
				warn = customMsg[0]
				warn = strings.ReplaceAll(warn, "{name}", c.Message.From.Mention())
			}

			_, _ = c.Send().
				Text(warn).
				Temp(warnDuration).
				Go()
			c.Abort()
			return
		}
		c.Next()
	}
}

// AntiProfanity automatically deletes messages containing banned words with custom warnings
func AntiProfanity(warnDuration time.Duration, bannedWords []string, customMsg ...string) Handler {
	// Set default warning text if customMsg is empty
	defaultWarn := "⚠️ کاربر عزیز {name}، ارسال کلمات نامناسب در این گروه مجاز نیست!"
	if len(customMsg) > 0 && customMsg[0] != "" {
		defaultWarn = customMsg[0]
	}

	return func(c *Ctx) {
		if c.Message != nil && c.Message.Text != "" {
			c.Bot.mu.RLock()
			isOwner := c.Message.From.ID == c.Bot.MaintenanceAdminID
			c.Bot.mu.RUnlock()
			isAdmin, _ := c.Chat().IsAdmin().Go()
			if isOwner || isAdmin {
				c.Next()
				return
			}
			text := strings.ToLower(c.Message.Text)
			for _, word := range bannedWords {
				if strings.Contains(text, strings.ToLower(word)) {
					_ = c.Del().Go() // Delete the profanity message

					warn := strings.ReplaceAll(defaultWarn, "{name}", c.Message.From.Mention())
					_, _ = c.Send().
						Text(warn).
						Temp(warnDuration). // Automatically delete after duration
						Go()

					c.Abort()
					return
				}
			}
		}
		c.Next()
	}
}

// MediaType defines specific media categories for filtering
type MediaType string

// Supported media types for strict filtering
const (
	MediaPhoto     MediaType = "photo"
	MediaVideo     MediaType = "video"
	MediaAudio     MediaType = "audio"
	MediaDocument  MediaType = "document"
	MediaVoice     MediaType = "voice"
	MediaSticker   MediaType = "sticker"
	MediaAnimation MediaType = "animation"
	MediaLocation  MediaType = "location"
	MediaContact   MediaType = "contact"
)

// AntiMedia restricts non-admin members from posting selected media types fluidly
func AntiMedia(warnDuration time.Duration, blockedTypes ...MediaType) Handler {
	typesMap := make(map[MediaType]bool)
	for _, t := range blockedTypes {
		typesMap[t] = true
	}
	return func(c *Ctx) {
		if c.Message == nil {
			c.Next()
			return
		}
		c.Bot.mu.RLock()
		isOwner := c.Message.From.ID == c.Bot.MaintenanceAdminID
		c.Bot.mu.RUnlock()
		isAdmin, _ := c.Chat().IsAdmin().Go()
		if isOwner || isAdmin {
			c.Next()
			return
		}
		matched := false
		var matchedType string
		if typesMap[MediaPhoto] && len(c.Message.Photo) > 0 {
			matched = true
			matchedType = "تصویر"
		} else if typesMap[MediaVideo] && c.Message.Video != nil {
			matched = true
			matchedType = "ویدیو"
		} else if typesMap[MediaAudio] && c.Message.Audio != nil {
			matched = true
			matchedType = "فایل صوتی"
		} else if typesMap[MediaDocument] && c.Message.Document != nil {
			matched = true
			matchedType = "سند (فایل)"
		} else if typesMap[MediaVoice] && c.Message.Voice != nil {
			matched = true
			matchedType = "پیام صوتی (وویس)"
		} else if typesMap[MediaSticker] && c.Message.Sticker != nil {
			matched = true
			matchedType = "استیکر"
		} else if typesMap[MediaAnimation] && c.Message.Animation != nil {
			matched = true
			matchedType = "گیف (انیمیشن)"
		} else if typesMap[MediaLocation] && c.Message.Location != nil {
			matched = true
			matchedType = "موقعیت مکانی"
		} else if typesMap[MediaContact] && c.Message.Contact != nil {
			matched = true
			matchedType = "مخاطب"
		}
		if matched {
			_ = c.Del().Go()
			_, _ = c.Send().
				Text(fmt.Sprintf("⚠️ ارسال رسانه از نوع [%s] در این گروه مجاز نیست!", matchedType)).
				Temp(warnDuration).
				Go()
			c.Abort()
			return
		}
		c.Next()
	}
}

type repeatState struct {
	mu       sync.Mutex
	lastText string
	lastTime time.Time
}

// AntiRepeat deletes duplicate identical messages sent by the same user in sequence thread-safely
func AntiRepeat(warnDuration time.Duration, customMsg ...string) Handler {
	var users sync.Map
	return func(c *Ctx) {
		if c.Message == nil || c.Message.From == nil || c.Message.Text == "" {
			c.Next()
			return
		}
		userID := c.Message.From.ID
		text := strings.TrimSpace(c.Message.Text)
		c.Bot.mu.RLock()
		isOwner := userID == c.Bot.MaintenanceAdminID
		c.Bot.mu.RUnlock()
		isAdmin, _ := c.Chat().IsAdmin().Go()
		if isOwner || isAdmin {
			c.Next()
			return
		}
		val, _ := users.LoadOrStore(userID, &repeatState{})
		rs := val.(*repeatState)
		rs.mu.Lock()
		isDuplicate := rs.lastText == text && time.Since(rs.lastTime) < 1*time.Minute
		rs.lastText = text
		rs.lastTime = time.Now()
		rs.mu.Unlock()
		if isDuplicate {
			_ = c.Del().Go()

			warn := fmt.Sprintf("⚠️ کاربر عزیز %s، ارسال پیام تکراری و کپی‌پست در این گروه ممنوع است!", c.Message.From.Mention())
			if len(customMsg) > 0 && customMsg[0] != "" {
				warn = customMsg[0]
				warn = strings.ReplaceAll(warn, "{name}", c.Message.From.Mention())
			}

			_, _ = c.Send().
				Text(warn).
				Temp(warnDuration).
				Go()
			c.Abort()
			return
		}
		c.Next()
	}
}

// raidTracker monitors recent join intervals for group chats
type raidTracker struct {
	mu        sync.Mutex
	joinTimes []time.Time
}

// AntiRaid detects rapid member joins and automatically locks the group locally inside GOB DB
func AntiRaid(limit int, window time.Duration) Handler {
	var groupTrackers sync.Map // maps chatID (int64) -> *raidTracker
	return func(c *Ctx) {
		id, err := c.ChatID()
		if err != nil {
			c.Next()
			return
		}
		now := time.Now()

		val, _ := groupTrackers.LoadOrStore(id, &raidTracker{})
		tracker := val.(*raidTracker)

		tracker.mu.Lock()
		// Filter out historical timestamps
		var cleanList []time.Time
		for _, t := range tracker.joinTimes {
			if now.Sub(t) < window {
				cleanList = append(cleanList, t)
			}
		}

		// Append new timestamps
		for range c.Message.NewChatMembers {
			cleanList = append(cleanList, now)
		}
		tracker.joinTimes = cleanList
		currentCount := len(cleanList)
		tracker.mu.Unlock()

		// Trigger Panic Shield and lock group automatically if join rate exceeds limit
		if currentCount > limit {
			dbKey := fmt.Sprintf("group_lock_%d", id)
			val, ok := c.DB().Get(dbKey).Go()
			alreadyLocked := false
			if ok {
				alreadyLocked, _ = val.(bool)
			}

			if !alreadyLocked {
				// Lock the group locally inside GOB database atomically
				_ = c.DB().Set(dbKey, true).Go()

				// Announce the emergency lockdown event fluidly with markdown
				_, _ = c.Send().
					Text("⚠️ *[سپر اضطراری ربات]* حمله ربات‌های تبلیغاتی (Raid Attack) شناسایی شد! گروه به طور خودکار برای امنیت چت قفل گردید.").
					Markdown().
					Go()
			}
			c.Abort()
			return
		}
		c.Next()
	}
}

// AntiCharLimit deletes messages exceeding the character limit from non-admin members automatically
func AntiCharLimit(limit int, warnDuration time.Duration, customMsg ...string) Handler {
	return func(c *Ctx) {
		if c.Message != nil && c.Message.Text != "" {
			c.Bot.mu.RLock()
			isOwner := c.Message.From.ID == c.Bot.MaintenanceAdminID
			c.Bot.mu.RUnlock()
			isAdmin, _ := c.Chat().IsAdmin().Go()
			if isOwner || isAdmin {
				c.Next()
				return
			}
			if len([]rune(c.Message.Text)) > limit {
				_ = c.Del().Go()

				warn := fmt.Sprintf("⚠️ کاربر عزیز %s، ارسال متون طولانی و بنرهای تبلیغاتی مجاز نیست! (سقف مجاز: %d کاراکتر)", c.Message.From.Mention(), limit)
				if len(customMsg) > 0 && customMsg[0] != "" {
					warn = customMsg[0]
					warn = strings.ReplaceAll(warn, "{name}", c.Message.From.Mention())
					warn = strings.ReplaceAll(warn, "{limit}", strconv.Itoa(limit))
				}

				_, _ = c.Send().
					Text(warn).
					Temp(warnDuration).
					Go()
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// AntiSpamProfile inspects newly joined members' profiles and bans them if spam keywords are matched
func AntiSpamProfile(banOnMatch bool, bannedKeywords []string) Handler {
	return func(c *Ctx) {
		if c.Message == nil || len(c.Message.NewChatMembers) == 0 {
			c.Next()
			return
		}
		for _, user := range c.Message.NewChatMembers {
			fullName := strings.ToLower(user.FirstName + " " + user.LastName)
			username := strings.ToLower(user.Username)
			matched := false
			matchedKeyword := ""
			for _, word := range bannedKeywords {
				w := strings.ToLower(word)
				if strings.Contains(fullName, w) || strings.Contains(username, w) {
					matched = true
					matchedKeyword = word
					break
				}
			}
			if matched {
				c.Bot.Log().Warn("Detected spam profile on join: UserID=%d, Keyword=%q", user.ID, matchedKeyword).Go()
				if banOnMatch {
					_ = c.Chat().Ban(user.ID).Go()
					_, _ = c.Send().
						Text(fmt.Sprintf("🚨 *[سپر هوشمند]* کاربر فیک با نام کاربری نامناسب [%s] در بدو ورود شناسایی و مسدود گردید.", user.FirstName)).
						Markdown().
						Temp(10 * time.Second).
						Go()
				}
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// AntiSelfBot detects and bans automated user accounts (self-bots) that post immediately after joining
func AntiSelfBot(minInterval time.Duration) Handler {
	return func(c *Ctx) {
		if c.Message == nil || c.Message.From == nil || c.IsPrivate() {
			c.Next()
			return
		}
		userID := c.Message.From.ID
		chatID, err := c.ChatID()
		if err != nil {
			c.Next()
			return
		}

		// Check if there is a recorded join time for this user in this group
		joinKey := fmt.Sprintf("join_time_%d_%d", chatID, userID)
		val, ok := c.DB().Get(joinKey).Go()
		if ok {
			if joinTimeNs, ok := val.(int64); ok && joinTimeNs > 0 {
				elapsed := time.Since(time.Unix(0, joinTimeNs))

				// If they posted a message faster than the minInterval (e.g. 3 seconds), they are a self-bot!
				if elapsed < minInterval {
					c.Bot.mu.RLock()
					isOwner := userID == c.Bot.MaintenanceAdminID
					c.Bot.mu.RUnlock()
					isAdmin, _ := c.Chat().IsAdmin().Go()

					if !isOwner && !isAdmin {
						// Delete the spam message and ban the self-bot instantly
						_ = c.Del().Go()
						_ = c.Chat().Ban(userID).Go()
						_ = c.DB().Del(joinKey).Go()

						c.Bot.Log().Warn("Self-bot detected! User %d posted within %v of joining. Banned.", userID, elapsed).Go()

						c.Abort()
						return
					}
				}
				// Clean up the key after they safely pass the first message threshold to optimize DB size
				_ = c.DB().Del(joinKey).Go()
			}
		}
		c.Next()
	}
}

// AntiNight restricts non-admin members from sending any messages during specified night hours
func AntiNight(startHour, endHour int, warnDuration time.Duration, customMsg ...string) Handler {
	return func(c *Ctx) {
		if c.Message == nil {
			c.Next()
			return
		}
		now := time.Now()
		hour := now.Hour()
		isNight := false
		if startHour > endHour {
			isNight = hour >= startHour || hour < endHour
		} else {
			isNight = hour >= startHour && hour < endHour
		}
		if isNight {
			c.Bot.mu.RLock()
			isOwner := c.Message.From.ID == c.Bot.MaintenanceAdminID
			c.Bot.mu.RUnlock()
			isAdmin, _ := c.Chat().IsAdmin().Go()
			if !isOwner && !isAdmin {
				_ = c.Del().Go()

				warn := fmt.Sprintf("⚠️ گفتگو در ساعات خاموشی شبانه گروه (%02d:00 الی %02d:00) ممنوع است!", startHour, endHour)
				if len(customMsg) > 0 && customMsg[0] != "" {
					warn = customMsg[0]
					warn = strings.ReplaceAll(warn, "{start}", fmt.Sprintf("%02d:00", startHour))
					warn = strings.ReplaceAll(warn, "{end}", fmt.Sprintf("%02d:00", endHour))
				}

				_, _ = c.Send().
					Text(warn).
					Temp(warnDuration).
					Go()
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// AntiCaps deletes messages containing excessive uppercase English letters (shouting) from non-admins
func AntiCaps(thresholdPercent float64, minLength int, warnDuration time.Duration) Handler {
	return func(c *Ctx) {
		if c.Message != nil && c.Message.Text != "" {
			c.Bot.mu.RLock()
			isOwner := c.Message.From.ID == c.Bot.MaintenanceAdminID
			c.Bot.mu.RUnlock()
			isAdmin, _ := c.Chat().IsAdmin().Go()
			if isOwner || isAdmin {
				c.Next()
				return
			}
			runes := []rune(c.Message.Text)
			if len(runes) >= minLength {
				totalLetters := 0
				upperLetters := 0
				for _, r := range runes {
					if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
						totalLetters++
						if r >= 'A' && r <= 'Z' {
							upperLetters++
						}
					}
				}
				if totalLetters > 0 {
					percent := (float64(upperLetters) / float64(totalLetters)) * 100.0
					if percent >= thresholdPercent {
						_ = c.Del().Go()
						_, _ = c.Send().
							Text(fmt.Sprintf("⚠️ کاربر عزیز %s، ارسال پیام با حروف بزرگ پی‌درپی (فریاد زدن) در این گروه ممنوع است!", c.Message.From.FirstName)).
							Temp(warnDuration).
							Go()
						c.Abort()
						return
					}
				}
			}
		}
		c.Next()
	}
}

// MandatoryAddGuard restricts non-admin members from posting unless they have invited a minimum number of users
func MandatoryAddGuard(defaultLimit int) Handler {
	return func(c *Ctx) {
		// Bypass callback queries (button clicks)
		if c.Update != nil && c.Update.CallbackQuery != nil {
			c.Next()
			return
		}

		// Bypass service messages, joins, exits, or messages without a valid sender
		if c.Message == nil || c.Message.From == nil || len(c.Message.NewChatMembers) > 0 || c.Message.LeftChatMember != nil {
			c.Next()
			return
		}

		if c.IsGroup() {
			id, err := c.ChatID()
			if err == nil {
				// Query if the group has custom mandatory add limit in database, otherwise fallback to defaultLimit
				limitKey := fmt.Sprintf("mandatory_add_limit_%d", id)
				valLimit, okLimit := c.DB().Get(limitKey).Go()
				limit := defaultLimit
				if okLimit {
					if l, ok := valLimit.(int); ok {
						limit = l
					}
				}

				// Execute guard only if the limit is set greater than 0
				if limit > 0 {
					isAdmin, errAdmin := c.Chat().IsAdmin().Go()
					if errAdmin == nil {
						if !isAdmin {
							senderID := c.SenderID()
							userInvitesKey := fmt.Sprintf("invites_%d_%d", id, senderID)

							valInvites, okInvites := c.DB().Get(userInvitesKey).Go()
							invites := 0
							if okInvites {
								if i, ok := valInvites.(int); ok {
									invites = i
								}
							}

							// If user invites count is less than required limit, delete message and block pipeline
							if invites < limit {
								_ = c.Del().Go()

								report := Text().
									Line("⚠️ *[اد اجباری]* کاربر عزیز {name}، برای چت در این گروه باید ابتدا اعضا را دعوت کنید!").
									Line().
									Line("📊 آمار دعوت‌های شما: {count} از {limit} نفر").
									Bind("name", c.Message.From.Mention()).
									Bind("count", invites).
									Bind("limit", limit).
									Go()

								_, _ = c.Send().Text(report).Markdown().Temp(5 * time.Second).Go()
								c.Abort()
								return
							}
						}
					}
				}
			}
		}
		c.Next()
	}
}

// Pipe combines multiple middleware handlers into one and aborts if any fails
func Pipe(middlewares ...Handler) Handler {
	return func(c *Ctx) {
		for _, mw := range middlewares {
			aborted := false
			original := c.index

			mw(c)

			if c.index >= int8(len(c.handlers)) {
				aborted = true
			}

			if aborted {
				c.index = int8(len(c.handlers))
				return
			}

			c.index = original
		}
		c.Next()
	}
}

// CallbackRateLimit restricts callback query intervals per user using token bucket
func CallbackRateLimit(rate, capacity float64, onLimit Handler) Handler {
	var limiters sync.Map
	return func(c *Ctx) {
		if c.Update == nil || c.Update.CallbackQuery == nil {
			c.Next()
			return
		}
		userID := c.Update.CallbackQuery.From.ID

		val, _ := limiters.LoadOrStore(userID, &tokenBucket{
			tokens:     capacity,
			lastRefill: time.Now(),
			rate:       rate,
			cap:        capacity,
		})
		tb := val.(*tokenBucket)
		if !tb.allow() {
			if tb.shouldWarn(3*time.Second) && onLimit != nil {
				onLimit(c)
			}
			c.Abort()
			return
		}
		c.Next()
	}
}

// EditHistory saves message text with timestamp to local DB to track modifications
func EditHistory(dbPath string, keepDuration time.Duration) Handler {
	db := NewDatabase(dbPath)

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			now := time.Now().UnixNano()
			db.mu.Lock()
			var keysToDelete []string
			for k, v := range db.store {
				if valStr, ok := v.(string); ok {
					parts := strings.SplitN(valStr, "|", 2)
					if len(parts) == 2 {
						var exp int64
						_, err := fmt.Sscanf(parts[0], "%d", &exp)
						if err == nil && now > exp {
							keysToDelete = append(keysToDelete, k)
						}
					}
				}
			}
			db.mu.Unlock()
			for _, k := range keysToDelete {
				_ = db.Del(k)
			}
		}
	}()

	return func(c *Ctx) {
		if c.Update == nil {
			c.Next()
			return
		}

		if c.Update.Message != nil && c.Update.Message.Text != "" {
			key := fmt.Sprintf("msg:%d:%d", c.Update.Message.Chat.ID, c.Update.Message.MessageID)
			val := fmt.Sprintf("%d|%s", time.Now().Add(keepDuration).UnixNano(), c.Update.Message.Text)
			_ = db.Set(key, val)
		} else if c.Update.EditedMessage != nil {
			key := fmt.Sprintf("msg:%d:%d", c.Update.EditedMessage.Chat.ID, c.Update.EditedMessage.MessageID)
			val, ok := db.Get(key)
			if ok {
				if valStr, okStr := val.(string); okStr {
					parts := strings.SplitN(valStr, "|", 2)
					if len(parts) == 2 {
						c.mu.Lock()
						if c.Keys == nil {
							c.Keys = make(map[string]any)
						}
						c.Keys["_sys_prev_text"] = parts[1]
						c.mu.Unlock()
					}
				}
			}

			valNew := fmt.Sprintf("%d|%s", time.Now().Add(keepDuration).UnixNano(), c.Update.EditedMessage.Text)
			_ = db.Set(key, valNew)
		}

		c.Next()
	}
}

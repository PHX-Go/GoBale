package gobale

import (
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Dynamic thread-safe and lock-free cache to store banned words per group chat ID
var profanityCache atomic.Value

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
	if now.Sub(t.lastWarn) >= cooldown {
		t.lastWarn = now
		return true
	}
	return false
}

// ChatRateLimit restricts chat message intervals using token bucket.
// Idle chat buckets are swept periodically to avoid unbounded memory growth.
func ChatRateLimit(rate, capacity float64, onLimit Handler) Handler {
	var limiters sync.Map

	// Background sweeper: drops buckets untouched for over 1 hour
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			limiters.Range(func(key, value any) bool {
				tb := value.(*tokenBucket)
				tb.mu.Lock()
				idle := time.Since(tb.lastRefill) > time.Hour
				tb.mu.Unlock()
				if idle {
					limiters.Delete(key)
				}
				return true
			})
		}
	}()

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

// AntiSpam prevents rapid message flood anomalies with customizable alerts or dynamic WarnEngine
func AntiSpam(engine *WarnEngine, limit int, window time.Duration, warnMsg ...string) Handler {
	defaultWarn := "⚠️ کاربر عزیز {name}، لطفاً از ارسال پیام‌های پی‌درپی خودداری کنید!"
	if len(warnMsg) > 0 && warnMsg[0] != "" {
		defaultWarn = warnMsg[0]
	}

	return func(c *Ctx) {
		if c.Update != nil && c.Update.CallbackQuery != nil {
			c.Next()
			return
		}
		if c.Message == nil || c.Message.From == nil {
			c.Next()
			return
		}
		userID := c.Message.From.ID
		chatID, err := c.ChatID()
		if err != nil {
			c.Next()
			return
		}

		c.Bot.mu.RLock()
		isOwner := userID == c.Bot.MaintenanceAdminID
		c.Bot.mu.RUnlock()

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

		// Safely fetch or store dynamic rate limits inside BotCache
		key := fmt.Sprintf("antispam:%d:%d", chatID, userID)
		cache := c.Bot.cache
		cache.mu.Lock()
		var ul *userLimit
		if item, ok := cache.store[key]; ok && time.Now().Before(item.expiresAt) {
			ul = item.value.(*userLimit)
			item.expiresAt = time.Now().Add(window * 2)
		} else {
			ul = &userLimit{}
			cache.store[key] = &cacheItem{
				value:     ul,
				expiresAt: time.Now().Add(window * 2),
			}
		}
		cache.mu.Unlock()

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
			_ = c.Del().Go()
			if count == activeLimit+1 {
				if engine != nil {
					_ = engine.Warn(c, "ارسال پیام‌های متوالی و اسپم")
				} else {
					warn := defaultWarn
					if activeShield {
						warn = "🚨 *[سپر دفاعی فعال]* اسپم متوالی شناسایی شد! پیام‌ها به طور خودکار حذف می‌شوند."
					} else {
						warn = strings.ReplaceAll(warn, "{name}", c.Message.From.Mention())
					}
					_, _ = c.Send().Text(warn).Markdown().Temp(5 * time.Second).Go()
				}
			}
			c.Abort()
			return
		}
		c.Next()
	}
}

// AntiLink deletes unwanted links and advertisements from chat messages, supporting optional WarnEngine
func AntiLink(engine *WarnEngine, warnDuration time.Duration, customMsg string, customTLDs ...string) Handler {
	tlds := []string{"com", "ir", "net", "org", "co", "ble\\.ir"}
	tlds = append(tlds, customTLDs...)
	pattern := fmt.Sprintf(`(?i)(https?://)?([a-zA-Z0-9-]+\.)+(%s)(/[^\s]*)?`, strings.Join(tlds, "|"))
	regex := regexp.MustCompile(pattern)

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
				_ = c.Del().Go()

				if engine != nil {
					_ = engine.Warn(c, "ارسال لینک و تبلیغات غیرمجاز")
				} else {
					warn := strings.ReplaceAll(defaultWarn, "{name}", c.Message.From.Mention())
					_, _ = c.Send().Text(warn).Temp(warnDuration).Go()
				}

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
	return func(c *Ctx) {
		c.Bot.mu.RLock()
		defer c.Bot.mu.RUnlock()
		if c.SenderID() != c.Bot.MaintenanceAdminID {
			warn := "⚠️ این بخش فقط مخصوص مالک ربات است."
			if len(customMsg) > 0 && customMsg[0] != "" {
				warn = customMsg[0]
			}
			_, _ = c.Send().Text(warn).Temp(5 * time.Second).Go()
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

		lockKey := fmt.Sprintf("group_lock_%d", chatID)
		if val, ok := db.Get(lockKey); ok {
			if locked, okBool := val.(bool); okBool && locked {
				_ = c.Del().Go()
				_, _ = c.Send().Text("⚠️ چت گروه در حال حاضر توسط مدیریت قفل شده است.").Temp(5 * time.Second).Go()
				c.Abort()
				return
			}
		}

		captchaKey := fmt.Sprintf("captcha_mute_%d_%d", chatID, senderID)
		if val, ok := db.Get(captchaKey); ok {
			if isMuted, ok := val.(bool); ok && isMuted {
				_ = c.Del().Go()
				c.Abort()
				return
			}
		}

		blockedTypes := make(map[string]bool)
		c.Bot.mu.RLock()
		for _, entry := range c.Bot.settings {
			if entry.IsLocal {
				dbKey := fmt.Sprintf("group_config_%d_%s", chatID, entry.Key)
				if val, ok := db.Get(dbKey); ok {
					if active, okBool := val.(bool); okBool && active {
						switch entry.Key {
						case "g_lock":
							blockedMapKey := string(MediaAll)
							blockedTypes[blockedMapKey] = true
						case "g_lock_photo":
							blockedTypes[string(MediaPhoto)] = true
						case "g_lock_voice":
							blockedTypes[string(MediaVoice)] = true
						case "g_lock_video":
							blockedTypes[string(MediaVideo)] = true
						case "g_lock_sticker":
							blockedTypes[string(MediaSticker)] = true
						}
					}
				}
			}
		}
		c.Bot.mu.RUnlock()

		groupKey := fmt.Sprintf("blocked_media_group_%d", chatID)
		if groupVal, okGroup := db.Get(groupKey); okGroup {
			if blockedSlice, okSlice := groupVal.([]string); okSlice {
				for _, b := range blockedSlice {
					blockedTypes[b] = true
				}
			}
		}

		userKey := fmt.Sprintf("blocked_media_%d_%d", chatID, senderID)
		if userVal, okUser := db.Get(userKey); okUser {
			if blockedSlice, okSlice := userVal.([]string); okSlice {
				for _, b := range blockedSlice {
					blockedTypes[b] = true
				}
			}
		}

		if len(blockedTypes) == 0 {
			c.Next()
			return
		}

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
			isBlocked := blockedTypes[string(detected)] || blockedTypes[string(MediaAll)]
			if isBlocked {
				_ = c.Del().Go()

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

	// Publish system error event to the central EventBus asynchronously
	if b != nil && b.Bus != nil {
		b.Bus.Publish("sys.error", err)
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

// AntiForward deletes any forwarded messages from non-admin members automatically, supporting optional WarnEngine and hidden privacy bypass
func AntiForward(engine *WarnEngine, warnDuration time.Duration, customMsg ...string) Handler {
	return func(c *Ctx) {
		// Use modern ForwardOrigin alongside legacy fields to catch 100% of forwards (including hidden users)
		if c.Message != nil && (c.Message.ForwardOrigin != nil || c.Message.ForwardFrom != nil || c.Message.ForwardFromChat != nil) {
			c.Bot.mu.RLock()
			isOwner := c.Message.From.ID == c.Bot.MaintenanceAdminID
			c.Bot.mu.RUnlock()
			isAdmin, _ := c.Chat().IsAdmin().Go()
			if isOwner || isAdmin {
				c.Next()
				return
			}
			_ = c.Del().Go() // Delete the forwarded message

			if engine != nil {
				// Resolve the exact origin type (user, channel, chat, hidden_user) for detailed WAL logging
				originType := "نامشخص"
				if c.Message.ForwardOrigin != nil {
					originType = c.Message.ForwardOrigin.Type
				}
				_ = engine.Warn(c, fmt.Sprintf("ارسال پیام بازارسال شده (%s)", originType))
			} else {
				warn := "⚠️ ارسال پیام‌های بازارسال شده (Forward) در این گروه ممنوع است!"
				if len(customMsg) > 0 && customMsg[0] != "" {
					warn = customMsg[0]
					warn = strings.ReplaceAll(warn, "{name}", c.Message.From.Mention())
				}
				_, _ = c.Send().Text(warn).Temp(warnDuration).Go()
			}
			c.Abort()
			return
		}
		c.Next()
	}
}

// AntiProfanity deletes messages containing banned words dynamically loaded from the atomic cache
func AntiProfanity(engine *WarnEngine, warnDuration time.Duration, customMsg ...string) Handler {
	defaultWarn := "⚠️ کاربر عزیز {name}، ارسال کلمات نامناسب در این گروه مجاز نیست!"
	if len(customMsg) > 0 && customMsg[0] != "" {
		defaultWarn = customMsg[0]
	}

	return func(c *Ctx) {
		// Bypass and ignore all callback queries to prevent scanning bot's own menus
		if c.Update != nil && c.Update.CallbackQuery != nil {
			c.Next()
			return
		}

		if c.Message == nil || c.Message.Text == "" {
			c.Next()
			return
		}

		chatID, err := c.ChatID()
		if err != nil {
			c.Next()
			return
		}

		// Bypass global administrators and bot owner
		c.Bot.mu.RLock()
		isOwner := c.Message.From.ID == c.Bot.MaintenanceAdminID
		c.Bot.mu.RUnlock()
		isAdmin, _ := c.Chat().IsAdmin().Go()
		if isOwner || isAdmin {
			c.Next()
			return
		}

		// Retrieve banned words lock-free from atomic cache
		bannedWords := GetBannedWords(c, chatID)
		if len(bannedWords) == 0 {
			c.Next()
			return
		}

		text := strings.ToLower(c.Message.Text)
		matched := false
		matchedWord := ""

		for _, word := range bannedWords {
			if strings.Contains(text, strings.ToLower(word)) {
				matched = true
				matchedWord = word
				break
			}
		}

		if matched {
			_ = c.Del().Go()

			if engine != nil {
				_ = engine.Warn(c, fmt.Sprintf("استفاده از کلمه ممنوعه: %s", matchedWord))
			} else {
				warn := strings.ReplaceAll(defaultWarn, "{name}", c.Message.From.Mention())
				_, _ = c.Send().Text(warn).Temp(warnDuration).Go()
			}
			c.Abort()
			return
		}
		c.Next()
	}
}

// localAsInt64 converts interface values to int64 safely inside middleware scope
func localAsInt64(val any) (int64, bool) {
	switch v := val.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	}
	return 0, false
}

// GetBannedWords returns the isolated banned words list for a chat, loading from group GOB DB on demand
func GetBannedWords(c *Ctx, chatID int64) []string {
	rawMap := profanityCache.Load()
	var m map[int64][]string
	if rawMap == nil {
		m = make(map[int64][]string)
	} else {
		m = rawMap.(map[int64][]string)
	}

	if words, ok := m[chatID]; ok {
		return words
	}

	// Read directly from isolated group file: data/Bad Words/<id>.gob
	words := readGroupBannedWords(chatID)

	// Perform safe atomic Copy-on-Write swap to prevent concurrent map write races
	newMap := make(map[int64][]string)
	for k, v := range m {
		newMap[k] = v
	}
	newMap[chatID] = words
	profanityCache.Store(newMap)

	return words
}

// UpdateBannedWords updates GOB database and performs atomic lock-free cache reload
func UpdateBannedWords(c *Ctx, chatID int64, words []string) {
	// Write directly and atomically to data/Bad Words/<id>.gob
	_ = writeGroupBannedWords(chatID, words)

	rawMap := profanityCache.Load()
	var m map[int64][]string
	if rawMap == nil {
		m = make(map[int64][]string)
	} else {
		m = rawMap.(map[int64][]string)
	}

	// Swap atomic cache with freshly allocated map structure
	newMap := make(map[int64][]string)
	for k, v := range m {
		newMap[k] = v
	}
	newMap[chatID] = words
	profanityCache.Store(newMap)
}

const wordsPerPage = 10

// RenderWordsPage formats the words list into a clean report with nav buttons
func RenderWordsPage(c *Ctx, chatID int64, page int) (string, any) {
	words := GetBannedWords(c, chatID)
	totalPages := (len(words) + wordsPerPage - 1) / wordsPerPage
	if totalPages == 0 {
		totalPages = 1
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * wordsPerPage
	end := start + wordsPerPage
	if end > len(words) {
		end = len(words)
	}

	// Fetch target chat metadata dynamically using the native Chat Chain APIs
	chatTitle := "ناشناخته"
	chatInfo, err := c.Bot.Chat(chatID).Info().Go()
	if err == nil && chatInfo != nil {
		if chatInfo.Title != "" {
			chatTitle = chatInfo.Title
		} else if chatInfo.FirstName != "" {
			chatTitle = chatInfo.FirstName
		}
	}

	// Construct clean, formatted Persian header utilizing standard Text builder
	report := Text().
		Line("🛡️ *لیست کلمات ممنوعه* 📖 *صفحه:* *{page} از {total}*").
		Line("📢 *گروه:* {title}").
		Line("🆔 *شناسه:* `{chat_id}`").
		Line().
		Bind("title", chatTitle).
		Bind("chat_id", chatID).
		Bind("page", page+1).
		Bind("total", totalPages)

	// Display empty or filled list state formally
	if len(words) == 0 {
		report.Line("⚠️ لیست کلمات ممنوعه این گروه در حال حاضر خالی است.")
	} else {
		pageWords := words[start:end]
		for i, w := range pageWords {
			report.Line(fmt.Sprintf("  %d) `%s`", start+i+1, w))
		}
	}

	markupBuilder := InlineMarkup()
	var row []any

	// Calculate looping prev and next pages
	prevPage := (page - 1 + totalPages) % totalPages
	nextPage := (page + 1) % totalPages

	// Append right navigation arrow if total pages exceed one
	if totalPages > 1 {
		row = append(row, Btn("▶️").Callback(fmt.Sprintf("_profanity_page:%d", nextPage)))
	}

	// Symmetrical action buttons: Delete, Edit, Add
	row = append(row, Btn("🗑").Callback(fmt.Sprintf("_profanity_act:delete:%d", page)))
	row = append(row, Btn("✏️").Callback(fmt.Sprintf("_profanity_act:edit:%d", page)))
	row = append(row, Btn("🆕").Callback(fmt.Sprintf("_profanity_act:add:%d", page)))

	// Append left navigation arrow if total pages exceed one
	if totalPages > 1 {
		row = append(row, Btn("◀️").Callback(fmt.Sprintf("_profanity_page:%d", prevPage)))
	}

	markupBuilder.Row(row...)

	// Append a standalone close button row at the bottom of the inline keyboard
	markupBuilder.Row(Btn("❌ بستن منو").Callback("_profanity_act:close:0"))

	return report.Go(), markupBuilder.Build()
}

// BadWordsCommand handles the unified /badwords remote and local command
func BadWordsCommand() Handler {
	return func(c *Ctx) {
		args, ok := c.Arg().([]string)
		var targetID int64
		var err error

		if ok && len(args) > 0 {
			targetID, err = strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				_, _ = c.Send().Text("❌ شناسه گروه وارد شده نامعتبر است.").Go()
				return
			}
		} else {
			targetID, err = c.ChatID()
			if err != nil {
				return
			}
		}

		// Delete the user command trigger in groups to prevent clutter
		if !c.IsPrivate() {
			_ = c.Del().Go()
		}

		sess := c.Session()

		// Delete any previously left hanging menu to maintain a clean chat
		oldIDVal, errOld := sess.Data("menu_msg_id").Go()
		if errOld == nil && oldIDVal != nil {
			if oldID, okInt := localAsInt64(oldIDVal); okInt && oldID > 0 {
				_ = c.Bot.BaseRequest(context.Background(), "deleteMessage", map[string]any{
					"chat_id":    targetID,
					"message_id": oldID,
				}, nil)
			}
		}

		// Store target chat target inside the user session memory
		_, _ = sess.Data("managed_chat_id", targetID).Go()

		text, markup := RenderWordsPage(c, targetID, 0)
		msg, errSend := c.Send().Text(text).Markup(markup).Markdown().Go()
		if errSend == nil && msg != nil {
			// Save the new menu message ID to the session
			_, _ = sess.Data("menu_msg_id", msg.MessageID).Go()
		}
	}
}

// WordsCommand handles the /words command to display the textual pagination list
func WordsCommand() Handler {
	return func(c *Ctx) {
		chatID, _ := c.ChatID()
		text, markup := RenderWordsPage(c, chatID, 0)

		send := c.Send().Text(text).Markdown()
		if markup != nil {
			send = send.Markup(markup)
		}
		_, _ = send.Go()
	}
}

// WordsCallback handles interactive transition clicks for looping page arrows
func WordsCallback() Handler {
	return func(c *Ctx) {
		var page int
		_ = c.ScanCallbackArgs(&page)

		sess := c.Session()
		targetChatVal, _ := sess.Data("managed_chat_id").Go()
		targetChatID, _ := localAsInt64(targetChatVal)
		if targetChatID == 0 {
			targetChatID, _ = c.ChatID()
		}

		text, markup := RenderWordsPage(c, targetChatID, page)
		_, _ = c.Edit().Text(text).Markup(markup).Markdown().Go()
		_ = c.Answer().Go()
	}
}

// BadWordsActionCallback routes emoji button clicks to their corresponding FSM states with full resource cleanup
func BadWordsActionCallback() Handler {
	return func(c *Ctx) {
		var action string
		var page int
		_ = c.ScanCallbackArgs(&action, &page)

		sess := c.Session()
		_, _ = sess.Data("profanity_page", page).Go()

		// Capture the exact current interaction chat ID for safe GUI deletions
		chatID, errChat := c.ChatID()
		if errChat != nil {
			return
		}

		_ = c.Answer().Go()

		// Clean up any previously remaining prompt messages directly from the active chat
		if promptVal, errPrompt := sess.Data("prompt_msg_id").Go(); errPrompt == nil && promptVal != nil {
			if promptID, okInt := localAsInt64(promptVal); okInt && promptID > 0 {
				_ = c.Bot.BaseRequest(context.Background(), "deleteMessage", map[string]any{
					"chat_id":    chatID,
					"message_id": promptID,
				}, nil)
			}
			_, _ = sess.Data("prompt_msg_id", 0).Go()
		}

		switch action {
		case "add":
			_, _ = sess.State("wait_add_word").Go()
			msg, _ := c.Send().Text("🆕 لطفاً کلمه‌ای که می‌خواهید به لیست ممنوعه‌ها اضافه کنید را ارسال کنید:").Go()
			if msg != nil {
				_, _ = sess.Data("prompt_msg_id", msg.MessageID).Go()
			}
		case "edit":
			_, _ = sess.State("wait_edit_word").Go()
			msg, _ := c.Send().Text("✏️ لطفاً برای ویرایش، اطلاعات را به این فرم ارسال کنید:\n`[شماره]: [کلمه_جدید]`\nمثال: `12: سلام`").Markdown().Go()
			if msg != nil {
				_, _ = sess.Data("prompt_msg_id", msg.MessageID).Go()
			}
		case "delete":
			_, _ = sess.State("wait_delete_word").Go()
			msg, _ := c.Send().Text("🗑 لطفاً شماره ردیف کلمه‌ای که می‌خواهید حذف شود را ارسال کنید:").Go()
			if msg != nil {
				_, _ = sess.Data("prompt_msg_id", msg.MessageID).Go()
			}
		case "close":
			// Delete any remaining prompt instruction message from the active chat
			if promptVal, errPrompt := sess.Data("prompt_msg_id").Go(); errPrompt == nil && promptVal != nil {
				if promptID, okInt := localAsInt64(promptVal); okInt && promptID > 0 {
					_ = c.Bot.BaseRequest(context.Background(), "deleteMessage", map[string]any{
						"chat_id":    chatID,
						"message_id": promptID,
					}, nil)
				}
			}

			// Delete the active menu message from the group chat
			_ = c.Del().Go()

			// Fully reset all active FSM states and session metadata
			_, _ = sess.State("").Go()
			_, _ = sess.Data("menu_msg_id", 0).Go()
			_, _ = sess.Data("prompt_msg_id", 0).Go()
			_, _ = sess.Data("profanity_page", 0).Go()
			_, _ = sess.Data("active_admin_id", 0).Go()

			// Send a temporary self-destroying close notification to keep chat clean
			_, _ = c.Send().Text("🔒 منو با موفقیت بسته شد.").Temp(5 * time.Second).Go()
		}
	}
}

// AddWordState handles wait_add_word and keeps user in state on validation failure
func AddWordState() Handler {
	return func(c *Ctx) {
		sess := c.Session()

		// Bypass and ignore messages sent by other standard group members silently
		activeAdminVal, _ := sess.Data("active_admin_id").Go()
		activeAdminID, _ := localAsInt64(activeAdminVal)
		if activeAdminID > 0 && c.SenderID() != activeAdminID {
			c.Next()
			return
		}

		word := strings.TrimSpace(c.Text())

		// Delete the user's input message immediately to prevent group chat clutter
		_ = c.Del().Go()

		// Capture the exact current interaction chat ID for safe GUI deletions
		chatID, _ := c.ChatID()

		targetChatVal, _ := sess.Data("managed_chat_id").Go()
		targetChatID, _ := localAsInt64(targetChatVal)
		if targetChatID == 0 {
			targetChatID = chatID
		}

		// Keep active state if word is empty
		if word == "" {
			return
		}

		words := GetBannedWords(c, targetChatID)

		// Prevent duplicate entries - keeps the active state on validation failure
		for _, w := range words {
			if strings.EqualFold(w, word) {
				_, _ = c.Send().Text("⚠️ این کلمه قبلاً در لیست ممنوعه‌ها ثبت شده است. لطفاً کلمه دیگری ارسال کنید:").Temp(5 * time.Second).Go()
				return
			}
		}

		// Clear interactive state only after input validation succeeds
		_, _ = sess.State("").Go()
		_, _ = sess.Data("active_admin_id", 0).Go()

		// Delete any remaining prompt instruction messages from the active chat
		if promptVal, errPrompt := sess.Data("prompt_msg_id").Go(); errPrompt == nil && promptVal != nil {
			if promptID, okInt := localAsInt64(promptVal); okInt && promptID > 0 {
				_ = c.Bot.BaseRequest(context.Background(), "deleteMessage", map[string]any{
					"chat_id":    chatID,
					"message_id": promptID,
				}, nil)
			}
			_, _ = sess.Data("prompt_msg_id", 0).Go()
		}

		words = append(words, word)
		UpdateBannedWords(c, targetChatID, words)

		// Delete the old menu message from the active chat
		oldIDVal, errOld := sess.Data("menu_msg_id").Go()
		if errOld == nil && oldIDVal != nil {
			if oldID, okInt := localAsInt64(oldIDVal); okInt && oldID > 0 {
				_ = c.Bot.BaseRequest(context.Background(), "deleteMessage", map[string]any{
					"chat_id":    chatID,
					"message_id": oldID,
				}, nil)
			}
		}

		pageVal, _ := sess.Data("profanity_page").Go()
		page, _ := pageVal.(int)
		text, markup := RenderWordsPage(c, targetChatID, page)

		// Send success status which self-destructs after 5 seconds
		_, _ = c.Send().Text(fmt.Sprintf("✅ کلمه `%s` با موفقیت به لیست اضافه شد.", word)).Markdown().Temp(5 * time.Second).Go()

		// Send the new menu and save its ID to the session
		msg, errSend := c.Send().Text(text).Markup(markup).Markdown().Go()
		if errSend == nil && msg != nil {
			_, _ = sess.Data("menu_msg_id", msg.MessageID).Go()
		}
	}
}

// EditWordState handles wait_edit_word and keeps user in state on validation failure
func EditWordState() Handler {
	return func(c *Ctx) {
		sess := c.Session()

		// Bypass and ignore messages sent by other standard group members silently
		activeAdminVal, _ := sess.Data("active_admin_id").Go()
		activeAdminID, _ := localAsInt64(activeAdminVal)
		if activeAdminID > 0 && c.SenderID() != activeAdminID {
			c.Next()
			return
		}

		input := strings.TrimSpace(c.Text())

		// Delete the user's input message immediately to prevent group chat clutter
		_ = c.Del().Go()

		// Capture the exact current interaction chat ID for safe GUI deletions
		chatID, _ := c.ChatID()

		targetChatVal, _ := sess.Data("managed_chat_id").Go()
		targetChatID, _ := localAsInt64(targetChatVal)
		if targetChatID == 0 {
			targetChatID = chatID
		}

		parts := strings.SplitN(input, ":", 2)
		if len(parts) < 2 {
			_, _ = c.Send().Text("⚠️ فرمت ورودی نامعتبر است! باید به این صورت ارسال کنید:\n`[شماره]: [کلمه_جدید]`").Temp(5 * time.Second).Go()
			return
		}

		numStr := strings.TrimSpace(parts[0])
		newWord := strings.TrimSpace(parts[1])

		num, err := strconv.Atoi(numStr)
		if err != nil || num <= 0 || newWord == "" {
			_, _ = c.Send().Text("❌ قالب ورودی یا شماره وارد شده معتبر نیست. لطفاً مجدداً تلاش کنید:").Temp(5 * time.Second).Go()
			return
		}

		words := GetBannedWords(c, targetChatID)
		idx := num - 1

		if idx < 0 || idx >= len(words) {
			_, _ = c.Send().Text("❌ شماره ردیف وارد شده در لیست وجود ندارد. لطفاً شماره معتبر وارد کنید:").Temp(5 * time.Second).Go()
			return
		}

		// Clear interactive state only after input validation succeeds
		_, _ = sess.State("").Go()
		_, _ = sess.Data("active_admin_id", 0).Go()

		// Delete any remaining prompt instruction messages from the active chat
		if promptVal, errPrompt := sess.Data("prompt_msg_id").Go(); errPrompt == nil && promptVal != nil {
			if promptID, okInt := localAsInt64(promptVal); okInt && promptID > 0 {
				_ = c.Bot.BaseRequest(context.Background(), "deleteMessage", map[string]any{
					"chat_id":    chatID,
					"message_id": promptID,
				}, nil)
			}
			_, _ = sess.Data("prompt_msg_id", 0).Go()
		}

		// Delete the old menu message from the active chat
		oldIDVal, errOld := sess.Data("menu_msg_id").Go()
		if errOld == nil && oldIDVal != nil {
			if oldID, okInt := localAsInt64(oldIDVal); okInt && oldID > 0 {
				_ = c.Bot.BaseRequest(context.Background(), "deleteMessage", map[string]any{
					"chat_id":    chatID,
					"message_id": oldID,
				}, nil)
			}
		}

		oldWord := words[idx]
		words[idx] = newWord
		UpdateBannedWords(c, targetChatID, words)

		pageVal, _ := sess.Data("profanity_page").Go()
		page, _ := pageVal.(int)
		text, markup := RenderWordsPage(c, targetChatID, page)

		// Send success status which self-destructs after 5 seconds
		_, _ = c.Send().Text(fmt.Sprintf("📝 کلمه `%s` با کلمه جدید `%s` در ردیف %d با موفقیت ویرایش شد.", oldWord, newWord, num)).Markdown().Temp(5 * time.Second).Go()

		// Send the new menu and save its ID to the session
		msg, errSend := c.Send().Text(text).Markup(markup).Markdown().Go()
		if errSend == nil && msg != nil {
			_, _ = sess.Data("menu_msg_id", msg.MessageID).Go()
		}
	}
}

// DeleteWordState handles wait_delete_word and keeps user in state on validation failure
func DeleteWordState() Handler {
	return func(c *Ctx) {
		sess := c.Session()

		// Bypass and ignore messages sent by other standard group members silently
		activeAdminVal, _ := sess.Data("active_admin_id").Go()
		activeAdminID, _ := localAsInt64(activeAdminVal)
		if activeAdminID > 0 && c.SenderID() != activeAdminID {
			c.Next()
			return
		}

		input := strings.TrimSpace(c.Text())

		// Delete the user's input message immediately to prevent group chat clutter
		_ = c.Del().Go()

		// Capture the exact current interaction chat ID for safe GUI deletions
		chatID, _ := c.ChatID()

		targetChatVal, _ := sess.Data("managed_chat_id").Go()
		targetChatID, _ := localAsInt64(targetChatVal)
		if targetChatID == 0 {
			targetChatID = chatID
		}

		num, err := strconv.Atoi(input)
		if err != nil || num <= 0 {
			_, _ = c.Send().Text("❌ شماره ردیف وارد شده معتبر نیست. لطفاً مجدداً شماره معتبر بفرستید:").Temp(5 * time.Second).Go()
			return
		}

		words := GetBannedWords(c, targetChatID)
		idx := num - 1

		if idx < 0 || idx >= len(words) {
			_, _ = c.Send().Text("❌ شماره ردیف در لیست وجود ندارد. لطفاً شماره ردیف معتبر وارد کنید:").Temp(5 * time.Second).Go()
			return
		}

		// Clear interactive state only after input validation succeeds
		_, _ = sess.State("").Go()
		_, _ = sess.Data("active_admin_id", 0).Go()

		// Delete any remaining prompt instruction messages from the active chat
		if promptVal, errPrompt := sess.Data("prompt_msg_id").Go(); errPrompt == nil && promptVal != nil {
			if promptID, okInt := localAsInt64(promptVal); okInt && promptID > 0 {
				_ = c.Bot.BaseRequest(context.Background(), "deleteMessage", map[string]any{
					"chat_id":    chatID,
					"message_id": promptID,
				}, nil)
			}
			_, _ = sess.Data("prompt_msg_id", 0).Go()
		}

		// Delete the old menu message from the active chat
		oldIDVal, errOld := sess.Data("menu_msg_id").Go()
		if errOld == nil && oldIDVal != nil {
			if oldID, okInt := localAsInt64(oldIDVal); okInt && oldID > 0 {
				_ = c.Bot.BaseRequest(context.Background(), "deleteMessage", map[string]any{
					"chat_id":    chatID,
					"message_id": oldID,
				}, nil)
			}
		}

		removedWord := words[idx]
		words = append(words[:idx], words[idx+1:]...)
		UpdateBannedWords(c, targetChatID, words)

		pageVal, _ := sess.Data("profanity_page").Go()
		page, _ := pageVal.(int)
		text, markup := RenderWordsPage(c, targetChatID, page)

		// Send success status which self-destructs after 5 seconds
		_, _ = c.Send().Text(fmt.Sprintf("✅ کلمه `%s` (ردیف %d) با موفقیت حذف شد.", removedWord, num)).Markdown().Temp(5 * time.Second).Go()

		// Send the new menu and save its ID to the session
		msg, errSend := c.Send().Text(text).Markup(markup).Markdown().Go()
		if errSend == nil && msg != nil {
			_, _ = sess.Data("menu_msg_id", msg.MessageID).Go()
		}
	}
}

// getGroupDBPath resolves and creates the isolated data/Bad Words/<chatID>.gob path recursively
func getGroupDBPath(chatID int64) string {
	_ = os.MkdirAll(DataPath("Bad Words"), 0755)
	return DataPath(filepath.Join("Bad Words", fmt.Sprintf("%d.gob", chatID)))
}

// readGroupBannedWords reads []string directly from data/Bad Words/<id>.gob
func readGroupBannedWords(chatID int64) []string {
	path := getGroupDBPath(chatID)
	file, err := os.Open(path)
	if err != nil {
		return []string{}
	}
	defer file.Close()

	var words []string
	_ = gob.NewDecoder(file).Decode(&words)
	return words
}

// writeGroupBannedWords writes []string atomically to data/Bad Words/<id>.gob
func writeGroupBannedWords(chatID int64, words []string) error {
	path := getGroupDBPath(chatID)
	tmp := path + ".tmp"

	file, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	err = gob.NewEncoder(file).Encode(words)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return err
	}

	_ = file.Sync()
	_ = file.Close()

	return os.Rename(tmp, path)
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

// AntiMedia restricts non-admin members from posting selected media types, supporting optional WarnEngine
func AntiMedia(engine *WarnEngine, warnDuration time.Duration, blockedTypes ...MediaType) Handler {
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

			if engine != nil {
				_ = engine.Warn(c, fmt.Sprintf("ارسال رسانه غیرمجاز (%s)", matchedType))
			} else {
				_, _ = c.Send().
					Text(fmt.Sprintf("⚠️ ارسال رسانه از نوع [%s] در این گروه مجاز نیست!", matchedType)).
					Temp(warnDuration).
					Go()
			}
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

// AntiRepeat deletes duplicate identical messages, supporting optional WarnEngine
func AntiRepeat(engine *WarnEngine, warnDuration time.Duration, customMsg ...string) Handler {
	return func(c *Ctx) {
		if c.Message == nil || c.Message.From == nil || c.Message.Text == "" {
			c.Next()
			return
		}
		userID := c.Message.From.ID
		chatID, err := c.ChatID()
		if err != nil {
			c.Next()
			return
		}
		text := strings.TrimSpace(c.Message.Text)

		c.Bot.mu.RLock()
		isOwner := userID == c.Bot.MaintenanceAdminID
		c.Bot.mu.RUnlock()

		isAdmin, _ := c.Chat().IsAdmin().Go()
		if isOwner || isAdmin {
			c.Next()
			return
		}

		// Safely fetch or store duplicate detection states inside BotCache
		key := fmt.Sprintf("antirepeat:%d:%d", chatID, userID)
		cache := c.Bot.cache
		cache.mu.Lock()
		var rs *repeatState
		if item, ok := cache.store[key]; ok && time.Now().Before(item.expiresAt) {
			rs = item.value.(*repeatState)
			item.expiresAt = time.Now().Add(5 * time.Minute)
		} else {
			rs = &repeatState{}
			cache.store[key] = &cacheItem{
				value:     rs,
				expiresAt: time.Now().Add(5 * time.Minute),
			}
		}
		cache.mu.Unlock()

		rs.mu.Lock()
		isDuplicate := rs.lastText == text && time.Since(rs.lastTime) < 1*time.Minute
		rs.lastText = text
		rs.lastTime = time.Now()
		rs.mu.Unlock()

		if isDuplicate {
			_ = c.Del().Go()

			if engine != nil {
				_ = engine.Warn(c, "ارسال پیام تکراری و کپی‌پیست متوالی")
			} else {
				warn := fmt.Sprintf("⚠️ کاربر عزیز %s، ارسال پیام تکراری و کپی‌پیست در این گروه ممنوع است!", c.Message.From.Mention())
				if len(customMsg) > 0 && customMsg[0] != "" {
					warn = customMsg[0]
					warn = strings.ReplaceAll(warn, "{name}", c.Message.From.Mention())
				}
				_, _ = c.Send().Text(warn).Temp(warnDuration).Go()
			}
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
	return func(c *Ctx) {
		id, err := c.ChatID()
		if err != nil {
			c.Next()
			return
		}
		now := time.Now()

		// Safely fetch or store raid group trackers inside BotCache
		key := fmt.Sprintf("antiraid:%d", id)
		cache := c.Bot.cache
		cache.mu.Lock()
		var tracker *raidTracker
		if item, ok := cache.store[key]; ok && time.Now().Before(item.expiresAt) {
			tracker = item.value.(*raidTracker)
			item.expiresAt = time.Now().Add(window * 2)
		} else {
			tracker = &raidTracker{}
			cache.store[key] = &cacheItem{
				value:     tracker,
				expiresAt: time.Now().Add(window * 2),
			}
		}
		cache.mu.Unlock()

		tracker.mu.Lock()
		var cleanList []time.Time
		for _, t := range tracker.joinTimes {
			if now.Sub(t) < window {
				cleanList = append(cleanList, t)
			}
		}

		for range c.Message.NewChatMembers {
			cleanList = append(cleanList, now)
		}
		tracker.joinTimes = cleanList
		currentCount := len(cleanList)
		tracker.mu.Unlock()

		if currentCount > limit {
			dbKey := fmt.Sprintf("group_lock_%d", id)
			val, ok := c.DB().Get(dbKey).Go()
			alreadyLocked := false
			if ok {
				alreadyLocked, _ = val.(bool)
			}

			if !alreadyLocked {
				_ = c.DB().Set(dbKey, true).Go()
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

// AntiCharLimit deletes messages exceeding the character limit, supporting optional WarnEngine integration
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
				// Log structural profiles variables sequentially
				c.Bot.Log().Warn("Detected spam profile on join").
					Int64("user_id", user.ID).
					Str("keyword", matchedKeyword).
					Go()

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

		joinKey := fmt.Sprintf("join_time_%d_%d", chatID, userID)
		val, ok := c.DB().Get(joinKey).Go()
		if ok {
			if joinTimeNs, ok := val.(int64); ok && joinTimeNs > 0 {
				elapsed := time.Since(time.Unix(0, joinTimeNs))

				if elapsed < minInterval {
					c.Bot.mu.RLock()
					isOwner := userID == c.Bot.MaintenanceAdminID
					c.Bot.mu.RUnlock()
					isAdmin, _ := c.Chat().IsAdmin().Go()

					if !isOwner && !isAdmin {
						_ = c.Del().Go()
						_ = c.Chat().Ban(userID).Go()
						_ = c.DB().Del(joinKey).Go()

						// Log automated self-bot properties sequentially
						c.Bot.Log().Warn("Self-bot detected! User posted within minInterval of joining. Banned.").
							Int64("user_id", userID).
							Any("elapsed", elapsed).
							Go()
						c.Abort()
						return
					}
				}
				_ = c.DB().Del(joinKey).Go()
			}
		}
		c.Next()
	}
}

// AntiNight restricts non-admin members from sending any messages during specified night hours, supporting optional WarnEngine
func AntiNight(engine *WarnEngine, startHour, endHour int, warnDuration time.Duration, customMsg ...string) Handler {
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

				if engine != nil {
					_ = engine.Warn(c, "گفتگو در ساعات خاموشی شبانه گروه")
				} else {
					warn := fmt.Sprintf("⚠️ گفتگو در ساعات خاموشی شبانه گروه (%02d:00 الی %02d:00) ممنوع است!", startHour, endHour)
					if len(customMsg) > 0 && customMsg[0] != "" {
						warn = customMsg[0]
						warn = strings.ReplaceAll(warn, "{start}", fmt.Sprintf("%02d:00", startHour))
						warn = strings.ReplaceAll(warn, "{end}", fmt.Sprintf("%02d:00", endHour))
					}
					_, _ = c.Send().Text(warn).Temp(warnDuration).Go()
				}
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// AntiCaps deletes messages containing excessive uppercase English letters, supporting optional WarnEngine
func AntiCaps(engine *WarnEngine, thresholdPercent float64, minLength int, warnDuration time.Duration) Handler {
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

						if engine != nil {
							_ = engine.Warn(c, "ارسال پیام با حروف بزرگ پی‌درپی")
						} else {
							_, _ = c.Send().
								Text(fmt.Sprintf("⚠️ کاربر عزیز %s، ارسال پیام با حروف بزرگ پی‌درپی در این گروه ممنوع است!", c.Message.From.FirstName)).
								Temp(warnDuration).
								Go()
						}
						c.Abort()
						return
					}
				}
			}
		}
		c.Next()
	}
}

// MandatoryAddGuard restricts non-admin members from posting unless they have invited a minimum number of users, supporting optional WarnEngine
func MandatoryAddGuard(engine *WarnEngine, defaultLimit int) Handler {
	return func(c *Ctx) {
		if c.Update != nil && c.Update.CallbackQuery != nil {
			c.Next()
			return
		}
		if c.Message == nil || c.Message.From == nil || len(c.Message.NewChatMembers) > 0 || c.Message.LeftChatMember != nil {
			c.Next()
			return
		}

		if c.IsGroup() {
			id, err := c.ChatID()
			if err == nil {
				limitKey := fmt.Sprintf("mandatory_add_limit_%d", id)
				valLimit, okLimit := c.DB().Get(limitKey).Go()
				limit := defaultLimit
				if okLimit {
					if l, ok := valLimit.(int); ok {
						limit = l
					}
				}

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
								} else if i, ok := valInvites.(int64); ok {
									invites = int(i)
								}
							}

							if invites < limit {
								_ = c.Del().Go()

								if engine != nil {
									_ = engine.Warn(c, fmt.Sprintf("عدم دعوت از حداقل %d کاربر جدید به گروه", limit))
								} else {
									report := Text().
										Line("⚠️ *[اد اجباری]* کاربر عزیز {name}، برای چت در این گروه باید ابتدا اعضا را دعوت کنید!").
										Line().
										Line("📊 آمار دعوت‌های شما: {count} از {limit} نفر").
										Bind("name", c.Message.From.Mention()).
										Bind("count", invites).
										Bind("limit", limit).
										Go()
									_, _ = c.Send().Text(report).Markdown().Temp(5 * time.Second).Go()
								}
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
		now := time.Now()
		val, _ := limiters.LoadOrStore(userID, &tokenBucket{
			tokens:     capacity,
			lastRefill: now,
			rate:       rate,
			cap:        capacity,
		})
		tb := val.(*tokenBucket)
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

// FLUENT PERMISSION & LOCATION GUARDS

// MemberRole defines the hierarchical role of a chat member
type MemberRole string

const (
	RoleOwner   MemberRole = "owner"   // Global bot creator/owner
	RoleAdmin   MemberRole = "admin"   // Group administrator or creator
	RoleRegular MemberRole = "regular" // Standard chat member
)

// ChatLoc defines the physical location type of a chat
type ChatLoc string

const (
	LocPrivate    ChatLoc = "private"    // Bot direct message PV
	LocGroup      ChatLoc = "group"      // Regular chat group
	LocSuperGroup ChatLoc = "supergroup" // Supergroup chat
	LocChannel    ChatLoc = "channel"    // Channel chat
)

// Roles restricts the command execution to specific member roles (e.g., owner, admin, regular)
func (r *RouteChain) Roles(roles ...MemberRole) *RouteChain {
	r.Guard(func(c *Ctx) bool {
		var senderRole MemberRole = RoleRegular

		// Resolve sender role hierarchically
		if c.IsOwner() {
			senderRole = RoleOwner
		} else {
			isAdmin, err := c.Chat().IsAdmin().Go()
			if err == nil && isAdmin {
				senderRole = RoleAdmin
			}
		}

		// Validate if sender role matches the authorized scopes
		for _, role := range roles {
			if senderRole == role {
				return true
			}
			// Owner implicitly inherits administrator privileges
			if role == RoleAdmin && senderRole == RoleOwner {
				return true
			}
		}

		// If unauthorized, send a temporary 5-second warning alert
		_, _ = c.Send().Text("⚠️ شما دسترسی لازم را ندارید.").Temp(5 * time.Second).Go()
		return false
	})
	return r
}

// Locs restricts the command execution to specific chat locations (e.g., private, group, supergroup, channel)
func (r *RouteChain) Locs(locs ...ChatLoc) *RouteChain {
	r.Guard(func(c *Ctx) bool {
		var currentLoc ChatLoc

		// Resolve current chat location type safely
		if c.IsPrivate() {
			currentLoc = LocPrivate
		} else if c.IsSuperGroup() {
			currentLoc = LocSuperGroup
		} else if c.IsGroup() {
			currentLoc = LocGroup
		} else if c.IsChannel() {
			currentLoc = LocChannel
		}

		// Validate if current location is allowed
		for _, loc := range locs {
			if currentLoc == loc {
				return true
			}
		}

		// If location is unauthorized, send a temporary 5-second warning alert
		_, _ = c.Send().Text("⚠️ این دستور در این نوع محیط گفتگو قابل اجرا نیست.").Temp(5 * time.Second).Go()
		return false
	})
	return r
}

// ReplyGuard filters and deletes group replies when the reply lock is enabled in settings
func ReplyGuard(warnDuration time.Duration, customMsg ...string) Handler {
	// Set default shamsi fallback warning message
	defaultWarn := "⚠️ کاربر عزیز {name}، ریپلای در این گروه قفل شده است."
	if len(customMsg) > 0 && customMsg[0] != "" {
		defaultWarn = customMsg[0]
	}

	return func(c *Ctx) {
		// Skip messages that are not sent inside group chats
		if !c.IsGroup() {
			c.Next()
			return
		}

		// Skip messages that are not replies
		if c.Message == nil || c.Message.ReplyToMessage == nil {
			c.Next()
			return
		}

		// Bypass checks for group administrators
		isAdmin, _ := c.Chat().IsAdmin().Go()
		if isAdmin {
			c.Next()
			return
		}

		// Bypass checks for the global bot owner
		if c.IsOwner() {
			c.Next()
			return
		}

		// Fetch current group chat ID
		chatID, err := c.ChatID()
		if err != nil {
			c.Next()
			return
		}

		// Retrieve the reply lock toggle value from the GOB database
		dbKey := fmt.Sprintf("group_config_%d_lock_reply", chatID)
		val, ok := c.DB().Get(dbKey).Go()

		// Delete the reply and send temporary alert if lock is active
		if ok {
			if active, okBool := val.(bool); okBool && active {
				_ = c.Del().Go()

				// Only send warning alerts if warnDuration is actively specified
				if warnDuration > 0 {
					warnText := strings.ReplaceAll(defaultWarn, "{name}", c.Message.From.Mention())
					_, _ = c.Send().Text(warnText).Temp(warnDuration).Go()
				}
				c.Abort()
				return
			}
		}

		c.Next()
	}
}
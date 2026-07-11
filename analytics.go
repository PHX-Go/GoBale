package gobale

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PeriodType defines the temporal scope of the analytics report
type PeriodType string

const (
	PeriodDaily    PeriodType = "daily"
	PeriodLifetime PeriodType = "lifetime"
)

// UserScore represents the sorted active chatter data structure
type UserScore struct {
	UserID int64
	Score  int64
}

// AnalyticsResult contains the raw enriched metrics for complete custom design
type AnalyticsResult struct {
	ChatID        int64
	Period        PeriodType
	TextCount     int64
	WordCount     int64
	CharCount     int64
	ReplyCount    int64
	ForwardCount  int64
	EditCount     int64
	DeleteCount   int64
	PhotoCount    int64
	VideoCount    int64
	VoiceCount    int64
	AudioCount    int64
	DocCount      int64
	StickerCount  int64
	AnimCount     int64
	LocationCount int64
	ContactCount  int64
	CommandCount  int64
	TotalMedia    int64
	TotalMsgs     int64
	PeakHour      int
	PeakHourMsgs  int64
}

var analyticsOnce sync.Once

// Compiled regex pattern to identify raw domains (e.g. google.com) and secure/insecure link layouts
var rxLinkPattern = regexp.MustCompile(`(?i)(https?://)?([a-zA-Z0-9-]+\.)+(com|ir|net|org|co|info|biz|cc|me|ble\.ir)(/[^\s]*)?`)

// initAnalyticsDB dynamically instantiates the analytics GOB database only when activated
func (b *Bot) initAnalyticsDB() {
	analyticsOnce.Do(func() {
		if b.analyticsDB == nil {
			b.analyticsDB = NewDatabase(DataPath("gobale_analytics.gob"))
		}
	})
}

// incrementAnalyticsCount safely increments a GOB database counter thread-safely
func (b *Bot) incrementAnalyticsCount(key string, delta int64) {
	db := b.analyticsDB
	if db == nil {
		return
	}
	current := int64(0)
	if val, ok := db.Get(key); ok {
		if iVal, okInt := val.(int64); okInt {
			current = iVal
		} else if iVal, okInt := val.(int); okInt {
			current = int64(iVal)
		}
	}
	_ = db.Set(key, current+delta)
}

// AnalyticsLogger is a high-performance, panic-proof global traffic logger middleware
func AnalyticsLogger() Handler {
	return func(c *Ctx) {
		c.Bot.initAnalyticsDB()

		if c.Message == nil || c.Message.From == nil {
			c.Next()
			return
		}

		// Ignore service messages
		if len(c.Message.NewChatMembers) > 0 || c.Message.LeftChatMember != nil {
			c.Next()
			return
		}

		chatID, err := c.ChatID()
		if err != nil {
			c.Next()
			return
		}

		db := c.Bot.analyticsDB
		dbConcrete, ok := db.(*Database)
		if !ok || dbConcrete == nil {
			c.Next()
			return
		}

		now := time.Now()
		currentHour := now.Hour()
		text := c.Message.Text
		userID := c.Message.From.ID

		dbConcrete.mu.Lock()

		inc := func(key string, delta int64) {
			current := int64(0)
			if val, ok := dbConcrete.store[key]; ok {
				if iVal, okInt := val.(int64); okInt {
					current = iVal
				} else if iVal, okInt := val.(int); okInt {
					current = int64(iVal)
				}
			}
			newVal := current + delta
			dbConcrete.store[key] = newVal
			dbConcrete.appendWAL(walEntry{Op: walSet, Key: key, Val: newVal})
		}

		logMetric := func(metric string, delta int64) {
			inc(fmt.Sprintf("stat_daily:%d:%s", chatID, metric), delta)
			inc(fmt.Sprintf("stat_lifetime:%d:%s", chatID, metric), delta)
		}

		// Helper to automatically increment user-specific granular metrics
		logUserMetric := func(metric string, delta int64) {
			inc(fmt.Sprintf("user_daily:%d:%d:%s", chatID, userID, metric), delta)
			inc(fmt.Sprintf("user_lifetime:%d:%d:%s", chatID, userID, metric), delta)
		}

		isEdit := c.Update.EditedMessage != nil
		if isEdit {
			logMetric("edits", 1)
			logUserMetric("edits", 1)
		}

		peakKey := fmt.Sprintf("stat_peak:%d:%d", chatID, currentHour)
		inc(peakKey, 1)

		if text != "" {
			words := int64(len(strings.Fields(text)))
			chars := int64(len([]rune(text)))
			logMetric("words", words)
			logMetric("chars", chars)

			if text[0] == '/' {
				logMetric("command", 1)
				logUserMetric("command", 1)
			}

			hasLink := rxLinkPattern.MatchString(text)
			if hasLink {
				logMetric("links", 1)
			}
		}

		if c.Message.ReplyToMessage != nil {
			logMetric("replies", 1)
			logUserMetric("replies", 1)
		}

		if c.Message.ForwardDate != 0 {
			logMetric("forwards", 1)
			logUserMetric("forwards", 1)
		}

		var detected string
		var isVoiceDoc, isMusicDoc bool
		if c.Message.Document != nil {
			mime := strings.ToLower(c.Message.Document.MimeType)
			ext := strings.ToLower(filepath.Ext(c.Message.Document.FileName))

			isVoiceFormat := strings.Contains(mime, "ogg") || strings.Contains(mime, "opus") || strings.Contains(mime, "amr") || strings.Contains(mime, "3gpp") ||
				ext == ".ogg" || ext == ".oga" || ext == ".opus" || ext == ".amr" || ext == ".3gp" || ext == ".3gpp"

			isMusicFormat := strings.Contains(mime, "mpeg") || strings.Contains(mime, "mp3") || strings.Contains(mime, "m4a") || strings.Contains(mime, "flac") || strings.Contains(mime, "wav") ||
				ext == ".mp3" || ext == ".m4a" || ext == ".flac" || ext == ".wav" || ext == ".wma" || ext == ".aac"

			if strings.HasPrefix(mime, "audio/") {
				if isVoiceFormat {
					isVoiceDoc = true
				} else {
					isMusicDoc = true
				}
			} else {
				if isVoiceFormat {
					isVoiceDoc = true
				} else if isMusicFormat {
					isMusicDoc = true
				}
			}
		}

		if len(c.Message.Photo) > 0 {
			detected = "photo"
		} else if c.Message.Voice != nil || isVoiceDoc {
			detected = "voice"
		} else if c.Message.Sticker != nil {
			detected = "sticker"
		} else if c.Message.Animation != nil {
			detected = "animation"
		} else if c.Message.Video != nil {
			detected = "video"
		} else if c.Message.Audio != nil || isMusicDoc {
			detected = "audio"
		} else if c.Message.Location != nil {
			detected = "location"
		} else if c.Message.Contact != nil {
			detected = "contact"
		} else if c.Message.Document != nil {
			detected = "document"
		}

		if detected != "" {
			logMetric(detected, 1)
			logUserMetric(detected, 1)
		} else if !isEdit {
			logMetric("text", 1)
			logUserMetric("text", 1)
		}

		// Increment user's total active message count (sum of text and media)
		if !isEdit {
			logUserMetric("msgs", 1)
		}

		// Store user's actual name (not username) natively for leaderboard rendering
		fullName := c.Message.From.FirstName
		if c.Message.From.LastName != "" {
			fullName += " " + c.Message.From.LastName
		}
		dbConcrete.store[fmt.Sprintf("user_name:%d", userID)] = fullName

		// Append userID to active group users list if not already present
		activeUsersKey := fmt.Sprintf("active_users:%d", chatID)
		var userList []int64
		if val, ok := dbConcrete.store[activeUsersKey]; ok {
			if list, okSlice := val.([]int64); okSlice {
				userList = list
			}
		}
		found := false
		for _, id := range userList {
			if id == userID {
				found = true
				break
			}
		}
		if !found {
			userList = append(userList, userID)
			dbConcrete.store[activeUsersKey] = userList
			dbConcrete.appendWAL(walEntry{Op: walSet, Key: activeUsersKey, Val: userList})
		}

		dbConcrete.mu.Unlock()

		c.Next()
	}
}

// AnalyticsChain manages fluent retrieval, compilation, and scheduling of statistics
type AnalyticsChain struct {
	bot        *Bot
	ctx        context.Context
	chatID     any
	period     PeriodType
	resetDaily bool
	schedTime  string
	schedTask  func(*AnalyticsResult)
}

// Analytics opens the fluent statistics tracker dot-system from Bot context
func (b *Bot) Analytics() *AnalyticsChain {
	b.initAnalyticsDB()
	return &AnalyticsChain{
		bot:    b,
		ctx:    context.Background(),
		period: PeriodDaily,
	}
}

// Analytics opens the fluent statistics tracker dot-system from Ctx context
func (c *Ctx) Analytics() *AnalyticsChain {
	c.Bot.initAnalyticsDB()
	id, _ := c.ChatID()
	return &AnalyticsChain{
		bot:    c.Bot,
		ctx:    c.ctx,
		chatID: id,
		period: PeriodDaily,
	}
}

// Chat targets a specific group/channel chat
func (a *AnalyticsChain) Chat(chatID any) *AnalyticsChain {
	a.chatID = chatID
	return a
}

// Period sets the temporal period filter
func (a *AnalyticsChain) Period(p PeriodType) *AnalyticsChain {
	a.period = p
	return a
}

// ResetDaily schedules clearing of daily statistics keys after generation
func (a *AnalyticsChain) ResetDaily() *AnalyticsChain {
	a.resetDaily = true
	return a
}

// Schedule registers a daily automated background task to run a custom reporting function
func (a *AnalyticsChain) Schedule(timeStr string, task func(*AnalyticsResult)) *AnalyticsChain {
	a.schedTime = timeStr
	a.schedTask = task
	return a
}

// Go compiles, formats, and dispatches the raw analytics report struct
func (a *AnalyticsChain) Go() (*AnalyticsResult, error) {
	a.bot.initAnalyticsDB()
	resolved := a.bot.ResolveChatID(a.chatID)
	var chatID int64
	switch v := resolved.(type) {
	case int64:
		chatID = v
	case int:
		chatID = int64(v)
	case int32:
		chatID = int64(v)
	default:
		return nil, fmt.Errorf("unable to resolve chat ID")
	}

	db := a.bot.analyticsDB
	if db == nil {
		return nil, fmt.Errorf("analytics database is nil")
	}

	prefix := "stat_daily"
	if a.period == PeriodLifetime {
		prefix = "stat_lifetime"
	}

	// Read analytics counter value with generic conversion helper
	getVal := func(metric string) int64 {
		key := fmt.Sprintf("%s:%d:%s", prefix, chatID, metric)
		if val, ok := db.Get(key); ok {
			if num, okNum := asInt64(val); okNum {
				return num
			}
		}
		return 0
	}

	res := &AnalyticsResult{
		ChatID:        chatID,
		Period:        a.period,
		TextCount:     getVal("text"),
		WordCount:     getVal("words"),
		CharCount:     getVal("chars"),
		ReplyCount:    getVal("replies"),
		ForwardCount:  getVal("forwards"),
		EditCount:     getVal("edits"),
		DeleteCount:   getVal("deletions"),
		PhotoCount:    getVal("photo"),
		VideoCount:    getVal("video"),
		VoiceCount:    getVal("voice"),
		AudioCount:    getVal("audio"),
		DocCount:      getVal("document"),
		StickerCount:  getVal("sticker"),
		AnimCount:     getVal("animation"),
		LocationCount: getVal("location"),
		ContactCount:  getVal("contact"),
		CommandCount:  getVal("command"),
	}

	// Calculate comprehensive metrics totals including new types
	res.TotalMedia = res.PhotoCount + res.VideoCount + res.VoiceCount + res.AudioCount + res.DocCount + res.StickerCount + res.AnimCount + res.LocationCount + res.ContactCount
	res.TotalMsgs = res.TextCount + res.TotalMedia

	// Find the peak active hour with type-coercion
	for h := 0; h < 24; h++ {
		peakKey := fmt.Sprintf("stat_peak:%d:%d", chatID, h)
		if val, ok := db.Get(peakKey); ok {
			var score int64
			switch v := val.(type) {
			case int64:
				score = v
			case int:
				score = int64(v)
			case int32:
				score = int64(v)
			}
			if score > res.PeakHourMsgs {
				res.PeakHourMsgs = score
				res.PeakHour = h
			}
		}
	}

	// If scheduling is requested, register a background Task and return nil struct
	if a.schedTime != "" && a.schedTask != nil {
		parts := strings.Split(a.schedTime, ":")
		if len(parts) == 2 {
			hour, _ := strconv.Atoi(parts[0])
			minute, _ := strconv.Atoi(parts[1])

			reset := a.resetDaily
			period := a.period
			taskFn := a.schedTask

			a.bot.Task().Daily(hour, minute, func() {
				res, err := a.bot.Analytics().
					Chat(chatID).
					Period(period).
					Go()
				if err == nil && res != nil {
					taskFn(res)
				}
				if reset {
					a.purgeDailyStats(chatID)
				}
			})
			return nil, nil
		}
	}

	return res, nil
}

// purgeDailyStats purges daily and hourly keys to start fresh the next day
func (a *AnalyticsChain) purgeDailyStats(chatID int64) {
	db := a.bot.analyticsDB
	dbConcrete, ok := db.(*Database)
	if !ok {
		return
	}

	dbConcrete.mu.Lock()
	defer dbConcrete.mu.Unlock()

	var keysToDel []string
	prefixDaily := fmt.Sprintf("stat_daily:%d:", chatID)
	prefixPeak := fmt.Sprintf("stat_peak:%d:", chatID)
	prefixUserDaily := fmt.Sprintf("user_daily:%d:", chatID) // Clears daily user message count at midnight

	for k := range dbConcrete.store {
		if strings.HasPrefix(k, prefixDaily) || strings.HasPrefix(k, prefixPeak) || strings.HasPrefix(k, prefixUserDaily) {
			keysToDel = append(keysToDel, k)
		}
	}

	for _, k := range keysToDel {
		delete(dbConcrete.store, k)
		dbConcrete.appendWAL(walEntry{Op: walDel, Key: k})
	}
}

// Leaderboard compiles and returns a beautifully formatted Persian RTL report of top N active chatters by specific metric (with remote support)
func (c *Ctx) Leaderboard(metric string, limit int, targetChat any, p ...PeriodType) (string, error) {
	period := PeriodDaily
	if len(p) > 0 {
		period = p[0]
	}

	resolved := c.Bot.ResolveChatID(targetChat)
	if resolved == nil || resolved == "" {
		id, err := c.ChatID()
		if err != nil {
			return "", err
		}
		resolved = c.Bot.ResolveChatID(id)
	}

	// Resolve the dynamic target chat ID safely into int64 for database keys
	var chatID int64
	switch v := resolved.(type) {
	case int64:
		chatID = v
	case int:
		chatID = int64(v)
	case int32:
		chatID = int64(v)
	case string:
		var id int64
		if _, err := fmt.Sscanf(v, "%d", &id); err == nil {
			chatID = id
		}
	}

	if chatID == 0 {
		return "", fmt.Errorf("unable to resolve chat ID as int64")
	}

	db := c.Bot.analyticsDB
	dbConcrete, ok := db.(*Database)
	if !ok || dbConcrete == nil {
		return "", fmt.Errorf("analytics database is not initialized")
	}

	// Normalize and resolve dynamic aliases natively
	metric = strings.ToLower(strings.TrimSpace(metric))
	switch metric {
	case "", "all", "messages":
		metric = "msgs"
	case "gif", "gifs":
		metric = "animation"
	case "doc", "docs", "file", "files":
		metric = "document"
	case "music":
		metric = "audio"
	case "picture", "pic", "pics":
		metric = "photo"
	}

	metricNames := map[string]string{
		"msgs":      "کل پیام‌های ارسالی",
		"text":      "پیام‌های متنی",
		"photo":     "تصاویر ارسالی (Photo)",
		"video":     "ویدیوهای ارسالی (Video)",
		"voice":     "پیام‌های صوتی (Voice)",
		"audio":     "فایل‌های موسیقی (Audio)",
		"document":  "اسناد و فایل‌ها (Document)",
		"sticker":   "استیکرهای ارسالی (Sticker)",
		"animation": "گیف‌های ارسالی (GIF)",
		"location":  "موقعیت‌های مکانی (Location)",
		"contact":   "مخاطبان به اشتراک گذاشته شده",
		"replies":   "ریپلای‌های ارسالی",
		"forwards":  "پیام‌های فوروارد شده",
		"edits":     "پیام‌های ویرایش شده",
		"command":   "دستورات صادر شده",
	}

	unitName, okMetric := metricNames[metric]
	if !okMetric {
		return "⚠️ معیار سنجش وارد شده نامعتبر است.\n\n**معیارهای مجاز:**\n`msgs, text, photo, video, voice, audio, doc, sticker, gif, location, contact, replies, forwards, edits, command`", nil
	}

	dbConcrete.mu.RLock()
	activeUsersKey := fmt.Sprintf("active_users:%d", chatID)
	var userList []int64
	if val, ok := dbConcrete.store[activeUsersKey]; ok {
		if list, okSlice := val.([]int64); okSlice {
			userList = list
		}
	}

	type chatter struct {
		userID int64
		count  int64
		name   string
	}
	var chatters []chatter

	prefix := "user_daily"
	if period == PeriodLifetime {
		prefix = "user_lifetime"
	}

	// Retrieve message counts and cached names dynamically for each active user
	for _, uid := range userList {
		countKey := fmt.Sprintf("%s:%d:%d:%s", prefix, chatID, uid, metric)
		count := int64(0)
		if val, ok := dbConcrete.store[countKey]; ok {
			if iVal, okInt := asInt64(val); okInt {
				count = iVal
			}
		}

		if count > 0 {
			nameKey := fmt.Sprintf("user_name:%d", uid)
			name := fmt.Sprintf("User %d", uid)
			if val, ok := dbConcrete.store[nameKey]; ok {
				if str, okStr := val.(string); okStr {
					name = str
				}
			}
			chatters = append(chatters, chatter{userID: uid, count: count, name: name})
		}
	}
	dbConcrete.mu.RUnlock()

	if len(chatters) == 0 {
		return fmt.Sprintf("📊 لیست برترین‌ها برای فیلد «%s» خالی است.", unitName), nil
	}

	// Sort active chatters in descending order
	sort.Slice(chatters, func(i, j int) bool {
		return chatters[i].count > chatters[j].count
	})

	periodName := "امروز (روزانه)"
	if period == PeriodLifetime {
		periodName = "کل دوره (تا به امروز)"
	}

	// Pass the resolved target chat ID explicitly to ChatTitle to prevent argument clashes
	title := c.ChatTitle(resolved)

	report := Text().
		Line("🏆 **فعال‌ترین کاربران گروه ", title, "**").
		Line("📊 **دوره آمارگیر:** *{period_name}*").
		Line("💬 **معیار سنجش:** *{unit_name}*").
		Line().
		Bind("period_name", periodName).
		Bind("unit_name", unitName)

	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	if len(chatters) < limit {
		limit = len(chatters)
	}

	getRankEmoji := func(rank int) string {
		switch rank {
		case 1:
			return "🥇"
		case 2:
			return "🥈"
		case 3:
			return "🥉"
		case 4:
			return "4️⃣"
		case 5:
			return "5️⃣"
		case 6:
			return "6️⃣"
		case 7:
			return "7️⃣"
		case 8:
			return "8️⃣"
		case 9:
			return "9️⃣"
		case 10:
			return "🔟"
		}
		return fmt.Sprintf("%d.", rank)
	}

	// Unicode BiDi Isolation Constants (LRI, RLI, PDI)
	const (
		unicodeLRI = "\u2066" // Left-to-Right Isolate
		unicodeRLI = "\u2067" // Right-to-Left Isolate
		unicodePDI = "\u2069" // Pop Directional Isolate
	)

	// Compile report rows in LTR layout using Unicode Bidi Isolation
	for i := 0; i < limit; i++ {
		ch := chatters[i]
		emoji := getRankEmoji(i + 1)

		// Wrap the Persian name inside RLI...PDI to isolate its directionality
		isolatedName := fmt.Sprintf("%s%s%s", unicodeRLI, ch.name, unicodePDI)

		// Build Bale specific mention link
		userLink := Link(isolatedName, fmt.Sprintf("uid:%d", ch.userID))

		// Prepend \u2066 (LRI) to completely lock the visual row direction LTR, regardless of Persian names [1.2.2]
		lineText := fmt.Sprintf("%s  %s %s - `%s` %s", unicodeLRI, emoji, userLink, Money(ch.count), unicodePDI)
		report.Line(lineText)
	}

	return report.Go(), nil
}

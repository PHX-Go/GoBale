package gobale

import (
	"context"
	"fmt"
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

// initAnalyticsDB dynamically instantiates the analytics GOB database only when activated
func (b *Bot) initAnalyticsDB() {
	analyticsOnce.Do(func() {
		if b.analyticsDB == nil {
			b.analyticsDB = NewDatabase("gobale_analytics.gob")
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
		// Initialize the database on-demand upon first message activity
		c.Bot.initAnalyticsDB()

		if c.Message == nil || c.Message.From == nil {
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
			dbConcrete.store[key] = current + delta
		}

		// Helper to increment both daily and lifetime keyspaces
		logMetric := func(metric string, delta int64) {
			inc(fmt.Sprintf("stat_daily:%d:%s", chatID, metric), delta)
			inc(fmt.Sprintf("stat_lifetime:%d:%s", chatID, metric), delta)
		}

		// Increment message counters
		isEdit := c.Update.EditedMessage != nil
		if isEdit {
			logMetric("edits", 1)
		} else {
			logMetric("text", 1)
		}

		// Increment peak hourly counters
		peakKey := fmt.Sprintf("stat_peak:%d:%d", chatID, currentHour)
		inc(peakKey, 1)

		// Log rich text behaviors (words, characters, links, and replies)
		if text != "" {
			words := int64(len(strings.Fields(text)))
			chars := int64(len([]rune(text)))
			logMetric("words", words)
			logMetric("chars", chars)

			// Detect if message is a bot command
			if text[0] == '/' {
				logMetric("command", 1)
			}

			hasLink := strings.Contains(text, "http://") || strings.Contains(text, "https://") || strings.Contains(text, "ble.ir/")
			if hasLink {
				logMetric("links", 1)
			}
		}

		if c.Message.ReplyToMessage != nil {
			logMetric("replies", 1)
		}

		// Log incoming forwards securely by checking ForwardDate
		if c.Message.ForwardDate != 0 {
			logMetric("forwards", 1)
		}

		// Detect and log specific media types with strict priority order (Voice and Sticker checked first)
		var detected string
		isMusicFile := c.Message.Document != nil && strings.HasPrefix(c.Message.Document.MimeType, "audio/")

		if len(c.Message.Photo) > 0 {
			detected = "photo"
		} else if c.Message.Voice != nil {
			detected = "voice" // Voice checked before Audio to prevent Bale audio/voice override
		} else if c.Message.Sticker != nil {
			detected = "sticker" // Sticker checked before Document override
		} else if c.Message.Animation != nil {
			detected = "animation" // Animation checked before Document override
		} else if c.Message.Video != nil {
			detected = "video"
		} else if c.Message.Audio != nil || isMusicFile {
			detected = "audio" // Classifies document-audio files safely as audio
		} else if c.Message.Location != nil {
			detected = "location"
		} else if c.Message.Contact != nil {
			detected = "contact"
		} else if c.Message.Document != nil {
			detected = "document" // Generic document fallback checked last
		}

		if detected != "" {
			logMetric(detected, 1)
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

	// Dynamic Type-Coercive helper to safly cast any GOB numeric type to int64
	getVal := func(metric string) int64 {
		key := fmt.Sprintf("%s:%d:%s", prefix, chatID, metric)
		if val, ok := db.Get(key); ok {
			switch v := val.(type) {
			case int64:
				return v
			case int:
				return int64(v)
			case int32:
				return int64(v)
			case float64:
				return int64(v)
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

	for k := range dbConcrete.store {
		if strings.HasPrefix(k, prefixDaily) || strings.HasPrefix(k, prefixPeak) {
			keysToDel = append(keysToDel, k)
		}
	}

	for _, k := range keysToDel {
		delete(dbConcrete.store, k)
	}
}

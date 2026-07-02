# Analytics: Simplified Moderation & Custom Statistics

The `Analytics` system in GoBale manages and processes group chat traffic metrics natively inside a dedicated, isolated GOB database file (`gobale_analytics.gob`). 

To optimize resources, this database is initialized **on-demand** (Lazy Initialization) only when the analytics features are explicitly activated in the bot.

---

## Key Architectural Design

### 1. Isolated Storage Strategy
All metrics are stored in a dedicated database `gobale_analytics.gob`. This ensures the primary database remains lightweight and prevents write lock contention during high-traffic message processing.

### 2. High-Performance Logging (`AnalyticsLogger`)
The traffic logging middleware (`AnalyticsLogger()`) automatically increments daily and lifetime counters in a single-pass write transaction, minimizing lock overhead. It features panic-proof safeguards to immediately bypass unpopulated message structures (such as anonymous channel posts).

### 3. Native Deletion Hook
The native message deletion method (`c.Del().Go()`) automatically triggers the initialization of the analytics database on-demand. Whenever the bot deletes a message (due to spam, links, dynamic locks, or admin purges), the deletion event is recorded under `deletions` in the GOB storage.

---

## API Reference

### 1. Registration (`OnChain`)

| Method | Return Type | Description |
| :--- | :--- | :--- |
| `Use(gobale.AnalyticsLogger())` | `*OnChain` | Appends the traffic logger middleware to the global pipeline. |

### 2. Transaction Chain (`AnalyticsChain`)

Initiated via `bot.Analytics()` or `c.Analytics()` inside handlers.

| Method | Return Type | Description |
| :--- | :--- | :--- |
| `Chat(chatID any)` | `*AnalyticsChain` | Targets a specific group/channel chat. |
| `Period(p PeriodType)` | `*AnalyticsChain` | Sets the temporal period (`PeriodDaily` or `PeriodLifetime`). |
| `ResetDaily()` | `*AnalyticsChain` | Clears the 24-hour daily GOB keyspace after generating the report. |
| `Schedule(time string, task func(*AnalyticsResult))` | `*AnalyticsChain` | Registers a daily background task to execute a custom reporting function. |
| `Go()` | `(*AnalyticsResult, error)` | Compiles and returns the raw metrics struct. |

---

## Supported Simplified Metrics (`AnalyticsResult`)

When calling `stats, err := c.Analytics().Go()`, the returned `AnalyticsResult` struct contains the following raw, GOB-backed metrics:

```go
type AnalyticsResult struct {
	ChatID        int64
	Period        PeriodType
	TextCount     int64
	WordCount     int64
	CharCount     int64
	ReplyCount    int64
	CommandCount  int64
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
	TotalMedia    int64
	TotalMsgs     int64
	PeakHour      int
	PeakHourMsgs  int64
}
```

---

## Setup & Initialization

To run the analytics and security guard, register the `AnalyticsLogger()` global middleware in your `main.go` boot sequence:

```go
package main

import (
	"log"
	"time"

	gobale "github.com/PHX-Go/GoBale"
)

func main() {
	gobale.Env().Go()

	token := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")

	bot, err := gobale.New(token).Admin(adminID).Go()
	if err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}

	// Register the high-performance global traffic logging middleware
	bot.On().Use(gobale.AnalyticsLogger())

	// Initialize the unified ChatGuard middleware
	bot.On().MediaGuard().
		Commands().
		DelCmds().
		Warn(5 * time.Second).
		Go()

	log.Println("Bot is running...")
	bot.Run().Polling().Go()
}
```

---

## Practical Examples

Because the `.Go()` method returns raw data fields, developers have complete control over formatting, language styling, and output layouts.

### Example 1: On-Demand Custom Statistics Command

This scenario registers an admin-only command to compile daily metrics and post a custom, styled Markdown report to the group.

```go
bot.On().Cmd("stats").Do(gobale.AdminsOnly(), func(c *gobale.Ctx) {
	_ = c.Del().Go()

	// Fetch raw daily analytics metrics dynamically from the database
	stats, errStats := c.Analytics().Period(gobale.PeriodDaily).Go()
	if errStats != nil || stats == nil {
		c.Log().Error("Failed to compile stats: %v", errStats).Go()
		return
	}

	// Compile custom detailed report using the native Text builder
	t := gobale.Text().
		Line("📊 **گزارش آماری همه‌جانبه گروه**").
		Line().
		Line("💬 مجموع کل پیام‌ها: {total}").
		Line("📝 پیام‌های متنی ساده: {texts}").
		Line("✍️ مجموع کلمات: {words}").
		Line("🔤 مجموع کاراکترها: {chars}").
		Line("🔄 ریپلای‌ها (تعامل): {replies}").
		Line("✏️ پیام‌های ویرایش‌شده: {edits}").
		Line("🗑️ پیام‌های حذف‌شده (ربات): {deletes}").
		Line("🤖 دستورات اجرا شده: {commands}").
		Line().
		Line("📂 **تفکیک رسانه‌های ارسالی امروز:**").
		Line("  ├ 🖼️ تصویر (Photo): {photos}").
		Line("  ├ 🎬 ویدیو (Video): {videos}").
		Line("  ├ 🎙️ پیام صوتی (Voice): {voices}").
		Line("  ├ 🎵 موسیقی (Audio/Music): {audios}").
		Line("  ├ 📁 سند و فایل (Document): {docs}").
		Line("  ├ 🌟 استیکرها (Sticker): {stickers}").
		Line("  ├ 🎞️ انیمیشن (GIF): {anims}").
		Line("  ├ 📍 موقعیت مکانی (Location): {locations}").
		Line("  └ 👤 مخاطب مشترک شده (Contact): {contacts}").
		Line().
		Line("🕒 **ساعت شلوغی گفتگوها:** ساعت {peak_hour}").
		Bind("total", stats.TotalMsgs).
		Bind("texts", stats.TextCount).
		Bind("words", stats.WordCount).
		Bind("chars", stats.CharCount).
		Bind("replies", stats.ReplyCount).
		Bind("commands", stats.CommandCount).
		Bind("photos", stats.PhotoCount).
		Bind("videos", stats.VideoCount).
		Bind("voices", stats.VoiceCount).
		Bind("audios", stats.AudioCount).
		Bind("docs", stats.DocCount).
		Bind("stickers", stats.StickerCount).
		Bind("anims", stats.AnimCount).
		Bind("locations", stats.LocationCount).
		Bind("contacts", stats.ContactCount).
		Bind("edits", stats.EditCount).
		Bind("deletes", stats.DeleteCount).
		Bind("peak_hour", fmt.Sprintf("%02d:00 الی %02d:00", stats.PeakHour, (stats.PeakHour+1)%24)).
		Go()

	// Send the custom compiled report to the group chat as a 25-second temp message
	_, _ = c.Send().Text(t).Markdown().Temp(25 * time.Second).Go()
})
```

---

### Example 2: Daily Scheduled Report to Owner (Task Scheduling)

This scenario registers a daily automated background task during startup. It compiles metrics for a target group at 23:30 (11:30 PM), dispatches the customized report to the owner's private chat, and clears the daily counters.

```go
bot.On().Start().Do(func() {
	var targetGroupID int64 = 123456789 // Your target group ID
	ownerID := bot.MaintenanceAdminID

	// Auto-compiles metrics daily at 23:30 (11:30 PM) and fires callback
	bot.Analytics().
		Chat(targetGroupID).
		Period(gobale.PeriodDaily).
		ResetDaily(). // Purges daily counters after sending
		Schedule("23:30", func(stats *gobale.AnalyticsResult) {
			report := fmt.Sprintf("📊 **آمار روزانه گروه در تاریخ %s**\n\n💬 کل پیام‌ها: %d\n📝 پیام‌های متنی: %d\n✍️ کلمات: %d\n🗑️ حذف‌شده توسط ربات: %d", 
				gobale.Jalali(time.Now()).Go(), stats.TotalMsgs, stats.TextCount, stats.WordCount, stats.DeleteCount)
			
			_, _ = bot.Send(ownerID).Text(report).Markdown().Go()
		}).
		Go()
})
```

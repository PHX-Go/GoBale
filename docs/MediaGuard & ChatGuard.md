# MediaGuard & ChatGuard: Dynamic Group Lockdown & Settings System

`MediaGuard` is an architectural extension for the **GoBale** framework. It unifies individual user restrictions, group-wide dynamic lockdowns, administrative mutes, and interactive group-specific configuration keyboards under a single-pass, database-backed processing system.

By consolidating media filtering, captcha verification, and administrative locking under the high-performance **`ChatGuard`** middleware, this module optimizes server resources while remaining compatible across standard groups, supergroups, and channels.

---

## Architectural Highlights

* **Consolidated GOB Storage:** Unifies both global setting records and group-specific lock states under the primary GOB database instance (`dbInstance`), completely eliminating the overhead of multiple parallel GOB files.
* **Unified Middleware (`ChatGuard`):** Replaces split verification guards with a single-pass protection pipeline. It evaluates group locks, captcha verification, administrative mutes, and both group/user media restrictions in one synchronous pass.
* **Chat-Isolated Settings (`RegisterLocal`):** Extends the core `.Settings()` module to support local chat-specific toggles (e.g., closing voice notes for Group A while leaving them open in Group B) without global pointer leaks.
* **Private Admin Alerts:** Handlers verify button-click context via `IsAdmin`. Unauthorized users are rejected with a private popup alert (`.Alert()`) instead of cluttering the chat with security warnings.
* **Idempotent / restrict Commands:** Pre-built, auto-adaptive `/restrict` and `/unrestrict` commands automatically detect whether they are being called inside a group (via replies or manual IDs) or remotely in private messages.

---

## Supported Media Categories

* `photo` — Images and photos.
* `video` — Videos.
* `voice` — Voice notes.
* `audio` — Audio files or music.
* `document` — Standard files, documents, and PDFs.
* `sticker` — Dynamic and static stickers.
* `animation` — GIFs and animations.
* `location` — Location share coordinates.
* `contact` — Shared contact cards.
* `all` — Combines all of the above categories.

---

## Setup & Initialization

To run `MediaGuard` and `ChatGuard`, make sure you have applied the codebase modifications to `bot.go`, `types.go`, `settings.go`, `send.go`, and `middleware.go`, and added `media_guard.go` to your package directory.

Then, initialize the system in your `main.go` as shown below:

```go
package main

import (
	"log"
	"time"

	gobale "github.com/PHX-Go/GoBale"
)

var maintenanceMode bool

func main() {
	gobale.Env().Go()

	token := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")

	bot, err := gobale.New(token).Admin(adminID).Go()
	if err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}

	// 1. Register Global settings (applies to the entire bot instance)
	bot.Settings().Register("maintenance", "🛠️ حالت تعمیرات", &maintenanceMode)

	// 2. Register Chat-Isolated settings (toggled independently per group)
	bot.Settings().RegisterLocal("g_lock", "🔒 قفل کل رسانه‌ها", false)
	bot.Settings().RegisterLocal("g_lock_photo", "🖼️ قفل تصاویر", false)
	bot.Settings().RegisterLocal("g_lock_voice", "🎙️ قفل وویس‌ها", false)
	bot.Settings().RegisterLocal("g_lock_sticker", "✨ قفل استیکرها", false)

	// 3. Start MediaGuard with standard commands, command auto-clean, and 5s warnings
	bot.On().MediaGuard().
		Commands().
		DelCmds().
		Warn(5 * time.Second).
		Go()

	// 4. Command to display the dynamic group-isolated settings panel
	bot.On().Cmd("settings").Do(gobale.AdminsOnly(), func(c *gobale.Ctx) {
		_ = c.Del().Go()
		_, _ = c.Send().Text("⚙️ پنل تنظیمات پیشرفته گروه:").Settings().Go()
	})

	log.Println("Bot is running...")
	bot.Run().Polling().Go()
}
```

---

## Setup Chain API Reference (`MediaGuardChain`)

| Method | Return Type | Description |
| :--- | :--- | :--- |
| `Warn(d time.Duration)` | `*MediaGuardChain` | Sets the temporary warning alerts visibility TTL. |
| `Msg(text string)` | `*MediaGuardChain` | Overrides the default violation message template. Supports `{name}` and `{type}` placeholders. |
| `Silent()` | `*MediaGuardChain` | Mutes all warning messages. Prohibited media gets deleted silently. |
| `Commands()` | `*MediaGuardChain` | Registers default administrative commands. |
| `DelCmds()` | `*MediaGuardChain` | Deletes incoming admin command messages after execution. |
| `Go()` | `*OnChain` | Finalizes configurations and appends the unified `ChatGuard` middleware. |

---

## Transaction Chain API Reference (`MediaRestrictChain`)

Initiated via `bot.RestrictMedia(userID)` or `c.RestrictMedia(userID)` inside handlers.

| Method | Return Type | Description |
| :--- | :--- | :--- |
| `Chat(chatID any)` | `*MediaRestrictChain` | Targets a specific group/channel. Auto-resolves username and numeric IDs. |
| `Group()` | `*MediaRestrictChain` | Targets the restriction to the **entire chat group** instead of an individual. |
| `Block(types ...MediaType)` | `*MediaRestrictChain` | Registers selected media types into the database blacklist. |
| `Allow(types ...MediaType)` | `*MediaRestrictChain` | Removes selected media types from the database blacklist. |
| `Clear()` | `*MediaRestrictChain` | Removes all existing restrictions for the specified scope. |
| `Go()` | `error` | Executes the direct database transaction. |

---

## Usage Scenarios & Commands Reference

### Scenario 1: Interactive Settings Keyboard (`/settings`)
Administrators can summon the interactive menu to toggle configurations visually.

* **Command:** `/settings`
* Clicking any button (e.g. `🖼️ قفل تصاویر`) dynamically updates the GOB record `group_config_[chatID]_g_lock_photo`, triggers an in-place edit (`c.Edit()`) showing `🟢 روشن`, and instantly activates the restriction inside `ChatGuard`.
* Any clicks by non-admin members are securely intercepted and answered with a silent popup alert: `❌ تغییر تنظیمات فقط مخصوص مدیران گروه است!`

---

### Scenario 2: Dynamic Group Lockdown
Lock down media categories for all non-admin members of the group.

* **Complete Media Lockdown:**
  Locks down all media formats for the entire group. Users are restricted to standard text messages only.
  ```text
  /restrict
  ```
  *Response:* `🚫 گروه با موفقیت قفل شد. ارسال هرگونه رسانه برای کل اعضا مسدود گردید.`

* **Specific Media Restrictions (e.g., Photos and Videos):**
  ```text
  /restrict photo video
  ```
  *Response:* `🚫 ارسال موارد [photo, video] برای کل اعضای گروه مسدود شد.`

* **Unlocking Group Completely:**
  Clears the entire group-wide media restriction list.
  ```text
  /unrestrict
  ```
  *Response:* `✅ قفل گروه کاملاً باز شد. ارسال تمامی رسانه‌ها مجدداً برای همه آزاد گردید.`

* **Partial Group Unlock (e.g., Allowing Voice Notes again):**
  ```text
  /unrestrict voice
  ```

---

### Scenario 3: Individual Restriction via Reply
Moderate a specific user instantly inside a group chat by replying directly to one of their messages.

* **Prohibit a User from sending Stickers and GIFs:**
  Reply to the user's message with:
  ```text
  /restrict sticker animation
  ```
  *Response:* `🚫 دسترسی کاربر 123456 به موارد [sticker, animation] مسدود شد.`

* **Remove All Access Restrictions for a User:**
  Reply to the user's message with:
  ```text
  /unrestrict
  ```
  *Response:* `✅ تمامی محدودیت‌های رسانه‌ای کاربر 123456 لغو شد.`

---

### Scenario 4: Remote Moderation via Private DM (Bot's PV)
Administrators can moderate remote group chats from the bot's private chat. This requires providing the target **Chat ID** (usually negative) as the first argument, followed by the **User ID** (for individual restrictions) or simply the media keywords (for group-wide restrictions).

* **Example - Block Videos remotely in Group `-100998877` for User `554433`:**
  ```text
  /restrict -100998877 554433 video
  ```
  *Response:* `🚫 دسترسی کاربر 554433 در چت -100998877 به موارد [video] مسدود شد.`

* **Example - Lock Stickers remotely for the entire Group `-100998877`:**
  ```text
  /restrict -100998877 sticker
  ```
  *Response:* `🚫 ارسال موارد [sticker] برای کل اعضای گروه مسدود شد.`

---

### Scenario 5: Programmatic Developer Implementations

#### Programmatic Lockdown from Handler Context:
```go
// Dynamic programmatic group voice note lockdown
bot.On().Cmd("lock_voice").Do(gobale.AdminsOnly(), func(c *gobale.Ctx) {
	// Restrict voice notes for the entire chat natively
	err := c.RestrictMedia(0).Group().Block(gobale.MediaVoice).Go()
	if err != nil {
		c.Log().Error("Failed group voice lockdown: %v", err).Go()
		return
	}

	c.Send().Text("🚫 Media voice restriction applied.").Temp(5 * time.Second).Go()
})
```

#### Usage outside of Handlers (using `*Bot`):
```go
// Direct GOB transaction to lock stickers on group chat
err := bot.RestrictMedia(0).Chat("-10012345678").Group().Block(gobale.MediaSticker).Go()
if err != nil {
    log.Printf("Transaction error: %v", err)
}
```
---

### Custom Warning Message

```go
// Initialize MediaGuard with customized warning and 5-second TTL
bot.On().MediaGuard().
	Commands().
	DelCmds().
	Warn(5 * time.Second). // Warnings auto-delete after 5 seconds
	Msg("🚨 همکار گرامی {name}، ارسال [{type}] در این گروه چت مسدود است!"). // Custom template
	Go()
```

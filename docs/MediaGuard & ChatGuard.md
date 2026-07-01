# MediaGuard & ChatGuard: Consolidated Security & Settings System

`MediaGuard` is an architectural extension for the **GoBale** framework. It unifies individual user restrictions, group-wide dynamic lockdowns, administrative mutes, and interactive group-specific configuration keyboards under a single-pass, database-backed processing system.

By consolidating media filtering, captcha verification, and administrative locking under the high-performance **`ChatGuard`** middleware, this module optimizes server resources while remaining compatible across standard groups, supergroups, and channels.

---

## Technical Architecture & Design

### 1. High-Performance Middleware (`ChatGuard`)
Replaces split verification guards with a single-pass protection pipeline. It evaluates group locks, captcha verification, administrative mutes, and both group/user media restrictions in one synchronous pass. This minimizes database queries and eliminates resource overhead.

### 2. Consolidated GOB Storage
Unifies both global setting records and group-specific lock states under the primary GOB database instance (`dbInstance`), completely eliminating the overhead of multiple parallel GOB files.

### 3. Local Scoped Group Settings (`RegisterLocal`)
Extends the core `.Settings()` module to support local chat-specific toggles (e.g., closing voice notes for Group A while leaving them open in Group B) without global pointer leaks.

### 4. Remote Settings Management (`Settings(chatID)`)
Enables administrators to manage group settings dynamically from the bot's private chat. The generated inline keyboards securely embed the target group's ID into the callback payload (`_sys_cfg:[key]:[groupID]`). The bot automatically parses the parameters and performs remote modifications without any custom callback code needed in `main.go`.

### 5. Multi-Layer Security & Owner Bypass
* **Bot Owner (`c.IsOwner()`):** The global owner is bypassed immediately for all safety checks. They are authorized to change any setting (global or local, remote or in-group).
* **Group Administrators:** Evaluates whether the user who clicked the button has admin privileges specifically in the target chat being managed.
* **Global Settings:** Sensitive options (such as maintenance mode) are strictly reserved for the owner and cannot be accessed by group admins.
* **Private DM Checks:** Bypasses `getChatMember` checks on private chat IDs to prevent Bale API errors (`400 Bad Request: unsupported peer type`).

---

## Supported Media Categories

* `photo` — Standard images.
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

	// 3. Global Maintenance Middleware (protects all routes)
	bot.On().Use(func(c *gobale.Ctx) {
		// Bypass the owner so they can still manage the bot or toggle settings
		if c.IsOwner() {
			c.Next()
			return
		}

		// Block regular users if maintenance mode is active
		if maintenanceMode {
			_, _ = c.Send().
				Text("⚠️ ربات در حال حاضر در دست تعمیر و به‌روزرسانی است. لطفاً بعداً تلاش کنید.").
				Temp(10 * time.Second).
				Go()
			c.Abort()
			return
		}

		c.Next()
	})

	// 4. Start MediaGuard with standard commands, command auto-clean, and 5s warnings
	bot.On().MediaGuard().
		Commands().
		DelCmds().
		Warn(5 * time.Second).
		Go()

	// 5. Command for Admin inside a group to display the local settings panel
	bot.On().Cmd("settings").Do(gobale.AdminsOnly(), func(c *gobale.Ctx) {
		// If used in Private DM, parse target group ID to manage remotely
		if c.IsPrivate() {
			var groupChat string
			_ = c.ScanArgs(&groupChat)
			if groupChat == "" {
				// Display global settings in Private DM if no group ID is supplied
				_, _ = c.Send().Text("⚙️ پنل تنظیمات سراسری ربات:").Settings().Go()
				return
			}
			// Display remote group settings in Private DM if target group ID is supplied
			_, _ = c.Send().Text("⚙️ پنل تنظیمات اختصاصی گروه هدف:").Settings(groupChat).Go()
			return
		}

		// Display group-isolated settings inside the group chat
		_ = c.Del().Go()
		_, _ = c.Send().Text("⚙️ پنل تنظیمات پیشرفته گروه:").Settings().Go()
	})

	log.Println("Bot is running...")
	bot.Run().Polling().Go()
}
```

---

## API Chain Reference

### 1. Registration Chain (`MediaGuardChain`)

| Method | Return Type | Description |
| :--- | :--- | :--- |
| `Warn(d time.Duration)` | `*MediaGuardChain` | Sets the temporary warning alerts visibility TTL. |
| `Msg(text string)` | `*MediaGuardChain` | Overrides the default violation message template. Supports `{name}` and `{type}` placeholders. |
| `Silent()` | `*MediaGuardChain` | Mutes all warning messages. Prohibited media gets deleted silently. |
| `Commands()` | `*MediaGuardChain` | Registers default administrative commands. |
| `DelCmds()` | `*MediaGuardChain` | Deletes incoming admin command messages after execution. |
| `Go()` | `*OnChain` | Finalizes configurations and appends the unified `ChatGuard` middleware. |

---

### 2. Transaction Chain (`MediaRestrictChain`)

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

* **In-Group Command:** `/settings`
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

### Scenario 4: Individual Restriction via User ID
Administer restrictions directly inside the group using a specific user's numeric ID.

* **Format:** `/restrict [userID] [types...]`

* **Example - Block Files (Documents) for User `776655`:**
  ```text
  /restrict 776655 document
  ```
  *Response:* `🚫 دسترسی کاربر 776655 به موارد [document] مسدود شد.`

* **Example - Allow Files (Documents) again for User `776655`:**
  ```text
  /unrestrict 776655 document
  ```
  *Response:* `✅ محدودیت کاربر 776655 برای ارسال موارد [document] لغو شد.`

---

### Scenario 5: Remote Moderation via Private DM (Bot's PV)
Administrators can moderate remote group chats from the bot's private chat. This requires providing the target **Chat ID** (usually negative) as the first argument, followed by the **User ID** (for individual restrictions) or simply the media keywords (for group-wide restrictions).

* **Example - Block Videos remotely in Group `100998877` for User `554433`:**
  ```text
  /restrict 100998877 554433 video
  ```
  *Response:* `🚫 دسترسی کاربر 554433 در چت 100998877 به موارد [video] مسدود شد.`

* **Example - Lock Stickers remotely for the entire Group `100998877`:**
  ```text
  /restrict 100998877 sticker
  ```
  *Response:* `🚫 ارسال موارد [sticker] برای کل اعضای گروه مسدود شد.`

---

### Scenario 6: Programmatic Developer Implementations

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

## Custom Warning Messages

By default, the `MediaGuard` middleware alerts the group with a standard Persian message when a restriction is violated. You can easily override this warning template using the `.Msg()` method during initialization.

The custom message template supports two dynamic placeholders:
* `{name}` — Automatically replaced with the violator's mention (prefixed with `@` if they have a username).
* `{type}` — Automatically replaced with the translated Persian name of the violated media type (e.g., "تصویر", "ویدیو", "پیام صوتی (وویس)").

### Example: Implementing a Custom Self-Destroying Warning Alert

```go
bot.On().MediaGuard().
	Commands().
	DelCmds().
	Warn(5 * time.Second). // Warnings auto-delete after 5 seconds
	Msg("🚨 Dear {name}, posting [{type}] is temporarily locked in this group!"). // Custom template
	Go()
```

---

## Disabling Warnings (Silent Deletion Mode)

If you prefer to maintain absolute silence in the chat and want the bot to delete violating media without sending any alert message at all, chain the `.Silent()` method:

```go
bot.On().MediaGuard().
	Commands().
	DelCmds().
	Silent(). // Prohibited media will be deleted silently with no warning sent to the group
	Go()
```

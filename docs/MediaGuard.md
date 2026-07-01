# MediaGuard: Dynamic Media & Group Lockdown Extension for GoBale

`MediaGuard` is an extension module for the **GoBale** framework. It provides user-specific, chat-isolated media restriction capabilities alongside dynamic group-wide media locking (chat lockdowns) without relying on native messenger restricted member APIs (which typically require Supergroups). 

Working locally via the bot's direct GOB database, `MediaGuard` intercepts, identifies, and deletes prohibited media types instantly on standard groups, supergroups, and channels alike.

---

## Technical Overview & Key Features

* **Granular Individual Restrictions:** Block or allow specific media types (or all of them) for an isolated user in a specific chat.
* **Dynamic Group-Wide Locking:** Toggle group-wide media lockdowns (e.g., closing image, video, or voice note submissions for all non-admin members) on-the-fly via quick commands or code.
* **Auto-Adaptive Execution:** Standard commands natively detect if they are running within a group or remotely in the bot's private messages (DMs).
* **Silent & Verbose Warnings:** Toggle custom violating warning alerts featuring customizable self-destroying Time-to-Live (TTL).
* **Command Auto-Clean:** Deletes incoming admin commands (`/restrict` or `/unrestrict`) automatically to preserve chat cleanliness.
* **Local State Persistence:** Uses GOB-encoded map storage under `blocked_media_group_%v` (group key) and `blocked_media_%v_%d` (user key) for lightweight, thread-safe transactional performance.

---

## Supported Media Categories

The system supports granular restrictions via the following mapped keywords:

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

To implement `MediaGuard` inside your GoBale bot, save the `media_guard.go` file inside your `gobale` package namespace. Then, initialize the fluent chain in your main entry file:

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

	// Register MediaGuard dynamically
	bot.On().MediaGuard().
		Commands().               // Registers /restrict and /unrestrict commands
		DelCmds().                // Automatically deletes incoming admin commands
		Warn(5 * time.Second).    // Self-destroys warnings after 5 seconds
		Go()

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
| `Silent()` | `*MediaGuardChain` | Mutes all warning messages. Restricted media gets deleted silently. |
| `Commands()` | `*MediaGuardChain` | Registers default administrative commands. |
| `DelCmds()` | `*MediaGuardChain` | Deletes incoming admin command messages after execution. |
| `Go()` | `*OnChain` | Finalizes configurations and appends the middleware globally. |

---

### 2. Transaction Chain (`MediaRestrictChain`)

Initiated via `bot.RestrictMedia(userID)` or `c.RestrictMedia(userID)` inside handlers.

| Method | Return Type | Description |
| :--- | :--- | :--- |
| `Chat(chatID any)` | `*MediaRestrictChain` | Targets a specific group/channel. Auto-resolves username and numeric IDs. |
| `Group()` | `*MediaRestrictChain` | Targets the dynamic restriction to the **entire chat group** instead of an individual. |
| `Block(types ...MediaType)` | `*MediaRestrictChain` | Registers selected media types into the database blacklist. |
| `Allow(types ...MediaType)` | `*MediaRestrictChain` | Removes selected media types from the database blacklist. |
| `Clear()` | `*MediaRestrictChain` | Removes all existing restrictions for the specified scope. |
| `Go()` | `error` | Executes the direct database transaction. |

---

## Usage Scenarios & Commands Reference

Administrators can moderate chats natively in-group, via replies, or remotely via private messages with the bot.

### Scenario 1: Dynamic Group Lockdown (Entire Chat Restriction)
This scenario is useful when the entire group needs to be restricted from posting certain media, or when closing the group media access completely.

* **Case A: Complete Media Lockdown**
  Locks down all media formats for the entire group. Users are restricted to standard text messages only.
  ```text
  /restrict
  ```
  *Response:* `🚫 گروه با موفقیت قفل شد. ارسال هرگونه رسانه برای کل اعضا مسدود گردید.`

* **Case B: Restricting Specific Media (e.g., Photos & Videos)**
  Prohibits all non-admin members from posting images and video files.
  ```text
  /restrict photo video
  ```
  *Response:* `🚫 ارسال موارد [photo, video] برای کل اعضای گروه مسدود شد.`

* **Case C: Unlocking Group Completely**
  Clears the entire group-wide media restriction list.
  ```text
  /unrestrict
  ```
  *Response:* `✅ قفل گروه کاملاً باز شد. ارسال تمامی رسانه‌ها مجدداً برای همه آزاد گردید.`

* **Case D: Partial Group Unlock (e.g., Allowing Voice Notes again)**
  ```text
  /unrestrict voice
  ```
  *Response:* `✅ محدودیت کل اعضای گروه برای ارسال موارد [voice] لغو شد.`

---

### Scenario 2: Individual Restriction via Reply
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

### Scenario 3: Individual Restriction via User ID
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
For custom automation scripts, bots can execute restrictions programmatically inside route handlers or external cron tasks.

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

	_, _ = c.Send().Text("🚨 ارسال پیام صوتی (وویس) برای کل اعضا قفل شد.").Temp(5 * time.Second).Go()
})
```

#### Programmatic Restriction from Bot/System Context:
```go
// Restrict a user across a specific group remotely
err := bot.RestrictMedia(userID).Chat(groupID).Block(gobale.MediaPhoto, gobale.MediaDocument).Go()
if err != nil {
	log.Printf("Transactional failure: %v", err)
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

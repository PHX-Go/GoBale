# `GroupGuard` & Dynamic Settings Ecosystem

The `GroupGuard` ecosystem in the GoBale framework is a high-performance, developer-centric group moderation shield and settings UI generator. It moves 90% of the security and filtering logic behind the scenes, allowing developers to write minimal, readable, and highly fluent code while maintaining absolute control over custom settings keys and layout grids.

---

## 1. `GroupGuard` vs. `ChatGuard`

While both middlewares protect group chats, they serve distinct architectural purposes:

| Feature | `ChatGuard` (Legacy/Default) | `GroupGuard` (Dynamic/Modern) |
| :--- | :--- | :--- |
| **Enforcement Style** | Instant block & delete with warning alerts. | Silent instant block OR dynamic **Self-Destruction** with custom duration. |
| **Key Mapping** | Hardcoded switch statements (e.g. only supports standard keys). | **Fully Dynamic Suffix Mapping**. Auto-detects and binds custom keys natively. |
| **Scope** | General group locks (`group_lock`) and join-captcha mutes. | Highly optimized for dynamic 10-type media & analytic shields. |

---

## 2. Dynamic Setting Suffixes (Automated Media Locks)

`GroupGuard` automatically scans all registered settings on every incoming message. If an active GOB database setting matches the message type based on the key's suffix, it is intercepted and deleted silently.

The supported suffixes are:
* Suffix `_sticker` $\rightarrow$ Blocks Sticker.
* Suffix `_gif` $\rightarrow$ Blocks Animation/GIF.
* Suffix `_photo` $\rightarrow$ Blocks Photo.
* Suffix `_video` $\rightarrow$ Blocks Video.
* Suffix `_doc` / `_document` $\rightarrow$ Blocks Document/File.
* Suffix `_voice` $\rightarrow$ Blocks Voice.
* Suffix `_audio` $\rightarrow$ Blocks Audio/Music.
* Suffix `_location` $\rightarrow$ Blocks Location.
* Suffix `_contact` $\rightarrow$ Blocks Contact.
* Suffix `_destroy` $\rightarrow$ Triggers **Self-Destroying Media** (Allows standard media temporarily, then silenty deletes them after a specified duration).

---

## 3. Registering & Using Custom Keys

You can register any custom key (like `"auto_welcome"` or `"anti_spam"`) natively. 

### Step 1: Register in `main()`
```go
bot.Settings().RegisterLocal("auto_welcome", "خوش‌آمدگویی خودکار", true)
```

### Step 2: Read Status in your Handler
Using the ultra-short `.GetBool("key")` method, you can retrieve the live GOB Database status (falling back to registered defaults) in a single line:
```go
if c.GetBool("auto_welcome") {
    _, _ = c.ReplyText("👋 Welcome to our group!")
}
```

---

## 4. Hand-Crafted Matrix Settings UI

Instead of being forced into one button per row, you can hand-craft any grid, matrix, or custom column layout natively using the polymorphic `.Buttons()` row-appending method alongside `c.SettingBtn(key)`.

```go
bot.On().Cmd("settings").Roles(gobale.RoleOwner).Do(func(c *gobale.Ctx) {
    // Lock settings panel interaction securely to the specific group creator [1]
    c.SetData("active_settings_admin_id", c.SenderID())

    _, _ = c.SendMarkdown(fmt.Sprintf("⚙️ *Settings Panel for %s:*", c.ChatTitle())).
        Buttons(c.SettingBtn("lock_sticker"), c.SettingBtn("lock_gif")). // Row 1 (2 columns)
        Buttons(c.SettingBtn("auto_welcome")).                          // Row 2 (1 column)
        Buttons("❌ Close Panel", "close_settings").                    // Row 3 (Close button)
        Go()
	
    _ = c.Delete()
})
```

---

## 5. Security & Auto-Restoration Flow

Every time a user clicks an inline settings button:
1. **Dynamic Confirmation Dialog**: The settings panel is replaced with an elegant confirmation menu ("Are you sure you want to turn ON Sticker Lock?") containing **Yes**, **No**, and **Back** buttons.
2. **Owner-Only Interaction Lock**: Only the specific admin who originally triggered `/settings` can click the buttons. Other administrators are blocked with an screen popup (`❌ این پنل توسط شما باز نشده است!`).
3. **Zero-Loss Layout Restoration**: When they click "Back", "No", or confirm "Yes", the framework dynamically captures the original layout from the live update markup (`ReplyMarkup`), updates only the toggled button's status emoji (`🟢/🔴`), and restores your hand-crafted matrix UI cleanly in-place.

---

## 6. Universal Remote Toggling

Using the universal `c.ToggleSetting()` helper, you can toggle any registered setting natively, supporting optional target chats for **Remote Management** directly from the bot's Private Chat (PV):

* Toggle locally in group: `/toggle lock_sticker on`
* Toggle remotely from bot PV: `/toggle lock_gif off 4542691229`

---

## 7. Self-Destroying Media (AWD & Dynamic Timing)

The `media_destroy` setting supports polymorphic values natively inside `GroupGuard` [1]:
* **Boolean `true`**: Defaults to silent self-destruction in **5 minutes**.
* **Integer (Seconds)**: Sets custom self-destruction seconds (e.g., `30` for 30 seconds).
* **String**: Dynamically parses duration strings (e.g. `"10m"`, `"30s"`, `"1h"`).

Administrators can set custom durations from the line of command:
`/toggle media_destroy 30s`

---

## 8. Complete Modern Fluent Example (`main.go`)

Here is the complete, extremely clean, and compile-safe `main.go` demonstrating the entire ecosystem integrated natively:

```go
package main

import (
	"fmt"
	"log"

	gobale "github.com/PHX-Go/GoBale"
)

func main() {
	_ = gobale.Env().Go()

	token := gobale.GetEnv[string]("BALE-TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")

	bot, err := gobale.New(token).Admin(adminID).
		LogLadder("").SuppressEmptyUpdates(true).Go()
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	// Register setting keys, labels, and defaults natively
	bot.Settings().
		RegisterLocal("lock_sticker", "قفل استیکر", false).
		RegisterLocal("lock_gif", "قفل گیف (انیمیشن)", false).
		RegisterLocal("media_destroy", "خودتخریبی ۵ دقیقه‌ای رسانه‌ها", false).
		RegisterLocal("auto_welcome", "خوش‌آمدگویی خودکار به اعضا", true)

	// Register our smart, dynamic GroupGuard middleware globally
	bot.On().Use(gobale.GroupGuard())

	// Welcome Flow: Custom setting "auto_welcome" processed natively with 1-line GetBool helper
	bot.On().Join().Do(func(c *gobale.Ctx) {
		// Verify custom switch status natively in 1 line
		if c.GetBool("auto_welcome") {
			for _, user := range c.Message.NewChatMembers {
				if user.IsBot {
					continue
				}
				_, _ = c.ReplyText(fmt.Sprintf("👋 کاربر %s به گروه خوش آمدید!", user.Mention()))
			}
		}
	})

	// Settings Panel: Hand-crafted matrix layout using c.SettingBtn with zero-config auto resolution
	bot.On().Cmd("settings").Roles(gobale.RoleOwner).Do(func(c *gobale.Ctx) {
		// Save the executor's ID into the session to lock panel interaction to this specific admin
		c.SetData("active_settings_admin_id", c.SenderID())

		// Send Settings natively with Markdown formatting and hand-crafted matrix buttons [1]
		_, _ = c.SendMarkdown(fmt.Sprintf("⚙️ *پنل تنظیمات گروه %s:*\n\nبرای تغییر وضعیت دکمه‌ها کلیک کنید:", c.ChatTitle())).
			Buttons(c.SettingBtn("lock_sticker"), c.SettingBtn("lock_gif")).
			Buttons(c.SettingBtn("media_destroy"), c.SettingBtn("auto_welcome")).
			Buttons("❌ بستن پنل", "close_settings").
			Go()

		_ = c.Delete() // Delete the trigger command natively
	})

	// Close Settings Callback: Deletes the settings menu panel silently
	bot.On().Callback("close_settings").Do(func(c *gobale.Ctx) {
		_ = c.Answer().Go()
		_ = c.Delete()
	})

	// Toggle Command: Unified local & remote setting switcher (works in group and privately via bot PV!)
	bot.On().Cmd("toggle").Roles(gobale.RoleOwner).Do(func(c *gobale.Ctx) {
		key := c.ArgString(0)   // e.g. "lock_sticker"
		state := c.ArgString(1) // e.g. "on", "10m" (optional)
		chat := c.ArgString(2)  // e.g. "4542691229" (optional for remote PV management)

		if key == "" {
			_, _ = c.ReplyText("⚠️ دستور نامعتبر! مثال:\n`/toggle lock_sticker on`\n`/toggle media_destroy 30s` (تایمر دلخواه)")
			return
		}

		active, errToggle := c.ToggleSetting(key, state, chat)
		if errToggle != nil {
			_, _ = c.ReplyText(fmt.Sprintf("❌ خطایی رخ داد: %v", errToggle.Error()))
			return
		}

		status := "🔴 خاموش"
		if active {
			status = "🟢 روشن"
		}
		_, _ = c.ReplyText(fmt.Sprintf("✅ تنظیم `%s` با موفقیت به حالت [%s] تغییر یافت.", key, status))
	})

	log.Println("Bot is running...")
	bot.Run().Polling().Go()
}
```

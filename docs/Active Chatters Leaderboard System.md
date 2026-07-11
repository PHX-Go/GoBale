# Active Chatters Leaderboard System

The Active Chatters Leaderboard in the GoBale framework is a high-performance, dynamic, and bidirectional-stable active users tracking system. It continuously records group message metrics, caches user display names, and generates clean, beautifully formatted charts of the top 10 most active members by any of the 15 supported metrics, either locally or remotely.

---

## 1. Key Capabilities

* **15 Granular Metrics**: Fetch top chatters by total messages, plain texts, specific media (photos, videos, GIFs, stickers, files, locations, contacts, audios, voice notes), or interaction types (replies, forwards, edits, command counts).
* **Automatic Bidirectional Isolation (W3C Standard)**: Solves the common MTProto RTL line-flipping bug. It wraps lines in `LRI/PDI` and Persian user names in `RLI/PDI` to guarantee that emojis, names, and code-blocked counts are rendered strictly and beautifully Left-to-Right regardless of Persian/Latin scripts.
* **Dynamic PV & Remote Resolving**: Safely run `/top gif 10` inside a group chat, or `/top photo 5 4542691229` remotely from the bot's private chat (PV).
* **Argument Safeguards**: Built-in defensive parameter verification ensures that commands like `/top gif 10` do not clash with remote chat ID resolvers.
* **Automatic Midnight Reset**: Clean WAL GOB database integration automatically purges daily user message stats at midnight to start fresh the next day.

---

## 2. Supported Metrics & Aliases

When calling `.Leaderboard()`, you can pass any of the following metric strings:

| Query Metric | Resolved Suffix | Persian Display Title |
| :--- | :--- | :--- |
| `msgs` / `all` / `messages` | `msgs` | کل پیام‌های ارسالی |
| `text` | `text` | پیام‌های متنی |
| `photo` / `pic` / `pics` | `photo` | تصاویر ارسالی (Photo) |
| `video` | `video` | ویدیوهای ارسالی (Video) |
| `voice` | `voice` | پیام‌های صوتی (Voice) |
| `audio` / `music` | `audio` | فایل‌های موسیقی (Audio) |
| `document` / `doc` / `file` | `document` | اسناد و فایل‌ها (Document) |
| `sticker` | `sticker` | استیکرهای ارسالی (Sticker) |
| `animation` / `gif` / `gifs` | `animation` | گیف‌های ارسالی (GIF) |
| `location` | `location` | موقعیت‌های مکانی (Location) |
| `contact` | `contact` | مخاطبان به اشتراک گذاشته شده |
| `replies` | `replies` | ریپلای‌های ارسالی |
| `forwards` | `forwards` | پیام‌های فوروارد شده |
| `edits` | `edits` | پیام‌های ویرایش شده |
| `command` | `command` | دستورات صادر شده |

---

## 3. How it Works (Behind the Scenes)

Every time a user sends a message, the `AnalyticsLogger` middleware:
1. Extracts the user's actual name (`FirstName` + optional `LastName`, omitting `@username`) and caches it securely under `user_name:<userID>`.
2. Appends the user's ID to `active_users:<chatID>` to maintain a dynamic roster of active members.
3. Increments both daily and lifetime message counts for that user's specific media/text type.

When `.Leaderboard()` is called:
1. It retrieves the chat's roster and queries their respective counts.
2. It sorts the chatters descendingly and slices the top $N$.
3. It maps ranks 1-3 to medals (`🥇`, `🥈`, `🥉`) and ranks 4-10 to numeric emojis (`4️⃣` through `🔟`).
4. It isolates the line directionality to prevent bidirectional layout flips.

---

## 4. Fluent API Integration

Using the framework's native `ArgString` and `ArgInt` shortcuts, the entire dynamic, remote-capable, and multi-metric leaderboard is written in **just 6 lines of code** inside `main.go`:

```go
bot.On().Cmd("top").Do(func(c *gobale.Ctx) {
	// Parse metric (default: msgs), limit (default: 10), and optional remote chat ID natively
	metric := c.ArgString(0, "msgs")
	limit := c.ArgInt(1, 10)
	chat := c.ArgString(2)

	// Fetch dynamic leaderboard with dynamic title and Unicode Bidi Isolation
	report, err := c.Leaderboard(metric, limit, chat, gobale.PeriodDaily)
	if err != nil {
		_, _ = c.ReplyMarkdown("❌ Failed to query leaderboard!").Go()
		return
	}

	_, _ = c.ReplyMarkdown(report).Go() // Chained with Go to execute natively
        _ = c.Delete() // Delete trigger command natively
})
```

---

## 5. Complete Modern Example (`main.go`)

Here is the complete, extremely clean, and compile-safe `main.go` demonstrating both daily and lifetime leaderboards in action:

```go
package main

import (
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

	// Register our smart, dynamic AnalyticsLogger middleware globally
	bot.On().Use(gobale.AnalyticsLogger())

	// Daily Leaderboard: Compile and send top N active chatters natively by custom metric
	// (supporting remote PV)
	bot.On().Cmd("top").Do(func(c *gobale.Ctx) {

		// Parse metric, limit, and optional target chat ID for remote PV usage fluidly
		metric := c.ArgString(0, "msgs")
		limit := c.ArgInt(1, 10)
		chat := c.ArgString(2) // Remote target chat ID (optional)

		report, err := c.Leaderboard(metric, limit, chat, gobale.PeriodDaily)
		if err != nil {
			_, _ = c.ReplyMarkdown("❌ Failed to query daily leaderboard!").Go()
			return
		}
		_, _ = c.ReplyMarkdown(report).Go()
		_ = c.Delete() // Delete trigger command natively
	})

	// Lifetime Leaderboard: Compile and send top N lifetime active chatters natively by custom metric
	// (supporting remote PV)
	bot.On().Cmd("top_all").Do(func(c *gobale.Ctx) {

		// Parse metric, limit, and optional target chat ID for remote PV usage fluidly
		metric := c.ArgString(0, "msgs")
		limit := c.ArgInt(1, 10)
		chat := c.ArgString(2) // Remote target chat ID (optional)

		report, err := c.Leaderboard(metric, limit, chat, gobale.PeriodLifetime)
		if err != nil {
			_, _ = c.ReplyMarkdown("❌ Failed to query lifetime leaderboard!").Go()
			return
		}

		_, _ = c.ReplyMarkdown(report).Go()
		_ = c.Delete() // Delete trigger command natively
	})

	log.Println("Bot is running...")
	bot.Run().Polling().Go()
}
```

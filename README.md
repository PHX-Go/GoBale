# GoBale

GoBale is a concurrent, thread-safe, and modular bot framework for the Bale Messenger Bot API written in Go.

## Installation

```bash
go get github.com/PHX-Go/GoBale
OR
go get github.com/PHX-Go/GoBale@main
```

## Quick Start

Import the package in your project:

```go
import "github.com/PHX-Go/GoBale"
OR with your desired name
import gobale "github.com/PHX-Go/GoBale"
```

### Example Usage

Create a `.env` file in your project root:
```env
BALE_TOKEN="YOUR_BALE_BOT_TOKEN"
ADMIN_ID=99999999
```

Create a `main.go` file:

```go
package main

import (
	"log"

	gobale "github.com/PHX-Go/GoBale"
)

func main() {
	gobale.Env().Go()

	token := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")

	bot, err := gobale.New(token).Admin(adminID).Gzip().Go()

	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	bot.On().Cmd("start").Do(func(c *gobale.Ctx) {
		c.Send().Text("Welcome to the GoBale Bot! Send me any text and I will echo it back.").Go()
	})

	bot.On().Msg().Do(func(c *gobale.Ctx) {
		c.Send().Text(c.Text()).Go()
	})

	log.Println("Bot is running...")
	bot.Run().Polling().Go()
}
```
---
### Update

The `Update` struct represents the root incoming envelope received from the Bale API servers. Every event (e.g., text messages, edits, callback queries, payment validations) is wrapped inside this structure.

```go
type Update struct {
	UpdateID         int               `json:"update_id"`
	Message          *Message          `json:"message,omitempty"`
	EditedMessage    *Message          `json:"edited_message,omitempty"`
	CallbackQuery    *CallbackQuery    `json:"callback_query,omitempty"`
	PreCheckoutQuery *PreCheckoutQuery `json:"pre_checkout_query,omitempty"`
}
```

#### Usage

Within any route handler or middleware, you can access the raw `Update` structure directly from the handler context (`Ctx`):

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	// Log the incoming update ID and message details safely
	log.Printf("Processing Update ID: %d", c.Update.UpdateID)

	if c.Update.Message != nil {
		log.Printf("Message text received: %s", c.Update.Message.Text)
	}
})
```
---

### getUpdates

The `getUpdates` API method allows you to manually retrieve outstanding updates from the Bale servers using a fluent builder chain (`UpdatesChain`). This is useful for manual polling implementations or custom background tasks.

#### Usage

Call `.Updates()` from the `Bot` context, configure your `Offset` and `Limit` dynamically, and run the transaction using `.Go()`:

```go
// Fetch up to 50 updates starting from offset 100 manually
updates, err := bot.Updates().
	Offset(100).
	Limit(50).
	Go()

if err != nil {
	log.Printf("Failed to fetch manual updates: %v", err)
	return
}

for _, u := range updates {
	log.Printf("Manually retrieved update ID: %d", u.UpdateID)
}
```
---
### setWebhook

The `setWebhook` API method can be used in **two distinct ways** depending on whether you want the framework to manage the HTTP server automatically, or if you prefer to manage the webhook registration manually.

#### Method 1: Automatic Setup via the Webhook Runner (Recommended)

This is the standard approach for production hosting. The framework automatically spins up a secure HTTPS server (or plain HTTP behind reverse proxies like ngrok/Nginx), handles incoming update routing, and registers the webhook URL on the Bale servers in a single builder chain.

```go
// Start internal webhook server and automatically call setWebhook on Bale
err := bot.Run().
	Webhook().
	Addr(":443").
	Path("/bale-updates").
	URL("https://yourdomain.com"). // Automatically registers https://yourdomain.com/bale-updates on Bale
	Cert("cert.pem").             // Optional: SSL Certificate path
	Key("key.pem").               // Optional: SSL Private Key path
	Go()

if err != nil {
	log.Fatalf("Failed to run webhook runner: %v", err)
}
```

*Note: For local tunnel testing (e.g., using ngrok), you can use the `.Insecure()` and `.Ngrok()` configurations to bypass SSL requirement checks and automatically resolve the forwarding URL.*

#### Method 2: Manual Setup via the Webhook Chain

Use this approach if you are managing your own external HTTP server or reverse proxy, and only need to notify the Bale servers where to send the updates without starting GoBale's internal HTTP server.

```go
// Manually register a webhook URL on Bale servers from the Bot context
ok, err := bot.Webhook().
	Set("https://yourdomain.com/custom-endpoint").
	Go()

if err != nil {
	log.Printf("Failed to manually register webhook: %v", err)
	return
}

if ok {
	log.Println("Webhook successfully registered on Bale servers")
}
```
---
### deleteWebhook

The `deleteWebhook` API method can be utilized in **two ways** within GoBale: automatically by the framework when switching to polling mode, or manually through the fluent webhook builder chain.

#### Method 1: Automatic Webhook Deletion (Polling Mode)

When you start the bot in long polling mode (`bot.Run().Polling().Go()`), GoBale automatically executes a `deleteWebhook` API call first. This ensures that any stale or active webhook registrations are cleared so they do not block or interfere with the polling update stream.

```go
// Starting polling mode automatically deletes any active webhook on startup
bot.Run().Polling().Go()
```

#### Method 2: Manual Webhook Deletion via the Webhook Chain

Use this approach if you need to manually deregister the active webhook on the Bale servers from the bot context without starting a long polling loop (e.g., during administrative scripts or deployment transitions).

```go
// Manually delete the active webhook registration on Bale servers
ok, err := bot.Webhook().
	Del().
	Go()

if err != nil {
	log.Printf("Failed to manually delete webhook: %v", err)
	return
}

if ok {
	log.Println("Webhook successfully removed from Bale servers")
}
```

---
### WebhookInfo & getWebhookInfo

The `WebhookInfo` struct represents the current configuration and diagnostic status of your active webhook registration on the Bale servers. You can retrieve this metadata using the fluent `getWebhookInfo` API chain (`bot.Webhook().Info().Go()`).

#### WebhookInfo Struct

```go
type WebhookInfo struct {
	URL                  string   `json:"url"`
	HasCustomCertificate bool     `json:"has_custom_certificate"`
	PendingUpdateCount   int      `json:"pending_update_count"`
	IPAddress            string   `json:"ip_address,omitempty"`
	LastErrorDate        int64    `json:"last_error_date,omitempty"`
	LastErrorMessage     string   `json:"last_error_message,omitempty"`
	MaxConnections       int      `json:"max_connections,omitempty"`
	AllowedUpdates       []string `json:"allowed_updates,omitempty"`
}
```

#### Usage

To query the active webhook details and check for errors or pending updates, invoke the `.Info()` chain from the `Webhook` context:

```go
// Fetch active webhook configuration metadata from Bale servers
info, err := bot.Webhook().
	Info().
	Go()

if err != nil {
	log.Printf("Failed to retrieve webhook status: %v", err)
	return
}

// Log retrieved diagnostic metrics safely
log.Printf("Registered Webhook URL: %s", info.URL)
log.Printf("Pending Updates in Queue: %d", info.PendingUpdateCount)

if info.LastErrorMessage != "" {
	log.Printf("Recent Webhook Error: %s (occurred at Unix time: %d)", info.LastErrorMessage, info.LastErrorDate)
}
```
---
### User

The `User` struct represents a Bale user account or bot account identity. It contains basic profile fields and includes a built-in helper method `Mention()` to dynamically format user mentions.

```go
type User struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}
```

#### Helper Methods

* **`Mention() string`**: Returns a string formatted with the `@` prefix if the user has a registered username; otherwise, it returns the user's first name wrapped in Markdown bold tags (`*FirstName*`).

#### Usage

Typically accessed through incoming message updates via `c.Message.From`:

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	if c.Message != nil && c.Message.From != nil {
		user := c.Message.From

		// Log basic user metrics safely
		log.Printf("Received message from user ID: %d (Username: %s)", user.ID, user.Username)

		// Utilize built-in Mention helper to greet the user
		userMention := user.Mention()
		
		_, _ = c.Send().
			Text(fmt.Sprintf("Hello %s, how can I help you today?", userMention)).
			Markdown().
			Go()
	}
})
```
---
### Chat

The `Chat` struct represents a private direct message conversation, a group, a supergroup, or a channel window.

```go
type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title,omitempty"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}
```

#### Field Details

* **`ID`**: Unique 64-bit integer identifier for this chat.
* **`Type`**: Type of chat, which can be `"private"`, `"group"`, `"supergroup"`, or `"channel"`.
* **`Title`**: Display title (for channels and group chats).
* **`Username`**: Optional username (prefixed with `@` for public groups and channels).

#### Usage

Typically accessed through incoming message updates via `c.Message.Chat`:

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	if c.Message != nil {
		chat := c.Message.Chat

		// Log basic incoming chat metadata safely
		log.Printf("Processing message in Chat ID: %d (Type: %s)", chat.ID, chat.Type)

		// Check if the chat is a public channel or group with a username
		if chat.Type == "channel" || chat.Type == "supergroup" {
			log.Printf("Active public channel/group username: %s", chat.Username)
		}
	}
})
```
---
### ChatFullInfo

The `ChatFullInfo` struct represents detailed metadata returned for specific chats when querying the `getChat` API. It contains additional fields that are not returned in the standard `Chat` struct, such as bio, description, invite links, and photo metadata.

```go
type ChatFullInfo struct {
	ID           int64      `json:"id"`
	Type         string     `json:"type"`
	Title        string     `json:"title,omitempty"`
	Username     string     `json:"username,omitempty"`
	FirstName    string     `json:"first_name,omitempty"`
	LastName     string     `json:"last_name,omitempty"`
	Photo        *ChatPhoto `json:"photo,omitempty"`
	Bio          string     `json:"bio,omitempty"`
	Description  string     `json:"description,omitempty"`
	InviteLink   string     `json:"invite_link,omitempty"`
	LinkedChatID int64      `json:"linked_chat_id,omitempty"`
}

type ChatPhoto struct {
	SmallFileID       string `json:"small_file_id"`
	SmallFileUniqueID string `json:"small_file_unique_id"`
	BigFileID         string `json:"big_file_id"`
	BigFileUniqueID   string `json:"big_file_unique_id"`
}
```

#### Usage

Typically queried using the `.Info()` chain from either the `Bot` or `Ctx` context:

```go
bot.On().Cmd("chatinfo").Do(func(c *gobale.Ctx) {
	// Query detailed metadata of the current chat
	info, err := c.Chat().Info().Go()
	if err != nil {
		log.Printf("Failed to retrieve chat full info: %v", err)
		return
	}

	// Safely extract and check description metadata
	chatDesc := info.Description
	if chatDesc == "" {
		chatDesc = "No description provided."
	}

	response := fmt.Sprintf("Chat Details:\nTitle: %s\nInvite Link: %s\nDescription: %s", 
		info.Title, info.InviteLink, chatDesc)

	_, _ = c.Send().
		Text(response).
		Go()
})
```
---
### Message

The `Message` struct represents a single chat message. It is the central container for all text-based messages, media attachments (photos, videos, voice notes, stickers, etc.), replies, forwarded messages, service messages, and payment/invoice updates.

```go
type Message struct {
	MessageID            int64                 `json:"message_id"`
	Date                 int64                 `json:"date"`
	Chat                 Chat                  `json:"chat"`
	From                 *User                 `json:"from,omitempty"`
	SenderChat           *Chat                 `json:"sender_chat,omitempty"`
	ForwardFrom          *User                 `json:"forward_from,omitempty"`
	ForwardFromChat      *Chat                 `json:"forward_from_chat,omitempty"`
	ForwardFromMessageID int64                 `json:"forward_from_message_id,omitempty"`
	ForwardDate          int64                 `json:"forward_date,omitempty"`
	ReplyToMessage       *Message              `json:"reply_to_message,omitempty"`
	EditDate             int64                 `json:"edit_date,omitempty"`
	MediaGroupID         string                `json:"media_group_id,omitempty"`
	Text                 string                `json:"text,omitempty"`
	Entities             []MessageEntity       `json:"entities,omitempty"`
	Animation            *Animation            `json:"animation,omitempty"`
	Audio                *Audio                `json:"audio,omitempty"`
	Document             *Document             `json:"document,omitempty"`
	Photo                []PhotoSize           `json:"photo,omitempty"`
	Sticker              *Sticker              `json:"sticker,omitempty"`
	Video                *Video                `json:"video,omitempty"`
	Voice                *Voice                `json:"voice,omitempty"`
	Caption              string                `json:"caption,omitempty"`
	CaptionEntities      []MessageEntity       `json:"caption_entities,omitempty"`
	Contact              *Contact              `json:"contact,omitempty"`
	Location             *Location             `json:"location,omitempty"`
	NewChatMembers       []User                `json:"new_chat_members,omitempty"`
	LeftChatMember       *User                 `json:"left_chat_member,omitempty"`
	Invoice              *Invoice              `json:"invoice,omitempty"`
	SuccessfulPayment    *SuccessfulPayment    `json:"successful_payment,omitempty"`
	WebAppData           *WebAppData           `json:"web_app_data,omitempty"`
	ReplyMarkup          *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}
```

#### Usage

Typically accessed directly through the `Ctx` context using `c.Message`:

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	msg := c.Message
	if msg == nil {
		return
	}

	// Verify if the incoming message is a reply to another message
	if msg.ReplyToMessage != nil {
		repliedMsg := msg.ReplyToMessage
		
		// Log replied message details safely
		log.Printf("User replied to message ID %d containing text: %q", repliedMsg.MessageID, repliedMsg.Text)
		
		_, _ = c.Send().
			Text(fmt.Sprintf("Replying to your previous message: %q", repliedMsg.Text)).
			Reply(repliedMsg.MessageID).
			Go()
		return
	}

	// Log standard incoming message metrics
	log.Printf("Received Message ID: %d, Text: %q", msg.MessageID, msg.Text)
})
```

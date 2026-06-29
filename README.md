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
BALE_TOKEN=YOUR_BALE_BOT_TOKEN
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
---
### MessageId & MessageEntity

The `MessageId` and `MessageEntity` structs represent specialized message identification wrappers and text formatting entities, respectively.

```go
type MessageId struct {
	MessageID int64 `json:"message_id"`
}

type MessageEntity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
}
```

#### Struct Details

* **`MessageId`**: A simple structure representing the raw ID envelope returned by the Bale API (such as the payload response of the `copyMessage` method).
* **`MessageEntity`**: Represents a formatting entity in the message text. This includes text styling entities (like `"bold"`, `"italic"`, `"code"`) as well as interactive elements (like `"mention"`, `"url"`, `"bot_command"`).

#### Usage

The following example shows how to iterate over message text entities and perform a copy action (which internally utilizes `MessageId` to populate the returned message ID):

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	msg := c.Message
	if msg == nil {
		return
	}

	// 1. Iterate through message entities to inspect formatting styles
	for _, entity := range msg.Entities {
		log.Printf("Text Entity Type: %s (Offset: %d, Length: %d)", 
			entity.Type, entity.Offset, entity.Length)
	}

	// 2. Copy the incoming message using the fluent copying pipeline
	copiedMsg, err := c.Send().
		Copy(msg.Chat.ID, msg.MessageID).
		Go()

	if err != nil {
		log.Printf("Failed to copy message: %v", err)
		return
	}

	// Log the newly generated copied message ID
	log.Printf("Message copied successfully under new ID: %d", copiedMsg.MessageID)
})
```
---
### PhotoSize

The `PhotoSize` struct represents one size or resolution of a photo or file thumbnail. When a photo is sent, the Bale API returns an array of `PhotoSize` objects representing the same image at different resolutions.

```go
type PhotoSize struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int    `json:"file_size,omitempty"`
}
```

#### Struct Details

* **`FileID`**: Unique file identifier on the Bale servers, which can be reused to send or download the file.
* **`FileUniqueID`**: Unique identifier for the same file across different bots.
* **`Width` & `Height`**: Photo dimensions in pixels.
* **`FileSize`**: File size in bytes.

#### Usage

Bale/Telegram returns photo sizes sorted from **smallest to largest**. Select the last element of the slice to obtain the highest resolution image for processing or downloading:

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	msg := c.Message
	if msg == nil || len(msg.Photo) == 0 {
		return
	}

	// The last element represents the highest resolution available
	largestPhoto := msg.Photo[len(msg.Photo)-1]

	log.Printf("Received Photo. File ID: %s, Dimensions: %dx%d, Size: %d bytes", 
		largestPhoto.FileID, largestPhoto.Width, largestPhoto.Height, largestPhoto.FileSize)

	// Download the photo using the built-in File download API
	filePath, err := c.File(largestPhoto.FileID).
		Download().
		Path("./downloads").
		Go()

	if err != nil {
		log.Printf("Failed to download photo: %v", err)
		return
	}

	log.Printf("Photo saved to disk: %s", filePath)
})
```
---
### Animation

Represents a silent loop video animation file (such as a GIF sticker).

```go
type Animation struct {
	FileID       string     `json:"file_id"`
	FileUniqueID string     `json:"file_unique_id"`
	Width        int        `json:"width"`
	Height       int        `json:"height"`
	Duration     int        `json:"duration"`
	Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
	FileName     string     `json:"file_name,omitempty"`
	MimeType     string     `json:"mime_type,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
}
```

#### Usage

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	if c.Message != nil && c.Message.Animation != nil {
		anim := c.Message.Animation
		log.Printf("Received Animation: %s (Size: %d bytes, Duration: %ds)", anim.FileName, anim.FileSize, anim.Duration)
	}
})
```

---

### Audio

Represents a music or audio file playable inside standard media players.

```go
type Audio struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	Title        string `json:"title,omitempty"`
	FileName     string `json:"file_name,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}
```

#### Usage

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	if c.Message != nil && c.Message.Audio != nil {
		aud := c.Message.Audio
		log.Printf("Received Audio: %s by %s (File: %s)", aud.Title, aud.FileName, aud.MimeType)
	}
})
```

---

### Document

Represents any generic file attachment (such as PDFs, ZIP files, etc.).

```go
type Document struct {
	FileID       string     `json:"file_id"`
	FileUniqueID string     `json:"file_unique_id"`
	Thumbnail    *PhotoSize `json:"thumbnail,omitempty"`
	FileName     string     `json:"file_name,omitempty"`
	MimeType     string     `json:"mime_type,omitempty"`
	FileSize     int64      `json:"file_size,omitempty"`
}
```

#### Usage

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	if c.Message != nil && c.Message.Document != nil {
		doc := c.Message.Document
		log.Printf("Received Document: %s (Type: %s, Size: %d bytes)", doc.FileName, doc.MimeType, doc.FileSize)
	}
})
```

---

### Video

Represents a video file containing both moving picture and sound.

```go
type Video struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Duration     int    `json:"duration"`
	FileName     string `json:"file_name,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}
```

#### Usage

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	if c.Message != nil && c.Message.Video != nil {
		vid := c.Message.Video
		log.Printf("Received Video: %s (Resolution: %dx%d, Duration: %ds)", vid.FileName, vid.Width, vid.Height, vid.Duration)
	}
})
```

---

### Voice

Represents a voice note (audio message recorded on the fly).

```go
type Voice struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}
```

#### Usage

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	if c.Message != nil && c.Message.Voice != nil {
		voice := c.Message.Voice
		log.Printf("Received Voice Note: %s (Duration: %ds, Size: %d bytes)", voice.FileID, voice.Duration, voice.FileSize)
	}
})
```

---

### Contact

Represents a shared contact card from a user's address book.

```go
type Contact struct {
	PhoneNumber string `json:"phone_number"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name,omitempty"`
	UserID      int64  `json:"user_id,omitempty"`
}
```

#### Usage

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	if c.Message != nil && c.Message.Contact != nil {
		contact := c.Message.Contact
		log.Printf("Received Contact Card: Name: %s %s, Phone: %s (UserID: %d)", 
			contact.FirstName, contact.LastName, contact.PhoneNumber, contact.UserID)
	}
})
```

---

### Location

Represents a shared geographic coordinate point on the map.

```go
type Location struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
}
```

#### Usage

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	if c.Message != nil && c.Message.Location != nil {
		loc := c.Message.Location
		log.Printf("Received Location Pin: Latitude: %f, Longitude: %f", loc.Latitude, loc.Longitude)
	}
})
```

---

### Invoice

Represents basic parameters of a sent payment invoice.

```go
type Invoice struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	TotalAmount int64  `json:"total_amount"`
}
```

#### Usage

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	if c.Message != nil && c.Message.Invoice != nil {
		inv := c.Message.Invoice
		log.Printf("Received Invoice Details: Title: %s, Amount: %d IRR", inv.Title, inv.TotalAmount)
	}
})
```

---

### File

Represents raw physical file metadata (such as server-relative path and dimensions) returned by the `getFile` API.

```go
type File struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileSize     int64  `json:"file_size,omitempty"`
	FilePath     string `json:"file_path,omitempty"`
}
```

#### Usage

Used to fetch paths of pre-uploaded media objects on Bale servers before initiating a physical download:

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	if c.Message != nil && len(c.Message.Photo) > 0 {
		photoFileID := c.Message.Photo[0].FileID

		// Retrieve the download path metadata using the File API
		fileInfo, err := c.File(photoFileID).Info().Go()
		if err != nil {
			log.Printf("Failed to retrieve file metadata: %v", err)
			return
		}

		log.Printf("File metadata resolved. Server Path: %s, File Size: %d bytes", 
			fileInfo.FilePath, fileInfo.FileSize)
	}
})
```
---
### ReplyKeyboardMarkup & KeyboardButton

Instead of manually constructing verbose, nested structs, GoBale provides a clean, fluent builder chain (`ReplyMarkup` and `ReplyBtn`) to generate customized reply keyboards with ease.

```go
type ReplyKeyboardMarkup struct {
	Keyboard        [][]KeyboardButton `json:"keyboard"`
	ResizeKeyboard  bool               `json:"resize_keyboard,omitempty"`
	OneTimeKeyboard bool               `json:"one_time_keyboard,omitempty"`
}

type KeyboardButton struct {
	Text            string      `json:"text"`
	RequestContact  bool        `json:"request_contact,omitempty"`
	RequestLocation bool        `json:"request_location,omitempty"`
	WebApp          *WebAppInfo `json:"web_app,omitempty"`
}
```

#### Usage

To generate a reply keyboard, call `ReplyMarkup()` and chain buttons using `ReplyBtn()` helper methods:

```go
bot.On().Cmd("menu").Do(func(c *gobale.Ctx) {
	// Build a structured reply keyboard fluently using built-in builders
	keyboard := gobale.ReplyMarkup().
		Row(
			gobale.ReplyBtn("Share Contact").Contact(),
			gobale.ReplyBtn("Share Location").Location(),
		).
		Row(
			gobale.ReplyBtn("Open Mini App").WebApp("https://example.com"),
		).
		OneTime(true).
		Build()

	_, _ = c.Send().
		Text("Please share your contact or location details:").
		Markup(keyboard).
		Go()
})
```

---

### InlineKeyboardMarkup & InlineKeyboardButton

Inline keyboards are generated fluently using the `InlineMarkup()` builder. Buttons are constructed utilizing `Btn()` which provides fluent setters for URLs, callbacks, text copying, and WebApps.

```go
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
	Text         string          `json:"text"`
	URL          string          `json:"url,omitempty"`
	CallbackData string          `json:"callback_data,omitempty"`
	WebApp       *WebAppInfo     `json:"web_app,omitempty"`
	CopyText     *CopyTextButton `json:"copy_text,omitempty"`
}
```

#### Usage

Call `InlineMarkup()` and chain rows of buttons constructed fluently via `Btn()`:

```go
bot.On().Cmd("options").Do(func(c *gobale.Ctx) {
	// Build an interactive inline keyboard fluently using built-in builders
	inlineKeyboard := gobale.InlineMarkup().
		Row(
			gobale.Btn("🔗 External Link").URL("https://google.com"),
			gobale.Btn("⚙️ Callback Action").Callback("action_clicked"),
		).
		Row(
			gobale.Btn("📋 Copy Key").Copy("BaleSecretToken123"),
			gobale.Btn("🌐 Open Mini App").WebApp("https://example.com"),
		).
		Build()

	_, _ = c.Send().
		Text("Choose an interactive action:").
		Markup(inlineKeyboard).
		Go()
})
```
---
### ChatMember

The `ChatMember` struct represents a user's membership status and detailed permissions within a group, supergroup, or channel. GoBale simplifies the complex Telegram/Bale union types (such as `ChatMemberOwner`, `ChatMemberAdministrator`, `ChatMemberMember`, and `ChatMemberRestricted`) by unifying them into this single, easy-to-use structure.

```go
type ChatMember struct {
	Status              string `json:"status"`
	User                User   `json:"user"`
	CanDeleteMessages   bool   `json:"can_delete_messages,omitempty"`
	CanManageVideoChats bool   `json:"can_manage_video_chats,omitempty"`
	CanRestrictMembers  bool   `json:"can_restrict_members,omitempty"`
	CanPromoteMembers   bool   `json:"can_promote_members,omitempty"`
	CanChangeInfo       bool   `json:"can_change_info,omitempty"`
	CanInviteUsers      bool   `json:"can_invite_users,omitempty"`
	CanPostStories      bool   `json:"can_post_stories,omitempty"`
	CanPostMessages     bool   `json:"can_post_messages,omitempty"`
	CanEditMessages     bool   `json:"can_edit_messages,omitempty"`
	CanPinMessages      bool   `json:"can_pin_messages,omitempty"`
	IsMember            bool   `json:"is_member,omitempty"`
	CanSendMessages     bool   `json:"can_send_messages,omitempty"`
	CanSendAudios       bool   `json:"can_send_audios,omitempty"`
	CanSendDocuments    bool   `json:"can_send_documents,omitempty"`
	CanSendPhotos       bool   `json:"can_send_photos,omitempty"`
	CanSendVideos       bool   `json:"can_send_videos,omitempty"`
}
```

#### Helper Methods

* **`IsCreator() bool`**: Returns `true` if the member's status is `"creator"` (the chat owner).
* **`IsAdmin() bool`**: Returns `true` if the member is either an `"administrator"` or `"creator"` (has admin privileges).
* **`IsRegularMember() bool`**: Returns `true` if the member's status is `"member"` (a standard group member).

#### Usage

Query a specific user's status using the `.Member()` chain from the `Chat` context:

```go
bot.On().Cmd("checkmember").Do(func(c *gobale.Ctx) {
	// Query current sender's membership status in the group
	member, err := c.Chat().Member(c.SenderID()).Go()
	if err != nil {
		log.Printf("Failed to retrieve chat member status: %v", err)
		return
	}

	// Inspect roles easily using built-in helpers
	if member.IsCreator() {
		_, _ = c.Send().Text("You are the owner/creator of this group!").Go()
	} else if member.IsAdmin() {
		_, _ = c.Send().Text("You are an administrator of this group.").Go()
	} else if member.IsRegularMember() {
		_, _ = c.Send().Text("You are a standard member of this group.").Go()
	}

	// Check specific permission flags directly
	log.Printf("Can this member pin messages? %t", member.CanPinMessages)
})
```

---

### ChatPhoto

The `ChatPhoto` struct represents the file identifiers of a chat's avatar (profile photo), returned inside `ChatFullInfo`.

```go
type ChatPhoto struct {
	SmallFileID       string `json:"small_file_id"`
	SmallFileUniqueID string `json:"small_file_unique_id"`
	BigFileID         string `json:"big_file_id"`
	BigFileUniqueID   string `json:"big_file_unique_id"`
}
```

#### Usage

Nested within `ChatFullInfo.Photo`, which is queried via `.Info()`:

```go
bot.On().Cmd("getavatar").Do(func(c *gobale.Ctx) {
    // Query detailed chat info containing the avatar metadata
    info, err := c.Chat().Info().Go()
    if err != nil {
        log.Printf("Failed to query chat details: %v", err)
        return
    }

    if info.Photo != nil {
        avatar := info.Photo
        
        // Log the small and big avatar file IDs safely
        log.Printf("Chat Small Avatar File ID: %s", avatar.SmallFileID)
        log.Printf("Chat Large Avatar File ID: %s", avatar.BigFileID)
    }
})
```
---
### ResponseParameters

The `ResponseParameters` struct is returned by the Bale API when a request fails due to rate-limiting (HTTP 429). It contains a directive on how long the bot must wait before retrying the action.

```go
type ResponseParameters struct {
	RetryAfter int `json:"retry_after,omitempty"`
}
```

*Note: GoBale's internal API client (`client.go`) handles `ResponseParameters` automatically. When an HTTP 429 error occurs, it parses `RetryAfter`, sleeps for the directed duration, and replays the request without throwing errors to the parent handler.*

---

### InputMedia Interface & Types

The `InputMedia` interface defines the standard contract for elements inside media group albums. GoBale provides concrete struct implementations for photos, videos, animations, audios, and documents.

```go
type InputMedia interface {
	MediaType() string
}
```

#### Struct Definitions

```go
type InputMediaPhoto struct {
	Type    string `json:"type"`
	Media   string `json:"media"`
	Caption string `json:"caption,omitempty"`
}

type InputMediaVideo struct {
	Type      string `json:"type"`
	Media     string `json:"media"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Caption   string `json:"caption,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Duration  int    `json:"duration,omitempty"`
}

type InputMediaAnimation struct {
	Type      string `json:"type"`
	Media     string `json:"media"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Caption   string `json:"caption,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Duration  int    `json:"duration,omitempty"`
}

type InputMediaAudio struct {
	Type      string `json:"type"`
	Media     string `json:"media"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Caption   string `json:"caption,omitempty"`
	Duration  int    `json:"duration,omitempty"`
	Title     string `json:"title,omitempty"`
}

type InputMediaDocument struct {
	Type      string `json:"type"`
	Media     string `json:"media"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Caption   string `json:"caption,omitempty"`
}
```

#### Usage

To send a group of media objects as an album, utilize the `.Album()` chain from the `Send` context:

```go
bot.On().Cmd("album").Do(func(c *gobale.Ctx) {
	// Send an album group containing multiple photo and video files
	messages, err := c.Send().
		Album().
		Photo("./images/banner.jpg", "Check out the first banner").
		Video("./videos/promo.mp4", "Watch the promo video clip").
		Go()

	if err != nil {
		log.Printf("Failed to send album group: %v", err)
		return
	}

	log.Printf("Successfully sent album containing %d messages", len(messages))
})
```

---

### InputFile

The `InputFile` struct represents a file stream ready to be uploaded to Bale servers via a multipart/form-data request. It is used when transmitting local disk files, in-memory buffers, or remote byte streams.

```go
type InputFile struct {
	Field    string
	FileName string
	Reader   io.Reader
}
```

#### Usage

To upload an arbitrary file from a local disk stream or any custom reader, construct an `InputFile` and pass it directly to the media sending chain:

```go
bot.On().Cmd("sendpdf").Do(func(c *gobale.Ctx) {
	file, err := os.Open("./reports/annual_report.pdf")
	if err != nil {
		log.Printf("Failed to open report file: %v", err)
		return
	}
	defer file.Close()

	// Construct an InputFile wrapper around the active file reader stream
	inputFile := gobale.InputFile{
		FileName: "annual_report.pdf",
		Reader:   file,
	}

	// Stream and upload the document directly
	_, err = c.Send().
		Doc(inputFile).
		Caption("Here is the requested annual report document:").
		Go()

	if err != nil {
		log.Printf("Failed to upload document file: %v", err)
		return
	}
})
```
---
## Sending Files

GoBale provides a unified, polymorphic media sending pipeline. When you call media methods (such as `.Photo()`, `.Doc()`, `.Audio()`, `.Video()`, `.Voice()`, or `.Sticker()`), the framework automatically detects the transmission method based on the input type. 

There are **three distinct methods** supported by the Bale platform to transmit files:

---

### 1. Sending by `file_id` (Server Resending)

If a file has already been uploaded and stored on the Bale servers, it is highly recommended to reuse its unique `file_id`. 
* **Pros:** Blazing fast, consumes zero bandwidth, and has **no size limits**.
* **How to use:** Pass the raw `file_id` string directly to any media sender method.

```go
bot.On().Cmd("resend").Do(func(c *gobale.Ctx) {
	// Resend an existing photo already stored on Bale servers using its File ID
	_, err := c.Send().
		Photo("AgACAgIAAxkBAAEY_mock_file_id_string"). // Raw file_id string
		Caption("Here is the resent image using server cache").
		Go()

	if err != nil {
		log.Printf("Failed to resend photo: %v", err)
	}
})
```

#### Technical Constraints for `file_id`:
* **Type Conservation:** You cannot change the file type during resending. For example, a video cannot be resent as a photo, and a photo cannot be sent as a document.
* **No Thumbnail Resending:** You cannot resend or reuse photo thumbnails using a file ID.
* **Uniqueness:** A `file_id` is unique per bot. You cannot copy or transfer a `file_id` directly from one bot account to another.
* **Single File, Multiple IDs:** A single physical file can have multiple valid file IDs even within the same bot.

---

### 2. Sending by HTTP URL (Remote Download)

You can pass a public HTTP/HTTPS URL of the target file. The Bale servers will automatically download and transmit the file to the chat.
* **Size Limits:** Maximum **5 MB** for images, and **20 MB** for other media types.
* **How to use:** Pass the remote URL string directly to any media sender method.

```go
bot.On().Cmd("sendbyurl").Do(func(c *gobale.Ctx) {
	// Send an audio file via a remote HTTP URL
	_, err := c.Send().
		Audio("https://example.com/music/track1.mp3"). // Remote file URL
		Caption("Music track downloaded and sent by Bale").
		Go()

	if err != nil {
		log.Printf("Failed to send audio via URL: %v", err)
	}
})
```

#### Technical Constraints for URLs:
* **MIME Types:** The remote file must serve the correct MIME type (e.g., `audio/mpeg` for `sendAudio`).
* **Documents restriction:** In `sendDocument`, sending via URL is restricted to **GIF**, **PDF**, and **ZIP** files only.
* **Voice messages:** In `sendVoice`, the URL file must be of type `audio/ogg` and must not exceed **1 MB**. Voice files between 1 MB and 20 MB will be delivered as generic audio document files instead of playable voice bubbles.

---

### 3. Sending by Multipart Upload (Local File)

You can upload a local file from your disk or an in-memory buffer using standard `multipart/form-data` POST requests.
* **Size Limits:** Maximum **10 MB** for images, and **50 MB** for other file types.
* **How to use:** Pass a local file path string (e.g., `"./images/poster.png"`) or an `InputFile` struct. GoBale automatically detects local files and handles the streaming headers.

```go
bot.On().Cmd("upload").Do(func(c *gobale.Ctx) {
	// Send a local photo. GoBale automatically detects the local path and uploads it.
	_, err := c.Send().
		Photo("./images/local_poster.jpg"). // Local file path on disk
		Caption("Uploaded directly from local disk").
		Go()

	if err != nil {
		log.Printf("Failed to upload local file: %v", err)
	}
})
```
---
## Text Formatting Helpers

GoBale provides native helper functions in `utils.go` to safely format text using Bale's Markdown rules without manually managing asterisks, underscores, or spaces.

---

### Built-in Formatting Helpers

* **`gobale.Bold(text string) string`**: Safely wraps text in bold formatting with appropriate spaces (` *text* `).
* **`gobale.Italic(text string) string`**: Safely wraps text in italic formatting with appropriate spaces (` _text_ `).
* **`gobale.Link(text, url string) string`**: Safely compiles bracketed hyperlink Markdown (`[text](url)`).
* **`gobale.Tooltip(text, desc string) string`**: Compiles an instant view hover tooltip utilizing monospaced backticks.

```go
bot.On().Cmd("hello").Do(func(c *gobale.Ctx) {
	// Format text dynamically using native GoBale formatting helpers
	welcomeMessage := fmt.Sprintf("Hello %s, welcome to %s! Please check the %s.",
		gobale.Bold("Tester"),
		gobale.Italic("GoBale Framework"),
		gobale.Link("Online Manual", "https://bale.ai"),
	)

	_, _ = c.Send().
		Text(welcomeMessage).
		Markdown(). // Enables Markdown parsing for the formatted helpers
		Go()
})
```

---

### Advanced Multi-line Text Builder (`TextChain`)

GoBale includes a fluent, multi-line string builder (`TextChain`) that allows compiling clean messages and dynamically binding variables with zero string concatenation overhead.

```go
bot.On().Cmd("status").Do(func(c *gobale.Ctx) {
	// Compile multi-line text dynamically and bind variables safely
	report := gobale.Text().
		Line("📊 ", gobale.Bold("System Report")).
		Line().
		Line("👤 Target User: {mention}").
		Line("🚀 Version: ", gobale.Italic("v1.0.0-stable")).
		Line("🔗 Resource: ", gobale.Link("GoBale Repo", "https://github.com/PHX-Go/GoBale")).
		Bind("mention", c.Message.From.Mention()).
		Go()

	_, _ = c.Send().
		Text(report).
		Markdown().
		Go()
})
```

---

## API Methods Reference Mapping

GoBale maps every official Bale API endpoint to intuitive, fluent chaining methods. Below is the comprehensive API reference mapping:

### 1. Identity & Diagnostics

| Bale API Method | GoBale Fluent Chain | Description |
| :--- | :--- | :--- |
| `getMe` | `bot.Me().Go()` | Retrieves the bot's own profile and identity. |
| `getFile` | `c.File(id).Info().Go()` | Retrieves metadata of a stored file (size, path). |
| `askReview` | `c.Review().Delay(s).Go()` | Prompts the user with Bale's native rating/review form. |

#### Notes on `askReview`:
* **Timing:** Best triggered immediately after a successful transaction or when a useful service is delivered.
* **Server Constraints:** Dispatching the review form does not guarantee its display. Bale applies internal checks to prevent spamming users. Developers do not need to manage these limits manually.
* **Visibility:** The review form is only visible to the user while they are actively inside your bot's chat window or Mini App interface.
* **Audience:** Highly recommended for engaged, returning users; avoid triggering it for newly joined users.

```go
bot.On().Cmd("done").Do(func(c *gobale.Ctx) {
	// Query bot identity and prompt for a review after 5 seconds
	me, _ := c.Me().Go()
	log.Printf("Bot username: %s", me.Username)

	_, _ = c.Review().Delay(5).Go()
})
```

---

### 2. Messaging & Media Dispatching

| Bale API Method | GoBale Fluent Chain | Description |
| :--- | :--- | :--- |
| `sendMessage` | `c.Send().Text(str).Go()` | Sends a text message to the chat. |
| `forwardMessage` | `c.Send().Forward(from, msgID).Go()` | Forwards an existing message to another chat. |
| `copyMessage` | `c.Send().Copy(from, msgID).Go()` | Copies a message without the original author link. |
| `sendPhoto` | `c.Send().Photo(any).Go()` | Sends a photo (File ID, URL, or local path). |
| `sendAudio` | `c.Send().Audio(any).Go()` | Sends an audio file. |
| `sendDocument` | `c.Send().Doc(any).Go()` | Sends a generic document/file. |
| `sendVideo` | `c.Send().Video(any).Go()` | Sends a video. |
| `sendAnimation` | `c.Send().Anim(any).Go()` | Sends a silent loop animation (GIF). |
| `sendVoice` | `c.Send().Voice(any).Go()` | Sends a voice note (OGG format). |
| `sendMediaGroup` | `c.Send().Album().Photo().Go()` | Sends a grouped album of photos or videos. |
| `sendLocation` | `c.Send().Location(lat, lon).Go()` | Sends geographic map coordinates. |
| `sendContact` | `c.Send().Contact(phone, first, last).Go()` | Sends a directory contact card. |
| `sendChatAction` | `c.Action().Typing().Go()` | Sends a chat state indicator (typing, uploading). |

```go
bot.On().Cmd("share").Do(func(c *gobale.Ctx) {
	// Trigger typing state and send location map
	_, _ = c.Action().Typing().Go()
	_, _ = c.Send().Location(35.6892, 51.3890).Go()
})
```

---

### 3. Callbacks

| Bale API Method | GoBale Fluent Chain | Description |
| :--- | :--- | :--- |
| `answerCallbackQuery` | `c.Answer().Text(str).Go()` | Clears the loading spinner on inline button clicks. |

```go
bot.On().Callback("btn_click").Do(func(c *gobale.Ctx) {
	_ = c.Answer().Text("Action processed!").Alert().Go()
})
```

---

### 4. Chat Administration & Management

| Bale API Method | GoBale Fluent Chain | Description |
| :--- | :--- | :--- |
| `banChatMember` | `c.Chat().Ban(userID).Go()` | Bans a member from the group. |
| `unbanChatMember` | `c.Chat().Unban(userID).Go()` | Unbans a restricted or kicked group member. |
| `promoteChatMember` | `c.Chat().Promote(userID).Go()` | Promotes a user and configures administrator privileges. |
| `setChatPhoto` | `c.Chat().SetPhoto(any).Go()` | Changes the chat's avatar photo. |
| `deleteChatPhoto` | `c.Chat().DelPhoto().Go()` | Deletes the active chat avatar photo. |
| `setChatTitle` | `c.Chat().Title(str).Go()` | Edits the title of the chat. |
| `setChatDescription` | `c.Chat().Desc(str).Go()` | Edits the description text of the chat. |
| `pinChatMessage` | `c.Chat().Pin(msgID).Go()` | Pins a message at the top of the chat window. |
| `unPinChatMessage` | `c.Chat().Unpin(msgID).Go()` | Unpins a specific pinned message. |
| `unpinAllChatMessages`| `c.Chat().UnpinAll().Go()` | Unpins all pinned messages in the chat. |
| `leaveChat` | `c.Chat().Leave().Go()` | Instructs the bot to leave the group or channel. |
| `getChat` | `c.Chat().Info().Go()` | Retrieves full chat metadata (`ChatFullInfo`). |
| `getChatMember` | `c.Chat().Member(userID).Go()` | Retrieves membership status of a specific user. |
| `getChatAdministrators`| `c.Chat().Admins().Go()` | Retrieves a list of administrators in the group. |
| `getChatMembersCount` | `c.Chat().MembersCount().Go()`| Retrieves total count of members in the group/channel. |
| `createChatInviteLink`| `c.Chat().InviteLink().Go()` | Generates a new invitation link for the chat. |
| `revokeChatInviteLink`| `c.Chat().RevokeLink(link).Go()`| Revokes an active invitation link. |
| `exportChatInviteLink`| `c.Chat().ExportLink().Go()` | Exports the primary invitation link of the chat. |

```go
bot.On().Cmd("cleanup").Do(func(c *gobale.Ctx) {
	// Restrict to admins, unpin all messages, and change description
	isAdmin, _ := c.Chat().IsAdmin().Go()
	if isAdmin {
		_ = c.Chat().UnpinAll().Go()
		_ = c.Chat().Desc("Cleaned up group chat").Go()
	}
})
```
---
## Modifying & Deleting Messages

GoBale provides a unified fluent API (`EditChain` and `DelChain`) to dynamically update text, media captions, or keyboard markups, as well as schedule message deletions.

---

### Editing Messages (`editMessageText` / `editMessageCaption` / `editMessageReplyMarkup`)

Instead of calling separate endpoints, GoBale unifies all edit actions under the `.Edit()` builder. The framework dynamically detects which fields are updated and dispatches the corresponding API call.

#### Usage

Invoke `.Edit()` from the handler context to modify the message that triggered the event (essential for inline keyboard callback updates):

```go
bot.On().Callback("update_panel").Do(func(c *gobale.Ctx) {
	// Dynamically edit the text and inline keyboard markup of the active message
	_, err := c.Edit().
		Text("This is the updated text panel.").
		Markup(gobale.InlineMarkup().
			Row(gobale.Btn("Option A").Callback("opt_a")).
			Build(),
		).
		Go()

	if err != nil {
		log.Printf("Failed to edit active message: %v", err)
	}
})
```

---

### Deleting Messages (`deleteMessage`)

GoBale offers three ways to delete messages:
1. **Immediate deletion:** Delete the incoming message using `.Del().Go()`.
2. **Delayed deletion:** Schedule a delayed deletion in the background using `.Del().Delay(duration).Go()`.
3. **By Message ID:** Delete a specific message by its ID using `c.Chat().DelMsg(messageID).Go()`.

#### Usage

The following example demonstrates sending a self-destructing message and deleting the user's triggering command after a delay:

```go
bot.On().Cmd("temp").Do(func(c *gobale.Ctx) {
	// 1. Send a temporary message that automatically deletes itself after 10 seconds
	_, err := c.Send().
		Text("This message will automatically delete in 10 seconds.").
		Temp(10 * time.Second). // Utilizing SendChain's built-in Temp helper
		Go()

	if err != nil {
		log.Printf("Failed to send temporary message: %v", err)
	}

	// 2. Schedule the deletion of the user's incoming command after a 5-second delay
	err = c.Del().
		Delay(5 * time.Second). // Utilizing Ctx's built-in Del delayed helper
		Go()

	if err != nil {
		log.Printf("Failed to schedule command deletion: %v", err)
	}
})
```
---
## Stickers & Custom Sets

GoBale provides a comprehensive fluent API (`StickerChain`) to upload, create, retrieve, and manage sticker packs. It includes a polymorphic `StickerInput` interface to dynamically accept local files, URLs, or file IDs.

---

### Sticker & StickerSet Structs

```go
type Sticker struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int    `json:"file_size,omitempty"`
}

type StickerSet struct {
	Name      string     `json:"name"`
	Title     string     `json:"title"`
	Stickers  []Sticker  `json:"stickers"`
	Thumbnail *PhotoSize `json:"thumbnail,omitempty"`
}
```

#### Input Polymorphism (`StickerInput`)
Any method requiring sticker files accepts the `StickerInput` interface. You can pass:
* **`StickerFilePath("path")`**: For local image files on disk.
* **`StickerFileID("file_id")`**: For pre-uploaded file IDs on Bale servers.
* **`StickerURL("url")`**: For remote HTTP URLs.

---

### 1. Uploading a Sticker File (`uploadStickerFile`)

Upload a raw static file (PNG/WEBP format) to the Bale servers to generate an active `File` reference.

```go
bot.On().Cmd("uploadstk").Do(func(c *gobale.Ctx) {
	// Upload a local PNG file as a sticker file
	file, err := c.Sticker().
		Upload(c.SenderID(), gobale.StickerFilePath("./assets/smile.png")).
		Go()

	if err != nil {
		log.Printf("Failed to upload sticker file: %v", err)
		return
	}

	log.Printf("Sticker successfully uploaded. File ID: %s", file.FileID)
})
```

---

### 2. Creating a Sticker Set (`createNewStickerSet`)

Build a brand-new, customized sticker package containing one or multiple stickers.

```go
bot.On().Cmd("createset").Do(func(c *gobale.Ctx) {
	// Create a new sticker set with a single initial sticker
	ok, err := c.Sticker().
		Create(c.SenderID(), "my_pack_by_phx", "My Custom Pack Title").
		Add(gobale.StickerFileID("uploaded_file_id_123"), []string{"😊", "👍"}).
		Go()

	if err != nil {
		log.Printf("Failed to create sticker set: %v", err)
		return
	}

	if ok {
		log.Println("Sticker set successfully created!")
	}
})
```

---

### 3. Adding a Sticker to a Set (`addStickerToSet`)

Add a new sticker item directly to your previously created sticker set.

```go
bot.On().Cmd("addsticker").Do(func(c *gobale.Ctx) {
	// Prepare the input sticker item with emoji configurations
	inputSticker := gobale.InputSticker{
		Sticker:   gobale.StickerFileID("another_uploaded_file_id"),
		EmojiList: []string{"🔥"},
	}

	// Add the sticker item to the existing package
	ok, err := c.Sticker().
		Add(c.SenderID(), "my_pack_by_phx", inputSticker).
		Go()

	if err != nil {
		log.Printf("Failed to add sticker to set: %v", err)
		return
	}

	if ok {
		log.Println("Sticker successfully added to the pack!")
	}
})
```
---
## Bale Wallet & E-Commerce Payments

The Bale Messenger platform provides a secure, fast, and integrated **Electronic Wallet (E-Wallet)** service for developers. This allows bots to send "Request Money" messages that users can instantly pay using their Bale E-Wallet, eliminating the need to type card numbers, expiration dates, or CVV2/OTPs.

> ⚠️ **Important Architecture Notice:** The legacy Card-to-Card (کارت به کارت) payment method has been **deprecated and removed** by Bale. The official, stable, and secure standard for e-commerce transactions on Bale is strictly the **Electronic Wallet (E-Wallet)**.

---

### Wallet Provider Tokens & Testing
* **Production Tokens:** Generated through `@botfather` by registering your business wallet.
* **Sandbox / Testing Token:** Utilize the official mock token `WALLET-TEST-1111111111111111`. It functions identically to production tokens but does not perform actual monetary transfers.

---

### E-Commerce Struct Definitions

```go
type LabeledPrice struct {
	Label  string `json:"label"`
	Amount int64  `json:"amount"` // Amount in IRR (Rials)
}

type PreCheckoutQuery struct {
	ID             string `json:"id"`
	From           User   `json:"from"`
	Currency       string `json:"currency"`
	TotalAmount    int64  `json:"total_amount"`
	InvoicePayload string `json:"invoice_payload"`
}

type SuccessfulPayment struct {
	Currency                string `json:"currency"`
	TotalAmount             int64  `json:"total_amount"`
	InvoicePayload          string `json:"invoice_payload"`
	ShippingOptionID        string `json:"shipping_option_id,omitempty"`
	BalePaymentChargeID     string `json:"bale_payment_charge_id,omitempty"`
	ProviderPaymentChargeID string `json:"provider_payment_charge_id,omitempty"`
}

type Transaction struct {
	TransactionID string `json:"transaction_id"`
	Status        string `json:"status"`
	Amount        int64  `json:"amount"`
	UserID        int64  `json:"userID"`
	CreatedAt     int64  `json:"createdAt"`
}
```

---

### 1. Sending an Invoice (`sendInvoice`)

Generate and send an interactive invoice bubble. Users can click the button to pay directly from their Bale E-Wallet balance.

```go
bot.On().Cmd("checkout").Do(func(c *gobale.Ctx) {
	// Build and dispatch an invoice utilizing the secure mock e-wallet provider token
	_, err := c.Pay().
		Invoice("Gold Premium Account", "Access all premium features for 1 year", "pay_payload_id_101", "WALLET-TEST-1111111111111111").
		Price("Annual Membership", 850000). // 850,000 Rials (IRR)
		Price("Value Added Tax (VAT)", 85000).
		NeedName(true).
		NeedPhone(true).
		Go()

	if err != nil {
		log.Printf("Failed to dispatch invoice: %v", err)
	}
})
```

---

### 2. Generating a Payment Link (`createInvoiceLink`)

Compile a digital payment gateway URL that can be embedded inside inline keyboard buttons or web mini-apps.

```go
bot.On().Cmd("paylink").Do(func(c *gobale.Ctx) {
	// Compile a secure payments URL link
	link, err := c.Pay().
		Link("Custom Merchandise", "Payment for custom designed mug", "pay_payload_id_102", "WALLET-TEST-1111111111111111").
		Price("Mug & Print Work", 350000).
		Go()

	if err != nil {
		log.Printf("Failed to generate payment gateway link: %v", err)
		return
	}

	// Send the compiled link embedded inside an inline keyboard row
	inlineKeyboard := gobale.InlineMarkup().
		Row(gobale.Btn("💳 Pay with Wallet").URL(link)).
		Build()

	_, _ = c.Send().
		Text("Your payment invoice is ready. Please use the button below:").
		Markup(inlineKeyboard).
		Go()
})
```

---

### 3. Answering Pre-Checkout Validation (`answerPreCheckoutQuery`)

Before a user's wallet is debited, the Bale servers send a `PreCheckoutQuery` to your bot. You must validate item availability within 10 seconds and return either success (`OK(true)`) or a failure warning message to halt the transaction.

```go
bot.On().PreCheckout(func(c *gobale.Ctx) {
	query := c.Update.PreCheckoutQuery
	if query == nil {
		return
	}

	log.Printf("Validating purchase limit for Invoice Payload: %s, Total: %d IRR", 
		query.InvoicePayload, query.TotalAmount)

	// Perform internal inventory checks. If stock is available:
	_ = c.PreCheckout().OK(true).Go()
})
```

---

### 4. Handling Successful Payment Confirmation

Once the transaction is successfully completed, the user's client app posts a confirmation `SuccessfulPayment` message into the chat room.

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	msg := c.Message
	if msg == nil || msg.SuccessfulPayment == nil {
		return
	}

	payment := msg.SuccessfulPayment
	log.Printf("Payment Verified! Transaction ID: %s, Total: %d IRR", 
		payment.BalePaymentChargeID, payment.TotalAmount)

	_, _ = c.Send().
		Text(fmt.Sprintf("✅ Thank you for your payment! Transaction ID: %s", payment.BalePaymentChargeID)).
		Go()
})
```

---

### 5. Querying Transactions (`getTransaction` & `inquireTransaction`)

You can retrieve transaction details or inquire about a pending transaction status directly:

```go
bot.On().Cmd("checktx").Do(func(c *gobale.Ctx) {
	args, ok := c.Arg().([]string)
	if !ok || len(args) == 0 {
		return
	}
	transactionID := args[0]

	// Inquire transaction status directly on Bale servers
	tx, err := c.Pay().InquireTx(transactionID).Go()
	if err != nil {
		log.Printf("Inquiry failed: %v", err)
		return
	}

	log.Printf("Transaction ID: %s, Status: %s, Amount: %d IRR", tx.ID, tx.Status, tx.Amount)
})
```
---
## Bale Mini Apps Integration

GoBale provides native structures, fluent button builders, cryptographic verifiers, and context parsers to handle interactions with Bale Mini Apps entirely on the backend.

---

### 1. Launching Mini Apps via Fluent Keyboards

To launch a Mini App from a button, utilize the `.WebApp(url)` builder on either inline or reply keyboard builders:

```go
bot.On().Cmd("launch").Do(func(c *gobale.Ctx) {
	// Launch WebApp from an inline button
	inlineKeyboard := gobale.InlineMarkup().
		Row(gobale.Btn("🌐 Open Mini App").WebApp("https://your-app-domain.com")).
		Build()

	// Launch WebApp from a reply keyboard button
	replyKeyboard := gobale.ReplyMarkup().
		Row(gobale.ReplyBtn("🌐 Launch App").WebApp("https://your-app-domain.com")).
		Build()

	_, _ = c.Send().
		Text("Select an entry point:").
		Markup(inlineKeyboard).
		Go()

	_ = replyKeyboard // Retain reference to suppress unused variable warning
})
```

---

### 2. Launch Data Verification (`WebappChain`)

Before trusting any launch payload from your Mini App frontend, use GoBale’s cryptographic `Webapp` helper to verify the signature of `initData` using the bot's secret token and prevent replay attacks:

```go
// Verify WebApp initData on your Go backend API endpoint
func VerifyLaunchData(bot *gobale.Bot, rawInitData string) {
	// Verify signature and reject payloads older than 2 hours to prevent replay attacks
	isValid, err := bot.Webapp().
		Verify(rawInitData).
		Expire(2 * time.Hour).
		Go()

	if err != nil || !isValid {
		log.Printf("Authentication failed: %v", err)
		return
	}

	log.Println("WebApp launch parameters successfully verified!")
}
```

---

### 3. Handling Deep Links & WebApp Submissions

When a user launches a Mini App using a deep link parameter or submits data using the SDK, GoBale parses these payloads directly inside the handler context:

```go
bot.On().Msg().Do(func(c *gobale.Ctx) {
	msg := c.Message
	if msg == nil {
		return
	}

	// 1. Process data submitted directly from the Mini App
	if msg.WebAppData != nil {
		submittedData := msg.WebAppData.Data
		log.Printf("WebApp submitted data: %s", submittedData)
		return
	}

	// 2. Extract startapp parameter if the user entered via a ?startapp link
	if strings.HasPrefix(msg.Text, "/start ") {
		startParam := c.DeepLink()
		log.Printf("Mini App entry parameter: %s", startParam)
	}
})
```
---
## Safir REST API (Enterprise Messaging)

The **Safir** service is Bale's high-throughput, transactional enterprise messaging engine. It operates via a dedicated RESTful API, allowing organization accounts to dispatch text messages, One-Time Passcodes (OTPs), files, and inline keyboards directly to users. 

Authentication is managed via an organization-specific header: `api-access-key`.

---

### Strict Phone Number Normalization
To prevent delivery failures, Safir strictly rejects phone numbers containing spaces, dashes, or non-98 prefixes. Numbers **must** begin with `98` followed by exactly 10 digits (e.g., `989123456789`).
* **Built-in Automation:** GoBale's `.Phone()` helper automatically runs the built-in `NormalizeSafirPhone` routine under the hood to sanitize standard Iranian formats (e.g., converting `09123456789` to `989123456789`) and strip invalid characters.

---

### Safir Struct Definitions

```go
type SafirResponse struct {
	MessageID string     `json:"message_id"`
	ErrorData []SafirErr `json:"error_data"`
}

type SafirErr struct {
	PhoneNumber string `json:"phone_number"`
	Code        int    `json:"code"`
	Description string `json:"description"`
}
```

---

### 1. Sending Standard & Secure Messages

You can dispatch standard text messages or enable password-locked secure encryption (`is_secure: true`) using the fluent `Safir()` chain:

```go
bot.On().Cmd("safirsend").Do(func(c *gobale.Ctx) {
	// Dispatch a secure, password-locked transactional message via Safir
	resp, err := c.Safir().
		Phone("09123456789"). // GoBale automatically normalizes this to 989123456789
		Text("Confidential: Your billing statement is ready for review.").
		Secure(true).         // Enables secure password-locked bubble
		Go()

	if err != nil {
		log.Printf("Safir transaction failed: %v", err)
		return
	}

	log.Printf("Safir Message Dispatched. Message ID: %s", resp.MessageID)
})
```

---

### 2. Sending One-Time Passcodes (OTP)

To transmit critical numerical security passcodes, use the `.OTP()` configuration. This routes the message through Bale's official OTP system, adding native copy buttons and secure sender tags automatically.

```go
bot.On().Cmd("sendotp").Do(func(c *gobale.Ctx) {
	// Dispatch a secure numerical OTP code
	resp, err := c.Safir().
		Phone("09123456789").
		OTP("123456"). // Sends 123456 inside Bale's official OTP bubble
		Go()

	if err != nil {
		log.Printf("Failed to dispatch OTP: %v", err)
		return
	}

	log.Printf("OTP successfully dispatched. ID: %s", resp.MessageID)
})
```

---

### 3. File Uploading & Media Transactions

To send files or images via Safir, upload the physical file first using the `.Upload()` chain to obtain a unique `file_id`, then pass the ID to the messaging pipeline:

```go
bot.On().Cmd("safirupload").Do(func(c *gobale.Ctx) {
	file, err := os.Open("./assets/statement.pdf")
	if err != nil {
		log.Printf("Failed to open file: %v", err)
		return
	}
	defer file.Close()

	inputFile := gobale.InputFile{
		FileName: "statement.pdf",
		Reader:   file,
	}

	// 1. Upload the file first to the Safir upload endpoint
	safirChain := c.Safir()
	fileID, err := safirChain.Upload(inputFile).Go()
	if err != nil {
		log.Printf("Failed to upload file to Safir: %v", err)
		return
	}

	// 2. Dispatch the media message using the obtained file ID
	resp, err := safirChain.
		Phone("09123456789").
		FileID(fileID).
		Text("Here is your requested account statement:").
		Go()

	if err != nil {
		log.Printf("Failed to send Safir document: %v", err)
		return
	}

	log.Printf("Document sent successfully. Message ID: %s", resp.MessageID)
})
```

---

### 4. Idempotency Protection (`request_id`)
To prevent duplicate message dispatches caused by temporary network retries, GoBale automatically generates a cryptographically random, 12-character hexadecimal token as the `request_id` for each Safir chain call. 

If you prefer to enforce custom transaction IDs, specify them using the `.RequestID(id)` method:

```go
_, _ = c.Safir().
	Phone("09123456789").
	RequestID("unique_invoice_ref_456"). // Custom Idempotency Token
	Text("Invoice payment acknowledged.").
	Go()
```

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

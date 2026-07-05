# Centralized EventBus: Architectural Design, API Reference, and Verification Guide

The Unified EventBus is a high-performance, thread-safe, and panic-proof event distribution subsystem built natively into the core framework. It implements an asynchronous, topic-based **Publish-Subscribe (Pub-Sub)** pattern, decoupling core framework events (such as user joins, exits, payments, warnings, deep-links, system exceptions, and file transfers) from the main execution pipeline.

By routing lifecycle updates through the EventBus, secondary operations (such as auditing, remote logging, database writes, and administrative alerts) are offloaded to concurrent background worker goroutines, maintaining a sub-millisecond response latency on the main bot update thread.

---

## 1. Architectural Design & Thread-Safe Panic Isolation

```
+-------------------------------------------------------------------------------+
|                            UNIFIED EVENTBUS ARCHITECTURE                      |
|                                                                               |
|   [Core Framework Hooks] ----> (Publish)                                      |
|   - user.join                    |                                            |
|   - user.exit                    v                                            |
|   - payment.success --------> [Spawn Asynchronous Goroutine per Subscriber]    |
|   - user.warn                    |                                            |
|   - file.download                +---> [Deferred Panic Recovery Guard]        |
|   - bot.start                    |         |                                  |
|   - sys.error                    |         +---> Catch & Log Listener Panic   |
|                                  |               (Prevents bot crash!)        |
|                                  v                                            |
|                          [Listener Callback]                                  |
+-------------------------------------------------------------------------------+
```

### I. Thread-Safe Topic Routing
The EventBus maintains a synchronized map of subscriber lists partitioned by string namespace topics (e.g. `"user.join"` or `"payment.success"`). Access to the subscriber registry is protected by an optimized reader-writer mutual exclusion lock (`sync.RWMutex`), preventing data races during concurrent subscription setups at startup and runtime.

### II. Panic-Proof Concurrency Guard (Goroutine Isolation)
In Go, spawning a raw goroutine (`go listener()`) carries a critical runtime risk: if a panic occurs inside that spawned goroutine, and it is not explicitly recovered within that same goroutine, **the entire application crashes instantly**, bypassing any parent recovery middlewares.
* **The Solution:** The EventBus wraps every concurrent listener execution inside a deferred panic-recovery block:
  ```go
  go func(l EventListener) {
      defer func() {
          if r := recover(); r != nil {
              log.Printf("[EventBus Panic Recovery] Recovered from listener panic: %v", r)
          }
      }()
      l(payload)
  }(listener)
  ```
  This ensures that even if a developer writes a buggy event subscriber callback (e.g., resulting in a nil pointer dereference), the panic is caught and logged, while the main bot execution thread remains stable and operational.

---

## 2. Built-in System Event Hooks

The core framework automatically publishes the following events to the central EventBus behind the scenes [1]:

| Topic Name | Payload Type | Description |
| :--- | :--- | :--- |
| **`"user.join"`** | `map[string]any` | Fired when new members join a group chat. The map contains `"ChatID"` (`int64`) and `"User"` (`User`). |
| **`"user.exit"`** | `map[string]any` | Fired when a member leaves or is removed from a group chat. The map contains `"ChatID"` (`int64`) and `"User"` (`User`). |
| **`"payment.success"`** | `*SuccessfulPayment` | Fired when an invoice payment is processed successfully. Contains total amount and currency details. |
| **`"user.warn"`** | `map[string]any` | Fired when a policy warning is issued by the `WarnEngine`. Contains `"ChatID"`, `"UserID"`, `"Reason"`, and `"Count"`. |
| **`"file.download"`** | `map[string]any` | Fired when a file download completes. Contains local destination `"Path"`, source `"URL"`, and destination `"ChatID"` (`int64`). |
| **`"bot.start"`** | `map[string]any` | Fired when a user starts the bot with a deep link. Contains `"ChatID"`, `"DeepLink"` parameter, and `"Sender"` info. |
| **`"sys.error"`** | `error` | Fired when a system exception or recovered runtime panic is intercepted. |

---

## 3. API Reference

### `bot.On().Event(topic string, fn EventListener) *OnChain`
Registers a subscriber callback function `fn` to a specific topic on the unified EventBus.

---

## 4. Advanced E2E Event-Driven Bot Example (`main.go`)

The following complete, compiled-safe Go application (`main.go`) demonstrates how to build a fully event-driven bot that subscribes to all core system-level event hooks concurrently in the background, keeping the main bot update thread highly responsive:

```go
package main

import (
	"fmt"
	"log"
	"os"

	gobale "github.com/PHX-Go/GoBale"
)

func main() {
	// Load configurations from environment file
	gobale.Env().Go()

	token := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")

	// Initialize the bot instance
	bot, err := gobale.New(token).Admin(adminID).Go()
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	// 1. SUBSCRIBE TO SYSTEM EVENTS ON THE UNIFIED EVENTBUS

	// Catch user joins (user.join)
	bot.On().Event("user.join", func(payload any) {
		data := payload.(map[string]any)
		chat := data["ChatID"].(int64)
		user := data["User"].(gobale.User)
		
		log.Printf("[Event] User %s (%d) joined group chat %d", user.FirstName, user.ID, chat)
	})

	// Catch user exits (user.exit)
	bot.On().Event("user.exit", func(payload any) {
		data := payload.(map[string]any)
		chat := data["ChatID"].(int64)
		user := data["User"].(gobale.User)
		
		log.Printf("[Event] User %s (%d) left group chat %d", user.FirstName, user.ID, chat)
	})

	// Catch successful bank payments (payment.success)
	bot.On().Event("payment.success", func(payload any) {
		payment := payload.(*gobale.SuccessfulPayment)
		
		log.Printf("[Event] Received payment of %s IRR. Payload: %q", gobale.Money(payment.TotalAmount), payment.InvoicePayload)
	})

	// Catch warning infractions (user.warn)
	bot.On().Event("user.warn", func(payload any) {
		data := payload.(map[string]any)
		user := data["UserID"].(int64)
		reason := data["Reason"].(string)
		count := data["Count"].(int)
		
		log.Printf("[Event] User %d received warning #%d. Reason: %s", user, count, reason)
	})

	// Catch completed file downloads (file.download)
	bot.On().Event("file.download", func(payload any) {
		data := payload.(map[string]any)
		filePath := data["Path"].(string)
		chatID := data["ChatID"].(int64)

		log.Printf("[Event] File downloaded cleanly: %s. Relaying back...", filePath)

		// Send upload progress status message
		msg, err := bot.Send(chatID).Text("📤 File downloaded. Relaying back to client...").Go()
		if err != nil {
			_ = os.Remove(filePath)
			return
		}

		msgID := msg.MessageID
		var lastStep int

		// Upload file back to the user with a progress bar
		_, errUpload := bot.Send(chatID).
			Doc(filePath).
			OnProgress(func(pct float64) {
				step := int(pct) / 20
				if step > lastStep {
					lastStep = step
					bar := gobale.BuildProgressBar(pct)
					progressText := fmt.Sprintf("📤 Relaying file back to you...\n%s %.0f%%", bar, pct)
					_, _ = bot.Edit(chatID, msgID).Text(progressText).Go()
				}
			}).
			Queue().
			Go()

		// Always delete temporary file from server disk to prevent leaks
		_ = os.Remove(filePath)

		if errUpload != nil {
			_, _ = bot.Edit(chatID, msgID).Text("❌ Upload failed to server!").Go()
			return
		}

		// Update message with success indicator
		_, _ = bot.Edit(chatID, msgID).Text("✅ File relay completed. Disk cache cleaned.").Go()
	})

	// Catch marketing campaign deep-links (bot.start)
	bot.On().Event("bot.start", func(payload any) {
		data := payload.(map[string]any)
		chatID := data["ChatID"].(int64)
		deepLink := data["DeepLink"].(string)
		
		log.Printf("[Event] User in Chat %d joined via campaign: %s", chatID, deepLink)
	})

	// Catch critical system errors and panics (sys.error)
	bot.On().Event("sys.error", func(payload any) {
		sysErr := payload.(error)
		
		log.Printf("[Event] ALERT: Captured runtime exception: %v", sysErr)
	})

	// 2. REGISTER BASIC ROUTERS AND SERVICES

	// Register core system join and exit handlers
	bot.On().Join().Go()
	bot.On().Exit().Go()

	// Build WarnEngine
	bot.Warns().Limit(3).Build()

	// Handle incoming documents for background downloading
	bot.On().Msg().Do(func(c *gobale.Ctx) {
		if c.Message.Document == nil {
			c.Next()
			return
		}

		doc := c.Message.Document
		chatID, _ := c.ChatID()

		// Reject files exceeding 20MB pre-download
		const maxLimitBytes int64 = 20 * 1024 * 1024
		if doc.FileSize > maxLimitBytes {
			_, _ = c.Send().Text("⚠️ File size exceeds 20MB limit!").Go()
			return
		}

		_, _ = c.Send().Text("📥 Downloading document to server...").Go()

		// Copy permanent pointers to prevent context-recycling race conditions
		botInstance := c.Bot
		fileID := doc.FileID
		fileName := doc.FileName

		// Launch background download safely
		go func() {
			_, _ = botInstance.File(fileID).
				Download().
				ChatID(chatID).
				Path("./downloads").
				Name(fileName).
				Queue().
				Go()
		}()
	})

	// Handle start command
	bot.On().Cmd("start").Do(func(c *gobale.Ctx) {
		_, _ = c.Send().Text("Welcome to the fully event-driven bot!").Go()
	})

	log.Println("Bot is running...")
	bot.Run().Polling().Go()
}
```

---

## 5. End-to-End System Integration Test Suite (`event_bus_test.go`)

The following comprehensive, channel-based, and offline-capable integration test verifies that all 7 built-in system-level event hooks are concurrently published and captured by the centralized EventBus cleanly:

```go
package gobale

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// TestEventBusSystemIntegration executes a complete E2E integration test
// verifying that user joins, exits, payments, warnings, deep-links, system errors,
// and file downloads are automatically published and captured by the central EventBus.
func TestEventBusSystemIntegration(t *testing.T) {
	dbPath := DataPath("suite_event_db.gob")
	_ = os.Remove(dbPath)
	_ = os.Remove(dbPath + ".wal")

	// Initialize bot with DryRun to prevent real network requests
	bot, _ := New("mock_token_event_test").Go()
	bot.dbInstance = NewDatabase(dbPath)
	bot.Client.DryRun = true

	defer func() {
		_ = bot.dbInstance.Close()
		_ = os.Remove(dbPath)
		_ = os.Remove(dbPath + ".wal")
	}()

	const chatID int64 = 555666
	const userID int64 = 777888

	// Isolated buffered channels to prevent WaitGroup negative panics
	joinChan := make(chan bool, 10)
	exitChan := make(chan bool, 10)
	paymentChan := make(chan bool, 10)
	warnChan := make(chan bool, 10)
	downloadChan := make(chan bool, 10)
	startChan := make(chan bool, 10)
	errorChan := make(chan bool, 10)

	// Subscribe listeners to their respective topics
	bot.Bus.Subscribe("user.join", func(payload any) {
		joinChan <- true
	})

	bot.Bus.Subscribe("user.exit", func(payload any) {
		exitChan <- true
	})

	bot.Bus.Subscribe("payment.success", func(payload any) {
		paymentChan <- true
	})

	bot.Bus.Subscribe("user.warn", func(payload any) {
		warnChan <- true
	})

	bot.Bus.Subscribe("file.download", func(payload any) {
		downloadChan <- true
	})

	bot.Bus.Subscribe("bot.start", func(payload any) {
		startChan <- true
	})

	bot.Bus.Subscribe("sys.error", func(payload any) {
		errorChan <- true
	})

	t.Log("Simulating User Join update...")
	joinUpdate := &Update{
		Message: &Message{
			Chat: Chat{ID: chatID, Type: "group"},
			NewChatMembers: []User{
				{ID: userID, FirstName: "Kourosh"},
			},
		},
	}
	bot.On().Join().Go()
	bot.processUpdate(context.Background(), joinUpdate)

	t.Log("Simulating User Exit update...")
	exitUpdate := &Update{
		Message: &Message{
			Chat:           Chat{ID: chatID, Type: "group"},
			LeftChatMember: &User{ID: userID, FirstName: "Kourosh"},
		},
	}
	bot.On().Exit().Go()
	bot.processUpdate(context.Background(), exitUpdate)

	t.Log("Simulating Successful Payment update...")
	paymentUpdate := &Update{
		Message: &Message{
			Chat: Chat{ID: chatID, Type: "private"},
			SuccessfulPayment: &SuccessfulPayment{
				Currency:    "IRR",
				TotalAmount: 50000,
			},
		},
	}
	bot.processUpdate(context.Background(), paymentUpdate)

	t.Log("Simulating WarnEngine infraction...")
	engine := bot.Warns().Limit(3).Build()
	cWarn := &Ctx{
		Bot:     bot,
		Update:  &Update{Message: &Message{Chat: Chat{ID: chatID}, From: &User{ID: userID}}},
		Message: &Message{Chat: Chat{ID: chatID}, From: &User{ID: userID}},
	}
	_ = engine.Warn(cWarn, "Test spam")

	t.Log("Simulating Bot Start Deep Link update...")
	startUpdate := &Update{
		Message: &Message{
			Chat: Chat{ID: chatID, Type: "private"},
			From: &User{ID: userID, FirstName: "PHX"},
			Text: "/start campaign_event_bus_test",
		},
	}
	bot.processUpdate(context.Background(), startUpdate)

	t.Log("Simulating System Error Recovery...")
	// Temporarily mute OnError during simulated panic to keep test console clean
	oldOnError := bot.OnError
	bot.OnError = nil
	defer func() { bot.OnError = oldOnError }()

	handlePanic(bot, errors.New("simulated database panic"), nil)

	t.Log("Simulating File Download Completion...")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mock_file_contents"))
	}))
	defer server.Close()

	destPath := DataPath("event_bus_download.zip")
	_ = os.Remove(destPath)
	defer os.Remove(destPath)

	cDownload := &Ctx{
		Bot:     bot,
		Update:  &Update{Message: &Message{Chat: Chat{ID: chatID}}},
		Message: &Message{Chat: Chat{ID: chatID}},
	}

	// Execute download targeting the local offline mock server
	_, errDownload := cDownload.File(server.URL).
		Download().
		Path("test_downloads").
		Name("event_bus_download.zip").
		Go()

	defer os.RemoveAll("test_downloads")

	if errDownload != nil {
		t.Fatalf("Download failed inside test: %v", errDownload)
	}

	// Helper function to safely wait for a channel with a timeout boundary
	wait := func(ch chan bool, name string) {
		select {
		case <-ch:
			t.Logf("Success: Captured %q event cleanly", name)
		case <-time.After(200 * time.Millisecond):
			t.Errorf("Timeout: %q event failed to fire", name)
		}
	}

	// Assert each event individually using non-blocking channels
	wait(joinChan, "user.join")
	wait(exitChan, "user.exit")
	wait(paymentChan, "payment.success")
	wait(warnChan, "user.warn")
	wait(downloadChan, "file.download")
	wait(startChan, "bot.start")
	wait(errorChan, "sys.error")
}
```

---

## 6. Verification and Execution Commands

Navigate your terminal to the framework's root directory and run the following command to execute the isolated E2E integration test:

```bash
go test -v -run=TestEventBusSystemIntegration
```

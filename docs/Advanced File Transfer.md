# Advanced File Transfer, Resolution & Cancellation Guide

The Concurrent Transfer Engine is a high-performance, enterprise-grade file transfer subsystem designed to handle heavy concurrent uploads and downloads under strict memory constraints. It incorporates an autonomous, non-site-specific **Link Resolution Engine** that dynamically scans, parses, and resolves indirect landing pages into direct fetchable file links.

---

## 1. Architectural Overview & System Design

```
+-----------------------------------------------------------------------------+
|                          AUTONOMOUS RESOLUTION FLOW                         |
|                                                                             |
|  [User Link] ---> (ResolveDownloadURL)                                      |
|                            |                                                |
|                            +---> [Peek HTML Page]                           |
|                            |         |                                      |
|                            |         +---> Check Regex Patterns             |
|                            |         |       - Meta Refresh                 |
|                            |         |       - JS locations                 |
|                            |         |       - Download keyword anchors     |
|                            |         |       - Direct file extension HREFs  |
|                            |                                                |
|                            v                                                |
|                     [Direct File URL] ---> (resilientDownload)              |
|                                                  |                          |
|                                                  v                          |
|                                         - 5x Retries & Jitter               |
|                                         - Stall Watchdog (60s)              |
|                                         - Range-Resume appending            |
+-----------------------------------------------------------------------------+
```

### I. Streamed Multipart Uploads (`io.Pipe`)
In standard multipart uploading, files are fully buffered in memory before being sent over the network, causing severe Out-Of-Memory (OOM) risks under heavy load.
* **The Solution:** The framework utilizes `io.Pipe` and concurrent goroutines. The multipart fields and file blocks are streamed sequentially into the HTTP request body in real-time. This keeps the heap memory footprint constant (near zero) regardless of the file size.

### II. Resilient Downloads & Inactivity Watchdogs
Downloading large files over unstable networks often leads to silent stalls or timeouts.
* **The Solution:** The downloader incorporates a `StallTimeout` watchdog (defaulting to 60s). If no bytes are received within this window, the current attempt is cancelled and retried using exponential backoff with random jitter (`backoffWithJitter`). If the server supports HTTP Range headers, the downloader automatically resumes from the last successfully written byte on the disk.

### III. Autonomous Link Resolution Engine
Many online files are hosted behind redirection gates or landing pages.
* **The Solution:** Before starting a download, the downloader invokes `ResolveDownloadURL`. This utility peeks at the target URL's headers:
  - If it is a direct file, it starts downloading immediately.
  - If it is HTML, it reads up to 2MB of the page and scans it using optimized regular expressions to extract redirect patterns (meta refresh, JS location mutations, anchor tags mentioning "download"/"دانلود", or direct file extension hrefs).
  - It resolves relative paths to absolute URLs and follows them up to 3 hops.

### IV. Safe Multi-Job Cancellation Registry
The framework features a central, thread-safe `cancelRegistry` that monitors active background workers.
* **The Solution:** Calling `(*Bot).CancelDownload` or `CancelUpload` triggers context cancellation on all background worker goroutines associated with a specific chat ID. The workers immediately stop reading/writing streams, close the open system file descriptors, and clean up the registry references to prevent memory leaks.

---

## 2. API Reference

The following utility functions and methods are available globally under the package namespace:

| Function / Method | Signature | Description |
| :--- | :--- | :--- |
| **`ResolveDownloadURL`** | `ResolveDownloadURL(ctx, client, rawURL)` | Parses and resolves indirect links into direct fetchable download links. |
| **`BuildProgressBar`** | `BuildProgressBar(pct float64) string` | Generates a standard, visual text progress bar (e.g. `■■■■■□□□□□`) with memory-safe heap allocations. |
| **`CancelDownload`** | `(b *Bot) CancelDownload(chatID any) bool` | Cancels all active download tasks for a specific Chat ID globally. |
| **`CancelUpload`** | `CancelUpload(b *Bot, chatID any) bool` | Cancels all active upload tasks for a specific Chat ID globally. |

---

## 3. Production Scenarios & Implementation Guides

The following distinct scenarios demonstrate how to integrate the concurrent transfer pool, progress tracking, and automated cancellation and disk cleanup mechanisms into standard bot operations.

---

### Scenario A: Direct File Downloader (Direct URL Download)

This bot listens for `/download [URL]` commands, downloads the target file directly from the web to the server's local disk, and displays a dynamic progress bar.

```go
package main

import (
	"fmt"
	"log"

	gobale "github.com/PHX-Go/GoBale"
)

func main() {
	// Load environment variables
	gobale.Env().Go()

	token := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")

	// Initialize the bot instance
	bot, err := gobale.New(token).Admin(adminID).Go()
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	// Command to download files via url
	bot.On().Cmd("download").Do(func(c *gobale.Ctx) {
		args, ok := c.Arg().([]string)
		if !ok || len(args) == 0 {
			_, _ = c.Send().Text("⚠️ Invalid format:\n`/download [URL]`").Go()
			return
		}

		targetURL := args[0]
		chatID, _ := c.ChatID()

		// Send initial progress message
		msg, err := c.Send().Text("⏳ Preparing client connection...").Go()
		if err != nil {
			return
		}

		msgID := msg.MessageID
		var lastStep int

		// Start background download job
		destPath, errDownload := c.File(targetURL).
			Download().
			Path("./downloads").
			Name("web_document.zip").
			OnProgress(func(pct float64) {
				// Throttle edits to 20% steps to prevent rate limits
				step := int(pct) / 20
				if step > lastStep {
					lastStep = step
					bar := gobale.BuildProgressBar(pct)
					progressText := fmt.Sprintf("📥 Downloading file to server...\n%s %.0f%%", bar, pct)

					_, _ = c.Bot.Edit(chatID, msgID).Text(progressText).Go()
				}
			}).
			Queue(). // Dispatch to background worker pool
			Go()

		if errDownload != nil {
			_, _ = c.Bot.Edit(chatID, msgID).Text(fmt.Sprintf("❌ Download failed: %v", errDownload)).Go()
			return
		}

		// Update message with success indicator
		_, _ = c.Bot.Edit(chatID, msgID).Text(fmt.Sprintf("✅ File downloaded successfully:\n📁 %s", destPath)).Go()
	})

	log.Println("Direct Downloader Bot is running...")
	bot.Run().Polling().Go()
}
```

---

### Scenario B: Automated User File Relay Bridge (Download, Filter & Upload)

This bot listens for incoming documents sent directly by users. It validates the file size pre-download to reject any transfers exceeding **20 MB**, downloads the file locally, uploads it back to the user, and performs mandatory automatic file deletion immediately after transfer.

```go
package main

import (
	"fmt"
	"log"
	"os"

	gobale "github.com/PHX-Go/GoBale"
)

func main() {
	// Load environment variables
	gobale.Env().Go()

	token := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")

	// Initialize the bot instance
	bot, err := gobale.New(token).Admin(adminID).Go()
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	// Listen for incoming documents
	bot.On().Msg().Do(func(c *gobale.Ctx) {
		// Ignore updates with no document attachments
		if c.Message.Document == nil {
			c.Next()
			return
		}

		doc := c.Message.Document
		chatID, _ := c.ChatID()
		c.Log().Info("Received document: %s | Size: %d", doc.FileName, doc.FileSize)

		// Reject files exceeding 20MB pre-download
		const maxLimitBytes int64 = 20 * 1024 * 1024
		if doc.FileSize > maxLimitBytes {
			_, _ = c.Send().Text("⚠️ File size exceeds 20MB limit!").Go()
			return
		}

		// Send initial progress message
		msg, err := c.Send().Text("⏳ Downloading document to server...").Go()
		if err != nil {
			return
		}

		msgID := msg.MessageID
		var lastStep int

		// Start background download job
		destPath, errDownload := c.File(doc.FileID).
			Download().
			Path("./downloads").
			Name(doc.FileName).
			OnProgress(func(pct float64) {
				step := int(pct) / 20
				if step > lastStep {
					lastStep = step
					bar := gobale.BuildProgressBar(pct)
					progressText := fmt.Sprintf("📥 Downloading file to disk...\n%s %.0f%%", bar, pct)

					_, _ = c.Bot.Edit(chatID, msgID).Text(progressText).Go()
				}
			}).
			Queue(). // Dispatch to background worker pool
			Go()

		// Cleanup partial file on download error
		if errDownload != nil {
			_ = os.Remove(destPath)
			_, _ = c.Bot.Edit(chatID, msgID).Text("❌ Download failed from server!").Go()
			return
		}

		// Update message for upload phase
		_, _ = c.Bot.Edit(chatID, msgID).Text("📤 File downloaded. Relaying back to client...").Go()
		lastStep = 0

		// Upload file back to user
		_, errUpload := c.Send().
			Doc(destPath).
			OnProgress(func(pct float64) {
				step := int(pct) / 20
				if step > lastStep {
					lastStep = step
					bar := gobale.BuildProgressBar(pct)
					progressText := fmt.Sprintf("📤 Relaying file back to you...\n%s %.0f%%", bar, pct)

					_, _ = c.Bot.Edit(chatID, msgID).Text(progressText).Go()
				}
			}).
			Queue(). // Dispatch to background worker pool
			Go()

		// Always delete temporary file from server disk to prevent leaks
		_ = os.Remove(destPath)

		if errUpload != nil {
			_, _ = c.Bot.Edit(chatID, msgID).Text("❌ Upload failed to server!").Go()
			return
		}

		// Update message with success indicator
		_, _ = c.Bot.Edit(chatID, msgID).Text("✅ File relay completed. Disk cache cleaned.").Go()
	})

	log.Println("Relay Bridge Bot is running...")
	bot.Run().Polling().Go()
}
```

---

### Scenario C: Active Transfer Cancellation (لغو و توقف ترانسفرهای فعال چت)

This bot registers a `/cancel` command. When triggered, it invokes `CancelDownload` and `CancelUpload` to instantly abort any background file transfers currently active for that specific chat session.

```go
package main

import (
	"log"

	gobale "github.com/PHX-Go/GoBale"
)

func main() {
	// Load environment variables
	gobale.Env().Go()

	token := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")

	// Initialize the bot instance
	bot, err := gobale.New(token).Admin(adminID).Go()
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	// Command to cancel all ongoing transfers in this chat
	bot.On().Cmd("cancel").Do(func(c *gobale.Ctx) {
		chatID, _ := c.ChatID()

		// Cancel any active background downloads in this chat
		canceledDownload := c.Bot.CancelDownload(chatID)

		// Cancel any active background uploads in this chat
		canceledUpload := gobale.CancelUpload(c.Bot, chatID)

		// Send feedback based on canceled jobs
		if canceledDownload || canceledUpload {
			_, _ = c.Send().Text("🛑 Active background transfers canceled successfully.").Go()
		} else {
			_, _ = c.Send().Text("❓ No active transfers found for this chat session.").Go()
		}
	})

	log.Println("Cancellation Controller Bot is running...")
	bot.Run().Polling().Go()
}
```

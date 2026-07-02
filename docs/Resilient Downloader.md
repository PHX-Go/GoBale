# Concurrent Resilient Downloader Pool Module

The **Concurrent Resilient Downloader Pool** is an enterprise-grade, bounded-queue download manager built directly into the framework core. It handles file downloads from both Bale servers (`file_id`) and external direct HTTP/HTTPS links safely. It protects server resources (CPU, Disk I/O, and Bandwidth) under high concurrency, provides auto-resuming, multi-attempt retries on network drops, and supports live, throttled progress updates and manual cancellations.

---

## Core Capabilities

1. **Bounded Concurrency Pool:** Spawns a dedicated, configurable worker pool to process queued downloads, protecting your VPS from bandwidth choking and disk IO lockups.
2. **Multi-Attempt Auto-Retry:** Transparently sleeps and retries up to `5` times on read timeouts, connection resets, or temporary network drops.
3. **HTTP Range Resuming (Auto-Resume):** If a download is interrupted, the system automatically requests partial content (`Range: bytes=X-`) and appends incoming bytes to the existing file, saving time and bandwidth.
4. **Smart Progress Tracking:** Percentage calculations are throttled and fire exactly at integer increments (1% to 100%) to respect Bale's strict message-editing rate limits.
5. **Original Name Preservation:** Automatically extracts and maintains the original filename and extension of user-uploaded documents, audio, or video files synchronously before context recycling.
6. **Direct URL Downloading (Leeching):** Automatically detects if the input is an HTTP/HTTPS link, bypassing Bale's `getFile` API call and downloading the file directly while utilizing the same concurrent queue and progress tracking.
7. **Task Cancellation (Stop/Cancel):** Exposes a global `b.CancelDownload(chatID)` function to immediately cancel the active context of a download job, terminating the network stream and cleaning up resources.

---

## Fluent API Reference

### `bot.InitDownloadPool(workers ...int)`
* Initializes the concurrent download queue.
* `workers`: Optional. Configures the maximum number of simultaneous downloads (defaults to `4`).

### `c.File(source)` / `b.File(source)`
* Initiates the file chain container.
* `source`: A Bale native `file_id` string OR a direct HTTP/HTTPS URL.

### Fluent Methods on `DownloadChain`:
* `.Download()`: Prepares the download pipeline.
* `.Path(dir)`: Sets the target directory where the file will be saved.
* `.Name(filename)`: Optional. Overrides the filename with a custom one.
* `.OnProgress(func(pct float64))`: Optional. Registers a callback triggered when the download progress percentage increases.
* `.Queue()`: Configures the task to run inside the concurrent background worker pool.
* `.Go()`: Executes the download synchronously or asynchronously and returns the saved file path (`string`) and an `error`.

---

## Code Examples

### 1. Downloading Native Bale Files with Progress
This example handles files sent by users, verifies their size against a safe threshold (45MB), and downloads them asynchronously using the concurrent pool.

```go
package main

import (
	"context"
	"fmt"
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

	// Initialize download pool with 4 concurrent workers
	bot.InitDownloadPool(4)

	// Handler triggered when a user sends a file
	bot.On().Msg().Do(func(c *gobale.Ctx) {
		if c.Message == nil || c.Message.Document == nil {
			return
		}

		fileID := c.Message.Document.FileID

		// Fetch file metadata to inspect size before downloading
		fileInfo, err := c.File(fileID).Info().Go()
		if err != nil {
			return
		}

		// Enforce safety limit of 45MB
		const maxSafeLimitBytes = 45 * 1024 * 1024
		if fileInfo.FileSize > maxSafeLimitBytes {
			_, _ = c.Send().Text("⚠️ File size exceeds the allowed limit (45MB).").Go()
			return
		}

		// Initialize download chain synchronously to capture original name before Ctx recycling!
		downloadChain := c.File(fileID).
			Download().
			Path("./my_downloads"). // Saved automatically with original filename and extension
			Queue()

		// Capture safe local variables for background task to prevent context recycling races
		chatID, _ := c.ChatID()
		botInstance := c.Bot

		c.Go(func() {
			// Send starting status message
			statusMsg, err := botInstance.Send(chatID).Text("⏳ Starting download... (0%)").Go()
			if err != nil {
				return
			}

			// Execute the pre-constructed download chain in the background pool
			destPath, err := downloadChain.
				OnProgress(func(pct float64) {
					progressText := fmt.Sprintf("⏳ Downloading... (%.0f%%)", pct)
					_, _ = botInstance.Edit(chatID, statusMsg.MessageID).Text(progressText).Go()
				}).
				Go()

			if err != nil {
				if err == context.Canceled {
					_, _ = botInstance.Edit(chatID, statusMsg.MessageID).Text("⏹️ Download canceled by user.").Go()
				} else {
					_, _ = botInstance.Edit(chatID, statusMsg.MessageID).Text("❌ Download failed.").Go()
				}
				return
			}

			// Download completed successfully
			successText := fmt.Sprintf("✅ File successfully saved to disk:\n`%s`", destPath)
			_, _ = botInstance.Edit(chatID, statusMsg.MessageID).Text(successText).Go()
		})
	})

	log.Println("Bot is running...")
	bot.Run().Polling().Go()
}
```

---

### 2. Leeching Direct HTTP Links and Cancelling via `/stop`
This example demonstrates how to download files from direct web links using the `/leech [URL]` command, and how to stop/cancel any active download using the `/stop` command.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

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

	// Initialize download pool with 4 concurrent workers
	bot.InitDownloadPool(4)

	// Handler triggered when a user sends /stop command to cancel their active task
	bot.On().Cmd("stop").Do(func(c *gobale.Ctx) {
		chatID, _ := c.ChatID()

		// Attempt to cancel any active download for this chat ID globally
		if c.Bot.CancelDownload(chatID) {
			_, _ = c.Send().Text("⏹️ Download process cancelled successfully.").Go()
		} else {
			_, _ = c.Send().Text("⚠️ No active download found for this chat.").Go()
		}
	})

	// Handler triggered when a user sends /leech [URL] command
	bot.On().Cmd("leech").Do(func(c *gobale.Ctx) {
		var link string
		err := c.ScanArgs(&link)
		if err != nil || link == "" {
			_, _ = c.Send().Text("⚠️ Usage:\n`/leech https://example.com/file.zip`").Markdown().Go()
			return
		}

		if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
			_, _ = c.Send().Text("❌ Invalid link. Must start with http or https.").Go()
			return
		}

		// Initialize download chain synchronously to capture URL details before Ctx recycling!
		downloadChain := c.File(link).
			Download().
			Path("./my_downloads"). // Automatically extracts "file.zip" from URL and decodes %20 spaces
			Queue()

		// Capture safe local variables
		chatID, _ := c.ChatID()
		botInstance := c.Bot

		c.Go(func() {
			// Send starting status message
			statusMsg, err := botInstance.Send(chatID).Text("⏳ Leeching... (0%)").Go()
			if err != nil {
				return
			}

			// Run download inside the concurrent queue pool
			destPath, err := downloadChain.
				OnProgress(func(pct float64) {
					progressText := fmt.Sprintf("⏳ Leeching... (%.0f%%)", pct)
					_, _ = botInstance.Edit(chatID, statusMsg.MessageID).Text(progressText).Go()
				}).
				Go()

			if err != nil {
				if err == context.Canceled {
					_, _ = botInstance.Edit(chatID, statusMsg.MessageID).Text("⏹️ Leeching cancelled by user.").Go()
				} else {
					_, _ = botInstance.Edit(chatID, statusMsg.MessageID).Text("❌ Leeching failed.").Go()
				}
				return
			}

			// Leeching completed successfully
			successText := fmt.Sprintf("✅ File leeched and saved:\n`%s`", destPath)
			_, _ = botInstance.Edit(chatID, statusMsg.MessageID).Text(successText).Go()
		})
	})

	log.Println("Bot is running...")
	bot.Run().Polling().Go()
}
```

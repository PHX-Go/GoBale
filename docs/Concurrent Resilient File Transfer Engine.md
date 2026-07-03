# Concurrent Resilient File Transfer Engine

The **File Transfer Engine** (implemented via `transfer.go` with integrations in `context.go` and `send.go`) is a highly concurrent, resilient file transfer subsystem designed for Bale Messenger bots. It manages both inbound (downloads) and outbound (uploads) media pipelines. It protects server resources (CPU, Disk I/O, and Bandwidth) from traffic spikes by enforcing queue-based concurrency limits, implements range-resuming and multi-attempt retries for unreliable networks, tracks true network progress (throttled to respect rate limits), and automatically preserves original file names.

---

## 1. Core Architecture

### 1.1 Memory and Context Safety (`sync.Pool` Protection)
To minimize Garbage Collection overhead under heavy traffic, the framework recycles context (`Ctx`) pointers using a `sync.Pool` immediately after a message handler returns. 
When executing a file transfer in an asynchronous background goroutine (`c.Go`):
* The context pointer `c` must **never** be accessed inside the goroutine, as it will be recycled and its fields reset to `nil`, leading to nil pointer dereferences.
* **The Solution:** We capture the `chatID` and `Bot` instance synchronously outside the goroutine. Furthermore, the `DownloadChain` is pre-constructed synchronously, allowing the framework to safely capture original filenames and metadata before the context is recycled.

### 1.2 Resilient Inbound Pipeline (Downloads)
* **Automatic Range-Resuming:** If a download is interrupted midway, the system inspects the partial file size on disk and injects the HTTP `Range: bytes=X-` header in the next attempt. Server-side chunks are appended seamlessly (`os.O_APPEND`) to the existing file.
* **Multi-Attempt Backoff Retry:** Temporarily network drops or read timeouts trigger a sleep-and-retry cycle up to 5 times per task.
* **Dual-Source Input:** The engine automatically detects if the source is a native Bale `file_id` (queries metadata via `getFile` API) or an arbitrary direct HTTP/HTTPS URL (bypasses API calls and downloads directly).

### 1.3 Resilient Outbound Pipeline (Uploads)
* **True Network Progress Tracking:** Unlike client-side serialization counters, the uploader wraps the prepared multipart request body in a custom `progressReader`. As the standard HTTP library writes bytes to the TCP socket, the reader tracks exactly what is sent over the wire.
* **Multi-Attempt Reset Retry:** If an upload fails midway, the system seeks the file reader back to `0`, rebuilds the multipart payload, and retries the upload from scratch up to 5 times.

### 1.4 Smart Progress Throttling
Updating progress messages too frequently violates Bale's rate limits, causing the server to reject edits. The `progressReader` tracks transmission progress and triggers the `onProgress` callback **only when the integer percentage changes** (from 1% to 100%), reducing network overhead.

---

## 2. Fluent API Reference

### 2.1 Engine Initialization

#### `bot.InitDownloadPool(workers ...int)`
* Initializes the concurrent download queue.
* `workers`: Optional. Maximum simultaneous active downloads (defaults to `4`).

#### `bot.InitUploadPool(workers ...int)`
* Initializes the concurrent upload queue.
* `workers`: Optional. Maximum simultaneous active uploads (defaults to `4`).

---

### 2.2 Inbound API (`DownloadChain`)

#### `c.File(source)` / `b.File(source)`
* Initiates the file container chain.
* `source`: A Bale `file_id` OR a direct HTTP/HTTPS web link. When called on `c *Ctx`, it automatically extracts the original file name and extension from the incoming message.

#### `.Download()`
* Spawns the download pipeline.

#### `.Path(directory)`
* Configures the target local directory where the file will be saved.

#### `.Name(filename)`
* Optional. Overrides the auto-extracted filename with a custom name.

#### `.OnProgress(func(pct float64))`
* Optional. Registers a throttled callback receiving progress updates from `1.0` to `100.0`.

#### `.Queue()`
* Enforces the download to run inside the bounded concurrency pool. If omitted, the download runs synchronously on the current thread.

#### `.Go()`
* Executes the pipeline, blocking until completion, and returns the saved file path (`string`) and an `error`.

---

### 2.3 Outbound API (`SendChain`)

#### `bot.Send(chatID)` / `c.Send()`
* Initiates the standard message sending pipeline.

#### `.Doc(path)` / `.Audio(path)` / `.Video(path)` ...
* Appends a local media file to the pipeline.

#### `.Caption(text)`
* Optional. Appends a caption to the media attachment.

#### `.OnProgress(func(pct float64))`
* Optional. Registers a throttled callback receiving actual upload progress from `1.0` to `100.0`.

#### `.Queue()`
* Enforces the upload to run inside the bounded concurrency pool. If omitted, the upload runs synchronously on the current thread.

#### `.Go()`
* Executes the upload, blocking until completion, and returns the sent `*Message` and an `error`.

---

### 2.4 Cancellation API

#### `bot.CancelDownload(chatID)`
* Cancels an active download task for the given chat ID. Returns `true` if a task was successfully canceled.

#### `gobale.CancelUpload(bot, chatID)`
* Cancels an active upload task for the given chat ID. Returns `true` if a task was successfully canceled.

---

## 3. Complete Integration Example

The following `main.go` code demonstrates a fully featured, production-ready **Leech & Re-upload Bot**. It takes a direct web link, downloads it through the concurrent queue, uploads it back to the group with progress bars, cleans up the local file, and supports a unified `/stop` command.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
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

	// 1. Initialize concurrent pools for both downloads and uploads
	bot.InitDownloadPool(4)
	bot.InitUploadPool(4)

	// 2. Handler triggered when a user sends /stop command to abort active transfers
	bot.On().Cmd("stop").Do(func(c *gobale.Ctx) {
		chatID, _ := c.ChatID()

		// Cancel download and upload tasks concurrently
		downloadCanceled := c.Bot.CancelDownload(chatID)
		uploadCanceled := gobale.CancelUpload(c.Bot, chatID)

		if downloadCanceled || uploadCanceled {
			_, _ = c.Send().Text("⏹️ Transfer process canceled successfully.").Go()
		} else {
			_, _ = c.Send().Text("⚠️ No active download or upload found for this chat.").Go()
		}
	})

	// 3. Handler triggered when a user sends /leech [URL] command
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

		// Pre-construct the download chain synchronously to capture URL details and original name
		downloadChain := c.File(link).
			Download().
			Path("./my_downloads"). // Automatically extracts filename from URL and decodes URL spaces
			Queue()

		// Capture safe local variables to prevent context recycling races
		chatID, _ := c.ChatID()
		botInstance := c.Bot

		c.Go(func() {
			// Send starting status message
			statusMsg, err := botInstance.Send(chatID).Text("⏳ Leeching... (0%)").Go()
			if err != nil {
				return
			}

			// Run download inside the concurrent queue pool with range-resuming
			destPath, err := downloadChain.
				OnProgress(func(pct float64) {
					progressText := fmt.Sprintf("⏳ Leeching... (%.0f%%)", pct)
					_, _ = botInstance.Edit(chatID, statusMsg.MessageID).Text(progressText).Go()
				}).
				Go()

			if err != nil {
				if err == context.Canceled {
					_, _ = botInstance.Edit(chatID, statusMsg.MessageID).Text("⏹️ Leeching canceled by user.").Go()
				} else {
					_, _ = botInstance.Edit(chatID, statusMsg.MessageID).Text("❌ Leeching failed.").Go()
				}
				return
			}

			// Construct the upload chain synchronously for the downloaded local file
			uploadChain := botInstance.Send(chatID).
				Doc(destPath).
				Caption("📦 File leeched and uploaded successfully.").
				OnProgress(func(pct float64) {
					progressText := fmt.Sprintf("📤 Uploading to Bale... (%.0f%%)", pct)
					_, _ = botInstance.Edit(chatID, statusMsg.MessageID).Text(progressText).Go()
				}).
				Queue() // Bounded upload queue concurrency control

			// Update status message to starting upload
			_, _ = botInstance.Edit(chatID, statusMsg.MessageID).Text("📤 Starting upload... (0%)").Go()

			// Run the upload chain in background pool
			_, errUpload := uploadChain.Go()
			_ = os.Remove(destPath) // Guarantee cleanup of the temporary file from VPS hdd

			if errUpload != nil {
				if errUpload == context.Canceled {
					_, _ = botInstance.Edit(chatID, statusMsg.MessageID).Text("⏹️ Upload canceled by user.").Go()
				} else {
					_, _ = botInstance.Edit(chatID, statusMsg.MessageID).Text("❌ Upload failed.").Go()
				}
				return
			}

			// Delete the status progress message cleanly upon successful completion
			_ = botInstance.BaseRequest(context.Background(), "deleteMessage", map[string]any{
				"chat_id":    chatID,
				"message_id": statusMsg.MessageID,
			}, nil)
		})
	})

	log.Println("Bot is running...")
	bot.Run().Polling().Go()
}
```

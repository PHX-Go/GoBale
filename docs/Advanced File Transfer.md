# Advanced File Transfer, Split-ZIP Archiving & Dynamic Resolution Guide

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

### II. Native Split-ZIP Volume Archiving
Bale Bot API enforces a strict 20MB limit for file uploads.
* **The Solution:** Enabling `.Split(true)` invokes `zipAndSplitFile` in the background. This utility compresses the original file using standard Deflate compression into a single `.zip` archive, and then splits it into multiple standard compressed ZIP volumes (named `.zip.001`, `.zip.002`, etc.) matching the specified `.SplitSize()` threshold. These volumes can be natively joined and extracted by standard ZIP utilities like WinRAR or 7-Zip.

### III. Automated Original File Cleanup (`.Cleanup(true)`)
Hosting downloaded files and split part volumes on the disk poses a risk of disk space exhaustion.
* **The Solution:** Chaining `.Cleanup(true)` registers a deferred `os.Remove(localPath)` block at the beginning of the transmission execution. This guarantees that the original local file is automatically and silently deleted from the server's disk upon completion, whether the upload succeeds, fails, or panics in the middle of execution.

### IV. Dynamic Multi-Format Media Resolver (`c.GetActiveFile()`)
Media files (photos, animations, voice notes, stickers, or nameless documents) often lack a native file name or extension.
* **The Solution:** `c.GetActiveFile()` dynamically inspects the incoming message:
  - If a file name is present in `Document`, it extracts its extension natively.
  - If no file name is found but a `MimeType` is present, it dynamically parses the extension from the MIME subtype (e.g., converting `video/mp4` to `.mp4` or `image/webp` to `.webp`).
  - If no media is found in the current message but a `reply_to_message` is present, it automatically falls back to inspect the replied-to file!
  - It resolves Bale-specific colon-separated `file_id` parameters to extract the unique trailing access hash, preventing duplicate name collisions on disk.

---

## 2. API Reference

The following utility functions and methods are available globally under the package namespace:

| Function / Method | Signature | Description |
| :--- | :--- | :--- |
| **`c.DownloadFile`** | `(c *Ctx) DownloadFile(fileID, path, name...)` | Natively downloads any file by its ID into a local path in a single line. |
| **`c.GetActiveFile`** | `(c *Ctx) GetActiveFile() (id, name, err)` | Automatically resolves file ID and dynamic extension for any active/replied media. |
| **`ParseSize`** | `ParseSize(s string) (bytes, err)` | Converts human-readable size strings (supporting KB/MB, e.g., `"15m"`, `"500k"`) into raw bytes. |
| **`BuildProgressBar`** | `BuildProgressBar(pct float64) string` | Generates a standard, visual text progress bar (e.g. `■■■■■□□□□□`) with memory-safe heap allocations. |
| **`CancelDownload`** | `(b *Bot) CancelDownload(chatID any) bool` | Cancels all active download tasks for a specific Chat ID globally. |
| **`CancelUpload`** | `CancelUpload(b *Bot, chatID any) bool` | Cancels all active upload tasks for a specific Chat ID globally. |

---

## 3. Production Scenarios & Implementation Guides

The following distinct scenarios demonstrate how to integrate the concurrent transfer pool, split-ZIP archiving, progress tracking, and automated disk cleanup mechanisms into standard bot operations.

---

### Scenario A: URL Uploader Bot (with Split-ZIP Archiving & Progress)

This bot listens for `/upload [URL]` commands, downloads the target file natively to the server's disk, compresses and splits it into standard 15MB ZIP volumes, uploads them with a dynamic progress bar, and automatically cleans up the disk.

```go
package main

import (
	"fmt"
	"log"

	gobale "github.com/PHX-Go/GoBale"
)

func main() {
	gobale.Env().Go()

	token := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")

	bot, err := gobale.New(token).Admin(adminID).Go()
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	// URL Uploader: Download a large file, auto-compress, split into ZIP volumes, and upload natively
	bot.On().Cmd("upload").Roles(gobale.RoleOwner).Do(func(c *gobale.Ctx) {
		url := c.ArgString(0)
		if url == "" {
			_, _ = c.ReplyText("⚠️ Please provide a direct download link. Example:\n`/upload http://example.com/movie.mp4`")
			return
		}

		// Send initial progress message
		statusMsg, err := c.ReplyText("⏳ Downloading file from link to server...")
		if err != nil {
			return
		}

		// Download the large file natively in 1 line (auto-extracts name from URL)
		localPath, err := c.DownloadFile(url, "./downloads")
		if err != nil {
			_, _ = c.Bot.Edit(c.Message.Chat.ID, statusMsg.MessageID).Text("❌ Download failed!").Go()
			return
		}

		_, _ = c.Bot.Edit(c.Message.Chat.ID, statusMsg.MessageID).Text("⏳ Initiating safe split ZIP upload (15MB parts)...").Go()

		// Natively compress, split into 15MB ZIP volumes, upload with progress, and auto-cleanup
		_, errUpload := c.Send().
			Doc(localPath).
			Split(true).
			SplitSize("15m"). // Compress and split into 15MB ZIP parts natively (supports "500k", "15m", etc.)
			Cleanup(true).   // Automatically deletes original localPath file from disk on completion natively!
			OnProgress(func(pct float64) {
				progressBar := gobale.BuildProgressBar(pct)
				_, _ = c.Bot.Edit(c.Message.Chat.ID, statusMsg.MessageID).
					Text(fmt.Sprintf("⏳ Uploading split ZIP archive...\n\n%s %.0f%%", progressBar, pct)).
					Go()
			}).
			Go()

		if errUpload != nil {
			_, _ = c.Bot.Edit(c.Message.Chat.ID, statusMsg.MessageID).Text("❌ Split upload failed!").Go()
			return
		}

		_, _ = c.Bot.Edit(c.Message.Chat.ID, statusMsg.MessageID).
			Text("✅ Multi-part split ZIP upload completed! Original downloads and temp parts cleared.").
			Go()

		_ = c.Delete() // Delete trigger command natively at the very end
	})

	log.Println("Uploader Bot is running...")
	bot.Run().Polling().Go()
}
```

---

### Scenario B: Multi-Format Media Downloader (with Reply & MIME Fallbacks)

This bot listens for any incoming media files (or replied-to documents, photos, stickers, GIFs, voice notes). It dynamically resolves the file ID, extracts or resolves the original file name and extension based on the MIME type, and downloads the file natively in a single line.

```go
package main

import (
	"fmt"
	"log"

	gobale "github.com/PHX-Go/GoBale"
)

func main() {
	gobale.Env().Go()

	token := gobale.GetEnv[string]("BALE_TOKEN")
	adminID := gobale.GetEnv[int64]("ADMIN_ID")

	bot, err := gobale.New(token).Admin(adminID).Go()
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	// Message handler: Download any incoming media or replied file natively with original name
	bot.On().Msg().Do(func(c *gobale.Ctx) {
		// Dynamically resolves active/replied file ID and handles MIME subtype fallback extensions
		fileID, fileName, err := c.GetActiveFile()
		if err != nil {
			return // Ignore text messages or non-media updates silently
		}

		// Download the resolved file natively in 1 line
		path, errDl := c.DownloadFile(fileID, "./downloads", fileName)
		if errDl != nil {
			_, _ = c.ReplyText("❌ Failed to download your file!")
			return
		}

		_, _ = c.ReplyText(fmt.Sprintf("💾 Saved natively as [%s] in: %s", fileName, path))
	})

	log.Println("Downloader Bot is running...")
	bot.Run().Polling().Go()
}
```

---

### Scenario C: Active Transfer Cancellation

This bot registers a `/cancel` command. When triggered, it invokes `CancelDownload` and `CancelUpload` to instantly abort any background file transfers currently active for that specific chat session.

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

	bot, err := gobale.New(token).Admin(adminID).Go()
	if err != nil {
		log.Fatalf("Failed to initialize bot: %v", err)
	}

	// Command to cancel all ongoing transfers in this chat
	bot.On().Cmd("cancel").Do(func(c *gobale.Ctx) {
		chatID, _ := c.ChatID()

		// Cancel any active background downloads and uploads in this chat
		canceledDownload := c.Bot.CancelDownload(chatID)
		canceledUpload := gobale.CancelUpload(c.Bot, chatID)

		if canceledDownload || canceledUpload {
			_, _ = c.ReplyText("🛑 Active background transfers canceled successfully.")
		} else {
			_, _ = c.ReplyText("❓ No active transfers found for this chat session.")
		}
		
		_ = c.Delete() // Delete trigger command natively at the very end
	})

	log.Println("Cancellation Controller Bot is running...")
	bot.Run().Polling().Go()
}
```

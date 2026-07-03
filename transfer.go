package gobale

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"
)

// progressReader wraps io.Reader to track and throttle data transfer progress events (1% to 100%)
type progressReader struct {
	r          io.Reader
	total      int64
	read       int64
	lastPct    int
	onProgress func(percent float64)
}

// Read implements standard io.Reader interface with progress notification
func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	pr.read += int64(n)
	if pr.total > 0 && pr.onProgress != nil && n > 0 {
		pct := int(float64(pr.read) / float64(pr.total) * 100.0)
		// Trigger callback only when integer percentage increases to prevent flooding
		if pct > pr.lastPct {
			pr.lastPct = pct
			pr.onProgress(float64(pct))
		}
	}
	return n, err
}

// DownloadJob encapsulates task state for background queue pipeline
type DownloadJob struct {
	url        string
	destPath   string
	totalSize  int64
	ctx        context.Context
	client     *http.Client
	onProgress func(percent float64)
	resultChan chan error
	chatID     string
}

// DownloadPool manages bounded concurrent downloads using background workers and cancellation maps
type DownloadPool struct {
	jobChan chan *DownloadJob
	workers int
	once    sync.Once
	active  sync.Map
}

// start spawns concurrent download workers with context cancellation wrappers
func (dp *DownloadPool) start(workers int) {
	dp.once.Do(func() {
		dp.jobChan = make(chan *DownloadJob, 1000)
		dp.workers = workers
		for i := 0; i < workers; i++ {
			go func() {
				for job := range dp.jobChan {
					// Create a cancellable context for each queued job
					jobCtx, cancel := context.WithCancel(job.ctx)
					if job.chatID != "" {
						dp.active.Store(job.chatID, cancel)
					}

					// Run the resilient download with retry and resume capabilities (updated: removed job.chatID)
					err := resilientDownload(jobCtx, job.client, job.url, job.destPath, job.totalSize, job.onProgress)

					if job.chatID != "" {
						dp.active.Delete(job.chatID)
					}
					// Clean up context resources
					cancel()

					job.resultChan <- err
				}
			}()
		}
	})
}

// resilientDownload handles range-resuming, multi-attempt retries, and manual cancellations (updated: removed chatID parameter)
func resilientDownload(ctx context.Context, client *http.Client, url, destPath string, expectedSize int64, onProgress func(percent float64)) error {
	const maxRetries = 5
	var currentSize int64 = 0
	var lastPct = -1

	// If the file already exists, check its size to resume partial content
	if stat, err := os.Stat(destPath); err == nil {
		currentSize = stat.Size()
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Stop immediately if the download has been canceled
		if ctx.Err() != nil {
			return ctx.Err()
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}

		// Inject HTTP Range header if resuming from a partial file
		if currentSize > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", currentSize))
		}

		resp, err := client.Do(req)
		if err != nil {
			// Backoff and retry on network dropouts
			time.Sleep(2 * time.Second)
			continue
		}

		isResume := resp.StatusCode == http.StatusPartialContent
		if resp.StatusCode == http.StatusOK {
			// Range header was ignored by server; restart download from scratch
			currentSize = 0
			isResume = false
		} else if resp.StatusCode != http.StatusPartialContent && currentSize > 0 {
			resp.Body.Close()
			if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
				// File is already fully downloaded!
				return nil
			}
			// Reset and retry without Range header
			currentSize = 0
			time.Sleep(1 * time.Second)
			continue
		}

		// Open target file either in append mode (resume) or truncate mode (new start)
		var out *os.File
		if isResume {
			out, err = os.OpenFile(destPath, os.O_WRONLY|os.O_APPEND, 0600)
		} else {
			out, err = os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		}
		if err != nil {
			resp.Body.Close()
			return err
		}

		// Recalculate true file size based on partial or full HTTP headers
		totalSize := resp.ContentLength
		if isResume {
			totalSize += currentSize
		}
		if totalSize <= 0 {
			totalSize = expectedSize
		}

		var buffer = make([]byte, 32*1024)
		var readErr error
		var bytesRead int

		// Stream reading block loop
		for {
			if ctx.Err() != nil {
				out.Close()
				resp.Body.Close()
				return ctx.Err()
			}

			bytesRead, readErr = resp.Body.Read(buffer)
			if bytesRead > 0 {
				_, writeErr := out.Write(buffer[:bytesRead])
				if writeErr != nil {
					out.Close()
					resp.Body.Close()
					return writeErr
				}
				currentSize += int64(bytesRead)

				// Calculate progress
				if totalSize > 0 && onProgress != nil {
					pct := int(float64(currentSize) / float64(totalSize) * 100.0)
					if pct > lastPct {
						if pct > 100 {
							pct = 100
						}
						lastPct = pct
						onProgress(float64(pct))
					}
				}
			}

			if readErr != nil {
				break
			}
		}

		out.Close()
		resp.Body.Close()

		// If EOF is reached, the download completed successfully
		if readErr == io.EOF {
			return nil
		}

		// Connection aborted or read timeout; retry from currentSize
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("download aborted after %d failed attempts", maxRetries)
}

var downloadPoolOnce sync.Once
var globalDownloadPool *DownloadPool

// MaxDownloadWorkers sets the maximum concurrent downloads allowed (default: 4)
var MaxDownloadWorkers = 4

// initDownloadPool helper to initialize the global concurrent download pool lazily
func initDownloadPool() {
	downloadPoolOnce.Do(func() {
		globalDownloadPool = &DownloadPool{}
		globalDownloadPool.start(MaxDownloadWorkers)
	})
}

// InitDownloadPool allows external configuration of the download pool
func (b *Bot) InitDownloadPool(workers ...int) {
	if len(workers) > 0 && workers[0] > 0 {
		MaxDownloadWorkers = workers[0]
	}
	initDownloadPool()
}

// CancelDownload cancels an active download task for a specific Chat ID globally
func (b *Bot) CancelDownload(chatID any) bool {
	initDownloadPool()
	resolved := b.ResolveChatID(chatID)
	resolvedStr := fmt.Sprintf("%v", resolved)
	if cancelVal, ok := globalDownloadPool.active.Load(resolvedStr); ok {
		if cancel, okFunc := cancelVal.(context.CancelFunc); okFunc {
			cancel() // Cancel the context to abort download loop instantly
			return true
		}
	}
	return false
}

// UploadJob encapsulates task state for background upload pipeline
type UploadJob struct {
	sendChain  *SendChain
	resultChan chan *UploadResult
}

// UploadResult holds the outcome of a queued upload task
type UploadResult struct {
	Msg *Message
	Err error
}

// UploadPool manages bounded concurrent uploads using background workers and cancellation maps
type UploadPool struct {
	jobChan chan *UploadJob
	workers int
	once    sync.Once
	active  sync.Map
}

// start spawns concurrent upload workers with context cancellation wrappers
func (up *UploadPool) start(workers int) {
	up.once.Do(func() {
		up.jobChan = make(chan *UploadJob, 1000)
		up.workers = workers
		for i := 0; i < workers; i++ {
			go func() {
				for job := range up.jobChan {
					// Create a cancellable context for each queued upload job
					jobCtx, cancel := context.WithCancel(job.sendChain.ctx)
					resolved := job.sendChain.bot.ResolveChatID(job.sendChain.chat)
					chatIDStr := fmt.Sprintf("%v", resolved)

					up.active.Store(chatIDStr, cancel)

					// Execute the upload operation safely
					msg, err := job.sendChain.executeUpload(jobCtx)

					up.active.Delete(chatIDStr)
					cancel()

					job.resultChan <- &UploadResult{Msg: msg, Err: err}
				}
			}()
		}
	})
}

var uploadPoolOnce sync.Once
var globalUploadPool *UploadPool

// MaxUploadWorkers sets the maximum concurrent uploads allowed (default: 4)
var MaxUploadWorkers = 4

// initUploadPool helper to initialize the global concurrent upload pool lazily
func initUploadPool() {
	uploadPoolOnce.Do(func() {
		globalUploadPool = &UploadPool{}
		globalUploadPool.start(MaxUploadWorkers)
	})
}

// InitUploadPool allows external configuration of the upload pool
func (b *Bot) InitUploadPool(workers ...int) {
	if len(workers) > 0 && workers[0] > 0 {
		MaxUploadWorkers = workers[0]
	}
	initUploadPool()
}

// CancelUpload cancels an active upload task for a specific Chat ID globally
func CancelUpload(b *Bot, chatID any) bool {
	initUploadPool()
	resolved := b.ResolveChatID(chatID)
	resolvedStr := fmt.Sprintf("%v", resolved)
	if cancelVal, ok := globalUploadPool.active.Load(resolvedStr); ok {
		if cancel, okFunc := cancelVal.(context.CancelFunc); okFunc {
			// Cancel the context to abort upload loop instantly
			cancel()
			return true
		}
	}
	return false
}

// BaseRequestMultipartWithProgress executes resilient multipart uploads with true network progress tracking and multi-attempt retries
func (c *Client) BaseRequestMultipartWithProgress(ctx context.Context, method string, params any, files []InputFile, onProgress func(pct float64), result any) error {
	if ctx == nil {
		ctx = context.Background()
	}

	const maxRetries = 5
	var reqErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Re-seek and reset file readers on each retry attempt
		for _, f := range files {
			if seeker, ok := f.Reader.(io.ReadSeeker); ok {
				_, _ = seeker.Seek(0, io.SeekStart)
			}
		}

		buf := new(bytes.Buffer)
		writer := multipart.NewWriter(buf)

		// Serialize parameters into multipart fields
		if params != nil {
			v := reflect.ValueOf(params)
			if v.Kind() == reflect.Ptr {
				v = v.Elem()
			}
			if v.Kind() == reflect.Struct {
				t := v.Type()
				for i := 0; i < v.NumField(); i++ {
					f := v.Field(i)
					sf := t.Field(i)
					tag := sf.Tag.Get("json")
					if tag == "" || tag == "-" {
						continue
					}
					name := strings.Split(tag, ",")[0]
					if f.Kind() == reflect.String {
						_ = writer.WriteField(name, f.String())
					} else {
						jsonVal, err := json.Marshal(f.Interface())
						if err == nil {
							_ = writer.WriteField(name, string(jsonVal))
						}
					}
				}
			} else if v.Kind() == reflect.Map {
				for _, key := range v.MapKeys() {
					val := v.MapIndex(key)
					if !val.IsValid() {
						continue
					}
					name := fmt.Sprintf("%v", key.Interface())
					elem := val.Interface()
					if elem == nil {
						continue
					}
					switch typedVal := elem.(type) {
					case string:
						_ = writer.WriteField(name, typedVal)
					case int64, int, int32:
						_ = writer.WriteField(name, fmt.Sprintf("%d", typedVal))
					case bool:
						_ = writer.WriteField(name, fmt.Sprintf("%t", typedVal))
					default:
						jsonVal, err := json.Marshal(typedVal)
						if err == nil {
							_ = writer.WriteField(name, string(jsonVal))
						}
					}
				}
			}
		}

		// Write file payloads into multipart fields
		for _, f := range files {
			part, err := writer.CreateFormFile(f.Field, f.FileName)
			if err != nil {
				return err
			}
			_, _ = io.Copy(part, f.Reader)
		}
		_ = writer.Close()

		// Wrap request body in a progressReader to track actual byte transfer on the socket
		bodyReader := bytes.NewReader(buf.Bytes())
		var requestBody io.Reader = bodyReader
		if onProgress != nil {
			requestBody = &progressReader{
				r:          bodyReader,
				total:      int64(buf.Len()),
				onProgress: onProgress,
			}
		}

		url := fmt.Sprintf("%s%s/%s", c.baseURL, c.token, method)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, requestBody)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())

		resp, err := c.httpClient.Do(req)
		if err != nil {
			reqErr = err
			// Delay before next retry
			time.Sleep(2 * time.Second)
			continue
		}

		// Read response bytes
		respBytes, errRead := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if errRead != nil {
			reqErr = errRead
			time.Sleep(2 * time.Second)
			continue
		}

		var apiResp Res
		if err := json.Unmarshal(respBytes, &apiResp); err != nil {
			reqErr = err
			time.Sleep(2 * time.Second)
			continue
		}
		if !apiResp.OK {
			return fmt.Errorf("api error [%d]: %s", apiResp.Code, apiResp.Desc)
		}

		if result != nil && apiResp.Result != nil {
			return json.Unmarshal(apiResp.Result, result)
		}
		return nil
	}

	return fmt.Errorf("upload failed after %d attempts: %w", maxRetries, reqErr)
}

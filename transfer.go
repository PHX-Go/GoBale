package gobale

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"mime/multipart"
	"net/http"
	neturl "net/url"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Dedicated HTTP client for large transfers.
var transferHTTPClient = &http.Client{
	Transport: &http.Transport{
		TLSNextProto:          make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		// Optimize buffer sizes to prevent connection resets on unstable networks
		ReadBufferSize:  512 * 1024, // 512 KB
		WriteBufferSize: 512 * 1024, // 512 KB
	},
}

// StallTimeout is the maximum time to wait without receiving any new bytes
// before an in-progress attempt is aborted and retried (issue #2).
var StallTimeout = 60 * time.Second

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
	} else if pr.total <= 0 && pr.onProgress != nil && n > 0 {
		mbs := float64(pr.read) / (1024 * 1024)
		pr.onProgress(-mbs)
	}
	return n, err
}

// backoffWithJitter returns an exponential backoff duration (base 500ms, capped
// at 10s) with up to 250ms of random jitter added, to avoid thundering-herd
// retries against the server.
func backoffWithJitter(attempt int) time.Duration {
	base := 500 * time.Millisecond
	max := 10 * time.Second
	d := base << uint(attempt)
	if d <= 0 || d > max {
		d = max
	}
	jitter := time.Duration(rand.Int63n(int64(250 * time.Millisecond)))
	return d + jitter
}

// isRetryableStatus reports whether an HTTP status code should be retried.
// 4xx client errors (other than 429) indicate a request that will never
// succeed no matter how many times it's retried, so we fail fast on those
func isRetryableStatus(code int) bool {
	if code == http.StatusTooManyRequests {
		return true
	}
	if code >= 500 {
		return true
	}
	return false
}

// CANCEL REGISTRY (shared by download & upload pools)

// cancelRegistry tracks active cancel functions per key, allowing multiple
// concurrent jobs for the same key (e.g. the same chat ID) without one job's
// cancellation overwriting another's.
type cancelRegistry struct {
	mu sync.Mutex
	m  map[string][]context.CancelFunc
}

func newCancelRegistry() *cancelRegistry {
	return &cancelRegistry{m: make(map[string][]context.CancelFunc)}
}

// add registers a cancel func under key and returns a remove func that must
// be called when the job finishes, so the registry doesn't leak entries.
func (cr *cancelRegistry) add(key string, cancel context.CancelFunc) func() {
	cr.mu.Lock()
	cr.m[key] = append(cr.m[key], cancel)
	idx := len(cr.m[key]) - 1
	cr.mu.Unlock()

	return func() {
		cr.mu.Lock()
		defer cr.mu.Unlock()
		list := cr.m[key]
		if idx < len(list) {
			list[idx] = nil // mark as removed without shifting other indices
		}
		allNil := true
		for _, c := range list {
			if c != nil {
				allNil = false
				break
			}
		}
		if allNil {
			delete(cr.m, key)
		}
	}
}

// cancelAll cancels every active job registered under key. Returns true if
// at least one job was cancelled.
func (cr *cancelRegistry) cancelAll(key string) bool {
	cr.mu.Lock()
	list := cr.m[key]
	cr.mu.Unlock()

	cancelled := false
	for _, c := range list {
		if c != nil {
			c()
			cancelled = true
		}
	}
	return cancelled
}

// CONCURRENT DOWNLOAD POOL

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
	active  *cancelRegistry
}

// start spawns concurrent download workers with context cancellation wrappers
func (dp *DownloadPool) start(workers int) {
	dp.once.Do(func() {
		dp.jobChan = make(chan *DownloadJob, 1000)
		dp.workers = workers
		dp.active = newCancelRegistry()
		for i := 0; i < workers; i++ {
			go func() {
				for job := range dp.jobChan {
					// Create a cancellable context for each queued job
					jobCtx, cancel := context.WithCancel(job.ctx)

					var remove func()
					if job.chatID != "" {
						remove = dp.active.add(job.chatID, cancel)
					}

					// Honor a per-job client override if one was supplied,
					// otherwise fall back to the shared transferHTTPClient.
					client := job.client
					if client == nil {
						client = transferHTTPClient
					}

					err := resilientDownload(jobCtx, client, job.url, job.destPath, job.totalSize, job.onProgress)

					if remove != nil {
						remove()
					}
					cancel() // Clean up context resources

					job.resultChan <- err
				}
			}()
		}
	})
}

// resilientDownload handles range-resuming, multi-attempt retries, browser-mimicking
// headers, unknown sizes, stall detection and post-download size verification.
// It also resolves indirect links (share pages, landing pages with a
// "Download" button, etc.) into a direct file URL before downloading.
func resilientDownload(ctx context.Context, client *http.Client, urlStr, destPath string, expectedSize int64, onProgress func(percent float64)) error {
	const maxRetries = 5
	var currentSize int64 = 0
	var lastPct = -1
	var lastErr error
	var alreadyResolved bool

	if resolved, err := ResolveDownloadURL(ctx, client, urlStr); err == nil && resolved.URL != "" {
		urlStr = resolved.URL
		alreadyResolved = true
	}
	// If resolution errored, we just fall through and try urlStr as-is —
	// it may already be a direct link that no resolver needed to touch.

	if stat, err := os.Stat(destPath); err == nil {
		currentSize = stat.Size()
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Per-attempt context that can be cancelled either by the caller (ctx)
		// or by the stall watchdog below.
		attemptCtx, attemptCancel := context.WithCancel(ctx)

		req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, urlStr, nil)
		if err != nil {
			attemptCancel()
			return err
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

		if parsedURL, err := neturl.Parse(urlStr); err == nil {
			req.Header.Set("Referer", fmt.Sprintf("%s://%s/", parsedURL.Scheme, parsedURL.Host))
		}

		if currentSize > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", currentSize))
		}

		resp, err := client.Do(req)
		if err != nil {
			attemptCancel()
			lastErr = err
			if ctx.Err() != nil {
				return ctx.Err()
			}
			time.Sleep(backoffWithJitter(attempt))
			continue
		}

		// Fail fast on non-retryable client errors instead of burning all retries.
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			resp.Body.Close()
			attemptCancel()
			if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
				return nil
			}
			if !isRetryableStatus(resp.StatusCode) {
				return fmt.Errorf("unexpected http status code: %d (%s)", resp.StatusCode, resp.Status)
			}
			lastErr = fmt.Errorf("unexpected http status code: %d (%s)", resp.StatusCode, resp.Status)
			time.Sleep(backoffWithJitter(attempt))
			continue
		}

		// The URL looked direct (or resolution said so) but actually served a
		// landing page instead of a file — try resolving it for real before
		// burning through retries on something that will never be a file.
		if ct := resp.Header.Get("Content-Type"); strings.Contains(strings.ToLower(ct), "text/html") {
			resp.Body.Close()
			attemptCancel()

			if !alreadyResolved {
				if resolved, rerr := ResolveDownloadURL(ctx, client, urlStr); rerr == nil && resolved.URL != "" && resolved.URL != urlStr {
					urlStr = resolved.URL
					alreadyResolved = true
					continue
				}
			}
			lastErr = fmt.Errorf("server returned an html page instead of a file (link may require manual resolution)")
			time.Sleep(backoffWithJitter(attempt))
			continue
		}

		isResume := resp.StatusCode == http.StatusPartialContent
		if resp.StatusCode == http.StatusOK {
			currentSize = 0
			isResume = false
		}

		var out *os.File
		if isResume {
			out, err = os.OpenFile(destPath, os.O_WRONLY|os.O_APPEND, 0600)
		} else {
			out, err = os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		}
		if err != nil {
			resp.Body.Close()
			attemptCancel()
			return err
		}

		totalSize := resp.ContentLength
		if isResume {
			totalSize += currentSize
		}
		if totalSize <= 0 {
			totalSize = expectedSize
		}

		// Stall watchdog: aborts the attempt if no bytes arrive for StallTimeout,
		// without capping the total duration of a healthy, slow transfer.
		watchdog := time.AfterFunc(StallTimeout, attemptCancel)

		var buffer = make([]byte, 32*1024)
		var readErr error
		var bytesRead int

		for {
			if attemptCtx.Err() != nil {
				watchdog.Stop()
				out.Close()
				resp.Body.Close()
				if ctx.Err() != nil {
					return ctx.Err()
				}
				readErr = attemptCtx.Err()
				break
			}

			bytesRead, readErr = resp.Body.Read(buffer)
			if bytesRead > 0 {
				watchdog.Reset(StallTimeout)

				_, writeErr := out.Write(buffer[:bytesRead])
				if writeErr != nil {
					watchdog.Stop()
					out.Close()
					resp.Body.Close()
					attemptCancel()
					return writeErr
				}
				currentSize += int64(bytesRead)

				if onProgress != nil {
					if totalSize > 0 {
						pct := int(float64(currentSize) / float64(totalSize) * 100.0)
						if pct > lastPct {
							if pct > 100 {
								pct = 100
							}
							lastPct = pct
							onProgress(float64(pct))
						}
					} else {
						mbs := float64(currentSize) / (1024 * 1024)
						onProgress(-mbs)
					}
				}
			}

			if readErr != nil {
				break
			}
		}

		watchdog.Stop()
		out.Close()
		resp.Body.Close()
		attemptCancel()

		if readErr == io.EOF {
			// Verify the file we ended up with actually matches the
			// size the server told us to expect, instead of trusting a clean
			// EOF blindly (a server can close early and still look "clean").
			if totalSize > 0 && currentSize != totalSize {
				lastErr = fmt.Errorf("incomplete download: got %d bytes, expected %d", currentSize, totalSize)
				time.Sleep(backoffWithJitter(attempt))
				continue
			}
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		lastErr = readErr
		time.Sleep(backoffWithJitter(attempt))
	}

	return fmt.Errorf("download aborted after %d failed attempts: %w", maxRetries, lastErr)
}

var downloadPoolOnce sync.Once
var globalDownloadPool *DownloadPool

// MaxDownloadWorkers sets the maximum concurrent downloads allowed (default: 4).
// Because pool startup is guarded by sync.Once, changing this
// value after the pool has already been started has no effect. Call
// (*Bot).InitDownloadPool with the desired worker count before the pool is
// used for the first time (e.g. at startup), not to "resize" it later.
var MaxDownloadWorkers = 4

// initDownloadPool helper to initialize the global concurrent download pool lazily
func initDownloadPool() {
	downloadPoolOnce.Do(func() {
		globalDownloadPool = &DownloadPool{}
		globalDownloadPool.start(MaxDownloadWorkers)
	})
}

// InitDownloadPool allows external configuration of the download pool.
// Must be called before the pool processes its first job to take effect (see
// MaxDownloadWorkers doc).
func (b *Bot) InitDownloadPool(workers ...int) {
	if len(workers) > 0 && workers[0] > 0 {
		MaxDownloadWorkers = workers[0]
	}
	initDownloadPool()
}

// CancelDownload cancels all active download tasks for a specific Chat ID globally.
func (b *Bot) CancelDownload(chatID any) bool {
	initDownloadPool()
	resolved := b.ResolveChatID(chatID)
	resolvedStr := fmt.Sprintf("%v", resolved)
	return globalDownloadPool.active.cancelAll(resolvedStr)
}

// CONCURRENT UPLOAD POOL

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
	active  *cancelRegistry
}

// start spawns concurrent upload workers with context cancellation wrappers
func (up *UploadPool) start(workers int) {
	up.once.Do(func() {
		up.jobChan = make(chan *UploadJob, 1000)
		up.workers = workers
		up.active = newCancelRegistry()
		for i := 0; i < workers; i++ {
			go func() {
				for job := range up.jobChan {
					jobCtx, cancel := context.WithCancel(job.sendChain.ctx)
					resolved := job.sendChain.bot.ResolveChatID(job.sendChain.chat)
					chatIDStr := fmt.Sprintf("%v", resolved)

					remove := up.active.add(chatIDStr, cancel)

					msg, err := job.sendChain.executeUpload(jobCtx)

					remove()
					cancel()

					job.resultChan <- &UploadResult{Msg: msg, Err: err}
				}
			}()
		}
	})
}

var uploadPoolOnce sync.Once
var globalUploadPool *UploadPool

// MaxUploadWorkers sets the maximum concurrent uploads allowed (default: 4).
// See MaxDownloadWorkers doc: this must be set before first use to take effect.
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

// CancelUpload cancels all active upload tasks for a specific Chat ID globally.
func CancelUpload(b *Bot, chatID any) bool {
	initUploadPool()
	resolved := b.ResolveChatID(chatID)
	resolvedStr := fmt.Sprintf("%v", resolved)
	return globalUploadPool.active.cancelAll(resolvedStr)
}

// CLIENT PROGRESS UPLOADER

// writeParams serializes params (struct or map) into multipart fields.
// Struct fields tagged with `,omitempty` are now skipped when they
// hold a zero value (nil pointer, empty string, zero number, false, empty
// slice/map), matching encoding/json semantics instead of always emitting a
// field (which previously could send literal "null" for nil pointer fields).
func writeParams(writer *multipart.Writer, params any) {
	if params == nil {
		return
	}
	v := reflect.ValueOf(params)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			sf := t.Field(i)
			tag := sf.Tag.Get("json")
			if tag == "" || tag == "-" {
				continue
			}
			parts := strings.Split(tag, ",")
			name := parts[0]
			omitEmpty := false
			for _, p := range parts[1:] {
				if p == "omitempty" {
					omitEmpty = true
				}
			}
			if omitEmpty && isZeroValue(f) {
				continue
			}

			if f.Kind() == reflect.String {
				_ = writer.WriteField(name, f.String())
				continue
			}
			jsonVal, err := json.Marshal(f.Interface())
			if err == nil {
				_ = writer.WriteField(name, string(jsonVal))
			}
		}
	case reflect.Map:
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

func isZeroValue(f reflect.Value) bool {
	switch f.Kind() {
	case reflect.Ptr, reflect.Interface:
		return f.IsNil()
	case reflect.Slice, reflect.Map:
		return f.Len() == 0
	default:
		return f.IsZero()
	}
}

// BaseRequestMultipartWithProgress executes resilient multipart uploads with true
// network progress tracking and multi-attempt retries. The multipart body is now
// streamed through an io.Pipe directly into the HTTP request instead of being
// fully buffered in memory first, which matters a lot for large file
// uploads. Retries are also now skipped for non-retryable 4xx responses and use
// exponential backoff with jitter.
func (c *Client) BaseRequestMultipartWithProgress(ctx context.Context, method string, params any, files []InputFile, onProgress func(pct float64), result any) error {
	if ctx == nil {
		ctx = context.Background()
	}

	const maxRetries = 5
	var lastErr error

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

		// Estimate total upload size (best-effort) for percentage progress.
		// Field overhead is ignored; this is an approximation, not exact bytes.
		var estimatedTotal int64
		knownSize := true
		for _, f := range files {
			if seeker, ok := f.Reader.(io.ReadSeeker); ok {
				cur, err := seeker.Seek(0, io.SeekCurrent)
				if err != nil {
					knownSize = false
					break
				}
				end, err := seeker.Seek(0, io.SeekEnd)
				if err != nil {
					knownSize = false
					break
				}
				_, _ = seeker.Seek(cur, io.SeekStart)
				estimatedTotal += end - cur
			} else {
				knownSize = false
				break
			}
		}
		if !knownSize {
			estimatedTotal = 0
		}

		pr, pw := io.Pipe()
		writer := multipart.NewWriter(pw)

		go func() {
			var werr error
			defer func() {
				if werr != nil {
					_ = pw.CloseWithError(werr)
				} else {
					_ = pw.Close()
				}
			}()

			writeParams(writer, params)

			for _, f := range files {
				part, err := writer.CreateFormFile(f.Field, f.FileName)
				if err != nil {
					werr = err
					return
				}
				if _, err := io.Copy(part, f.Reader); err != nil {
					werr = err
					return
				}
			}
			werr = writer.Close()
		}()

		var requestBody io.Reader = pr
		if onProgress != nil {
			requestBody = &progressReader{
				r:          pr,
				total:      estimatedTotal,
				onProgress: onProgress,
			}
		}

		url := fmt.Sprintf("%s%s/%s", c.baseURL, c.token, method)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, requestBody)
		if err != nil {
			_ = pr.CloseWithError(err)
			return err
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())

		resp, err := transferHTTPClient.Do(req)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return ctx.Err()
			}
			time.Sleep(backoffWithJitter(attempt))
			continue
		}

		respBytes, errRead := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if errRead != nil {
			lastErr = errRead
			time.Sleep(backoffWithJitter(attempt))
			continue
		}

		// Fail fast on non-retryable client errors.
		if resp.StatusCode != http.StatusOK && !isRetryableStatus(resp.StatusCode) {
			return fmt.Errorf("unexpected http status code: %d (%s)", resp.StatusCode, resp.Status)
		}
		if resp.StatusCode != http.StatusOK && isRetryableStatus(resp.StatusCode) {
			lastErr = fmt.Errorf("unexpected http status code: %d (%s)", resp.StatusCode, resp.Status)
			time.Sleep(backoffWithJitter(attempt))
			continue
		}

		var apiResp Res
		if err := json.Unmarshal(respBytes, &apiResp); err != nil {
			lastErr = err
			time.Sleep(backoffWithJitter(attempt))
			continue
		}
		if !apiResp.OK {
			// API-level error: not a transport failure, don't retry blindly.
			return fmt.Errorf("api error [%d]: %s", apiResp.Code, apiResp.Desc)
		}

		if result != nil && apiResp.Result != nil {
			return json.Unmarshal(apiResp.Result, result)
		}
		return nil
	}

	return fmt.Errorf("upload failed after %d attempts: %w", maxRetries, lastErr)
}

// INDIRECT LINK RESOLUTION

// ResolvedLink is the outcome of trying to turn an indirect page URL (a
// landing page, a "click here to download" page, a redirect page, ...) into
// a direct, fetchable file URL.
type ResolvedLink struct {
	URL  string // Direct, fetchable file URL
	Size int64  // Best-effort size in bytes, -1 if unknown
}

// Tunables for the generic resolver.
var (
	// MaxResolveHops caps how many "this page points to another page" jumps
	// we're willing to follow before giving up, so a chain of landing pages
	// (or a loop) can't stall a download forever.
	MaxResolveHops = 3
	// maxResolveBodyBytes caps how much of a candidate HTML page we read
	// while looking for the real link, so we never accidentally "download"
	// someone's huge page just to scan it.
	maxResolveBodyBytes int64 = 2 * 1024 * 1024 // 2 MB
)

// ResolveDownloadURL is a generic, non-site-specific resolver used before
// (and, if needed, during) a download. It works like this:
//
//  1. Fetch rawURL and look at Content-Type.
//  2. If it's not HTML, it's already a direct file link — done.
//  3. If it is HTML, scan the page (regex-based, no site knowledge) for the
//     most likely "real" link using a few common patterns, in priority
//     order: meta-refresh redirect -> JS location redirect -> an anchor
//     whose text says "download"/"دانلود" -> any href pointing at a known
//     file extension.
//  4. Repeat against the newly found link, up to MaxResolveHops times, in
//     case that "real" link is itself another landing page.
//
// If nothing usable is found, it returns the last URL it had (which may
// still be the original rawURL) so the caller can just try downloading it as
// a last resort.
func ResolveDownloadURL(ctx context.Context, client *http.Client, rawURL string) (*ResolvedLink, error) {
	if client == nil {
		client = transferHTTPClient
	}

	current := rawURL
	visited := map[string]bool{}

	for hop := 0; hop < MaxResolveHops; hop++ {
		if visited[current] {
			// Looped back to a page we already scanned — bail out with
			// whatever we currently have rather than spinning forever.
			break
		}
		visited[current] = true

		html, contentLength, isHTML, err := peekPage(ctx, client, current)
		if err != nil {
			// Couldn't fetch it for inspection; let the caller try it
			// directly — the real download attempt will surface the real error.
			return &ResolvedLink{URL: current, Size: -1}, err
		}
		if !isHTML {
			return &ResolvedLink{URL: current, Size: contentLength}, nil
		}

		next, found := extractLikelyDownloadLink(html, current)
		if !found || next == current {
			// It's HTML but nothing better was found inside it. Return it
			// as-is; the download loop detects the HTML response itself and
			// surfaces a clear error.
			return &ResolvedLink{URL: current, Size: -1}, nil
		}

		current = next
	}

	return &ResolvedLink{URL: current, Size: -1}, nil
}

// peekPage fetches url and reports whether the response is HTML. For HTML
// responses it also returns a size-bounded copy of the body for scanning;
// for anything else it just returns the Content-Length so the caller doesn't
// need to fetch it again.
func peekPage(ctx context.Context, client *http.Client, url string) (html string, contentLength int64, isHTML bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", -1, false, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", -1, false, err
	}
	defer resp.Body.Close()

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.Contains(ct, "text/html") {
		return "", resp.ContentLength, false, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResolveBodyBytes))
	if err != nil {
		return "", -1, true, err
	}
	return string(body), -1, true, nil
}

// Regex patterns for the handful of common "the real link is over here"
// signals found on landing/redirect pages. None of this is site-specific.
var (
	resolveMetaRefreshRe = regexp.MustCompile(`(?i)<meta[^>]+http-equiv=["']?refresh["']?[^>]+content=["'][^;]+;\s*url=([^"'>]+)["']`)

	resolveJSLocationRe = regexp.MustCompile(`(?i)(?:window\.location(?:\.href)?|location\.href)\s*=\s*["']([^"']+)["']`)
	resolveJSReplaceRe  = regexp.MustCompile(`(?i)location\.replace\(\s*["']([^"']+)["']\s*\)`)

	// An <a> tag whose visible text mentions "download"/"دانلود" somewhere.
	resolveDownloadKeywordAnchorRe = regexp.MustCompile(`(?is)<a\s[^>]*href=["']([^"']+)["'][^>]*>(?:(?!</a>).)*?(?:download|دانلود)(?:(?!</a>).)*?</a>`)

	// Any href pointing straight at a file with a common download-able
	// extension — last-resort signal.
	resolveFileExtHrefRe = regexp.MustCompile(`(?i)href=["']([^"']+\.(?:zip|rar|7z|tar|gz|exe|msi|apk|dmg|iso|mp4|mkv|avi|mp3|flac|pdf|docx?|xlsx?|pptx?))(?:\?[^"']*)?["']`)
)

// extractLikelyDownloadLink scans html (from pageURL) for the first pattern
// that matches, in priority order, and resolves it to an absolute URL.
func extractLikelyDownloadLink(html, pageURL string) (string, bool) {
	patterns := []*regexp.Regexp{
		resolveMetaRefreshRe,
		resolveJSReplaceRe,
		resolveJSLocationRe,
		resolveDownloadKeywordAnchorRe,
		resolveFileExtHrefRe,
	}
	for _, re := range patterns {
		if m := re.FindStringSubmatch(html); len(m) > 1 {
			return resolveRelative(pageURL, m[1]), true
		}
	}
	return "", false
}

// resolveRelative resolves href against the page it was scraped from, so
// relative or absolute-path links turn into a full, fetchable URL.
func resolveRelative(pageURL, href string) string {
	href = strings.TrimSpace(strings.ReplaceAll(href, "&amp;", "&"))
	base, err := neturl.Parse(pageURL)
	if err != nil {
		return href
	}
	ref, err := neturl.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

// PROGRESS BAR

// BuildProgressBar generates a standard, visual text progress bar
// (e.g. "■■■■■□□□□□") for any percentage value from 0 to 100.
func BuildProgressBar(pct float64) string {
	const width = 10

	if math.IsNaN(pct) || math.IsInf(pct, 0) {
		pct = 0
	}

	filled := int(math.Round(pct / 10.0))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}

	var sb strings.Builder
	sb.Grow(width * len("■"))
	for i := 0; i < filled; i++ {
		sb.WriteString("■")
	}
	for i := filled; i < width; i++ {
		sb.WriteString("□")
	}
	return sb.String()
}

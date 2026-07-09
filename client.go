package gobale

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// State representing circuit breaker status
const (
	StateClosed uint32 = iota
	StateOpen
	StateHalfOpen
)

// CB protects the client from hitting unstable servers
type CB struct {
	state     uint32
	fails     uint32
	lastFail  int64
	threshold uint32
	cooldown  int64
	halfAllow int32
}

// NewCB creates a new circuit breaker
func NewCB(threshold uint32, cooldown time.Duration) *CB {
	return &CB{
		state:     StateClosed,
		threshold: threshold,
		cooldown:  int64(cooldown),
	}
}

// CanExecute checks if requests are allowed to pass
func (c *CB) CanExecute() bool {
	state := atomic.LoadUint32(&c.state)
	if state == StateClosed {
		return true
	}
	if state == StateOpen {
		last := atomic.LoadInt64(&c.lastFail)
		if time.Now().UnixNano()-last > c.cooldown {
			if atomic.CompareAndSwapUint32(&c.state, StateOpen, StateHalfOpen) {
				atomic.StoreInt32(&c.halfAllow, 1)
				return true
			}
		}
		return false
	}
	if state == StateHalfOpen {
		return atomic.CompareAndSwapInt32(&c.halfAllow, 1, 0)
	}
	return true
}

// RecordSuccess resets failure counters on success
func (c *CB) RecordSuccess() {
	state := atomic.LoadUint32(&c.state)
	switch state {
	case StateHalfOpen:
		atomic.StoreUint32(&c.fails, 0)
		atomic.StoreUint32(&c.state, StateClosed)
	case StateClosed:
		atomic.StoreUint32(&c.fails, 0)
	}
}

// RecordFailure increments failures and opens circuit if needed
func (c *CB) RecordFailure() {
	state := atomic.LoadUint32(&c.state)
	if state == StateHalfOpen {
		atomic.StoreUint32(&c.state, StateOpen)
		atomic.StoreInt64(&c.lastFail, time.Now().UnixNano())
		return
	}
	fails := atomic.AddUint32(&c.fails, 1)
	if fails >= c.threshold {
		atomic.StoreUint32(&c.state, StateOpen)
		atomic.StoreInt64(&c.lastFail, time.Now().UnixNano())
	}
}

// RL controls the request rate
type RL struct {
	rate       float64
	cap        float64
	tokens     float64
	lastRefill int64
	mu         sync.Mutex
}

// NewRL creates a token bucket rate limiter
func NewRL(rate int, interval time.Duration) *RL {
	return &RL{
		rate:       float64(rate) / float64(interval.Nanoseconds()),
		cap:        float64(rate),
		tokens:     float64(rate),
		lastRefill: time.Now().UnixNano(),
	}
}

// Wait blocks until a token becomes available with strict garbage collection on timer leaks
func (r *RL) Wait(ctx context.Context) error {
	r.mu.Lock()
	now := time.Now().UnixNano()
	var elapsed int64
	if now > r.lastRefill {
		elapsed = now - r.lastRefill
		r.lastRefill = now
	}
	r.tokens += float64(elapsed) * r.rate
	if r.tokens > r.cap {
		r.tokens = r.cap
	}
	if r.tokens >= 1.0 {
		r.tokens -= 1.0
		r.mu.Unlock()
		return nil
	}
	needed := 1.0 - r.tokens
	sleepNs := int64(needed / r.rate)
	r.lastRefill += sleepNs
	r.tokens = 0.0
	r.mu.Unlock()

	timer := time.NewTimer(time.Duration(sleepNs))
	defer timer.Stop()

	select {
	case <-ctx.Done():
		r.mu.Lock()
		r.lastRefill -= sleepNs
		r.mu.Unlock()
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// FC caches sent file ids locally
type FC struct {
	mu    sync.RWMutex
	store map[string]string
	path  string
}

// NewFC creates a file cache
func NewFC(path string) *FC {
	fc := &FC{
		store: make(map[string]string),
		path:  path,
	}
	_ = fc.load()
	return fc
}

// Store saves a file ID to cache thread-safely
func (f *FC) Store(k, v string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.store[k] = v
	_ = f.saveUnderLock()
}

// saveUnderLock writes cache storage to disk safely under write lock protection
func (f *FC) saveUnderLock() error {
	tmp := f.path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	err = gob.NewEncoder(file).Encode(f.store)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return err
	}
	_ = file.Sync()
	_ = file.Close()
	return os.Rename(tmp, f.path)
}

// Load retrieves a cached file ID
func (f *FC) Load(k string) (string, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	val, ok := f.store[k]
	return val, ok
}

// load reads cache storage from disk
func (f *FC) load() error {
	file, err := os.Open(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()
	return gob.NewDecoder(file).Decode(&f.store)
}

// Res represents raw API response layout
type Res struct {
	OK     bool                `json:"ok"`
	Result json.RawMessage     `json:"result,omitempty"`
	Code   int                 `json:"error_code,omitempty"`
	Desc   string              `json:"description,omitempty"`
	Params *ResponseParameters `json:"parameters,omitempty"`
}

type Client struct {
	token        string
	baseURL      string
	httpClient   *http.Client
	Logger       bool
	fileCache    *FC
	rateLimit    *RL
	cb           *CB
	Gzip         bool
	NetLatencyNs int64
	DryRun       bool
}

// NewClient creates a high-performance cloud-optimized API communication client
func NewClient(token string) *Client {
	clean := strings.TrimPrefix(token, "bot")
	clean = strings.Trim(clean, `"' `)

	// Advanced customized transport optimized for high-concurrency cloud environments
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second, // Max time to establish a new TCP socket
			KeepAlive: 90 * time.Second, // Interval between active keep-alive probes
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          200,              // Increased to 200 for microservices/cloud scaling
		MaxIdleConnsPerHost:   100,              // Max idle connections per single host (Bale API)
		IdleConnTimeout:       90 * time.Second, // Idle connection timeout to recycle sockets
		TLSHandshakeTimeout:   10 * time.Second, // Prevents hanging on TLS negotiation under pressure
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &Client{
		token:     clean,
		baseURL:   "https://tapi.bale.ai/bot",
		rateLimit: NewRL(30, time.Second),
		cb:        NewCB(5, 30*time.Second),
		fileCache: NewFC(DataPath("gobale_file_cache.db")),
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

// extractChatID reads a chat_id out of the outgoing params (struct field "ChatID"
// or map key "chat_id") so Dry-Run mocked responses reflect the real target chat
// instead of always returning a hardcoded value.
func extractChatID(params any) int64 {
	const fallback int64 = 111
	if params == nil {
		return fallback
	}
	v := reflect.ValueOf(params)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return fallback
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Struct:
		f := v.FieldByName("ChatID")
		if f.IsValid() {
			switch f.Kind() {
			case reflect.Int, reflect.Int32, reflect.Int64:
				return f.Int()
			case reflect.String:
				var id int64
				if _, err := fmt.Sscanf(f.String(), "%d", &id); err == nil {
					return id
				}
			}
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			if fmt.Sprintf("%v", key.Interface()) != "chat_id" {
				continue
			}
			val := v.MapIndex(key).Interface()
			switch tv := val.(type) {
			case int64:
				return tv
			case int:
				return int64(tv)
			case int32:
				return int64(tv)
			case string:
				var id int64
				if _, err := fmt.Sscanf(tv, "%d", &id); err == nil {
					return id
				}
			}
		}
	}
	return fallback
}

// BaseRequest sends a JSON request to the Bale Bot API with retry and circuit-breaker logic
func (c *Client) BaseRequest(ctx context.Context, method string, params any, result any) error {
	// Ensure context is never nil
	if ctx == nil {
		ctx = context.Background()
	}

	// Exit early if context is already canceled
	if err := ctx.Err(); err != nil {
		return err
	}

	// Check circuit breaker status
	if !c.cb.CanExecute() {
		return fmt.Errorf("circuit breaker is open, api is offline")
	}

	// Intercept outgoing requests in Dry-Run mode
	if c.DryRun && method != "getUpdates" && method != "getMe" {
		log.Printf("[Dry-Run Intercept] POST /%s | Params: %+v", method, params)
		mockLatency := time.Duration(2+rand.Intn(7)) * time.Millisecond
		atomic.StoreInt64(&c.NetLatencyNs, int64(mockLatency))

		if result != nil {
			if boolPtr, ok := result.(*bool); ok {
				*boolPtr = true
			} else if msgPtr, ok := result.(*Message); ok {
				msgPtr.MessageID = 999111
				msgPtr.Date = time.Now().Unix()
				msgPtr.Chat.ID = extractChatID(params)
			}
		}
		return nil
	}

	url := fmt.Sprintf("%s%s/%s", c.baseURL, c.token, method)
	buf := new(bytes.Buffer)
	if params != nil {
		if err := json.NewEncoder(buf).Encode(params); err != nil {
			return err
		}
	}
	body := buf.Bytes()

	const maxRetries429 = 5
	retries429 := 0

	for {
		// Wait for rate limiter token
		if err := c.rateLimit.Wait(ctx); err != nil {
			return err
		}

		var resp *http.Response
		var reqErr error
		start := time.Now()

		// Inner loop for transient network retries
		for attempt := 0; attempt < 3; attempt++ {
			// Stop immediately if context is canceled
			if err := ctx.Err(); err != nil {
				return err
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			if c.Gzip {
				req.Header.Set("Accept-Encoding", "gzip")
			}

			resp, reqErr = c.httpClient.Do(req)
			if reqErr != nil {
				// Don't retry or record failure if shutdown is in progress
				if ctx.Err() != nil {
					return reqErr
				}
				if attempt < 2 {
					time.Sleep(time.Duration(100*math.Pow(3, float64(attempt))) * time.Millisecond)
					continue
				}
				c.cb.RecordFailure()
				return reqErr
			}
			break
		}

		// Read and decompress response body
		respBytes, errRead := func() ([]byte, error) {
			defer func() {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}()
			var reader io.ReadCloser = resp.Body
			if c.Gzip && resp.Header.Get("Content-Encoding") == "gzip" {
				gzipReader, err := gzip.NewReader(resp.Body)
				if err != nil {
					return nil, err
				}
				defer gzipReader.Close()
				reader = gzipReader
			}
			return io.ReadAll(reader)
		}()

		if errRead != nil {
			if ctx.Err() == nil {
				c.cb.RecordFailure()
			}
			return errRead
		}

		// Update network latency metrics
		if method != "getUpdates" {
			atomic.StoreInt64(&c.NetLatencyNs, int64(time.Since(start)))
		}

		var apiResp Res
		if err := json.Unmarshal(respBytes, &apiResp); err != nil {
			if ctx.Err() == nil {
				c.cb.RecordFailure()
			}
			return fmt.Errorf("failed to parse JSON response: %w", err)
		}

		// Handle API level errors
		if !apiResp.OK {
			if apiResp.Code >= 500 && ctx.Err() == nil {
				c.cb.RecordFailure()
			}
			// Handle rate limiting specifically
			if apiResp.Code == 429 {
				retries429++
				if retries429 > maxRetries429 {
					return fmt.Errorf("api error [429]: rate limited after %d retries", maxRetries429)
				}
				wait := 5 * time.Second
				if apiResp.Params != nil && apiResp.Params.RetryAfter > 0 {
					wait = time.Duration(apiResp.Params.RetryAfter) * time.Second
				}
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
					timer.Stop()
					continue
				}
			}
			return fmt.Errorf("api error [%d]: %s", apiResp.Code, apiResp.Desc)
		}

		// Reset circuit breaker on success
		c.cb.RecordSuccess()
		if result != nil && apiResp.Result != nil {
			return json.Unmarshal(apiResp.Result, result)
		}
		return nil
	}
}

// BaseRequestMultipart sends a multipart/form-data request (file uploads) to
func (c *Client) BaseRequestMultipart(ctx context.Context, method string, params any, files []InputFile, result any) error {
	// Panic-Proof Shield: Ensure context is never nil
	if ctx == nil {
		ctx = context.Background()
	}

	if !c.cb.CanExecute() {
		return fmt.Errorf("circuit breaker is open, api is offline")
	}

	// Intercept and sandbox all outgoing multipart media uploads in Dry-Run mode
	if c.DryRun {
		var fileNames []string
		for _, f := range files {
			fileNames = append(fileNames, f.FileName)
		}
		log.Printf("[Dry-Run Intercept] MULTIPART /%s | Params: %+v | Files: %s", method, params, strings.Join(fileNames, ", "))

		mockLatency := time.Duration(2+rand.Intn(7)) * time.Millisecond
		atomic.StoreInt64(&c.NetLatencyNs, int64(mockLatency))

		if result != nil {
			if boolPtr, ok := result.(*bool); ok {
				*boolPtr = true
			} else if strPtr, ok := result.(*string); ok {
				*strPtr = "https://mock.bale.ai/invoice/link_123"
			} else if msgPtr, ok := result.(*Message); ok {
				msgPtr.MessageID = 999111
				msgPtr.Date = time.Now().Unix()
				msgPtr.Chat.ID = extractChatID(params)
				msgPtr.Chat.Type = "private"
			} else {
				mockBytes := []byte(`{"message_id": 999111, "chat": {"id": 111, "type": "private"}}`)
				_ = json.Unmarshal(mockBytes, result)
			}
		}
		return nil
	}

	url := fmt.Sprintf("%s%s/%s", c.baseURL, c.token, method)
	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)

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
				case int64:
					_ = writer.WriteField(name, fmt.Sprintf("%d", typedVal))
				case int:
					_ = writer.WriteField(name, fmt.Sprintf("%d", typedVal))
				case int32:
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

	for _, f := range files {
		part, err := writer.CreateFormFile(f.Field, f.FileName)
		if err != nil {
			return err
		}
		_, _ = io.Copy(part, f.Reader)
	}
	_ = writer.Close()

	// Body is fully serialized in memory already, safe to reuse bytes across retries
	body := buf.Bytes()
	contentType := writer.FormDataContentType()

	const maxRetries429 = 5
	retries429 := 0

	for {
		if err := c.rateLimit.Wait(ctx); err != nil {
			return err
		}

		var resp *http.Response
		var reqErr error
		start := time.Now()

		for attempt := 0; attempt < 3; attempt++ {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", contentType)

			resp, reqErr = c.httpClient.Do(req)
			if reqErr != nil {
				if attempt < 2 {
					time.Sleep(time.Duration(100*math.Pow(3, float64(attempt))) * time.Millisecond)
					continue
				}
				if ctx.Err() == nil {
					c.cb.RecordFailure()
				}
				return reqErr
			}
			break
		}

		// Read and close body immediately, safe to repeat across 429 retries
		respBytes, errRead := func() ([]byte, error) {
			defer func() {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}()
			return io.ReadAll(resp.Body)
		}()

		atomic.StoreInt64(&c.NetLatencyNs, int64(time.Since(start)))

		if errRead != nil {
			c.cb.RecordFailure()
			return errRead
		}

		var apiResp Res
		if err := json.Unmarshal(respBytes, &apiResp); err != nil {
			c.cb.RecordFailure()
			return fmt.Errorf("failed to parse Multipart JSON response (status %d): %w. Raw body: %s", resp.StatusCode, err, string(respBytes))
		}

		if !apiResp.OK {
			if apiResp.Code >= 500 {
				c.cb.RecordFailure()
			}
			if apiResp.Code == 429 {
				retries429++
				if retries429 > maxRetries429 {
					return fmt.Errorf("api error [429]: rate limited after %d retries", maxRetries429)
				}
				wait := 5 * time.Second
				if apiResp.Params != nil && apiResp.Params.RetryAfter > 0 {
					wait = time.Duration(apiResp.Params.RetryAfter) * time.Second
				}
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
					timer.Stop()
					continue
				}
			}
			return fmt.Errorf("api error [%d]: %s", apiResp.Code, apiResp.Desc)
		}

		c.cb.RecordSuccess()
		if result != nil && apiResp.Result != nil {
			return json.Unmarshal(apiResp.Result, result)
		}
		return nil
	}
}

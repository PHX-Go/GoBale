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
	"mime/multipart"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PHX-Go/GoBale/models"
)

type contextKey string

const (
	StateClosed uint32 = iota
	StateOpen
	StateHalfOpen
	loggerKey contextKey = "logger"
)

type CircuitBreaker struct {
	state           uint32
	failureCount    uint32
	lastFailureTime int64
	threshold       uint32
	cooldown        int64
	halfOpenAllowed int32
}

var bufferPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

type APIResponse struct {
	OK          bool                       `json:"ok"`
	Result      json.RawMessage            `json:"result,omitempty"`
	ErrorCode   int                        `json:"error_code,omitempty"`
	Description string                     `json:"description,omitempty"`
	Parameters  *models.ResponseParameters `json:"parameters,omitempty"`
}

type RateLimiter struct {
	rate       float64
	capacity   float64
	tokens     float64
	lastRefill int64
	mu         sync.Mutex
}

func NewCircuitBreaker(threshold uint32, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:     StateClosed,
		threshold: threshold,
		cooldown:  int64(cooldown),
	}
}

func (cb *CircuitBreaker) CanExecute() bool {
	state := atomic.LoadUint32(&cb.state)
	if state == StateClosed {
		return true
	}

	if state == StateOpen {
		lastFail := atomic.LoadInt64(&cb.lastFailureTime)
		if time.Now().UnixNano()-lastFail > cb.cooldown {
			if atomic.CompareAndSwapUint32(&cb.state, StateOpen, StateHalfOpen) {
				atomic.StoreInt32(&cb.halfOpenAllowed, 1)
				return true
			}
		}
		return false
	}

	if state == StateHalfOpen {
		return atomic.CompareAndSwapInt32(&cb.halfOpenAllowed, 1, 0)
	}

	return true
}

func (cb *CircuitBreaker) RecordSuccess() {
	state := atomic.LoadUint32(&cb.state)
	switch state {
	case StateHalfOpen:
		atomic.StoreUint32(&cb.failureCount, 0)
		atomic.StoreUint32(&cb.state, StateClosed)
	case StateClosed:
		atomic.StoreUint32(&cb.failureCount, 0)
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	state := atomic.LoadUint32(&cb.state)
	if state == StateHalfOpen {
		atomic.StoreUint32(&cb.state, StateOpen)
		atomic.StoreInt64(&cb.lastFailureTime, time.Now().UnixNano())
		return
	}

	fails := atomic.AddUint32(&cb.failureCount, 1)
	if fails >= cb.threshold {
		atomic.StoreUint32(&cb.state, StateOpen)
		atomic.StoreInt64(&cb.lastFailureTime, time.Now().UnixNano())
	}
}

func NewRateLimiter(rate int, interval time.Duration) *RateLimiter {
	return &RateLimiter{
		rate:       float64(rate) / float64(interval.Nanoseconds()),
		capacity:   float64(rate),
		tokens:     float64(rate),
		lastRefill: time.Now().UnixNano(),
	}
}

func (rl *RateLimiter) Wait(ctx context.Context) error {
	rl.mu.Lock()

	now := time.Now().UnixNano()

	var elapsed int64
	if now > rl.lastRefill {
		elapsed = now - rl.lastRefill
		rl.lastRefill = now
	} else {
		elapsed = 0
	}

	rl.tokens += float64(elapsed) * rl.rate
	if rl.tokens > rl.capacity {
		rl.tokens = rl.capacity
	}

	if rl.tokens >= 1.0 {
		rl.tokens -= 1.0
		rl.mu.Unlock()
		return nil
	}

	needed := 1.0 - rl.tokens
	sleepNs := int64(needed / rl.rate)

	rl.lastRefill = rl.lastRefill + sleepNs
	rl.tokens = 0.0
	rl.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(sleepNs)):
		return nil
	}
}

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
	Logger     bool
	fileCache  *FileCache // تبدیل sync.Map قدیمی به ساختار اتمیک FileCache جدید
	rateLimit  *RateLimiter
	cb         *CircuitBreaker
	Gzip       bool
}

type FileCache struct {
	mu       sync.RWMutex
	store    map[string]string
	filePath string
}

func NewClient(token string) *Client {
	cleanToken := strings.TrimPrefix(token, "bot")
	cleanToken = strings.TrimSpace(cleanToken)

	return &Client{
		token:     cleanToken,
		baseURL:   "https://tapi.bale.ai/bot",
		Logger:    false,
		rateLimit: NewRateLimiter(30, time.Second),
		cb:        NewCircuitBreaker(5, 30*time.Second),
		fileCache: NewFileCache("gobale_file_cache.db"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func stretchText(text string) string {
	if len([]rune(text)) < 30 {
		return text + "\n\u2800\u2800\u2800\u2800\u2800\u2800\u2800\u2800\u2800\u2800\u2800\u2800\u2800\u2800\u2800"
	}
	return text
}

func prettyJSON(raw []byte) string {
	var pretty bytes.Buffer
	err := json.Indent(&pretty, raw, " ", "  ")
	if err != nil {
		return string(raw)
	}
	return pretty.String()
}

func (c *Client) BaseRequest(ctx context.Context, method string, params any, result any) error {
	if !c.cb.CanExecute() {
		return fmt.Errorf("circuit breaker is open, bale api is currently unavailable")
	}

	if err := c.rateLimit.Wait(ctx); err != nil {
		return err
	}

	url := fmt.Sprintf("%s%s/%s", c.baseURL, c.token, method)

	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	if params != nil {
		if err := json.NewEncoder(buf).Encode(params); err != nil {
			return err
		}
	}

	isLogger := c.Logger
	if ctx != nil {
		if val, ok := ctx.Value(loggerKey).(bool); ok && val {
			isLogger = true
		}
	}

	bodyBytes := buf.Bytes()

	var resp *http.Response
	var respBytes []byte
	var reqErr error

	maxAttempts := 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if c.Gzip {
			req.Header.Set("Accept-Encoding", "gzip")
		}

		resp, reqErr = c.httpClient.Do(req)
		if reqErr != nil {
			if attempt < maxAttempts-1 {
				delay := backoffDelay(attempt)
				log.Printf("⚠️ [GoBale Network Alert] Connection failed: %v. Retrying in %v...", reqErr, delay)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
			c.cb.RecordFailure()
			return reqErr
		}

		var reader io.ReadCloser = resp.Body
		if c.Gzip && resp.Header.Get("Content-Encoding") == "gzip" {
			gzipReader, err := gzip.NewReader(resp.Body)
			if err != nil {
				_ = resp.Body.Close()
				if attempt < maxAttempts-1 {
					delay := backoffDelay(attempt)
					log.Printf("⚠️ [GoBale Network Alert] Gzip decompression failed. Retrying in %v...", delay)
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(delay):
					}
					continue
				}
				c.cb.RecordFailure()
				return err
			}
			reader = gzipReader
		}

		respBytes, reqErr = io.ReadAll(reader)
		_ = reader.Close()

		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}

		if reqErr != nil {
			if attempt < maxAttempts-1 {
				delay := backoffDelay(attempt)
				log.Printf("⚠️ [GoBale Network Alert] Failed to read HTTP response body. Retrying in %v...", delay)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
			c.cb.RecordFailure()
			return reqErr
		}

		resp.Body = io.NopCloser(bytes.NewReader(respBytes))

		var apiResp APIResponse
		if errJSON := json.NewDecoder(resp.Body).Decode(&apiResp); errJSON == nil {
			if !apiResp.OK && apiResp.ErrorCode >= 500 {
				if attempt < maxAttempts-1 {
					delay := backoffDelay(attempt)
					log.Printf("⚠️ [GoBale Network Alert] Bale API returned 5xx error (%d): %s. Retrying in %v...", apiResp.ErrorCode, apiResp.Description, delay)
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(delay):
					}
					continue
				}
			}
		}

		break
	}

	resp.Body = io.NopCloser(bytes.NewReader(respBytes))
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		c.cb.RecordFailure()
		return err
	}

	if isLogger {
		timeStr := time.Now().Format("15:04:05")
		status := "OK (200)"
		if !apiResp.OK {
			status = fmt.Sprintf("FAIL (%d)", apiResp.ErrorCode)
		}
		log.Printf("\n[GoBale] %s ↩️  RECV /%s | Status: %s\n %s", timeStr, method, status, prettyJSON(respBytes))
	}

	if !apiResp.OK {
		if apiResp.ErrorCode >= 500 {
			c.cb.RecordFailure()
		}

		if apiResp.ErrorCode == 429 {
			waitDuration := 5 * time.Second
			if apiResp.Parameters != nil && apiResp.Parameters.RetryAfter > 0 {
				waitDuration = time.Duration(apiResp.Parameters.RetryAfter) * time.Second
				log.Printf("[Rate Limit Warning] Hitting 429. Auto-retrying after %v...", waitDuration)
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitDuration):
				return c.BaseRequest(ctx, method, params, result)
			}
		}
		return fmt.Errorf("bale api error [%d]: %s", apiResp.ErrorCode, apiResp.Description)
	}

	c.cb.RecordSuccess()

	if result != nil && apiResp.Result != nil {
		if err := json.Unmarshal(apiResp.Result, result); err != nil {
			return err
		}
	}
	return nil
}

func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}

func (c *Client) BaseRequestMultipart(ctx context.Context, method string, params any, files []models.InputFile, result any) error {
	if !c.cb.CanExecute() {
		return fmt.Errorf("circuit breaker is open, bale api is currently unavailable")
	}

	if err := c.rateLimit.Wait(ctx); err != nil {
		return err
	}

	url := fmt.Sprintf("%s%s/%s", c.baseURL, c.token, method)

	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	writer := multipart.NewWriter(buf)

	if params != nil {
		v := reflect.ValueOf(params)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}

		if v.Kind() == reflect.Struct {
			t := v.Type()
			for i := 0; i < v.NumField(); i++ {
				field := v.Field(i)
				structField := t.Field(i)

				jsonTag := structField.Tag.Get("json")
				if jsonTag == "" || jsonTag == "-" {
					continue
				}

				tagParts := strings.Split(jsonTag, ",")
				fieldName := tagParts[0]

				hasOmitempty := len(tagParts) > 1 && tagParts[1] == "omitempty"
				if hasOmitempty && isZeroValue(field) {
					continue
				}

				if field.Kind() == reflect.String {
					_ = writer.WriteField(fieldName, field.String())
				} else if field.Kind() == reflect.Interface && field.Elem().Kind() == reflect.String {
					_ = writer.WriteField(fieldName, field.Elem().String())
				} else {
					jsonVal, err := json.Marshal(field.Interface())
					if err == nil {
						_ = writer.WriteField(fieldName, string(jsonVal))
					}
				}
			}
		}
	}

	for _, file := range files {
		part, err := writer.CreateFormFile(file.Field, file.FileName)
		if err != nil {
			return fmt.Errorf("failed to create form file part: %w", err)
		}
		_, err = io.Copy(part, file.Reader)
		if err != nil {
			return fmt.Errorf("failed to copy file bytes: %w", err)
		}
	}

	_ = writer.Close()

	isLogger := c.Logger
	if ctx != nil {
		if val, ok := ctx.Value(loggerKey).(bool); ok && val {
			isLogger = true
		}
	}

	bodyBytes := buf.Bytes()

	var resp *http.Response
	var respBytes []byte
	var reqErr error

	maxAttempts := 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		if c.Gzip {
			req.Header.Set("Accept-Encoding", "gzip")
		}

		if isLogger && attempt == 0 {
			timeStr := time.Now().Format("15:04:05")
			log.Printf("\n[GoBale] %s 📤  MULTIPART /%s | Uploading %d file(s)",
				timeStr, method, len(files))
		}

		resp, reqErr = c.httpClient.Do(req)
		if reqErr != nil {
			if attempt < maxAttempts-1 {
				delay := backoffDelay(attempt)
				log.Printf("⚠️ [GoBale Network Alert] Connection failed: %v. Retrying in %v...", reqErr, delay)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
			c.cb.RecordFailure()
			return reqErr
		}

		var reader io.ReadCloser = resp.Body
		if c.Gzip && resp.Header.Get("Content-Encoding") == "gzip" {
			gzipReader, err := gzip.NewReader(resp.Body)
			if err != nil {
				_ = resp.Body.Close()
				if attempt < maxAttempts-1 {
					delay := backoffDelay(attempt)
					log.Printf("⚠️ [GoBale Network Alert] Gzip decompression failed. Retrying in %v...", delay)
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(delay):
					}
					continue
				}
				c.cb.RecordFailure()
				return err
			}
			reader = gzipReader
		}

		respBytes, reqErr = io.ReadAll(reader)
		_ = reader.Close()

		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}

		if reqErr != nil {
			if attempt < maxAttempts-1 {
				delay := backoffDelay(attempt)
				log.Printf("⚠️ [GoBale Network Alert] Failed to read Multipart response body. Retrying in %v...", delay)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
			c.cb.RecordFailure()
			return reqErr
		}

		resp.Body = io.NopCloser(bytes.NewReader(respBytes))

		var apiResp APIResponse
		if errJSON := json.NewDecoder(resp.Body).Decode(&apiResp); errJSON == nil {
			if !apiResp.OK && apiResp.ErrorCode >= 500 {
				if attempt < maxAttempts-1 {
					delay := backoffDelay(attempt)
					log.Printf("⚠️ [GoBale Network Alert] Bale API returned 5xx error (%d) for Multipart: %s. Retrying in %v...", apiResp.ErrorCode, apiResp.Description, delay)
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(delay):
					}
					continue
				}
			}
		}

		break
	}

	resp.Body = io.NopCloser(bytes.NewReader(respBytes))
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		c.cb.RecordFailure()
		return err
	}

	if isLogger {
		timeStr := time.Now().Format("15:04:05")
		status := "OK (200)"
		if !apiResp.OK {
			status = fmt.Sprintf("FAIL (%d)", apiResp.ErrorCode)
		}
		log.Printf("\n[GoBale] %s ↩️  RECV /%s | Status: %s\n %s", timeStr, method, status, prettyJSON(respBytes))
	}

	if !apiResp.OK {
		if apiResp.ErrorCode >= 500 {
			c.cb.RecordFailure()
		}
		return fmt.Errorf("bale api error [%d]: %s", apiResp.ErrorCode, apiResp.Description)
	}

	c.cb.RecordSuccess()

	if result != nil && apiResp.Result != nil {
		if err := json.Unmarshal(apiResp.Result, result); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) SetRateLimit(rate int, interval time.Duration) {
	c.rateLimit = NewRateLimiter(rate, interval)
}

func NewFileCache(filePath string) *FileCache {
	fc := &FileCache{
		store:    make(map[string]string),
		filePath: filePath,
	}
	_ = fc.load()
	return fc
}

func (fc *FileCache) Store(key, value string) {
	fc.mu.Lock()
	fc.store[key] = value
	fc.mu.Unlock()
	_ = fc.save()
}

func (fc *FileCache) Load(key string) (any, bool) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	val, ok := fc.store[key]
	return val, ok
}

func (fc *FileCache) save() error {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	tmpPath := fc.filePath + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	err = gob.NewEncoder(file).Encode(fc.store)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return err
	}

	err = file.Sync()
	_ = file.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, fc.filePath)
}

func (fc *FileCache) load() error {
	file, err := os.Open(fc.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()
	return gob.NewDecoder(file).Decode(&fc.store)
}

func backoffDelay(attempt int) time.Duration {
	delay := time.Duration(100) * time.Millisecond * time.Duration(math.Pow(3, float64(attempt)))
	if delay > 5*time.Second {
		return 5 * time.Second
	}
	return delay
}

package gobale

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

// RotateWriter implements io.Writer with automatic file size-based rotation and backup limits
type RotateWriter struct {
	mu       sync.Mutex
	path     string
	file     *os.File
	maxSize  int64
	currSize int64
	backups  int
}

// NewRotateWriter instantiates a thread-safe rotating file stream writer
func NewRotateWriter(path string, maxSize int64, backups int) *RotateWriter {
	rw := &RotateWriter{
		path:    path,
		maxSize: maxSize,
		backups: backups,
	}
	rw.openAndSetSize()
	return rw
}

func (rw *RotateWriter) openAndSetSize() {
	if dir := filepath.Dir(rw.path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0755)
	}
	file, err := os.OpenFile(rw.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		rw.file = file
		if info, err := file.Stat(); err == nil {
			rw.currSize = info.Size()
		}
	}
}

func (rw *RotateWriter) rotate() {
	if rw.file != nil {
		_ = rw.file.Close()
		rw.file = nil
	}
	if rw.backups == 0 {
		_ = os.Remove(rw.path)
		rw.openAndSetSize()
		return
	}
	for i := rw.backups; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", rw.path, i)
		if i == rw.backups {
			_ = os.Remove(oldPath)
			continue
		}
		newPath := fmt.Sprintf("%s.%d", rw.path, i+1)
		_ = os.Rename(oldPath, newPath)
	}
	_ = os.Rename(rw.path, rw.path+".1")
	rw.openAndSetSize()
}

func (rw *RotateWriter) Write(p []byte) (n int, err error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if rw.file == nil {
		return 0, fmt.Errorf("file not open")
	}
	n, err = rw.file.Write(p)
	if err == nil {
		rw.currSize += int64(n)
		if rw.currSize >= rw.maxSize {
			rw.rotate()
		}
	}
	return n, err
}

// GoBaleLogger wraps slog.Logger with configurations for dynamic dual-output formats
type GoBaleLogger struct {
	slogLogger           *slog.Logger
	rotateW              *RotateWriter
	SuppressHTTP         bool
	SuppressGetUpdates   bool
	SuppressEmptyUpdates bool
}

// NewGoBaleLogger configures structured slog handler with optional shamsi ladder formatting
func NewGoBaleLogger(level slog.Level, path string, toConsole bool, jsonFormat bool, ladderShamsi bool) *GoBaleLogger {
	var handlers []slog.Handler
	var rotateW *RotateWriter

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	}

	// Configure structured file logging if path is provided
	if path != "" {
		rotateW = NewRotateWriter(path, 10*1024*1024, 3)
		var fileHandler slog.Handler
		if jsonFormat {
			fileHandler = slog.NewJSONHandler(rotateW, opts)
		} else {
			fileHandler = slog.NewTextHandler(rotateW, opts)
		}
		handlers = append(handlers, fileHandler)
	}

	// Configure console output (with optional ladder shamsi formatting)
	if toConsole {
		var consoleHandler slog.Handler
		if ladderShamsi {
			consoleHandler = &LadderShamsiHandler{Level: level}
		} else if jsonFormat {
			consoleHandler = slog.NewJSONHandler(os.Stdout, opts)
		} else {
			consoleHandler = slog.NewTextHandler(os.Stdout, opts)
		}
		handlers = append(handlers, consoleHandler)
	}

	// Ensure at least stdout handler exists
	if len(handlers) == 0 {
		handlers = append(handlers, slog.NewTextHandler(os.Stdout, opts))
	}

	var finalHandler slog.Handler
	if len(handlers) == 1 {
		finalHandler = handlers[0]
	} else {
		finalHandler = &TeeHandler{handlers: handlers}
	}

	return &GoBaleLogger{
		slogLogger: slog.New(finalHandler),
		rotateW:    rotateW,
	}
}

// LogChain provides a high-performance, fluent structured dot-system API wrapping slog
type LogChain struct {
	logger *slog.Logger
	level  slog.Level
	msg    string
	attrs  []slog.Attr
	ctx    context.Context
}

// Log opens fluent structured LogChain from Bot context safely
func (b *Bot) Log() *LogChain {
	var slogL *slog.Logger
	if b.loggerInstance != nil {
		slogL = b.loggerInstance.slogLogger
	}
	return &LogChain{
		logger: slogL,
		ctx:    context.Background(),
	}
}

// Log opens fluent structured LogChain from Handler context capturing current chatID
func (c *Ctx) Log() *LogChain {
	var slogL *slog.Logger
	if c.Bot.loggerInstance != nil {
		slogL = c.Bot.loggerInstance.slogLogger
	}

	chain := &LogChain{
		logger: slogL,
		ctx:    c.ctx,
	}

	// Capture chat_id only if the logger is actually active
	if slogL != nil {
		id, _ := c.ChatID()
		if id > 0 {
			chain = chain.Int64("chat_id", id)
		}
	}
	return chain
}

// Debug registers debug level and sets main log message
func (l *LogChain) Debug(msg string) *LogChain {
	if l.logger == nil {
		return l
	}
	l.level = slog.LevelDebug
	l.msg = msg
	return l
}

// Info registers info level and sets main log message
func (l *LogChain) Info(msg string) *LogChain {
	if l.logger == nil {
		return l
	}
	l.level = slog.LevelInfo
	l.msg = msg
	return l
}

// Warn registers warn level and sets main log message
func (l *LogChain) Warn(msg string) *LogChain {
	if l.logger == nil {
		return l
	}
	l.level = slog.LevelWarn
	l.msg = msg
	return l
}

// Error registers error level and sets main log message
func (l *LogChain) Error(msg string) *LogChain {
	if l.logger == nil {
		return l
	}
	l.level = slog.LevelError
	l.msg = msg
	return l
}

// Str appends a structured string attribute
func (l *LogChain) Str(key, val string) *LogChain {
	if l.logger == nil {
		return l
	}
	l.attrs = append(l.attrs, slog.String(key, val))
	return l
}

// Int appends a structured integer attribute
func (l *LogChain) Int(key string, val int) *LogChain {
	if l.logger == nil {
		return l
	}
	l.attrs = append(l.attrs, slog.Int(key, val))
	return l
}

// Int64 appends a structured int64 attribute
func (l *LogChain) Int64(key string, val int64) *LogChain {
	if l.logger == nil {
		return l
	}
	l.attrs = append(l.attrs, slog.Int64(key, val))
	return l
}

// Bool appends a structured boolean attribute
func (l *LogChain) Bool(key string, val bool) *LogChain {
	if l.logger == nil {
		return l
	}
	l.attrs = append(l.attrs, slog.Bool(key, val))
	return l
}

// Float appends a structured float64 attribute
func (l *LogChain) Float(key string, val float64) *LogChain {
	if l.logger == nil {
		return l
	}
	l.attrs = append(l.attrs, slog.Float64(key, val))
	return l
}

// Any appends any dynamic interface attribute
func (l *LogChain) Any(key string, val any) *LogChain {
	if l.logger == nil {
		return l
	}
	l.attrs = append(l.attrs, slog.Any(key, val))
	return l
}

// Err appends a standard structured error attribute if non-nil
func (l *LogChain) Err(err error) *LogChain {
	if l.logger == nil || err == nil {
		return l
	}
	l.attrs = append(l.attrs, slog.Any("error", err))
	return l
}

// Context registers custom execution context
func (l *LogChain) Context(ctx context.Context) *LogChain {
	if l.logger == nil || ctx == nil {
		return l
	}
	l.ctx = ctx
	return l
}

// Group appends grouped nested attributes
func (l *LogChain) Group(name string, attrs ...slog.Attr) *LogChain {
	if l.logger == nil {
		return l
	}
	args := make([]any, len(attrs))
	for i, attr := range attrs {
		args[i] = attr
	}
	l.attrs = append(l.attrs, slog.Group(name, args...))
	return l
}

// Go executes the structured logging pipeline using optimized LogAttrs
func (l *LogChain) Go() {
	if l.logger == nil {
		return
	}
	l.logger.LogAttrs(l.ctx, l.level, l.msg, l.attrs...)
}

// NewCustomLogger instantiates a GoBaleLogger using a custom slog.Handler
func NewCustomLogger(handler slog.Handler) *GoBaleLogger {
	return &GoBaleLogger{
		slogLogger: slog.New(handler),
	}
}

// LadderShamsiHandler pretty-prints shamsi logs recursively in a box-drawing nested tree
type LadderShamsiHandler struct {
	Level slog.Level
	attrs []slog.Attr
}

// Enabled checks if the record level meets the configured severity
func (h *LadderShamsiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.Level
}

// Handle converts Gregorian time to Jalali and renders recursive shamsi ladder box logs using json.Indent
func (h *LadderShamsiHandler) Handle(ctx context.Context, r slog.Record) error {
	shamsiDate := Jalali(r.Time).Format("yyyy/m/d").Go()
	timeStr := fmt.Sprintf("%s %s", shamsiDate, r.Time.Format("15:04:05"))
	levelStr := r.Level.String()

	// Print main bracket header of the ladder log event
	fmt.Printf("┌ [%s] [%s] %s\n", timeStr, levelStr, r.Message)

	// Combine pre-attached attributes with dynamic attributes
	var allAttrs []slog.Attr
	allAttrs = append(allAttrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		allAttrs = append(allAttrs, a)
		return true
	})

	// Print every attribute recursively utilizing the json.Indent ladder engine
	for i, a := range allAttrs {
		isLast := i == len(allAttrs)-1
		printNested(a.Key, a.Value.Any(), "", isLast)
	}

	if len(allAttrs) == 0 {
		fmt.Println("└──────────────────────────────────────────────────")
	}
	return nil
}

// WithAttrs returns a new handler containing pre-attached attributes
func (h *LadderShamsiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LadderShamsiHandler{
		Level: h.Level,
		attrs: append(h.attrs, attrs...),
	}
}

// WithGroup satisfies standard slog.Handler interface specs
func (h *LadderShamsiHandler) WithGroup(name string) slog.Handler {
	return h
}

// printNested recursively formats primitive types, maps, structs, and unmarshals raw JSON strings using UseNumber
func printNested(key string, val any, indent string, isLast bool) {
	if val == nil {
		printLine(indent, key, "<nil>", isLast)
		return
	}

	// Check if the value is a raw JSON string (unmarshals using UseNumber to preserve exact formats)
	if s, ok := val.(string); ok {
		trimmed := strings.TrimSpace(s)
		if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
			(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
			decoder := json.NewDecoder(strings.NewReader(trimmed))
			decoder.UseNumber()
			var generic any
			if decoder.Decode(&generic) == nil {
				printNested(key, generic, indent, isLast)
				return
			}
		}
	}

	v := reflect.ValueOf(val)

	// Safely dereference pointers and interface wrappers
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			printLine(indent, key, "<nil>", isLast)
			return
		}
		v = v.Elem()
	}

	connector := "├─"
	if isLast {
		connector = "└─"
	}

	switch v.Kind() {
	case reflect.Map:
		fmt.Printf("%s%s %s:\n", indent, connector, key)
		nextIndent := indent + "│   "
		if isLast {
			nextIndent = indent + "    "
		}

		// Sort keys to maintain a stable, predictable console layout
		keys := v.MapKeys()
		var sortedKeys []string
		keyMap := make(map[string]reflect.Value, len(keys))
		for _, k := range keys {
			kStr := fmt.Sprintf("%v", k.Interface())
			sortedKeys = append(sortedKeys, kStr)
			keyMap[kStr] = v.MapIndex(k)
		}
		sort.Strings(sortedKeys)

		for i, kStr := range sortedKeys {
			printNested(kStr, keyMap[kStr].Interface(), nextIndent, i == len(sortedKeys)-1)
		}

	case reflect.Struct:
		fmt.Printf("%s%s %s:\n", indent, connector, key)
		nextIndent := indent + "│   "
		if isLast {
			nextIndent = indent + "    "
		}

		t := v.Type()
		var fields []string
		fieldMap := make(map[string]reflect.Value)
		for i := 0; i < v.NumField(); i++ {
			sf := t.Field(i)
			// Skip unexported fields
			if !sf.IsExported() {
				continue
			}
			name := sf.Name
			// Read json tag to display correct API key names if available
			if tag := sf.Tag.Get("json"); tag != "" && tag != "-" {
				name = strings.Split(tag, ",")[0]
			}
			fields = append(fields, name)
			fieldMap[name] = v.Field(i)
		}
		sort.Strings(fields)

		for i, fName := range fields {
			printNested(fName, fieldMap[fName].Interface(), nextIndent, i == len(fields)-1)
		}

	case reflect.Slice, reflect.Array:
		// Fast path for raw byte slices to avoid recursive formatting loops
		if v.Type().Elem().Kind() == reflect.Uint8 {
			fmt.Printf("%s%s %s: %s\n", indent, connector, key, string(v.Bytes()))
			return
		}

		fmt.Printf("%s%s %s:\n", indent, connector, key)
		nextIndent := indent + "│   "
		if isLast {
			nextIndent = indent + "    "
		}
		l := v.Len()
		for i := 0; i < l; i++ {
			printNested(fmt.Sprintf("[%d]", i), v.Index(i).Interface(), nextIndent, i == l-1)
		}

	default:
		// Prints primitive types including json.Number safely as raw integers
		fmt.Printf("%s%s %s: %v\n", indent, connector, key, v.Interface())
	}
}

// printLine outputs a formatted single-line bracket step
func printLine(indent, key, val string, isLast bool) {
	connector := "├─"
	if isLast {
		connector = "└─"
	}
	fmt.Printf("%s%s %s: %s\n", indent, connector, key, val)
}

// TeeHandler fanouts records to multiple slog.Handlers simultaneously
type TeeHandler struct {
	handlers []slog.Handler
}

// Enabled checks if any sub-handler accepts this log level
func (t *TeeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range t.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle dispatches records safely to all enabled sub-handlers
func (t *TeeHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range t.handlers {
		if h.Enabled(ctx, r.Level) {
			_ = h.Handle(ctx, r)
		}
	}
	return nil
}

// WithAttrs appends attributes to all sub-handlers
func (t *TeeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(t.handlers))
	for i, h := range t.handlers {
		newHandlers[i] = h.WithAttrs(attrs)
	}
	return &TeeHandler{handlers: newHandlers}
}

// WithGroup configures structural groups for all sub-handlers
func (t *TeeHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(t.handlers))
	for i, h := range t.handlers {
		newHandlers[i] = h.WithGroup(name)
	}
	return &TeeHandler{handlers: newHandlers}
}

// LoggingRoundTripper intercepts low-level HTTP roundtrips to log raw request and response bytes
type LoggingRoundTripper struct {
	proxied http.RoundTripper
	bot     *Bot
}

// RoundTrip intercepts, clones, and pretty-prints raw network packets in shamsi ladder format
func (l *LoggingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	isGetUpdates := strings.Contains(req.URL.Path, "getUpdates")

	var reqBody []byte
	if req.Body != nil {
		var err error
		reqBody, err = io.ReadAll(req.Body)
		if err == nil {
			// Restore read closer for the actual HTTP execution
			req.Body = io.NopCloser(bytes.NewReader(reqBody))
		}
	}

	start := time.Now()
	resp, err := l.proxied.RoundTrip(req)
	elapsed := time.Since(start)

	if err != nil {
		if req.Context().Err() != nil {
			return nil, err
		}

		if l.bot.loggerInstance != nil && !l.bot.loggerInstance.SuppressHTTP {
			l.bot.Log().Error("خطا در تراکنش شبکه (HTTP Transport Error)").
				Str("url", req.URL.String()).
				Err(err).
				Any("latency", elapsed).
				Go()
		}
		return nil, err
	}

	var respBody []byte
	if resp.Body != nil {
		var err error
		respBody, err = io.ReadAll(resp.Body)
		if err == nil {
			// Restore read closer for the client JSON decoder
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
		}
	}

	// Bypass logging if global suppression is enabled
	if l.bot.loggerInstance != nil {
		if l.bot.loggerInstance.SuppressHTTP {
			return resp, nil
		}
		if isGetUpdates && l.bot.loggerInstance.SuppressGetUpdates {
			return resp, nil
		}
	}

	// Log raw low-level HTTP transaction details cleanly in the Shamsi ladder
	l.bot.Log().Info("تبادل پکت شبکه").
		Str("url", req.URL.Path).
		Str("request", string(reqBody)).
		Str("response", string(respBody)).
		Any("latency", elapsed).
		Go()

	return resp, nil
}

// EnableNetworkInterceptor wraps the HTTP client transport with our LoggingRoundTripper
func (b *Bot) EnableNetworkInterceptor() {
	if b.loggerInstance == nil {
		return
	}
	if b.Client != nil && b.Client.httpClient != nil {
		currentTransport := b.Client.httpClient.Transport
		b.Client.httpClient.Transport = &LoggingRoundTripper{
			proxied: currentTransport,
			bot:     b,
		}
	}
}

// Float64 appends a structured float64 attribute (renamed from Float)
func (l *LogChain) Float64(key string, val float64) *LogChain {
	if l.logger == nil {
		return l
	}
	l.attrs = append(l.attrs, slog.Float64(key, val))
	return l
}

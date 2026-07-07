package gobale

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
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
	slogLogger *slog.Logger
	rotateW    *RotateWriter
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
			consoleHandler = &LadderShamsiHandler{level: level}
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
	} else {
		slogL = slog.Default()
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
	} else {
		slogL = slog.Default()
	}

	chain := &LogChain{
		logger: slogL,
		ctx:    c.ctx,
	}

	id, _ := c.ChatID()
	if id > 0 {
		chain = chain.Int64("chat_id", id)
	}
	return chain
}

// Debug registers debug level and sets main log message
func (l *LogChain) Debug(msg string) *LogChain {
	l.level = slog.LevelDebug
	l.msg = msg
	return l
}

// Info registers info level and sets main log message
func (l *LogChain) Info(msg string) *LogChain {
	l.level = slog.LevelInfo
	l.msg = msg
	return l
}

// Warn registers warn level and sets main log message
func (l *LogChain) Warn(msg string) *LogChain {
	l.level = slog.LevelWarn
	l.msg = msg
	return l
}

// Error registers error level and sets main log message
func (l *LogChain) Error(msg string) *LogChain {
	l.level = slog.LevelError
	l.msg = msg
	return l
}

// Str appends a structured string attribute
func (l *LogChain) Str(key, val string) *LogChain {
	l.attrs = append(l.attrs, slog.String(key, val))
	return l
}

// Int appends a structured integer attribute
func (l *LogChain) Int(key string, val int) *LogChain {
	l.attrs = append(l.attrs, slog.Int(key, val))
	return l
}

// Int64 appends a structured int64 attribute
func (l *LogChain) Int64(key string, val int64) *LogChain {
	l.attrs = append(l.attrs, slog.Int64(key, val))
	return l
}

// Bool appends a structured boolean attribute
func (l *LogChain) Bool(key string, val bool) *LogChain {
	l.attrs = append(l.attrs, slog.Bool(key, val))
	return l
}

// Float appends a structured float64 attribute
func (l *LogChain) Float(key string, val float64) *LogChain {
	l.attrs = append(l.attrs, slog.Float64(key, val))
	return l
}

// Any appends any dynamic interface attribute
func (l *LogChain) Any(key string, val any) *LogChain {
	l.attrs = append(l.attrs, slog.Any(key, val))
	return l
}

// Err appends a standard structured error attribute if non-nil
func (l *LogChain) Err(err error) *LogChain {
	if err != nil {
		l.attrs = append(l.attrs, slog.Any("error", err))
	}
	return l
}

// Context registers custom execution context
func (l *LogChain) Context(ctx context.Context) *LogChain {
	if ctx != nil {
		l.ctx = ctx
	}
	return l
}

// Group appends grouped nested attributes
func (l *LogChain) Group(name string, attrs ...slog.Attr) *LogChain {
	args := make([]any, len(attrs))
	for i, attr := range attrs {
		args[i] = attr
	}
	l.attrs = append(l.attrs, slog.Group(name, args...))
	return l
}

// Go executes the structured logging pipeline using optimized LogAttrs
func (l *LogChain) Go() {
	l.logger.LogAttrs(l.ctx, l.level, l.msg, l.attrs...)
}

// NewCustomLogger instantiates a GoBaleLogger using a custom slog.Handler
func NewCustomLogger(handler slog.Handler) *GoBaleLogger {
	return &GoBaleLogger{
		slogLogger: slog.New(handler),
	}
}

// LadderShamsiHandler implements slog.Handler to pretty-print shamsi logs vertically
type LadderShamsiHandler struct {
	level slog.Level
	attrs []slog.Attr
}

// Enabled checks if the record level meets the configured severity
func (h *LadderShamsiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle converts Gregorian time to Jalali and pretty-prints attributes recursively
func (h *LadderShamsiHandler) Handle(ctx context.Context, r slog.Record) error {
	// Convert standard gregorian time to native Jalali format (e.g. 1405/4/16)
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

	// Print every attribute on its own indented step
	for i, a := range allAttrs {
		connector := "├"
		if i == len(allAttrs)-1 {
			connector = "└"
		}
		fmt.Printf("%s   %s: %v\n", connector, a.Key, a.Value.Any())
	}

	// Print standard footer line if no attributes exist
	if len(allAttrs) == 0 {
		fmt.Println("└──────────────────────────────────────────────────")
	}
	return nil
}

// WithAttrs returns a new handler containing pre-attached attributes
func (h *LadderShamsiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LadderShamsiHandler{
		level: h.level,
		attrs: append(h.attrs, attrs...),
	}
}

// WithGroup satisfies standard slog.Handler interface specs
func (h *LadderShamsiHandler) WithGroup(name string) slog.Handler {
	return h
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

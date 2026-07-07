package gobale

import (
	"context"
	"fmt"
	"io"
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

// NewGoBaleLogger configures structured slog handler with rotating files and console outputs
func NewGoBaleLogger(level slog.Level, path string, toConsole bool, jsonFormat bool) *GoBaleLogger {
	var writer io.Writer
	var rotateW *RotateWriter

	if path != "" {
		rotateW = NewRotateWriter(path, 10*1024*1024, 3)
		if toConsole {
			writer = io.MultiWriter(os.Stdout, rotateW)
		} else {
			writer = rotateW
		}
	} else {
		writer = os.Stdout
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	}

	var handler slog.Handler
	if jsonFormat {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}

	return &GoBaleLogger{
		slogLogger: slog.New(handler),
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

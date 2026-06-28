package gobale

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// LogLevel defines severity tags for logger filters
type LogLevel int

// Supported logging levels
const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Logger manages central log destinations, visual color outputs, and native automatic log rotation
type Logger struct {
	mu        sync.Mutex
	level     LogLevel
	file      *os.File
	toConsole bool
	path      string
	maxSize   int64
	currSize  int64
	backups   int
}

// NewLogger instantiates a thread-safe system logger with native adaptive log rotation defaults
func NewLogger(level LogLevel, path string, toConsole bool) *Logger {
	l := &Logger{
		level:     level,
		toConsole: toConsole,
		path:      path,
		maxSize:   10 * 1024 * 1024, // 10 Megabytes default size limit
		backups:   3,                // Retain maximum 3 backup files on disk
	}
	if path != "" {
		l.openAndSetSize()
	}
	return l
}

// openAndSetSize opens the active file stream and reads its current size
func (l *Logger) openAndSetSize() {
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		l.file = file
		if info, err := file.Stat(); err == nil {
			l.currSize = info.Size()
		}
	}
}

// MaxSize configures custom file rotation size limit in Megabytes fluidly
func (l *Logger) MaxSize(megabytes int64) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maxSize = megabytes * 1024 * 1024
	return l
}

// Backups configures maximum backup files to preserve on system disk fluidly
func (l *Logger) Backups(n int) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n >= 0 {
		l.backups = n
	}
	return l
}

// rotate closes active file, shifts old backups, deletes stale logs, and opens a new stream
func (l *Logger) rotate() {
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}

	// Sequentially shift and rename old backup files (e.g., .2 to .3, .1 to .2)
	for i := l.backups; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", l.path, i)
		if i == l.backups {
			_ = os.Remove(oldPath)
			continue
		}
		newPath := fmt.Sprintf("%s.%d", l.path, i+1)
		_ = os.Rename(oldPath, newPath)
	}

	// Rename the active log file to become the first backup .1
	_ = os.Rename(l.path, l.path+".1")

	// Open a fresh new empty log file stream
	l.openAndSetSize()
}

// Log handles final logs formatting, stream printing, and triggers automatic rotation on overflow
func (l *Logger) Log(level LogLevel, prefix, format string, args []any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if level < l.level {
		return
	}
	timeStr := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	var levelStr string
	var color string
	switch level {
	case LevelDebug:
		levelStr = "DEBUG"
		color = "\033[36m"
	case LevelInfo:
		levelStr = "INFO"
		color = "\033[32m"
	case LevelWarn:
		levelStr = "WARN"
		color = "\033[33m"
	case LevelError:
		levelStr = "ERROR"
		color = "\033[31m"
	}
	if l.file != nil {
		rawLine := fmt.Sprintf("%s [%s] %s%s\n", timeStr, levelStr, prefix, msg)
		bytesWritten, err := l.file.WriteString(rawLine)
		if err == nil {
			l.currSize += int64(bytesWritten)
			// Trigger atomic automatic file rotation if size exceeds configured maxSize
			if l.currSize >= l.maxSize {
				l.rotate()
			}
		}
	}
	if l.toConsole {
		fmt.Printf("%s %s[%s]\033[0m %s%s\n", timeStr, color, levelStr, prefix, msg)
	}
}

// LogChain handles fluent logging setups ending with a Go terminal
type LogChain struct {
	logger *Logger
	prefix string
	level  LogLevel
	format string
	args   []any
}

// Log opens unified LogChain dot system from the Bot context safely with Singleton Instance
func (b *Bot) Log() *LogChain {
	return &LogChain{logger: b.loggerInstance, prefix: ""}
}

// Log opens unified LogChain dot system from the Handler context safely with Singleton Instance
func (c *Ctx) Log() *LogChain {
	id, _ := c.ChatID()
	var prefix string
	if id > 0 {
		prefix = fmt.Sprintf("[Chat: %d] ", id)
	}
	return &LogChain{
		logger: c.Bot.loggerInstance,
		prefix: prefix,
	}
}

// Debug registers a debug level stream log
func (l *LogChain) Debug(format string, args ...any) *LogChain {
	l.level = LevelDebug
	l.format = format
	l.args = args
	return l
}

// Info registers an info level stream log
func (l *LogChain) Info(format string, args ...any) *LogChain {
	l.level = LevelInfo
	l.format = format
	l.args = args
	return l
}

// Warn registers a warn level stream log
func (l *LogChain) Warn(format string, args ...any) *LogChain {
	l.level = LevelWarn
	l.format = format
	l.args = args
	return l
}

// Error registers an error level stream log
func (l *LogChain) Error(format string, args ...any) *LogChain {
	l.level = LevelError
	l.format = format
	l.args = args
	return l
}

// Go executes the printing pipeline on stream destinations
func (l *LogChain) Go() {
	l.logger.Log(l.level, l.prefix, l.format, l.args)
}

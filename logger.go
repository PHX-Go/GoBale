package gobale

import (
	"fmt"
	"os"
	"sync"
	"time"
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

type Logger struct {
	mu        sync.Mutex
	level     LogLevel
	file      *os.File
	toConsole bool
}

func NewLogger(level LogLevel, filePath string, toConsole bool) *Logger {
	l := &Logger{
		level:     level,
		toConsole: toConsole,
	}

	if filePath != "" {
		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			l.file = file
		}
	}
	return l
}

func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	l.level = level
	l.mu.Unlock()
}

func (l *Logger) log(level LogLevel, prefix string, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.level {
		return
	}

	timeStr := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)

	var levelStr string
	var colorCode string

	switch level {
	case LevelDebug:
		levelStr = "DEBUG"
		colorCode = "\033[36m" // Cyan
	case LevelInfo:
		levelStr = "INFO"
		colorCode = "\033[32m" // Green
	case LevelWarn:
		levelStr = "WARN"
		colorCode = "\033[33m" // Yellow
	case LevelError:
		levelStr = "ERROR"
		colorCode = "\033[31m" // Red
	}

	if l.file != nil {
		rawLine := fmt.Sprintf("%s [%s] %s%s\n", timeStr, levelStr, prefix, msg)
		_, _ = l.file.WriteString(rawLine)
	}

	if l.toConsole {
		coloredLine := fmt.Sprintf("%s %s[%s]\033[0m %s%s\n", timeStr, colorCode, levelStr, prefix, msg)
		fmt.Print(coloredLine)
	}
}

func (l *Logger) Debug(format string, args ...any) { l.log(LevelDebug, "", format, args...) }
func (l *Logger) Info(format string, args ...any)  { l.log(LevelInfo, "", format, args...) }
func (l *Logger) Warn(format string, args ...any)  { l.log(LevelWarn, "", format, args...) }
func (l *Logger) Error(format string, args ...any) { l.log(LevelError, "", format, args...) }

type ContextLogger struct {
	logger *Logger
	prefix string
}

func (cl *ContextLogger) Debug(format string, args ...any) {
	cl.logger.log(LevelDebug, cl.prefix, format, args...)
}
func (cl *ContextLogger) Info(format string, args ...any) {
	cl.logger.log(LevelInfo, cl.prefix, format, args...)
}
func (cl *ContextLogger) Warn(format string, args ...any) {
	cl.logger.log(LevelWarn, cl.prefix, format, args...)
}
func (cl *ContextLogger) Error(format string, args ...any) {
	cl.logger.log(LevelError, cl.prefix, format, args...)
}
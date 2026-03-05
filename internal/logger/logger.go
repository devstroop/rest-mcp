package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Level represents a log severity level.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

var levelNames = map[Level]string{
	LevelDebug: "debug",
	LevelInfo:  "info",
	LevelWarn:  "warn",
	LevelError: "error",
}

// ParseLevel parses a log level string (case-insensitive).
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelWarn
	}
}

// Logger is a structured JSON logger that writes to stderr.
type Logger struct {
	level Level
	mu    sync.Mutex
}

// logEntry is a single JSON log line.
type logEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"msg"`
	// Extra fields merged at top level.
	Fields map[string]interface{} `json:"-"`
}

var defaultLogger = &Logger{level: LevelWarn}

// Default returns the package-level logger.
func Default() *Logger {
	return defaultLogger
}

// SetLevel sets the minimum log level.
func SetLevel(l Level) {
	defaultLogger.mu.Lock()
	defaultLogger.level = l
	defaultLogger.mu.Unlock()
}

// New creates a new Logger with the given level.
func New(level Level) *Logger {
	return &Logger{level: level}
}

func (l *Logger) log(level Level, msg string, fields map[string]interface{}) {
	l.mu.Lock()
	minLevel := l.level
	l.mu.Unlock()

	if level < minLevel {
		return
	}

	entry := map[string]interface{}{
		"time":  time.Now().UTC().Format(time.RFC3339),
		"level": levelNames[level],
		"msg":   msg,
	}
	for k, v := range fields {
		entry[k] = v
	}

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, `{"time":"%s","level":"error","msg":"failed to marshal log entry"}`+"\n",
			time.Now().UTC().Format(time.RFC3339))
		return
	}
	fmt.Fprintln(os.Stderr, string(data))
}

// Debug logs at debug level.
func (l *Logger) Debug(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelDebug, msg, f)
}

// Info logs at info level.
func (l *Logger) Info(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelInfo, msg, f)
}

// Warn logs at warn level.
func (l *Logger) Warn(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelWarn, msg, f)
}

// Error logs at error level.
func (l *Logger) Error(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelError, msg, f)
}

// Package-level convenience wrappers.

func Debug(msg string, fields ...map[string]interface{}) { defaultLogger.Debug(msg, fields...) }
func Info(msg string, fields ...map[string]interface{})  { defaultLogger.Info(msg, fields...) }
func Warn(msg string, fields ...map[string]interface{})  { defaultLogger.Warn(msg, fields...) }
func Error(msg string, fields ...map[string]interface{}) { defaultLogger.Error(msg, fields...) }

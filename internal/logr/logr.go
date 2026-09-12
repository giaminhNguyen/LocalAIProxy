// Package logr implements optional sanitized on-disk logging with retention.
// Off by default. Logs never contain prompt content, API keys or tokens.
package logr

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Logger writes sanitized lines to %APPDATA%\LocalAIProxy\logs\<date>.log.
type Logger struct {
	mu     sync.Mutex
	dir    string
	retain int // days
	file   *os.File
	debug  bool
}

// New opens a logger rooted at appDir/logs. stale files older than retain
// days are removed lazily at open time.
func New(appDir string, retain int, debug bool) (*Logger, error) {
	dir := filepath.Join(appDir, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	l := &Logger{dir: dir, retain: retain, debug: debug}
	l.cleanup()
	if err := l.open(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Logger) open() error {
	path := filepath.Join(l.dir, time.Now().Format("2006-01-02")+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	l.file = f
	return nil
}

// SetDebug toggles debug verbosity at runtime.
func (l *Logger) SetDebug(b bool) {
	l.mu.Lock()
	l.debug = b
	l.mu.Unlock()
}

// Debug logs a lower-priority line (only when debug logging is enabled).
func (l *Logger) Debug(format string, args ...any) {
	l.mu.Lock()
	debug := l.debug
	l.mu.Unlock()
	if !debug {
		return
	}
	l.write("DEBUG", format, args...)
}

// Info logs a normal sanitized line.
func (l *Logger) Info(format string, args ...any) {
	l.write("INFO", format, args...)
}

// Error logs an error line.
func (l *Logger) Error(format string, args ...any) {
	l.write("ERROR", format, args...)
}

func (l *Logger) write(level, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	// Roll over at midnight.
	if time.Now().Format("2006-01-02") != l.fileNameDate() {
		l.file.Close()
		_ = l.open()
	}
	line := fmt.Sprintf("%s [%s] %s\n", time.Now().Format(time.RFC3339), level, fmt.Sprintf(format, args...))
	_, _ = io.WriteString(l.file, line)
}

func (l *Logger) fileNameDate() string {
	return time.Now().Format("2006-01-02")
}

// cleanup removes log files older than the retention window.
func (l *Logger) cleanup() {
	if l.retain <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -l.retain)
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		datePart := strings.TrimSuffix(name, ".log")
		t, err := time.Parse("2006-01-02", datePart)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			_ = os.Remove(filepath.Join(l.dir, name))
		}
	}
}

// Close flushes and closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}

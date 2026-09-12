// Package activity keeps an in-memory ring buffer of recent proxy activity.
// Prompts, auth headers, API keys and tokens are NEVER logged.
package activity

import (
	"sync"
	"time"
)

// Entry is one recent event row shown on the Dashboard.
type Entry struct {
	Time       string `json:"time"`     // HH:MM:SS
	Provider   string `json:"provider"` // display name
	Alias      string `json:"alias"`
	Status     string `json:"status"` // Success | Error code | disabled, etc.
	OK         bool   `json:"ok"`
	DurationMS int64  `json:"durationMs"`
	Message    string `json:"message"` // short, sanitized
}

// Log is a fixed-capacity in-memory ring.
type Log struct {
	mu    sync.Mutex
	items []Entry
	max   int
}

// New creates a ring that keeps up to max entries.
func New(max int) *Log {
	return &Log{max: max}
}

// Add prepends an entry (newest first).
func (l *Log) Add(e Entry) {
	if e.Time == "" {
		e.Time = time.Now().Format("15:04:05")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = append([]Entry{e}, l.items...)
	if len(l.items) > l.max {
		l.items = l.items[:l.max]
	}
}

// List returns a copy, newest first.
func (l *Log) List() []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Entry, len(l.items))
	copy(out, l.items)
	return out
}

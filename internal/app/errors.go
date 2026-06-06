package app

import (
	"sync"
	"time"

	"github.com/lsxiaoxin/GoClaw/internal/recovery"
)

// ErrorRecorder keeps recent per-chat error summaries for status output.
type ErrorRecorder struct {
	mu      sync.Mutex
	limit   int
	entries map[string][]ErrorSummary
}

// ErrorSummary is one recent error item.
type ErrorSummary struct {
	At      time.Time
	Summary string
}

// NewErrorRecorder creates a bounded in-memory recorder.
func NewErrorRecorder(limit int) *ErrorRecorder {
	if limit <= 0 {
		limit = 8
	}
	return &ErrorRecorder{
		limit:   limit,
		entries: make(map[string][]ErrorSummary),
	}
}

// Record stores one error summary for chatID.
func (r *ErrorRecorder) Record(chatID string, err error) {
	if r == nil || err == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := ErrorSummary{
		At:      time.Now().UTC(),
		Summary: recovery.Summary(err),
	}
	r.entries[chatID] = append(r.entries[chatID], entry)
	if len(r.entries[chatID]) > r.limit {
		r.entries[chatID] = append([]ErrorSummary(nil), r.entries[chatID][len(r.entries[chatID])-r.limit:]...)
	}
}

// Last returns the latest error summary for chatID.
func (r *ErrorRecorder) Last(chatID string) string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entries := r.entries[chatID]
	if len(entries) == 0 {
		return ""
	}
	return entries[len(entries)-1].Summary
}

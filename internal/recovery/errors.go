// Package recovery classifies errors and defines retry behavior.
package recovery

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Category identifies the subsystem that produced an error.
type Category string

const (
	ModelError      Category = "ModelError"
	ToolError       Category = "ToolError"
	PermissionError Category = "PermissionError"
	HookError       Category = "HookError"
	CompactError    Category = "CompactError"
	StoreError      Category = "StoreError"
	ConfigError     Category = "ConfigError"
	UnknownError    Category = "UnknownError"
)

// Error is a typed error wrapper.
type Error struct {
	Category Category
	Err      error
}

func (e Error) Error() string {
	if e.Err == nil {
		return string(e.Category)
	}
	return string(e.Category) + ": " + e.Err.Error()
}

func (e Error) Unwrap() error {
	return e.Err
}

// Wrap annotates err with a recovery category.
func Wrap(category Category, err error) error {
	if err == nil {
		return nil
	}
	return Error{Category: category, Err: err}
}

// Classify returns the best known category for err.
func Classify(err error) Category {
	if err == nil {
		return UnknownError
	}
	var typed Error
	if errors.As(err, &typed) && typed.Category != "" {
		return typed.Category
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "stream model") ||
		strings.Contains(text, "receive model") ||
		strings.Contains(text, "join model"):
		return ModelError
	case strings.Contains(text, "hook"):
		return HookError
	case strings.Contains(text, "permission"):
		return PermissionError
	case strings.Contains(text, "compact") || strings.Contains(text, "summarize context"):
		return CompactError
	case strings.Contains(text, "config"):
		return ConfigError
	case strings.Contains(text, "store") ||
		strings.Contains(text, "save") ||
		strings.Contains(text, "load") ||
		strings.Contains(text, "read") ||
		strings.Contains(text, "write"):
		return StoreError
	default:
		return UnknownError
	}
}

// RetryPolicy controls bounded retries.
type RetryPolicy struct {
	MaxAttempts int
	Backoff     time.Duration
}

// DefaultRetryPolicy returns conservative model retry defaults.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 2,
		Backoff:     10 * time.Millisecond,
	}
}

// Normalize fills default values.
func (p RetryPolicy) Normalize() RetryPolicy {
	defaults := DefaultRetryPolicy()
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = defaults.MaxAttempts
	}
	if p.Backoff < 0 {
		p.Backoff = 0
	}
	return p
}

// ShouldRetry reports whether category can be retried at attempt.
func (p RetryPolicy) ShouldRetry(category Category, attempt int) bool {
	p = p.Normalize()
	if attempt < 1 || attempt >= p.MaxAttempts {
		return false
	}
	return category == ModelError
}

// BackoffFor returns the wait duration before the next attempt.
func (p RetryPolicy) BackoffFor(attempt int) time.Duration {
	p = p.Normalize()
	if attempt <= 0 || p.Backoff == 0 {
		return 0
	}
	return time.Duration(attempt) * p.Backoff
}

// Summary formats an error for status output.
func Summary(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", Classify(err), err.Error())
}

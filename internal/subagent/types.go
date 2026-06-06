// Package subagent implements isolated child agent task execution.
package subagent

import (
	"fmt"
	"strings"
)

// Status is the outcome of one subagent run.
type Status string

const (
	StatusCompleted       Status = "completed"
	StatusFailed          Status = "failed"
	StatusWaitingApproval Status = "waiting_approval"
	StatusCancelled       Status = "cancelled"
)

// Request describes one delegated child-agent task.
type Request struct {
	Prompt      string `json:"prompt"`
	Description string `json:"description,omitempty"`
}

// Validate checks that a subagent request is executable.
func (r Request) Validate() error {
	if strings.TrimSpace(r.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	return nil
}

// Result is the parent-facing result of one subagent task.
type Result struct {
	Status  Status `json:"status"`
	Summary string `json:"summary"`
	Error   string `json:"error,omitempty"`
}

// Text formats a result as a concise tool result for the parent agent.
func (r Result) Text() string {
	summary := strings.TrimSpace(r.Summary)
	errText := strings.TrimSpace(r.Error)
	switch r.Status {
	case StatusCompleted:
		if summary == "" {
			return "Subagent completed with no summary."
		}
		return "Subagent completed: " + summary
	case StatusWaitingApproval:
		if errText == "" {
			errText = "child agent requested tool approval"
		}
		return "Subagent blocked by permission: " + errText
	case StatusCancelled:
		if errText == "" {
			errText = "cancelled"
		}
		return "Subagent cancelled: " + errText
	default:
		if errText == "" {
			errText = "unknown error"
		}
		if summary != "" {
			return "Subagent failed: " + summary + " (" + errText + ")"
		}
		return "Subagent failed: " + errText
	}
}

// Limits bounds nested and concurrent subagent execution.
type Limits struct {
	MaxDepth      int
	MaxConcurrent int
}

// DefaultLimits returns conservative defaults. A max depth of one forbids
// recursive delegation for normal child agents.
func DefaultLimits() Limits {
	return Limits{
		MaxDepth:      1,
		MaxConcurrent: 2,
	}
}

// Normalize fills missing limits and rejects invalid explicit values.
func (l Limits) Normalize() (Limits, error) {
	defaults := DefaultLimits()
	if l.MaxDepth == 0 {
		l.MaxDepth = defaults.MaxDepth
	}
	if l.MaxConcurrent == 0 {
		l.MaxConcurrent = defaults.MaxConcurrent
	}
	if l.MaxDepth < 0 {
		return Limits{}, fmt.Errorf("max depth must be non-negative")
	}
	if l.MaxConcurrent <= 0 {
		return Limits{}, fmt.Errorf("max concurrent must be positive")
	}
	return l, nil
}

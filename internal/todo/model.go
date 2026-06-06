// Package todo implements persistent per-session task lists.
package todo

import (
	"fmt"
	"strings"
	"time"
)

// Status is the lifecycle state of one todo item.
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
)

// Priority is the scheduling priority of one todo item.
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
)

// Item is one persisted task for a single session.
type Item struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Status    Status    `json:"status"`
	Priority  Priority  `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Summary counts todo items by status.
type Summary struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Completed  int `json:"completed"`
}

// Validate checks one todo item.
func (i Item) Validate() error {
	if strings.TrimSpace(i.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(i.Content) == "" {
		return fmt.Errorf("content is required")
	}
	if !ValidStatus(i.Status) {
		return fmt.Errorf("invalid status %q", i.Status)
	}
	if !ValidPriority(i.Priority) {
		return fmt.Errorf("invalid priority %q", i.Priority)
	}
	return nil
}

// ValidStatus reports whether status is supported.
func ValidStatus(status Status) bool {
	switch status {
	case StatusPending, StatusInProgress, StatusCompleted:
		return true
	default:
		return false
	}
}

// ValidPriority reports whether priority is supported.
func ValidPriority(priority Priority) bool {
	switch priority {
	case PriorityLow, PriorityMedium, PriorityHigh:
		return true
	default:
		return false
	}
}

// Summarize counts items by status.
func Summarize(items []Item) Summary {
	summary := Summary{Total: len(items)}
	for _, item := range items {
		switch item.Status {
		case StatusPending:
			summary.Pending++
		case StatusInProgress:
			summary.InProgress++
		case StatusCompleted:
			summary.Completed++
		}
	}
	return summary
}

// ValidateList checks all items and enforces one in-progress todo.
func ValidateList(items []Item) error {
	seen := make(map[string]struct{}, len(items))
	var inProgress int
	for _, item := range items {
		if err := item.Validate(); err != nil {
			return err
		}
		if _, exists := seen[item.ID]; exists {
			return fmt.Errorf("duplicate todo id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		if item.Status == StatusInProgress {
			inProgress++
		}
	}
	if inProgress > 1 {
		return fmt.Errorf("only one todo can be in_progress")
	}
	return nil
}

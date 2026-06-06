// Package memory implements long-lived project and user memory.
package memory

import (
	"fmt"
	"strings"
	"time"
)

// Category groups memories by intended use.
type Category string

const (
	CategoryUser      Category = "user"
	CategoryFeedback  Category = "feedback"
	CategoryProject   Category = "project"
	CategoryReference Category = "reference"
)

// Entry is one durable memory item.
type Entry struct {
	ID        string    `json:"id"`
	Category  Category  `json:"category"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Categories returns the supported category order.
func Categories() []Category {
	return []Category{CategoryUser, CategoryFeedback, CategoryProject, CategoryReference}
}

// ValidCategory reports whether category is supported.
func ValidCategory(category Category) bool {
	switch category {
	case CategoryUser, CategoryFeedback, CategoryProject, CategoryReference:
		return true
	default:
		return false
	}
}

// Validate checks one entry.
func (e Entry) Validate() error {
	if strings.TrimSpace(e.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if !ValidCategory(e.Category) {
		return fmt.Errorf("invalid memory category %q", e.Category)
	}
	if strings.TrimSpace(e.Content) == "" {
		return fmt.Errorf("content is required")
	}
	return nil
}

// NormalizeCategory validates and normalizes category input.
func NormalizeCategory(value string) (Category, error) {
	category := Category(strings.ToLower(strings.TrimSpace(value)))
	if !ValidCategory(category) {
		return "", fmt.Errorf("invalid memory category %q", value)
	}
	return category, nil
}

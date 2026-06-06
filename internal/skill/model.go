// Package skill loads and selects local GoClaw skills.
package skill

import (
	"fmt"
	"strings"
)

// Skill is one locally configured assistant capability.
type Skill struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	Path         string `json:"path,omitempty"`
}

// Validate checks one skill document.
func (s Skill) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("skill name is required")
	}
	if strings.ContainsAny(s.Name, `/\`) || s.Name == "." || s.Name == ".." {
		return fmt.Errorf("skill name must be a simple directory name")
	}
	if strings.TrimSpace(s.Description) == "" {
		return fmt.Errorf("skill description is required")
	}
	if strings.TrimSpace(s.Instructions) == "" {
		return fmt.Errorf("skill instructions are required")
	}
	return nil
}

// Summary returns the prompt-safe header form injected before a run.
func (s Skill) Summary() string {
	return "- " + strings.TrimSpace(s.Name) + ": " + strings.TrimSpace(s.Description)
}

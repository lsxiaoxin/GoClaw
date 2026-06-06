// Package permission decides whether model-requested tools may execute.
package permission

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Behavior is the outcome of the permission pipeline.
type Behavior string

const (
	Allow   Behavior = "allow"
	Ask     Behavior = "ask"
	Deny    Behavior = "deny"
	Invalid Behavior = "invalid"
)

// Decision describes one permission outcome.
type Decision struct {
	Behavior Behavior
	Reason   string
}

// Validator performs tool-specific argument validation without executing it.
type Validator func(string) error

// Engine applies hard denies, rules, validation, and approval requirements.
type Engine struct{}

// New creates the default s03 permission engine.
func New() *Engine {
	return &Engine{}
}

// Decide runs the four permission layers in their fixed order.
func (e *Engine) Decide(toolName, arguments string, validate Validator) Decision {
	if reason := hardDenyReason(toolName, arguments); reason != "" {
		return Decision{Behavior: Deny, Reason: reason}
	}

	askReason := ruleReason(toolName, arguments)
	if validate == nil {
		return Decision{Behavior: Invalid, Reason: "tool validator is not available"}
	}
	if err := validate(arguments); err != nil {
		return Decision{Behavior: Invalid, Reason: err.Error()}
	}
	if askReason != "" {
		return Decision{Behavior: Ask, Reason: askReason}
	}
	return Decision{Behavior: Allow}
}

func hardDenyReason(toolName, arguments string) string {
	if toolName != "bash" {
		return ""
	}
	command, ok := bashCommand(arguments)
	if !ok {
		return ""
	}
	return HardDenyBash(command)
}

func ruleReason(toolName, arguments string) string {
	switch toolName {
	case "write_file":
		return "write_file modifies workspace files"
	case "edit_file":
		return "edit_file modifies workspace files"
	case "memory_write":
		return "memory_write persists long-term memory"
	case "bash":
		command, ok := bashCommand(arguments)
		if ok && ClearlyReadOnlyBash(command) {
			return ""
		}
		return "bash command is not clearly read-only"
	default:
		return ""
	}
}

func bashCommand(arguments string) (string, bool) {
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", false
	}
	command := strings.TrimSpace(input.Command)
	return command, command != ""
}

// FormatDecision returns a model-facing result for a non-allow decision.
func FormatDecision(decision Decision) string {
	switch decision.Behavior {
	case Deny:
		return fmt.Sprintf("Permission denied: %s", decision.Reason)
	case Invalid:
		return fmt.Sprintf("Error: %s", decision.Reason)
	default:
		return ""
	}
}

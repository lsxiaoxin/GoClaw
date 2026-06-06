// Package contextmgr manages conversation history and compaction.
package contextmgr

import (
	"fmt"
	"strings"
	"time"
)

// Role identifies the source of one conversation message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleSummary   Role = "summary"
)

// Message is one persisted conversation entry.
type Message struct {
	Role      Role      `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Validate checks one message.
func (m Message) Validate() error {
	switch m.Role {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool, RoleSummary:
	default:
		return fmt.Errorf("invalid message role %q", m.Role)
	}
	if strings.TrimSpace(m.Content) == "" {
		return fmt.Errorf("message content is required")
	}
	return nil
}

// ConversationHistory stores one session's compactable messages.
type ConversationHistory struct {
	SessionID string    `json:"session_id"`
	Messages  []Message `json:"messages"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CompactPolicy controls when and how history is compacted.
type CompactPolicy struct {
	MaxMessages    int
	MaxCharacters  int
	RetainMessages int
}

// DefaultPolicy returns conservative local defaults.
func DefaultPolicy() CompactPolicy {
	return CompactPolicy{
		MaxMessages:    40,
		MaxCharacters:  32 * 1024,
		RetainMessages: 8,
	}
}

// Normalize fills missing policy values and validates explicit ones.
func (p CompactPolicy) Normalize() (CompactPolicy, error) {
	defaults := DefaultPolicy()
	if p.MaxMessages == 0 {
		p.MaxMessages = defaults.MaxMessages
	}
	if p.MaxCharacters == 0 {
		p.MaxCharacters = defaults.MaxCharacters
	}
	if p.RetainMessages == 0 {
		p.RetainMessages = defaults.RetainMessages
	}
	if p.MaxMessages <= 0 {
		return CompactPolicy{}, fmt.Errorf("max messages must be positive")
	}
	if p.MaxCharacters <= 0 {
		return CompactPolicy{}, fmt.Errorf("max characters must be positive")
	}
	if p.RetainMessages <= 0 {
		return CompactPolicy{}, fmt.Errorf("retain messages must be positive")
	}
	if p.RetainMessages >= p.MaxMessages {
		return CompactPolicy{}, fmt.Errorf("retain messages must be less than max messages")
	}
	return p, nil
}

// ShouldCompact reports whether history exceeds configured limits.
func (p CompactPolicy) ShouldCompact(messages []Message) bool {
	p, err := p.Normalize()
	if err != nil {
		return false
	}
	if len(messages) > p.MaxMessages {
		return true
	}
	return characterCount(messages) > p.MaxCharacters
}

func characterCount(messages []Message) int {
	var count int
	for _, message := range messages {
		count += len(message.Content)
	}
	return count
}

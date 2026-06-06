package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrApprovalNotFound reports that a chat has no pending approval.
var ErrApprovalNotFound = errors.New("approval not found")

// Approval is the persistent state required to resume one suspended Agent.
type Approval struct {
	ID          string          `json:"id"`
	ChatID      string          `json:"chat_id"`
	RequestedBy string          `json:"requested_by,omitempty"`
	ToolName    string          `json:"tool_name"`
	Arguments   string          `json:"arguments"`
	Reason      string          `json:"reason"`
	Checkpoint  json.RawMessage `json:"checkpoint"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// SaveApproval atomically creates or replaces the pending approval for a chat.
func (s *Store) SaveApproval(approval Approval) error {
	if approval.ID == "" {
		return fmt.Errorf("approval ID is required")
	}
	if approval.ChatID == "" {
		return fmt.Errorf("chat ID is required")
	}
	if approval.ToolName == "" {
		return fmt.Errorf("tool name is required")
	}
	if len(approval.Checkpoint) == 0 || !json.Valid(approval.Checkpoint) {
		return fmt.Errorf("valid approval checkpoint is required")
	}
	now := s.now().UTC()
	if approval.CreatedAt.IsZero() {
		approval.CreatedAt = now
	}
	approval.UpdatedAt = now
	return atomicWriteJSON(s.approvalPath(approval.ChatID), approval)
}

// LoadApproval loads the pending approval for one chat.
func (s *Store) LoadApproval(chatID string) (Approval, error) {
	if chatID == "" {
		return Approval{}, fmt.Errorf("chat ID is required")
	}
	data, err := os.ReadFile(s.approvalPath(chatID))
	if errors.Is(err, os.ErrNotExist) {
		return Approval{}, ErrApprovalNotFound
	}
	if err != nil {
		return Approval{}, fmt.Errorf("read approval: %w", err)
	}
	var approval Approval
	if err := json.Unmarshal(data, &approval); err != nil {
		return Approval{}, fmt.Errorf("decode approval: %w", err)
	}
	if approval.ChatID != chatID {
		return Approval{}, fmt.Errorf("approval chat ID mismatch")
	}
	if approval.ID == "" || approval.ToolName == "" || len(approval.Checkpoint) == 0 {
		return Approval{}, fmt.Errorf("approval record is incomplete")
	}
	return approval, nil
}

// DeleteApproval removes a pending approval. Empty approvalID removes any ID.
func (s *Store) DeleteApproval(chatID, approvalID string) error {
	approval, err := s.LoadApproval(chatID)
	if err != nil {
		return err
	}
	if approvalID != "" && approval.ID != approvalID {
		return ErrApprovalNotFound
	}
	if err := os.Remove(s.approvalPath(chatID)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrApprovalNotFound
		}
		return fmt.Errorf("delete approval: %w", err)
	}
	return nil
}

func (s *Store) approvalPath(chatID string) string {
	return filepath.Join(s.root, "approvals", hashedName(chatID)+".json")
}

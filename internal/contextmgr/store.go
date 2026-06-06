package contextmgr

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store persists conversation histories by session ID.
type Store struct {
	root string
	now  func() time.Time
}

// StoreOption customizes Store.
type StoreOption func(*Store)

// WithStoreClock sets a deterministic store clock.
func WithStoreClock(now func() time.Time) StoreOption {
	return func(s *Store) {
		s.now = now
	}
}

// NewStore creates a store rooted at one directory.
func NewStore(root string, opts ...StoreOption) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("context store root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve context store root: %w", err)
	}
	store := &Store{root: absolute, now: time.Now}
	for _, opt := range opts {
		if opt != nil {
			opt(store)
		}
	}
	if err := os.MkdirAll(store.root, 0o700); err != nil {
		return nil, fmt.Errorf("create context store directory: %w", err)
	}
	if err := os.Chmod(store.root, 0o700); err != nil {
		return nil, fmt.Errorf("set context store directory permissions: %w", err)
	}
	return store, nil
}

// Load returns one history. Missing files return an empty history.
func (s *Store) Load(sessionID string) (ConversationHistory, error) {
	if strings.TrimSpace(sessionID) == "" {
		return ConversationHistory{}, fmt.Errorf("session ID is required")
	}
	data, err := os.ReadFile(s.path(sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return ConversationHistory{SessionID: sessionID}, nil
	}
	if err != nil {
		return ConversationHistory{}, fmt.Errorf("read context history: %w", err)
	}
	var history ConversationHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return ConversationHistory{}, fmt.Errorf("decode context history: %w", err)
	}
	if history.SessionID != sessionID {
		return ConversationHistory{}, fmt.Errorf("context session ID mismatch")
	}
	for _, message := range history.Messages {
		if err := message.Validate(); err != nil {
			return ConversationHistory{}, fmt.Errorf("validate context history: %w", err)
		}
	}
	return history, nil
}

// Save atomically persists one history.
func (s *Store) Save(history ConversationHistory) error {
	if strings.TrimSpace(history.SessionID) == "" {
		return fmt.Errorf("session ID is required")
	}
	now := s.now().UTC()
	if history.UpdatedAt.IsZero() {
		history.UpdatedAt = now
	}
	for index := range history.Messages {
		if history.Messages[index].CreatedAt.IsZero() {
			history.Messages[index].CreatedAt = now
		}
		if err := history.Messages[index].Validate(); err != nil {
			return err
		}
	}
	return atomicWriteJSON(s.path(history.SessionID), history)
}

func (s *Store) path(sessionID string) string {
	sum := sha256.Sum256([]byte(sessionID))
	return filepath.Join(s.root, hex.EncodeToString(sum[:])+".json")
}

func atomicWriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal context JSON: %w", err)
	}
	data = append(data, '\n')

	file, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary context file: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return fmt.Errorf("set temporary context file permissions: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write context file: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync context file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close context file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace context file: %w", err)
	}
	return nil
}

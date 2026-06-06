package todo

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

// Store persists todo lists by chat/session ID.
type Store struct {
	root string
	now  func() time.Time
}

// StoreOption customizes Store.
type StoreOption func(*Store)

// WithClock replaces the wall clock, primarily for deterministic tests.
func WithClock(now func() time.Time) StoreOption {
	return func(store *Store) {
		store.now = now
	}
}

// NewStore creates a todo store under one root directory.
func NewStore(root string, opts ...StoreOption) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("todo store root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve todo store root: %w", err)
	}
	store := &Store{
		root: absolute,
		now:  time.Now,
	}
	for _, opt := range opts {
		opt(store)
	}
	if err := os.MkdirAll(store.root, 0o700); err != nil {
		return nil, fmt.Errorf("create todo store directory: %w", err)
	}
	if err := os.Chmod(store.root, 0o700); err != nil {
		return nil, fmt.Errorf("set todo store directory permissions: %w", err)
	}
	return store, nil
}

// Load returns the todo items for one chat. Missing files return an empty list.
func (s *Store) Load(chatID string) ([]Item, error) {
	if strings.TrimSpace(chatID) == "" {
		return nil, fmt.Errorf("chat ID is required")
	}
	data, err := os.ReadFile(s.path(chatID))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read todo file: %w", err)
	}
	var document fileDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode todo file: %w", err)
	}
	if document.ChatID != chatID {
		return nil, fmt.Errorf("todo chat ID mismatch")
	}
	if err := ValidateList(document.Items); err != nil {
		return nil, fmt.Errorf("validate todo file: %w", err)
	}
	return append([]Item(nil), document.Items...), nil
}

// Save atomically writes the full todo list for one chat.
func (s *Store) Save(chatID string, items []Item) ([]Item, error) {
	if strings.TrimSpace(chatID) == "" {
		return nil, fmt.Errorf("chat ID is required")
	}
	normalized, err := s.normalize(chatID, items)
	if err != nil {
		return nil, err
	}
	if err := ValidateList(normalized); err != nil {
		return nil, err
	}
	document := fileDocument{
		ChatID:    chatID,
		Items:     normalized,
		UpdatedAt: s.now().UTC(),
	}
	if err := atomicWriteJSON(s.path(chatID), document); err != nil {
		return nil, err
	}
	return append([]Item(nil), normalized...), nil
}

// Summary returns item counts for one chat.
func (s *Store) Summary(chatID string) (Summary, error) {
	items, err := s.Load(chatID)
	if err != nil {
		return Summary{}, err
	}
	return Summarize(items), nil
}

func (s *Store) normalize(chatID string, items []Item) ([]Item, error) {
	existing, err := s.Load(chatID)
	if err != nil {
		return nil, err
	}
	created := make(map[string]time.Time, len(existing))
	for _, item := range existing {
		created[item.ID] = item.CreatedAt
	}
	now := s.now().UTC()
	normalized := make([]Item, len(items))
	for index, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		item.Content = strings.TrimSpace(item.Content)
		if item.CreatedAt.IsZero() {
			item.CreatedAt = created[item.ID]
		}
		if item.CreatedAt.IsZero() {
			item.CreatedAt = now
		}
		item.UpdatedAt = now
		normalized[index] = item
	}
	return normalized, nil
}

func (s *Store) path(chatID string) string {
	sum := sha256.Sum256([]byte(chatID))
	return filepath.Join(s.root, hex.EncodeToString(sum[:])+".json")
}

type fileDocument struct {
	ChatID    string    `json:"chat_id"`
	Items     []Item    `json:"items"`
	UpdatedAt time.Time `json:"updated_at"`
}

func atomicWriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal todo JSON: %w", err)
	}
	data = append(data, '\n')

	file, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary todo file: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return fmt.Errorf("set temporary todo file permissions: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write todo file: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync todo file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close todo file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace todo file: %w", err)
	}
	return nil
}

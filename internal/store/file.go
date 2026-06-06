// Package store persists readable GoClaw runtime state on disk.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	sessionStatusIdle    = "idle"
	sessionStatusRunning = "running"
)

// Session describes persistent state for one channel conversation.
type Session struct {
	ChatID     string    `json:"chat_id"`
	Generation uint64    `json:"generation"`
	Status     string    `json:"status"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Store persists events and sessions under one root directory.
type Store struct {
	root string
	now  func() time.Time
}

// Option customizes Store.
type Option func(*Store)

// WithClock replaces the wall clock, primarily for deterministic tests.
func WithClock(now func() time.Time) Option {
	return func(store *Store) {
		store.now = now
	}
}

// New creates the state directory structure.
func New(root string, opts ...Option) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("store root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve store root: %w", err)
	}

	store := &Store{
		root: absolute,
		now:  time.Now,
	}
	for _, opt := range opts {
		opt(store)
	}

	for _, dir := range []string{
		store.root,
		filepath.Join(store.root, "events"),
		filepath.Join(store.root, "sessions"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create store directory %s: %w", dir, err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return nil, fmt.Errorf("set store directory permissions %s: %w", dir, err)
		}
	}
	return store, nil
}

// Root returns the absolute state directory.
func (s *Store) Root() string {
	return s.root
}

// RecordEvent records an Event ID once. It returns false for a duplicate event.
func (s *Store) RecordEvent(eventID string) (bool, error) {
	if eventID == "" {
		return false, fmt.Errorf("event ID is required")
	}
	record := struct {
		EventID string    `json:"event_id"`
		SeenAt  time.Time `json:"seen_at"`
	}{
		EventID: eventID,
		SeenAt:  s.now().UTC(),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal event: %w", err)
	}
	data = append(data, '\n')

	path := filepath.Join(s.root, "events", hashedName(eventID)+".json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create event record: %w", err)
	}

	writeErr := func() error {
		defer file.Close()
		if _, err := file.Write(data); err != nil {
			return err
		}
		return file.Sync()
	}()
	if writeErr != nil {
		_ = os.Remove(path)
		return false, fmt.Errorf("write event record: %w", writeErr)
	}
	return true, nil
}

// LoadSession loads a session or returns a new idle session when none exists.
func (s *Store) LoadSession(chatID string) (Session, error) {
	if chatID == "" {
		return Session{}, fmt.Errorf("chat ID is required")
	}
	path := s.sessionPath(chatID)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Session{
			ChatID:    chatID,
			Status:    sessionStatusIdle,
			UpdatedAt: s.now().UTC(),
		}, nil
	}
	if err != nil {
		return Session{}, fmt.Errorf("read session: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return Session{}, fmt.Errorf("decode session: %w", err)
	}
	if session.ChatID != chatID {
		return Session{}, fmt.Errorf("session chat ID mismatch")
	}
	return session, nil
}

// SaveSession atomically writes one session.
func (s *Store) SaveSession(session Session) error {
	if session.ChatID == "" {
		return fmt.Errorf("chat ID is required")
	}
	if session.Status != sessionStatusIdle && session.Status != sessionStatusRunning {
		return fmt.Errorf("invalid session status %q", session.Status)
	}
	session.UpdatedAt = s.now().UTC()
	return atomicWriteJSON(s.sessionPath(session.ChatID), session)
}

// ResetSession starts a new idle generation for one conversation.
func (s *Store) ResetSession(chatID string) (Session, error) {
	session, err := s.LoadSession(chatID)
	if err != nil {
		return Session{}, err
	}
	session.Generation++
	session.Status = sessionStatusIdle
	if err := s.SaveSession(session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *Store) sessionPath(chatID string) string {
	return filepath.Join(s.root, "sessions", hashedName(chatID)+".json")
}

func hashedName(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func atomicWriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	data = append(data, '\n')

	file, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return fmt.Errorf("set temporary file permissions: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace JSON file: %w", err)
	}
	return nil
}

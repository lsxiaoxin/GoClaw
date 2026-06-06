package memory

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxEntriesPerCategory = 200

// Store persists memories as readable Markdown files grouped by category.
type Store struct {
	root string
	now  func() time.Time
}

// StoreOption customizes Store.
type StoreOption func(*Store)

// WithClock sets a deterministic clock for tests.
func WithClock(now func() time.Time) StoreOption {
	return func(s *Store) {
		s.now = now
	}
}

// NewStore creates a memory store rooted at one directory.
func NewStore(root string, opts ...StoreOption) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("memory store root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve memory store root: %w", err)
	}
	store := &Store{root: absolute, now: time.Now}
	for _, opt := range opts {
		if opt != nil {
			opt(store)
		}
	}
	if err := os.MkdirAll(store.root, 0o700); err != nil {
		return nil, fmt.Errorf("create memory directory: %w", err)
	}
	if err := os.Chmod(store.root, 0o700); err != nil {
		return nil, fmt.Errorf("set memory directory permissions: %w", err)
	}
	return store, nil
}

// Root returns the absolute store root.
func (s *Store) Root() string {
	return s.root
}

// Load returns entries for one category. Missing files return an empty list.
func (s *Store) Load(category Category) ([]Entry, error) {
	if !ValidCategory(category) {
		return nil, fmt.Errorf("invalid memory category %q", category)
	}
	data, err := os.ReadFile(s.path(category))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read memory file: %w", err)
	}
	entries, err := parseMarkdown(category, string(data))
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// LoadAll returns entries from all categories.
func (s *Store) LoadAll() ([]Entry, error) {
	var all []Entry
	for _, category := range Categories() {
		entries, err := s.Load(category)
		if err != nil {
			return nil, err
		}
		all = append(all, entries...)
	}
	sortEntries(all)
	return all, nil
}

// Upsert creates or updates one entry.
func (s *Store) Upsert(entry Entry) (Entry, error) {
	if !ValidCategory(entry.Category) {
		return Entry{}, fmt.Errorf("invalid memory category %q", entry.Category)
	}
	entry.Content = strings.TrimSpace(entry.Content)
	if entry.Content == "" {
		return Entry{}, fmt.Errorf("content is required")
	}
	now := s.now().UTC()
	if strings.TrimSpace(entry.ID) == "" {
		entry.ID = memoryID(entry.Category, entry.Content)
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now
	if err := entry.Validate(); err != nil {
		return Entry{}, err
	}

	entries, err := s.Load(entry.Category)
	if err != nil {
		return Entry{}, err
	}
	updated := false
	for index := range entries {
		if entries[index].ID != entry.ID {
			continue
		}
		entry.CreatedAt = entries[index].CreatedAt
		entries[index] = entry
		updated = true
		break
	}
	if !updated {
		entries = append(entries, entry)
	}
	sortEntries(entries)
	archive, err := s.trimAndArchive(entry.Category, &entries)
	if err != nil {
		return Entry{}, err
	}
	if err := s.save(entry.Category, entries); err != nil {
		return Entry{}, err
	}
	if len(archive) > 0 {
		if err := s.saveArchive(entry.Category, archive); err != nil {
			return Entry{}, err
		}
	}
	return entry, nil
}

// Select returns memories matching a query using simple keyword scoring.
func (s *Store) Select(query string, limit int) ([]Entry, error) {
	entries, err := s.LoadAll()
	if err != nil {
		return nil, err
	}
	return Select(entries, query, limit), nil
}

func (s *Store) trimAndArchive(category Category, entries *[]Entry) ([]Entry, error) {
	if len(*entries) <= maxEntriesPerCategory {
		return nil, nil
	}
	sort.SliceStable(*entries, func(i, j int) bool {
		return (*entries)[i].UpdatedAt.After((*entries)[j].UpdatedAt)
	})
	archive := append([]Entry(nil), (*entries)[maxEntriesPerCategory:]...)
	*entries = append([]Entry(nil), (*entries)[:maxEntriesPerCategory]...)
	sortEntries(*entries)
	return archive, nil
}

func (s *Store) save(category Category, entries []Entry) error {
	for _, entry := range entries {
		if err := entry.Validate(); err != nil {
			return err
		}
	}
	return atomicWriteMemory(s.path(category), formatMarkdown(category, entries))
}

func (s *Store) saveArchive(category Category, entries []Entry) error {
	archiveRoot := filepath.Join(s.root, "archive")
	if err := os.MkdirAll(archiveRoot, 0o700); err != nil {
		return fmt.Errorf("create memory archive directory: %w", err)
	}
	path := filepath.Join(archiveRoot, fmt.Sprintf("%s-%d.md", category, s.now().UTC().UnixNano()))
	return atomicWriteMemory(path, formatMarkdown(category, entries))
}

func (s *Store) path(category Category) string {
	return filepath.Join(s.root, string(category)+".md")
}

func sortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Category != entries[j].Category {
			return entries[i].Category < entries[j].Category
		}
		if entries[i].UpdatedAt.Equal(entries[j].UpdatedAt) {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].UpdatedAt.Before(entries[j].UpdatedAt)
	})
}

func memoryID(category Category, content string) string {
	sum := sha1.Sum([]byte(string(category) + "\x00" + strings.ToLower(strings.TrimSpace(content))))
	return hex.EncodeToString(sum[:])[:12]
}

func atomicWriteMemory(path string, content string) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary memory file: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return fmt.Errorf("set temporary memory permissions: %w", err)
	}
	if _, err := file.WriteString(content); err != nil {
		file.Close()
		return fmt.Errorf("write memory file: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync memory file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close memory file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace memory file: %w", err)
	}
	return nil
}

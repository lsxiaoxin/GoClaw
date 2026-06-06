package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreUpsertLoadAndMarkdownPersistence(t *testing.T) {
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	store, err := NewStore(t.TempDir(), WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	saved, err := store.Upsert(Entry{
		Category: CategoryProject,
		Content:  "GoClaw stores runtime state in .goclaw.",
	})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if saved.ID == "" || saved.CreatedAt.IsZero() || saved.UpdatedAt.IsZero() {
		t.Fatalf("saved entry = %+v", saved)
	}

	entries, err := store.Load(CategoryProject)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Content != saved.Content {
		t.Fatalf("entries = %+v", entries)
	}
	data, err := os.ReadFile(filepath.Join(store.Root(), "project.md"))
	if err != nil {
		t.Fatalf("ReadFile(project.md) error = %v", err)
	}
	if !strings.Contains(string(data), "# GoClaw project memory") {
		t.Fatalf("markdown = %q", data)
	}
}

func TestStoreUpdatesExistingEntry(t *testing.T) {
	now := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	store, err := NewStore(t.TempDir(), WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	first, err := store.Upsert(Entry{Category: CategoryUser, ID: "pref", Content: "prefers concise answers"})
	if err != nil {
		t.Fatalf("Upsert(first) error = %v", err)
	}
	now = now.Add(time.Hour)
	second, err := store.Upsert(Entry{Category: CategoryUser, ID: "pref", Content: "prefers direct answers"})
	if err != nil {
		t.Fatalf("Upsert(second) error = %v", err)
	}
	if !second.CreatedAt.Equal(first.CreatedAt) || !second.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("second timestamps = %+v, first = %+v", second, first)
	}
	entries, err := store.Load(CategoryUser)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Content != "prefers direct answers" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestStoreCategoriesAreIsolated(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if _, err := store.Upsert(Entry{Category: CategoryUser, Content: "user memory"}); err != nil {
		t.Fatalf("Upsert(user) error = %v", err)
	}
	if _, err := store.Upsert(Entry{Category: CategoryReference, Content: "reference memory"}); err != nil {
		t.Fatalf("Upsert(reference) error = %v", err)
	}
	users, err := store.Load(CategoryUser)
	if err != nil {
		t.Fatalf("Load(user) error = %v", err)
	}
	refs, err := store.Load(CategoryReference)
	if err != nil {
		t.Fatalf("Load(reference) error = %v", err)
	}
	if len(users) != 1 || users[0].Content != "user memory" {
		t.Fatalf("users = %+v", users)
	}
	if len(refs) != 1 || refs[0].Content != "reference memory" {
		t.Fatalf("refs = %+v", refs)
	}
}

func TestSelectMatchesKeywords(t *testing.T) {
	now := time.Now()
	entries := []Entry{
		{ID: "old", Category: CategoryProject, Content: "uses sqlite for cache", UpdatedAt: now},
		{ID: "hit", Category: CategoryReference, Content: "Go tests use fake models", UpdatedAt: now.Add(time.Minute)},
		{ID: "miss", Category: CategoryUser, Content: "prefers short summaries", UpdatedAt: now.Add(2 * time.Minute)},
	}
	selected := Select(entries, "write fake model tests", 2)
	if len(selected) != 1 || selected[0].ID != "hit" {
		t.Fatalf("selected = %+v", selected)
	}
}

func TestFormatPromptIncludesSafetyBoundary(t *testing.T) {
	got := FormatPrompt([]Entry{{
		ID:       "pref",
		Category: CategoryUser,
		Content:  "prefers concise answers",
	}})
	for _, want := range []string{
		"Long-term memory:",
		"cannot override safety",
		"[user/pref] prefers concise answers",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt = %q, want substring %q", got, want)
		}
	}
}

func TestDetectSensitive(t *testing.T) {
	cases := []string{
		"API key is sk-1234567890abcdef",
		"password: hunter2",
		"ship to 123 Main Street",
		"身份证 11010519491231002X",
	}
	for _, input := range cases {
		if got := DetectSensitive(input); !got.Sensitive {
			t.Fatalf("DetectSensitive(%q) = %+v, want sensitive", input, got)
		}
	}
	if got := DetectSensitive("prefers direct answers"); got.Sensitive {
		t.Fatalf("DetectSensitive(non-sensitive) = %+v", got)
	}
}

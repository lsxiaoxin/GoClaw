package todo

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStorePersistsUpdatesAndIsolatesChats(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	store, err := NewStore(t.TempDir(), WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	created, err := store.Save("chat-1", []Item{
		{ID: "todo-1", Content: "create tests", Status: StatusPending, Priority: PriorityHigh},
	})
	if err != nil {
		t.Fatalf("Save(create) error = %v", err)
	}
	if len(created) != 1 || created[0].CreatedAt != now || created[0].UpdatedAt != now {
		t.Fatalf("created items = %+v", created)
	}

	later := now.Add(time.Minute)
	store.now = func() time.Time { return later }
	updated, err := store.Save("chat-1", []Item{
		{ID: "todo-1", Content: "create focused tests", Status: StatusCompleted, Priority: PriorityMedium},
		{ID: "todo-2", Content: "update docs", Status: StatusInProgress, Priority: PriorityLow},
	})
	if err != nil {
		t.Fatalf("Save(update) error = %v", err)
	}
	if updated[0].CreatedAt != now || updated[0].UpdatedAt != later ||
		updated[0].Content != "create focused tests" ||
		updated[0].Status != StatusCompleted ||
		updated[0].Priority != PriorityMedium {
		t.Fatalf("updated first item = %+v", updated[0])
	}
	if updated[1].CreatedAt != later || updated[1].Status != StatusInProgress {
		t.Fatalf("updated second item = %+v", updated[1])
	}

	loaded, err := store.Load("chat-1")
	if err != nil {
		t.Fatalf("Load(chat-1) error = %v", err)
	}
	if len(loaded) != 2 || loaded[0].ID != "todo-1" || loaded[1].ID != "todo-2" {
		t.Fatalf("loaded = %+v", loaded)
	}

	summary, err := store.Summary("chat-1")
	if err != nil {
		t.Fatalf("Summary(chat-1) error = %v", err)
	}
	if summary.Total != 2 || summary.InProgress != 1 || summary.Completed != 1 {
		t.Fatalf("summary = %+v", summary)
	}

	other, err := store.Load("chat-2")
	if err != nil {
		t.Fatalf("Load(chat-2) error = %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("chat-2 todos = %+v, want empty", other)
	}

	entries, err := os.ReadDir(store.root)
	if err != nil {
		t.Fatalf("ReadDir(todo root) error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("todo file count = %d, want 1", len(entries))
	}
	info, err := os.Stat(filepath.Join(store.root, entries[0].Name()))
	if err != nil {
		t.Fatalf("Stat(todo file) error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("todo file mode = %o, want 600", got)
	}
}

func TestStoreRejectsInvalidItems(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if _, err := store.Save("chat-1", []Item{{
		ID:       "todo-1",
		Content:  "bad",
		Status:   "unknown",
		Priority: PriorityHigh,
	}}); err == nil {
		t.Fatal("Save() error = nil, want invalid status")
	}
}

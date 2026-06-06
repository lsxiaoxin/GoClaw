package contextmgr

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStorePersistsAndIsolatesHistories(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	store, err := NewStore(t.TempDir(), WithStoreClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	err = store.Save(ConversationHistory{
		SessionID: "chat-1",
		Messages:  []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Save(chat-1) error = %v", err)
	}
	loaded, err := store.Load("chat-1")
	if err != nil {
		t.Fatalf("Load(chat-1) error = %v", err)
	}
	if loaded.SessionID != "chat-1" ||
		len(loaded.Messages) != 1 ||
		loaded.Messages[0].CreatedAt != now ||
		loaded.UpdatedAt != now {
		t.Fatalf("loaded = %+v", loaded)
	}

	other, err := store.Load("chat-2")
	if err != nil {
		t.Fatalf("Load(chat-2) error = %v", err)
	}
	if other.SessionID != "chat-2" || len(other.Messages) != 0 {
		t.Fatalf("other = %+v", other)
	}

	entries, err := os.ReadDir(store.root)
	if err != nil {
		t.Fatalf("ReadDir(context root) error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("context file count = %d, want 1", len(entries))
	}
	info, err := os.Stat(filepath.Join(store.root, entries[0].Name()))
	if err != nil {
		t.Fatalf("Stat(context file) error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("context file mode = %o, want 600", got)
	}
}

func TestStoreRejectsInvalidHistory(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if err := store.Save(ConversationHistory{
		SessionID: "chat-1",
		Messages:  []Message{{Role: "bad", Content: "hello"}},
	}); err == nil {
		t.Fatal("Save() error = nil, want invalid role")
	}
}

package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRecordEventDeduplicatesConcurrently(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	const workers = 16
	results := make(chan bool, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			recorded, err := store.RecordEvent("event-1")
			results <- recorded
			errs <- err
		}()
	}
	wait.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("RecordEvent() error = %v", err)
		}
	}
	var recordedCount int
	for recorded := range results {
		if recorded {
			recordedCount++
		}
	}
	if recordedCount != 1 {
		t.Fatalf("recorded count = %d, want 1", recordedCount)
	}
}

func TestApprovalPersistsAndResetRemovesIt(t *testing.T) {
	now := time.Date(2026, 6, 6, 13, 0, 0, 0, time.UTC)
	state, err := New(t.TempDir(), WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	approval := Approval{
		ID:          "approval-1",
		ChatID:      "chat-1",
		RequestedBy: "user-1",
		ToolName:    "write_file",
		Arguments:   `{"path":"note.txt","content":"hello"}`,
		Reason:      "write_file modifies workspace files",
		Checkpoint:  json.RawMessage(`{"messages":[]}`),
	}
	if err := state.SaveApproval(approval); err != nil {
		t.Fatalf("SaveApproval() error = %v", err)
	}

	loaded, err := state.LoadApproval("chat-1")
	if err != nil {
		t.Fatalf("LoadApproval() error = %v", err)
	}
	if loaded.ID != approval.ID || loaded.RequestedBy != approval.RequestedBy ||
		loaded.CreatedAt != now || loaded.UpdatedAt != now {
		t.Fatalf("loaded approval = %+v", loaded)
	}

	entries, err := os.ReadDir(filepath.Join(state.Root(), "approvals"))
	if err != nil {
		t.Fatalf("ReadDir(approvals) error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("approval file count = %d, want 1", len(entries))
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("approval mode = %o, want 600", got)
	}

	if _, err := state.ResetSession("chat-1"); err != nil {
		t.Fatalf("ResetSession() error = %v", err)
	}
	if _, err := state.LoadApproval("chat-1"); !errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("LoadApproval() after reset error = %v", err)
	}
}

func TestDeleteApprovalRequiresMatchingID(t *testing.T) {
	state, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := state.SaveApproval(Approval{
		ID:         "approval-1",
		ChatID:     "chat-1",
		ToolName:   "bash",
		Checkpoint: json.RawMessage(`{"messages":[]}`),
	}); err != nil {
		t.Fatalf("SaveApproval() error = %v", err)
	}
	if err := state.DeleteApproval("chat-1", "other"); !errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("DeleteApproval(other) error = %v", err)
	}
	if _, err := state.LoadApproval("chat-1"); err != nil {
		t.Fatalf("approval removed by wrong ID: %v", err)
	}
	if err := state.DeleteApproval("chat-1", "approval-1"); err != nil {
		t.Fatalf("DeleteApproval() error = %v", err)
	}
}

func TestResetSessionPersistsReadableJSON(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	store, err := New(t.TempDir(), WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	session, err := store.ResetSession("chat/with unsafe path")
	if err != nil {
		t.Fatalf("ResetSession() error = %v", err)
	}
	if session.Generation != 1 {
		t.Fatalf("Generation = %d, want 1", session.Generation)
	}

	loaded, err := store.LoadSession(session.ChatID)
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if loaded.Generation != 1 || loaded.UpdatedAt != now {
		t.Fatalf("loaded session = %+v", loaded)
	}

	entries, err := os.ReadDir(filepath.Join(store.Root(), "sessions"))
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("session file count = %d, want 1", len(entries))
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("session mode = %o, want 600", got)
	}
}

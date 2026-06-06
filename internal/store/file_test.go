package store

import (
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

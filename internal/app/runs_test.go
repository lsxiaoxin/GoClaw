package app

import (
	"context"
	"testing"
)

func TestRunRegistryCancel(t *testing.T) {
	registry := NewRunRegistry()
	ctx, finish, err := registry.Begin(context.Background(), "chat-1")
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer finish()
	if !registry.Running("chat-1") {
		t.Fatal("Running() = false")
	}
	if _, _, err := registry.Begin(context.Background(), "chat-1"); err == nil {
		t.Fatal("second Begin() error = nil")
	}
	if !registry.Cancel("chat-1") {
		t.Fatal("Cancel() = false")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("run context was not cancelled")
	}
	if registry.Cancel("chat-1") {
		t.Fatal("second Cancel() = true")
	}
}

func TestRunRegistryOldFinishDoesNotDeleteNewRun(t *testing.T) {
	registry := NewRunRegistry()
	_, finishOld, err := registry.Begin(context.Background(), "chat-1")
	if err != nil {
		t.Fatalf("first Begin() error = %v", err)
	}
	if !registry.Cancel("chat-1") {
		t.Fatal("Cancel() = false")
	}
	_, finishNew, err := registry.Begin(context.Background(), "chat-1")
	if err != nil {
		t.Fatalf("second Begin() error = %v", err)
	}
	defer finishNew()

	finishOld()
	if !registry.Running("chat-1") {
		t.Fatal("old finish deleted the new run")
	}
}

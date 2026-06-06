package app

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/lsxiaoxin/GoClaw/internal/channel"
	"github.com/lsxiaoxin/GoClaw/internal/channel/fake"
	"github.com/lsxiaoxin/GoClaw/internal/store"
)

func TestAppCommandsAndPersistentDeduplication(t *testing.T) {
	state, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	responder := fake.New()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := New(state, responder, NewRunRegistry(), "/workspace", logger)

	messages := []channel.Message{
		{EventID: "event-help", MessageID: "message-1", ChatID: "chat-1", Content: "/help"},
		{EventID: "event-new", MessageID: "message-2", ChatID: "chat-1", Content: "/new"},
		{EventID: "event-status", MessageID: "message-3", ChatID: "chat-1", Content: "/status"},
		{EventID: "event-text", MessageID: "message-4", ChatID: "chat-1", Content: "hello"},
	}
	for _, message := range messages {
		if err := application.Handle(context.Background(), message); err != nil {
			t.Fatalf("Handle(%q) error = %v", message.Content, err)
		}
	}

	if err := application.Handle(context.Background(), messages[0]); err != nil {
		t.Fatalf("Handle(duplicate) error = %v", err)
	}

	responses := responder.Responses()
	if len(responses) != len(messages) {
		t.Fatalf("response count = %d, want %d", len(responses), len(messages))
	}
	for _, response := range responses {
		if !response.Closed {
			t.Fatal("response stream was not closed")
		}
	}
	if got := strings.Join(responses[2].Chunks, ""); !strings.Contains(got, "会话代次：1") {
		t.Fatalf("status response = %q", got)
	}
	if got := strings.Join(responses[3].Chunks, ""); !strings.Contains(got, "s01") {
		t.Fatalf("plain response = %q", got)
	}
}

func TestAppCancelActiveRun(t *testing.T) {
	state, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	responder := fake.New()
	runs := NewRunRegistry()
	runCtx, finish, err := runs.Begin(context.Background(), "chat-1")
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer finish()
	application := New(
		state,
		responder,
		runs,
		"/workspace",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	err = application.Handle(context.Background(), channel.Message{
		EventID:   "event-cancel",
		MessageID: "message-1",
		ChatID:    "chat-1",
		Content:   "/cancel",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	select {
	case <-runCtx.Done():
	default:
		t.Fatal("run context was not cancelled")
	}
}

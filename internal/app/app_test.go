package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lsxiaoxin/GoClaw/internal/agent"
	"github.com/lsxiaoxin/GoClaw/internal/channel"
	"github.com/lsxiaoxin/GoClaw/internal/channel/fake"
	"github.com/lsxiaoxin/GoClaw/internal/store"
	"github.com/lsxiaoxin/GoClaw/internal/todo"
)

func TestAppCommandsAndPersistentDeduplication(t *testing.T) {
	state, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	responder := fake.New()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := New(
		context.Background(),
		state,
		responder,
		NewRunRegistry(),
		&stubAgent{
			run: func(ctx context.Context, _ string, emit func(context.Context, string) error) error {
				return emit(ctx, "model reply")
			},
		},
		"/workspace",
		logger,
	)

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

	responses := waitForResponses(t, responder, len(messages))
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
	if got := strings.Join(responses[3].Chunks, ""); got != "model reply" {
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
	started := make(chan struct{})
	cancelled := make(chan struct{})
	application := New(
		context.Background(),
		state,
		responder,
		runs,
		&stubAgent{
			run: func(ctx context.Context, _ string, _ func(context.Context, string) error) error {
				close(started)
				<-ctx.Done()
				close(cancelled)
				return ctx.Err()
			},
		},
		"/workspace",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	err = application.Handle(context.Background(), channel.Message{
		EventID:   "event-prompt",
		MessageID: "message-1",
		ChatID:    "chat-1",
		Content:   "keep running",
	})
	if err != nil {
		t.Fatalf("Handle(prompt) error = %v", err)
	}
	waitForSignal(t, started, "agent start")

	err = application.Handle(context.Background(), channel.Message{
		EventID:   "event-cancel",
		MessageID: "message-2",
		ChatID:    "chat-1",
		Content:   "/cancel",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	waitForSignal(t, cancelled, "agent cancellation")

	responses := waitForResponses(t, responder, 2)
	if got := responseText(t, responses, "message-1"); got != "任务已取消。" {
		t.Fatalf("agent response = %q", got)
	}
	if got := responseText(t, responses, "message-2"); got != "已取消当前任务。" {
		t.Fatalf("cancel response = %q", got)
	}
}

func TestAppRejectsSecondRunForSameChat(t *testing.T) {
	state, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	responder := fake.New()
	started := make(chan struct{})
	release := make(chan struct{})
	application := New(
		context.Background(),
		state,
		responder,
		NewRunRegistry(),
		&stubAgent{
			run: func(ctx context.Context, _ string, emit func(context.Context, string) error) error {
				close(started)
				select {
				case <-release:
					return emit(ctx, "done")
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		},
		"/workspace",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	first := channel.Message{EventID: "event-1", MessageID: "message-1", ChatID: "chat-1", Content: "first"}
	if err := application.Handle(context.Background(), first); err != nil {
		t.Fatalf("Handle(first) error = %v", err)
	}
	waitForSignal(t, started, "agent start")

	second := channel.Message{EventID: "event-2", MessageID: "message-2", ChatID: "chat-1", Content: "second"}
	if err := application.Handle(context.Background(), second); err != nil {
		t.Fatalf("Handle(second) error = %v", err)
	}
	close(release)

	responses := waitForResponses(t, responder, 2)
	if got := responseText(t, responses, "message-2"); !strings.Contains(got, "已有运行中的任务") {
		t.Fatalf("second response = %q", got)
	}
}

func TestAppStatusShowsTodoSummary(t *testing.T) {
	state, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	todos, err := todo.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("todo.NewStore() error = %v", err)
	}
	if _, err := todos.Save("chat-1", []todo.Item{
		{ID: "todo-1", Content: "pending", Status: todo.StatusPending, Priority: todo.PriorityHigh},
		{ID: "todo-2", Content: "active", Status: todo.StatusInProgress, Priority: todo.PriorityMedium},
		{ID: "todo-3", Content: "done", Status: todo.StatusCompleted, Priority: todo.PriorityLow},
	}); err != nil {
		t.Fatalf("Save(chat-1 todos) error = %v", err)
	}
	if _, err := todos.Save("chat-2", []todo.Item{
		{ID: "todo-other", Content: "other", Status: todo.StatusPending, Priority: todo.PriorityLow},
	}); err != nil {
		t.Fatalf("Save(chat-2 todos) error = %v", err)
	}

	responder := fake.New()
	application := New(
		context.Background(),
		state,
		responder,
		NewRunRegistry(),
		&stubAgent{run: func(context.Context, string, func(context.Context, string) error) error {
			return nil
		}},
		"/workspace",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	application.SetTodoStore(todos)

	if err := application.Handle(context.Background(), channel.Message{
		EventID:   "event-status-todo",
		MessageID: "message-status-todo",
		ChatID:    "chat-1",
		Content:   "/status",
	}); err != nil {
		t.Fatalf("Handle(/status) error = %v", err)
	}

	responses := waitForResponses(t, responder, 1)
	got := strings.Join(responses[0].Chunks, "")
	for _, want := range []string{
		"阶段：s07-skill-loading",
		"Todo：total=3 pending=1 in_progress=1 completed=1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status response = %q, want substring %q", got, want)
		}
	}
}

type stubAgent struct {
	run func(context.Context, string, func(context.Context, string) error) error
}

func (a *stubAgent) Start(
	ctx context.Context,
	prompt string,
	emit agent.TextEmitter,
) (agent.RunResult, error) {
	err := a.run(ctx, prompt, emit)
	if err != nil {
		return agent.RunResult{Status: agent.StatusFailed}, err
	}
	return agent.RunResult{Status: agent.StatusCompleted}, nil
}

func (a *stubAgent) Resume(
	context.Context,
	*agent.Checkpoint,
	agent.ApprovalDecision,
	agent.TextEmitter,
) (agent.RunResult, error) {
	return agent.RunResult{Status: agent.StatusFailed}, errors.New("unexpected Resume call")
}

func waitForResponses(t *testing.T, responder *fake.Channel, count int) []fake.Response {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		responses := responder.Responses()
		if len(responses) >= count {
			allClosed := true
			for _, response := range responses[:count] {
				allClosed = allClosed && response.Closed
			}
			if allClosed {
				return responses
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d closed responses", count)
	return nil
}

func waitForSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func responseText(t *testing.T, responses []fake.Response, messageID string) string {
	t.Helper()
	for _, response := range responses {
		if response.Message.MessageID == messageID {
			return strings.Join(response.Chunks, "")
		}
	}
	t.Fatalf("no response for message %s", messageID)
	return ""
}

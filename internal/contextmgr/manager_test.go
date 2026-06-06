package contextmgr

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestManagerDoesNotCompactBelowThreshold(t *testing.T) {
	manager, err := New(CompactPolicy{MaxMessages: 5, MaxCharacters: 1000, RetainMessages: 2}, &fakeSummarizer{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	history, compacted, err := manager.Apply(context.Background(), ConversationHistory{SessionID: "chat-1"}, []Message{
		{Role: RoleUser, Content: "hello"},
		{Role: RoleAssistant, Content: "hi"},
	}, "")
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if compacted || len(history.Messages) != 2 {
		t.Fatalf("compacted = %v, history = %+v", compacted, history)
	}
}

func TestManagerCompactsAndRetainsRecentMessages(t *testing.T) {
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	summarizer := &fakeSummarizer{summary: "tools: read_file README.md\nerrors: none"}
	manager, err := New(
		CompactPolicy{MaxMessages: 3, MaxCharacters: 1000, RetainMessages: 2},
		summarizer,
		WithClock(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	history, compacted, err := manager.Apply(context.Background(), ConversationHistory{SessionID: "chat-1"}, []Message{
		{Role: RoleUser, Content: "first"},
		{Role: RoleTool, Content: "read_file README.md"},
		{Role: RoleAssistant, Content: "middle"},
		{Role: RoleUser, Content: "latest"},
	}, "total=1 pending=1")
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !compacted {
		t.Fatal("compacted = false, want true")
	}
	if len(history.Messages) != 3 {
		t.Fatalf("messages = %+v", history.Messages)
	}
	if history.Messages[0].Role != RoleSummary ||
		history.Messages[0].Content != "tools: read_file README.md\nerrors: none" {
		t.Fatalf("summary message = %+v", history.Messages[0])
	}
	if history.Messages[1].Content != "middle" || history.Messages[2].Content != "latest" {
		t.Fatalf("recent messages = %+v", history.Messages)
	}
	if summarizer.request.TodoSummary != "total=1 pending=1" {
		t.Fatalf("todo summary = %q", summarizer.request.TodoSummary)
	}
}

func TestManagerTriggersByCharacterCount(t *testing.T) {
	manager, err := New(
		CompactPolicy{MaxMessages: 10, MaxCharacters: 8, RetainMessages: 1},
		&fakeSummarizer{summary: "summary"},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, compacted, err := manager.Apply(context.Background(), ConversationHistory{SessionID: "chat-1"}, []Message{
		{Role: RoleUser, Content: "long text"},
		{Role: RoleAssistant, Content: "more"},
	}, "")
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !compacted {
		t.Fatal("compacted = false, want true")
	}
}

func TestManagerSummarizerFailureReturnsOriginalHistory(t *testing.T) {
	manager, err := New(
		CompactPolicy{MaxMessages: 2, MaxCharacters: 1000, RetainMessages: 1},
		&fakeSummarizer{err: errors.New("model unavailable")},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	history, compacted, err := manager.Apply(context.Background(), ConversationHistory{SessionID: "chat-1"}, []Message{
		{Role: RoleUser, Content: "first"},
		{Role: RoleAssistant, Content: "second"},
		{Role: RoleUser, Content: "third"},
	}, "")
	if err == nil || !strings.Contains(err.Error(), "model unavailable") {
		t.Fatalf("Apply() error = %v, want summarizer error", err)
	}
	if compacted {
		t.Fatal("compacted = true, want false on failure")
	}
	if len(history.Messages) != 3 {
		t.Fatalf("history = %+v", history)
	}
}

func TestRuleSummarizerPreservesToolAndTodoSignals(t *testing.T) {
	summary, err := (RuleSummarizer{}).Summarize(context.Background(), SummaryRequest{
		TodoSummary: "total=2 pending=1 in_progress=1",
		Messages: []Message{
			{Role: RoleTool, Content: "read_file README.md\nError: missing file"},
		},
	})
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	for _, want := range []string{"Todo:", "read_file README.md", "Error: missing file"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary = %q, want substring %q", summary, want)
		}
	}
}

type fakeSummarizer struct {
	summary string
	err     error
	request SummaryRequest
}

func (f *fakeSummarizer) Summarize(_ context.Context, request SummaryRequest) (string, error) {
	f.request = request
	if f.err != nil {
		return "", f.err
	}
	return f.summary, nil
}

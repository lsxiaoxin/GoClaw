package contextmgr

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TodoSummaryProvider returns a compact todo state string for a session.
type TodoSummaryProvider interface {
	SummaryText(string) (string, error)
}

// Manager owns a single in-memory conversation history.
type Manager struct {
	policy     CompactPolicy
	summarizer Summarizer
	now        func() time.Time
}

// Option customizes Manager.
type Option func(*Manager)

// WithClock sets a deterministic clock.
func WithClock(now func() time.Time) Option {
	return func(m *Manager) {
		m.now = now
	}
}

// New creates a context manager.
func New(policy CompactPolicy, summarizer Summarizer, opts ...Option) (*Manager, error) {
	normalized, err := policy.Normalize()
	if err != nil {
		return nil, err
	}
	if summarizer == nil {
		summarizer = RuleSummarizer{}
	}
	manager := &Manager{
		policy:     normalized,
		summarizer: summarizer,
		now:        time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(manager)
		}
	}
	return manager, nil
}

// Apply appends new messages and compacts history if thresholds are exceeded.
func (m *Manager) Apply(
	ctx context.Context,
	history ConversationHistory,
	newMessages []Message,
	todoSummary string,
) (ConversationHistory, bool, error) {
	messages := append([]Message(nil), history.Messages...)
	now := m.now().UTC()
	for _, message := range newMessages {
		if message.CreatedAt.IsZero() {
			message.CreatedAt = now
		}
		if err := message.Validate(); err != nil {
			return ConversationHistory{}, false, err
		}
		messages = append(messages, message)
	}
	history.Messages = messages
	history.UpdatedAt = now
	if !m.policy.ShouldCompact(history.Messages) {
		return history, false, nil
	}
	compacted, err := m.compact(ctx, history, todoSummary)
	if err != nil {
		return history, false, err
	}
	return compacted, true, nil
}

func (m *Manager) compact(
	ctx context.Context,
	history ConversationHistory,
	todoSummary string,
) (ConversationHistory, error) {
	retain := m.policy.RetainMessages
	if retain > len(history.Messages) {
		retain = len(history.Messages)
	}
	cut := len(history.Messages) - retain
	older := history.Messages[:cut]
	recent := append([]Message(nil), history.Messages[cut:]...)
	previousSummary, older := splitSummary(older)
	summary, err := m.summarizer.Summarize(ctx, SummaryRequest{
		PreviousSummary: previousSummary,
		Messages:        older,
		TodoSummary:     todoSummary,
	})
	if err != nil {
		return ConversationHistory{}, fmt.Errorf("summarize context: %w", err)
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = "No prior context."
	}
	compacted := ConversationHistory{
		SessionID: history.SessionID,
		Messages: append([]Message{{
			Role:      RoleSummary,
			Content:   summary,
			CreatedAt: m.now().UTC(),
		}}, recent...),
		UpdatedAt: m.now().UTC(),
	}
	return compacted, nil
}

func splitSummary(messages []Message) (string, []Message) {
	if len(messages) == 0 || messages[0].Role != RoleSummary {
		return "", messages
	}
	return messages[0].Content, messages[1:]
}

package contextmgr

import (
	"context"
	"strings"
)

// SummaryRequest is the input to a summarizer implementation.
type SummaryRequest struct {
	PreviousSummary string
	Messages        []Message
	TodoSummary     string
}

// Summarizer creates a compact summary of older conversation messages.
type Summarizer interface {
	Summarize(context.Context, SummaryRequest) (string, error)
}

// RuleSummarizer is a deterministic fallback used when no model summarizer is configured.
type RuleSummarizer struct{}

// Summarize preserves the key operational facts required by s08.
func (RuleSummarizer) Summarize(_ context.Context, request SummaryRequest) (string, error) {
	var lines []string
	if strings.TrimSpace(request.PreviousSummary) != "" {
		lines = append(lines, strings.TrimSpace(request.PreviousSummary))
	}
	if strings.TrimSpace(request.TodoSummary) != "" {
		lines = append(lines, "Todo: "+strings.TrimSpace(request.TodoSummary))
	}
	for _, message := range request.Messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		switch message.Role {
		case RoleTool:
			lines = append(lines, "Tool result: "+truncate(content, 240))
		case RoleUser, RoleAssistant:
			lines = append(lines, string(message.Role)+": "+truncate(content, 180))
		}
	}
	if len(lines) == 0 {
		return "No prior context.", nil
	}
	return strings.Join(lines, "\n"), nil
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	if limit <= 3 {
		return text[:limit]
	}
	return text[:limit-3] + "..."
}

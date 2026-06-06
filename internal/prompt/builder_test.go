package prompt

import (
	"strings"
	"testing"

	"github.com/lsxiaoxin/GoClaw/internal/memory"
	"github.com/lsxiaoxin/GoClaw/internal/skill"
	"github.com/lsxiaoxin/GoClaw/internal/todo"
)

func TestBuilderBasePrompt(t *testing.T) {
	got := NewBuilder().Build(Context{
		Workspace: "/workspace",
		SessionID: "chat-1",
		Channel:   "cli",
		Stage:     "s10-system-prompt",
		Tools:     []string{"memory_write", "read_file"},
	})
	for _, want := range []string{
		"## Identity",
		"Current stage: s10-system-prompt.",
		"Workspace: /workspace",
		"Session: chat-1",
		"Available tools: memory_write, read_file",
		"Dangerous tools require permission",
		"Never reveal secrets",
		"PreToolUse hooks",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt = %q, want substring %q", got, want)
		}
	}
}

func TestBuilderInjectsSkillsMemoryTodoAndSummary(t *testing.T) {
	got := NewBuilder().Build(Context{
		Skills: []skill.Skill{{
			Name:        "go-tests",
			Description: "Write Go tests",
		}},
		Memories: []memory.Entry{{
			ID:       "pref",
			Category: memory.CategoryUser,
			Content:  "prefers direct answers",
		}},
		TodoSummary: todo.Summary{
			Total:      2,
			Pending:    1,
			InProgress: 1,
		},
		ContextSummary: "Previously edited README.",
	})
	for _, want := range []string{
		"## Skills",
		"- go-tests: Write Go tests",
		"## Memory",
		"[user/pref] prefers direct answers",
		"## Todo",
		"total=2 pending=1 in_progress=1 completed=0",
		"## Summary",
		"Previously edited README.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt = %q, want substring %q", got, want)
		}
	}
}

func TestBuilderOmitsDisabledModules(t *testing.T) {
	got := NewBuilder().Build(Context{
		Modules: ModuleFlags{
			Set:     true,
			Skills:  false,
			Memory:  false,
			Todo:    false,
			Compact: false,
		},
		Skills: []skill.Skill{{Name: "docs", Description: "Docs"}},
		Memories: []memory.Entry{{
			ID:       "pref",
			Category: memory.CategoryUser,
			Content:  "prefers direct answers",
		}},
		TodoSummary:    todo.Summary{Total: 1, Pending: 1},
		ContextSummary: "summary",
	})
	for _, unwanted := range []string{
		"## Skills",
		"## Memory",
		"## Todo",
		"## Summary",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("prompt = %q, should not contain %q", got, unwanted)
		}
	}
}

func TestBuilderHashChangesWithContent(t *testing.T) {
	builder := NewBuilder()
	first := builder.Hash(Context{Workspace: "/one"})
	second := builder.Hash(Context{Workspace: "/two"})
	if first == second {
		t.Fatalf("hash did not change: %s", first)
	}
}

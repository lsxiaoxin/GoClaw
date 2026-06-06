package tool

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lsxiaoxin/GoClaw/internal/permission"
	"github.com/lsxiaoxin/GoClaw/internal/todo"
)

func TestTodoWritePersistsAndUpdatesCurrentChat(t *testing.T) {
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	store, err := todo.NewStore(root, todo.WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	tool, err := NewTodoWrite(store)
	if err != nil {
		t.Fatalf("NewTodoWrite() error = %v", err)
	}
	ctx := todo.WithChatID(context.Background(), "chat-1")

	output, err := tool.Run(ctx, `{"items":[
		{"id":"todo-1","content":"write tests","status":"pending","priority":"high"},
		{"id":"todo-2","content":"update docs","status":"in_progress","priority":"medium"}
	]}`)
	if err != nil {
		t.Fatalf("Run(create) error = %v", err)
	}
	if output != "Updated 2 todos: 1 pending, 1 in_progress, 0 completed" {
		t.Fatalf("create output = %q", output)
	}

	later := now.Add(time.Minute)
	store, err = todo.NewStore(root, todo.WithClock(func() time.Time { return later }))
	if err != nil {
		t.Fatalf("NewStore(reopen) error = %v", err)
	}
	tool, err = NewTodoWrite(store)
	if err != nil {
		t.Fatalf("NewTodoWrite(reopen) error = %v", err)
	}
	output, err = tool.Run(ctx, `{"items":[
		{"id":"todo-1","content":"write focused tests","status":"completed","priority":"medium"},
		{"id":"todo-2","content":"update docs","status":"completed","priority":"low"}
	]}`)
	if err != nil {
		t.Fatalf("Run(update) error = %v", err)
	}
	if output != "Updated 2 todos: 0 pending, 0 in_progress, 2 completed" {
		t.Fatalf("update output = %q", output)
	}

	items, err := store.Load("chat-1")
	if err != nil {
		t.Fatalf("Load(chat-1) error = %v", err)
	}
	if len(items) != 2 ||
		items[0].Content != "write focused tests" ||
		items[0].Status != todo.StatusCompleted ||
		items[0].Priority != todo.PriorityMedium ||
		items[1].Priority != todo.PriorityLow {
		t.Fatalf("items = %+v", items)
	}

	other, err := store.Load("chat-2")
	if err != nil {
		t.Fatalf("Load(chat-2) error = %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("chat-2 items = %+v, want empty", other)
	}
}

func TestTodoWriteRejectsInvalidArguments(t *testing.T) {
	store, err := todo.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	tool, err := NewTodoWrite(store)
	if err != nil {
		t.Fatalf("NewTodoWrite() error = %v", err)
	}

	tests := []struct {
		name      string
		arguments string
		want      string
	}{
		{
			name:      "missing items",
			arguments: `{}`,
			want:      "items is required",
		},
		{
			name:      "invalid status",
			arguments: `{"items":[{"id":"todo-1","content":"bad","status":"blocked","priority":"high"}]}`,
			want:      "invalid status",
		},
		{
			name:      "invalid priority",
			arguments: `{"items":[{"id":"todo-1","content":"bad","status":"pending","priority":"urgent"}]}`,
			want:      "invalid priority",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := tool.Validate(test.arguments)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}

	if _, err := tool.Run(context.Background(), `{"items":[]}`); err == nil ||
		!strings.Contains(err.Error(), "chat ID") {
		t.Fatalf("Run() without chat ID error = %v", err)
	}
}

func TestTodoWriteRegistryPermissionUsesValidation(t *testing.T) {
	store, err := todo.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	todoTool, err := NewTodoWrite(store)
	if err != nil {
		t.Fatalf("NewTodoWrite() error = %v", err)
	}
	registry, err := NewRegistry(todoTool)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	decision := registry.Permission(Call{
		Name:      "todo_write",
		Arguments: `{"items":[{"id":"todo-1","content":"write tests","status":"pending","priority":"high"}]}`,
	})
	if decision.Behavior != permission.Allow {
		t.Fatalf("valid todo_write decision = %+v", decision)
	}

	decision = registry.Permission(Call{
		Name:      "todo_write",
		Arguments: `{"items":[{"id":"todo-1","content":"bad","status":"invalid","priority":"high"}]}`,
	})
	if decision.Behavior != permission.Invalid || !strings.Contains(decision.Reason, "invalid status") {
		t.Fatalf("invalid todo_write decision = %+v", decision)
	}
}

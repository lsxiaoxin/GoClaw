package tool

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"

	"github.com/lsxiaoxin/GoClaw/internal/todo"
)

// TodoWrite persists the current session todo list.
type TodoWrite struct {
	store *todo.Store
	info  *schema.ToolInfo
}

// NewTodoWrite creates the todo_write tool.
func NewTodoWrite(store *todo.Store) (*TodoWrite, error) {
	if store == nil {
		return nil, fmt.Errorf("todo store is required")
	}
	return &TodoWrite{
		store: store,
		info: &schema.ToolInfo{
			Name: "todo_write",
			Desc: "Replace the current session todo list. Use it to track task progress with pending, in_progress, and completed items.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"items": {
					Type:     schema.Array,
					Desc:     "Complete todo list for the current session.",
					Required: true,
					ElemInfo: &schema.ParameterInfo{
						Type: schema.Object,
						SubParams: map[string]*schema.ParameterInfo{
							"id": {
								Type:     schema.String,
								Desc:     "Stable todo ID.",
								Required: true,
							},
							"content": {
								Type:     schema.String,
								Desc:     "Task description.",
								Required: true,
							},
							"status": {
								Type:     schema.String,
								Desc:     "One of pending, in_progress, completed.",
								Required: true,
							},
							"priority": {
								Type:     schema.String,
								Desc:     "One of low, medium, high.",
								Required: true,
							},
						},
					},
				},
			}),
		},
	}, nil
}

func (t *TodoWrite) Info() *schema.ToolInfo { return t.info }

// ConcurrencySafe returns false because todo_write replaces the full list.
func (t *TodoWrite) ConcurrencySafe() bool { return false }

// Validate checks todo_write arguments without writing.
func (t *TodoWrite) Validate(arguments string) error {
	items, err := todoWriteItemsFrom(arguments)
	if err != nil {
		return err
	}
	return todo.ValidateList(items)
}

// Run replaces the current session todo list.
func (t *TodoWrite) Run(ctx context.Context, arguments string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	chatID, ok := todo.ChatIDFromContext(ctx)
	if !ok {
		return "", fmt.Errorf("todo_write requires chat ID in context")
	}
	items, err := todoWriteItemsFrom(arguments)
	if err != nil {
		return "", err
	}
	saved, err := t.store.Save(chatID, items)
	if err != nil {
		return "", err
	}
	summary := todo.Summarize(saved)
	return fmt.Sprintf(
		"Updated %d todos: %d pending, %d in_progress, %d completed",
		summary.Total,
		summary.Pending,
		summary.InProgress,
		summary.Completed,
	), nil
}

type todoWriteInput struct {
	Items *[]todoWriteItem `json:"items"`
}

type todoWriteItem struct {
	ID       string        `json:"id"`
	Content  string        `json:"content"`
	Status   todo.Status   `json:"status"`
	Priority todo.Priority `json:"priority"`
}

func todoWriteItemsFrom(arguments string) ([]todo.Item, error) {
	var input todoWriteInput
	if err := decodeArguments(arguments, &input); err != nil {
		return nil, err
	}
	if input.Items == nil {
		return nil, fmt.Errorf("items is required")
	}
	items := make([]todo.Item, len(*input.Items))
	for index, item := range *input.Items {
		items[index] = todo.Item{
			ID:       item.ID,
			Content:  item.Content,
			Status:   item.Status,
			Priority: item.Priority,
		}
	}
	return items, nil
}

package tool

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"

	"github.com/lsxiaoxin/GoClaw/internal/subagent"
)

// Task delegates a read-oriented task to an isolated child agent.
type Task struct {
	executor subagent.Executor
	info     *schema.ToolInfo
}

// NewTask creates the task delegation tool.
func NewTask(executor subagent.Executor) (*Task, error) {
	if executor == nil {
		return nil, fmt.Errorf("subagent executor is required")
	}
	return &Task{
		executor: executor,
		info: &schema.ToolInfo{
			Name: "task",
			Desc: "Delegate a focused read-only investigation to an isolated child agent. The parent only receives the child summary.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"prompt": {
					Type:     schema.String,
					Desc:     "The concrete task for the child agent.",
					Required: true,
				},
				"description": {
					Type: schema.String,
					Desc: "Optional short label for the delegated task.",
				},
			}),
		},
	}, nil
}

func (t *Task) Info() *schema.ToolInfo { return t.info }

// ConcurrencySafe returns false so subagent concurrency is controlled by its executor.
func (t *Task) ConcurrencySafe() bool { return false }

// Validate checks task arguments without launching a child agent.
func (t *Task) Validate(arguments string) error {
	request, err := taskRequestFrom(arguments)
	if err != nil {
		return err
	}
	return request.Validate()
}

// Run executes one delegated child-agent task.
func (t *Task) Run(ctx context.Context, arguments string) (string, error) {
	request, err := taskRequestFrom(arguments)
	if err != nil {
		return "", err
	}
	result, err := t.executor.Execute(ctx, request)
	if err != nil {
		return "", err
	}
	return result.Text(), nil
}

func taskRequestFrom(arguments string) (subagent.Request, error) {
	var request subagent.Request
	if err := decodeArguments(arguments, &request); err != nil {
		return subagent.Request{}, err
	}
	return request, nil
}

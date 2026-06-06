package tool

import (
	"context"
	"strings"
	"testing"

	"github.com/lsxiaoxin/GoClaw/internal/subagent"
)

func TestTaskDelegatesToSubagentExecutor(t *testing.T) {
	executor := &fakeSubagentExecutor{
		result: subagent.Result{
			Status:  subagent.StatusCompleted,
			Summary: "README explains the project.",
		},
	}
	task, err := NewTask(executor)
	if err != nil {
		t.Fatalf("NewTask() error = %v", err)
	}

	output, err := task.Run(context.Background(), `{"prompt":"read README","description":"docs"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if output != "Subagent completed: README explains the project." {
		t.Fatalf("output = %q", output)
	}
	if executor.request.Prompt != "read README" ||
		executor.request.Description != "docs" {
		t.Fatalf("request = %+v", executor.request)
	}
}

func TestTaskRejectsInvalidArguments(t *testing.T) {
	task, err := NewTask(&fakeSubagentExecutor{})
	if err != nil {
		t.Fatalf("NewTask() error = %v", err)
	}
	tests := []struct {
		name      string
		arguments string
		want      string
	}{
		{
			name:      "missing prompt",
			arguments: `{}`,
			want:      "prompt is required",
		},
		{
			name:      "unknown field",
			arguments: `{"prompt":"read","extra":true}`,
			want:      "unknown field",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := task.Validate(test.arguments)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

type fakeSubagentExecutor struct {
	request subagent.Request
	result  subagent.Result
	err     error
}

func (e *fakeSubagentExecutor) Execute(
	_ context.Context,
	request subagent.Request,
) (subagent.Result, error) {
	e.request = request
	return e.result, e.err
}

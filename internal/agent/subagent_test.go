package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"github.com/lsxiaoxin/GoClaw/internal/subagent"
)

func TestSubagentExecutorReturnsSummary(t *testing.T) {
	agentModel := &sequentialModel{
		responses: [][]*schema.AgenticMessage{{
			assistantText("README "),
			assistantText("explains GoClaw."),
		}},
	}
	executor, err := NewSubagentExecutor(agentModel, 3, mustRegistry(t))
	if err != nil {
		t.Fatalf("NewSubagentExecutor() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), subagent.Request{
		Prompt:      "inspect README",
		Description: "summarize docs",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != subagent.StatusCompleted ||
		result.Summary != "README explains GoClaw." {
		t.Fatalf("result = %+v", result)
	}
	if got := agentModel.inputs[0][0].ContentBlocks[0].UserInputText.Text; !strings.Contains(got, "Subagent task: summarize docs") {
		t.Fatalf("child prompt = %q", got)
	}
}

func TestSubagentExecutorReturnsFailureSummary(t *testing.T) {
	agentModel := &sequentialModel{
		repeat: []*schema.AgenticMessage{{
			Role: schema.AgenticRoleTypeAssistant,
		}},
	}
	executor, err := NewSubagentExecutor(agentModel, 1, mustRegistry(t))
	if err != nil {
		t.Fatalf("NewSubagentExecutor() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), subagent.Request{Prompt: "loop"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != subagent.StatusFailed ||
		!strings.Contains(result.Error, ErrMaxSteps.Error()) {
		t.Fatalf("result = %+v", result)
	}
}

func TestSubagentExecutorReportsApprovalWithoutResuming(t *testing.T) {
	write := &stubTool{
		info: &schema.ToolInfo{Name: "write_file"},
		run: func(context.Context, string) (string, error) {
			t.Fatal("write_file should not run before approval")
			return "", nil
		},
	}
	agentModel := &sequentialModel{
		responses: [][]*schema.AgenticMessage{{
			assistantToolCall("write_file", "call-write", `{"path":"note.txt","content":"hello"}`),
		}},
	}
	executor, err := NewSubagentExecutor(agentModel, 3, mustRegistry(t, write))
	if err != nil {
		t.Fatalf("NewSubagentExecutor() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), subagent.Request{Prompt: "write a file"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != subagent.StatusWaitingApproval ||
		!strings.Contains(result.Error, "modifies workspace") {
		t.Fatalf("result = %+v", result)
	}
}

func TestSubagentExecutorEnforcesDepthLimit(t *testing.T) {
	agentModel := &sequentialModel{
		responses: [][]*schema.AgenticMessage{{
			assistantText("unexpected"),
		}},
	}
	executor, err := NewSubagentExecutor(
		agentModel,
		3,
		mustRegistry(t),
		WithSubagentLimits(subagent.Limits{MaxDepth: 1, MaxConcurrent: 1}),
	)
	if err != nil {
		t.Fatalf("NewSubagentExecutor() error = %v", err)
	}

	result, err := executor.Execute(
		subagent.WithDepth(context.Background(), 1),
		subagent.Request{Prompt: "nested task"},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != subagent.StatusFailed ||
		!strings.Contains(result.Error, "max depth 1 exceeded") {
		t.Fatalf("result = %+v", result)
	}
	if len(agentModel.inputs) != 0 {
		t.Fatalf("model calls = %d, want 0", len(agentModel.inputs))
	}
}

func TestSubagentExecutorCancellationPropagates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	executor, err := NewSubagentExecutor(&sequentialModel{}, 3, mustRegistry(t))
	if err != nil {
		t.Fatalf("NewSubagentExecutor() error = %v", err)
	}

	result, err := executor.Execute(ctx, subagent.Request{Prompt: "inspect"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
	if result.Status != subagent.StatusCancelled {
		t.Fatalf("result = %+v", result)
	}
}

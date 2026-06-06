package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"

	"github.com/lsxiaoxin/GoClaw/internal/subagent"
	goclawtool "github.com/lsxiaoxin/GoClaw/internal/tool"
)

// SubagentExecutor runs delegated tasks with an isolated child Runner.
type SubagentExecutor struct {
	model    model.AgenticModel
	maxSteps int
	tools    *goclawtool.Registry
	limits   subagent.Limits
	limiter  *subagent.Limiter
}

type subagentExecutorOptions struct {
	limits  subagent.Limits
	limiter *subagent.Limiter
}

// SubagentOption customizes child-agent execution.
type SubagentOption func(*subagentExecutorOptions)

// WithSubagentLimits sets maximum nesting depth and concurrency.
func WithSubagentLimits(limits subagent.Limits) SubagentOption {
	return func(options *subagentExecutorOptions) {
		options.limits = limits
	}
}

// WithSubagentLimiter shares one concurrency limiter across executors.
func WithSubagentLimiter(limiter *subagent.Limiter) SubagentOption {
	return func(options *subagentExecutorOptions) {
		options.limiter = limiter
	}
}

// NewSubagentExecutor creates a child-agent executor.
func NewSubagentExecutor(
	agentModel model.AgenticModel,
	maxSteps int,
	tools *goclawtool.Registry,
	opts ...SubagentOption,
) (*SubagentExecutor, error) {
	if agentModel == nil {
		return nil, fmt.Errorf("agent model is required")
	}
	if maxSteps <= 0 {
		return nil, fmt.Errorf("max steps must be positive")
	}
	if tools == nil {
		return nil, fmt.Errorf("tool registry is required")
	}
	options := subagentExecutorOptions{limits: subagent.DefaultLimits()}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	limiter := options.limiter
	limits := subagent.Limits{}
	var err error
	if limiter == nil {
		limiter, limits, err = subagent.NewLimiter(options.limits)
		if err != nil {
			return nil, err
		}
	} else {
		limits, err = options.limits.Normalize()
		if err != nil {
			return nil, err
		}
	}
	return &SubagentExecutor{
		model:    agentModel,
		maxSteps: maxSteps,
		tools:    tools,
		limits:   limits,
		limiter:  limiter,
	}, nil
}

// Execute runs a child agent and returns only its final parent-facing summary.
func (e *SubagentExecutor) Execute(
	ctx context.Context,
	request subagent.Request,
) (subagent.Result, error) {
	if err := request.Validate(); err != nil {
		return subagent.Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return subagent.Result{Status: subagent.StatusCancelled, Error: err.Error()}, err
	}
	depth := subagent.DepthFromContext(ctx)
	if depth >= e.limits.MaxDepth {
		return subagent.Result{
			Status: subagent.StatusFailed,
			Error:  fmt.Sprintf("subagent max depth %d exceeded", e.limits.MaxDepth),
		}, nil
	}
	release, err := e.limiter.Acquire(ctx)
	if err != nil {
		return subagent.Result{Status: subagent.StatusCancelled, Error: err.Error()}, err
	}
	defer release()

	child, err := New(e.model, e.maxSteps, e.tools)
	if err != nil {
		return subagent.Result{}, err
	}
	var summary strings.Builder
	result, err := child.Start(
		subagent.WithDepth(ctx, depth+1),
		childPrompt(request),
		func(_ context.Context, text string) error {
			_, writeErr := summary.WriteString(text)
			return writeErr
		},
	)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return subagent.Result{
				Status: subagent.StatusCancelled,
				Error:  err.Error(),
			}, err
		}
		return subagent.Result{
			Status:  subagent.StatusFailed,
			Summary: strings.TrimSpace(summary.String()),
			Error:   err.Error(),
		}, nil
	}

	switch result.Status {
	case StatusCompleted:
		return subagent.Result{
			Status:  subagent.StatusCompleted,
			Summary: strings.TrimSpace(summary.String()),
		}, nil
	case StatusWaitingApproval:
		reason := "child agent requested tool approval"
		if result.Approval != nil && strings.TrimSpace(result.Approval.Reason) != "" {
			reason = strings.TrimSpace(result.Approval.Reason)
		}
		return subagent.Result{
			Status:  subagent.StatusWaitingApproval,
			Summary: strings.TrimSpace(summary.String()),
			Error:   reason,
		}, nil
	case StatusCancelled:
		return subagent.Result{
			Status:  subagent.StatusCancelled,
			Summary: strings.TrimSpace(summary.String()),
			Error:   "cancelled",
		}, nil
	default:
		return subagent.Result{
			Status:  subagent.StatusFailed,
			Summary: strings.TrimSpace(summary.String()),
			Error:   "child agent failed",
		}, nil
	}
}

func childPrompt(request subagent.Request) string {
	prompt := strings.TrimSpace(request.Prompt)
	description := strings.TrimSpace(request.Description)
	if description == "" {
		return prompt
	}
	return "Subagent task: " + description + "\n\n" + prompt
}

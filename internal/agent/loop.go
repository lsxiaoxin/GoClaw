// Package agent implements the GoClaw agent loop.
package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/lsxiaoxin/GoClaw/internal/permission"
	goclawtool "github.com/lsxiaoxin/GoClaw/internal/tool"
)

var (
	ErrMaxSteps         = errors.New("maximum agent steps exceeded")
	ErrApprovalRequired = errors.New("tool approval required")
)

// RunStatus is the terminal or suspended state of one Agent invocation.
type RunStatus string

const (
	StatusCompleted       RunStatus = "completed"
	StatusWaitingApproval RunStatus = "waiting_approval"
	StatusCancelled       RunStatus = "cancelled"
	StatusFailed          RunStatus = "failed"
)

// ApprovalDecision is the user's response to a pending tool call.
type ApprovalDecision string

const (
	DecisionApprove ApprovalDecision = "approve"
	DecisionDeny    ApprovalDecision = "deny"
)

// ApprovalRequest describes the tool call that suspended the Agent.
type ApprovalRequest struct {
	CallID    string `json:"call_id"`
	ToolName  string `json:"tool_name"`
	Arguments string `json:"arguments"`
	Reason    string `json:"reason"`
}

// PendingCall is one model-requested call retained across approval.
type PendingCall struct {
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Checkpoint contains everything required to continue after process restart.
type Checkpoint struct {
	Messages     []*schema.AgenticMessage `json:"messages"`
	Steps        int                      `json:"steps"`
	PendingCalls []PendingCall            `json:"pending_calls,omitempty"`
	PendingIndex int                      `json:"pending_index,omitempty"`
}

// RunResult describes a completed, suspended, cancelled, or failed run.
type RunResult struct {
	Status     RunStatus
	Approval   *ApprovalRequest
	Checkpoint *Checkpoint
}

// TextEmitter receives incremental assistant text.
type TextEmitter = func(context.Context, string) error

// Runner executes the model, permission, and tool loop.
type Runner struct {
	model    model.AgenticModel
	tools    *goclawtool.Registry
	maxSteps int
}

// New creates an agent runner.
func New(agentModel model.AgenticModel, maxSteps int, tools *goclawtool.Registry) (*Runner, error) {
	if agentModel == nil {
		return nil, fmt.Errorf("agent model is required")
	}
	if maxSteps <= 0 {
		return nil, fmt.Errorf("max steps must be positive")
	}
	if tools == nil {
		return nil, fmt.Errorf("tool registry is required")
	}
	return &Runner{model: agentModel, tools: tools, maxSteps: maxSteps}, nil
}

// Start begins one user request.
func (r *Runner) Start(ctx context.Context, prompt string, emit TextEmitter) (RunResult, error) {
	if strings.TrimSpace(prompt) == "" {
		return failedResult(fmt.Errorf("prompt is required"))
	}
	if emit == nil {
		return failedResult(fmt.Errorf("text emitter is required"))
	}

	checkpoint := &Checkpoint{
		Messages: []*schema.AgenticMessage{schema.UserAgenticMessage(prompt)},
	}
	return r.continueRun(ctx, checkpoint, emit)
}

// Resume continues a checkpoint after an approval decision.
func (r *Runner) Resume(
	ctx context.Context,
	checkpoint *Checkpoint,
	decision ApprovalDecision,
	emit TextEmitter,
) (RunResult, error) {
	if emit == nil {
		return failedResult(fmt.Errorf("text emitter is required"))
	}
	if err := validateCheckpoint(checkpoint); err != nil {
		return failedResult(err)
	}
	if decision != DecisionApprove && decision != DecisionDeny {
		return failedResult(fmt.Errorf("invalid approval decision %q", decision))
	}

	call := checkpoint.PendingCalls[checkpoint.PendingIndex]
	if decision == DecisionDeny {
		checkpoint.Messages = append(
			checkpoint.Messages,
			functionToolResult(call.CallID, call.Name, "Permission denied by user."),
		)
	} else {
		policyDecision := r.tools.Permission(goclawtool.Call{
			Name:      call.Name,
			Arguments: call.Arguments,
		})
		switch policyDecision.Behavior {
		case permission.Deny, permission.Invalid:
			checkpoint.Messages = append(
				checkpoint.Messages,
				functionToolResult(
					call.CallID,
					call.Name,
					permission.FormatDecision(policyDecision),
				),
			)
		default:
			result := r.tools.Execute(ctx, []goclawtool.Call{{
				Name:      call.Name,
				Arguments: call.Arguments,
			}})[0]
			if err := ctx.Err(); err != nil {
				return cancelledResult(checkpoint, err)
			}
			checkpoint.Messages = append(
				checkpoint.Messages,
				functionToolResult(call.CallID, call.Name, toolResultText(result)),
			)
		}
	}
	checkpoint.PendingIndex++
	return r.continueRun(ctx, checkpoint, emit)
}

// Run preserves the s01/s02 convenience API for requests that do not suspend.
func (r *Runner) Run(ctx context.Context, prompt string, emit TextEmitter) error {
	result, err := r.Start(ctx, prompt, emit)
	if err != nil {
		return err
	}
	if result.Status == StatusWaitingApproval {
		return ErrApprovalRequired
	}
	return nil
}

func (r *Runner) continueRun(
	ctx context.Context,
	checkpoint *Checkpoint,
	emit TextEmitter,
) (RunResult, error) {
	for {
		if err := ctx.Err(); err != nil {
			return cancelledResult(checkpoint, err)
		}
		if checkpoint.PendingIndex < len(checkpoint.PendingCalls) {
			result, waiting, err := r.processPending(ctx, checkpoint)
			if err != nil {
				return failedCheckpointResult(checkpoint, err)
			}
			if waiting {
				return result, nil
			}
		}

		checkpoint.PendingCalls = nil
		checkpoint.PendingIndex = 0
		if checkpoint.Steps >= r.maxSteps {
			return failedCheckpointResult(checkpoint, ErrMaxSteps)
		}

		response, emitted, err := r.runModel(ctx, checkpoint.Messages, emit)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return cancelledResult(checkpoint, err)
			}
			return failedCheckpointResult(checkpoint, err)
		}
		checkpoint.Steps++
		checkpoint.Messages = append(checkpoint.Messages, response)

		calls := functionToolCalls(response)
		if len(calls) == 0 {
			if emitted {
				return RunResult{Status: StatusCompleted}, nil
			}
			continue
		}
		checkpoint.PendingCalls = make([]PendingCall, len(calls))
		for index, call := range calls {
			checkpoint.PendingCalls[index] = PendingCall{
				CallID:    call.CallID,
				Name:      call.Name,
				Arguments: call.Arguments,
			}
		}
	}
}

func (r *Runner) processPending(
	ctx context.Context,
	checkpoint *Checkpoint,
) (RunResult, bool, error) {
	start := checkpoint.PendingIndex
	end := len(checkpoint.PendingCalls)
	outputs := make(map[int]string, end-start)
	var (
		allowedCalls   []goclawtool.Call
		allowedIndices []int
		approval       *ApprovalRequest
	)

	for index := start; index < end; index++ {
		call := checkpoint.PendingCalls[index]
		decision := r.tools.Permission(goclawtool.Call{
			Name:      call.Name,
			Arguments: call.Arguments,
		})
		if decision.Behavior == permission.Ask {
			end = index
			approval = &ApprovalRequest{
				CallID:    call.CallID,
				ToolName:  call.Name,
				Arguments: call.Arguments,
				Reason:    decision.Reason,
			}
			break
		}
		if decision.Behavior == permission.Allow {
			allowedCalls = append(allowedCalls, goclawtool.Call{
				Name:      call.Name,
				Arguments: call.Arguments,
			})
			allowedIndices = append(allowedIndices, index)
			continue
		}
		outputs[index] = permission.FormatDecision(decision)
	}

	results := r.tools.Execute(ctx, allowedCalls)
	for index, result := range results {
		outputs[allowedIndices[index]] = toolResultText(result)
	}
	for index := start; index < end; index++ {
		if err := ctx.Err(); err != nil {
			return RunResult{}, false, err
		}
		call := checkpoint.PendingCalls[index]
		checkpoint.Messages = append(
			checkpoint.Messages,
			functionToolResult(call.CallID, call.Name, outputs[index]),
		)
		checkpoint.PendingIndex = index + 1
	}

	if approval != nil {
		return RunResult{
			Status:     StatusWaitingApproval,
			Approval:   approval,
			Checkpoint: checkpoint,
		}, true, nil
	}
	return RunResult{}, false, nil
}

func validateCheckpoint(checkpoint *Checkpoint) error {
	if checkpoint == nil {
		return fmt.Errorf("checkpoint is required")
	}
	if len(checkpoint.Messages) == 0 {
		return fmt.Errorf("checkpoint messages are required")
	}
	if checkpoint.Steps < 0 {
		return fmt.Errorf("checkpoint steps are invalid")
	}
	if checkpoint.PendingIndex < 0 || checkpoint.PendingIndex >= len(checkpoint.PendingCalls) {
		return fmt.Errorf("checkpoint has no pending approval")
	}
	return nil
}

func toolResultText(result goclawtool.Result) string {
	text := result.Output
	if result.Err != nil {
		text = "Error: " + result.Err.Error()
	}
	for _, message := range result.HookMessages {
		if strings.TrimSpace(message) == "" {
			continue
		}
		if text != "" {
			text += "\n"
		}
		text += "Hook message: " + strings.TrimSpace(message)
	}
	return text
}

func failedResult(err error) (RunResult, error) {
	return RunResult{Status: StatusFailed}, err
}

func failedCheckpointResult(checkpoint *Checkpoint, err error) (RunResult, error) {
	return RunResult{Status: StatusFailed, Checkpoint: checkpoint}, err
}

func cancelledResult(checkpoint *Checkpoint, err error) (RunResult, error) {
	return RunResult{Status: StatusCancelled, Checkpoint: checkpoint}, err
}

func (r *Runner) runModel(
	ctx context.Context,
	messages []*schema.AgenticMessage,
	emit TextEmitter,
) (*schema.AgenticMessage, bool, error) {
	stream, err := r.model.Stream(ctx, messages, model.WithTools(r.tools.Infos()))
	if err != nil {
		return nil, false, fmt.Errorf("stream model response: %w", err)
	}
	defer stream.Close()

	var (
		chunks  []*schema.AgenticMessage
		emitted bool
	)
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, emitted, fmt.Errorf("receive model response: %w", err)
		}
		if chunk == nil {
			return nil, emitted, fmt.Errorf("receive model response: nil chunk")
		}
		chunks = append(chunks, chunk)

		for _, block := range chunk.ContentBlocks {
			if block == nil || block.AssistantGenText == nil || block.AssistantGenText.Text == "" {
				continue
			}
			if err := emit(ctx, block.AssistantGenText.Text); err != nil {
				return nil, emitted, fmt.Errorf("emit model response: %w", err)
			}
			emitted = true
		}
	}

	response, err := schema.ConcatAgenticMessages(chunks)
	if err != nil {
		return nil, emitted, fmt.Errorf("join model response: %w", err)
	}
	return response, emitted, nil
}

func functionToolCalls(message *schema.AgenticMessage) []*schema.FunctionToolCall {
	if message == nil {
		return nil
	}
	var calls []*schema.FunctionToolCall
	for _, block := range message.ContentBlocks {
		if block != nil && block.FunctionToolCall != nil {
			calls = append(calls, block.FunctionToolCall)
		}
	}
	return calls
}

func functionToolResult(callID, name, result string) *schema.AgenticMessage {
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeUser,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.FunctionToolResult{
				CallID: callID,
				Name:   name,
				Content: []*schema.FunctionToolResultContentBlock{
					{
						Type: schema.FunctionToolResultContentBlockTypeText,
						Text: &schema.UserInputText{Text: result},
					},
				},
			}),
		},
	}
}

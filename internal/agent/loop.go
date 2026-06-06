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

	goclawtool "github.com/lsxiaoxin/GoClaw/internal/tool"
)

var ErrMaxSteps = errors.New("maximum agent steps exceeded")

// TextEmitter receives incremental assistant text.
type TextEmitter = func(context.Context, string) error

// Runner executes the model and tool loop.
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

// Run sends one user prompt through the model and tool loop.
func (r *Runner) Run(ctx context.Context, prompt string, emit TextEmitter) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	if emit == nil {
		return fmt.Errorf("text emitter is required")
	}

	messages := []*schema.AgenticMessage{schema.UserAgenticMessage(prompt)}
	for step := 0; step < r.maxSteps; step++ {
		response, emitted, err := r.runModel(ctx, messages, emit)
		if err != nil {
			return err
		}
		messages = append(messages, response)

		calls := functionToolCalls(response)
		if len(calls) == 0 {
			if emitted {
				return nil
			}
			continue
		}
		toolCalls := make([]goclawtool.Call, len(calls))
		for index, call := range calls {
			toolCalls[index] = goclawtool.Call{Name: call.Name, Arguments: call.Arguments}
		}
		results := r.tools.Execute(ctx, toolCalls)
		for index, result := range results {
			if err := ctx.Err(); err != nil {
				return err
			}
			output := result.Output
			if result.Err != nil {
				output = "Error: " + result.Err.Error()
			}
			messages = append(messages, functionToolResult(calls[index], output))
		}
	}
	return ErrMaxSteps
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

func functionToolResult(call *schema.FunctionToolCall, result string) *schema.AgenticMessage {
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeUser,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.FunctionToolResult{
				CallID: call.CallID,
				Name:   call.Name,
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

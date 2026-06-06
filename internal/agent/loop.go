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
)

var ErrMaxSteps = errors.New("maximum agent steps exceeded")

// TextEmitter receives incremental assistant text.
type TextEmitter = func(context.Context, string) error

// Tool is a function tool available to the model.
type Tool interface {
	Info() *schema.ToolInfo
	Run(context.Context, string) string
}

// Runner executes the model and tool loop.
type Runner struct {
	model    model.AgenticModel
	tools    map[string]Tool
	toolInfo []*schema.ToolInfo
	maxSteps int
}

// New creates an agent runner.
func New(agentModel model.AgenticModel, maxSteps int, tools ...Tool) (*Runner, error) {
	if agentModel == nil {
		return nil, fmt.Errorf("agent model is required")
	}
	if maxSteps <= 0 {
		return nil, fmt.Errorf("max steps must be positive")
	}

	runner := &Runner{
		model:    agentModel,
		tools:    make(map[string]Tool, len(tools)),
		toolInfo: make([]*schema.ToolInfo, 0, len(tools)),
		maxSteps: maxSteps,
	}
	for _, tool := range tools {
		if tool == nil {
			return nil, fmt.Errorf("tool name is required")
		}
		info := tool.Info()
		if info == nil || info.Name == "" {
			return nil, fmt.Errorf("tool name is required")
		}
		name := info.Name
		if _, exists := runner.tools[name]; exists {
			return nil, fmt.Errorf("duplicate tool %q", name)
		}
		runner.tools[name] = tool
		runner.toolInfo = append(runner.toolInfo, info)
	}
	return runner, nil
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
		for _, call := range calls {
			result := r.runTool(ctx, call)
			if err := ctx.Err(); err != nil {
				return err
			}
			messages = append(messages, functionToolResult(call, result))
		}
	}
	return ErrMaxSteps
}

func (r *Runner) runModel(
	ctx context.Context,
	messages []*schema.AgenticMessage,
	emit TextEmitter,
) (*schema.AgenticMessage, bool, error) {
	stream, err := r.model.Stream(ctx, messages, model.WithTools(r.toolInfo))
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

func (r *Runner) runTool(ctx context.Context, call *schema.FunctionToolCall) string {
	tool, ok := r.tools[call.Name]
	if !ok {
		return fmt.Sprintf("tool %q is not available", call.Name)
	}
	return tool.Run(ctx, call.Arguments)
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

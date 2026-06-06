package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	goclawtool "github.com/lsxiaoxin/GoClaw/internal/tool"
)

func TestRunnerStreamsTextResponse(t *testing.T) {
	agentModel := &sequentialModel{
		responses: [][]*schema.AgenticMessage{{
			assistantText("hello "),
			assistantText("world"),
		}},
	}
	runner, err := New(agentModel, 4, mustRegistry(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var chunks []string
	err = runner.Run(context.Background(), "say hello", func(_ context.Context, text string) error {
		chunks = append(chunks, text)
		return nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if want := []string{"hello ", "world"}; !reflect.DeepEqual(chunks, want) {
		t.Fatalf("emitted chunks = %#v, want %#v", chunks, want)
	}
	if got := agentModel.inputs[0][0].ContentBlocks[0].UserInputText.Text; got != "say hello" {
		t.Fatalf("user prompt = %q", got)
	}
}

func TestRunnerExecutesToolAndReturnsResultToModel(t *testing.T) {
	tool := &stubTool{
		info: &schema.ToolInfo{Name: "bash"},
		run: func(_ context.Context, arguments string) (string, error) {
			if arguments != `{"command":"pwd"}` {
				t.Fatalf("tool arguments = %q", arguments)
			}
			return "/workspace", nil
		},
	}
	agentModel := &sequentialModel{
		responses: [][]*schema.AgenticMessage{
			{assistantToolCall("bash", "call-1", `{"command":"pwd"}`)},
			{assistantText("The workspace is /workspace.")},
		},
	}
	runner, err := New(agentModel, 4, mustRegistry(t, tool))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var output string
	err = runner.Run(context.Background(), "where am I?", func(_ context.Context, text string) error {
		output += text
		return nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if output != "The workspace is /workspace." {
		t.Fatalf("output = %q", output)
	}
	if len(agentModel.inputs) != 2 {
		t.Fatalf("model calls = %d, want 2", len(agentModel.inputs))
	}
	if got := agentModel.toolNames[0]; !reflect.DeepEqual(got, []string{"bash"}) {
		t.Fatalf("model tools = %#v", got)
	}

	secondInput := agentModel.inputs[1]
	if len(secondInput) != 3 {
		t.Fatalf("second model input length = %d, want 3", len(secondInput))
	}
	result := secondInput[2].ContentBlocks[0].FunctionToolResult
	if result == nil || result.CallID != "call-1" || result.Name != "bash" {
		t.Fatalf("tool result = %#v", result)
	}
	if got := result.Content[0].Text.Text; got != "/workspace" {
		t.Fatalf("tool result text = %q", got)
	}
}

func TestRunnerStopsAtMaxSteps(t *testing.T) {
	agentModel := &sequentialModel{
		repeat: []*schema.AgenticMessage{{
			Role: schema.AgenticRoleTypeAssistant,
		}},
	}
	runner, err := New(agentModel, 3, mustRegistry(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = runner.Run(context.Background(), "hello", func(context.Context, string) error {
		return nil
	})
	if !errors.Is(err, ErrMaxSteps) {
		t.Fatalf("Run() error = %v, want ErrMaxSteps", err)
	}
	if len(agentModel.inputs) != 3 {
		t.Fatalf("model calls = %d, want 3", len(agentModel.inputs))
	}
}

func TestRunnerReturnsUnknownToolResultToModel(t *testing.T) {
	agentModel := &sequentialModel{
		responses: [][]*schema.AgenticMessage{
			{assistantToolCall("missing", "call-1", `{}`)},
			{assistantText("The tool is unavailable.")},
		},
	}
	runner, err := New(agentModel, 3, mustRegistry(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = runner.Run(context.Background(), "use missing", func(context.Context, string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := agentModel.inputs[1][2].ContentBlocks[0].FunctionToolResult.Content[0].Text.Text
	if got != `Error: tool "missing" is not available` {
		t.Fatalf("unknown tool result = %q", got)
	}
}

type sequentialModel struct {
	responses [][]*schema.AgenticMessage
	repeat    []*schema.AgenticMessage
	inputs    [][]*schema.AgenticMessage
	toolNames [][]string
}

func (m *sequentialModel) Generate(
	context.Context,
	[]*schema.AgenticMessage,
	...model.Option,
) (*schema.AgenticMessage, error) {
	return nil, errors.New("unexpected Generate call")
}

func (m *sequentialModel) Stream(
	_ context.Context,
	input []*schema.AgenticMessage,
	opts ...model.Option,
) (*schema.StreamReader[*schema.AgenticMessage], error) {
	m.inputs = append(m.inputs, append([]*schema.AgenticMessage(nil), input...))
	options := model.GetCommonOptions(&model.Options{}, opts...)
	var names []string
	for _, tool := range options.Tools {
		names = append(names, tool.Name)
	}
	m.toolNames = append(m.toolNames, names)

	index := len(m.inputs) - 1
	if index < len(m.responses) {
		return schema.StreamReaderFromArray(m.responses[index]), nil
	}
	if m.repeat != nil {
		return schema.StreamReaderFromArray(m.repeat), nil
	}
	return nil, errors.New("no model response configured")
}

type stubTool struct {
	info *schema.ToolInfo
	safe bool
	run  func(context.Context, string) (string, error)
}

func (t *stubTool) Info() *schema.ToolInfo {
	return t.info
}

func (t *stubTool) ConcurrencySafe() bool {
	return t.safe
}

func (t *stubTool) Run(ctx context.Context, arguments string) (string, error) {
	return t.run(ctx, arguments)
}

func mustRegistry(t *testing.T, tools ...goclawtool.Tool) *goclawtool.Registry {
	t.Helper()
	registry, err := goclawtool.NewRegistry(tools...)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry
}

func assistantText(text string) *schema.AgenticMessage {
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text: text}),
		},
	}
}

func assistantToolCall(name, callID, arguments string) *schema.AgenticMessage {
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.FunctionToolCall{
				Name:      name,
				CallID:    callID,
				Arguments: arguments,
			}),
		},
	}
}

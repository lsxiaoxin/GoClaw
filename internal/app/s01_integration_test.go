package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/lsxiaoxin/GoClaw/internal/agent"
	"github.com/lsxiaoxin/GoClaw/internal/app"
	"github.com/lsxiaoxin/GoClaw/internal/channel"
	"github.com/lsxiaoxin/GoClaw/internal/channel/fake"
	"github.com/lsxiaoxin/GoClaw/internal/store"
	goclawtool "github.com/lsxiaoxin/GoClaw/internal/tool"
)

func TestS01AgentLoopRunsBashAndStreamsFinalReply(t *testing.T) {
	workspace := t.TempDir()
	agentModel := &recordingModel{
		stream: func(
			_ context.Context,
			call int,
			_ []*schema.AgenticMessage,
			_ ...model.Option,
		) (*schema.StreamReader[*schema.AgenticMessage], error) {
			switch call {
			case 0:
				return messageStream(functionCall(
					"bash",
					"call-1",
					`{"command":"printf s01 > result.txt && cat result.txt"}`,
				)), nil
			case 1:
				return messageStream(assistantText("bash "), assistantText("completed")), nil
			default:
				return nil, errors.New("unexpected model call")
			}
		},
	}
	application, responder, runs := newS01Application(t, workspace, agentModel)

	err := application.Handle(context.Background(), channel.Message{
		EventID:   "event-run-bash",
		MessageID: "message-run-bash",
		ChatID:    "chat-1",
		Content:   "Run the workspace command and report the result.",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	responses := waitForClosedResponses(t, responder, 1)
	if got := responses[0].Chunks; !reflect.DeepEqual(got, []string{"bash ", "completed"}) {
		t.Fatalf("stream chunks = %#v", got)
	}
	if runs.Running("chat-1") {
		t.Fatal("run remains active after final model response")
	}

	data, err := os.ReadFile(filepath.Join(workspace, "result.txt"))
	if err != nil {
		t.Fatalf("ReadFile(result.txt) error = %v", err)
	}
	if string(data) != "s01" {
		t.Fatalf("result.txt = %q", data)
	}

	inputs, toolNames := agentModel.snapshot()
	if len(inputs) != 2 {
		t.Fatalf("model calls = %d, want 2", len(inputs))
	}
	if got := toolNames[0]; !reflect.DeepEqual(got, []string{"bash"}) {
		t.Fatalf("first model call tools = %#v", got)
	}
	result := inputs[1][2].ContentBlocks[0].FunctionToolResult
	if result == nil {
		t.Fatal("second model input has no function tool result")
	}
	if got := result.Content[0].Text.Text; got != "s01" {
		t.Fatalf("bash tool result = %q", got)
	}
}

func TestS01CancelStopsBashAndChatCanRunAgain(t *testing.T) {
	workspace := t.TempDir()
	agentModel := &recordingModel{
		stream: func(
			_ context.Context,
			_ int,
			input []*schema.AgenticMessage,
			_ ...model.Option,
		) (*schema.StreamReader[*schema.AgenticMessage], error) {
			prompt := userPrompt(input[0])
			switch prompt {
			case "long task":
				return messageStream(functionCall(
					"bash",
					"call-long",
					`{"command":"printf started > started.txt; sleep 30; printf finished > finished.txt"}`,
				)), nil
			case "after cancel":
				return messageStream(assistantText("ready again")), nil
			default:
				return nil, errors.New("unexpected prompt: " + prompt)
			}
		},
	}
	application, responder, runs := newS01Application(t, workspace, agentModel)

	if err := application.Handle(context.Background(), channel.Message{
		EventID:   "event-long",
		MessageID: "message-long",
		ChatID:    "chat-1",
		Content:   "long task",
	}); err != nil {
		t.Fatalf("Handle(long task) error = %v", err)
	}
	waitForFile(t, filepath.Join(workspace, "started.txt"))
	if !runs.Running("chat-1") {
		t.Fatal("run is not active while bash is executing")
	}

	if err := application.Handle(context.Background(), channel.Message{
		EventID:   "event-cancel",
		MessageID: "message-cancel",
		ChatID:    "chat-1",
		Content:   "/cancel",
	}); err != nil {
		t.Fatalf("Handle(/cancel) error = %v", err)
	}

	responses := waitForClosedResponses(t, responder, 2)
	if got := strings.Join(responses[0].Chunks, ""); got != "任务已取消。" {
		t.Fatalf("cancelled agent response = %q", got)
	}
	if got := strings.Join(responses[1].Chunks, ""); got != "已取消当前任务。" {
		t.Fatalf("/cancel response = %q", got)
	}
	if _, err := os.Stat(filepath.Join(workspace, "finished.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("finished.txt exists after cancellation, stat error = %v", err)
	}

	if err := application.Handle(context.Background(), channel.Message{
		EventID:   "event-after-cancel",
		MessageID: "message-after-cancel",
		ChatID:    "chat-1",
		Content:   "after cancel",
	}); err != nil {
		t.Fatalf("Handle(after cancel) error = %v", err)
	}

	responses = waitForClosedResponses(t, responder, 3)
	if got := strings.Join(responses[2].Chunks, ""); got != "ready again" {
		t.Fatalf("response after cancellation = %q", got)
	}
	if runs.Running("chat-1") {
		t.Fatal("run remains active after second request")
	}
}

func newS01Application(
	t *testing.T,
	workspace string,
	agentModel model.AgenticModel,
) (*app.App, *fake.Channel, *app.RunRegistry) {
	t.Helper()
	bashTool, err := goclawtool.NewBash(workspace, 5*time.Second, 64*1024)
	if err != nil {
		t.Fatalf("NewBash() error = %v", err)
	}
	registry, err := goclawtool.NewRegistry(bashTool)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	runner, err := agent.New(agentModel, 4, registry)
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	state, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	responder := fake.New()
	runs := app.NewRunRegistry()
	application := app.New(
		context.Background(),
		state,
		responder,
		runs,
		runner,
		workspace,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	return application, responder, runs
}

func waitForClosedResponses(t *testing.T, responder *fake.Channel, count int) []fake.Response {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		responses := responder.Responses()
		if len(responses) >= count {
			allClosed := true
			for _, response := range responses[:count] {
				if !response.Closed {
					allClosed = false
					break
				}
			}
			if allClosed {
				return responses
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d closed responses", count)
	return nil
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for file %s", path)
}

type recordingModel struct {
	mu        sync.Mutex
	inputs    [][]*schema.AgenticMessage
	toolNames [][]string
	stream    func(
		context.Context,
		int,
		[]*schema.AgenticMessage,
		...model.Option,
	) (*schema.StreamReader[*schema.AgenticMessage], error)
}

func (m *recordingModel) Generate(
	context.Context,
	[]*schema.AgenticMessage,
	...model.Option,
) (*schema.AgenticMessage, error) {
	return nil, errors.New("unexpected Generate call")
}

func (m *recordingModel) Stream(
	ctx context.Context,
	input []*schema.AgenticMessage,
	opts ...model.Option,
) (*schema.StreamReader[*schema.AgenticMessage], error) {
	m.mu.Lock()
	call := len(m.inputs)
	m.inputs = append(m.inputs, append([]*schema.AgenticMessage(nil), input...))
	options := model.GetCommonOptions(&model.Options{}, opts...)
	names := make([]string, 0, len(options.Tools))
	for _, tool := range options.Tools {
		names = append(names, tool.Name)
	}
	m.toolNames = append(m.toolNames, names)
	m.mu.Unlock()
	return m.stream(ctx, call, input, opts...)
}

func (m *recordingModel) snapshot() ([][]*schema.AgenticMessage, [][]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inputs := make([][]*schema.AgenticMessage, len(m.inputs))
	for index, input := range m.inputs {
		inputs[index] = append([]*schema.AgenticMessage(nil), input...)
	}
	toolNames := make([][]string, len(m.toolNames))
	for index, names := range m.toolNames {
		toolNames[index] = append([]string(nil), names...)
	}
	return inputs, toolNames
}

func messageStream(messages ...*schema.AgenticMessage) *schema.StreamReader[*schema.AgenticMessage] {
	return schema.StreamReaderFromArray(messages)
}

func assistantText(text string) *schema.AgenticMessage {
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text: text}),
		},
	}
}

func functionCall(name, callID, arguments string) *schema.AgenticMessage {
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

func userPrompt(message *schema.AgenticMessage) string {
	for _, block := range message.ContentBlocks {
		if block != nil && block.UserInputText != nil {
			return block.UserInputText.Text
		}
	}
	return ""
}

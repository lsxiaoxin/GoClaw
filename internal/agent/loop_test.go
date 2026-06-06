package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/lsxiaoxin/GoClaw/internal/hooks"
	"github.com/lsxiaoxin/GoClaw/internal/subagent"
	"github.com/lsxiaoxin/GoClaw/internal/todo"
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

func TestRunnerSuspendsAndResumesApprovedTool(t *testing.T) {
	var writeRuns int
	read := &stubTool{
		info: &schema.ToolInfo{Name: "read_file"},
		safe: true,
		run: func(context.Context, string) (string, error) {
			return "read result", nil
		},
	}
	write := &stubTool{
		info: &schema.ToolInfo{Name: "write_file"},
		run: func(context.Context, string) (string, error) {
			writeRuns++
			return "write result", nil
		},
	}
	agentModel := &sequentialModel{
		responses: [][]*schema.AgenticMessage{
			{
				assistantToolCall("read_file", "call-read", `{"path":"README.md"}`),
				assistantToolCall("write_file", "call-write", `{"path":"note.txt","content":"hello"}`),
			},
			{assistantText("done")},
		},
	}
	runner, err := New(agentModel, 4, mustRegistry(t, read, write))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := runner.Start(context.Background(), "update a file", func(context.Context, string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if result.Status != StatusWaitingApproval || result.Approval.ToolName != "write_file" {
		t.Fatalf("Start() result = %+v", result)
	}
	if writeRuns != 0 {
		t.Fatalf("write runs before approval = %d", writeRuns)
	}
	if len(result.Checkpoint.Messages) != 3 {
		t.Fatalf("checkpoint messages = %d, want user + assistant + read result", len(result.Checkpoint.Messages))
	}

	data, err := json.Marshal(result.Checkpoint)
	if err != nil {
		t.Fatalf("Marshal(checkpoint) error = %v", err)
	}
	var restored Checkpoint
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal(checkpoint) error = %v", err)
	}

	var output string
	result, err = runner.Resume(
		context.Background(),
		&restored,
		DecisionApprove,
		func(_ context.Context, text string) error {
			output += text
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if result.Status != StatusCompleted {
		t.Fatalf("Resume() result = %+v", result)
	}
	if writeRuns != 1 {
		t.Fatalf("write runs after approval = %d", writeRuns)
	}
	if output != "done" {
		t.Fatalf("output = %q", output)
	}
	got := agentModel.inputs[1][3].ContentBlocks[0].FunctionToolResult.Content[0].Text.Text
	if got != "write result" {
		t.Fatalf("approved tool result = %q", got)
	}
}

func TestRunnerResumesDeniedToolWithoutExecuting(t *testing.T) {
	var writeRuns int
	write := &stubTool{
		info: &schema.ToolInfo{Name: "write_file"},
		run: func(context.Context, string) (string, error) {
			writeRuns++
			return "unexpected", nil
		},
	}
	agentModel := &sequentialModel{
		responses: [][]*schema.AgenticMessage{
			{assistantToolCall("write_file", "call-write", `{"path":"note.txt","content":"hello"}`)},
			{assistantText("not written")},
		},
	}
	runner, err := New(agentModel, 4, mustRegistry(t, write))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := runner.Start(context.Background(), "update a file", func(context.Context, string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	result, err = runner.Resume(
		context.Background(),
		result.Checkpoint,
		DecisionDeny,
		func(context.Context, string) error { return nil },
	)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if result.Status != StatusCompleted || writeRuns != 0 {
		t.Fatalf("result = %+v, write runs = %d", result, writeRuns)
	}
	got := agentModel.inputs[1][2].ContentBlocks[0].FunctionToolResult.Content[0].Text.Text
	if got != "Permission denied by user." {
		t.Fatalf("denied tool result = %q", got)
	}
}

func TestRunnerIncludesHookMessagesInToolResultContext(t *testing.T) {
	tool := &stubTool{
		info: &schema.ToolInfo{Name: "bash"},
		run: func(context.Context, string) (string, error) {
			return "/workspace", nil
		},
	}
	registry := mustRegistry(t, tool)
	registry.SetHooks(hooks.NewBus(hooks.Config{Hooks: []hooks.Definition{
		{
			Event:   hooks.PreToolUse,
			Matcher: "bash",
			Builtin: hooks.BuiltinInject,
			Timeout: time.Second,
			Message: "pre saw {{tool}}",
		},
		{
			Event:   hooks.PostToolUse,
			Matcher: "*",
			Builtin: hooks.BuiltinInject,
			Timeout: time.Second,
			Message: "post saw {{output}}",
		},
	}}, nil))
	agentModel := &sequentialModel{
		responses: [][]*schema.AgenticMessage{
			{assistantToolCall("bash", "call-1", `{"command":"pwd"}`)},
			{assistantText("done")},
		},
	}
	runner, err := New(agentModel, 4, registry)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = runner.Run(context.Background(), "where am I?", func(context.Context, string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := agentModel.inputs[1][2].ContentBlocks[0].FunctionToolResult.Content[0].Text.Text
	want := "/workspace\nHook message: pre saw bash\nHook message: post saw /workspace"
	if got != want {
		t.Fatalf("tool result text = %q, want %q", got, want)
	}
}

func TestRunnerHookBlockReturnsToolResultAndSkipsExecution(t *testing.T) {
	var runs int
	tool := &stubTool{
		info: &schema.ToolInfo{Name: "bash"},
		run: func(context.Context, string) (string, error) {
			runs++
			return "unexpected", nil
		},
	}
	registry := mustRegistry(t, tool)
	registry.SetHooks(hooks.NewBus(hooks.Config{Hooks: []hooks.Definition{{
		Event:   hooks.PreToolUse,
		Matcher: "bash",
		Builtin: hooks.BuiltinBlock,
		Timeout: time.Second,
		Message: "bash is disabled",
	}}}, nil))
	agentModel := &sequentialModel{
		responses: [][]*schema.AgenticMessage{
			{assistantToolCall("bash", "call-1", `{"command":"pwd"}`)},
			{assistantText("blocked")},
		},
	}
	runner, err := New(agentModel, 4, registry)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = runner.Run(context.Background(), "run bash", func(context.Context, string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if runs != 0 {
		t.Fatalf("tool runs = %d, want 0", runs)
	}
	got := agentModel.inputs[1][2].ContentBlocks[0].FunctionToolResult.Content[0].Text.Text
	if got != "Hook blocked: bash is disabled" {
		t.Fatalf("tool result text = %q", got)
	}
}

func TestRunnerDoesNotRunHooksForPermissionAskBeforeApproval(t *testing.T) {
	var hookCalls int
	write := &stubTool{
		info: &schema.ToolInfo{Name: "write_file"},
		run: func(context.Context, string) (string, error) {
			return "write result", nil
		},
	}
	registry := mustRegistry(t, write)
	registry.SetHooks(hooks.NewBus(hooks.Config{Hooks: []hooks.Definition{{
		Event:   hooks.PreToolUse,
		Matcher: "write_file",
		Builtin: hooks.BuiltinInject,
		Timeout: time.Second,
		Message: "pre write",
	}}}, &hooks.FakeRunner{
		Wait: func(context.Context, hooks.Definition, hooks.Request) error {
			hookCalls++
			return nil
		},
	}))
	agentModel := &sequentialModel{
		responses: [][]*schema.AgenticMessage{
			{assistantToolCall("write_file", "call-write", `{"path":"note.txt","content":"hello"}`)},
			{assistantText("done")},
		},
	}
	runner, err := New(agentModel, 4, registry)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result, err := runner.Start(context.Background(), "write", func(context.Context, string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if result.Status != StatusWaitingApproval {
		t.Fatalf("Start() result = %+v", result)
	}
	if hookCalls != 0 {
		t.Fatalf("hook calls before approval = %d, want 0", hookCalls)
	}

	result, err = runner.Resume(
		context.Background(),
		result.Checkpoint,
		DecisionApprove,
		func(context.Context, string) error { return nil },
	)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if result.Status != StatusCompleted {
		t.Fatalf("Resume() result = %+v", result)
	}
	if hookCalls != 1 {
		t.Fatalf("hook calls after approval = %d, want 1", hookCalls)
	}
}

func TestRunnerDoesNotRunHooksForPermissionDeny(t *testing.T) {
	var hookCalls int
	var toolRuns int
	bash := &stubTool{
		info: &schema.ToolInfo{Name: "bash"},
		run: func(context.Context, string) (string, error) {
			toolRuns++
			return "unexpected", nil
		},
	}
	registry := mustRegistry(t, bash)
	registry.SetHooks(hooks.NewBus(hooks.Config{Hooks: []hooks.Definition{{
		Event:   hooks.PreToolUse,
		Matcher: "bash",
		Builtin: hooks.BuiltinInject,
		Timeout: time.Second,
		Message: "pre bash",
	}}}, &hooks.FakeRunner{
		Wait: func(context.Context, hooks.Definition, hooks.Request) error {
			hookCalls++
			return nil
		},
	}))
	agentModel := &sequentialModel{
		responses: [][]*schema.AgenticMessage{
			{assistantToolCall("bash", "call-deny", `{"command":"sudo rm file"}`)},
			{assistantText("denied")},
		},
	}
	runner, err := New(agentModel, 4, registry)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = runner.Run(context.Background(), "run denied command", func(context.Context, string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if hookCalls != 0 {
		t.Fatalf("hook calls = %d, want 0", hookCalls)
	}
	if toolRuns != 0 {
		t.Fatalf("tool runs = %d, want 0", toolRuns)
	}
	got := agentModel.inputs[1][2].ContentBlocks[0].FunctionToolResult.Content[0].Text.Text
	if got != "Permission denied: privilege escalation is forbidden" {
		t.Fatalf("tool result text = %q", got)
	}
}

func TestRunnerExecutesTodoWriteThroughHooksAndContext(t *testing.T) {
	todoStore, err := todo.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	todoTool, err := goclawtool.NewTodoWrite(todoStore)
	if err != nil {
		t.Fatalf("NewTodoWrite() error = %v", err)
	}
	registry := mustRegistry(t, todoTool)
	registry.SetHooks(hooks.NewBus(hooks.Config{Hooks: []hooks.Definition{{
		Event:   hooks.PreToolUse,
		Matcher: "todo_write",
		Builtin: hooks.BuiltinInject,
		Timeout: time.Second,
		Message: "pre {{tool}}",
	}}}, nil))
	agentModel := &sequentialModel{
		responses: [][]*schema.AgenticMessage{
			{assistantToolCall("todo_write", "call-todo", `{"items":[{"id":"todo-1","content":"write tests","status":"in_progress","priority":"high"}]}`)},
			{assistantText("todos updated")},
		},
	}
	runner, err := New(agentModel, 4, registry)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := todo.WithChatID(context.Background(), "chat-1")
	err = runner.Run(ctx, "track the work", func(context.Context, string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	items, err := todoStore.Load("chat-1")
	if err != nil {
		t.Fatalf("Load(chat-1) error = %v", err)
	}
	if len(items) != 1 ||
		items[0].ID != "todo-1" ||
		items[0].Status != todo.StatusInProgress ||
		items[0].Priority != todo.PriorityHigh {
		t.Fatalf("items = %+v", items)
	}
	got := agentModel.inputs[1][2].ContentBlocks[0].FunctionToolResult.Content[0].Text.Text
	want := "Updated 1 todos: 0 pending, 1 in_progress, 0 completed\nHook message: pre todo_write"
	if got != want {
		t.Fatalf("todo_write result = %q, want %q", got, want)
	}
}

func TestRunnerInjectsTodoReminderAfterThreeTurnsWithoutUpdate(t *testing.T) {
	agentModel := &sequentialModel{
		repeat: []*schema.AgenticMessage{{
			Role: schema.AgenticRoleTypeAssistant,
		}},
	}
	runner, err := New(agentModel, 4, mustRegistry(t))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = runner.Run(context.Background(), "multi-step task", func(context.Context, string) error {
		return nil
	})
	if !errors.Is(err, ErrMaxSteps) {
		t.Fatalf("Run() error = %v, want ErrMaxSteps", err)
	}
	if len(agentModel.inputs) != 4 {
		t.Fatalf("model calls = %d, want 4", len(agentModel.inputs))
	}
	if !messagesContainText(agentModel.inputs[3], "Todo reminder:") {
		t.Fatalf("fourth model input did not contain todo reminder: %#v", agentModel.inputs[3])
	}
}

func TestRunnerExecutesTaskWithIsolatedSubagentContext(t *testing.T) {
	agentModel := &sequentialModel{
		responses: [][]*schema.AgenticMessage{
			{assistantToolCall("task", "call-task", `{"prompt":"read README","description":"docs"}`)},
			{assistantText("child summary")},
			{assistantText("parent done")},
		},
	}
	childExecutor, err := NewSubagentExecutor(
		agentModel,
		3,
		mustRegistry(t),
		WithSubagentLimits(subagent.Limits{MaxDepth: 1, MaxConcurrent: 1}),
	)
	if err != nil {
		t.Fatalf("NewSubagentExecutor() error = %v", err)
	}
	taskTool, err := goclawtool.NewTask(childExecutor)
	if err != nil {
		t.Fatalf("NewTask() error = %v", err)
	}
	runner, err := New(agentModel, 5, mustRegistry(t, taskTool))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var output string
	err = runner.Run(context.Background(), "delegate work", func(_ context.Context, text string) error {
		output += text
		return nil
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if output != "parent done" {
		t.Fatalf("output = %q", output)
	}
	if len(agentModel.inputs) != 3 {
		t.Fatalf("model calls = %d, want parent, child, parent", len(agentModel.inputs))
	}
	if childPrompt := agentModel.inputs[1][0].ContentBlocks[0].UserInputText.Text; !strings.Contains(childPrompt, "Subagent task: docs") {
		t.Fatalf("child prompt = %q", childPrompt)
	}
	parentFollowup := agentModel.inputs[2]
	if len(parentFollowup) != 3 {
		t.Fatalf("parent followup messages = %d, want 3", len(parentFollowup))
	}
	if messagesContainText(parentFollowup, "Subagent task: docs") ||
		messagesContainText(parentFollowup, "read README") {
		t.Fatalf("parent context leaked child prompt: %#v", parentFollowup)
	}
	got := parentFollowup[2].ContentBlocks[0].FunctionToolResult.Content[0].Text.Text
	if got != "Subagent completed: child summary" {
		t.Fatalf("task result = %q", got)
	}
}

func TestSubagentExecutorRunsChildToolThroughHooks(t *testing.T) {
	read := &stubTool{
		info: &schema.ToolInfo{Name: "read_file"},
		safe: true,
		run: func(context.Context, string) (string, error) {
			return "file content", nil
		},
	}
	childRegistry := mustRegistry(t, read)
	childRegistry.SetHooks(hooks.NewBus(hooks.Config{Hooks: []hooks.Definition{{
		Event:   hooks.PreToolUse,
		Matcher: "read_file",
		Builtin: hooks.BuiltinInject,
		Timeout: time.Second,
		Message: "child hook {{tool}}",
	}}}, nil))
	agentModel := &sequentialModel{
		responses: [][]*schema.AgenticMessage{
			{assistantToolCall("read_file", "call-read", `{"path":"README.md"}`)},
			{assistantText("child saw file")},
		},
	}
	executor, err := NewSubagentExecutor(agentModel, 4, childRegistry)
	if err != nil {
		t.Fatalf("NewSubagentExecutor() error = %v", err)
	}

	result, err := executor.Execute(context.Background(), subagent.Request{Prompt: "inspect README"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != subagent.StatusCompleted || result.Summary != "child saw file" {
		t.Fatalf("result = %+v", result)
	}
	got := agentModel.inputs[1][2].ContentBlocks[0].FunctionToolResult.Content[0].Text.Text
	want := "file content\nHook message: child hook read_file"
	if got != want {
		t.Fatalf("child tool result = %q, want %q", got, want)
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

func (t *stubTool) Validate(string) error {
	return nil
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

func messagesContainText(messages []*schema.AgenticMessage, text string) bool {
	for _, message := range messages {
		for _, block := range message.ContentBlocks {
			if block != nil && block.UserInputText != nil &&
				strings.Contains(block.UserInputText.Text, text) {
				return true
			}
		}
	}
	return false
}

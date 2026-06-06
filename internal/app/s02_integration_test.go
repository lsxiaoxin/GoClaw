package app_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
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

func TestS02FileToolWorkflow(t *testing.T) {
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
				return messageStream(
					functionCall("write_file", "call-write", `{"path":"notes/note.txt","content":"hello"}`),
					functionCall("edit_file", "call-edit", `{"path":"notes/note.txt","old_text":"hello","new_text":"hello s02"}`),
				), nil
			case 1:
				return messageStream(
					functionCall("read_file", "call-read", `{"path":"notes/note.txt"}`),
					functionCall("glob", "call-glob", `{"pattern":"**/*.txt"}`),
				), nil
			case 2:
				return messageStream(assistantText("s02 "), assistantText("completed")), nil
			default:
				return nil, nil
			}
		},
	}
	application, responder := newS02Application(t, workspace, agentModel)

	err := application.Handle(context.Background(), channel.Message{
		EventID:   "event-s02-files",
		MessageID: "message-s02-files",
		ChatID:    "chat-s02",
		Content:   "Create, edit, read, and find a file.",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	responses := waitForClosedResponses(t, responder, 1)
	if got := responses[0].Chunks; !reflect.DeepEqual(got, []string{"s02 ", "completed"}) {
		t.Fatalf("stream chunks = %#v", got)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "notes", "note.txt"))
	if err != nil {
		t.Fatalf("ReadFile(note.txt) error = %v", err)
	}
	if string(data) != "hello s02" {
		t.Fatalf("note.txt = %q", data)
	}

	inputs, toolNames := agentModel.snapshot()
	if len(inputs) != 3 {
		t.Fatalf("model calls = %d, want 3", len(inputs))
	}
	wantTools := []string{"bash", "read_file", "write_file", "edit_file", "glob"}
	if !reflect.DeepEqual(toolNames[0], wantTools) {
		t.Fatalf("model tools = %#v, want %#v", toolNames[0], wantTools)
	}
	assertToolResult(t, inputs[1][2], "call-write", "write_file", "Wrote 5 bytes to notes/note.txt")
	assertToolResult(t, inputs[1][3], "call-edit", "edit_file", "Edited notes/note.txt")
	assertToolResult(t, inputs[2][5], "call-read", "read_file", "hello s02")
	assertToolResult(t, inputs[2][6], "call-glob", "glob", "notes/note.txt")
}

func newS02Application(
	t *testing.T,
	workspace string,
	agentModel model.AgenticModel,
) (*app.App, *fake.Channel) {
	t.Helper()
	bash, err := goclawtool.NewBash(workspace, 5*time.Second, 64*1024)
	if err != nil {
		t.Fatalf("NewBash() error = %v", err)
	}
	readFile, err := goclawtool.NewReadFile(workspace)
	if err != nil {
		t.Fatalf("NewReadFile() error = %v", err)
	}
	writeFile, err := goclawtool.NewWriteFile(workspace)
	if err != nil {
		t.Fatalf("NewWriteFile() error = %v", err)
	}
	editFile, err := goclawtool.NewEditFile(workspace)
	if err != nil {
		t.Fatalf("NewEditFile() error = %v", err)
	}
	glob, err := goclawtool.NewGlob(workspace)
	if err != nil {
		t.Fatalf("NewGlob() error = %v", err)
	}
	registry, err := goclawtool.NewRegistry(bash, readFile, writeFile, editFile, glob)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	runner, err := agent.New(agentModel, 6, registry)
	if err != nil {
		t.Fatalf("agent.New() error = %v", err)
	}
	state, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	responder := fake.New()
	application := app.New(
		context.Background(),
		state,
		responder,
		app.NewRunRegistry(),
		runner,
		workspace,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	return application, responder
}

func assertToolResult(
	t *testing.T,
	message *schema.AgenticMessage,
	callID string,
	name string,
	output string,
) {
	t.Helper()
	if len(message.ContentBlocks) != 1 || message.ContentBlocks[0].FunctionToolResult == nil {
		t.Fatalf("message has no function tool result: %#v", message)
	}
	result := message.ContentBlocks[0].FunctionToolResult
	if result.CallID != callID || result.Name != name {
		t.Fatalf("tool result identity = %s/%s, want %s/%s", result.CallID, result.Name, callID, name)
	}
	if len(result.Content) != 1 || result.Content[0].Text == nil || result.Content[0].Text.Text != output {
		t.Fatalf("tool result output = %#v, want %q", result.Content, output)
	}
}

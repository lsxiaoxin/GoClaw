package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

func TestS03ApprovalSurvivesRestart(t *testing.T) {
	workspace := t.TempDir()
	stateRoot := t.TempDir()
	firstModel := &recordingModel{
		stream: func(
			_ context.Context,
			call int,
			_ []*schema.AgenticMessage,
			_ ...model.Option,
		) (*schema.StreamReader[*schema.AgenticMessage], error) {
			if call != 0 {
				return nil, errors.New("unexpected model call before approval")
			}
			return messageStream(functionCall(
				"write_file",
				"call-write",
				`{"path":"notes/restart.txt","content":"restored"}`,
			)), nil
		},
	}
	firstResponder := fake.New()
	firstState := mustState(t, stateRoot)
	firstApp := newS03Application(t, workspace, firstState, firstResponder, firstModel)

	if err := firstApp.Handle(context.Background(), channel.Message{
		EventID:   "event-s03-start",
		MessageID: "message-s03-start",
		ChatID:    "chat-s03-restart",
		UserID:    "user-1",
		Content:   "Create the restart marker.",
	}); err != nil {
		t.Fatalf("Handle(start) error = %v", err)
	}
	approvals := waitForApprovals(t, firstResponder, 1)
	approvalID := approvals[0].Request.ID
	if approvals[0].Request.ToolName != "write_file" {
		t.Fatalf("approval request = %+v", approvals[0].Request)
	}
	if _, err := os.Stat(filepath.Join(workspace, "notes", "restart.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file exists before approval, stat error = %v", err)
	}
	persisted, err := firstState.LoadApproval("chat-s03-restart")
	if err != nil {
		t.Fatalf("LoadApproval() before restart error = %v", err)
	}
	if persisted.ID != approvalID {
		t.Fatalf("persisted approval ID = %q, want %q", persisted.ID, approvalID)
	}

	secondModel := &recordingModel{
		stream: func(
			_ context.Context,
			call int,
			_ []*schema.AgenticMessage,
			_ ...model.Option,
		) (*schema.StreamReader[*schema.AgenticMessage], error) {
			if call != 0 {
				return nil, errors.New("unexpected model call after approval")
			}
			return messageStream(assistantText("resumed after restart")), nil
		},
	}
	secondResponder := fake.New()
	secondState := mustState(t, stateRoot)
	secondApp := newS03Application(t, workspace, secondState, secondResponder, secondModel)

	if err := secondApp.Handle(context.Background(), channel.Message{
		EventID:   "event-s03-status",
		MessageID: "message-s03-status",
		ChatID:    "chat-s03-restart",
		UserID:    "user-1",
		Content:   "/status",
	}); err != nil {
		t.Fatalf("Handle(status) error = %v", err)
	}
	if err := secondApp.Handle(context.Background(), channel.Message{
		EventID:   "event-s03-wrong-user",
		MessageID: "message-s03-wrong-user",
		ChatID:    "chat-s03-restart",
		UserID:    "user-2",
		Content:   "/approve " + approvalID,
	}); err != nil {
		t.Fatalf("Handle(wrong user) error = %v", err)
	}
	if err := secondApp.Handle(context.Background(), channel.Message{
		EventID:   "event-s03-missing-user",
		MessageID: "message-s03-missing-user",
		ChatID:    "chat-s03-restart",
		Content:   "/approve " + approvalID,
	}); err != nil {
		t.Fatalf("Handle(missing user) error = %v", err)
	}
	if err := secondApp.Handle(context.Background(), channel.Message{
		EventID:   "event-s03-approve",
		MessageID: "message-s03-approve",
		ChatID:    "chat-s03-restart",
		UserID:    "user-1",
		Content:   "/approve " + approvalID,
	}); err != nil {
		t.Fatalf("Handle(approve) error = %v", err)
	}

	responses := waitForClosedResponses(t, secondResponder, 4)
	if got := integrationResponseText(t, responses, "message-s03-status"); !strings.Contains(got, "waiting_approval") {
		t.Fatalf("status response = %q", got)
	}
	if got := integrationResponseText(t, responses, "message-s03-wrong-user"); !strings.Contains(got, "发起任务的用户") {
		t.Fatalf("wrong-user response = %q", got)
	}
	if got := integrationResponseText(t, responses, "message-s03-missing-user"); !strings.Contains(got, "发起任务的用户") {
		t.Fatalf("missing-user response = %q", got)
	}
	if got := integrationResponseText(t, responses, "message-s03-approve"); got != "resumed after restart" {
		t.Fatalf("approved response = %q", got)
	}

	data, err := os.ReadFile(filepath.Join(workspace, "notes", "restart.txt"))
	if err != nil {
		t.Fatalf("ReadFile(restart.txt) error = %v", err)
	}
	if string(data) != "restored" {
		t.Fatalf("restart.txt = %q", data)
	}
	if _, err := secondState.LoadApproval("chat-s03-restart"); !errors.Is(err, store.ErrApprovalNotFound) {
		t.Fatalf("approval remains after resume: %v", err)
	}
	inputs, _ := secondModel.snapshot()
	if len(inputs) != 1 || len(inputs[0]) != 3 {
		t.Fatalf("resumed model inputs = %#v", inputs)
	}
	assertToolResult(t, inputs[0][2], "call-write", "write_file", "Wrote 8 bytes to notes/restart.txt")
}

func TestS03DeniedWriteReturnsToolResultWithoutExecuting(t *testing.T) {
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
					"write_file",
					"call-denied",
					`{"path":"denied.txt","content":"no"}`,
				)), nil
			case 1:
				return messageStream(assistantText("write was denied")), nil
			default:
				return nil, errors.New("unexpected model call")
			}
		},
	}
	responder := fake.New()
	application := newS03Application(
		t,
		workspace,
		mustState(t, t.TempDir()),
		responder,
		agentModel,
	)

	if err := application.Handle(context.Background(), channel.Message{
		EventID:   "event-s03-deny-start",
		MessageID: "message-s03-deny-start",
		ChatID:    "chat-s03-deny",
		UserID:    "user-1",
		Content:   "Write a file.",
	}); err != nil {
		t.Fatalf("Handle(start) error = %v", err)
	}
	approval := waitForApprovals(t, responder, 1)[0]
	if err := application.Handle(context.Background(), channel.Message{
		EventID:   "event-s03-deny",
		MessageID: "message-s03-deny",
		ChatID:    "chat-s03-deny",
		UserID:    "user-1",
		Content:   "/deny " + approval.Request.ID,
	}); err != nil {
		t.Fatalf("Handle(deny) error = %v", err)
	}

	responses := waitForClosedResponses(t, responder, 1)
	if got := integrationResponseText(t, responses, "message-s03-deny"); got != "write was denied" {
		t.Fatalf("denial response = %q", got)
	}
	if _, err := os.Stat(filepath.Join(workspace, "denied.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("denied file exists, stat error = %v", err)
	}
	inputs, _ := agentModel.snapshot()
	assertToolResult(t, inputs[1][2], "call-denied", "write_file", "Permission denied by user.")
}

func TestS03TextApprovalFallbackAndCancel(t *testing.T) {
	workspace := t.TempDir()
	state := mustState(t, t.TempDir())
	base := fake.New()
	responder := &streamOnlyResponder{base: base}
	agentModel := &recordingModel{
		stream: func(
			_ context.Context,
			call int,
			_ []*schema.AgenticMessage,
			_ ...model.Option,
		) (*schema.StreamReader[*schema.AgenticMessage], error) {
			if call != 0 {
				return nil, errors.New("unexpected model call")
			}
			return messageStream(functionCall(
				"bash",
				"call-bash",
				`{"command":"go build ./..."}`,
			)), nil
		},
	}
	application := newS03Application(t, workspace, state, responder, agentModel)

	if err := application.Handle(context.Background(), channel.Message{
		EventID:   "event-s03-text",
		MessageID: "message-s03-text",
		ChatID:    "chat-s03-text",
		UserID:    "local",
		Content:   "Build the project.",
	}); err != nil {
		t.Fatalf("Handle(start) error = %v", err)
	}
	responses := waitForClosedResponses(t, base, 1)
	text := integrationResponseText(t, responses, "message-s03-text")
	if !strings.Contains(text, "/approve ") || !strings.Contains(text, "/deny ") {
		t.Fatalf("text approval fallback = %q", text)
	}

	if err := application.Handle(context.Background(), channel.Message{
		EventID:   "event-s03-text-cancel",
		MessageID: "message-s03-text-cancel",
		ChatID:    "chat-s03-text",
		UserID:    "local",
		Content:   "/cancel",
	}); err != nil {
		t.Fatalf("Handle(cancel) error = %v", err)
	}
	responses = waitForClosedResponses(t, base, 2)
	if got := integrationResponseText(t, responses, "message-s03-text-cancel"); got != "已取消等待审批的任务。" {
		t.Fatalf("cancel response = %q", got)
	}
	if _, err := state.LoadApproval("chat-s03-text"); !errors.Is(err, store.ErrApprovalNotFound) {
		t.Fatalf("approval remains after cancel: %v", err)
	}
}

func newS03Application(
	t *testing.T,
	workspace string,
	state *store.Store,
	responder channel.Responder,
	agentModel model.AgenticModel,
) *app.App {
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
	return app.New(
		context.Background(),
		state,
		responder,
		app.NewRunRegistry(),
		runner,
		workspace,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
}

func mustState(t *testing.T, root string) *store.Store {
	t.Helper()
	state, err := store.New(root)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	return state
}

type streamOnlyResponder struct {
	base *fake.Channel
}

func (r *streamOnlyResponder) Stream(
	ctx context.Context,
	message channel.Message,
	options channel.StreamOptions,
) (channel.Stream, error) {
	return r.base.Stream(ctx, message, options)
}

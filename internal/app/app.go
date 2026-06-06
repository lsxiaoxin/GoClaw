// Package app contains transport-neutral GoClaw application behavior.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/lsxiaoxin/GoClaw/internal/agent"
	"github.com/lsxiaoxin/GoClaw/internal/channel"
	"github.com/lsxiaoxin/GoClaw/internal/contextmgr"
	"github.com/lsxiaoxin/GoClaw/internal/recovery"
	"github.com/lsxiaoxin/GoClaw/internal/store"
	"github.com/lsxiaoxin/GoClaw/internal/todo"
)

// SessionStore is the persistent state used by the command router.
type SessionStore interface {
	RecordEvent(string) (bool, error)
	LoadSession(string) (store.Session, error)
	ResetSession(string) (store.Session, error)
	SaveApproval(store.Approval) error
	LoadApproval(string) (store.Approval, error)
	DeleteApproval(string, string) error
}

// TodoSummaryStore reads per-chat todo counts for status.
type TodoSummaryStore interface {
	Summary(string) (todo.Summary, error)
}

// ContextHistoryStore persists compacted conversation history.
type ContextHistoryStore interface {
	Load(string) (contextmgr.ConversationHistory, error)
	Save(contextmgr.ConversationHistory) error
}

// ContextManager compacts conversation history according to s08 policy.
type ContextManager interface {
	Apply(
		context.Context,
		contextmgr.ConversationHistory,
		[]contextmgr.Message,
		string,
	) (contextmgr.ConversationHistory, bool, error)
}

// AgentRunner executes and resumes Agent requests.
type AgentRunner interface {
	Start(context.Context, string, agent.TextEmitter) (agent.RunResult, error)
	Resume(
		context.Context,
		*agent.Checkpoint,
		agent.ApprovalDecision,
		agent.TextEmitter,
	) (agent.RunResult, error)
}

type contextAgentRunner interface {
	StartWithHistory(
		context.Context,
		[]contextmgr.Message,
		string,
		agent.TextEmitter,
	) (agent.RunResult, error)
}

// App handles normalized inbound messages.
type App struct {
	runContext     context.Context
	store          SessionStore
	responder      channel.Responder
	runs           *RunRegistry
	agent          AgentRunner
	workspace      string
	todos          TodoSummaryStore
	contextStore   ContextHistoryStore
	contextManager ContextManager
	errors         *ErrorRecorder
	logger         *slog.Logger
}

// New creates the GoClaw application.
func New(
	runContext context.Context,
	sessionStore SessionStore,
	responder channel.Responder,
	runs *RunRegistry,
	agentRunner AgentRunner,
	workspace string,
	logger *slog.Logger,
) *App {
	return &App{
		runContext: runContext,
		store:      sessionStore,
		responder:  responder,
		runs:       runs,
		agent:      agentRunner,
		workspace:  workspace,
		errors:     NewErrorRecorder(8),
		logger:     logger,
	}
}

// SetTodoStore enables todo summary reporting in /status.
func (a *App) SetTodoStore(todoStore TodoSummaryStore) {
	a.todos = todoStore
}

// SetContextManager enables per-chat context history persistence and compaction.
func (a *App) SetContextManager(store ContextHistoryStore, manager ContextManager) {
	a.contextStore = store
	a.contextManager = manager
}

// Handle processes one inbound message.
func (a *App) Handle(ctx context.Context, message channel.Message) error {
	eventID := message.EventID
	if eventID == "" {
		eventID = message.MessageID
	}
	recorded, err := a.store.RecordEvent(eventID)
	if err != nil {
		return fmt.Errorf("record inbound event: %w", err)
	}
	if !recorded {
		a.logger.DebugContext(ctx, "ignore duplicate event", "event_id", eventID)
		return nil
	}

	content := strings.TrimSpace(message.Content)
	if !strings.HasPrefix(content, "/") {
		return a.handlePrompt(ctx, message, content)
	}

	fields := strings.Fields(content)
	command := strings.ToLower(fields[0])
	switch command {
	case "/help":
		return a.reply(ctx, message, helpText)
	case "/status":
		return a.handleStatus(ctx, message)
	case "/new":
		return a.handleNew(ctx, message)
	case "/cancel":
		return a.handleCancel(ctx, message)
	case "/approve":
		return a.handleApproval(ctx, message, fields[1:], agent.DecisionApprove)
	case "/deny":
		return a.handleApproval(ctx, message, fields[1:], agent.DecisionDeny)
	default:
		return a.reply(ctx, message, "未知命令。输入 /help 查看可用命令。")
	}
}

func (a *App) handlePrompt(ctx context.Context, message channel.Message, prompt string) error {
	if _, err := a.store.LoadApproval(message.ChatID); err == nil {
		return a.reply(ctx, message, "当前有等待审批的任务。请使用 /approve 或 /deny 处理。")
	} else if !errors.Is(err, store.ErrApprovalNotFound) {
		return fmt.Errorf("load pending approval: %w", err)
	}

	runCtx, finish, err := a.runs.Begin(a.runContext, message.ChatID)
	if err != nil {
		return a.reply(ctx, message, "当前已有运行中的任务。可使用 /cancel 取消。")
	}
	runCtx = todo.WithChatID(runCtx, message.ChatID)
	history, err := a.loadContextHistory(message.ChatID)
	if err != nil {
		finish()
		return fmt.Errorf("load context history: %w", err)
	}
	go a.runAgent(runCtx, finish, message, func(emit agent.TextEmitter) (agent.RunResult, error) {
		return a.startAgent(runCtx, history.Messages, prompt, emit)
	})
	return nil
}

func (a *App) handleApproval(
	ctx context.Context,
	message channel.Message,
	arguments []string,
	decision agent.ApprovalDecision,
) error {
	approval, err := a.store.LoadApproval(message.ChatID)
	if errors.Is(err, store.ErrApprovalNotFound) {
		return a.reply(ctx, message, "当前没有等待审批的任务。")
	}
	if err != nil {
		return fmt.Errorf("load approval: %w", err)
	}
	if len(arguments) > 1 {
		return a.reply(ctx, message, "审批命令格式：/approve [审批ID] 或 /deny [审批ID]。")
	}
	if len(arguments) == 1 && arguments[0] != approval.ID {
		return a.reply(ctx, message, "审批 ID 不匹配。")
	}
	if approval.RequestedBy != "" && approval.RequestedBy != message.UserID {
		return a.reply(ctx, message, "只有发起任务的用户可以处理该审批。")
	}

	var checkpoint agent.Checkpoint
	if err := json.Unmarshal(approval.Checkpoint, &checkpoint); err != nil {
		return fmt.Errorf("decode approval checkpoint: %w", err)
	}
	runCtx, finish, err := a.runs.Begin(a.runContext, message.ChatID)
	if err != nil {
		return a.reply(ctx, message, "当前已有运行中的任务。可使用 /cancel 取消。")
	}
	runCtx = todo.WithChatID(runCtx, message.ChatID)
	if err := a.store.DeleteApproval(message.ChatID, approval.ID); err != nil {
		finish()
		return fmt.Errorf("delete approval before resume: %w", err)
	}

	go a.runAgent(runCtx, finish, message, func(emit agent.TextEmitter) (agent.RunResult, error) {
		return a.agent.Resume(runCtx, &checkpoint, decision, emit)
	})
	return nil
}

func (a *App) runAgent(
	ctx context.Context,
	finish func(),
	message channel.Message,
	run func(agent.TextEmitter) (agent.RunResult, error),
) {
	defer finish()

	writer := &agentReply{
		responder: a.responder,
		message:   message,
	}
	result, err := run(writer.Append)
	if persistErr := a.persistContextHistory(ctx, message.ChatID, result); persistErr != nil {
		a.errors.Record(message.ChatID, persistErr)
		a.logger.WarnContext(ctx, "persist context history", "chat_id", message.ChatID, "error", persistErr)
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err != nil {
		a.errors.Record(message.ChatID, err)
		a.logAgentError(ctx, message, err)
		if appendErr := writer.Append(closeCtx, agentErrorText(err)); appendErr != nil {
			a.errors.Record(message.ChatID, appendErr)
			a.logger.Error("append agent error", "chat_id", message.ChatID, "error", appendErr)
		}
	} else if result.Status == agent.StatusWaitingApproval {
		if err := a.persistAndRequestApproval(closeCtx, writer, message, result); err != nil {
			a.errors.Record(message.ChatID, recovery.Wrap(recovery.StoreError, err))
			a.logger.Error("request tool approval", "chat_id", message.ChatID, "error", err)
			if appendErr := writer.Append(closeCtx, "创建审批失败："+err.Error()); appendErr != nil {
				a.errors.Record(message.ChatID, appendErr)
				a.logger.Error("append approval error", "chat_id", message.ChatID, "error", appendErr)
			}
		}
	}
	if err := writer.Close(closeCtx); err != nil {
		a.errors.Record(message.ChatID, err)
		a.logger.Error("close agent reply stream", "chat_id", message.ChatID, "error", err)
	}
}

func (a *App) persistAndRequestApproval(
	ctx context.Context,
	writer *agentReply,
	message channel.Message,
	result agent.RunResult,
) error {
	if result.Approval == nil || result.Checkpoint == nil {
		return fmt.Errorf("Agent returned incomplete approval state")
	}
	checkpoint, err := json.Marshal(result.Checkpoint)
	if err != nil {
		return fmt.Errorf("encode approval checkpoint: %w", err)
	}
	approvalID, err := newApprovalID()
	if err != nil {
		return err
	}
	approval := store.Approval{
		ID:          approvalID,
		ChatID:      message.ChatID,
		RequestedBy: message.UserID,
		ToolName:    result.Approval.ToolName,
		Arguments:   result.Approval.Arguments,
		Reason:      result.Approval.Reason,
		Checkpoint:  checkpoint,
	}
	if err := a.store.SaveApproval(approval); err != nil {
		return fmt.Errorf("save approval: %w", err)
	}

	request := channel.ApprovalRequest{
		ID:        approval.ID,
		ToolName:  approval.ToolName,
		Arguments: approval.Arguments,
		Reason:    approval.Reason,
	}
	if native, ok := a.responder.(channel.ApprovalResponder); ok {
		if err := native.RequestApproval(ctx, message, request); err == nil {
			return nil
		} else {
			a.logger.Warn("native approval prompt failed; using text fallback", "error", err)
		}
	}
	return writer.Append(ctx, approvalText(request))
}

func (a *App) logAgentError(ctx context.Context, message channel.Message, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	a.logger.ErrorContext(
		ctx,
		"agent run failed",
		"chat_id", message.ChatID,
		"message_id", message.MessageID,
		"error", err,
	)
}

func (a *App) handleStatus(ctx context.Context, message channel.Message) error {
	session, err := a.store.LoadSession(message.ChatID)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	todoSummary := todo.Summary{}
	if a.todos != nil {
		var err error
		todoSummary, err = a.todos.Summary(message.ChatID)
		if err != nil {
			return fmt.Errorf("load todo summary: %w", err)
		}
	}
	status := session.Status
	switch {
	case a.runs.Running(message.ChatID):
		status = "running"
	default:
		if _, err := a.store.LoadApproval(message.ChatID); err == nil {
			status = string(agent.StatusWaitingApproval)
		} else if !errors.Is(err, store.ErrApprovalNotFound) {
			return fmt.Errorf("load approval status: %w", err)
		}
	}
	return a.reply(ctx, message, fmt.Sprintf(
		"状态：%s\n会话代次：%d\n工作区：%s\n阶段：s11-error-recovery\nTodo：total=%d pending=%d in_progress=%d completed=%d\n最近错误：%s",
		status,
		session.Generation,
		a.workspace,
		todoSummary.Total,
		todoSummary.Pending,
		todoSummary.InProgress,
		todoSummary.Completed,
		a.recentErrorSummary(message.ChatID),
	))
}

func (a *App) loadContextHistory(chatID string) (contextmgr.ConversationHistory, error) {
	if a.contextStore == nil {
		return contextmgr.ConversationHistory{SessionID: chatID}, nil
	}
	return a.contextStore.Load(chatID)
}

func (a *App) startAgent(
	ctx context.Context,
	history []contextmgr.Message,
	prompt string,
	emit agent.TextEmitter,
) (agent.RunResult, error) {
	if runner, ok := a.agent.(contextAgentRunner); ok {
		return runner.StartWithHistory(ctx, history, prompt, emit)
	}
	return a.agent.Start(ctx, prompt, emit)
}

func (a *App) persistContextHistory(ctx context.Context, chatID string, result agent.RunResult) error {
	if a.contextStore == nil || result.Checkpoint == nil {
		return nil
	}
	messages := agent.HistoryMessages(result.Checkpoint.Messages)
	if len(messages) == 0 {
		return nil
	}
	history := contextmgr.ConversationHistory{SessionID: chatID}
	todoSummary, err := a.contextTodoSummary(chatID)
	if err != nil {
		return err
	}
	if a.contextManager == nil {
		history.Messages = messages
		history.UpdatedAt = time.Now().UTC()
		return a.contextStore.Save(history)
	}
	compacted, _, err := a.contextManager.Apply(ctx, history, messages, todoSummary)
	if err != nil {
		history.Messages = messages
		history.UpdatedAt = time.Now().UTC()
		if saveErr := a.contextStore.Save(history); saveErr != nil {
			return fmt.Errorf("%w; save uncompacted context: %v", err, saveErr)
		}
		return err
	}
	return a.contextStore.Save(compacted)
}

func (a *App) recentErrorSummary(chatID string) string {
	if a.errors == nil {
		return "none"
	}
	summary := a.errors.Last(chatID)
	if strings.TrimSpace(summary) == "" {
		return "none"
	}
	return summary
}

func (a *App) contextTodoSummary(chatID string) (string, error) {
	if a.todos == nil {
		return "", nil
	}
	summary, err := a.todos.Summary(chatID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"total=%d pending=%d in_progress=%d completed=%d",
		summary.Total,
		summary.Pending,
		summary.InProgress,
		summary.Completed,
	), nil
}

func agentErrorText(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "任务已取消。"
	case errors.Is(err, context.DeadlineExceeded):
		return "任务执行超时。"
	default:
		return "Agent 运行失败：" + err.Error()
	}
}

func (a *App) handleNew(ctx context.Context, message channel.Message) error {
	a.runs.Cancel(message.ChatID)
	session, err := a.store.ResetSession(message.ChatID)
	if err != nil {
		return fmt.Errorf("reset session: %w", err)
	}
	return a.reply(ctx, message, fmt.Sprintf("已创建新会话，当前代次：%d。", session.Generation))
}

func (a *App) handleCancel(ctx context.Context, message channel.Message) error {
	if a.runs.Cancel(message.ChatID) {
		return a.reply(ctx, message, "已取消当前任务。")
	}
	approval, err := a.store.LoadApproval(message.ChatID)
	if errors.Is(err, store.ErrApprovalNotFound) {
		return a.reply(ctx, message, "当前没有运行中或等待审批的任务。")
	}
	if err != nil {
		return fmt.Errorf("load approval for cancellation: %w", err)
	}
	if err := a.store.DeleteApproval(message.ChatID, approval.ID); err != nil {
		return fmt.Errorf("delete approval: %w", err)
	}
	return a.reply(ctx, message, "已取消等待审批的任务。")
}

func (a *App) reply(ctx context.Context, message channel.Message, text string) error {
	stream, err := a.responder.Stream(ctx, message, channel.StreamOptions{Title: "GoClaw"})
	if err != nil {
		return fmt.Errorf("create reply stream: %w", err)
	}
	if err := stream.Append(ctx, text); err != nil {
		_ = stream.Close(ctx)
		return fmt.Errorf("append reply stream: %w", err)
	}
	if err := stream.Close(ctx); err != nil {
		return fmt.Errorf("close reply stream: %w", err)
	}
	return nil
}

func newApprovalID() (string, error) {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("generate approval ID: %w", err)
	}
	return hex.EncodeToString(data[:]), nil
}

func approvalText(request channel.ApprovalRequest) string {
	arguments := truncateText(request.Arguments, 2000)
	return fmt.Sprintf(
		"等待工具审批。\n审批 ID：%s\n原因：%s\n工具：%s\n参数：%s\n"+
			"允许：/approve %s\n拒绝：/deny %s",
		request.ID,
		request.Reason,
		request.ToolName,
		arguments,
		request.ID,
		request.ID,
	)
}

func truncateText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "... (truncated)"
}

type agentReply struct {
	responder channel.Responder
	message   channel.Message
	stream    channel.Stream
}

func (w *agentReply) Append(ctx context.Context, text string) error {
	if w.stream == nil {
		stream, err := w.responder.Stream(ctx, w.message, channel.StreamOptions{Title: "GoClaw"})
		if err != nil {
			return fmt.Errorf("create agent reply stream: %w", err)
		}
		w.stream = stream
	}
	return w.stream.Append(ctx, text)
}

func (w *agentReply) Close(ctx context.Context) error {
	if w.stream == nil {
		return nil
	}
	return w.stream.Close(ctx)
}

const helpText = `GoClaw s03 命令：

/help          查看帮助
/status        查看当前会话状态
/new           重置并创建新会话
/cancel        取消运行中或等待审批的任务
/approve [ID]  允许待审批工具
/deny [ID]     拒绝待审批工具

普通消息会交给模型处理。读工具和明确只读的 bash 自动执行；写文件、编辑文件和
非明确只读的 bash 需要人工审批。`

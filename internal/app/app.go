// Package app contains transport-neutral GoClaw application behavior.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/lsxiaoxin/GoClaw/internal/channel"
	"github.com/lsxiaoxin/GoClaw/internal/store"
)

// SessionStore is the persistent state used by the command router.
type SessionStore interface {
	RecordEvent(string) (bool, error)
	LoadSession(string) (store.Session, error)
	ResetSession(string) (store.Session, error)
}

// AgentRunner executes one user request.
type AgentRunner interface {
	Run(context.Context, string, func(context.Context, string) error) error
}

// App handles normalized inbound messages.
type App struct {
	runContext context.Context
	store      SessionStore
	responder  channel.Responder
	runs       *RunRegistry
	agent      AgentRunner
	workspace  string
	logger     *slog.Logger
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
		logger:     logger,
	}
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

	command := strings.ToLower(strings.Fields(content)[0])
	switch command {
	case "/help":
		return a.reply(ctx, message, helpText)
	case "/status":
		return a.handleStatus(ctx, message)
	case "/new":
		return a.handleNew(ctx, message)
	case "/cancel":
		return a.handleCancel(ctx, message)
	default:
		return a.reply(ctx, message, "未知命令。输入 /help 查看可用命令。")
	}
}

func (a *App) handlePrompt(ctx context.Context, message channel.Message, prompt string) error {
	runCtx, finish, err := a.runs.Begin(a.runContext, message.ChatID)
	if err != nil {
		return a.reply(ctx, message, "当前已有运行中的任务。可使用 /cancel 取消。")
	}

	stream, err := a.responder.Stream(ctx, message, channel.StreamOptions{Title: "GoClaw"})
	if err != nil {
		finish()
		return fmt.Errorf("create agent reply stream: %w", err)
	}
	go a.runAgent(runCtx, finish, stream, message, prompt)
	return nil
}

func (a *App) runAgent(
	ctx context.Context,
	finish func(),
	stream channel.Stream,
	message channel.Message,
	prompt string,
) {
	defer finish()

	wrote := false
	err := a.agent.Run(ctx, prompt, func(ctx context.Context, text string) error {
		if err := stream.Append(ctx, text); err != nil {
			return err
		}
		wrote = true
		return nil
	})

	closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			a.logger.ErrorContext(
				ctx,
				"agent run failed",
				"chat_id", message.ChatID,
				"message_id", message.MessageID,
				"error", err,
			)
		}
		text := agentErrorText(err)
		if wrote {
			text = "\n\n" + text
		}
		if appendErr := stream.Append(closeCtx, text); appendErr != nil {
			a.logger.Error("append agent error", "chat_id", message.ChatID, "error", appendErr)
		}
	}
	if err := stream.Close(closeCtx); err != nil {
		a.logger.Error("close agent reply stream", "chat_id", message.ChatID, "error", err)
	}
}

func (a *App) handleStatus(ctx context.Context, message channel.Message) error {
	session, err := a.store.LoadSession(message.ChatID)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	status := session.Status
	if a.runs.Running(message.ChatID) {
		status = "running"
	}
	return a.reply(ctx, message, fmt.Sprintf(
		"状态：%s\n会话代次：%d\n工作区：%s\n阶段：s02-tool-use",
		status,
		session.Generation,
		a.workspace,
	))
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
	if !a.runs.Cancel(message.ChatID) {
		return a.reply(ctx, message, "当前没有运行中的任务。")
	}
	return a.reply(ctx, message, "已取消当前任务。")
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

const helpText = `GoClaw s02 命令：

/help    查看帮助
/status  查看当前会话状态
/new     重置并创建新会话
/cancel  取消当前运行中的任务

普通消息会交给模型处理；当前提供 bash、read_file、write_file、edit_file 和 glob。`

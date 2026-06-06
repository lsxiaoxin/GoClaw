// Package app contains transport-neutral GoClaw application behavior.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/lsxiaoxin/GoClaw/internal/channel"
	"github.com/lsxiaoxin/GoClaw/internal/store"
)

// SessionStore is the persistent state used by the command router.
type SessionStore interface {
	RecordEvent(string) (bool, error)
	LoadSession(string) (store.Session, error)
	ResetSession(string) (store.Session, error)
}

// App handles normalized inbound messages.
type App struct {
	store     SessionStore
	responder channel.Responder
	runs      *RunRegistry
	workspace string
	logger    *slog.Logger
}

// New creates the s00 application.
func New(
	sessionStore SessionStore,
	responder channel.Responder,
	runs *RunRegistry,
	workspace string,
	logger *slog.Logger,
) *App {
	return &App{
		store:     sessionStore,
		responder: responder,
		runs:      runs,
		workspace: workspace,
		logger:    logger,
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
		return a.reply(ctx, message, "s00 已收到消息。Agent Loop 将在 s01 实现。输入 /help 查看命令。")
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
		"状态：%s\n会话代次：%d\n工作区：%s\n阶段：s00-bootstrap",
		status,
		session.Generation,
		a.workspace,
	))
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

const helpText = `GoClaw s00 命令：

/help    查看帮助
/status  查看当前会话状态
/new     重置并创建新会话
/cancel  取消当前运行中的任务`

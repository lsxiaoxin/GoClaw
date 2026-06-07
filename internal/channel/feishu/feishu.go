// Package feishu adapts the official Feishu Channel SDK to GoClaw.
package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkchannel "github.com/larksuite/oapi-sdk-go/v3/channel"
	larktypes "github.com/larksuite/oapi-sdk-go/v3/channel/types"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/lsxiaoxin/GoClaw/internal/channel"
	"github.com/lsxiaoxin/GoClaw/internal/config"
)

// Channel receives Feishu events over a WebSocket connection.
type Channel struct {
	sdk      larktypes.Channel
	wsClient *larkws.Client
	logger   *slog.Logger
}

// New creates a Feishu channel without opening the network connection.
func New(cfg config.FeishuConfig, logger *slog.Logger) *Channel {
	sdkLogger := slogAdapter{logger: logger}
	client := lark.NewClient(
		cfg.AppID,
		cfg.AppSecret,
		lark.WithLogLevel(larkcore.LogLevelInfo),
		lark.WithLogger(sdkLogger),
	)
	eventDispatcher := dispatcher.NewEventDispatcher("", "")
	wsClient := larkws.NewClient(
		cfg.AppID,
		cfg.AppSecret,
		larkws.WithEventHandler(eventDispatcher),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
		larkws.WithLogger(sdkLogger),
	)
	sdk := larkchannel.NewChannel(client, wsClient)

	requireMention := true
	respondToMentionAll := false
	policy := larktypes.PolicyConfig{
		DMMode:              "allowlist",
		DMAllowlist:         append([]string(nil), cfg.AllowedUserIDs...),
		RequireMention:      &requireMention,
		RespondToMentionAll: &respondToMentionAll,
	}
	if cfg.EnableGroups {
		policy.GroupAllowlist = append([]string(nil), cfg.AllowedGroupIDs...)
	} else {
		// The SDK treats an empty group allowlist as "allow every group".
		policy.GroupAllowlist = []string{"__goclaw_groups_disabled__"}
	}
	sdk.UpdatePolicy(policy)

	return &Channel{
		sdk:      sdk,
		wsClient: wsClient,
		logger:   logger,
	}
}

// Name returns the transport name.
func (c *Channel) Name() string {
	return "feishu"
}

// Start registers handlers and starts the official WebSocket client.
func (c *Channel) Start(ctx context.Context, handler channel.Handler) error {
	c.sdk.OnReady(func() {
		c.logger.Info("Feishu channel ready")
	})
	c.sdk.OnReconnecting(func() {
		c.logger.Warn("Feishu channel reconnecting")
	})
	c.sdk.OnReconnected(func() {
		c.logger.Info("Feishu channel reconnected")
	})
	c.sdk.OnDisconnected(func() {
		c.logger.Warn("Feishu channel disconnected")
	})
	c.sdk.OnError(func(err error) {
		c.logger.Error("Feishu channel error", "error", err)
	})
	c.sdk.OnReject(func(_ context.Context, event *larktypes.RejectEvent) error {
		c.logger.Warn(
			"Feishu message rejected",
			"chat_id", event.ChatID,
			"sender_id", event.SenderID,
			"reason", event.Reason,
		)
		return nil
	})
	c.sdk.OnMessage(func(ctx context.Context, message *larktypes.NormalizedMessage) error {
		eventID := message.EventID
		if eventID == "" {
			eventID = message.MessageID
		}
		return handler(ctx, channel.Message{
			EventID:   eventID,
			MessageID: message.MessageID,
			ChatID:    message.ChatID,
			ChatType:  message.ChatType,
			UserID:    message.UserID,
			Content:   message.Content,
		})
	})
	c.sdk.OnCardAction(func(ctx context.Context, event *larktypes.CardActionEvent) error {
		message, ok := approvalActionMessage(event)
		if !ok {
			return nil
		}
		return handler(ctx, message)
	})
	return c.sdk.Start(ctx)
}

// Stream creates a Feishu markdown reply stream.
func (c *Channel) Stream(ctx context.Context, message channel.Message, opts channel.StreamOptions) (channel.Stream, error) {
	return newFeishuStream(c.sdk, &larktypes.SendInput{
		ChatID:         message.ChatID,
		ReplyMessageID: message.MessageID,
		Title:          opts.Title,
	}), nil
}

// RequestApproval sends an interactive Feishu approval card.
func (c *Channel) RequestApproval(
	ctx context.Context,
	message channel.Message,
	request channel.ApprovalRequest,
) error {
	card, err := approvalCard(request)
	if err != nil {
		return fmt.Errorf("build Feishu approval card: %w", err)
	}
	if _, err := c.sdk.Send(ctx, &larktypes.SendInput{
		ChatID:         message.ChatID,
		ReplyMessageID: message.MessageID,
		Card:           card,
	}); err != nil {
		return fmt.Errorf("send Feishu approval card: %w", err)
	}
	return nil
}

// Close stops the Feishu WebSocket client.
func (c *Channel) Close(ctx context.Context) error {
	return c.sdk.Stop(ctx)
}

type feishuStream struct {
	sdk   larktypes.Channel
	input larktypes.SendInput

	mu      sync.Mutex
	content string
	closed  bool
}

func newFeishuStream(sdk larktypes.Channel, input *larktypes.SendInput) *feishuStream {
	stream := &feishuStream{sdk: sdk}
	if input != nil {
		stream.input = *input
	}
	return stream
}

func (s *feishuStream) Append(ctx context.Context, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("append Feishu reply: stream is closed")
	}
	s.content += text
	return nil
}

func (s *feishuStream) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	input := s.input
	input.Markdown = s.content
	s.mu.Unlock()

	if strings.TrimSpace(input.Markdown) == "" {
		return nil
	}
	if _, err := s.sdk.Send(ctx, &input); err != nil {
		return fmt.Errorf("send Feishu reply: %w", err)
	}
	return nil
}

type slogAdapter struct {
	logger *slog.Logger
}

func approvalActionMessage(event *larktypes.CardActionEvent) (channel.Message, bool) {
	if event == nil {
		return channel.Message{}, false
	}
	action, _ := event.Action.Value["goclaw_action"].(string)
	approvalID, _ := event.Action.Value["approval_id"].(string)
	if approvalID == "" || (action != "approve" && action != "deny") {
		return channel.Message{}, false
	}
	userID := event.Operator.OpenID
	if userID == "" {
		userID = event.Operator.UserID
	}
	eventID := event.EventID
	if eventID == "" {
		eventID = fmt.Sprintf(
			"feishu-card-%s-%s-%s",
			event.MessageID,
			approvalID,
			action,
		)
	}
	return channel.Message{
		EventID:   eventID,
		MessageID: event.MessageID,
		ChatID:    event.ChatID,
		ChatType:  "p2p",
		UserID:    userID,
		Content:   "/" + action + " " + approvalID,
	}, true
}

func approvalCard(request channel.ApprovalRequest) (string, error) {
	arguments := strings.ReplaceAll(request.Arguments, "```", "'''")
	runes := []rune(arguments)
	if len(runes) > 2000 {
		arguments = string(runes[:2000]) + "\n... (truncated)"
	}
	card := map[string]any{
		"config": map[string]any{
			"wide_screen_mode": true,
		},
		"header": map[string]any{
			"template": "orange",
			"title": map[string]any{
				"tag":     "plain_text",
				"content": "GoClaw 工具审批",
			},
		},
		"elements": []any{
			map[string]any{
				"tag": "markdown",
				"content": fmt.Sprintf(
					"**原因**：%s\n**工具**：`%s`\n**参数**：\n```json\n%s\n```",
					request.Reason,
					request.ToolName,
					arguments,
				),
			},
			map[string]any{
				"tag":    "action",
				"layout": "bisected",
				"actions": []any{
					map[string]any{
						"tag":  "button",
						"type": "primary",
						"text": map[string]any{"tag": "plain_text", "content": "允许"},
						"value": map[string]any{
							"goclaw_action": "approve",
							"approval_id":   request.ID,
						},
					},
					map[string]any{
						"tag":  "button",
						"type": "danger",
						"text": map[string]any{"tag": "plain_text", "content": "拒绝"},
						"value": map[string]any{
							"goclaw_action": "deny",
							"approval_id":   request.ID,
						},
					},
				},
			},
			map[string]any{
				"tag": "note",
				"elements": []any{
					map[string]any{
						"tag": "plain_text",
						"content": fmt.Sprintf(
							"后备命令：/approve %s 或 /deny %s",
							request.ID,
							request.ID,
						),
					},
				},
			},
		},
	}
	data, err := json.Marshal(card)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a slogAdapter) Debug(ctx context.Context, args ...interface{}) {
	a.logger.DebugContext(ctx, fmt.Sprint(args...))
}

func (a slogAdapter) Info(ctx context.Context, args ...interface{}) {
	a.logger.InfoContext(ctx, fmt.Sprint(args...))
}

func (a slogAdapter) Warn(ctx context.Context, args ...interface{}) {
	a.logger.WarnContext(ctx, fmt.Sprint(args...))
}

func (a slogAdapter) Error(ctx context.Context, args ...interface{}) {
	a.logger.ErrorContext(ctx, fmt.Sprint(args...))
}

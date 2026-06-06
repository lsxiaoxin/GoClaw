// Package feishu adapts the official Feishu Channel SDK to GoClaw.
package feishu

import (
	"context"
	"fmt"
	"log/slog"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkchannel "github.com/larksuite/oapi-sdk-go/v3/channel"
	larktypes "github.com/larksuite/oapi-sdk-go/v3/channel/types"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/lsxiaoxin/GoClaw/internal/channel"
	"github.com/lsxiaoxin/GoClaw/internal/config"
)

// Channel receives Feishu events over a WebSocket connection.
type Channel struct {
	sdk    larktypes.Channel
	logger *slog.Logger
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
	wsClient := larkws.NewClient(
		cfg.AppID,
		cfg.AppSecret,
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
		sdk:    sdk,
		logger: logger,
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
	return c.sdk.Start(ctx)
}

// Stream creates a Feishu markdown reply stream.
func (c *Channel) Stream(ctx context.Context, message channel.Message, opts channel.StreamOptions) (channel.Stream, error) {
	stream, err := c.sdk.Stream(ctx, &larktypes.SendInput{
		ChatID:         message.ChatID,
		ReplyMessageID: message.MessageID,
		Title:          opts.Title,
	})
	if err != nil {
		return nil, fmt.Errorf("start Feishu reply stream: %w", err)
	}
	return feishuStream{stream: stream}, nil
}

// Close stops the Feishu WebSocket client.
func (c *Channel) Close(ctx context.Context) error {
	return c.sdk.Stop(ctx)
}

type feishuStream struct {
	stream larktypes.StreamController
}

func (s feishuStream) Append(ctx context.Context, text string) error {
	if err := s.stream.Append(ctx, text); err != nil {
		return fmt.Errorf("append Feishu reply: %w", err)
	}
	return nil
}

func (s feishuStream) Close(ctx context.Context) error {
	if err := s.stream.Close(ctx); err != nil {
		return fmt.Errorf("close Feishu reply: %w", err)
	}
	return nil
}

type slogAdapter struct {
	logger *slog.Logger
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

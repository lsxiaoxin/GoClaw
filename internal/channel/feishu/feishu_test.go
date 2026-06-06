package feishu

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	larktypes "github.com/larksuite/oapi-sdk-go/v3/channel/types"

	"github.com/lsxiaoxin/GoClaw/internal/channel"
	"github.com/lsxiaoxin/GoClaw/internal/config"
)

func TestNewConfiguresClosedGroupsByDefault(t *testing.T) {
	transport := New(config.FeishuConfig{
		AppID:          "app-id",
		AppSecret:      "secret",
		AllowedUserIDs: []string{"user-1"},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	policy := transport.sdk.GetPolicy()
	if policy.DMMode != "allowlist" {
		t.Fatalf("DMMode = %q", policy.DMMode)
	}
	if len(policy.DMAllowlist) != 1 || policy.DMAllowlist[0] != "user-1" {
		t.Fatalf("DMAllowlist = %#v", policy.DMAllowlist)
	}
	if len(policy.GroupAllowlist) != 1 || policy.GroupAllowlist[0] != "__goclaw_groups_disabled__" {
		t.Fatalf("GroupAllowlist = %#v", policy.GroupAllowlist)
	}
}

func TestNewConfiguresExplicitGroupAllowlist(t *testing.T) {
	transport := New(config.FeishuConfig{
		AppID:           "app-id",
		AppSecret:       "secret",
		AllowedUserIDs:  []string{"user-1"},
		EnableGroups:    true,
		AllowedGroupIDs: []string{"group-1"},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	policy := transport.sdk.GetPolicy()
	if len(policy.GroupAllowlist) != 1 || policy.GroupAllowlist[0] != "group-1" {
		t.Fatalf("GroupAllowlist = %#v", policy.GroupAllowlist)
	}
	if policy.RequireMention == nil || !*policy.RequireMention {
		t.Fatal("RequireMention must be enabled")
	}
}

func TestRequestApprovalSendsInteractiveCard(t *testing.T) {
	sdk := &recordingSDK{}
	transport := &Channel{
		sdk:    sdk,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	request := channel.ApprovalRequest{
		ID:        "approval-1",
		ToolName:  "write_file",
		Arguments: `{"path":"note.txt","content":"hello"}`,
		Reason:    "write_file modifies workspace files",
	}
	if err := transport.RequestApproval(context.Background(), channel.Message{
		MessageID: "message-1",
		ChatID:    "chat-1",
	}, request); err != nil {
		t.Fatalf("RequestApproval() error = %v", err)
	}
	if sdk.sent == nil || sdk.sent.ChatID != "chat-1" || sdk.sent.ReplyMessageID != "message-1" {
		t.Fatalf("send input = %+v", sdk.sent)
	}
	if !json.Valid([]byte(sdk.sent.Card)) {
		t.Fatalf("card is invalid JSON: %s", sdk.sent.Card)
	}
	for _, value := range []string{
		`"goclaw_action":"approve"`,
		`"goclaw_action":"deny"`,
		`"approval_id":"approval-1"`,
		"/approve approval-1",
	} {
		if !strings.Contains(sdk.sent.Card, value) {
			t.Fatalf("card does not contain %q: %s", value, sdk.sent.Card)
		}
	}
}

func TestApprovalActionMessageConvertsCardButtonToCommand(t *testing.T) {
	message, ok := approvalActionMessage(&larktypes.CardActionEvent{
		EventID:   "event-1",
		MessageID: "message-1",
		ChatID:    "chat-1",
		Operator: larktypes.CardActionOperator{
			OpenID: "user-1",
		},
		Action: larktypes.CardActionPayload{
			Value: map[string]interface{}{
				"goclaw_action": "approve",
				"approval_id":   "approval-1",
			},
		},
	})
	if !ok {
		t.Fatal("approvalActionMessage() rejected valid action")
	}
	if message.EventID != "event-1" || message.ChatID != "chat-1" ||
		message.UserID != "user-1" || message.Content != "/approve approval-1" {
		t.Fatalf("message = %+v", message)
	}

	if _, ok := approvalActionMessage(&larktypes.CardActionEvent{
		Action: larktypes.CardActionPayload{
			Value: map[string]interface{}{"goclaw_action": "unknown"},
		},
	}); ok {
		t.Fatal("approvalActionMessage() accepted unknown action")
	}
}

type recordingSDK struct {
	larktypes.Channel
	sent *larktypes.SendInput
}

func (s *recordingSDK) Send(
	_ context.Context,
	input *larktypes.SendInput,
) (*larktypes.SendResult, error) {
	s.sent = input
	return &larktypes.SendResult{MessageID: "approval-card"}, nil
}

package feishu

import (
	"io"
	"log/slog"
	"testing"

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

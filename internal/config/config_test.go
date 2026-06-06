package config

import (
	"log/slog"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("GOCLAW_WORKSPACE", workspace)
	t.Setenv("GOCLAW_CHANNEL", "")
	t.Setenv("GOCLAW_DATA_DIR", "")
	t.Setenv("GOCLAW_LOG_LEVEL", "")
	t.Setenv("LLM_TIMEOUT", "")
	t.Setenv("FEISHU_ENABLE_GROUPS", "")

	t.Setenv("GOCLAW_CHANNEL", ChannelCLI)
	t.Setenv("GOCLAW_DATA_DIR", ".goclaw")
	t.Setenv("GOCLAW_LOG_LEVEL", "info")
	t.Setenv("LLM_TIMEOUT", "120s")
	t.Setenv("FEISHU_ENABLE_GROUPS", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Channel != ChannelCLI {
		t.Fatalf("Channel = %q, want %q", cfg.Channel, ChannelCLI)
	}
	if cfg.Workspace != workspace {
		t.Fatalf("Workspace = %q, want %q", cfg.Workspace, workspace)
	}
	if cfg.DataDir != filepath.Join(workspace, ".goclaw") {
		t.Fatalf("DataDir = %q", cfg.DataDir)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("LogLevel = %v", cfg.LogLevel)
	}
	if cfg.LLM.Timeout != 120*time.Second {
		t.Fatalf("LLM.Timeout = %v", cfg.LLM.Timeout)
	}
}

func TestLoadFeishuRequiresCredentialsAndAllowlist(t *testing.T) {
	t.Setenv("GOCLAW_WORKSPACE", t.TempDir())
	t.Setenv("GOCLAW_CHANNEL", ChannelFeishu)
	t.Setenv("GOCLAW_DATA_DIR", ".goclaw")
	t.Setenv("GOCLAW_LOG_LEVEL", "info")
	t.Setenv("LLM_TIMEOUT", "120s")
	t.Setenv("FEISHU_ENABLE_GROUPS", "false")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want missing credential error")
	}

	t.Setenv("FEISHU_APP_ID", "app-id")
	t.Setenv("FEISHU_APP_SECRET", "secret")
	t.Setenv("FEISHU_ALLOWED_USER_IDS", "user-a, user-a, user-b")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := len(cfg.Feishu.AllowedUserIDs), 2; got != want {
		t.Fatalf("AllowedUserIDs length = %d, want %d", got, want)
	}
}

func TestLoadFeishuGroupsRequireAllowlist(t *testing.T) {
	t.Setenv("GOCLAW_WORKSPACE", t.TempDir())
	t.Setenv("GOCLAW_CHANNEL", ChannelFeishu)
	t.Setenv("GOCLAW_DATA_DIR", ".goclaw")
	t.Setenv("GOCLAW_LOG_LEVEL", "info")
	t.Setenv("LLM_TIMEOUT", "120s")
	t.Setenv("FEISHU_ENABLE_GROUPS", "true")
	t.Setenv("FEISHU_APP_ID", "app-id")
	t.Setenv("FEISHU_APP_SECRET", "secret")
	t.Setenv("FEISHU_ALLOWED_USER_IDS", "user-a")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want group allowlist error")
	}
}

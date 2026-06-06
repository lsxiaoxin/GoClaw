package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Chdir(t.TempDir())
	workspace := t.TempDir()
	t.Setenv("GOCLAW_WORKSPACE", workspace)
	t.Setenv("GOCLAW_CHANNEL", "")
	t.Setenv("GOCLAW_DATA_DIR", "")
	t.Setenv("GOCLAW_LOG_LEVEL", "")
	t.Setenv("GOCLAW_MAX_STEPS", "")
	t.Setenv("GOCLAW_BASH_TIMEOUT", "")
	t.Setenv("GOCLAW_BASH_OUTPUT_LIMIT", "")
	t.Setenv("GOCLAW_HOOKS_CONFIG", "")
	t.Setenv("LLM_TIMEOUT", "")
	t.Setenv("FEISHU_ENABLE_GROUPS", "")

	t.Setenv("GOCLAW_CHANNEL", ChannelCLI)
	t.Setenv("GOCLAW_DATA_DIR", ".goclaw")
	t.Setenv("GOCLAW_LOG_LEVEL", "info")
	t.Setenv("GOCLAW_MAX_STEPS", "8")
	t.Setenv("GOCLAW_BASH_TIMEOUT", "10s")
	t.Setenv("GOCLAW_BASH_OUTPUT_LIMIT", "65536")
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
	if cfg.Agent.MaxSteps != 8 {
		t.Fatalf("Agent.MaxSteps = %d", cfg.Agent.MaxSteps)
	}
	if cfg.Agent.BashTimeout != 10*time.Second {
		t.Fatalf("Agent.BashTimeout = %v", cfg.Agent.BashTimeout)
	}
	if cfg.Agent.BashOutputLimit != 65536 {
		t.Fatalf("Agent.BashOutputLimit = %d", cfg.Agent.BashOutputLimit)
	}
	if !cfg.Hooks.Empty() {
		t.Fatalf("Hooks = %+v, want empty", cfg.Hooks)
	}
}

func TestLoadReadsDefaultHooksConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	workspace := t.TempDir()
	t.Setenv("GOCLAW_WORKSPACE", workspace)
	t.Setenv("GOCLAW_CHANNEL", ChannelCLI)
	t.Setenv("GOCLAW_DATA_DIR", ".goclaw")
	t.Setenv("GOCLAW_LOG_LEVEL", "info")
	t.Setenv("GOCLAW_MAX_STEPS", "8")
	t.Setenv("GOCLAW_BASH_TIMEOUT", "10s")
	t.Setenv("GOCLAW_BASH_OUTPUT_LIMIT", "65536")
	t.Setenv("GOCLAW_HOOKS_CONFIG", "")
	t.Setenv("LLM_TIMEOUT", "120s")
	t.Setenv("FEISHU_ENABLE_GROUPS", "false")

	hookDir := filepath.Join(workspace, ".goclaw")
	if err := os.MkdirAll(hookDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(.goclaw) error = %v", err)
	}
	content := `{
		"hooks": [
			{
				"event": "PreToolUse",
				"matcher": "bash",
				"builtin": "inject",
				"timeout": "250ms",
				"message": "check {{tool}}"
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(hookDir, "hooks.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(hooks.json) error = %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Hooks.Hooks) != 1 {
		t.Fatalf("hook count = %d, want 1", len(cfg.Hooks.Hooks))
	}
	if got := cfg.Hooks.Hooks[0]; got.Event != "PreToolUse" ||
		got.Matcher != "bash" ||
		got.Builtin != "inject" ||
		got.Message != "check {{tool}}" ||
		got.Timeout != 250*time.Millisecond {
		t.Fatalf("hook = %+v", got)
	}
}

func TestLoadReadsExplicitHooksConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	workspace := t.TempDir()
	t.Setenv("GOCLAW_WORKSPACE", workspace)
	t.Setenv("GOCLAW_CHANNEL", ChannelCLI)
	t.Setenv("GOCLAW_DATA_DIR", ".goclaw")
	t.Setenv("GOCLAW_LOG_LEVEL", "info")
	t.Setenv("GOCLAW_MAX_STEPS", "8")
	t.Setenv("GOCLAW_BASH_TIMEOUT", "10s")
	t.Setenv("GOCLAW_BASH_OUTPUT_LIMIT", "65536")
	t.Setenv("GOCLAW_HOOKS_CONFIG", "config/hooks.json")
	t.Setenv("LLM_TIMEOUT", "120s")
	t.Setenv("FEISHU_ENABLE_GROUPS", "false")

	hookDir := filepath.Join(workspace, "config")
	if err := os.MkdirAll(hookDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(config) error = %v", err)
	}
	content := `{"hooks":[{"event":"PostToolUse","matcher":"*","builtin":"record","message":"recorded"}]}`
	if err := os.WriteFile(filepath.Join(hookDir, "hooks.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(hooks.json) error = %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Hooks.Hooks) != 1 || cfg.Hooks.Hooks[0].Matcher != "*" {
		t.Fatalf("Hooks = %+v", cfg.Hooks)
	}
}

func TestLoadRejectsInvalidAgentLimits(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("GOCLAW_WORKSPACE", t.TempDir())
	t.Setenv("GOCLAW_CHANNEL", ChannelCLI)
	t.Setenv("GOCLAW_DATA_DIR", ".goclaw")
	t.Setenv("GOCLAW_LOG_LEVEL", "info")
	t.Setenv("LLM_TIMEOUT", "120s")
	t.Setenv("FEISHU_ENABLE_GROUPS", "false")
	t.Setenv("GOCLAW_MAX_STEPS", "0")
	t.Setenv("GOCLAW_BASH_TIMEOUT", "10s")
	t.Setenv("GOCLAW_BASH_OUTPUT_LIMIT", "65536")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want max steps error")
	}
}

func TestLoadFeishuRequiresCredentialsAndAllowlist(t *testing.T) {
	t.Chdir(t.TempDir())
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
	t.Chdir(t.TempDir())
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

func TestLoadReadsDotEnvAndPreservesProcessEnvironment(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	workspace := t.TempDir()
	content := "GOCLAW_WORKSPACE=" + workspace + "\n" +
		"GOCLAW_CHANNEL=cli\n" +
		"LLM_API_KEY=file-key\n" +
		"LLM_BASE_URL=https://file.example/v1\n" +
		"LLM_MODEL=file-model\n"
	if err := os.WriteFile(filepath.Join(directory, ".env"), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}
	unsetEnv(t, "GOCLAW_WORKSPACE")
	unsetEnv(t, "GOCLAW_CHANNEL")
	unsetEnv(t, "LLM_API_KEY")
	unsetEnv(t, "LLM_BASE_URL")
	t.Setenv("LLM_MODEL", "process-model")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Workspace != workspace {
		t.Fatalf("Workspace = %q, want %q", cfg.Workspace, workspace)
	}
	if cfg.LLM.APIKey != "file-key" {
		t.Fatalf("LLM.APIKey = %q", cfg.LLM.APIKey)
	}
	if cfg.LLM.BaseURL != "https://file.example/v1" {
		t.Fatalf("LLM.BaseURL = %q", cfg.LLM.BaseURL)
	}
	if cfg.LLM.Model != "process-model" {
		t.Fatalf("LLM.Model = %q, want process environment value", cfg.LLM.Model)
	}
}

func TestLoadRejectsInvalidDotEnv(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	if err := os.WriteFile(filepath.Join(directory, ".env"), []byte("INVALID LINE\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid .env error")
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	value, exists := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("Unsetenv(%q) error = %v", key, err)
	}
	t.Cleanup(func() {
		if exists {
			_ = os.Setenv(key, value)
			return
		}
		_ = os.Unsetenv(key)
	})
}

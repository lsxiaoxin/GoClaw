// Package config loads and validates GoClaw runtime configuration.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	ChannelCLI    = "cli"
	ChannelFeishu = "feishu"
)

// Config contains all runtime configuration.
type Config struct {
	Channel   string
	Workspace string
	DataDir   string
	LogLevel  slog.Level
	LLM       LLMConfig
	Feishu    FeishuConfig
}

// LLMConfig configures an OpenAI-compatible model endpoint.
type LLMConfig struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

// FeishuConfig configures the Feishu WebSocket channel.
type FeishuConfig struct {
	AppID           string
	AppSecret       string
	AllowedUserIDs  []string
	EnableGroups    bool
	AllowedGroupIDs []string
}

// Load reads configuration from environment variables.
func Load() (Config, error) {
	workspace, err := resolveWorkspace(envOrDefault("GOCLAW_WORKSPACE", "."))
	if err != nil {
		return Config{}, err
	}

	channel := strings.ToLower(strings.TrimSpace(envOrDefault("GOCLAW_CHANNEL", ChannelCLI)))
	if channel != ChannelCLI && channel != ChannelFeishu {
		return Config{}, fmt.Errorf("GOCLAW_CHANNEL must be %q or %q", ChannelCLI, ChannelFeishu)
	}

	dataDir := strings.TrimSpace(envOrDefault("GOCLAW_DATA_DIR", ".goclaw"))
	if !filepath.IsAbs(dataDir) {
		dataDir = filepath.Join(workspace, dataDir)
	}
	dataDir, err = filepath.Abs(dataDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve GOCLAW_DATA_DIR: %w", err)
	}

	logLevel, err := parseLogLevel(envOrDefault("GOCLAW_LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}

	timeout, err := parseDuration(envOrDefault("LLM_TIMEOUT", "120s"))
	if err != nil {
		return Config{}, fmt.Errorf("parse LLM_TIMEOUT: %w", err)
	}

	enableGroups, err := parseBool(envOrDefault("FEISHU_ENABLE_GROUPS", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("parse FEISHU_ENABLE_GROUPS: %w", err)
	}

	cfg := Config{
		Channel:   channel,
		Workspace: workspace,
		DataDir:   dataDir,
		LogLevel:  logLevel,
		LLM: LLMConfig{
			APIKey:  strings.TrimSpace(os.Getenv("LLM_API_KEY")),
			BaseURL: strings.TrimSpace(os.Getenv("LLM_BASE_URL")),
			Model:   strings.TrimSpace(os.Getenv("LLM_MODEL")),
			Timeout: timeout,
		},
		Feishu: FeishuConfig{
			AppID:           strings.TrimSpace(os.Getenv("FEISHU_APP_ID")),
			AppSecret:       strings.TrimSpace(os.Getenv("FEISHU_APP_SECRET")),
			AllowedUserIDs:  splitCSV(os.Getenv("FEISHU_ALLOWED_USER_IDS")),
			EnableGroups:    enableGroups,
			AllowedGroupIDs: splitCSV(os.Getenv("FEISHU_ALLOWED_GROUP_IDS")),
		},
	}

	if err := cfg.validateChannel(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validateChannel() error {
	if c.Channel != ChannelFeishu {
		return nil
	}
	if c.Feishu.AppID == "" {
		return fmt.Errorf("FEISHU_APP_ID is required when GOCLAW_CHANNEL=feishu")
	}
	if c.Feishu.AppSecret == "" {
		return fmt.Errorf("FEISHU_APP_SECRET is required when GOCLAW_CHANNEL=feishu")
	}
	if len(c.Feishu.AllowedUserIDs) == 0 {
		return fmt.Errorf("FEISHU_ALLOWED_USER_IDS is required when GOCLAW_CHANNEL=feishu")
	}
	if c.Feishu.EnableGroups && len(c.Feishu.AllowedGroupIDs) == 0 {
		return fmt.Errorf("FEISHU_ALLOWED_GROUP_IDS is required when FEISHU_ENABLE_GROUPS=true")
	}
	return nil
}

func resolveWorkspace(value string) (string, error) {
	workspace, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve GOCLAW_WORKSPACE: %w", err)
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", fmt.Errorf("resolve GOCLAW_WORKSPACE symlinks: %w", err)
	}
	info, err := os.Stat(workspace)
	if err != nil {
		return "", fmt.Errorf("stat GOCLAW_WORKSPACE: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("GOCLAW_WORKSPACE is not a directory: %s", workspace)
	}
	return workspace, nil
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid GOCLAW_LOG_LEVEL %q", value)
	}
}

func parseDuration(value string) (time.Duration, error) {
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0, err
	}
	if duration <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return duration, nil
}

func parseBool(value string) (bool, error) {
	return strconv.ParseBool(strings.TrimSpace(value))
}

func splitCSV(value string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

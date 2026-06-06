package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/lsxiaoxin/GoClaw/internal/agent"
	"github.com/lsxiaoxin/GoClaw/internal/app"
	"github.com/lsxiaoxin/GoClaw/internal/channel"
	channelcli "github.com/lsxiaoxin/GoClaw/internal/channel/cli"
	channelfeishu "github.com/lsxiaoxin/GoClaw/internal/channel/feishu"
	"github.com/lsxiaoxin/GoClaw/internal/config"
	"github.com/lsxiaoxin/GoClaw/internal/llm"
	"github.com/lsxiaoxin/GoClaw/internal/server"
	"github.com/lsxiaoxin/GoClaw/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "goclaw:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	state, err := store.New(cfg.DataDir)
	if err != nil {
		return err
	}
	transport, err := newChannel(cfg, logger)
	if err != nil {
		return err
	}
	agentModel, err := llm.NewOpenAICompatible(ctx, cfg.LLM)
	if err != nil {
		return err
	}
	bashTool, err := agent.NewBashTool(
		cfg.Workspace,
		cfg.Agent.BashTimeout,
		cfg.Agent.BashOutputLimit,
	)
	if err != nil {
		return err
	}
	agentRunner, err := agent.New(agentModel, cfg.Agent.MaxSteps, bashTool)
	if err != nil {
		return err
	}

	application := app.New(
		ctx,
		state,
		transport,
		app.NewRunRegistry(),
		agentRunner,
		cfg.Workspace,
		logger,
	)

	logger.Info(
		"starting GoClaw",
		"stage", "s01-agent-loop",
		"channel", transport.Name(),
		"workspace", cfg.Workspace,
		"data_dir", state.Root(),
		"max_steps", cfg.Agent.MaxSteps,
	)
	return server.Run(ctx, transport, application.Handle, logger)
}

func newChannel(cfg config.Config, logger *slog.Logger) (channel.Channel, error) {
	switch cfg.Channel {
	case config.ChannelCLI:
		return channelcli.New(os.Stdin, os.Stdout, logger), nil
	case config.ChannelFeishu:
		return channelfeishu.New(cfg.Feishu, logger), nil
	default:
		return nil, fmt.Errorf("unsupported channel %q", cfg.Channel)
	}
}

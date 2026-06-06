package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/lsxiaoxin/GoClaw/internal/app"
	"github.com/lsxiaoxin/GoClaw/internal/channel"
	channelcli "github.com/lsxiaoxin/GoClaw/internal/channel/cli"
	channelfeishu "github.com/lsxiaoxin/GoClaw/internal/channel/feishu"
	"github.com/lsxiaoxin/GoClaw/internal/config"
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

	state, err := store.New(cfg.DataDir)
	if err != nil {
		return err
	}
	transport, err := newChannel(cfg, logger)
	if err != nil {
		return err
	}

	application := app.New(
		state,
		transport,
		app.NewRunRegistry(),
		cfg.Workspace,
		logger,
	)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info(
		"starting GoClaw",
		"stage", "s00-bootstrap",
		"channel", transport.Name(),
		"workspace", cfg.Workspace,
		"data_dir", state.Root(),
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

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudwego/eino/components/model"

	"github.com/lsxiaoxin/GoClaw/internal/agent"
	"github.com/lsxiaoxin/GoClaw/internal/app"
	"github.com/lsxiaoxin/GoClaw/internal/channel"
	channelcli "github.com/lsxiaoxin/GoClaw/internal/channel/cli"
	channelfeishu "github.com/lsxiaoxin/GoClaw/internal/channel/feishu"
	"github.com/lsxiaoxin/GoClaw/internal/config"
	"github.com/lsxiaoxin/GoClaw/internal/hooks"
	"github.com/lsxiaoxin/GoClaw/internal/llm"
	"github.com/lsxiaoxin/GoClaw/internal/server"
	"github.com/lsxiaoxin/GoClaw/internal/store"
	"github.com/lsxiaoxin/GoClaw/internal/todo"
	goclawtool "github.com/lsxiaoxin/GoClaw/internal/tool"
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
	todoStore, err := todo.NewStore(cfg.DataDir + "/todos")
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
	tools, err := newToolRegistry(cfg, todoStore, agentModel)
	if err != nil {
		return err
	}
	agentRunner, err := agent.New(agentModel, cfg.Agent.MaxSteps, tools)
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
	application.SetTodoStore(todoStore)

	logger.Info(
		"starting GoClaw",
		"stage", "s06-subagent",
		"channel", transport.Name(),
		"workspace", cfg.Workspace,
		"data_dir", state.Root(),
		"max_steps", cfg.Agent.MaxSteps,
	)
	return server.Run(ctx, transport, application.Handle, logger)
}

func newToolRegistry(
	cfg config.Config,
	todoStore *todo.Store,
	agentModel model.AgenticModel,
) (*goclawtool.Registry, error) {
	bash, err := goclawtool.NewBash(
		cfg.Workspace,
		cfg.Agent.BashTimeout,
		cfg.Agent.BashOutputLimit,
	)
	if err != nil {
		return nil, err
	}
	readFile, err := goclawtool.NewReadFile(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	writeFile, err := goclawtool.NewWriteFile(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	editFile, err := goclawtool.NewEditFile(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	glob, err := goclawtool.NewGlob(cfg.Workspace)
	if err != nil {
		return nil, err
	}
	todoWrite, err := goclawtool.NewTodoWrite(todoStore)
	if err != nil {
		return nil, err
	}
	childRegistry, err := goclawtool.NewRegistry(readFile, glob)
	if err != nil {
		return nil, err
	}
	childRegistry.SetHooks(hooks.NewBus(cfg.Hooks, nil))
	subagents, err := agent.NewSubagentExecutor(agentModel, cfg.Agent.MaxSteps, childRegistry)
	if err != nil {
		return nil, err
	}
	task, err := goclawtool.NewTask(subagents)
	if err != nil {
		return nil, err
	}
	registry, err := goclawtool.NewRegistry(bash, readFile, writeFile, editFile, glob, todoWrite, task)
	if err != nil {
		return nil, err
	}
	registry.SetHooks(hooks.NewBus(cfg.Hooks, nil))
	return registry, nil
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

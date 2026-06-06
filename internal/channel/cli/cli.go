// Package cli implements a local terminal channel.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lsxiaoxin/GoClaw/internal/channel"
)

// Channel reads one message per line and writes replies to the terminal.
type Channel struct {
	input  io.Reader
	output io.Writer
	logger *slog.Logger
	nextID atomic.Uint64
	mu     sync.Mutex
}

// New creates a terminal channel.
func New(input io.Reader, output io.Writer, logger *slog.Logger) *Channel {
	return &Channel{
		input:  input,
		output: output,
		logger: logger,
	}
}

// Name returns the transport name.
func (c *Channel) Name() string {
	return "cli"
}

// Start reads terminal input until EOF or an unrecoverable scanner error.
func (c *Channel) Start(ctx context.Context, handler channel.Handler) error {
	c.writeLine("GoClaw s00 已启动。输入 /help 查看命令。")
	scanner := bufio.NewScanner(c.input)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil
		}
		content := strings.TrimSpace(scanner.Text())
		if content == "" {
			continue
		}
		id := c.nextID.Add(1)
		message := channel.Message{
			EventID:   fmt.Sprintf("cli-event-%d", id),
			MessageID: fmt.Sprintf("cli-message-%d", id),
			ChatID:    "cli",
			ChatType:  "p2p",
			UserID:    "local",
			Content:   content,
		}
		if err := handler(ctx, message); err != nil {
			c.logger.ErrorContext(ctx, "handle CLI message", "error", err)
			c.writeLine("处理失败：" + err.Error())
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read CLI input: %w", err)
	}
	return nil
}

// Stream creates a terminal reply stream.
func (c *Channel) Stream(context.Context, channel.Message, channel.StreamOptions) (channel.Stream, error) {
	c.mu.Lock()
	_, err := fmt.Fprint(c.output, "GoClaw: ")
	c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write CLI reply prefix: %w", err)
	}
	return &stream{channel: c}, nil
}

// Close releases channel resources.
func (c *Channel) Close(context.Context) error {
	return nil
}

func (c *Channel) writeLine(value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := fmt.Fprintln(c.output, value); err != nil {
		c.logger.Error("write CLI output", "error", err)
	}
}

type stream struct {
	channel *Channel
	closed  bool
}

func (s *stream) Append(_ context.Context, text string) error {
	s.channel.mu.Lock()
	defer s.channel.mu.Unlock()
	if s.closed {
		return fmt.Errorf("stream is closed")
	}
	if _, err := fmt.Fprint(s.channel.output, text); err != nil {
		return fmt.Errorf("write CLI stream: %w", err)
	}
	return nil
}

func (s *stream) Close(context.Context) error {
	s.channel.mu.Lock()
	defer s.channel.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if _, err := fmt.Fprintln(s.channel.output); err != nil {
		return fmt.Errorf("close CLI stream: %w", err)
	}
	return nil
}

package cli

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/lsxiaoxin/GoClaw/internal/channel"
)

func TestChannelReadsLinesAndStreamsReplies(t *testing.T) {
	input := strings.NewReader("\n/help\nhello\n")
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cli := New(input, &output, logger)

	var messages []channel.Message
	err := cli.Start(context.Background(), func(ctx context.Context, message channel.Message) error {
		messages = append(messages, message)
		reply, err := cli.Stream(ctx, message, channel.StreamOptions{})
		if err != nil {
			return err
		}
		if err := reply.Append(ctx, "received "+message.Content); err != nil {
			return err
		}
		return reply.Close(ctx)
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(messages))
	}
	if messages[0].EventID == messages[1].EventID {
		t.Fatal("Event IDs must be unique")
	}
	if !strings.Contains(output.String(), "GoClaw: received /help") {
		t.Fatalf("output = %q", output.String())
	}
}

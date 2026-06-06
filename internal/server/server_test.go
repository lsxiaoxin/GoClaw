package server

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/lsxiaoxin/GoClaw/internal/channel"
	"github.com/lsxiaoxin/GoClaw/internal/channel/fake"
)

func TestRunStartsHandlesAndStopsChannel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	transport := fake.New()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	done := make(chan error, 1)

	go func() {
		done <- Run(ctx, transport, func(context.Context, channel.Message) error {
			return nil
		}, logger)
	}()

	readyCtx, readyCancel := context.WithTimeout(context.Background(), time.Second)
	defer readyCancel()
	if err := transport.WaitReady(readyCtx); err != nil {
		t.Fatalf("WaitReady() error = %v", err)
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop")
	}
}

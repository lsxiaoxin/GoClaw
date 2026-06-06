// Package server runs one messaging channel until shutdown.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/lsxiaoxin/GoClaw/internal/channel"
)

// Run starts a channel and closes it when the process context is cancelled.
func Run(ctx context.Context, transport channel.Channel, handler channel.Handler, logger *slog.Logger) error {
	errs := make(chan error, 1)
	go func() {
		errs <- transport.Start(ctx, handler)
	}()

	select {
	case err := <-errs:
		if err != nil {
			_ = closeChannel(transport)
			return fmt.Errorf("start %s channel: %w", transport.Name(), err)
		}
		return closeChannel(transport)
	case <-ctx.Done():
		logger.Info("shutting down channel", "channel", transport.Name())
		return closeChannel(transport)
	}
}

func closeChannel(transport channel.Channel) error {
	closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Close(closeCtx); err != nil {
		return fmt.Errorf("close %s channel: %w", transport.Name(), err)
	}
	return nil
}

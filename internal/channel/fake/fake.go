// Package fake provides an in-memory channel for tests.
package fake

import (
	"context"
	"fmt"
	"sync"

	"github.com/lsxiaoxin/GoClaw/internal/channel"
)

// Response contains one completed fake stream.
type Response struct {
	Message channel.Message
	Title   string
	Chunks  []string
	Closed  bool
}

// Channel is an in-memory channel and responder.
type Channel struct {
	mu        sync.Mutex
	handler   channel.Handler
	ready     chan struct{}
	readyOnce sync.Once
	responses []*Response
}

// New creates a fake channel.
func New() *Channel {
	return &Channel{ready: make(chan struct{})}
}

// Name returns the transport name.
func (c *Channel) Name() string {
	return "fake"
}

// Start registers the handler and waits for cancellation.
func (c *Channel) Start(ctx context.Context, handler channel.Handler) error {
	c.mu.Lock()
	c.handler = handler
	c.mu.Unlock()
	c.readyOnce.Do(func() { close(c.ready) })
	<-ctx.Done()
	return nil
}

// WaitReady waits until Start has installed its handler.
func (c *Channel) WaitReady(ctx context.Context) error {
	select {
	case <-c.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Deliver sends one message to the registered handler.
func (c *Channel) Deliver(ctx context.Context, message channel.Message) error {
	c.mu.Lock()
	handler := c.handler
	c.mu.Unlock()
	if handler == nil {
		return fmt.Errorf("fake channel is not started")
	}
	return handler(ctx, message)
}

// Stream records a new response.
func (c *Channel) Stream(_ context.Context, message channel.Message, opts channel.StreamOptions) (channel.Stream, error) {
	response := &Response{
		Message: message,
		Title:   opts.Title,
	}
	c.mu.Lock()
	c.responses = append(c.responses, response)
	c.mu.Unlock()
	return &stream{channel: c, response: response}, nil
}

// Responses returns a copy of all recorded responses.
func (c *Channel) Responses() []Response {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]Response, len(c.responses))
	for i, response := range c.responses {
		result[i] = Response{
			Message: response.Message,
			Title:   response.Title,
			Chunks:  append([]string(nil), response.Chunks...),
			Closed:  response.Closed,
		}
	}
	return result
}

// Close releases channel resources.
func (c *Channel) Close(context.Context) error {
	return nil
}

type stream struct {
	channel  *Channel
	response *Response
}

func (s *stream) Append(_ context.Context, text string) error {
	s.channel.mu.Lock()
	defer s.channel.mu.Unlock()
	if s.response.Closed {
		return fmt.Errorf("stream is closed")
	}
	s.response.Chunks = append(s.response.Chunks, text)
	return nil
}

func (s *stream) Close(context.Context) error {
	s.channel.mu.Lock()
	defer s.channel.mu.Unlock()
	s.response.Closed = true
	return nil
}

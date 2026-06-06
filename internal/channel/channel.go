// Package channel defines transport-neutral messaging interfaces.
package channel

import "context"

// Message is the normalized inbound message consumed by GoClaw.
type Message struct {
	EventID   string
	MessageID string
	ChatID    string
	ChatType  string
	UserID    string
	Content   string
}

// Handler processes one normalized message.
type Handler func(context.Context, Message) error

// StreamOptions configures an outbound streaming reply.
type StreamOptions struct {
	Title string
}

// Stream receives incremental reply text.
type Stream interface {
	Append(context.Context, string) error
	Close(context.Context) error
}

// Responder creates replies for inbound messages.
type Responder interface {
	Stream(context.Context, Message, StreamOptions) (Stream, error)
}

// ApprovalRequest is a transport-neutral human approval prompt.
type ApprovalRequest struct {
	ID        string
	ToolName  string
	Arguments string
	Reason    string
}

// ApprovalResponder can present a native approval UI such as a Feishu card.
type ApprovalResponder interface {
	RequestApproval(context.Context, Message, ApprovalRequest) error
}

// Channel connects one messaging transport to GoClaw.
type Channel interface {
	Responder
	Name() string
	Start(context.Context, Handler) error
	Close(context.Context) error
}

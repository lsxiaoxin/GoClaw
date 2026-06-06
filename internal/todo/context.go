package todo

import "context"

type chatIDContextKey struct{}

// WithChatID attaches the current chat/session ID to a run context.
func WithChatID(ctx context.Context, chatID string) context.Context {
	return context.WithValue(ctx, chatIDContextKey{}, chatID)
}

// ChatIDFromContext returns the chat/session ID for a tool run.
func ChatIDFromContext(ctx context.Context) (string, bool) {
	chatID, ok := ctx.Value(chatIDContextKey{}).(string)
	return chatID, ok && chatID != ""
}

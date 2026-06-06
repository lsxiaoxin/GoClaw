package subagent

import "context"

type depthContextKey struct{}

// WithDepth records current subagent nesting depth in context.
func WithDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, depthContextKey{}, depth)
}

// DepthFromContext returns the current subagent nesting depth.
func DepthFromContext(ctx context.Context) int {
	depth, ok := ctx.Value(depthContextKey{}).(int)
	if !ok || depth < 0 {
		return 0
	}
	return depth
}

func childContext(ctx context.Context) context.Context {
	return WithDepth(ctx, DepthFromContext(ctx)+1)
}

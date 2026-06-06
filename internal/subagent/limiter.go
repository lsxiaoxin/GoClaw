package subagent

import (
	"context"
	"fmt"
)

// Limiter bounds concurrent child-agent executions.
type Limiter struct {
	slots chan struct{}
}

// NewLimiter creates a concurrency limiter from normalized or raw limits.
func NewLimiter(limits Limits) (*Limiter, Limits, error) {
	normalized, err := limits.Normalize()
	if err != nil {
		return nil, Limits{}, err
	}
	return &Limiter{slots: make(chan struct{}, normalized.MaxConcurrent)}, normalized, nil
}

// Acquire reserves one subagent execution slot until the returned release is called.
func (l *Limiter) Acquire(ctx context.Context) (func(), error) {
	if l == nil {
		return nil, fmt.Errorf("subagent limiter is required")
	}
	select {
	case l.slots <- struct{}{}:
		return func() { <-l.slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

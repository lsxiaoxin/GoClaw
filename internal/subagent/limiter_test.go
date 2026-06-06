package subagent

import (
	"context"
	"testing"
	"time"
)

func TestLimiterBoundsConcurrentExecution(t *testing.T) {
	limiter, _, err := NewLimiter(Limits{MaxDepth: 1, MaxConcurrent: 1})
	if err != nil {
		t.Fatalf("NewLimiter() error = %v", err)
	}

	release, err := limiter.Acquire(context.Background())
	if err != nil {
		t.Fatalf("Acquire(first) error = %v", err)
	}

	acquired := make(chan struct{})
	go func() {
		secondRelease, err := limiter.Acquire(context.Background())
		if err == nil {
			secondRelease()
			close(acquired)
		}
	}()

	select {
	case <-acquired:
		t.Fatal("second acquire completed while first slot was held")
	case <-time.After(20 * time.Millisecond):
	}

	release()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("second acquire did not complete after release")
	}
}

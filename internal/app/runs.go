package app

import (
	"context"
	"fmt"
	"sync"
)

// RunRegistry owns cancellation functions for active chat runs.
type RunRegistry struct {
	mu     sync.Mutex
	nextID uint64
	active map[string]activeRun
}

type activeRun struct {
	id     uint64
	cancel context.CancelFunc
}

// NewRunRegistry creates an empty registry.
func NewRunRegistry() *RunRegistry {
	return &RunRegistry{active: make(map[string]activeRun)}
}

// Begin starts a cancellable run for one chat.
func (r *RunRegistry) Begin(parent context.Context, chatID string) (context.Context, func(), error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.active[chatID]; exists {
		return nil, nil, fmt.Errorf("chat %s already has an active run", chatID)
	}
	ctx, cancel := context.WithCancel(parent)
	r.nextID++
	runID := r.nextID
	r.active[chatID] = activeRun{id: runID, cancel: cancel}
	finish := func() {
		cancel()
		r.mu.Lock()
		defer r.mu.Unlock()
		current, exists := r.active[chatID]
		if exists && current.id == runID {
			delete(r.active, chatID)
		}
	}
	return ctx, finish, nil
}

// Cancel cancels one active run.
func (r *RunRegistry) Cancel(chatID string) bool {
	r.mu.Lock()
	run, exists := r.active[chatID]
	if exists {
		delete(r.active, chatID)
	}
	r.mu.Unlock()
	if exists {
		run.cancel()
	}
	return exists
}

// Running reports whether a chat currently has an active run.
func (r *RunRegistry) Running(chatID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, exists := r.active[chatID]
	return exists
}

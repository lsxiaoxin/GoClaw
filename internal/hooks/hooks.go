// Package hooks implements lifecycle hooks around agent tool execution.
package hooks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const DefaultTimeout = 2 * time.Second

// EventType is one supported hook lifecycle event.
type EventType string

const (
	PreToolUse  EventType = "PreToolUse"
	PostToolUse EventType = "PostToolUse"
)

// BuiltinAction is a safe in-process hook action.
type BuiltinAction string

const (
	BuiltinAllow  BuiltinAction = "allow"
	BuiltinBlock  BuiltinAction = "block"
	BuiltinInject BuiltinAction = "inject"
	BuiltinRecord BuiltinAction = "record"
)

// Definition is one configured hook.
type Definition struct {
	Event   EventType
	Matcher string
	Builtin BuiltinAction
	Command string
	Timeout time.Duration
	Message string
}

// Config contains all configured hooks.
type Config struct {
	Hooks []Definition
}

// Empty reports whether no hooks are configured.
func (c Config) Empty() bool {
	return len(c.Hooks) == 0
}

// Matches reports whether this hook applies to one event and tool name.
func (d Definition) Matches(event EventType, toolName string) bool {
	if d.Event != event {
		return false
	}
	return d.Matcher == "*" || d.Matcher == toolName
}

// Request describes the tool execution state visible to a hook.
type Request struct {
	Event       EventType
	ToolName    string
	Arguments   string
	Output      string
	Error       string
	ElapsedTime time.Duration
}

// Response is returned by one hook runner invocation.
type Response struct {
	Block   bool
	Message string
}

// PreToolUseResult is the aggregate result of matching PreToolUse hooks.
type PreToolUseResult struct {
	Blocked  bool
	Reason   string
	Messages []string
}

// PostToolUseResult is the aggregate result of matching PostToolUse hooks.
type PostToolUseResult struct {
	Messages []string
}

// Runner executes one hook definition.
type Runner interface {
	Run(context.Context, Definition, Request) (Response, error)
}

// Bus dispatches lifecycle events to matching hooks.
type Bus struct {
	config Config
	runner Runner
}

// NewBus creates a hook bus. A nil runner uses the safe builtin runner.
func NewBus(config Config, runner Runner) *Bus {
	if runner == nil {
		runner = BuiltinRunner{}
	}
	hooks := append([]Definition(nil), config.Hooks...)
	return &Bus{config: Config{Hooks: hooks}, runner: runner}
}

// Empty reports whether the bus has no configured hooks.
func (b *Bus) Empty() bool {
	return b == nil || b.config.Empty()
}

// RunPreToolUse runs all matching PreToolUse hooks in configuration order.
func (b *Bus) RunPreToolUse(
	ctx context.Context,
	toolName string,
	arguments string,
) (PreToolUseResult, error) {
	if b.Empty() {
		return PreToolUseResult{}, nil
	}
	var result PreToolUseResult
	request := Request{
		Event:     PreToolUse,
		ToolName:  toolName,
		Arguments: arguments,
	}
	for _, hook := range b.config.Hooks {
		if !hook.Matches(PreToolUse, toolName) {
			continue
		}
		response, err := b.runOne(ctx, hook, request)
		if err != nil {
			if parentCancelled(ctx, err) {
				return result, err
			}
			result.Messages = append(result.Messages, hookFailureMessage(hook, err))
			continue
		}
		if strings.TrimSpace(response.Message) != "" {
			if response.Block {
				result.Blocked = true
				result.Reason = strings.TrimSpace(response.Message)
				return result, nil
			}
			result.Messages = append(result.Messages, strings.TrimSpace(response.Message))
		}
		if response.Block {
			result.Blocked = true
			result.Reason = "blocked by hook"
			return result, nil
		}
	}
	return result, nil
}

// RunPostToolUse runs all matching PostToolUse hooks in configuration order.
func (b *Bus) RunPostToolUse(
	ctx context.Context,
	toolName string,
	arguments string,
	output string,
	err error,
	elapsed time.Duration,
) (PostToolUseResult, error) {
	if b.Empty() {
		return PostToolUseResult{}, nil
	}
	request := Request{
		Event:       PostToolUse,
		ToolName:    toolName,
		Arguments:   arguments,
		Output:      output,
		ElapsedTime: elapsed,
	}
	if err != nil {
		request.Error = err.Error()
	}

	var result PostToolUseResult
	for _, hook := range b.config.Hooks {
		if !hook.Matches(PostToolUse, toolName) {
			continue
		}
		response, runErr := b.runOne(ctx, hook, request)
		if runErr != nil {
			if parentCancelled(ctx, runErr) {
				return result, runErr
			}
			result.Messages = append(result.Messages, hookFailureMessage(hook, runErr))
			continue
		}
		if strings.TrimSpace(response.Message) != "" {
			result.Messages = append(result.Messages, strings.TrimSpace(response.Message))
		}
	}
	return result, nil
}

func (b *Bus) runOne(ctx context.Context, hook Definition, request Request) (Response, error) {
	timeout := hook.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type outcome struct {
		response Response
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		var out outcome
		defer func() {
			if value := recover(); value != nil {
				out.err = fmt.Errorf("hook panic: %v", value)
			}
			done <- out
		}()
		out.response, out.err = b.runner.Run(hookCtx, hook, request)
	}()

	select {
	case out := <-done:
		return out.response, out.err
	case <-hookCtx.Done():
		return Response{}, hookCtx.Err()
	}
}

func parentCancelled(ctx context.Context, err error) bool {
	if ctx.Err() == nil {
		return false
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func hookFailureMessage(hook Definition, err error) string {
	return fmt.Sprintf("Hook %s for %q failed: %v", hook.Event, hook.Matcher, err)
}

// BlockedMessage formats the model-facing tool result for a blocked hook.
func BlockedMessage(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "blocked by hook"
	}
	return "Hook blocked: " + reason
}

package hooks

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// BuiltinRunner executes safe in-process actions and rejects external commands.
type BuiltinRunner struct{}

// Run executes one configured builtin action.
func (BuiltinRunner) Run(ctx context.Context, hook Definition, request Request) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if hook.Command != "" && hook.Builtin == "" {
		return Response{}, fmt.Errorf("external command hooks are not enabled")
	}
	message := renderMessage(hook.Message, request)
	switch hook.Builtin {
	case BuiltinAllow, "":
		return Response{Message: message}, nil
	case BuiltinBlock:
		if message == "" {
			message = fmt.Sprintf("%s blocked %s", hook.Event, request.ToolName)
		}
		return Response{Block: true, Message: message}, nil
	case BuiltinInject, BuiltinRecord:
		return Response{Message: message}, nil
	default:
		return Response{}, fmt.Errorf("unsupported builtin %q", hook.Builtin)
	}
}

func renderMessage(template string, request Request) string {
	template = strings.TrimSpace(template)
	if template == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"{{event}}", string(request.Event),
		"{{tool}}", request.ToolName,
		"{{arguments}}", request.Arguments,
		"{{output}}", request.Output,
		"{{error}}", request.Error,
		"{{elapsed}}", request.ElapsedTime.String(),
	)
	return strings.TrimSpace(replacer.Replace(template))
}

// FakeRunner is a deterministic runner for tests.
type FakeRunner struct {
	mu        sync.Mutex
	Responses map[string]Response
	Errors    map[string]error
	Calls     []Request
	Wait      func(context.Context, Definition, Request) error
}

// Run records the request and returns the configured response.
func (r *FakeRunner) Run(ctx context.Context, hook Definition, request Request) (Response, error) {
	if r.Wait != nil {
		if err := r.Wait(ctx, hook, request); err != nil {
			return Response{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}

	key := fakeKey(hook.Event, hook.Matcher, request.ToolName)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Calls = append(r.Calls, request)
	if r.Errors != nil {
		if err := r.Errors[key]; err != nil {
			return Response{}, err
		}
	}
	if r.Responses != nil {
		if response, ok := r.Responses[key]; ok {
			return response, nil
		}
	}
	return BuiltinRunner{}.Run(ctx, hook, request)
}

// Snapshot returns a copy of recorded calls.
func (r *FakeRunner) Snapshot() []Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Request(nil), r.Calls...)
}

func fakeKey(event EventType, matcher string, toolName string) string {
	return string(event) + ":" + matcher + ":" + toolName
}

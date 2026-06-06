package tool

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/lsxiaoxin/GoClaw/internal/hooks"
	"github.com/lsxiaoxin/GoClaw/internal/permission"
)

func TestRegistryRejectsDuplicateTools(t *testing.T) {
	first := &fakeTool{name: "same"}
	second := &fakeTool{name: "same"}
	if _, err := NewRegistry(first, second); err == nil {
		t.Fatal("NewRegistry() error = nil, want duplicate error")
	}
}

func TestRegistryRunsReadBatchesConcurrentlyAndPreservesOrder(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	var readsFinished atomic.Int32
	var writeStarted atomic.Bool

	read := func(name string) *fakeTool {
		return &fakeTool{
			name: name,
			safe: true,
			run: func(context.Context, string) (string, error) {
				started <- name
				<-release
				readsFinished.Add(1)
				return name + "-result", nil
			},
		}
	}
	write := &fakeTool{
		name: "write",
		run: func(context.Context, string) (string, error) {
			if readsFinished.Load() != 2 {
				t.Error("write started before the read batch completed")
			}
			writeStarted.Store(true)
			return "write-result", nil
		},
	}
	readAfterWrite := &fakeTool{
		name: "read-after",
		safe: true,
		run: func(context.Context, string) (string, error) {
			if !writeStarted.Load() {
				t.Error("read after write started before write completed")
			}
			return "after-result", nil
		},
	}
	registry, err := NewRegistry(read("read-a"), read("read-b"), write, readAfterWrite)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	done := make(chan []Result, 1)
	go func() {
		done <- registry.Execute(context.Background(), []Call{
			{Name: "read-a"},
			{Name: "read-b"},
			{Name: "write"},
			{Name: "read-after"},
		})
	}()

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case name := <-started:
			seen[name] = true
		case <-time.After(time.Second):
			t.Fatal("read tools did not start concurrently")
		}
	}
	close(release)

	var results []Result
	select {
	case results = <-done:
	case <-time.After(time.Second):
		t.Fatal("registry execution did not finish")
	}
	var outputs []string
	for _, result := range results {
		if result.Err != nil {
			t.Fatalf("tool result error = %v", result.Err)
		}
		outputs = append(outputs, result.Output)
	}
	want := []string{"read-a-result", "read-b-result", "write-result", "after-result"}
	if !reflect.DeepEqual(outputs, want) {
		t.Fatalf("outputs = %#v, want %#v", outputs, want)
	}
}

func TestRegistryRunsUnsafeToolsSequentially(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32
	unsafe := func(name string) *fakeTool {
		return &fakeTool{
			name: name,
			run: func(context.Context, string) (string, error) {
				current := active.Add(1)
				defer active.Add(-1)
				for {
					previous := maximum.Load()
					if current <= previous || maximum.CompareAndSwap(previous, current) {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
				return name, nil
			},
		}
	}
	registry, err := NewRegistry(unsafe("write-a"), unsafe("write-b"))
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	results := registry.Execute(context.Background(), []Call{
		{Name: "write-a"},
		{Name: "write-b"},
	})
	if maximum.Load() != 1 {
		t.Fatalf("maximum concurrent unsafe tools = %d, want 1", maximum.Load())
	}
	if results[0].Output != "write-a" || results[1].Output != "write-b" {
		t.Fatalf("results = %#v", results)
	}
}

func TestRegistryReturnsUnknownToolAndCancellationErrors(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := registry.Execute(ctx, []Call{{Name: "missing"}})
	if len(results) != 1 || !errors.Is(results[0].Err, context.Canceled) {
		t.Fatalf("results = %#v", results)
	}

	results = registry.Execute(context.Background(), []Call{{Name: "missing"}})
	if len(results) != 1 || results[0].Err == nil {
		t.Fatalf("results = %#v", results)
	}
}

func TestRegistryPermissionUsesToolValidation(t *testing.T) {
	registry, err := NewRegistry(&fakeTool{
		name: "write_file",
		validate: func(string) error {
			return errors.New("invalid path")
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	decision := registry.Permission(Call{Name: "write_file", Arguments: `{}`})
	if decision.Behavior != permission.Invalid || decision.Reason != "invalid path" {
		t.Fatalf("decision = %+v", decision)
	}

	decision = registry.Permission(Call{Name: "missing", Arguments: `{}`})
	if decision.Behavior != permission.Invalid {
		t.Fatalf("unknown tool decision = %+v", decision)
	}
}

func TestRegistryRunsHooksAroundAllowedTool(t *testing.T) {
	registry, err := NewRegistry(&fakeTool{
		name: "bash",
		run: func(context.Context, string) (string, error) {
			return "tool output", nil
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	registry.SetHooks(hooks.NewBus(hooks.Config{Hooks: []hooks.Definition{
		{
			Event:   hooks.PreToolUse,
			Matcher: "bash",
			Builtin: hooks.BuiltinInject,
			Timeout: time.Second,
			Message: "pre {{tool}}",
		},
		{
			Event:   hooks.PostToolUse,
			Matcher: "*",
			Builtin: hooks.BuiltinInject,
			Timeout: time.Second,
			Message: "post {{tool}} {{output}}",
		},
	}}, nil))

	results := registry.Execute(context.Background(), []Call{{
		Name:      "bash",
		Arguments: `{"command":"pwd"}`,
	}})
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("results = %#v", results)
	}
	if results[0].Output != "tool output" {
		t.Fatalf("output = %q", results[0].Output)
	}
	wantMessages := []string{"pre bash", "post bash tool output"}
	if !reflect.DeepEqual(results[0].HookMessages, wantMessages) {
		t.Fatalf("hook messages = %#v, want %#v", results[0].HookMessages, wantMessages)
	}
}

func TestRegistryHookBlockSkipsToolExecution(t *testing.T) {
	var runs atomic.Int32
	registry, err := NewRegistry(&fakeTool{
		name: "bash",
		run: func(context.Context, string) (string, error) {
			runs.Add(1)
			return "unexpected", nil
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	registry.SetHooks(hooks.NewBus(hooks.Config{Hooks: []hooks.Definition{{
		Event:   hooks.PreToolUse,
		Matcher: "bash",
		Builtin: hooks.BuiltinBlock,
		Timeout: time.Second,
		Message: "blocked by policy",
	}}}, nil))

	results := registry.Execute(context.Background(), []Call{{
		Name:      "bash",
		Arguments: `{"command":"pwd"}`,
	}})
	if runs.Load() != 0 {
		t.Fatalf("tool runs = %d, want 0", runs.Load())
	}
	if len(results) != 1 || results[0].Err != nil ||
		results[0].Output != "Hook blocked: blocked by policy" {
		t.Fatalf("results = %#v", results)
	}
}

func TestRegistryHookFailureDoesNotPanicOrSkipTool(t *testing.T) {
	var runs atomic.Int32
	registry, err := NewRegistry(&fakeTool{
		name: "bash",
		run: func(context.Context, string) (string, error) {
			runs.Add(1)
			return "tool output", nil
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	runner := &hooks.FakeRunner{
		Errors: map[string]error{
			"PreToolUse:bash:bash": errors.New("hook failed"),
		},
	}
	registry.SetHooks(hooks.NewBus(hooks.Config{Hooks: []hooks.Definition{{
		Event:   hooks.PreToolUse,
		Matcher: "bash",
		Builtin: hooks.BuiltinInject,
		Timeout: time.Second,
	}}}, runner))

	results := registry.Execute(context.Background(), []Call{{
		Name:      "bash",
		Arguments: `{"command":"pwd"}`,
	}})
	if runs.Load() != 1 {
		t.Fatalf("tool runs = %d, want 1", runs.Load())
	}
	if len(results) != 1 || results[0].Err != nil || results[0].Output != "tool output" {
		t.Fatalf("results = %#v", results)
	}
	if len(results[0].HookMessages) != 1 ||
		!strings.Contains(results[0].HookMessages[0], "hook failed") {
		t.Fatalf("hook messages = %#v", results[0].HookMessages)
	}
}

type fakeTool struct {
	name     string
	safe     bool
	validate func(string) error
	run      func(context.Context, string) (string, error)
}

func (t *fakeTool) Info() *schema.ToolInfo {
	return &schema.ToolInfo{Name: t.name}
}

func (t *fakeTool) ConcurrencySafe() bool {
	return t.safe
}

func (t *fakeTool) Validate(arguments string) error {
	if t.validate == nil {
		return nil
	}
	return t.validate(arguments)
}

func (t *fakeTool) Run(ctx context.Context, arguments string) (string, error) {
	if t.run == nil {
		return "", nil
	}
	return t.run(ctx, arguments)
}

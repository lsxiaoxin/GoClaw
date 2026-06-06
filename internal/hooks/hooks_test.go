package hooks

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseConfig(t *testing.T) {
	config, err := ParseConfig([]byte(`{
		"hooks": [
			{
				"event": "PreToolUse",
				"matcher": "bash",
				"builtin": "inject",
				"timeout": "50ms",
				"message": "about to run {{tool}}"
			},
			{
				"event": "PostToolUse",
				"matcher": "*",
				"command": "./hooks/audit.sh",
				"timeout": "1s"
			}
		]
	}`))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	if len(config.Hooks) != 2 {
		t.Fatalf("hook count = %d, want 2", len(config.Hooks))
	}
	if got := config.Hooks[0]; got.Event != PreToolUse ||
		got.Matcher != "bash" ||
		got.Builtin != BuiltinInject ||
		got.Timeout != 50*time.Millisecond ||
		got.Message != "about to run {{tool}}" {
		t.Fatalf("first hook = %+v", got)
	}
	if got := config.Hooks[1]; got.Event != PostToolUse ||
		got.Matcher != "*" ||
		got.Command != "./hooks/audit.sh" ||
		got.Timeout != time.Second {
		t.Fatalf("second hook = %+v", got)
	}
}

func TestParseConfigRejectsInvalidDefinitions(t *testing.T) {
	tests := []string{
		`{"hooks":[{"event":"BeforeTool","matcher":"bash","builtin":"allow"}]}`,
		`{"hooks":[{"event":"PreToolUse","matcher":"","builtin":"allow"}]}`,
		`{"hooks":[{"event":"PreToolUse","matcher":"bad matcher","builtin":"allow"}]}`,
		`{"hooks":[{"event":"PreToolUse","matcher":"bash"}]}`,
		`{"hooks":[{"event":"PreToolUse","matcher":"bash","builtin":"allow","command":"./hook.sh"}]}`,
		`{"hooks":[{"event":"PreToolUse","matcher":"bash","builtin":"unknown"}]}`,
		`{"hooks":[{"event":"PreToolUse","matcher":"bash","builtin":"allow","timeout":"0s"}]}`,
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			if _, err := ParseConfig([]byte(input)); err == nil {
				t.Fatal("ParseConfig() error = nil")
			}
		})
	}
}

func TestHookMatchingSupportsExactAndWildcard(t *testing.T) {
	exact := Definition{Event: PreToolUse, Matcher: "bash"}
	if !exact.Matches(PreToolUse, "bash") {
		t.Fatal("exact matcher did not match tool")
	}
	if exact.Matches(PreToolUse, "read_file") {
		t.Fatal("exact matcher matched different tool")
	}
	if exact.Matches(PostToolUse, "bash") {
		t.Fatal("exact matcher matched different event")
	}

	wildcard := Definition{Event: PostToolUse, Matcher: "*"}
	if !wildcard.Matches(PostToolUse, "bash") || !wildcard.Matches(PostToolUse, "read_file") {
		t.Fatal("wildcard did not match all tools")
	}
}

func TestPreToolUseAllowsBlocksAndInjects(t *testing.T) {
	t.Run("allow", func(t *testing.T) {
		bus := NewBus(Config{Hooks: []Definition{{
			Event:   PreToolUse,
			Matcher: "bash",
			Builtin: BuiltinAllow,
			Timeout: time.Second,
		}}}, nil)
		result, err := bus.RunPreToolUse(context.Background(), "bash", `{"command":"pwd"}`)
		if err != nil {
			t.Fatalf("RunPreToolUse() error = %v", err)
		}
		if result.Blocked || len(result.Messages) != 0 {
			t.Fatalf("result = %+v", result)
		}
	})

	t.Run("block", func(t *testing.T) {
		bus := NewBus(Config{Hooks: []Definition{{
			Event:   PreToolUse,
			Matcher: "bash",
			Builtin: BuiltinBlock,
			Timeout: time.Second,
			Message: "bash disabled for this workspace",
		}}}, nil)
		result, err := bus.RunPreToolUse(context.Background(), "bash", `{"command":"pwd"}`)
		if err != nil {
			t.Fatalf("RunPreToolUse() error = %v", err)
		}
		if !result.Blocked || result.Reason != "bash disabled for this workspace" {
			t.Fatalf("result = %+v", result)
		}
		if got := BlockedMessage(result.Reason); got != "Hook blocked: bash disabled for this workspace" {
			t.Fatalf("BlockedMessage() = %q", got)
		}
	})

	t.Run("inject", func(t *testing.T) {
		bus := NewBus(Config{Hooks: []Definition{{
			Event:   PreToolUse,
			Matcher: "bash",
			Builtin: BuiltinInject,
			Timeout: time.Second,
			Message: "inspect {{tool}} {{arguments}}",
		}}}, nil)
		result, err := bus.RunPreToolUse(context.Background(), "bash", `{"command":"pwd"}`)
		if err != nil {
			t.Fatalf("RunPreToolUse() error = %v", err)
		}
		if result.Blocked || len(result.Messages) != 1 ||
			result.Messages[0] != `inspect bash {"command":"pwd"}` {
			t.Fatalf("result = %+v", result)
		}
	})
}

func TestPostToolUseInjectsMessage(t *testing.T) {
	bus := NewBus(Config{Hooks: []Definition{{
		Event:   PostToolUse,
		Matcher: "*",
		Builtin: BuiltinInject,
		Timeout: time.Second,
		Message: "observed {{tool}} output={{output}} error={{error}}",
	}}}, nil)
	result, err := bus.RunPostToolUse(
		context.Background(),
		"read_file",
		`{"path":"README.md"}`,
		"hello",
		errors.New("read failed"),
		10*time.Millisecond,
	)
	if err != nil {
		t.Fatalf("RunPostToolUse() error = %v", err)
	}
	if len(result.Messages) != 1 ||
		result.Messages[0] != "observed read_file output=hello error=read failed" {
		t.Fatalf("messages = %#v", result.Messages)
	}
}

func TestHookTimeoutAndExecutionErrorAreMessages(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		runner := &FakeRunner{
			Wait: func(ctx context.Context, _ Definition, _ Request) error {
				<-ctx.Done()
				return ctx.Err()
			},
		}
		bus := NewBus(Config{Hooks: []Definition{{
			Event:   PreToolUse,
			Matcher: "bash",
			Builtin: BuiltinAllow,
			Timeout: time.Millisecond,
		}}}, runner)
		result, err := bus.RunPreToolUse(context.Background(), "bash", `{}`)
		if err != nil {
			t.Fatalf("RunPreToolUse() error = %v", err)
		}
		if result.Blocked || len(result.Messages) != 1 ||
			!strings.Contains(result.Messages[0], "context deadline exceeded") {
			t.Fatalf("result = %+v", result)
		}
	})

	t.Run("error", func(t *testing.T) {
		runner := &FakeRunner{
			Errors: map[string]error{
				fakeKey(PreToolUse, "bash", "bash"): errors.New("hook failed"),
			},
		}
		bus := NewBus(Config{Hooks: []Definition{{
			Event:   PreToolUse,
			Matcher: "bash",
			Builtin: BuiltinAllow,
			Timeout: time.Second,
		}}}, runner)
		result, err := bus.RunPreToolUse(context.Background(), "bash", `{}`)
		if err != nil {
			t.Fatalf("RunPreToolUse() error = %v", err)
		}
		if len(result.Messages) != 1 || !strings.Contains(result.Messages[0], "hook failed") {
			t.Fatalf("result = %+v", result)
		}
	})
}

func TestBuiltinRunnerRejectsExternalCommandHook(t *testing.T) {
	_, err := BuiltinRunner{}.Run(context.Background(), Definition{
		Event:   PreToolUse,
		Matcher: "bash",
		Command: "./hooks/pre.sh",
	}, Request{Event: PreToolUse, ToolName: "bash"})
	if err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("Run() error = %v", err)
	}
}

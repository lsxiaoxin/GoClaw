package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// ErrConfigNotFound reports that no hook configuration file exists.
var ErrConfigNotFound = errors.New("hook config not found")

type fileConfig struct {
	Hooks []fileDefinition `json:"hooks"`
}

type fileDefinition struct {
	Event   string `json:"event"`
	Matcher string `json:"matcher"`
	Builtin string `json:"builtin,omitempty"`
	Command string `json:"command,omitempty"`
	Timeout string `json:"timeout,omitempty"`
	Message string `json:"message,omitempty"`
}

// LoadFile loads hooks from a JSON configuration file.
func LoadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, ErrConfigNotFound
	}
	if err != nil {
		return Config{}, fmt.Errorf("read hook config: %w", err)
	}
	return ParseConfig(data)
}

// ParseConfig parses and validates hook JSON.
func ParseConfig(data []byte) (Config, error) {
	var raw fileConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("decode hook config: %w", err)
	}
	config := Config{Hooks: make([]Definition, 0, len(raw.Hooks))}
	for index, hook := range raw.Hooks {
		parsed, err := parseDefinition(hook)
		if err != nil {
			return Config{}, fmt.Errorf("hook %d: %w", index, err)
		}
		config.Hooks = append(config.Hooks, parsed)
	}
	return config, nil
}

func parseDefinition(raw fileDefinition) (Definition, error) {
	event := EventType(strings.TrimSpace(raw.Event))
	switch event {
	case PreToolUse, PostToolUse:
	default:
		return Definition{}, fmt.Errorf("unsupported event %q", raw.Event)
	}

	matcher := strings.TrimSpace(raw.Matcher)
	if matcher == "" {
		return Definition{}, fmt.Errorf("matcher is required")
	}
	if matcher != "*" && strings.ContainsAny(matcher, " \t\r\n") {
		return Definition{}, fmt.Errorf("matcher must be a tool name or *")
	}

	builtin := BuiltinAction(strings.TrimSpace(raw.Builtin))
	command := strings.TrimSpace(raw.Command)
	if builtin == "" && command == "" {
		return Definition{}, fmt.Errorf("builtin or command is required")
	}
	if builtin != "" && command != "" {
		return Definition{}, fmt.Errorf("builtin and command cannot both be set")
	}
	if builtin != "" {
		switch builtin {
		case BuiltinAllow, BuiltinBlock, BuiltinInject, BuiltinRecord:
		default:
			return Definition{}, fmt.Errorf("unsupported builtin %q", raw.Builtin)
		}
	}

	timeout := DefaultTimeout
	if strings.TrimSpace(raw.Timeout) != "" {
		parsed, err := time.ParseDuration(strings.TrimSpace(raw.Timeout))
		if err != nil {
			return Definition{}, fmt.Errorf("parse timeout: %w", err)
		}
		if parsed <= 0 {
			return Definition{}, fmt.Errorf("timeout must be positive")
		}
		timeout = parsed
	}

	return Definition{
		Event:   event,
		Matcher: matcher,
		Builtin: builtin,
		Command: command,
		Timeout: timeout,
		Message: strings.TrimSpace(raw.Message),
	}, nil
}

package llm

import (
	"context"
	"testing"
	"time"

	"github.com/lsxiaoxin/GoClaw/internal/config"
)

func TestNewOpenAICompatibleValidatesConfiguration(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.LLMConfig
	}{
		{name: "missing key", cfg: config.LLMConfig{Model: "model"}},
		{name: "missing model", cfg: config.LLMConfig{APIKey: "key"}},
		{
			name: "invalid base URL",
			cfg: config.LLMConfig{
				APIKey:  "key",
				Model:   "model",
				BaseURL: "localhost:8000",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewOpenAICompatible(context.Background(), tt.cfg); err == nil {
				t.Fatal("NewOpenAICompatible() error = nil")
			}
		})
	}
}

func TestNewOpenAICompatibleDoesNotCallNetwork(t *testing.T) {
	model, err := NewOpenAICompatible(context.Background(), config.LLMConfig{
		APIKey:  "test-key",
		BaseURL: "http://127.0.0.1:1/v1",
		Model:   "test-model",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatible() error = %v", err)
	}
	if model == nil {
		t.Fatal("NewOpenAICompatible() model = nil")
	}
}

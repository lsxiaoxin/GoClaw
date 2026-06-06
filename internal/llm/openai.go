// Package llm creates model clients used by GoClaw.
package llm

import (
	"context"
	"fmt"
	"net/url"

	"github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino/components/model"

	"github.com/lsxiaoxin/GoClaw/internal/config"
)

// NewOpenAICompatible creates an Eino AgenticModel without making a network request.
func NewOpenAICompatible(ctx context.Context, cfg config.LLMConfig) (model.AgenticModel, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("LLM_API_KEY is required")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("LLM_MODEL is required")
	}
	if cfg.BaseURL != "" {
		parsed, err := url.ParseRequestURI(cfg.BaseURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("LLM_BASE_URL must be an absolute HTTP URL")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return nil, fmt.Errorf("LLM_BASE_URL must use http or https")
		}
	}

	return agenticopenai.NewChatModel(ctx, &agenticopenai.ChatConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		Timeout: cfg.Timeout,
	})
}

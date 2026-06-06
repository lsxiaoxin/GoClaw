package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/lsxiaoxin/GoClaw/internal/memory"
)

// MemoryRead returns selected long-term memories.
type MemoryRead struct {
	store *memory.Store
	info  *schema.ToolInfo
}

// NewMemoryRead creates the memory_read tool.
func NewMemoryRead(store *memory.Store) (*MemoryRead, error) {
	if store == nil {
		return nil, fmt.Errorf("memory store is required")
	}
	return &MemoryRead{
		store: store,
		info: &schema.ToolInfo{
			Name: "memory_read",
			Desc: "Read selected long-term memories by keyword and optional category.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"query": {
					Type:     schema.String,
					Desc:     "Optional keywords for memory selection.",
					Required: false,
				},
				"category": {
					Type:     schema.String,
					Desc:     "Optional category: user, feedback, project, or reference.",
					Required: false,
				},
				"limit": {
					Type:     schema.Integer,
					Desc:     "Maximum memories to return.",
					Required: false,
				},
			}),
		},
	}, nil
}

func (t *MemoryRead) Info() *schema.ToolInfo { return t.info }

func (t *MemoryRead) ConcurrencySafe() bool { return true }

func (t *MemoryRead) Validate(arguments string) error {
	_, err := memoryReadInputFrom(arguments)
	return err
}

func (t *MemoryRead) Run(ctx context.Context, arguments string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	input, err := memoryReadInputFrom(arguments)
	if err != nil {
		return "", err
	}
	var entries []memory.Entry
	if input.Category != "" {
		loaded, err := t.store.Load(input.Category)
		if err != nil {
			return "", err
		}
		entries = memory.Select(loaded, input.Query, input.Limit)
	} else {
		entries, err = t.store.Select(input.Query, input.Limit)
		if err != nil {
			return "", err
		}
	}
	if len(entries) == 0 {
		return "No matching memory.", nil
	}
	return memory.FormatPrompt(entries), nil
}

// MemoryWrite creates or updates one long-term memory entry.
type MemoryWrite struct {
	store *memory.Store
	info  *schema.ToolInfo
}

// NewMemoryWrite creates the memory_write tool.
func NewMemoryWrite(store *memory.Store) (*MemoryWrite, error) {
	if store == nil {
		return nil, fmt.Errorf("memory store is required")
	}
	return &MemoryWrite{
		store: store,
		info: &schema.ToolInfo{
			Name: "memory_write",
			Desc: "Write long-term memory after user approval. Do not store secrets, tokens, passwords, exact addresses, or identity numbers.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"category": {
					Type:     schema.String,
					Desc:     "Memory category: user, feedback, project, or reference.",
					Required: true,
				},
				"content": {
					Type:     schema.String,
					Desc:     "Memory content to persist.",
					Required: true,
				},
				"id": {
					Type:     schema.String,
					Desc:     "Optional stable memory ID to update.",
					Required: false,
				},
			}),
		},
	}, nil
}

func (t *MemoryWrite) Info() *schema.ToolInfo { return t.info }

func (t *MemoryWrite) ConcurrencySafe() bool { return false }

func (t *MemoryWrite) Validate(arguments string) error {
	input, err := memoryWriteInputFrom(arguments)
	if err != nil {
		return err
	}
	if sensitive := memory.DetectSensitive(input.Content); sensitive.Sensitive {
		return fmt.Errorf("memory content may contain sensitive information: %s", strings.Join(sensitive.Reasons, ", "))
	}
	return nil
}

func (t *MemoryWrite) Run(ctx context.Context, arguments string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	input, err := memoryWriteInputFrom(arguments)
	if err != nil {
		return "", err
	}
	if sensitive := memory.DetectSensitive(input.Content); sensitive.Sensitive {
		return "", fmt.Errorf("memory content may contain sensitive information: %s", strings.Join(sensitive.Reasons, ", "))
	}
	entry, err := t.store.Upsert(memory.Entry{
		ID:       input.ID,
		Category: input.Category,
		Content:  input.Content,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Memory saved: %s/%s", entry.Category, entry.ID), nil
}

type memoryReadInput struct {
	Query    string
	Category memory.Category
	Limit    int
}

func memoryReadInputFrom(arguments string) (memoryReadInput, error) {
	var input struct {
		Query    string `json:"query"`
		Category string `json:"category"`
		Limit    int    `json:"limit"`
	}
	if err := decodeArguments(arguments, &input); err != nil {
		return memoryReadInput{}, err
	}
	if input.Limit < 0 {
		return memoryReadInput{}, fmt.Errorf("limit must be non-negative")
	}
	parsed := memoryReadInput{
		Query: strings.TrimSpace(input.Query),
		Limit: input.Limit,
	}
	if strings.TrimSpace(input.Category) != "" {
		category, err := memory.NormalizeCategory(input.Category)
		if err != nil {
			return memoryReadInput{}, err
		}
		parsed.Category = category
	}
	return parsed, nil
}

type memoryWriteInput struct {
	ID       string
	Category memory.Category
	Content  string
}

func memoryWriteInputFrom(arguments string) (memoryWriteInput, error) {
	var input struct {
		ID       string `json:"id"`
		Category string `json:"category"`
		Content  string `json:"content"`
	}
	if err := decodeArguments(arguments, &input); err != nil {
		return memoryWriteInput{}, err
	}
	category, err := memory.NormalizeCategory(input.Category)
	if err != nil {
		return memoryWriteInput{}, err
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return memoryWriteInput{}, fmt.Errorf("content is required")
	}
	return memoryWriteInput{
		ID:       strings.TrimSpace(input.ID),
		Category: category,
		Content:  content,
	}, nil
}

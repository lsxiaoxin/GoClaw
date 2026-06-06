package tool

import (
	"context"
	"strings"
	"testing"

	"github.com/lsxiaoxin/GoClaw/internal/memory"
)

func TestMemoryReadReturnsSelectedMemory(t *testing.T) {
	store, err := memory.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	if _, err := store.Upsert(memory.Entry{
		Category: memory.CategoryProject,
		Content:  "Use FakeModel for model tests.",
	}); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	tool, err := NewMemoryRead(store)
	if err != nil {
		t.Fatalf("NewMemoryRead() error = %v", err)
	}

	output, err := tool.Run(context.Background(), `{"query":"fake model"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(output, "Use FakeModel for model tests.") {
		t.Fatalf("output = %q", output)
	}
}

func TestMemoryReadHandlesMissingMemoryFile(t *testing.T) {
	store, err := memory.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	tool, err := NewMemoryRead(store)
	if err != nil {
		t.Fatalf("NewMemoryRead() error = %v", err)
	}
	output, err := tool.Run(context.Background(), `{"query":"anything"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if output != "No matching memory." {
		t.Fatalf("output = %q", output)
	}
}

func TestMemoryWritePersistsEntry(t *testing.T) {
	store, err := memory.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	tool, err := NewMemoryWrite(store)
	if err != nil {
		t.Fatalf("NewMemoryWrite() error = %v", err)
	}

	output, err := tool.Run(context.Background(), `{"category":"user","content":"prefers direct answers","id":"pref"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if output != "Memory saved: user/pref" {
		t.Fatalf("output = %q", output)
	}
	entries, err := store.Load(memory.CategoryUser)
	if err != nil {
		t.Fatalf("Load(user) error = %v", err)
	}
	if len(entries) != 1 || entries[0].Content != "prefers direct answers" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestMemoryWriteRejectsSensitiveContent(t *testing.T) {
	store, err := memory.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	tool, err := NewMemoryWrite(store)
	if err != nil {
		t.Fatalf("NewMemoryWrite() error = %v", err)
	}

	err = tool.Validate(`{"category":"user","content":"my API key is sk-1234567890abcdef"}`)
	if err == nil || !strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("Validate() error = %v, want sensitive rejection", err)
	}
}

func TestMemoryToolsRejectInvalidArguments(t *testing.T) {
	store, err := memory.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	readTool, err := NewMemoryRead(store)
	if err != nil {
		t.Fatalf("NewMemoryRead() error = %v", err)
	}
	writeTool, err := NewMemoryWrite(store)
	if err != nil {
		t.Fatalf("NewMemoryWrite() error = %v", err)
	}

	tests := []struct {
		name      string
		tool      Tool
		arguments string
		want      string
	}{
		{name: "read invalid category", tool: readTool, arguments: `{"category":"bad"}`, want: "invalid memory category"},
		{name: "read unknown field", tool: readTool, arguments: `{"query":"x","extra":true}`, want: "unknown field"},
		{name: "write invalid category", tool: writeTool, arguments: `{"category":"bad","content":"x"}`, want: "invalid memory category"},
		{name: "write empty content", tool: writeTool, arguments: `{"category":"user","content":" "}`, want: "content is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.tool.Validate(test.arguments)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

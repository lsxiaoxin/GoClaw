package tool

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileToolsReadWriteEditAndGlob(t *testing.T) {
	workspace := t.TempDir()
	write := mustWriteFile(t, workspace)
	read := mustReadFile(t, workspace)
	edit := mustEditFile(t, workspace)
	glob := mustGlob(t, workspace)

	output, err := write.Run(context.Background(), `{"path":"nested/example.txt","content":"one\ntwo\nthree"}`)
	if err != nil {
		t.Fatalf("write_file error = %v", err)
	}
	if output != "Wrote 13 bytes to nested/example.txt" {
		t.Fatalf("write_file output = %q", output)
	}

	output, err = read.Run(context.Background(), `{"path":"nested/example.txt","limit":2}`)
	if err != nil {
		t.Fatalf("read_file error = %v", err)
	}
	if output != "one\ntwo\n... (1 more lines)" {
		t.Fatalf("read_file output = %q", output)
	}

	output, err = edit.Run(context.Background(), `{"path":"nested/example.txt","old_text":"two","new_text":"second"}`)
	if err != nil {
		t.Fatalf("edit_file error = %v", err)
	}
	if output != "Edited nested/example.txt" {
		t.Fatalf("edit_file output = %q", output)
	}

	output, err = read.Run(context.Background(), `{"path":"nested/example.txt"}`)
	if err != nil {
		t.Fatalf("read_file after edit error = %v", err)
	}
	if output != "one\nsecond\nthree" {
		t.Fatalf("edited content = %q", output)
	}

	if err := os.WriteFile(filepath.Join(workspace, "root.go"), []byte("package root"), 0o600); err != nil {
		t.Fatalf("WriteFile(root.go) error = %v", err)
	}
	output, err = glob.Run(context.Background(), `{"pattern":"**/*.txt"}`)
	if err != nil {
		t.Fatalf("glob error = %v", err)
	}
	if output != "nested/example.txt" {
		t.Fatalf("glob output = %q", output)
	}
	output, err = glob.Run(context.Background(), `{"pattern":"*.go"}`)
	if err != nil {
		t.Fatalf("glob root error = %v", err)
	}
	if output != "root.go" {
		t.Fatalf("glob root output = %q", output)
	}
}

func TestEditFileRequiresOneExactMatch(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "example.txt")
	if err := os.WriteFile(path, []byte("same\nsame\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	edit := mustEditFile(t, workspace)

	if _, err := edit.Run(
		context.Background(),
		`{"path":"example.txt","old_text":"missing","new_text":"new"}`,
	); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing edit error = %v", err)
	}
	if _, err := edit.Run(
		context.Background(),
		`{"path":"example.txt","old_text":"same","new_text":"new"}`,
	); err == nil || !strings.Contains(err.Error(), "2 times") {
		t.Fatalf("ambiguous edit error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "same\nsame\n" {
		t.Fatalf("file changed after rejected edit: %q", data)
	}
}

func TestFileToolsRejectPathEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile(secret) error = %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "outside")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}

	read := mustReadFile(t, workspace)
	write := mustWriteFile(t, workspace)
	edit := mustEditFile(t, workspace)
	glob := mustGlob(t, workspace)

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "read parent",
			run: func() error {
				_, err := read.Run(context.Background(), `{"path":"../secret.txt"}`)
				return err
			},
		},
		{
			name: "read absolute",
			run: func() error {
				_, err := read.Run(context.Background(), `{"path":"/etc/passwd"}`)
				return err
			},
		},
		{
			name: "read symlink",
			run: func() error {
				_, err := read.Run(context.Background(), `{"path":"outside/secret.txt"}`)
				return err
			},
		},
		{
			name: "write symlink",
			run: func() error {
				_, err := write.Run(context.Background(), `{"path":"outside/new.txt","content":"bad"}`)
				return err
			},
		},
		{
			name: "edit symlink",
			run: func() error {
				_, err := edit.Run(context.Background(), `{"path":"outside/secret.txt","old_text":"secret","new_text":"bad"}`)
				return err
			},
		},
		{
			name: "glob parent",
			run: func() error {
				_, err := glob.Run(context.Background(), `{"pattern":"../**"}`)
				return err
			},
		},
		{
			name: "glob outside symlink",
			run: func() error {
				_, err := glob.Run(context.Background(), `{"pattern":"outside"}`)
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); err == nil {
				t.Fatal("tool error = nil, want path escape error")
			}
		})
	}
	if _, err := os.Stat(filepath.Join(outside, "new.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside new.txt was created, stat error = %v", err)
	}
}

func TestFileToolsValidateArgumentsStrictly(t *testing.T) {
	read := mustReadFile(t, t.TempDir())
	if _, err := read.Run(context.Background(), `{"path":"file","unknown":true}`); err == nil {
		t.Fatal("unknown argument error = nil")
	}
	if _, err := read.Run(context.Background(), `{"path":"file","limit":0}`); err == nil {
		t.Fatal("invalid limit error = nil")
	}

	write := mustWriteFile(t, t.TempDir())
	if _, err := write.Run(context.Background(), `{"path":"file"}`); err == nil {
		t.Fatal("missing content error = nil")
	}
}

func mustReadFile(t *testing.T, workspace string) *ReadFile {
	t.Helper()
	tool, err := NewReadFile(workspace)
	if err != nil {
		t.Fatalf("NewReadFile() error = %v", err)
	}
	return tool
}

func mustWriteFile(t *testing.T, workspace string) *WriteFile {
	t.Helper()
	tool, err := NewWriteFile(workspace)
	if err != nil {
		t.Fatalf("NewWriteFile() error = %v", err)
	}
	return tool
}

func mustEditFile(t *testing.T, workspace string) *EditFile {
	t.Helper()
	tool, err := NewEditFile(workspace)
	if err != nil {
		t.Fatalf("NewEditFile() error = %v", err)
	}
	return tool
}

func mustGlob(t *testing.T, workspace string) *Glob {
	t.Helper()
	tool, err := NewGlob(workspace)
	if err != nil {
		t.Fatalf("NewGlob() error = %v", err)
	}
	return tool
}

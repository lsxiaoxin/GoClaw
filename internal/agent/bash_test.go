package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBashToolRunsInWorkspace(t *testing.T) {
	workspace := t.TempDir()
	tool := newTestBashTool(t, workspace, time.Second, 1024)

	output := tool.Run(context.Background(), `{"command":"pwd"}`)
	if strings.TrimSpace(output) != workspace {
		t.Fatalf("pwd output = %q, want %q", output, workspace)
	}

	output = tool.Run(context.Background(), `{"command":"printf test > result.txt && cat result.txt"}`)
	if output != "test" {
		t.Fatalf("command output = %q", output)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "result.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "test" {
		t.Fatalf("result file = %q", data)
	}
}

func TestBashToolRejectsDangerousCommands(t *testing.T) {
	tool := newTestBashTool(t, t.TempDir(), time.Second, 1024)
	commands := []string{
		`rm -rf /`,
		`rm -fr /`,
		`rm --recursive --force /`,
		`sudo shutdown now`,
		`mkfs.ext4 /dev/sda1`,
		`dd if=/dev/zero of=/dev/sda`,
		`:(){ :|:& };:`,
	}
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			output := tool.Run(context.Background(), `{"command":`+quoteJSON(command)+`}`)
			if !strings.HasPrefix(output, "command rejected:") {
				t.Fatalf("output = %q", output)
			}
		})
	}
}

func TestBashToolHonorsParentCancellation(t *testing.T) {
	tool := newTestBashTool(t, t.TempDir(), 5*time.Second, 1024)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tool.Run(ctx, `{"command":"sleep 5"}`)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bash command did not stop after context cancellation")
	}
}

func TestBashToolLimitsOutput(t *testing.T) {
	tool := newTestBashTool(t, t.TempDir(), time.Second, 8)
	output := tool.Run(context.Background(), `{"command":"printf 1234567890"}`)
	if output != "12345678\n[output truncated]" {
		t.Fatalf("output = %q", output)
	}
}

func TestBashToolTimesOut(t *testing.T) {
	tool := newTestBashTool(t, t.TempDir(), 50*time.Millisecond, 1024)
	output := tool.Run(context.Background(), `{"command":"sleep 5"}`)
	if !strings.Contains(output, "command timed out") {
		t.Fatalf("output = %q", output)
	}
}

func newTestBashTool(t *testing.T, workspace string, timeout time.Duration, limit int) *BashTool {
	t.Helper()
	tool, err := NewBashTool(workspace, timeout, limit)
	if err != nil {
		t.Fatalf("NewBashTool() error = %v", err)
	}
	return tool
}

func quoteJSON(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}

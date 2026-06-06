package tool

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBashRunsInWorkspace(t *testing.T) {
	workspace := t.TempDir()
	bash := mustBash(t, workspace, time.Second, 1024)

	output, err := bash.Run(context.Background(), `{"command":"printf test > result.txt && cat result.txt"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if output != "test" {
		t.Fatalf("output = %q", output)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "result.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "test" {
		t.Fatalf("result file = %q", data)
	}
}

func TestBashRejectsDangerousCommands(t *testing.T) {
	bash := mustBash(t, t.TempDir(), time.Second, 1024)
	for _, command := range []string{
		`rm -rf /`,
		`rm -fr /`,
		`rm --recursive --force /`,
		`sudo shutdown now`,
		`mkfs.ext4 /dev/sda1`,
		`dd if=/dev/zero of=/dev/sda`,
		`:(){ :|:& };:`,
	} {
		t.Run(command, func(t *testing.T) {
			_, err := bash.Run(context.Background(), `{"command":`+quoteJSON(command)+`}`)
			if err == nil || !strings.Contains(err.Error(), "command rejected") {
				t.Fatalf("Run() error = %v", err)
			}
		})
	}
}

func TestBashHonorsCancellationTimeoutAndOutputLimit(t *testing.T) {
	t.Run("cancellation", func(t *testing.T) {
		bash := mustBash(t, t.TempDir(), 5*time.Second, 1024)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := bash.Run(ctx, `{"command":"sleep 5"}`)
			done <- err
		}()
		time.Sleep(50 * time.Millisecond)
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Run() error = %v, want context.Canceled", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("bash did not stop after cancellation")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		bash := mustBash(t, t.TempDir(), 50*time.Millisecond, 1024)
		output, err := bash.Run(context.Background(), `{"command":"sleep 5"}`)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if !strings.Contains(output, "command timed out") {
			t.Fatalf("output = %q", output)
		}
	})

	t.Run("output limit", func(t *testing.T) {
		bash := mustBash(t, t.TempDir(), time.Second, 8)
		output, err := bash.Run(context.Background(), `{"command":"printf 1234567890"}`)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if output != "12345678\n[output truncated]" {
			t.Fatalf("output = %q", output)
		}
	})
}

func mustBash(t *testing.T, workspace string, timeout time.Duration, limit int) *Bash {
	t.Helper()
	bash, err := NewBash(workspace, timeout, limit)
	if err != nil {
		t.Fatalf("NewBash() error = %v", err)
	}
	return bash
}

func quoteJSON(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}

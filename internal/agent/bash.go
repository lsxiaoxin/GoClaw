package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cloudwego/eino/schema"
)

const bashToolName = "bash"

var deniedBashPatterns = []struct {
	pattern *regexp.Regexp
	reason  string
}{
	{regexp.MustCompile(`(?i)(^|[\s;&|])(shutdown|reboot|poweroff|halt)(\s|$)`), "system power commands"},
	{regexp.MustCompile(`(?i)(^|[\s;&|])(mkfs(?:\.[a-z0-9]+)?|fdisk|parted)(\s|$)`), "disk formatting or partitioning"},
	{regexp.MustCompile(`(?i)\bdd\b[^\n;]*\bof\s*=\s*/dev/`), "raw device writes"},
	{regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;?\s*:`), "fork bombs"},
}

// BashTool executes bounded bash commands in the configured workspace.
type BashTool struct {
	workspace   string
	timeout     time.Duration
	outputLimit int
	info        *schema.ToolInfo
}

// NewBashTool creates the s01 bash tool.
func NewBashTool(workspace string, timeout time.Duration, outputLimit int) (*BashTool, error) {
	if strings.TrimSpace(workspace) == "" {
		return nil, fmt.Errorf("bash workspace is required")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("bash timeout must be positive")
	}
	if outputLimit <= 0 {
		return nil, fmt.Errorf("bash output limit must be positive")
	}
	return &BashTool{
		workspace:   workspace,
		timeout:     timeout,
		outputLimit: outputLimit,
		info: &schema.ToolInfo{
			Name: bashToolName,
			Desc: "Execute a bash command in the project workspace. Use it for inspecting files, running tests, and other terminal tasks.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"command": {
					Type:     schema.String,
					Desc:     "The bash command to execute.",
					Required: true,
				},
			}),
		},
	}, nil
}

// Info returns the model-facing tool schema.
func (t *BashTool) Info() *schema.ToolInfo {
	return t.info
}

// Run validates and executes one command. Command failures are returned as tool output.
func (t *BashTool) Run(ctx context.Context, arguments string) string {
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return fmt.Sprintf("invalid bash arguments: %v", err)
	}
	input.Command = strings.TrimSpace(input.Command)
	if input.Command == "" {
		return "invalid bash arguments: command is required"
	}
	if reason := deniedBashCommand(input.Command); reason != "" {
		return "command rejected: " + reason
	}

	commandCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	output := &limitedOutput{limit: t.outputLimit}
	cmd := exec.CommandContext(commandCtx, "bash", "-lc", input.Command)
	cmd.Dir = t.workspace
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}

	err := cmd.Run()
	text := output.String()
	if output.Truncated() {
		text += "\n[output truncated]"
	}
	switch {
	case ctx.Err() != nil:
		return text
	case errors.Is(commandCtx.Err(), context.DeadlineExceeded):
		return appendStatus(text, fmt.Sprintf("command timed out after %s", t.timeout))
	case err != nil:
		return appendStatus(text, err.Error())
	case text == "":
		return "command completed successfully with no output"
	default:
		return text
	}
}

func deniedBashCommand(command string) string {
	if recursivelyRemovesRoot(command) {
		return "recursive deletion of the filesystem root"
	}
	for _, denied := range deniedBashPatterns {
		if denied.pattern.MatchString(command) {
			return denied.reason
		}
	}
	return ""
}

func recursivelyRemovesRoot(command string) bool {
	normalized := strings.NewReplacer(
		";", " ",
		"&&", " ",
		"||", " ",
		"|", " ",
		"\n", " ",
	).Replace(command)
	fields := strings.Fields(normalized)
	for index, field := range fields {
		name := strings.Trim(field, `"'`)
		if name != "rm" && !strings.HasSuffix(name, "/rm") {
			continue
		}

		var recursive, root bool
		for _, argument := range fields[index+1:] {
			argument = strings.Trim(argument, `"'`)
			if argument == "rm" || strings.HasSuffix(argument, "/rm") {
				break
			}
			if argument == "--no-preserve-root" || argument == "--recursive" {
				recursive = true
			}
			if strings.HasPrefix(argument, "-") && !strings.HasPrefix(argument, "--") &&
				strings.Contains(strings.ToLower(argument), "r") {
				recursive = true
			}
			if argument == "/" || argument == "/*" {
				root = true
			}
		}
		if recursive && root {
			return true
		}
	}
	return false
}

func appendStatus(output, status string) string {
	if output == "" {
		return "[" + status + "]"
	}
	return output + "\n[" + status + "]"
}

type limitedOutput struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func (w *limitedOutput) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	remaining := w.limit - len(w.data)
	if remaining > 0 {
		if remaining > len(data) {
			remaining = len(data)
		}
		w.data = append(w.data, data[:remaining]...)
	}
	if remaining < len(data) {
		w.truncated = true
	}
	return len(data), nil
}

func (w *limitedOutput) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.data)
}

func (w *limitedOutput) Truncated() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.truncated
}

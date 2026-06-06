package tool

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/lsxiaoxin/GoClaw/internal/permission"
)

// Bash executes bounded bash commands in the configured workspace.
type Bash struct {
	workspace   string
	timeout     time.Duration
	outputLimit int
	info        *schema.ToolInfo
}

// NewBash creates the bash tool.
func NewBash(workspace string, timeout time.Duration, outputLimit int) (*Bash, error) {
	if strings.TrimSpace(workspace) == "" {
		return nil, fmt.Errorf("bash workspace is required")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("bash timeout must be positive")
	}
	if outputLimit <= 0 {
		return nil, fmt.Errorf("bash output limit must be positive")
	}
	return &Bash{
		workspace:   workspace,
		timeout:     timeout,
		outputLimit: outputLimit,
		info: &schema.ToolInfo{
			Name: "bash",
			Desc: "Execute a bash command in the project workspace. Use it for tests and terminal tasks not covered by dedicated tools.",
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

func (t *Bash) Info() *schema.ToolInfo { return t.info }

// ConcurrencySafe returns false because bash commands execute sequentially.
func (t *Bash) ConcurrencySafe() bool { return false }

// Validate checks bash arguments and the hard deny list without executing.
func (t *Bash) Validate(arguments string) error {
	_, err := bashInputFrom(arguments)
	return err
}

// Run validates and executes one command. Command failures are returned as output.
func (t *Bash) Run(ctx context.Context, arguments string) (string, error) {
	input, err := bashInputFrom(arguments)
	if err != nil {
		return "", err
	}

	commandCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	output := &limitedOutput{limit: t.outputLimit}
	command := exec.CommandContext(commandCtx, "bash", "-lc", input.Command)
	command.Dir = t.workspace
	command.Stdout = output
	command.Stderr = output
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}

	err = command.Run()
	text := output.String()
	if output.Truncated() {
		text += "\n[output truncated]"
	}
	switch {
	case ctx.Err() != nil:
		return "", ctx.Err()
	case errors.Is(commandCtx.Err(), context.DeadlineExceeded):
		return appendStatus(text, fmt.Sprintf("command timed out after %s", t.timeout)), nil
	case err != nil:
		return appendStatus(text, err.Error()), nil
	case text == "":
		return "command completed successfully with no output", nil
	default:
		return text, nil
	}
}

type bashInput struct {
	Command string `json:"command"`
}

func bashInputFrom(arguments string) (bashInput, error) {
	var input bashInput
	if err := decodeArguments(arguments, &input); err != nil {
		return bashInput{}, err
	}
	input.Command = strings.TrimSpace(input.Command)
	if input.Command == "" {
		return bashInput{}, fmt.Errorf("command is required")
	}
	if reason := permission.HardDenyBash(input.Command); reason != "" {
		return bashInput{}, fmt.Errorf("command rejected: %s", reason)
	}
	return input, nil
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

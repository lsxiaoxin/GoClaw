package subagent

import (
	"strings"
	"testing"
)

func TestRequestValidate(t *testing.T) {
	if err := (Request{Prompt: "inspect README"}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	err := (Request{Prompt: "  "}).Validate()
	if err == nil || !strings.Contains(err.Error(), "prompt is required") {
		t.Fatalf("Validate() error = %v, want prompt error", err)
	}
}

func TestResultText(t *testing.T) {
	tests := []struct {
		name   string
		result Result
		want   string
	}{
		{
			name:   "completed",
			result: Result{Status: StatusCompleted, Summary: "README summarizes GoClaw."},
			want:   "Subagent completed: README summarizes GoClaw.",
		},
		{
			name:   "failed",
			result: Result{Status: StatusFailed, Error: "maximum agent steps exceeded"},
			want:   "Subagent failed: maximum agent steps exceeded",
		},
		{
			name:   "approval",
			result: Result{Status: StatusWaitingApproval, Error: "write_file modifies workspace files"},
			want:   "Subagent blocked by permission: write_file modifies workspace files",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.result.Text(); got != test.want {
				t.Fatalf("Text() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLimitsNormalize(t *testing.T) {
	got, err := (Limits{}).Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if got != DefaultLimits() {
		t.Fatalf("Normalize() = %+v, want %+v", got, DefaultLimits())
	}

	if _, err := (Limits{MaxDepth: -1, MaxConcurrent: 1}).Normalize(); err == nil {
		t.Fatal("Normalize() error = nil, want max depth error")
	}
	if _, err := (Limits{MaxDepth: 1, MaxConcurrent: -1}).Normalize(); err == nil {
		t.Fatal("Normalize() error = nil, want max concurrent error")
	}
}

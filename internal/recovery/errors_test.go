package recovery

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWrapAndClassifyTypedError(t *testing.T) {
	err := Wrap(ModelError, errors.New("temporary failure"))
	if got := Classify(err); got != ModelError {
		t.Fatalf("Classify() = %q, want %q", got, ModelError)
	}
	if !strings.Contains(err.Error(), "ModelError: temporary failure") {
		t.Fatalf("Error() = %q", err.Error())
	}
}

func TestClassifyByMessage(t *testing.T) {
	tests := []struct {
		err  error
		want Category
	}{
		{errors.New("stream model response: timeout"), ModelError},
		{errors.New("hook blocked: no bash"), HookError},
		{errors.New("Permission denied: write"), PermissionError},
		{errors.New("summarize context: failed"), CompactError},
		{errors.New("save approval: disk full"), StoreError},
		{errors.New("load config file"), ConfigError},
		{errors.New("other"), UnknownError},
	}
	for _, test := range tests {
		if got := Classify(test.err); got != test.want {
			t.Fatalf("Classify(%q) = %q, want %q", test.err, got, test.want)
		}
	}
}

func TestRetryPolicyRetriesOnlyModelErrors(t *testing.T) {
	policy := RetryPolicy{MaxAttempts: 3, Backoff: time.Second}
	if !policy.ShouldRetry(ModelError, 1) {
		t.Fatal("ShouldRetry(model, 1) = false")
	}
	if !policy.ShouldRetry(ModelError, 2) {
		t.Fatal("ShouldRetry(model, 2) = false")
	}
	if policy.ShouldRetry(ModelError, 3) {
		t.Fatal("ShouldRetry(model, 3) = true")
	}
	if policy.ShouldRetry(PermissionError, 1) {
		t.Fatal("ShouldRetry(permission, 1) = true")
	}
	if got := policy.BackoffFor(2); got != 2*time.Second {
		t.Fatalf("BackoffFor(2) = %s", got)
	}
}

func TestSummaryIncludesCategory(t *testing.T) {
	got := Summary(Wrap(StoreError, errors.New("disk full")))
	if !strings.Contains(got, "StoreError") || !strings.Contains(got, "disk full") {
		t.Fatalf("Summary() = %q", got)
	}
}

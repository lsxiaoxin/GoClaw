package permission

import (
	"errors"
	"strings"
	"testing"
)

func TestEngineAppliesPermissionLayers(t *testing.T) {
	engine := New()
	valid := func(string) error { return nil }

	tests := []struct {
		name       string
		tool       string
		arguments  string
		validate   Validator
		want       Behavior
		wantReason string
	}{
		{
			name:      "read is allowed",
			tool:      "read_file",
			arguments: `{"path":"README.md"}`,
			validate:  valid,
			want:      Allow,
		},
		{
			name:       "write asks",
			tool:       "write_file",
			arguments:  `{"path":"note.txt","content":"hello"}`,
			validate:   valid,
			want:       Ask,
			wantReason: "modifies workspace",
		},
		{
			name:      "read-only bash is allowed",
			tool:      "bash",
			arguments: `{"command":"git status | head"}`,
			validate:  valid,
			want:      Allow,
		},
		{
			name:       "uncertain bash asks",
			tool:       "bash",
			arguments:  `{"command":"go build ./..."}`,
			validate:   valid,
			want:       Ask,
			wantReason: "not clearly read-only",
		},
		{
			name:       "hard deny wins over validation",
			tool:       "bash",
			arguments:  `{"command":"sudo rm file"}`,
			validate:   func(string) error { return errors.New("invalid arguments") },
			want:       Deny,
			wantReason: "privilege escalation",
		},
		{
			name:       "invalid wins over ask",
			tool:       "write_file",
			arguments:  `{"path":"../outside","content":"bad"}`,
			validate:   func(string) error { return errors.New("path escapes workspace") },
			want:       Invalid,
			wantReason: "path escapes workspace",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := engine.Decide(test.tool, test.arguments, test.validate)
			if got.Behavior != test.want {
				t.Fatalf("behavior = %q, want %q", got.Behavior, test.want)
			}
			if test.wantReason != "" && !strings.Contains(got.Reason, test.wantReason) {
				t.Fatalf("reason = %q, want substring %q", got.Reason, test.wantReason)
			}
		})
	}
}

func TestClearlyReadOnlyBashIsConservative(t *testing.T) {
	allowed := []string{
		"pwd",
		"ls -la",
		"rg TODO internal | head -20",
		"git status --short",
		"go list ./...",
		"find . -name '*.go'",
	}
	for _, command := range allowed {
		if !ClearlyReadOnlyBash(command) {
			t.Errorf("ClearlyReadOnlyBash(%q) = false", command)
		}
	}

	asked := []string{
		"echo hello > file",
		"go build ./...",
		"go test ./...",
		"git checkout main",
		"git diff -- README.md",
		"awk 'BEGIN { system(\"rm file\") }'",
		"sed -i s/a/b/ file",
		"find . -delete",
		"sort -o output input",
		"rg --pre 'rm file' TODO",
		"ls && rm file",
		"cat $(which secret)",
	}
	for _, command := range asked {
		if ClearlyReadOnlyBash(command) {
			t.Errorf("ClearlyReadOnlyBash(%q) = true", command)
		}
	}
}

func TestHardDenyBash(t *testing.T) {
	for _, command := range []string{
		"sudo cat /etc/shadow",
		`"sudo" cat /etc/shadow`,
		"rm -rf /",
		"mkfs.ext4 /dev/sda",
		"dd if=/tmp/image of=/dev/sda",
	} {
		if reason := HardDenyBash(command); reason == "" {
			t.Errorf("HardDenyBash(%q) returned no reason", command)
		}
	}
}

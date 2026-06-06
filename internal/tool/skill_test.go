package tool

import (
	"context"
	"strings"
	"testing"

	"github.com/lsxiaoxin/GoClaw/internal/skill"
)

func TestLoadSkillReturnsInstructions(t *testing.T) {
	tool, err := NewLoadSkill([]skill.Skill{{
		Name:         "go-tests",
		Description:  "Write Go tests",
		Instructions: "Use table-driven tests.",
	}})
	if err != nil {
		t.Fatalf("NewLoadSkill() error = %v", err)
	}

	output, err := tool.Run(context.Background(), `{"name":"go-tests"}`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{
		"Skill: go-tests",
		"Description: Write Go tests",
		"Use table-driven tests.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want substring %q", output, want)
		}
	}
}

func TestLoadSkillRejectsInvalidArguments(t *testing.T) {
	tool, err := NewLoadSkill([]skill.Skill{{
		Name:         "docs",
		Description:  "Docs",
		Instructions: "Write docs.",
	}})
	if err != nil {
		t.Fatalf("NewLoadSkill() error = %v", err)
	}

	tests := []struct {
		name      string
		arguments string
		want      string
	}{
		{name: "missing name", arguments: `{}`, want: "name is required"},
		{name: "unknown skill", arguments: `{"name":"missing"}`, want: "not available"},
		{name: "unknown field", arguments: `{"name":"docs","extra":true}`, want: "unknown field"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := tool.Validate(test.arguments)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

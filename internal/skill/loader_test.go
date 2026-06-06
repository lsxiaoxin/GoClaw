package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMarkdown(t *testing.T) {
	got, err := ParseMarkdown(`---
name: go-tests
description: Write Go tests
---
Use table-driven tests.
`)
	if err != nil {
		t.Fatalf("ParseMarkdown() error = %v", err)
	}
	if got.Name != "go-tests" || got.Description != "Write Go tests" ||
		got.Instructions != "Use table-driven tests." {
		t.Fatalf("skill = %+v", got)
	}
}

func TestLoadSkills(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "go-tests", `---
name: go-tests
description: Write Go tests
---
Use table-driven tests.
`)
	writeSkill(t, root, "docs", `---
name: docs
description: Documentation helper
---
Keep docs concise.
`)

	loader, err := NewLoader(root)
	if err != nil {
		t.Fatalf("NewLoader() error = %v", err)
	}
	skills, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("skill count = %d, want 2", len(skills))
	}
	if skills[0].Name != "docs" || skills[1].Name != "go-tests" {
		t.Fatalf("skills = %+v", skills)
	}
	if skills[0].Path == "" {
		t.Fatalf("skill path was not set: %+v", skills[0])
	}
}

func TestLoadMissingRootReturnsEmpty(t *testing.T) {
	loader, err := NewLoader(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("NewLoader() error = %v", err)
	}
	skills, err := loader.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("skills = %+v, want empty", skills)
	}
}

func TestLoadRejectsDuplicateNamesAndBrokenFrontmatter(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "a", `---
name: same
description: A
---
A instructions.
`)
	writeSkill(t, root, "b", `---
name: same
description: B
---
B instructions.
`)
	loader, err := NewLoader(root)
	if err != nil {
		t.Fatalf("NewLoader() error = %v", err)
	}
	err = loadError(loader)
	if err == nil || !strings.Contains(err.Error(), "duplicate skill name") {
		t.Fatalf("Load() error = %v, want duplicate name", err)
	}

	brokenRoot := t.TempDir()
	writeSkill(t, brokenRoot, "bad", `name: bad
description: bad
`)
	loader, err = NewLoader(brokenRoot)
	if err != nil {
		t.Fatalf("NewLoader(broken) error = %v", err)
	}
	err = loadError(loader)
	if err == nil || !strings.Contains(err.Error(), "frontmatter") {
		t.Fatalf("Load() error = %v, want frontmatter error", err)
	}
}

func TestSelectorMatchesKeywords(t *testing.T) {
	selector := NewSelector([]Skill{
		{Name: "go-tests", Description: "Write Go tests", Instructions: "Use testing package"},
		{Name: "docs", Description: "Documentation", Instructions: "Keep docs concise"},
	})
	selected := selector.Select("please write go unit tests")
	if len(selected) != 1 || selected[0].Name != "go-tests" {
		t.Fatalf("selected = %+v", selected)
	}

	selected = selector.Select("update docs and tests")
	if len(selected) != 2 || selected[0].Name != "docs" || selected[1].Name != "go-tests" {
		t.Fatalf("selected = %+v", selected)
	}

	if selected := selector.Select("hi"); len(selected) != 0 {
		t.Fatalf("selected = %+v, want none", selected)
	}
}

func writeSkill(t *testing.T, root, name, content string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, SkillFileName), []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}
}

func loadError(loader *Loader) error {
	_, err := loader.Load()
	return err
}

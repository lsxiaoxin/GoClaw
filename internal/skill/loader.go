package skill

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const SkillFileName = "SKILL.md"

// Loader reads skills from a root directory.
type Loader struct {
	root string
}

// NewLoader creates a skill loader rooted at .goclaw/skills or another project path.
func NewLoader(root string) (*Loader, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("skill root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve skill root: %w", err)
	}
	return &Loader{root: absolute}, nil
}

// Load reads all skills. A missing root is treated as no configured skills.
func (l *Loader) Load() ([]Skill, error) {
	entries, err := os.ReadDir(l.root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read skill root: %w", err)
	}

	var skills []Skill
	seen := map[string]string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !safeDirName(name) {
			return nil, fmt.Errorf("invalid skill directory %q", name)
		}
		path := filepath.Join(l.root, name, SkillFileName)
		parsed, err := parseFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("load skill %q: %w", name, err)
		}
		parsed.Path = path
		if err := parsed.Validate(); err != nil {
			return nil, fmt.Errorf("load skill %q: %w", name, err)
		}
		key := strings.ToLower(parsed.Name)
		if previous, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate skill name %q in %s and %s", parsed.Name, previous, path)
		}
		seen[key] = path
		skills = append(skills, parsed)
	}
	sort.Slice(skills, func(i, j int) bool {
		return strings.ToLower(skills[i].Name) < strings.ToLower(skills[j].Name)
	})
	return skills, nil
}

func parseFile(path string) (Skill, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Skill{}, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return Skill{}, fmt.Errorf("skill file must not be a symlink")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	return ParseMarkdown(string(data))
}

// ParseMarkdown parses YAML-like frontmatter followed by instructions.
func ParseMarkdown(content string) (Skill, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return Skill{}, fmt.Errorf("frontmatter is required")
	}
	metadata := map[string]string{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			instructions := strings.TrimSpace(remaining(scanner))
			skill := Skill{
				Name:         metadata["name"],
				Description:  metadata["description"],
				Instructions: instructions,
			}
			return skill, skill.Validate()
		}
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Skill{}, fmt.Errorf("invalid frontmatter line %q", line)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch key {
		case "name", "description":
			metadata[key] = value
		default:
			return Skill{}, fmt.Errorf("unsupported frontmatter key %q", key)
		}
	}
	if err := scanner.Err(); err != nil {
		return Skill{}, err
	}
	return Skill{}, fmt.Errorf("frontmatter closing marker is required")
}

func remaining(scanner *bufio.Scanner) string {
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return strings.Join(lines, "\n")
}

func safeDirName(name string) bool {
	return name != "" && name != "." && name != ".." &&
		!strings.ContainsAny(name, `/\`) &&
		filepath.Base(name) == name
}

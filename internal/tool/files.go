package tool

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// ReadFile reads a UTF-8 text file in the workspace.
type ReadFile struct {
	paths *workspacePaths
	info  *schema.ToolInfo
}

// NewReadFile creates the read_file tool.
func NewReadFile(workspace string) (*ReadFile, error) {
	paths, err := newWorkspacePaths(workspace)
	if err != nil {
		return nil, err
	}
	return &ReadFile{
		paths: paths,
		info: &schema.ToolInfo{
			Name: "read_file",
			Desc: "Read a text file from the workspace. Paths must be relative to the workspace.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"path": {
					Type:     schema.String,
					Desc:     "Workspace-relative file path.",
					Required: true,
				},
				"limit": {
					Type: schema.Integer,
					Desc: "Optional maximum number of lines to return.",
				},
			}),
		},
	}, nil
}

func (t *ReadFile) Info() *schema.ToolInfo { return t.info }

func (t *ReadFile) ConcurrencySafe() bool { return true }

func (t *ReadFile) Run(ctx context.Context, arguments string) (string, error) {
	var input struct {
		Path  string `json:"path"`
		Limit *int   `json:"limit,omitempty"`
	}
	if err := decodeArguments(arguments, &input); err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Path) == "" {
		return "", fmt.Errorf("path is required")
	}
	if input.Limit != nil && *input.Limit <= 0 {
		return "", fmt.Errorf("limit must be positive")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	resolved, err := t.paths.existing(input.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}
	if input.Limit == nil {
		return string(data), nil
	}

	lines := strings.Split(string(data), "\n")
	if len(data) > 0 && data[len(data)-1] == '\n' {
		lines = lines[:len(lines)-1]
	}
	if len(lines) <= *input.Limit {
		return string(data), nil
	}
	remaining := len(lines) - *input.Limit
	return strings.Join(lines[:*input.Limit], "\n") +
		fmt.Sprintf("\n... (%d more lines)", remaining), nil
}

// WriteFile writes a complete file in the workspace.
type WriteFile struct {
	paths *workspacePaths
	info  *schema.ToolInfo
}

// NewWriteFile creates the write_file tool.
func NewWriteFile(workspace string) (*WriteFile, error) {
	paths, err := newWorkspacePaths(workspace)
	if err != nil {
		return nil, err
	}
	return &WriteFile{
		paths: paths,
		info: &schema.ToolInfo{
			Name: "write_file",
			Desc: "Write complete text content to a workspace file, creating parent directories when needed.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"path": {
					Type:     schema.String,
					Desc:     "Workspace-relative file path.",
					Required: true,
				},
				"content": {
					Type:     schema.String,
					Desc:     "Complete file content.",
					Required: true,
				},
			}),
		},
	}, nil
}

func (t *WriteFile) Info() *schema.ToolInfo { return t.info }

func (t *WriteFile) ConcurrencySafe() bool { return false }

func (t *WriteFile) Run(ctx context.Context, arguments string) (string, error) {
	var input struct {
		Path    string  `json:"path"`
		Content *string `json:"content"`
	}
	if err := decodeArguments(arguments, &input); err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Path) == "" {
		return "", fmt.Errorf("path is required")
	}
	if input.Content == nil {
		return "", fmt.Errorf("content is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	resolved, err := t.paths.writable(input.Path)
	if err != nil {
		return "", err
	}
	if err := writeTextFile(resolved, []byte(*input.Content)); err != nil {
		return "", err
	}
	return fmt.Sprintf("Wrote %d bytes to %s", len(*input.Content), input.Path), nil
}

// EditFile replaces one exact, unique text occurrence.
type EditFile struct {
	paths *workspacePaths
	info  *schema.ToolInfo
}

// NewEditFile creates the edit_file tool.
func NewEditFile(workspace string) (*EditFile, error) {
	paths, err := newWorkspacePaths(workspace)
	if err != nil {
		return nil, err
	}
	return &EditFile{
		paths: paths,
		info: &schema.ToolInfo{
			Name: "edit_file",
			Desc: "Replace one exact and unique text occurrence in a workspace file.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"path": {
					Type:     schema.String,
					Desc:     "Workspace-relative file path.",
					Required: true,
				},
				"old_text": {
					Type:     schema.String,
					Desc:     "Exact text to replace. It must occur exactly once.",
					Required: true,
				},
				"new_text": {
					Type:     schema.String,
					Desc:     "Replacement text.",
					Required: true,
				},
			}),
		},
	}, nil
}

func (t *EditFile) Info() *schema.ToolInfo { return t.info }

func (t *EditFile) ConcurrencySafe() bool { return false }

func (t *EditFile) Run(ctx context.Context, arguments string) (string, error) {
	var input struct {
		Path    string  `json:"path"`
		OldText string  `json:"old_text"`
		NewText *string `json:"new_text"`
	}
	if err := decodeArguments(arguments, &input); err != nil {
		return "", err
	}
	if strings.TrimSpace(input.Path) == "" {
		return "", fmt.Errorf("path is required")
	}
	if input.OldText == "" {
		return "", fmt.Errorf("old_text is required")
	}
	if input.NewText == nil {
		return "", fmt.Errorf("new_text is required")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	resolved, err := t.paths.existing(input.Path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}
	count := strings.Count(string(data), input.OldText)
	if count == 0 {
		return "", fmt.Errorf("old_text was not found in %s", input.Path)
	}
	if count > 1 {
		return "", fmt.Errorf("old_text occurs %d times in %s", count, input.Path)
	}
	updated := strings.Replace(string(data), input.OldText, *input.NewText, 1)
	if err := writeTextFile(resolved, []byte(updated)); err != nil {
		return "", err
	}
	return fmt.Sprintf("Edited %s", input.Path), nil
}

// Glob finds workspace paths without following directory symlinks.
type Glob struct {
	paths *workspacePaths
	info  *schema.ToolInfo
}

// NewGlob creates the glob tool.
func NewGlob(workspace string) (*Glob, error) {
	paths, err := newWorkspacePaths(workspace)
	if err != nil {
		return nil, err
	}
	return &Glob{
		paths: paths,
		info: &schema.ToolInfo{
			Name: "glob",
			Desc: "Find workspace paths matching a glob pattern. Supports *, ?, character classes, and **.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"pattern": {
					Type:     schema.String,
					Desc:     "Workspace-relative slash-separated glob pattern.",
					Required: true,
				},
			}),
		},
	}, nil
}

func (t *Glob) Info() *schema.ToolInfo { return t.info }

func (t *Glob) ConcurrencySafe() bool { return true }

func (t *Glob) Run(ctx context.Context, arguments string) (string, error) {
	var input struct {
		Pattern string `json:"pattern"`
	}
	if err := decodeArguments(arguments, &input); err != nil {
		return "", err
	}
	pattern := filepath.ToSlash(strings.TrimSpace(input.Pattern))
	if pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	if !filepath.IsLocal(filepath.FromSlash(pattern)) {
		return "", fmt.Errorf("pattern must be workspace-relative: %s", input.Pattern)
	}
	if err := validateGlobPattern(pattern); err != nil {
		return "", err
	}

	var matches []string
	err := fs.WalkDir(os.DirFS(t.paths.root), ".", func(entryPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entryPath == "." {
			return nil
		}
		relative := path.Clean(filepath.ToSlash(entryPath))
		matched, err := matchGlob(pattern, relative)
		if err != nil {
			return err
		}
		if !matched {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			if _, err := t.paths.existing(filepath.FromSlash(relative)); err != nil {
				return err
			}
		}
		matches = append(matches, relative)
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "(no matches)", nil
	}
	sort.Strings(matches)
	return strings.Join(matches, "\n"), nil
}

func writeTextFile(filePath string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(filePath); err == nil {
		if info.IsDir() {
			return fmt.Errorf("path is a directory")
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}

	file, err := os.CreateTemp(filepath.Dir(filePath), ".goclaw-write-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, filePath)
}

func validateGlobPattern(pattern string) error {
	for _, segment := range strings.Split(pattern, "/") {
		if segment == "**" {
			continue
		}
		if _, err := path.Match(segment, ""); err != nil {
			return fmt.Errorf("invalid glob pattern: %w", err)
		}
	}
	return nil
}

func matchGlob(pattern, name string) (bool, error) {
	return matchGlobParts(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchGlobParts(pattern, name []string) (bool, error) {
	if len(pattern) == 0 {
		return len(name) == 0, nil
	}
	if pattern[0] == "**" {
		matched, err := matchGlobParts(pattern[1:], name)
		if err != nil || matched {
			return matched, err
		}
		if len(name) == 0 {
			return false, nil
		}
		return matchGlobParts(pattern, name[1:])
	}
	if len(name) == 0 {
		return false, nil
	}
	matched, err := path.Match(pattern[0], name[0])
	if err != nil || !matched {
		return matched, err
	}
	return matchGlobParts(pattern[1:], name[1:])
}

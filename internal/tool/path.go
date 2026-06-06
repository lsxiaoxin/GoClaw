package tool

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type workspacePaths struct {
	root string
}

func newWorkspacePaths(root string) (*workspacePaths, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("workspace is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace symlinks: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("stat workspace: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace is not a directory")
	}
	return &workspacePaths{root: resolved}, nil
}

func (w *workspacePaths) existing(path string) (string, error) {
	candidate, err := w.lexical(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	if !w.contains(resolved) {
		return "", fmt.Errorf("path escapes workspace: %s", path)
	}
	return resolved, nil
}

func (w *workspacePaths) writable(path string) (string, error) {
	candidate, err := w.lexical(path)
	if err != nil {
		return "", err
	}

	if _, err := os.Lstat(candidate); err == nil {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			return "", err
		}
		if !w.contains(resolved) {
			return "", fmt.Errorf("path escapes workspace: %s", path)
		}
		return resolved, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	parent := filepath.Dir(candidate)
	for {
		if _, err := os.Lstat(parent); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return "", fmt.Errorf("find existing parent for %s", path)
		}
		parent = next
	}

	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", err
	}
	if !w.contains(resolvedParent) {
		return "", fmt.Errorf("path escapes workspace: %s", path)
	}
	remainder, err := filepath.Rel(parent, candidate)
	if err != nil {
		return "", err
	}
	resolved := filepath.Join(resolvedParent, remainder)
	if !w.contains(resolved) {
		return "", fmt.Errorf("path escapes workspace: %s", path)
	}
	return resolved, nil
}

func (w *workspacePaths) lexical(path string) (string, error) {
	if !filepath.IsLocal(path) {
		return "", fmt.Errorf("path must be workspace-relative: %s", path)
	}
	return filepath.Join(w.root, filepath.Clean(path)), nil
}

func (w *workspacePaths) contains(path string) bool {
	relative, err := filepath.Rel(w.root, path)
	if err != nil {
		return false
	}
	return relative == "." || filepath.IsLocal(relative)
}

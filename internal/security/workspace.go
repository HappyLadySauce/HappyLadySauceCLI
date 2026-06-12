package security

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// WorkspaceGuard validates future file-tool paths against allowed roots.
// WorkspaceGuard 根据允许的根目录校验未来文件工具路径。
type WorkspaceGuard struct {
	roots []string
}

// NewWorkspaceGuard creates a guard with canonical allowed roots.
// NewWorkspaceGuard 使用规范化允许根目录创建路径 guard。
func NewWorkspaceGuard(roots []string) (*WorkspaceGuard, error) {
	if len(roots) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
		roots = []string{cwd}
	}
	normalized := make([]string, 0, len(roots))
	for _, root := range roots {
		canonical, err := canonicalPath(root)
		if err != nil {
			return nil, fmt.Errorf("normalize workspace root %q: %w", root, err)
		}
		normalized = append(normalized, canonical)
	}
	return &WorkspaceGuard{roots: normalized}, nil
}

// NormalizePath returns the canonical path when it stays under an allowed root.
// NormalizePath 在路径位于允许根目录下时返回其规范化路径。
func (g *WorkspaceGuard) NormalizePath(path string) (string, error) {
	if g == nil {
		return "", fmt.Errorf("workspace guard is nil")
	}
	canonical, err := canonicalPath(path)
	if err != nil {
		return "", err
	}
	for _, root := range g.roots {
		if pathWithinRoot(canonical, root) {
			return canonical, nil
		}
	}
	return "", fmt.Errorf("path escapes workspace roots: %s", canonical)
}

func canonicalPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
		path = filepath.Join(cwd, path)
	}
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}
	resolved, err := resolveSymlinksUpToLastExisting(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func resolveSymlinksUpToLastExisting(path string) (string, error) {
	evaluated, err := filepath.EvalSymlinks(path)
	if err == nil {
		return evaluated, nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("resolve symlinks: %w", err)
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path, nil
	}
	resolvedParent, err := resolveSymlinksUpToLastExisting(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedParent, filepath.Base(path)), nil
}

func pathWithinRoot(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	if runtime.GOOS == "windows" && strings.EqualFold(path, root) {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

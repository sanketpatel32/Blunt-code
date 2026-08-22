// Package workspace handles local project roots and path containment.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func NormalizeRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("workspace path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("workspace path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace path must be a directory")
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve workspace path: %w", err)
	}
	// EvalSymlinks leaves NTFS junctions in place (they are not reported as
	// symlinks), which would register distinct workspace rows for the same
	// physical directory depending on the path the user typed.
	resolved = ResolveJunctions(resolved)
	return filepath.Clean(resolved), nil
}

func IsWithin(root, candidate string) (bool, error) {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false, err
	}
	if rel == "." {
		return true, nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}

func ValidateRelativePath(root, relative string) (string, error) {
	if relative == "" || relative == "." {
		return "", nil
	}
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("path must be relative")
	}
	clean := filepath.Clean(relative)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace")
	}
	full := filepath.Join(root, clean)
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return "", fmt.Errorf("resolve requested path: %w", err)
	}
	ok, err := IsWithin(root, resolved)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("path resolves outside workspace")
	}
	return clean, nil
}

func CanonicalKey(path string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(filepath.Clean(path))
	}
	return filepath.Clean(path)
}

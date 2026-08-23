// Package discovery walks workspaces without following external links.
package discovery

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"bluntcode/internal/core"
	"bluntcode/internal/workspace"
)

var defaultDirectories = map[string]struct{}{
	".git": {}, ".hg": {}, ".svn": {}, "node_modules": {}, ".venv": {}, "venv": {}, "env": {}, "__pycache__": {}, ".pytest_cache": {}, ".mypy_cache": {}, ".ruff_cache": {}, ".next": {}, ".nuxt": {}, "dist": {}, "build": {}, "out": {}, "coverage": {}, ".cache": {}, ".idea": {}, ".vscode": {},
}

func DefaultExcluded(path string, directory bool) bool {
	base := strings.ToLower(filepath.Base(path))
	if directory {
		_, ok := defaultDirectories[base]
		return ok
	}
	return strings.HasSuffix(base, ".min.js") || strings.HasSuffix(base, ".map") || strings.HasSuffix(base, ".pyc") || strings.HasSuffix(base, ".pyo")
}

func Language(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py", ".pyi":
		return "python"
	case ".ts", ".tsx", ".mts", ".cts":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	default:
		return ""
	}
}

type Result struct {
	Files     []core.FileEntry `json:"files"`
	Languages map[string]int   `json:"languages"`
	Skipped   int              `json:"skipped"`
}

func Discover(ctx context.Context, root string, userExcludes []string) (Result, error) {
	result := Result{Languages: map[string]int{}}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.Clean(rel)
		if entry.Type()&fs.ModeSymlink != 0 {
			result.Skipped++
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if DefaultExcluded(rel, entry.IsDir()) || excludedByUser(rel, userExcludes) {
			result.Skipped++
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type().IsRegular() == false {
			return nil
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil
		}
		within, _ := workspace.IsWithin(root, resolved)
		if !within {
			result.Skipped++
			return nil
		}
		lang := Language(rel)
		if lang == "" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		result.Languages[lang]++
		result.Files = append(result.Files, core.FileEntry{RelativePath: filepath.ToSlash(rel), Language: lang, SizeBytes: info.Size(), Selected: true})
		return nil
	})
	return result, err
}

// Tree returns only immediate safe children. The UI requests children when a
// folder is expanded, so this never walks an entire large repository.
func Tree(ctx context.Context, root, relative string, userExcludes []string) ([]core.FileEntry, error) {
	directory := root
	if relative != "" && relative != "." {
		directory = filepath.Join(root, relative)
	}
	if directory != root {
		if resolved, err := filepath.EvalSymlinks(directory); err == nil {
			if ok, _ := workspace.IsWithin(root, resolved); !ok {
				return nil, fmt.Errorf("path resolves outside workspace")
			}
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	items := make([]core.FileEntry, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := filepath.Join(directory, entry.Name())
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		if entry.Type()&fs.ModeSymlink != 0 || DefaultExcluded(rel, entry.IsDir()) || excludedByUser(rel, userExcludes) {
			continue
		}
		if entry.IsDir() {
			items = append(items, core.FileEntry{RelativePath: rel, IsDir: true, Selected: true})
			continue
		}
		if !entry.Type().IsRegular() {
			continue
		}
		language := Language(rel)
		if language == "" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		items = append(items, core.FileEntry{RelativePath: rel, Language: language, SizeBytes: info.Size(), Selected: true})
	}
	return items, nil
}
func excludedByUser(rel string, patterns []string) bool {
	rel = strings.ToLower(filepath.ToSlash(rel))
	for _, pattern := range patterns {
		pattern = strings.ToLower(filepath.ToSlash(strings.TrimSpace(pattern)))
		if pattern == "" {
			continue
		}
		matched, err := filepath.Match(pattern, rel)
		if err == nil && matched {
			return true
		}
		if strings.HasSuffix(pattern, "/**") && strings.HasPrefix(rel, strings.TrimSuffix(pattern, "/**")+"/") {
			return true
		}
		if strings.HasPrefix(pattern, "**/") && (rel == strings.TrimPrefix(pattern, "**/") || strings.HasSuffix(rel, "/"+strings.TrimPrefix(pattern, "**/"))) {
			return true
		}
	}
	return false
}

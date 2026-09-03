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

// DefaultExcluded reports whether a path is excluded from scanning without
// any user configuration: artifact directories (node_modules, dist, target,
// vendor, ...) for directories, and generated file names (minified bundles,
// lockfiles, protobuf and generator output, hash-sufficed bundler chunks)
// for files. The tables and their rationale live in artifacts.go.
func DefaultExcluded(path string, directory bool) bool {
	base := filepath.Base(path)
	if directory {
		return artifactDirectoryName(base)
	}
	return artifactFileName(base)
}

// extensionLanguages maps every file extension discovery classifies to a
// normalized lowercase language name. Extensions are matched case-
// insensitively. The python, javascript, and typescript names are load-
// bearing: analyzer routing, fingerprints, and persisted snapshots rely on
// them, so they must never be renamed. Binaries and assets are excluded by
// omission exactly as before — an extension that is not a key here makes the
// file a non-candidate.
//
// internal/analyzers keeps a mirrored copy of this table (plus the basename
// rules below) so its file filtering agrees with discovery; the analyzers
// tests assert the two stay in sync.
var extensionLanguages = map[string]string{
	// Original trio; names are exact and stable.
	".py": "python", ".pyi": "python",
	".ts": "typescript", ".tsx": "typescript", ".mts": "typescript", ".cts": "typescript",
	".js": "javascript", ".jsx": "javascript", ".mjs": "javascript", ".cjs": "javascript",
	// Code.
	".go": "go", ".java": "java",
	".kt": "kotlin", ".kts": "kotlin",
	".cs": "csharp",
	".c":  "c", ".h": "c",
	".cpp": "cpp", ".hpp": "cpp", ".cc": "cpp",
	".rb": "ruby", ".php": "php", ".rs": "rust", ".swift": "swift", ".scala": "scala",
	".m": "objective-c", ".mm": "objective-c",
	".vue": "vue", ".svelte": "svelte",
	// Web and data.
	".css": "css", ".scss": "scss", ".less": "less",
	".html": "html", ".htm": "html",
	".json": "json", ".jsonc": "json",
	".yaml": "yaml", ".yml": "yaml",
	".toml": "toml", ".xml": "xml", ".sql": "sql", ".graphql": "graphql",
	// Shell and scripts.
	".sh": "shell", ".bash": "shell", ".zsh": "shell",
	".ps1": "powershell", ".bat": "batch", ".cmd": "batch",
	// Config, docs, credentials.
	".md": "markdown", ".markdown": "markdown", ".txt": "text",
	".ini": "ini", ".cfg": "ini", ".conf": "ini", ".properties": "properties",
	".env": "env",
	".pem": "certificate", ".key": "certificate", ".pub": "certificate",
}

// ExtensionLanguages returns a copy of the extension-to-language table so
// other packages (and tests) can enumerate everything discovery classifies
// without duplicating the map. The copy is fresh on every call; callers
// cannot mutate the classifier through it.
func ExtensionLanguages() map[string]string {
	out := make(map[string]string, len(extensionLanguages))
	for ext, lang := range extensionLanguages {
		out[ext] = lang
	}
	return out
}

// Language classifies a path into a normalized lowercase language name, or ""
// when the file is not a scan candidate. Extensions are matched first; a few
// dotfile and extension-less basenames (.env*, Dockerfile*) are classified by
// name because their "extension" is either the whole filename (".env.local"
// has extension ".local") or absent ("Dockerfile").
func Language(path string) string {
	if lang, ok := extensionLanguages[strings.ToLower(filepath.Ext(path))]; ok {
		return lang
	}
	switch base := strings.ToLower(filepath.Base(path)); {
	case base == "dockerfile" || strings.HasPrefix(base, "dockerfile."):
		return "dockerfile"
	case base == ".env" || strings.HasPrefix(base, ".env."):
		return "env"
	}
	return ""
}

type Result struct {
	Files     []core.FileEntry `json:"files"`
	Languages map[string]int   `json:"languages"`
	Skipped   int              `json:"skipped"`
}

func Discover(ctx context.Context, root string, userExcludes []string) (Result, error) {
	userExcludes = WorkspaceExcludes(root, userExcludes)
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
		// Content heuristics: oversized candidates and large files whose
		// head is one enormous line (minified bundles under a source-looking
		// name) are generated output too (see artifacts.go).
		if skipGeneratedContent(path, info.Size()) {
			result.Skipped++
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
	userExcludes = WorkspaceExcludes(root, userExcludes)
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
		// Same content heuristics as Discover, so the Files page never
		// offers a file a scan would refuse to read.
		if skipGeneratedContent(path, info.Size()) {
			continue
		}
		items = append(items, core.FileEntry{RelativePath: rel, Language: language, SizeBytes: info.Size(), Selected: true})
	}
	return items, nil
}

// ExcludedByUser reports whether a workspace-relative path matches any user
// exclude pattern, with exactly the semantics Discover and Tree enforce.
// Scans reuse it to drop findings that directory-walking analyzers
// (gitleaks, checkov, sonarqube, trivy, osv) report inside configured-out
// paths their tools insist on traversing.
func ExcludedByUser(rel string, patterns []string) bool {
	return excludedByUser(rel, patterns)
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

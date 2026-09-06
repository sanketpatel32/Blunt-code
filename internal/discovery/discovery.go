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

// PolicyVersion versions the discovery classifier's behavior: which
// extensions map to which language, what counts as a license basename, which
// files are dependency inputs, and what the smart-skip layer excludes. Any
// change to classification bumps this number so scan snapshots (and anything
// comparing them) can see that two scans ran under different policies
// instead of silently assuming equivalence.
//
// 1: classification before 0.22.0.
// 2: terraform extensions, license basenames, dependency inputs, skip
//    reasons, and the generated-content smart-skip layer.
const PolicyVersion = 2

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
	// Infrastructure-as-code. Terraform is classified so IaC analyzers
	// (checkov, trivy) can route on it: before this, a pure-.tf workspace
	// had no language at all and no IaC scan ever applied to it.
	".tf": "terraform", ".tfvars": "terraform", ".hcl": "terraform",
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
// dotfile and extension-less basenames (.env*, Dockerfile*, LICENSE*) are
// classified by name because their "extension" is either the whole filename
// (".env.local" has extension ".local") or absent ("Dockerfile", "LICENSE").
func Language(path string) string {
	if lang, ok := extensionLanguages[strings.ToLower(filepath.Ext(path))]; ok {
		return lang
	}
	switch base := strings.ToLower(filepath.Base(path)); {
	case base == "dockerfile" || strings.HasPrefix(base, "dockerfile."):
		return "dockerfile"
	case base == ".env" || strings.HasPrefix(base, ".env."):
		return "env"
	case IsLicenseFileName(base):
		return "text"
	}
	return ""
}

// licenseFileNamePrefixes mark plain-text license artifacts. Extension-less
// or oddly-suffixed names (LICENSE, LICENSE-MIT, COPYING.LESSER with an
// unknown ".LESSER" extension) would otherwise be invisible to every
// language route, including the license scanner's own.
var licenseFileNamePrefixes = []string{"license", "licence", "copying", "notice"}

// IsLicenseFileName reports whether a lowercase basename is a license
// artifact: exactly one of the known names, or that name followed by a
// separator and a suffix (LICENSE.md, COPYING.LESSER, LICENSE-MIT). It never
// matches a longer word that merely starts with "license".
func IsLicenseFileName(lowerBase string) bool {
	for _, prefix := range licenseFileNamePrefixes {
		if lowerBase == prefix {
			return true
		}
		for _, sep := range []string{".", "-", "_"} {
			if strings.HasPrefix(lowerBase, prefix+sep) {
				return true
			}
		}
	}
	return false
}

type Result struct {
	Files     []core.FileEntry `json:"files"`
	Languages map[string]int   `json:"languages"`
	Skipped   int              `json:"skipped"`
	// SkipCounts breaks Skipped down by reason (symlink, excluded_default,
	// excluded_user, outside_root, generated_content) so a scan can explain
	// its coverage instead of one opaque number.
	SkipCounts map[string]int `json:"skip_counts"`
	// DependencyInputs lists workspace-relative dependency manifests and
	// lockfiles seen during the walk — including ones smart-skip excluded
	// from Files (lockfiles are inputs to osv/trivy, not per-file analyzers,
	// so their exclusion from Files must not hide them from routing).
	// Capped at maxDependencyInputs entries.
	DependencyInputs []string `json:"dependency_inputs"`
}

// Skip reason keys for SkipCounts.
const (
	SkipSymlink         = "symlink"
	SkipDefaultExcluded = "excluded_default"
	SkipUserExcluded    = "excluded_user"
	SkipOutsideRoot     = "outside_root"
	SkipGenerated       = "generated_content"
)

const maxDependencyInputs = 500

func Discover(ctx context.Context, root string, userExcludes []string) (Result, error) {
	userExcludes = WorkspaceExcludes(root, userExcludes)
	result := Result{Languages: map[string]int{}, SkipCounts: map[string]int{}}
	skip := func(reason string) {
		result.Skipped++
		result.SkipCounts[reason]++
	}
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
			skip(SkipSymlink)
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// Dependency inputs are tracked before any exclusion so a lockfile
		// hidden from Files (smart skip) still routes osv/trivy — their
		// vulnerability coverage is the reason lockfiles are kept at all.
		if !entry.IsDir() && len(result.DependencyInputs) < maxDependencyInputs && isDependencyInput(filepath.Base(rel)) {
			result.DependencyInputs = append(result.DependencyInputs, filepath.ToSlash(rel))
		}
		if DefaultExcluded(rel, entry.IsDir()) || excludedByUser(rel, userExcludes) {
			reason := SkipDefaultExcluded
			if !DefaultExcluded(rel, entry.IsDir()) {
				reason = SkipUserExcluded
			}
			skip(reason)
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
			skip(SkipOutsideRoot)
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
			skip(SkipGenerated)
			return nil
		}
		result.Languages[lang]++
		result.Files = append(result.Files, core.FileEntry{RelativePath: filepath.ToSlash(rel), Language: lang, SizeBytes: info.Size(), Selected: true})
		return nil
	})
	return result, err
}

// manifestFileNames are dependency manifests (not lockfiles): files that
// declare dependencies an ecosystem scanner reads for versions and licenses.
var manifestFileNames = map[string]struct{}{
	"package.json": {}, "requirements.txt": {}, "pyproject.toml": {}, "pipfile": {},
	"go.mod": {}, "cargo.toml": {}, "composer.json": {}, "pom.xml": {},
	"gemfile": {}, "packages.config": {}, "deno.lock": {}, "flake.lock": {},
	"mix.lock": {}, "pubspec.lock": {}, "chart.lock": {},
}

// isDependencyInput reports whether a basename is a dependency manifest or
// lockfile. It reuses the artifact tables (lockFileNames) so "lockfile as a
// dependency input" and "lockfile as an artifact" can never drift apart.
func isDependencyInput(base string) bool {
	lower := strings.ToLower(base)
	if _, ok := lockFileNames[lower]; ok {
		return true
	}
	_, ok := manifestFileNames[lower]
	return ok
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

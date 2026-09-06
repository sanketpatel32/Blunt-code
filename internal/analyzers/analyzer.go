// Package analyzers contains the normalized model and contracts shared by all
// local analysis adapters.  It deliberately has no dependency on HTTP, SQLite,
// or a particular process runner.
package analyzers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bluntcode/internal/workspace"
)

type Language string

// The original routing trio. These names are load-bearing (discovery
// classification, fingerprints, persisted snapshots); never rename them.
const (
	LanguagePython     Language = "python"
	LanguageJavaScript Language = "javascript"
	LanguageTypeScript Language = "typescript"
)

// Languages for everything else discovery classifies: code, web/data formats,
// shell scripts, config/docs, and credential material.
const (
	LanguageGo          Language = "go"
	LanguageJava        Language = "java"
	LanguageKotlin      Language = "kotlin"
	LanguageCSharp      Language = "csharp"
	LanguageC           Language = "c"
	LanguageCPP         Language = "cpp"
	LanguageRuby        Language = "ruby"
	LanguagePHP         Language = "php"
	LanguageRust        Language = "rust"
	LanguageSwift       Language = "swift"
	LanguageScala       Language = "scala"
	LanguageObjectiveC  Language = "objective-c"
	LanguageVue         Language = "vue"
	LanguageSvelte      Language = "svelte"
	LanguageCSS         Language = "css"
	LanguageSCSS        Language = "scss"
	LanguageLess        Language = "less"
	LanguageHTML        Language = "html"
	LanguageJSON        Language = "json"
	LanguageYAML        Language = "yaml"
	LanguageTOML        Language = "toml"
	LanguageXML         Language = "xml"
	LanguageSQL         Language = "sql"
	LanguageGraphQL     Language = "graphql"
	LanguageShell       Language = "shell"
	LanguagePowerShell  Language = "powershell"
	LanguageBatch       Language = "batch"
	LanguageMarkdown    Language = "markdown"
	LanguageText        Language = "text"
	LanguageINI         Language = "ini"
	LanguageProperties  Language = "properties"
	LanguageEnv         Language = "env"
	LanguageDockerfile  Language = "dockerfile"
	LanguageCertificate Language = "certificate"
	LanguageTerraform   Language = "terraform"
)

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

type Category string

const (
	CategoryBug             Category = "bug"
	CategoryVulnerability   Category = "vulnerability"
	CategorySecurity        Category = "security"
	CategoryCorrectness     Category = "correctness"
	CategoryMaintainability Category = "maintainability"
	CategoryCodeSmell       Category = "code_smell"
	CategoryPerformance     Category = "performance"
	CategoryComplexity      Category = "complexity"
	CategoryDuplication     Category = "duplication"
	CategoryStyle           Category = "style"
	CategoryOther           Category = "other"
)

type Finding struct {
	ID               string   `json:"id"`
	AnalyzerID       string   `json:"analyzer_id"`
	RuleID           string   `json:"rule_id"`
	Fingerprint      string   `json:"fingerprint"`
	Severity         Severity `json:"severity"`
	Category         Category `json:"category"`
	Title            string   `json:"title"`
	Message          string   `json:"message"`
	RelativePath     string   `json:"relative_path"`
	StartLine        int      `json:"start_line,omitempty"`
	StartColumn      int      `json:"start_column,omitempty"`
	EndLine          int      `json:"end_line,omitempty"`
	EndColumn        int      `json:"end_column,omitempty"`
	Remediation      string   `json:"remediation,omitempty"`
	DocumentationURL string   `json:"documentation_url,omitempty"`
	RawSeverity      string   `json:"raw_severity,omitempty"`
	// Status is populated when findings are read for a scan, relative to the
	// previous completed scan. It is not analyzer output and is not persisted.
	Status   string         `json:"status,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// SetFingerprint creates the V1 heuristic identity.  Do not include line
// numbers: a moved issue should remain comparable across scans.
func (f *Finding) SetFingerprint() {
	path := filepath.ToSlash(filepath.Clean(f.RelativePath))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		path = ""
	}
	message := strings.Join(strings.Fields(strings.ToLower(f.Message)), " ")
	s := strings.Join([]string{f.AnalyzerID, f.RuleID, path, message}, "\x00")
	hash := sha256.Sum256([]byte(s))
	f.Fingerprint = hex.EncodeToString(hash[:])
}

type Metric struct {
	AnalyzerID string  `json:"analyzer_id"`
	Key        string  `json:"key"`
	Label      string  `json:"label"`
	Value      float64 `json:"value"`
	Unit       string  `json:"unit,omitempty"`
}

type ToolStatus struct {
	Ready   bool   `json:"ready"`
	Version string `json:"version,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

type ToolEnvironment struct {
	ToolsDir string
	Offline  bool
}

// Scan tiers shared by orchestration and adapters. An empty profile string is
// treated as standard everywhere.
const (
	ProfileQuick    = "quick"
	ProfileStandard = "standard"
	ProfileDeep     = "deep"
	ProfilePentest  = "pentest"
)

type ScanRequest struct {
	WorkspaceRoot string
	Files         []string // absolute selected paths; adapter must never shell-concatenate them
	Languages     []Language
	Exclusions    []string
	WorkspaceID   string
	ScanID        string
	// Profile tells adapters which scan tier requested this analysis.
	// Adapters without tier-specific behavior may ignore it, and the empty
	// value must behave exactly like standard.
	Profile string
	// DependencyInputs lists workspace-relative dependency manifests and
	// lockfiles discovery saw, including ones excluded from Files (lockfiles
	// are smart-skipped as artifacts). Dependency-driven adapters (osv, trivy)
	// key their applicability off this list, not off source languages: a
	// workspace whose only dependency signal is package-lock.json has no
	// routable language file at all.
	DependencyInputs []string
}

type ProcessSpec struct {
	Executable string
	Args       []string
	Dir        string
	Env        map[string]string
}

type AnalyzerPlan struct {
	AnalyzerID string
	Version    string
	Commands   []ProcessSpec
	Metadata   map[string]any
}

type AnalyzerResult struct {
	Plan            AnalyzerPlan
	Stdout          []byte
	Stderr          []byte
	ExitCode        int
	StartedAt       time.Time
	FinishedAt      time.Time
	OutputTruncated bool
	// Warnings records degradations that do not fail the run but make its
	// coverage incomplete — historically the silently-dropped unparseable
	// output batch. A run with warnings completes, but the scan must surface
	// it (analyzer_runs.warning_count, completed_with_warnings, CLI exit 3)
	// instead of presenting a full-coverage result.
	Warnings []string
}

// MaxCommandArgumentCharacters stays well below the Windows 32,767-character
// command-line limit after executable paths and quoting are added.
const MaxCommandArgumentCharacters = 24_000

// FileArgumentBatches preserves the selected-file scope without creating a
// command line Windows cannot start. Each returned argument list includes the
// supplied prefix followed by a safe subset of files.
func FileArgumentBatches(prefix, files []string) [][]string {
	base := append([]string(nil), prefix...)
	length := commandArgumentLength(base)
	batch := append([]string(nil), base...)
	out := make([][]string, 0, 1)
	for _, file := range files {
		fileLength := len(file) + 3 // quoting and the separating space
		if len(batch) > len(base) && length+fileLength > MaxCommandArgumentCharacters {
			out = append(out, batch)
			batch = append([]string(nil), base...)
			length = commandArgumentLength(base)
		}
		batch = append(batch, file)
		length += fileLength
	}
	if len(batch) > len(base) || len(files) == 0 {
		out = append(out, batch)
	}
	return out
}

func commandArgumentLength(args []string) int {
	length := 0
	for _, arg := range args {
		length += len(arg) + 3
	}
	return length
}

type EventEmitter interface {
	Emit(context.Context, string, map[string]any)
}

// Analyzer keeps tool-specific parsing and lifecycle behavior outside scan orchestration.
type Analyzer interface {
	ID() string
	DisplayName() string
	SupportedLanguages() []Language
	Check(context.Context, ToolEnvironment) ToolStatus
	EnsureInstalled(context.Context, ToolEnvironment) error
	Plan(context.Context, ScanRequest) (AnalyzerPlan, error)
	Run(context.Context, AnalyzerPlan, EventEmitter) (AnalyzerResult, error)
	Normalize(context.Context, AnalyzerResult) ([]Finding, []Metric, error)
}

func HasLanguage(have []Language, wanted ...Language) bool {
	for _, h := range have {
		for _, w := range wanted {
			if h == w {
				return true
			}
		}
	}
	return false
}

// languageExtensions mirrors the discovery classifier's extension map (see
// discovery.ExtensionLanguages). It is duplicated here so the analyzers
// package stays importable without discovery; both must classify the same
// extensions — and apply the same .env*/Dockerfile basename rules — or
// per-language file filtering would disagree with how files were discovered.
var languageExtensions = map[string]Language{
	".py": LanguagePython, ".pyi": LanguagePython,
	".ts": LanguageTypeScript, ".tsx": LanguageTypeScript, ".mts": LanguageTypeScript, ".cts": LanguageTypeScript,
	".js": LanguageJavaScript, ".jsx": LanguageJavaScript, ".mjs": LanguageJavaScript, ".cjs": LanguageJavaScript,
	".go": LanguageGo, ".java": LanguageJava,
	".kt": LanguageKotlin, ".kts": LanguageKotlin,
	".cs": LanguageCSharp,
	".c":  LanguageC, ".h": LanguageC,
	".cpp": LanguageCPP, ".hpp": LanguageCPP, ".cc": LanguageCPP,
	".rb": LanguageRuby, ".php": LanguagePHP, ".rs": LanguageRust, ".swift": LanguageSwift, ".scala": LanguageScala,
	".m": LanguageObjectiveC, ".mm": LanguageObjectiveC,
	".vue": LanguageVue, ".svelte": LanguageSvelte,
	".tf": LanguageTerraform, ".tfvars": LanguageTerraform, ".hcl": LanguageTerraform,
	".css": LanguageCSS, ".scss": LanguageSCSS, ".less": LanguageLess,
	".html": LanguageHTML, ".htm": LanguageHTML,
	".json": LanguageJSON, ".jsonc": LanguageJSON,
	".yaml": LanguageYAML, ".yml": LanguageYAML,
	".toml": LanguageTOML, ".xml": LanguageXML, ".sql": LanguageSQL, ".graphql": LanguageGraphQL,
	".sh": LanguageShell, ".bash": LanguageShell, ".zsh": LanguageShell,
	".ps1": LanguagePowerShell, ".bat": LanguageBatch, ".cmd": LanguageBatch,
	".md": LanguageMarkdown, ".markdown": LanguageMarkdown, ".txt": LanguageText,
	".ini": LanguageINI, ".cfg": LanguageINI, ".conf": LanguageINI, ".properties": LanguageProperties,
	".env": LanguageEnv,
	".pem": LanguageCertificate, ".key": LanguageCertificate, ".pub": LanguageCertificate,
}

// languageOfPath classifies a single path with the same extension-first,
// basename-fallback rules discovery uses, so FilesForLanguages agrees with
// the walk that produced the file list.
func languageOfPath(file string) Language {
	if lang, ok := languageExtensions[strings.ToLower(filepath.Ext(file))]; ok {
		return lang
	}
	switch base := strings.ToLower(filepath.Base(file)); {
	case base == "dockerfile" || strings.HasPrefix(base, "dockerfile."):
		return LanguageDockerfile
	case base == ".env" || strings.HasPrefix(base, ".env."):
		return LanguageEnv
	case IsLicenseBaseName(base):
		return LanguageText
	}
	return ""
}

// IsLicenseBaseName mirrors discovery.IsLicenseFileName (extension-less
// LICENSE/COPYING/NOTICE artifacts classify as text) so FilesForLanguages
// agrees with the walk that found the file. The license adapter reuses it to
// pick license-like files out of its routed selection, whatever their
// extension (LICENSE.md is markdown, COPYING is text).
func IsLicenseBaseName(lowerBase string) bool {
	for _, prefix := range []string{"license", "licence", "copying", "notice"} {
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

// AllLanguages returns every language the shared classifier can produce,
// sorted for deterministic ordering. Adapters that want the broadest
// routable set (the built-in secrets detector) declare this instead of
// enumerating languages and drifting as discovery learns new ones.
func AllLanguages() []Language {
	set := map[Language]struct{}{LanguageDockerfile: {}, LanguageEnv: {}}
	for _, lang := range languageExtensions {
		set[lang] = struct{}{}
	}
	out := make([]Language, 0, len(set))
	for lang := range set {
		out = append(out, lang)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// FilesForLanguages filters file paths down to the extensions that belong to
// the given languages. Scan orchestration passes each adapter only the files
// it can actually lint, so a Python linter is never handed a minified
// JavaScript bundle to parse (found while dogfooding Blunt Code on itself:
// ruff produced 8 MB of syntax errors for cmd/bluntcode/static assets).
func FilesForLanguages(files []string, langs ...Language) []string {
	wanted := make(map[Language]bool, len(langs))
	for _, lang := range langs {
		wanted[lang] = true
	}
	out := make([]string, 0, len(files))
	for _, file := range files {
		if wanted[languageOfPath(file)] {
			out = append(out, file)
		}
	}
	return out
}

// RelativeInside reports whether path is inside root once symlinks and NTFS
// junctions are resolved. A path that is not even lexically under root is
// rejected without touching the disk; an empty root has nothing to contain
// against, so everything passes (adapters that need a root already refuse
// one in Plan). Adapters apply this before handing files to external tools
// or reading them in-process, so no caller can smuggle workspace-external
// files onto a scan.
func RelativeInside(root, path string) bool {
	if root == "" {
		return true
	}
	if ok, err := workspace.IsWithin(root, path); err != nil || !ok {
		return false
	}
	// Junction resolution runs before EvalSymlinks: EvalSymlinks errors on
	// any path that passes through a junction (and leaves the junction itself
	// in place), so it can neither detect nor survive the escape. A path that
	// does not exist (deleted since discovery, or a synthetic request) cannot
	// leak anything, so the junction-resolved lexical verdict stands.
	resolved := workspace.ResolveJunctionPath(path)
	if r, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = workspace.ResolveJunctionPath(r)
	}
	ok, err := workspace.IsWithin(root, resolved)
	return err == nil && ok
}

// FilesInside drops files that do not resolve inside root.
func FilesInside(root string, files []string) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		if RelativeInside(root, file) {
			out = append(out, file)
		}
	}
	return out
}

// FilesForLanguagesInside is FilesForLanguages plus the RelativeInside
// containment check, so the returned selection is both language-routable and
// provably inside the workspace root.
func FilesForLanguagesInside(root string, files []string, langs ...Language) []string {
	wanted := make(map[Language]bool, len(langs))
	for _, lang := range langs {
		wanted[lang] = true
	}
	out := make([]string, 0, len(files))
	for _, file := range files {
		if wanted[languageOfPath(file)] && RelativeInside(root, file) {
			out = append(out, file)
		}
	}
	return out
}

func SortedFindings(in []Finding) []Finding {
	out := append([]Finding(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RelativePath != out[j].RelativePath {
			return out[i].RelativePath < out[j].RelativePath
		}
		if out[i].StartLine != out[j].StartLine {
			return out[i].StartLine < out[j].StartLine
		}
		return out[i].RuleID < out[j].RuleID
	})
	return out
}

func ValidateFinding(f Finding) error {
	if f.AnalyzerID == "" || f.RuleID == "" || f.Message == "" || f.RelativePath == "" {
		return fmt.Errorf("finding requires analyzer, rule, message, and relative path")
	}
	return nil
}

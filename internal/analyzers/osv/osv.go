// Package osv adapts Google's OSV-Scanner (v2.x), a dependency vulnerability
// scanner, to the shared analyzer contract. Unlike the code analyzers, it does
// not inspect source files: it walks the workspace for dependency manifests
// and lockfiles (package-lock.json, requirements.txt, go.mod, Cargo.lock,
// composer.lock, pom.xml, Gemfile.lock, ...) and reports every known OSV
// advisory that affects a pinned version.
package osv

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"bluntcode/internal/analyzers"
)

const ID = "osv-dependencies"

type Adapter struct {
	Executable string
	Version    string
	// Offline switches the scan from the default OSV API queries to the
	// pre-downloaded local vulnerability databases (one zip per ecosystem,
	// fetched once with `scan source --offline-vulnerabilities
	// --download-offline-databases`). Without the databases the offline run
	// exits 127, so this mode is only useful alongside a prepared cache.
	Offline bool
	// DBCacheDir points OSV_SCANNER_LOCAL_DB_CACHE_DIRECTORY at a managed
	// cache directory so the offline databases travel with Blunt Code's tool
	// layout instead of the user-wide %LOCALAPPDATA% default. Empty keeps the
	// scanner's own default.
	DBCacheDir string
}

func New(executable, version string) *Adapter {
	return &Adapter{Executable: executable, Version: version}
}
func (a *Adapter) ID() string          { return ID }
func (a *Adapter) DisplayName() string { return "OSV Scanner" }

// SupportedLanguages lists the dependency-manifest ecosystems OSV-Scanner
// understands that the shared Language enum can also express. It is wider
// than the original routing trio on purpose: a lockfile vulnerability concern
// applies to any packaged ecosystem, not just Python and web code. Dart and
// Swift are absent because the enum has no language for them.
func (a *Adapter) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{
		analyzers.LanguagePython,     // PyPI: requirements.txt, poetry.lock, uv.lock
		analyzers.LanguageJavaScript, // npm: package-lock.json, yarn.lock, pnpm-lock.yaml
		analyzers.LanguageTypeScript,
		analyzers.LanguageGo,     // go.mod / go.sum
		analyzers.LanguageJava,   // Maven pom.xml, gradle.lockfile
		analyzers.LanguageCSharp, // NuGet: packages.config, project.assets.json
		analyzers.LanguagePHP,    // Composer: composer.lock
		analyzers.LanguageRuby,   // RubyGems: Gemfile.lock
		analyzers.LanguageRust,   // crates.io: Cargo.lock
	}
}
func (a *Adapter) Check(_ context.Context, _ analyzers.ToolEnvironment) analyzers.ToolStatus {
	_, err := os.Stat(a.Executable)
	return analyzers.ToolStatus{Ready: err == nil, Version: a.Version, Detail: statusDetail(err)}
}
func (a *Adapter) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error {
	if a.Executable == "" {
		return fmt.Errorf("osv-scanner executable is not managed yet")
	}
	return nil
}

// Plan builds exactly one process: the scanner does its own recursive
// workspace walk (file lists from the selection would miss the lockfiles the
// walk exists to find), so there is nothing to batch and one command covers
// the whole workspace.
func (a *Adapter) Plan(_ context.Context, req analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	// Dependency analysis is deep-only: it queries external advisory
	// databases, so quick and standard tiers deliberately exclude it. A
	// command-less plan is returned instead of an error so the executor
	// records a clean no-op run (succeeded, zero findings) rather than a
	// failure on every standard scan. Run treats it the same way.
	if req.Profile != analyzers.ProfileDeep {
		return analyzers.AnalyzerPlan{AnalyzerID: ID, Version: a.Version, Metadata: map[string]any{"reason": "deep_only"}}, nil
	}
	if req.WorkspaceRoot == "" {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("osv scanner requires a workspace root")
	}
	if !analyzers.HasLanguage(req.Languages, a.SupportedLanguages()...) {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("osv scanner does not apply")
	}
	args := []string{
		"scan", "source",
		"--format", "json", // the report goes to stdout, exactly like ruff and semgrep
		"--recursive", // find nested manifests (monorepos, vendored services)
		"--no-ignore", // scan what is on disk; a shipped-but-gitignored lockfile is still deployed
		// Workspaces without any dependency manifest are normal (a pure Go
		// tool repo has no package-lock.json); without this flag those scans
		// exit 128 as errors. With it they exit 0 and report "results": null.
		"--allow-no-lockfiles",
	}
	if a.Offline {
		args = append(args, "--offline", "--offline-vulnerabilities")
	}
	args = append(args, ".")
	env := map[string]string(nil)
	if a.DBCacheDir != "" {
		env = map[string]string{"OSV_SCANNER_LOCAL_DB_CACHE_DIRECTORY": a.DBCacheDir}
	}
	return analyzers.AnalyzerPlan{
		AnalyzerID: ID,
		Version:    a.Version,
		Commands: []analyzers.ProcessSpec{{
			Executable: a.Executable,
			Args:       args,
			Dir:        req.WorkspaceRoot, // "." above resolves against the workspace
			Env:        env,
		}},
		Metadata: map[string]any{"offline": a.Offline},
	}, nil
}

// Run executes the single planned command. Command-less (non-deep) plans are
// the documented no-op: the executor then persists state "succeeded" with no
// findings and emits analyzer.completed with findings: 0.
func (a *Adapter) Run(ctx context.Context, p analyzers.AnalyzerPlan, emit analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	if len(p.Commands) == 0 {
		return analyzers.AnalyzerResult{Plan: p}, nil
	}
	return analyzers.RunDirect(ctx, p, emit)
}

// report mirrors the subset of the v2 JSON schema the adapter consumes.
// Results is null (not []) when the scan finds no package sources, so it must
// stay a slice rather than a concrete length.
type report struct {
	Results []result `json:"results"`
}
type result struct {
	Source   source     `json:"source"`
	Packages []pkgEntry `json:"packages"`
}
type source struct {
	Path string `json:"path"` // absolute, forward slashes: C:/ws/package-lock.json
	Type string `json:"type"` // lockfile, lockfile-ish, or unknown (extracted)
}
type pkgEntry struct {
	Package         pkgInfo         `json:"package"`
	Groups          []groupEntry    `json:"groups"`
	Vulnerabilities []vulnerability `json:"vulnerabilities"`
}
type pkgInfo struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Ecosystem string `json:"ecosystem"`
}

// groupEntry clusters aliased advisories (GHSA + CVE + PYSEC ids for the same
// bug) and carries the highest CVSS score across the cluster as a string.
type groupEntry struct {
	IDs         []string `json:"ids"`
	MaxSeverity string   `json:"max_severity"`
}
type vulnerability struct {
	ID      string   `json:"id"`
	Summary string   `json:"summary"`
	Aliases []string `json:"aliases"`
	// DatabaseSpecific.Severity is labeled by GitHub advisories
	// (LOW/MODERATE/HIGH/CRITICAL) and is absent entirely for other records
	// (PYSEC entries in the offline databases ship no database_specific at
	// all) - severity() falls back to the alias group's max CVSS score.
	DatabaseSpecific struct {
		Severity string `json:"severity"`
	} `json:"database_specific"`
}

// Normalize converts one OSV-Scanner JSON report into findings. The scanner
// exits 1 when it finds vulnerabilities (like semgrep), 0 when it finds none.
func (a *Adapter) Normalize(_ context.Context, r analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	if r.ExitCode != 0 && r.ExitCode != 1 {
		return nil, nil, fmt.Errorf("osv-scanner exited %d: %s", r.ExitCode, strings.TrimSpace(string(r.Stderr)))
	}
	var raw report
	if len(r.Stdout) == 0 {
		return nil, nil, nil
	}
	if err := json.Unmarshal(r.Stdout, &raw); err != nil {
		return nil, nil, fmt.Errorf("parse osv-scanner JSON: %w", err)
	}
	root := ""
	if len(r.Plan.Commands) > 0 {
		root = r.Plan.Commands[0].Dir
	}
	out := make([]analyzers.Finding, 0)
	seen := make(map[string]bool)
	for _, res := range raw.Results {
		path := relative(root, res.Source.Path)
		for _, p := range res.Packages {
			for _, v := range p.Vulnerabilities {
				// The same pinned package can be reached through several
				// sources (a manifest extracted twice, or repeated lockfiles);
				// keep one finding per (package, version, advisory id).
				key := p.Package.Name + "\x00" + p.Package.Version + "\x00" + v.ID
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, finding(path, res.Source.Type, p.Package, p.Groups, v))
			}
		}
	}
	// The walk order depends on filesystem iteration; sort so two scans of the
	// same workspace produce byte-identical finding lists.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RelativePath != out[j].RelativePath {
			return out[i].RelativePath < out[j].RelativePath
		}
		if out[i].RuleID != out[j].RuleID {
			return out[i].RuleID < out[j].RuleID
		}
		return out[i].Message < out[j].Message
	})
	return out, nil, nil
}

// finding maps one advisory on one pinned package. Line information does not
// exist for manifest-level findings, so StartLine stays unset. The advisory
// id doubles as the rule id: it is the stable "what fired" key.
func finding(path, sourceType string, p pkgInfo, groups []groupEntry, v vulnerability) analyzers.Finding {
	sev, raw, cvss := severity(v, groups)
	f := analyzers.Finding{
		AnalyzerID:       ID,
		RuleID:           v.ID,
		Severity:         sev,
		Category:         analyzers.CategoryVulnerability, // a known-CVE dependency pin, not a code smell
		Title:            v.ID,
		Message:          message(p, v),
		RelativePath:     path,
		DocumentationURL: "https://osv.dev/" + v.ID, // the scanner's own link format
		RawSeverity:      raw,
		Metadata: map[string]any{
			"package":     p.Name,
			"ecosystem":   p.Ecosystem,
			"source_type": sourceType,
		},
	}
	if p.Version != "" {
		f.Metadata["package_version"] = p.Version
	}
	if len(v.Aliases) > 0 {
		f.Metadata["aliases"] = v.Aliases // CVE ids for cross-referencing
	}
	if cvss != "" {
		f.Metadata["cvss_max"] = cvss
	}
	f.SetFingerprint()
	return f
}

// message renders "GHSA-xxxx in lodash@4.17.15: summary"; PYSEC records often
// ship no summary, so the package coordinates carry the finding alone.
func message(p pkgInfo, v vulnerability) string {
	msg := v.ID + " in " + p.Name + "@" + p.Version
	if s := strings.TrimSpace(v.Summary); s != "" {
		msg += ": " + s
	}
	return msg
}

// severity maps the advisory onto the shared enum. Precedence: the labeled
// database_specific severity (GHSA), then a numeric severity string if a
// database ever ships one ("7.5"), then the advisory group's max CVSS score
// (the only signal PYSEC records carry), and finally low - an unknown
// severity must not be guessed high.
func severity(v vulnerability, groups []groupEntry) (sev analyzers.Severity, raw, cvss string) {
	raw = strings.TrimSpace(v.DatabaseSpecific.Severity)
	switch strings.ToUpper(raw) {
	case "CRITICAL":
		return analyzers.SeverityCritical, raw, ""
	case "HIGH":
		return analyzers.SeverityHigh, raw, ""
	case "MODERATE", "MEDIUM": // GitHub says MODERATE; the enum says medium
		return analyzers.SeverityMedium, raw, ""
	case "LOW":
		return analyzers.SeverityLow, raw, ""
	}
	score := -1.0
	if n, err := strconv.ParseFloat(raw, 64); err == nil {
		score = n
		cvss = raw
	} else if group := groupScore(groups, v.ID); group != "" {
		if n, err := strconv.ParseFloat(group, 64); err == nil {
			score = n
			cvss = group
		}
	}
	switch {
	case score >= 9.0:
		return analyzers.SeverityCritical, raw, cvss
	case score >= 7.0:
		return analyzers.SeverityHigh, raw, cvss
	case score >= 4.0:
		return analyzers.SeverityMedium, raw, cvss
	case score > 0:
		return analyzers.SeverityLow, raw, cvss
	}
	return analyzers.SeverityLow, raw, ""
}

// groupScore returns the max CVSS score string of the alias group containing
// the advisory id, or "" when the id is ungrouped or unscored.
func groupScore(groups []groupEntry, id string) string {
	for _, g := range groups {
		for _, gid := range g.IDs {
			if gid == id {
				return strings.TrimSpace(g.MaxSeverity)
			}
		}
	}
	return ""
}

// relative anchors an absolute report path (forward slashes) to the workspace
// root. A source outside the root keeps its full path rather than a ../ climb.
func relative(root, path string) string {
	if root != "" {
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}
func statusDetail(err error) string {
	if err == nil {
		return ""
	}
	return "managed executable not found"
}

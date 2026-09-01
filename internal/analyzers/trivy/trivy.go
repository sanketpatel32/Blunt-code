package trivy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bluntcode/internal/analyzers"
)

// ID carries the container/IaC domain prefix so trivy never collides with a
// future code-focused analyzer of the same vendor tool.
const ID = "container-trivy"

// Plan metadata keys. trivy writes its report to --output instead of stdout,
// so Run needs the reserved path to pull the JSON back into the result.
const (
	planKeyOutput = "output_path"
	planKeyWarmDB = "db_cache_warm"
)

// statusFail is the only misconfiguration status that becomes a finding;
// trivy also reports PASS entries when --include-successes is used.
const statusFail = "FAIL"

// Message and title bounds. trivy misconfiguration descriptions run to a few
// hundred characters and vulnerability descriptions to a few thousand, so
// both are truncated before they reach findings, fingerprints, and SQLite.
const (
	maxTitleRunes   = 200
	maxMessageRunes = 500
)

type Adapter struct {
	Executable string
	Version    string
	// CacheDir optionally pins trivy's cache (vulnerability DB + checks
	// bundle) by setting TRIVY_CACHE_DIR on the planned process, keeping the
	// ~1.3 GB decompressed DB inside the app's data dir. Empty means trivy's
	// own resolution: TRIVY_CACHE_DIR, then %LOCALAPPDATA%\trivy on Windows.
	CacheDir string
}

func New(executable, version string) *Adapter {
	return &Adapter{Executable: executable, Version: version}
}
func (a *Adapter) ID() string          { return ID }
func (a *Adapter) DisplayName() string { return "Trivy" }

// SupportedLanguages routes by the manifest and infrastructure formats the
// shared classifier knows: Dockerfiles, YAML, JSON, and TOML. Terraform has
// no entry in the Language enum (discovery does not classify .tf at all), so
// a terraform-only workspace will not select trivy; the closest declarative
// formats above do, and trivy itself scans the whole workspace root anyway.
func (a *Adapter) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguageDockerfile, analyzers.LanguageYAML, analyzers.LanguageJSON, analyzers.LanguageTOML}
}
func (a *Adapter) Check(_ context.Context, _ analyzers.ToolEnvironment) analyzers.ToolStatus {
	_, err := os.Stat(a.Executable)
	return analyzers.ToolStatus{Ready: err == nil, Version: a.Version, Detail: statusDetail(err)}
}
func (a *Adapter) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error {
	if a.Executable == "" {
		return fmt.Errorf("trivy executable is not managed yet")
	}
	return nil
}

func (a *Adapter) Plan(_ context.Context, req analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	// Deep tier only: trivy's first run downloads a ~110 MB vulnerability DB
	// (1.3 GB decompressed) and can take minutes, so it is far too heavy for
	// quick/standard scans. Following the ruff/semgrep convention, Plan
	// refuses with "trivy does not apply"; the executor records that as a
	// failed analyzer run for this scan (state "failed", error text, and an
	// analyzer.failed event) unless the file-language gate above already
	// skipped the adapter outright.
	if req.Profile != analyzers.ProfileDeep {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("trivy does not apply")
	}
	// The orchestrator already narrows req.Files by SupportedLanguages, but
	// the adapter filters again (the ruff convention) so no caller can start
	// a whole-workspace trivy sweep over a selection it does not belong to.
	if len(analyzers.FilesForLanguages(req.Files, a.SupportedLanguages()...)) == 0 {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("trivy does not apply")
	}
	if req.WorkspaceRoot == "" {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("workspace root is required")
	}
	// Reserve the report file up front (the biome config pattern): trivy is
	// given an absolute --output path, so the JSON never rides through
	// stdout where the 8 MB capture limit would clip large reports.
	tmp, err := os.CreateTemp(os.TempDir(), "bluntcode-trivy-*.json")
	if err != nil {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("reserve trivy output file: %w", err)
	}
	outputPath := tmp.Name()
	tmp.Close()

	warm := a.dbCacheWarm()
	args := []string{"fs", "--format", "json", "--output", outputPath, "--scanners", "vuln,secret,misconfig"}
	if warm {
		// Verified against trivy 0.74.0: with a populated cache the flag is
		// 3-4x faster and produces identical findings, while on a cold cache
		// trivy exits 1 with "The first run cannot skip downloading DB" and
		// writes no report - so it is added only when the DB is known to
		// exist locally.
		args = append(args, "--skip-db-update")
	}
	// trivy fs takes a directory, never a shell-concatenated file list; the
	// command line stays short no matter how large the selection is.
	args = append(args, ".")

	command := analyzers.ProcessSpec{Executable: a.Executable, Args: args, Dir: req.WorkspaceRoot}
	if a.CacheDir != "" {
		command.Env = map[string]string{"TRIVY_CACHE_DIR": a.CacheDir}
	}
	return analyzers.AnalyzerPlan{
		AnalyzerID: ID,
		Version:    a.Version,
		Commands:   []analyzers.ProcessSpec{command},
		Metadata:   map[string]any{planKeyOutput: outputPath, planKeyWarmDB: warm},
	}, nil
}

// dbCacheWarm reports whether the vulnerability DB is already present in the
// cache directory trivy will use, i.e. whether --skip-db-update is safe.
// trivy writes db/metadata.json only after a successful download.
func (a *Adapter) dbCacheWarm() bool {
	dir := a.cacheDir()
	if dir == "" {
		return false
	}
	meta, err := os.Stat(filepath.Join(dir, "db", "metadata.json"))
	if err != nil || meta.IsDir() {
		return false
	}
	db, err := os.Stat(filepath.Join(dir, "db", "trivy.db"))
	return err == nil && !db.IsDir() && db.Size() > 0
}

// cacheDir mirrors trivy's own resolution order so the adapter's warm check
// and the child process always look at the same directory: an explicit
// CacheDir, TRIVY_CACHE_DIR, then the OS user cache dir plus "trivy".
func (a *Adapter) cacheDir() string {
	if a.CacheDir != "" {
		return a.CacheDir
	}
	if env := os.Getenv("TRIVY_CACHE_DIR"); env != "" {
		return env
	}
	if user, err := os.UserCacheDir(); err == nil {
		return filepath.Join(user, "trivy")
	}
	return ""
}

func (a *Adapter) Run(ctx context.Context, p analyzers.AnalyzerPlan, emit analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	result, err := analyzers.RunDirect(ctx, p, emit)
	// The report lives in the reserved --output file, so pull it into Stdout
	// (the channel Normalize reads) and remove the file whether or not the
	// scan succeeded; failed runs must not leak temp files.
	if path, ok := p.Metadata[planKeyOutput].(string); ok && path != "" {
		if data, readErr := os.ReadFile(path); readErr == nil {
			result.Stdout = data
		}
		_ = os.Remove(path)
	}
	return result, err
}

type rawReport struct {
	SchemaVersion int         `json:"SchemaVersion"`
	Results       []rawResult `json:"Results"`
}
type rawResult struct {
	Target            string         `json:"Target"`
	Class             string         `json:"Class"`
	Type              string         `json:"Type"`
	Misconfigurations []rawMisconfig `json:"Misconfigurations"`
	Vulnerabilities   []rawVuln      `json:"Vulnerabilities"`
	Secrets           []rawSecret    `json:"Secrets"`
}
type rawMisconfig struct {
	ID            string   `json:"ID"`
	AVDID         string   `json:"AVDID"`
	Title         string   `json:"Title"`
	Message       string   `json:"Message"`
	Description   string   `json:"Description"`
	Resolution    string   `json:"Resolution"`
	Severity      string   `json:"Severity"`
	Status        string   `json:"Status"`
	PrimaryURL    string   `json:"PrimaryURL"`
	CauseMetadata rawCause `json:"CauseMetadata"`
}
type rawCause struct {
	StartLine int `json:"StartLine"`
	EndLine   int `json:"EndLine"`
}
type rawVuln struct {
	VulnerabilityID  string `json:"VulnerabilityID"`
	PkgName          string `json:"PkgName"`
	InstalledVersion string `json:"InstalledVersion"`
	FixedVersion     string `json:"FixedVersion"`
	Status           string `json:"Status"`
	Title            string `json:"Title"`
	Severity         string `json:"Severity"`
	PrimaryURL       string `json:"PrimaryURL"`
}
type rawSecret struct {
	RuleID    string `json:"RuleID"`
	Category  string `json:"Category"`
	Severity  string `json:"Severity"`
	Title     string `json:"Title"`
	StartLine int    `json:"StartLine"`
	EndLine   int    `json:"EndLine"`
}

func (a *Adapter) Normalize(_ context.Context, result analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	// trivy exits 0 even when the report is full of findings; any nonzero
	// exit is fatal (DB download failure, unreadable target) and writes no
	// report at all.
	if result.ExitCode != 0 {
		return nil, nil, fmt.Errorf("trivy exited %d: %s", result.ExitCode, truncate(strings.TrimSpace(string(result.Stderr)), maxMessageRunes))
	}
	var raw rawReport
	if len(result.Stdout) == 0 {
		return nil, nil, nil
	}
	if err := json.Unmarshal(result.Stdout, &raw); err != nil {
		return nil, nil, fmt.Errorf("parse trivy JSON: %w", err)
	}
	out := make([]analyzers.Finding, 0)
	for _, r := range raw.Results {
		path := relative(result.Plan, r.Target)
		for _, m := range r.Misconfigurations {
			// Only failures become findings; passed checks would just be
			// noise (and appear only under --include-successes anyway).
			if m.Status != statusFail {
				continue
			}
			id := m.ID
			if id == "" {
				id = m.AVDID
			}
			if id == "" {
				continue
			}
			f := analyzers.Finding{
				AnalyzerID: ID, RuleID: id, Severity: severity(m.Severity), Category: analyzers.CategorySecurity,
				Title:        truncate(m.Title, maxTitleRunes),
				Message:      truncate(fmt.Sprintf("%s: %s. %s", id, m.Title, m.Description), maxMessageRunes),
				RelativePath: path,
				StartLine:    m.CauseMetadata.StartLine, EndLine: m.CauseMetadata.EndLine,
				Remediation:      truncate(m.Resolution, maxMessageRunes),
				DocumentationURL: m.PrimaryURL,
				RawSeverity:      m.Severity,
				Metadata:         map[string]any{"file_type": r.Type},
			}
			f.SetFingerprint()
			out = append(out, f)
		}
		for _, v := range r.Vulnerabilities {
			if v.VulnerabilityID == "" {
				continue
			}
			remediation := ""
			if v.FixedVersion != "" {
				remediation = fmt.Sprintf("Upgrade %s to %s", v.PkgName, v.FixedVersion)
			}
			f := analyzers.Finding{
				AnalyzerID: ID, RuleID: v.VulnerabilityID, Severity: severity(v.Severity), Category: analyzers.CategoryVulnerability,
				Title:            truncate(v.Title, maxTitleRunes),
				Message:          truncate(fmt.Sprintf("%s: %s@%s - %s", v.VulnerabilityID, v.PkgName, v.InstalledVersion, v.Title), maxMessageRunes),
				RelativePath:     path,
				Remediation:      remediation,
				DocumentationURL: v.PrimaryURL,
				RawSeverity:      v.Severity,
				Metadata: map[string]any{
					"package": v.PkgName, "installed_version": v.InstalledVersion, "fixed_version": v.FixedVersion, "status": v.Status,
				},
			}
			f.SetFingerprint()
			out = append(out, f)
		}
		for _, s := range r.Secrets {
			if s.RuleID == "" {
				continue
			}
			// The matched value must never leave the raw report; trivy masks
			// it in JSON output, but the adapter does not rely on that and
			// keeps the message to rule metadata only.
			f := analyzers.Finding{
				AnalyzerID: ID, RuleID: s.RuleID, Severity: severity(s.Severity), Category: analyzers.CategorySecurity,
				Title:        truncate(s.Title, maxTitleRunes),
				Message:      truncate(fmt.Sprintf("%s: potential %s secret detected (matched value not captured)", s.Title, s.Category), maxMessageRunes),
				RelativePath: path,
				StartLine:    s.StartLine, EndLine: s.EndLine,
				RawSeverity: s.Severity,
				Metadata:    map[string]any{"category": s.Category},
			}
			f.SetFingerprint()
			out = append(out, f)
		}
	}
	return out, nil, nil
}

func severity(s string) analyzers.Severity {
	switch strings.ToUpper(s) {
	case "CRITICAL":
		return analyzers.SeverityCritical
	case "HIGH":
		return analyzers.SeverityHigh
	case "MEDIUM":
		return analyzers.SeverityMedium
	case "LOW":
		return analyzers.SeverityLow
	default:
		return analyzers.SeverityInfo
	}
}

// relative maps a trivy Target to a workspace-relative slash path. Targets
// are relative to the scanned directory already ("Dockerfile",
// "package-lock.json", "." for a terraform root module), so this only has to
// clean separators and fold any absolute target back under the plan's Dir.
func relative(p analyzers.AnalyzerPlan, path string) string {
	if path == "" {
		return "."
	}
	if filepath.IsAbs(path) && len(p.Commands) > 0 {
		if r, err := filepath.Rel(p.Commands[0].Dir, path); err == nil {
			return filepath.ToSlash(r)
		}
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return strings.TrimSpace(string(runes[:max])) + "..."
}

func statusDetail(err error) string {
	if err == nil {
		return ""
	}
	return "managed executable not found"
}

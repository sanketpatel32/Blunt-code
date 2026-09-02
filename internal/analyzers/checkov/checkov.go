// Package checkov adapts the Checkov infrastructure-as-code policy scanner
// (terraform, dockerfile, cloudformation, kubernetes manifests).
//
// Unlike semgrep, a uv-installed checkov has no hermetic launcher on Windows:
// uv writes a checkov.cmd batch shim that resolves python.exe from PATH, so
// the executable New receives is the virtual environment's Python interpreter
// and every invocation goes through `python -m checkov.main`. The scan is a
// single directory pass (-d <workspace root>) that writes one JSON report to
// a per-scan temp directory; Run promotes that file into AnalyzerResult.Stdout
// so Normalize stays a pure parser over captured output.
//
// Checkov is the deep-profile IaC pass: Plan refuses every other profile, so
// the orchestrator must not schedule it outside deep scans (see the package
// report for how internal/scans/exec.go records that refusal).
package checkov

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"bluntcode/internal/analyzers"
)

const ID = "iac-checkov"

// frameworks limits the scan to the IaC flavors whose files Blunt Code can
// route today (dockerfile basenames, yaml/json manifests). Terraform files
// are covered by the directory scan even though no Language enum value
// exists for them yet.
const frameworks = "terraform,dockerfile,kubernetes,cloudformation"

// moduleArgs invoke checkov inside the managed virtual environment. The
// generated bin/checkov script is not directly executable on Windows and
// checkov.cmd depends on whatever python.exe happens to be on PATH, so the
// interpreter-module form is the only hermetic entry point.
var moduleArgs = []string{"-m", "checkov.main"}

const (
	// outputDirPrefix namespaces the per-scan report directory checkov writes
	// to; the scan ID keeps concurrent scans from colliding.
	outputDirPrefix = "bluntcode-checkov-"
	// resultFileName is the file checkov creates inside --output-file-path
	// for the json output format.
	resultFileName = "results_json.json"
	// versionProbeTimeout bounds the `--version` probe in Check. Importing
	// checkov costs several seconds of Python startup, so the probe is
	// best-effort: on timeout the manifest version is kept.
	versionProbeTimeout = 15 * time.Second
)

type Adapter struct{ Executable, Version string }

func New(executable, version string) *Adapter {
	return &Adapter{Executable: executable, Version: version}
}
func (a *Adapter) ID() string          { return ID }
func (a *Adapter) DisplayName() string { return "Checkov" }

// SupportedLanguages declares the routable IaC formats. yaml covers
// kubernetes and cloudformation manifests, json covers cloudformation
// templates, and dockerfile covers Dockerfile basenames. Terraform (.tf)
// has no Language enum value, so a pure-.tf workspace is not routed to
// checkov today; when a yaml, json, or dockerfile file selects the
// analyzer, the directory scan still covers .tf files in the workspace.
func (a *Adapter) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguageYAML, analyzers.LanguageJSON, analyzers.LanguageDockerfile}
}

// Check verifies the managed interpreter exists, then makes one best-effort
// version probe. `python -m checkov.main --version` imports the whole
// package (multiple seconds), so a timeout or failure falls back to the
// pinned manifest version instead of failing the scan.
func (a *Adapter) Check(ctx context.Context, _ analyzers.ToolEnvironment) analyzers.ToolStatus {
	if _, err := os.Stat(a.Executable); err != nil {
		return analyzers.ToolStatus{Version: a.Version, Detail: "managed executable not found"}
	}
	probeCtx, cancel := context.WithTimeout(ctx, versionProbeTimeout)
	defer cancel()
	if probed := a.probeVersion(probeCtx); probed != "" {
		return analyzers.ToolStatus{Ready: true, Version: probed}
	}
	return analyzers.ToolStatus{Ready: true, Version: a.Version}
}

func (a *Adapter) probeVersion(ctx context.Context) string {
	if ctx.Err() != nil {
		return ""
	}
	probeArgs := append(append([]string(nil), moduleArgs...), "--version")
	cmd := exec.CommandContext(ctx, a.Executable, probeArgs...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (a *Adapter) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error {
	if a.Executable == "" {
		return fmt.Errorf("checkov executable is not managed yet")
	}
	return nil
}

func (a *Adapter) Plan(_ context.Context, req analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	// Checkov's full-framework policy sweep is the deep tier: every other
	// profile (including the empty default) skips it. The error text is what
	// internal/scans/exec.go records verbatim when it saves the failed run.
	if req.Profile != analyzers.ProfileDeep && req.Profile != analyzers.ProfilePentest {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("checkov requires the deep or pentest profile")
	}
	// The orchestrator already routes by language, but the adapter filters
	// again (the convention ruff and semgrep set) so no caller can start a
	// full-workspace directory scan from unrelated files.
	if len(analyzers.FilesForLanguages(req.Files, a.SupportedLanguages()...)) == 0 {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("checkov does not apply")
	}
	outputDir := filepath.Join(os.TempDir(), outputDirPrefix+req.ScanID)
	args := append([]string(nil), moduleArgs...)
	args = append(args,
		"-d", req.WorkspaceRoot,
		"-o", "json",
		"--output-file-path", outputDir,
		"--framework", frameworks,
		// --quiet keeps passed checks out of the report file and disables
		// progress bars; only failed_checks are normalized anyway.
		"--quiet",
	)
	return analyzers.AnalyzerPlan{
		AnalyzerID: ID,
		Version:    a.Version,
		Commands: []analyzers.ProcessSpec{{
			Executable: a.Executable,
			Args:       args,
			Dir:        req.WorkspaceRoot,
			Env: map[string]string{
				// RunDirect deletes environment keys whose override is empty:
				// neutralize ambient Python configuration and any Prisma
				// Cloud credentials so the managed interpreter and the
				// bundled open-source policy set are what the scan uses.
				"PYTHONPATH":    "",
				"PYTHONHOME":    "",
				"CKV_API_TOKEN": "",
				"BC_API_KEY":    "",
			},
		}},
		Metadata: map[string]any{"output_dir": outputDir, "frameworks": frameworks},
	}, nil
}

func (a *Adapter) Run(ctx context.Context, p analyzers.AnalyzerPlan, emit analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	if len(p.Commands) != 1 {
		return analyzers.AnalyzerResult{Plan: p}, fmt.Errorf("checkov plan must provide exactly one command")
	}
	outputDir, _ := p.Metadata["output_dir"].(string)
	if outputDir == "" {
		return analyzers.RunDirect(ctx, p, emit)
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return analyzers.AnalyzerResult{Plan: p}, err
	}
	defer os.RemoveAll(outputDir)
	result, err := analyzers.RunDirect(ctx, p, emit)
	if err != nil {
		return result, err
	}
	// The report file is authoritative; the console stdout echoes the same
	// JSON but can interleave with first-run diagnostics from checkov's
	// policy metadata download.
	if report, readErr := os.ReadFile(filepath.Join(outputDir, resultFileName)); readErr == nil && json.Valid(report) {
		result.Stdout = report
	}
	return result, nil
}

// report is one framework section of checkov's JSON output. The top level
// is an array of these, one per framework that found files to scan.
type report struct {
	CheckType string `json:"check_type"`
	Results   struct {
		FailedChecks []failedCheck `json:"failed_checks"`
	} `json:"results"`
	Summary struct {
		Passed    int `json:"passed"`
		Failed    int `json:"failed"`
		Skipped   int `json:"skipped"`
		Resources int `json:"resource_count"`
	} `json:"summary"`
}

type failedCheck struct {
	CheckID       string `json:"check_id"`
	CheckName     string `json:"check_name"`
	Severity      string `json:"severity"`
	Guideline     string `json:"guideline"`
	FilePath      string `json:"file_path"`
	FileLineRange []int  `json:"file_line_range"`
	Resource      string `json:"resource"`
	BCCheckID     string `json:"bc_check_id"`
}

// Normalize turns failed checks into security findings. Checkov exits 1 when
// checks fail (0 when clean), mirroring semgrep and ruff; both are success.
// Passed and skipped checks are ignored entirely.
func (a *Adapter) Normalize(_ context.Context, r analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	if r.ExitCode != 0 && r.ExitCode != 1 {
		return nil, nil, fmt.Errorf("checkov exited %d: %s", r.ExitCode, strings.TrimSpace(string(r.Stderr)))
	}
	if len(r.Stdout) == 0 {
		return nil, nil, nil
	}
	reports, err := decodeReports(r.Stdout)
	if err != nil {
		return nil, nil, fmt.Errorf("parse checkov JSON: %w", err)
	}
	scanDir := ""
	if len(r.Plan.Commands) > 0 {
		scanDir = r.Plan.Commands[0].Dir
	}
	findings := make([]analyzers.Finding, 0)
	var passed, resources int
	for _, rep := range reports {
		passed += rep.Summary.Passed
		resources += rep.Summary.Resources
		for _, c := range rep.Results.FailedChecks {
			f := analyzers.Finding{
				AnalyzerID:       ID,
				RuleID:           c.CheckID,
				Severity:         severity(c.Severity, c.CheckID),
				Category:         analyzers.CategorySecurity,
				Title:            c.CheckName,
				Message:          fmt.Sprintf("%s: %s", c.CheckID, c.CheckName),
				RelativePath:     relative(scanDir, c.FilePath),
				DocumentationURL: c.Guideline,
				RawSeverity:      c.Severity,
				Metadata:         map[string]any{"check_type": rep.CheckType, "resource": c.Resource},
			}
			if len(c.FileLineRange) > 0 {
				f.StartLine = c.FileLineRange[0]
			}
			if len(c.FileLineRange) > 1 {
				f.EndLine = c.FileLineRange[1]
			}
			if c.BCCheckID != "" {
				f.Metadata["bc_check_id"] = c.BCCheckID
			}
			f.SetFingerprint()
			findings = append(findings, f)
		}
	}
	metrics := []analyzers.Metric{
		{AnalyzerID: ID, Key: "checks_failed", Label: "Failed checks", Value: float64(len(findings))},
		{AnalyzerID: ID, Key: "checks_passed", Label: "Passed checks", Value: float64(passed)},
		{AnalyzerID: ID, Key: "resources_scanned", Label: "Resources scanned", Value: float64(resources)},
	}
	return findings, metrics, nil
}

func decodeReports(data []byte) ([]report, error) {
	var reports []report
	if err := json.Unmarshal(data, &reports); err == nil {
		return reports, nil
	}
	// Older checkov releases emitted a single report object instead of an
	// array; accept both shapes.
	var single report
	if err := json.Unmarshal(data, &single); err != nil {
		return nil, err
	}
	return []report{single}, nil
}

// severity prefers checkov's per-check severity, which only exists when the
// Prisma Cloud policy metadata download succeeded. The default offline
// install reports no severity at all, so unmatched checks default to
// medium and the informational operational-hygiene checks opt down to low.
func severity(reported, checkID string) analyzers.Severity {
	switch strings.ToUpper(strings.TrimSpace(reported)) {
	case "CRITICAL":
		return analyzers.SeverityCritical
	case "HIGH":
		return analyzers.SeverityHigh
	case "MEDIUM":
		return analyzers.SeverityMedium
	case "LOW":
		return analyzers.SeverityLow
	case "INFO", "INFORMATIONAL":
		return analyzers.SeverityInfo
	}
	if informationalChecks[checkID] {
		return analyzers.SeverityLow
	}
	return analyzers.SeverityMedium
}

// informationalChecks are policy failures about operational hygiene rather
// than a security property of the infrastructure, so they report low until
// checkov supplies real severities.
var informationalChecks = map[string]bool{
	"CKV_DOCKER_2": true, // container image has no HEALTHCHECK
}

// relative converts checkov's scan-relative file_path (reported with a
// leading separator, e.g. "\main.tf" or "/Dockerfile") into a workspace-
// relative slash path.
func relative(scanDir, path string) string {
	if scanDir != "" && filepath.IsAbs(filepath.FromSlash(path)) {
		if rel, err := filepath.Rel(scanDir, path); err == nil {
			path = rel
		}
	}
	cleaned := strings.TrimLeft(filepath.ToSlash(filepath.Clean(path)), "/")
	if cleaned == "" || cleaned == "." {
		return filepath.ToSlash(path)
	}
	return cleaned
}

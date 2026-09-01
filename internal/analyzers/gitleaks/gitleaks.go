// Package gitleaks adapts the external gitleaks secrets scanner
// (https://github.com/gitleaks/gitleaks). Unlike the built-in secrets
// detector it ships no rules of its own: gitleaks' default ruleset — plus any
// .gitleaks.toml config and .gitleaksignore exceptions the workspace itself
// declares — does the detection, and this adapter only plans the run and
// normalizes the JSON report.
package gitleaks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bluntcode/internal/analyzers"
)

const ID = "gitleaks-secrets"

const remediation = "Rotate the exposed credential, remove it from the source and version history, and load the replacement from a secret manager or environment variable."

// plan metadata keys. gitleaks writes its report to a file (stdout stays
// empty), so the path must travel through the plan: Plan embeds it in both
// the command arguments and Metadata, and Run collects the file, moves its
// bytes into Stdout for Normalize, and deletes it.
const (
	planKeyReportPath = "report_path"
	planKeySource     = "source"
)

type Adapter struct {
	Executable string
	Version    string
}

func New(executable, version string) *Adapter {
	return &Adapter{Executable: executable, Version: version}
}
func (a *Adapter) ID() string          { return ID }
func (a *Adapter) DisplayName() string { return "Gitleaks" }

// SupportedLanguages mirrors the built-in secrets detector: gitleaks reads
// every text file type (source, shell, YAML/TOML/INI configs, .env files,
// committed keys), so the adapter declares the broadest routable set instead
// of enumerating languages and drifting as discovery learns new ones.
func (a *Adapter) SupportedLanguages() []analyzers.Language {
	return analyzers.AllLanguages()
}
func (a *Adapter) Check(_ context.Context, _ analyzers.ToolEnvironment) analyzers.ToolStatus {
	_, err := os.Stat(a.Executable)
	return analyzers.ToolStatus{Ready: err == nil, Version: a.Version, Detail: statusDetail(err)}
}
func (a *Adapter) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error {
	if a.Executable == "" {
		return fmt.Errorf("gitleaks executable is not managed yet")
	}
	return nil
}

func (a *Adapter) Plan(_ context.Context, req analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	// The orchestrator already routes by language, but the adapter filters
	// again (the convention ruff set) so a selection with nothing gitleaks
	// could read never launches the tool.
	if len(analyzers.FilesForLanguages(req.Files, a.SupportedLanguages()...)) == 0 {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("gitleaks-secrets does not apply")
	}
	// gitleaks has no file allow-list for directory scans: --source takes the
	// workspace root and the scan covers everything under it, honoring the
	// workspace's own .gitleaks.toml and .gitleaksignore. Allow-listing the
	// discovery selection here would silently drop exactly the unclassified
	// files credentials love to hide in.
	if req.WorkspaceRoot == "" {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("gitleaks requires a workspace root to scan")
	}
	reportPath := reportPathFor(req)
	args := []string{
		"detect",
		"--source", req.WorkspaceRoot,
		"--no-git", // scan the files on disk; a workspace without git history must still scan
		"--report-format", "json",
		"--report-path", reportPath,
		"--exit-code", "0", // findings are data, not a process failure
		"--redact=0", // full values reach Normalize for preview/length only; they never reach findings
		"--no-banner",
	}
	return analyzers.AnalyzerPlan{
		AnalyzerID: ID,
		Version:    a.Version,
		Commands:   []analyzers.ProcessSpec{{Executable: a.Executable, Args: args, Dir: req.WorkspaceRoot}},
		Metadata:   map[string]any{planKeyReportPath: reportPath, planKeySource: req.WorkspaceRoot},
	}, nil
}

// reportPathFor derives a deterministic scratch path for one scan's JSON
// report under the system temp directory. A stable per-scan name lets a retry
// overwrite stale output instead of stacking reports; gitleaks writes the
// file even when it finds nothing (an empty JSON array), so Run always has
// something to collect on a successful exit.
func reportPathFor(req analyzers.ScanRequest) string {
	name := req.ScanID
	if name == "" {
		name = "scan"
	}
	// Scan ids are hex-and-dash ids from database.NewID, but a hostile or
	// exotic id must never escape the temp directory.
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		}
		return '_'
	}, name)
	return filepath.Join(os.TempDir(), "bluntcode-gitleaks-"+safe+".json")
}

// Run executes the single planned command, then moves the JSON report into
// Stdout so Normalize parses the same envelope ruff and semgrep do.
func (a *Adapter) Run(ctx context.Context, plan analyzers.AnalyzerPlan, emit analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	reportPath, _ := plan.Metadata[planKeyReportPath].(string)
	if reportPath == "" {
		return analyzers.AnalyzerResult{Plan: plan}, fmt.Errorf("gitleaks plan is missing its report path")
	}
	// The report is scratch data that can hold full secret values (--redact=0);
	// it must not outlive the run, even when the process failed.
	defer os.Remove(reportPath)
	result, err := analyzers.RunDirect(ctx, plan, emit)
	if err != nil {
		return result, err
	}
	if result.ExitCode != 0 {
		// --exit-code 0 makes a findings run exit 0, so any nonzero code is a
		// real failure (gitleaks writes no report then); Normalize turns the
		// captured Stderr into the analyzer error.
		return result, nil
	}
	report, readErr := os.ReadFile(reportPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return result, fmt.Errorf("read gitleaks report: %w", readErr)
	}
	result.Stdout = report // nil when the file is absent; Normalize reads that as no findings
	return result, nil
}

// rawFinding mirrors gitleaks' report struct (verified against v8.30.1
// output). The JSON keys are Go field names because gitleaks marshals its
// findings without json tags; the git provenance fields (Commit, Author,
// Email, Date, Message) stay empty for --no-git scans and are kept only to
// document the real shape.
type rawFinding struct {
	RuleID      string
	Description string
	StartLine   int
	EndLine     int
	StartColumn int
	EndColumn   int
	Match       string
	Secret      string
	File        string
	SymlinkFile string
	Commit      string
	Entropy     float64
	Author      string
	Email       string
	Date        string
	Message     string
	Tags        []string
	Fingerprint string
}

func (a *Adapter) Normalize(_ context.Context, result analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	if result.ExitCode != 0 {
		return nil, nil, fmt.Errorf("gitleaks exited %d: %s", result.ExitCode, strings.TrimSpace(string(result.Stderr)))
	}
	if len(result.Stdout) == 0 {
		return nil, nil, nil
	}
	var raw []rawFinding
	if err := json.Unmarshal(result.Stdout, &raw); err != nil {
		return nil, nil, fmt.Errorf("parse gitleaks JSON: %w", err)
	}
	out := make([]analyzers.Finding, 0, len(raw))
	for _, d := range raw {
		f := analyzers.Finding{
			AnalyzerID:   ID,
			RuleID:       d.RuleID,
			Severity:     severityFor(d.RuleID),
			Category:     analyzers.CategorySecurity,
			Title:        d.RuleID,
			Message:      messageFor(d),
			RelativePath: relative(result.Plan, d.File),
			StartLine:    d.StartLine,
			StartColumn:  d.StartColumn,
			EndLine:      d.EndLine,
			EndColumn:    d.EndColumn,
			Remediation:  remediation,
			Metadata:     map[string]any{"secret_length": len([]rune(d.Secret)), "entropy": d.Entropy},
		}
		if preview := redactedPreview(d.Secret); preview != "" {
			f.Metadata["preview"] = preview
		}
		if d.Fingerprint != "" {
			f.Metadata["gitleaks_fingerprint"] = d.Fingerprint
		}
		f.SetFingerprint()
		out = append(out, f)
	}
	return out, nil, nil
}

// severityFor maps rule ids to Blunt Code severities. gitleaks 8.30 reports
// carry no per-finding severity of their own, so the mapping encodes how
// directly the credential class can be exploited: cloud account keys and
// private keys work as-is (critical), while everything else gitleaks flags
// is still a live-looking credential (high).
func severityFor(ruleID string) analyzers.Severity {
	switch {
	case ruleID == "aws-access-token" || strings.Contains(ruleID, "private-key"):
		return analyzers.SeverityCritical
	default:
		return analyzers.SeverityHigh
	}
}

// messageFor names the rule and its description. The secret itself never
// enters the message; only its redacted preview and length ride along in
// Metadata, matching what the built-in secrets detector exposes.
func messageFor(d rawFinding) string {
	return d.RuleID + ": " + d.Description
}

// redactedPreview keeps the built-in detector's shape: four leading
// characters and an ellipsis, or stars when the value is that short.
func redactedPreview(secret string) string {
	runes := []rune(secret)
	if len(runes) <= 4 {
		return strings.Repeat("*", len(runes))
	}
	return string(runes[:4]) + "…"
}

func relative(p analyzers.AnalyzerPlan, path string) string {
	if len(p.Commands) > 0 {
		if r, err := filepath.Rel(p.Commands[0].Dir, path); err == nil {
			return filepath.ToSlash(r)
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

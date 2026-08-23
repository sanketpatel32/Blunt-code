// Package secrets implements Blunt Code's built-in committed-credential
// detector. Unlike the external-tool adapters it needs no managed executable:
// the scan runs in-process. It still keeps the adapter contract intact — Run
// serializes its diagnostics into AnalyzerResult.Stdout as JSON and Normalize
// parses that JSON into findings, exactly like the ruff and biome adapters
// parse tool output, so the pipeline treats it like any other analyzer.
package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"bluntcode/internal/analyzers"
)

const (
	ID      = "secrets"
	version = "1.0.0"
)

const remediation = "Rotate the exposed credential, remove it from the source and version history, and load the replacement from a secret manager or environment variable."

// Adapter is the built-in secrets analyzer. It holds no state and no
// executable; New is a tools-free constructor.
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) ID() string          { return ID }
func (a *Adapter) DisplayName() string { return "Secrets Detector" }

// SupportedLanguages declares every language the discovery layer classifies.
// The orchestrator routes files by language, and credentials hide everywhere
// a scan can reach: source code, shell scripts, YAML/TOML/INI configs,
// Dockerfiles, .env files, and committed certificates alike. Declaring
// analyzers.AllLanguages() keeps this literally true as discovery learns new
// languages instead of drifting behind a hand-maintained list.
func (a *Adapter) SupportedLanguages() []analyzers.Language {
	return analyzers.AllLanguages()
}

// Check always succeeds: there is no external tool to verify.
func (a *Adapter) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	return analyzers.ToolStatus{Ready: true, Version: version, Detail: "built-in detector; no external tool required"}
}

// EnsureInstalled is a no-op; the detector runs in-process.
func (a *Adapter) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error { return nil }

// plan metadata keys. The plan carries no ProcessSpec commands because
// nothing is executed; the file selection travels through Metadata instead.
const (
	planKeyFiles = "files"
	planKeyRoot  = "workspace_root"
)

func (a *Adapter) Plan(_ context.Context, req analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	// The orchestrator already routes by language, but the adapter filters
	// again (the convention ruff set) so no caller can smuggle unrelated
	// files into the scan.
	files := analyzers.FilesForLanguages(req.Files, a.SupportedLanguages()...)
	if len(files) == 0 {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("secrets does not apply")
	}
	return analyzers.AnalyzerPlan{
		AnalyzerID: ID,
		Version:    version,
		Metadata:   map[string]any{planKeyFiles: files, planKeyRoot: req.WorkspaceRoot},
	}, nil
}

type rawDiagnostic struct {
	Rule         string `json:"rule"`
	Kind         string `json:"kind"`
	Path         string `json:"path"`
	Key          string `json:"key,omitempty"`
	Line         int    `json:"line"`
	Column       int    `json:"column"`
	EndLine      int    `json:"end_line"`
	EndColumn    int    `json:"end_column"`
	SecretLength int    `json:"secret_length"`
	Preview      string `json:"preview"`
}

type rawEnvelope struct {
	AnalyzerID         string          `json:"analyzer"`
	Version            string          `json:"version"`
	FilesScanned       int             `json:"files_scanned"`
	FilesSkippedBinary int             `json:"files_skipped_binary"`
	FilesUnreadable    int             `json:"files_unreadable"`
	Truncated          bool            `json:"truncated"`
	Notes              []string        `json:"notes,omitempty"`
	Diagnostics        []rawDiagnostic `json:"diagnostics"`
}

// Run scans the planned files in-process and serializes the diagnostics as
// JSON into Stdout, preserving the Run -> Normalize contract.
func (a *Adapter) Run(ctx context.Context, plan analyzers.AnalyzerPlan, _ analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	started := time.Now()
	files, root, err := planSelection(plan)
	if err != nil {
		return analyzers.AnalyzerResult{Plan: plan}, err
	}
	env := rawEnvelope{AnalyzerID: ID, Version: version, Diagnostics: []rawDiagnostic{}}
	notedTotalTruncation := false
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return analyzers.AnalyzerResult{Plan: plan}, err
		}
		if len(env.Diagnostics) >= maxFindingsPerRun {
			env.Truncated = true
			if !notedTotalTruncation {
				env.Notes = append(env.Notes, fmt.Sprintf("scan truncated at the %d-finding limit; remaining files were not scanned", maxFindingsPerRun))
				notedTotalTruncation = true
			}
			break
		}
		data, binary, readErr := readCapped(file)
		if readErr != nil {
			env.FilesUnreadable++
			continue
		}
		if binary {
			env.FilesSkippedBinary++
			continue
		}
		env.FilesScanned++
		rel := relativePath(root, file)
		found, truncated := scanFile(rel, data)
		if truncated {
			env.Notes = append(env.Notes, fmt.Sprintf("%s truncated at the %d-finding per-file limit", rel, maxFindingsPerFile))
		}
		if room := maxFindingsPerRun - len(env.Diagnostics); len(found) > room {
			found = found[:room]
			env.Truncated = true
			if !notedTotalTruncation {
				env.Notes = append(env.Notes, fmt.Sprintf("scan truncated at the %d-finding limit", maxFindingsPerRun))
				notedTotalTruncation = true
			}
		}
		env.Diagnostics = append(env.Diagnostics, found...)
	}
	stdout, err := json.Marshal(env)
	if err != nil {
		return analyzers.AnalyzerResult{Plan: plan}, err
	}
	return analyzers.AnalyzerResult{Plan: plan, Stdout: stdout, ExitCode: 0, StartedAt: started, FinishedAt: time.Now()}, nil
}

func (a *Adapter) Normalize(_ context.Context, result analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	if result.ExitCode != 0 {
		return nil, nil, fmt.Errorf("secrets scan failed with exit code %d", result.ExitCode)
	}
	if len(result.Stdout) == 0 {
		return nil, nil, nil
	}
	var env rawEnvelope
	if err := json.Unmarshal(result.Stdout, &env); err != nil {
		return nil, nil, fmt.Errorf("parse secrets JSON: %w", err)
	}
	findings := make([]analyzers.Finding, 0, len(env.Diagnostics))
	for _, d := range env.Diagnostics {
		f := analyzers.Finding{
			AnalyzerID:   ID,
			RuleID:       d.Rule,
			Severity:     severityFor(d.Rule),
			Category:     analyzers.CategorySecurity,
			Title:        d.Kind,
			Message:      messageFor(d),
			RelativePath: filepath.ToSlash(d.Path),
			StartLine:    d.Line,
			StartColumn:  d.Column,
			EndLine:      d.EndLine,
			EndColumn:    d.EndColumn,
			Remediation:  remediation,
			RawSeverity:  "warning",
			Metadata:     map[string]any{"kind": d.Kind, "secret_length": d.SecretLength},
		}
		if d.Key != "" {
			f.Metadata["assignment_key"] = d.Key
		}
		f.SetFingerprint()
		findings = append(findings, f)
	}
	metrics := []analyzers.Metric{
		{AnalyzerID: ID, Key: "files_scanned", Label: "Files scanned", Value: float64(env.FilesScanned)},
		{AnalyzerID: ID, Key: "findings", Label: "Findings", Value: float64(len(env.Diagnostics))},
		{AnalyzerID: ID, Key: "files_skipped_binary", Label: "Binary files skipped", Value: float64(env.FilesSkippedBinary)},
	}
	return findings, metrics, nil
}

func severityFor(rule string) analyzers.Severity {
	if rule == ruleGenericAssign {
		return analyzers.SeverityMedium
	}
	return analyzers.SeverityHigh
}

// messageFor describes the finding without ever echoing the full secret: the
// matched kind, plus a redacted preview (first four characters) and length.
func messageFor(d rawDiagnostic) string {
	if d.Key != "" {
		return fmt.Sprintf("Hardcoded credential assigned to %q (value redacted: begins %q, %d characters).", d.Key, d.Preview, d.SecretLength)
	}
	return fmt.Sprintf("Possible %s committed in source (value redacted: begins %q, %d characters).", d.Kind, d.Preview, d.SecretLength)
}

func planSelection(plan analyzers.AnalyzerPlan) (files []string, root string, err error) {
	if plan.Metadata == nil {
		return nil, "", fmt.Errorf("secrets plan is missing its file selection")
	}
	files, ok := plan.Metadata[planKeyFiles].([]string)
	if !ok || len(files) == 0 {
		return nil, "", fmt.Errorf("secrets plan is missing its file selection")
	}
	root, _ = plan.Metadata[planKeyRoot].(string)
	return files, root, nil
}

// readCapped reads at most maxScanBytes and reports whether the file looks
// binary (a NUL byte within the first binarySniffBytes bytes).
func readCapped(path string) (data []byte, binary bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	buf := make([]byte, maxScanBytes)
	n, readErr := io.ReadFull(f, buf)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return nil, false, readErr
	}
	data = buf[:n]
	sniff := data
	if len(sniff) > binarySniffBytes {
		sniff = sniff[:binarySniffBytes]
	}
	if bytes.IndexByte(sniff, 0) >= 0 {
		return data, true, nil
	}
	return data, false, nil
}

// scanFile detects secrets in one file's content and caps the per-file result.
func scanFile(rel string, data []byte) (diagnostics []rawDiagnostic, truncated bool) {
	matches := detect(data)
	if len(matches) > maxFindingsPerFile {
		matches = matches[:maxFindingsPerFile]
		truncated = true
	}
	diagnostics = make([]rawDiagnostic, 0, len(matches))
	for _, m := range matches {
		d := rawDiagnostic{
			Rule:         m.rule,
			Kind:         m.kind,
			Path:         rel,
			Key:          m.key,
			SecretLength: len([]rune(m.secret)),
			Preview:      redactedPreview(m.secret),
		}
		d.Line, d.Column = position(data, m.start)
		d.EndLine, d.EndColumn = position(data, m.end)
		diagnostics = append(diagnostics, d)
	}
	return diagnostics, truncated
}

func relativePath(root, file string) string {
	if rel, err := filepath.Rel(root, file); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(file)
}

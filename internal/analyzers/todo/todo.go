// Package todo implements Blunt Code's built-in procrastination-debt tracker:
// TODO/FIXME/HACK/XXX/BUG markers left in comments. Like the secrets detector
// it needs no managed executable — the scan runs in-process — while keeping
// the adapter contract intact: Run serializes its diagnostics into
// AnalyzerResult.Stdout as JSON and Normalize parses that JSON into findings,
// exactly like the ruff and biome adapters parse tool output, so the pipeline
// treats it like any other analyzer.
//
// Findings can be dismissed in source with an inline bluntcode:ignore
// comment directive on the marker's line or the line immediately above;
// the canonical syntax and semantics are documented in
// internal/analyzers/ignore.go.
package todo

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
	ID      = "todo"
	version = "1.0.0"
)

const remediation = "Finish the tracked work and delete the marker, or move it into the issue tracker so the debt stays visible outside the code."

// Adapter is the built-in TODO/FIXME analyzer. It holds no state and no
// executable; New is a tools-free constructor.
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) ID() string          { return ID }
func (a *Adapter) DisplayName() string { return "TODO Comment Tracker" }

// SupportedLanguages declares the languages where TODO/FIXME-style markers
// plausibly live: every programming language discovery classifies (comments
// exist in all of them), the shell script family (powershell and batch
// included — '#' and REM comments carry markers too), config formats with a
// comment syntax worth reading (yaml, toml, ini, properties, dockerfile),
// and markdown (markers in docs are tracked debt). Deliberately excluded:
// json, css, scss, less, html, xml, sql, and graphql (machine-consumed,
// frequently generated or vendored — json has no comments at all — so marker
// hits there are noise, not deferred work); text (plain prose has no comment
// context); and the credential blobs env and certificate, which are the
// secrets detector's territory, not comment carriers.
func (a *Adapter) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{
		analyzers.LanguagePython, analyzers.LanguageJavaScript, analyzers.LanguageTypeScript,
		analyzers.LanguageGo, analyzers.LanguageJava, analyzers.LanguageKotlin,
		analyzers.LanguageCSharp, analyzers.LanguageC, analyzers.LanguageCPP,
		analyzers.LanguageRuby, analyzers.LanguagePHP, analyzers.LanguageRust,
		analyzers.LanguageSwift, analyzers.LanguageScala, analyzers.LanguageObjectiveC,
		analyzers.LanguageVue, analyzers.LanguageSvelte,
		analyzers.LanguageShell, analyzers.LanguagePowerShell, analyzers.LanguageBatch,
		analyzers.LanguageYAML, analyzers.LanguageTOML, analyzers.LanguageINI,
		analyzers.LanguageProperties, analyzers.LanguageDockerfile, analyzers.LanguageMarkdown,
	}
}

// Check always succeeds: there is no external tool to verify.
func (a *Adapter) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	return analyzers.ToolStatus{Ready: true, Version: version, Detail: "built-in tracker; no external tool required"}
}

// EnsureInstalled is a no-op; the tracker runs in-process.
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
		return analyzers.AnalyzerPlan{}, fmt.Errorf("todo does not apply")
	}
	return analyzers.AnalyzerPlan{
		AnalyzerID: ID,
		Version:    version,
		Metadata:   map[string]any{planKeyFiles: files, planKeyRoot: req.WorkspaceRoot},
	}, nil
}

type rawDiagnostic struct {
	Rule      string `json:"rule"`
	Marker    string `json:"marker"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"end_line"`
	EndColumn int    `json:"end_column"`
	Message   string `json:"message"`
	// Inline bluntcode:ignore context, parsed at Run time: the directive on
	// the finding's line and the one on the contiguous line above (nil when
	// that line carries no directive).
	IgnoreSameLine *analyzers.IgnoreDirective `json:"ignore_same_line,omitempty"`
	IgnorePrevLine *analyzers.IgnoreDirective `json:"ignore_prev_line,omitempty"`
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
		return nil, nil, fmt.Errorf("todo scan failed with exit code %d", result.ExitCode)
	}
	if len(result.Stdout) == 0 {
		return nil, nil, nil
	}
	var env rawEnvelope
	if err := json.Unmarshal(result.Stdout, &env); err != nil {
		return nil, nil, fmt.Errorf("parse todo JSON: %w", err)
	}
	findings := make([]analyzers.Finding, 0, len(env.Diagnostics))
	for _, d := range env.Diagnostics {
		// Inline bluntcode:ignore directives (same line or the contiguous
		// line above) are honored per finding, before it is built or
		// fingerprinted: suppressed diagnostics never become findings, and
		// surviving fingerprints are unaffected by ignored neighbors.
		if analyzers.IgnoreSuppressedDirectives(d.IgnoreSameLine, d.IgnorePrevLine, d.Rule) {
			continue
		}
		f := analyzers.Finding{
			AnalyzerID:   ID,
			RuleID:       d.Rule,
			Severity:     severityFor(d.Rule),
			Category:     analyzers.CategoryMaintainability,
			Title:        d.Marker + " comment marker",
			Message:      d.Message,
			RelativePath: filepath.ToSlash(d.Path),
			StartLine:    d.Line,
			StartColumn:  d.Column,
			EndLine:      d.EndLine,
			EndColumn:    d.EndColumn,
			Remediation:  remediation,
			RawSeverity:  "warning",
			Metadata:     map[string]any{"marker": d.Marker},
		}
		f.SetFingerprint()
		findings = append(findings, f)
	}
	metrics := []analyzers.Metric{
		{AnalyzerID: ID, Key: "files_scanned", Label: "Files scanned", Value: float64(env.FilesScanned)},
		{AnalyzerID: ID, Key: "findings", Label: "Findings", Value: float64(len(findings))}, // surviving findings, after inline ignores
		{AnalyzerID: ID, Key: "files_skipped_binary", Label: "Binary files skipped", Value: float64(env.FilesSkippedBinary)},
	}
	return findings, metrics, nil
}

// severityFor ranks the markers: FIXME and BUG declare something known-broken,
// so they land at medium; TODO, HACK, and XXX track ordinary deferred work at
// low.
func severityFor(rule string) analyzers.Severity {
	if rule == ruleFIXME || rule == ruleBUG {
		return analyzers.SeverityMedium
	}
	return analyzers.SeverityLow
}

func planSelection(plan analyzers.AnalyzerPlan) (files []string, root string, err error) {
	if plan.Metadata == nil {
		return nil, "", fmt.Errorf("todo plan is missing its file selection")
	}
	files, ok := plan.Metadata[planKeyFiles].([]string)
	if !ok || len(files) == 0 {
		return nil, "", fmt.Errorf("todo plan is missing its file selection")
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

// scanFile detects markers in one file's content and caps the per-file result.
func scanFile(rel string, data []byte) (diagnostics []rawDiagnostic, truncated bool) {
	matches := detect(data)
	if len(matches) > maxFindingsPerFile {
		matches = matches[:maxFindingsPerFile]
		truncated = true
	}
	diagnostics = make([]rawDiagnostic, 0, len(matches))
	for _, m := range matches {
		d := rawDiagnostic{
			Rule:      m.rule,
			Marker:    m.marker,
			Path:      rel,
			Line:      m.line,
			Column:    m.column,
			EndLine:   m.line,
			EndColumn: m.column + len(m.marker), // markers are ASCII, so rune length == byte length
			Message:   m.message,
		}
		// Parse the inline bluntcode:ignore directives for the marker's line
		// and the line above here, at Run time, so Normalize stays pure.
		d.IgnoreSameLine, d.IgnorePrevLine = analyzers.IgnoreDirectivesAt(data, m.line)
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

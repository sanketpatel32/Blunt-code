package ruff

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bluntcode/internal/analyzers"
)

const ID = "ruff"

type Adapter struct {
	Executable string
	Version    string
}

func New(executable, version string) *Adapter {
	return &Adapter{Executable: executable, Version: version}
}
func (a *Adapter) ID() string          { return ID }
func (a *Adapter) DisplayName() string { return "Ruff" }
func (a *Adapter) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguagePython}
}
func (a *Adapter) Check(_ context.Context, _ analyzers.ToolEnvironment) analyzers.ToolStatus {
	_, err := os.Stat(a.Executable)
	return analyzers.ToolStatus{Ready: err == nil, Version: a.Version, Detail: statusDetail(err)}
}
func (a *Adapter) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error {
	if a.Executable == "" {
		return fmt.Errorf("ruff executable is not managed yet")
	}
	return nil
}
func (a *Adapter) Plan(_ context.Context, req analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	if !analyzers.HasLanguage(req.Languages, analyzers.LanguagePython) {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("ruff does not apply")
	}
	args := []string{"check", "--output-format", "json", "--no-fix"}
	args = append(args, req.Files...)
	return analyzers.AnalyzerPlan{AnalyzerID: ID, Version: a.Version, Commands: []analyzers.ProcessSpec{{Executable: a.Executable, Args: args, Dir: req.WorkspaceRoot}}}, nil
}
func (a *Adapter) Run(ctx context.Context, p analyzers.AnalyzerPlan, emit analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	return analyzers.RunDirect(ctx, p, emit)
}

type rawDiagnostic struct {
	Code, Message, Filename, URL string
	Location                     rawLocation      `json:"location"`
	EndLocation                  rawLocation      `json:"end_location"`
	Fix                          *json.RawMessage `json:"fix"`
}
type rawLocation struct{ Row, Column int }

func (a *Adapter) Normalize(_ context.Context, result analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	if result.ExitCode != 0 && result.ExitCode != 1 {
		return nil, nil, fmt.Errorf("ruff exited %d: %s", result.ExitCode, strings.TrimSpace(string(result.Stderr)))
	}
	var raw []rawDiagnostic
	if len(result.Stdout) == 0 {
		return nil, nil, nil
	}
	if err := json.Unmarshal(result.Stdout, &raw); err != nil {
		return nil, nil, fmt.Errorf("parse ruff JSON: %w", err)
	}
	out := make([]analyzers.Finding, 0, len(raw))
	for _, d := range raw {
		path := relative(result.Plan, d.Filename)
		f := analyzers.Finding{AnalyzerID: ID, RuleID: d.Code, Severity: severity(d.Code), Category: category(d.Code), Title: d.Code, Message: d.Message, RelativePath: path, StartLine: d.Location.Row, StartColumn: d.Location.Column, EndLine: d.EndLocation.Row, EndColumn: d.EndLocation.Column, DocumentationURL: d.URL, RawSeverity: "warning", Metadata: map[string]any{"fix_available": d.Fix != nil}}
		f.SetFingerprint()
		out = append(out, f)
	}
	return out, nil, nil
}
func severity(code string) analyzers.Severity {
	if strings.HasPrefix(code, "S") {
		return analyzers.SeverityHigh
	}
	if strings.HasPrefix(code, "F") || strings.HasPrefix(code, "E") {
		return analyzers.SeverityMedium
	}
	return analyzers.SeverityLow
}
func category(code string) analyzers.Category {
	if strings.HasPrefix(code, "S") {
		return analyzers.CategorySecurity
	}
	if strings.HasPrefix(code, "F") {
		return analyzers.CategoryCorrectness
	}
	if strings.HasPrefix(code, "E") {
		return analyzers.CategoryStyle
	}
	return analyzers.CategoryMaintainability
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

package ruff

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bluntcode/internal/analyzers"
)

const ID = "ruff"

// deepSelect is the extended rule selection used only by the deep profile.
// Ruff's built-in default is E4,E7,E9,F; because --select replaces that
// default, the set below keeps F and widens every category deliberately:
//
//	E, W - full pycodestyle errors and warnings (the default covers only
//	       E4/E7/E9, so deep adds the remaining style checks)
//	F    - pyflakes correctness rules (carried over from the default)
//	I    - isort import ordering; ruff 0.16 enables it by default, so deep
//	       must keep it or --select would silently narrow the rule set
//	B    - flake8-bugbear, likely-bug detection
//	SIM  - flake8-simplify, simplifiable-code patterns
//	C4   - flake8-comprehensions, needless comprehension workarounds
//	RET  - flake8-return, needless return indirection
//	ARG  - flake8-unused-arguments, dead parameters
//	PLR  - pylint-refactor, complexity and maintainability
//
// Everything listed is a stable, non-preview ruff category that runs entirely
// offline, so deep is slower than standard but introduces no new failure
// modes. Noisy opinion-only families (like PL or PEP 8 naming rules) are left
// out on purpose.
const deepSelect = "E,W,F,I,B,SIM,C4,RET,ARG,PLR"

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
	// The orchestrator already narrows req.Files to Python, but the adapter
	// filters again so no caller can hand ruff a JavaScript or TypeScript
	// file: ruff parses whatever it is given as Python, and a minified web
	// bundle then produces megabytes of syntax-error JSON.
	pyFiles := analyzers.FilesForLanguages(req.Files, analyzers.LanguagePython)
	if len(pyFiles) == 0 {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("ruff does not apply")
	}
	args := []string{"check", "--output-format", "json", "--no-fix"}
	if req.Profile == analyzers.ProfileDeep || req.Profile == analyzers.ProfilePentest {
		args = append(args, "--select="+deepSelect)
	}
	// Windows caps a process command line at 32,767 characters, so large
	// workspaces are split into batches the same way biome and semgrep do;
	// Run merges the per-batch JSON outputs back into one result.
	commands := make([]analyzers.ProcessSpec, 0)
	for _, batch := range analyzers.FileArgumentBatches(args, pyFiles) {
		commands = append(commands, analyzers.ProcessSpec{Executable: a.Executable, Args: batch, Dir: req.WorkspaceRoot})
	}
	return analyzers.AnalyzerPlan{AnalyzerID: ID, Version: a.Version, Commands: commands}, nil
}
func (a *Adapter) Run(ctx context.Context, p analyzers.AnalyzerPlan, emit analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	if len(p.Commands) <= 1 {
		return analyzers.RunDirect(ctx, p, emit)
	}
	started := time.Now()
	merged := make([]rawDiagnostic, 0)
	for _, command := range p.Commands {
		partPlan := p
		partPlan.Commands = []analyzers.ProcessSpec{command}
		part, err := analyzers.RunDirect(ctx, partPlan, emit)
		if err != nil || (part.ExitCode != 0 && part.ExitCode != 1) {
			return part, err
		}
		if len(part.Stdout) == 0 {
			continue
		}
		var parsed []rawDiagnostic
		if err := json.Unmarshal(part.Stdout, &parsed); err != nil {
			return part, nil
		}
		merged = append(merged, parsed...)
	}
	stdout, err := json.Marshal(merged)
	if err != nil {
		return analyzers.AnalyzerResult{Plan: p}, err
	}
	exitCode := 0
	if len(merged) > 0 {
		exitCode = 1
	}
	return analyzers.AnalyzerResult{Plan: p, Stdout: stdout, ExitCode: exitCode, StartedAt: started, FinishedAt: time.Now()}, nil
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
	// SIM must be matched before the single-letter S (bandit) prefix, because
	// deep-mode flake8-simplify codes also start with "S".
	if strings.HasPrefix(code, "SIM") {
		return analyzers.SeverityLow
	}
	if strings.HasPrefix(code, "S") {
		return analyzers.SeverityHigh
	}
	// B (bugbear) reports likely bugs, so it sits with F/E at medium.
	if strings.HasPrefix(code, "F") || strings.HasPrefix(code, "E") || strings.HasPrefix(code, "B") {
		return analyzers.SeverityMedium
	}
	return analyzers.SeverityLow
}
func category(code string) analyzers.Category {
	if strings.HasPrefix(code, "SIM") {
		return analyzers.CategoryMaintainability
	}
	if strings.HasPrefix(code, "S") {
		return analyzers.CategorySecurity
	}
	if strings.HasPrefix(code, "F") || strings.HasPrefix(code, "B") {
		return analyzers.CategoryCorrectness
	}
	if strings.HasPrefix(code, "E") || strings.HasPrefix(code, "W") {
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

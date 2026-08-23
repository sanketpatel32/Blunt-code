package biome

import (
	"bluntcode/internal/analyzers"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ID = "biome"

type Adapter struct{ Executable, Version string }

func New(executable, version string) *Adapter { return &Adapter{executable, version} }
func (a *Adapter) ID() string                 { return ID }
func (a *Adapter) DisplayName() string        { return "Biome" }
func (a *Adapter) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguageJavaScript, analyzers.LanguageTypeScript}
}
func (a *Adapter) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	_, err := os.Stat(a.Executable)
	return analyzers.ToolStatus{Ready: err == nil, Version: a.Version, Detail: detail(err)}
}
func (a *Adapter) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error {
	if a.Executable == "" {
		return fmt.Errorf("biome executable is not managed yet")
	}
	return nil
}
func (a *Adapter) Plan(_ context.Context, req analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	// The orchestrator already narrows req.Files to JavaScript/TypeScript,
	// but the adapter filters again so no caller can hand biome files it
	// cannot lint: explicit non-web paths would otherwise depend on
	// --no-errors-on-unmatched to be silently ignored.
	webFiles := analyzers.FilesForLanguages(req.Files, analyzers.LanguageJavaScript, analyzers.LanguageTypeScript)
	if len(webFiles) == 0 {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("biome does not apply")
	}
	prefix := []string{"lint", "--reporter=json", "--no-errors-on-unmatched"}
	// React workspaces get the react domain injected via a generated biome
	// config (see react.go); every other workspace keeps exactly the command
	// line this adapter produced before, so non-React findings and their
	// fingerprints cannot shift. No logger is wired into planning, so the
	// detection reason travels in the plan metadata instead of a log line.
	var metadata map[string]any
	if d := detectReact(req.WorkspaceRoot, req.Files); d.Detected && !hasWorkspaceBiomeConfig(req.WorkspaceRoot) {
		if configPath, ok := writeInjectedConfig(); ok {
			prefix = append(prefix, "--config-path="+configPath)
			metadata = map[string]any{"react_domain": d.Reason}
		}
	}
	commands := make([]analyzers.ProcessSpec, 0)
	for _, args := range analyzers.FileArgumentBatches(prefix, webFiles) {
		commands = append(commands, analyzers.ProcessSpec{Executable: a.Executable, Args: args, Dir: req.WorkspaceRoot})
	}
	return analyzers.AnalyzerPlan{AnalyzerID: ID, Version: a.Version, Commands: commands, Metadata: metadata}, nil
}
func (a *Adapter) Run(ctx context.Context, p analyzers.AnalyzerPlan, emit analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	if len(p.Commands) <= 1 {
		return analyzers.RunDirect(ctx, p, emit)
	}
	started := time.Now()
	merged := output{}
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
		var parsed output
		if err := json.Unmarshal(part.Stdout, &parsed); err != nil {
			return part, nil
		}
		merged.Diagnostics = append(merged.Diagnostics, parsed.Diagnostics...)
	}
	stdout, err := json.Marshal(merged)
	if err != nil {
		return analyzers.AnalyzerResult{Plan: p}, err
	}
	return analyzers.AnalyzerResult{Plan: p, Stdout: stdout, StartedAt: started, FinishedAt: time.Now()}, nil
}

type output struct {
	Diagnostics []diagnostic `json:"diagnostics"`
}
type diagnostic struct {
	Category, Severity, Description, Message string
	Location                                 struct {
		Path  string `json:"path"`
		Start *struct {
			Line   int `json:"line"`
			Column int `json:"column"`
		} `json:"start"`
		End *struct {
			Line   int `json:"line"`
			Column int `json:"column"`
		} `json:"end"`
		Span *struct {
			Start int `json:"start"`
			End   int `json:"end"`
		} `json:"span"`
	} `json:"location"`
	Advices []struct {
		Log string `json:"log"`
	} `json:"advices"`
}

func (a *Adapter) Normalize(_ context.Context, r analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	if r.ExitCode != 0 && r.ExitCode != 1 {
		return nil, nil, fmt.Errorf("biome exited %d: %s", r.ExitCode, strings.TrimSpace(string(r.Stderr)))
	}
	var raw output
	if len(r.Stdout) == 0 {
		return nil, nil, nil
	}
	if err := json.Unmarshal(r.Stdout, &raw); err != nil {
		return nil, nil, fmt.Errorf("parse biome JSON: %w", err)
	}
	out := make([]analyzers.Finding, 0, len(raw.Diagnostics))
	for _, d := range raw.Diagnostics {
		rule := d.Category
		title := rule
		if i := strings.LastIndex(rule, "/"); i >= 0 {
			title = rule[i+1:]
		}
		message := strings.TrimSpace(d.Message)
		if message == "" {
			message = strings.TrimSpace(d.Description)
		}
		if message == "" {
			message = "Biome reported " + title + "."
		}
		path := d.Location.Path
		sourcePath := path
		if !filepath.IsAbs(sourcePath) && len(r.Plan.Commands) > 0 {
			sourcePath = filepath.Join(r.Plan.Commands[0].Dir, sourcePath)
		}
		if len(r.Plan.Commands) > 0 {
			if rel, err := filepath.Rel(r.Plan.Commands[0].Dir, sourcePath); err == nil {
				path = rel
			}
		}
		remediation := ""
		if len(d.Advices) > 0 {
			remediation = d.Advices[0].Log
		}
		f := analyzers.Finding{AnalyzerID: ID, RuleID: rule, Severity: mapSeverity(d.Severity), Category: mapCategory(rule), Title: title, Message: message, RelativePath: filepath.ToSlash(path), Remediation: remediation, RawSeverity: d.Severity, Metadata: map[string]any{"biome_category": d.Category}}
		if d.Location.Start != nil {
			f.StartLine, f.StartColumn = d.Location.Start.Line, d.Location.Start.Column
			if d.Location.End != nil {
				f.EndLine, f.EndColumn = d.Location.End.Line, d.Location.End.Column
			}
		} else if d.Location.Span != nil {
			f.StartLine, f.StartColumn = sourcePosition(sourcePath, d.Location.Span.Start)
			f.EndLine, f.EndColumn = sourcePosition(sourcePath, d.Location.Span.End)
		}
		f.SetFingerprint()
		out = append(out, f)
	}
	return out, nil, nil
}

// sourcePosition translates Biome's UTF-8 byte offset into a 1-based source
// position. If source is unavailable, normalization still returns the finding.
func sourcePosition(path string, offset int) (line, column int) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > 2<<20 {
		return 0, 0
	}
	data, err := os.ReadFile(path)
	if err != nil || offset < 0 || offset > len(data) {
		return 0, 0
	}
	line, column = 1, 1
	for _, b := range data[:offset] {
		if b == '\n' {
			line, column = line+1, 1
			continue
		}
		column++
	}
	return line, column
}
func mapSeverity(s string) analyzers.Severity {
	switch strings.ToLower(s) {
	case "fatal":
		return analyzers.SeverityHigh
	case "error":
		return analyzers.SeverityMedium
	case "warning":
		return analyzers.SeverityLow
	default:
		return analyzers.SeverityInfo
	}
}
func mapCategory(rule string) analyzers.Category {
	switch {
	case strings.Contains(rule, "security"):
		return analyzers.CategorySecurity
	case strings.Contains(rule, "correctness"):
		return analyzers.CategoryCorrectness
	case strings.Contains(rule, "style"):
		return analyzers.CategoryStyle
	default:
		return analyzers.CategoryMaintainability
	}
}
func detail(err error) string {
	if err != nil {
		return "managed executable not found"
	}
	return ""
}

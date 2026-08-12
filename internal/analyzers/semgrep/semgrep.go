package semgrep

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

const ID = "semgrep"

type Adapter struct{ Executable, Version, RulesPath string }

func New(executable, version, rulesPath string) *Adapter {
	return &Adapter{executable, version, rulesPath}
}
func (a *Adapter) ID() string          { return ID }
func (a *Adapter) DisplayName() string { return "Semgrep" }
func (a *Adapter) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguagePython, analyzers.LanguageJavaScript, analyzers.LanguageTypeScript}
}
func (a *Adapter) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	_, err := os.Stat(a.Executable)
	if err != nil {
		return analyzers.ToolStatus{Version: a.Version, Detail: "managed executable not found"}
	}
	if a.RulesPath == "" {
		return analyzers.ToolStatus{Version: a.Version, Detail: "local ruleset is not configured"}
	}
	if _, err := os.Stat(a.RulesPath); err != nil {
		return analyzers.ToolStatus{Version: a.Version, Detail: "local ruleset is unavailable"}
	}
	return analyzers.ToolStatus{Ready: true, Version: a.Version}
}
func (a *Adapter) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error {
	if a.Executable == "" {
		return fmt.Errorf("semgrep executable is not managed yet")
	}
	if a.RulesPath == "" {
		return fmt.Errorf("semgrep local ruleset is not configured")
	}
	return nil
}
func (a *Adapter) Plan(_ context.Context, req analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	if !analyzers.HasLanguage(req.Languages, a.SupportedLanguages()...) {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("semgrep does not apply")
	}
	if a.RulesPath == "" {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("semgrep local ruleset unavailable")
	}
	prefix := []string{
		"scan", "--json", "--config", a.RulesPath, "--no-rewrite-rule-ids",
		"--metrics=off", "--disable-version-check", "--oss-only",
	}
	commands := make([]analyzers.ProcessSpec, 0)
	for _, args := range analyzers.FileArgumentBatches(prefix, req.Files) {
		commands = append(commands, analyzers.ProcessSpec{
			Executable: a.Executable,
			Args:       args,
			Dir:        req.WorkspaceRoot,
			Env: map[string]string{
				"SEMGREP_APP_TOKEN":            "",
				"SEMGREP_ENABLE_VERSION_CHECK": "0",
				"SEMGREP_SEND_METRICS":         "off",
				"SEMGREP_SETTINGS_FILE":        filepath.Join(filepath.Dir(a.RulesPath), "settings.yml"),
			},
		})
	}
	return analyzers.AnalyzerPlan{
		AnalyzerID: ID,
		Version:    a.Version,
		Commands:   commands,
		Metadata:   map[string]any{"rules_path": a.RulesPath, "rules_source": "bundled_local", "offline": true},
	}, nil
}
func (a *Adapter) Run(ctx context.Context, p analyzers.AnalyzerPlan, e analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	if len(p.Commands) <= 1 {
		return analyzers.RunDirect(ctx, p, e)
	}
	started := time.Now()
	merged := result{}
	for _, command := range p.Commands {
		partPlan := p
		partPlan.Commands = []analyzers.ProcessSpec{command}
		part, err := analyzers.RunDirect(ctx, partPlan, e)
		if err != nil || (part.ExitCode != 0 && part.ExitCode != 1) {
			return part, err
		}
		if len(part.Stdout) == 0 {
			continue
		}
		var parsed result
		if err := json.Unmarshal(part.Stdout, &parsed); err != nil {
			return part, nil
		}
		merged.Results = append(merged.Results, parsed.Results...)
		merged.Errors = append(merged.Errors, parsed.Errors...)
	}
	stdout, err := json.Marshal(merged)
	if err != nil {
		return analyzers.AnalyzerResult{Plan: p}, err
	}
	return analyzers.AnalyzerResult{Plan: p, Stdout: stdout, StartedAt: started, FinishedAt: time.Now()}, nil
}

type result struct {
	Results []issue `json:"results"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}
type issue struct {
	CheckID string `json:"check_id"`
	Path    string `json:"path"`
	Start   point  `json:"start"`
	End     point  `json:"end"`
	Extra   extra  `json:"extra"`
}
type point struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}
type extra struct {
	Message  string         `json:"message"`
	Severity string         `json:"severity"`
	Metadata map[string]any `json:"metadata"`
	Lines    string         `json:"lines"`
	Fix      string         `json:"fix"`
}

func (a *Adapter) Normalize(_ context.Context, r analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	if r.ExitCode != 0 && r.ExitCode != 1 {
		return nil, nil, fmt.Errorf("semgrep exited %d: %s", r.ExitCode, strings.TrimSpace(string(r.Stderr)))
	}
	var raw result
	if len(r.Stdout) == 0 {
		return nil, nil, nil
	}
	if err := json.Unmarshal(r.Stdout, &raw); err != nil {
		return nil, nil, fmt.Errorf("parse semgrep JSON: %w", err)
	}
	out := make([]analyzers.Finding, 0, len(raw.Results))
	for _, x := range raw.Results {
		path := x.Path
		if len(r.Plan.Commands) > 0 {
			if rel, err := filepath.Rel(r.Plan.Commands[0].Dir, path); err == nil {
				path = rel
			}
		}
		msg := x.Extra.Message
		cat := analyzers.CategorySecurity
		if c, ok := x.Extra.Metadata["category"].(string); ok {
			cat = category(c)
		}
		f := analyzers.Finding{AnalyzerID: ID, RuleID: x.CheckID, Severity: severity(x.Extra.Severity), Category: cat, Title: x.CheckID, Message: msg, RelativePath: filepath.ToSlash(path), StartLine: x.Start.Line, StartColumn: x.Start.Col, EndLine: x.End.Line, EndColumn: x.End.Col, Remediation: x.Extra.Fix, RawSeverity: x.Extra.Severity, Metadata: x.Extra.Metadata}
		f.SetFingerprint()
		out = append(out, f)
	}
	return out, nil, nil
}
func severity(s string) analyzers.Severity {
	switch strings.ToLower(s) {
	case "error", "critical":
		return analyzers.SeverityHigh
	case "warning":
		return analyzers.SeverityMedium
	case "info":
		return analyzers.SeverityInfo
	default:
		return analyzers.SeverityLow
	}
}
func category(s string) analyzers.Category {
	switch strings.ToLower(s) {
	case "security":
		return analyzers.CategorySecurity
	case "vulnerability":
		return analyzers.CategoryVulnerability
	case "correctness":
		return analyzers.CategoryCorrectness
	default:
		return analyzers.CategoryMaintainability
	}
}

// Package sonarqube isolates all version-specific managed-server behavior.
// It contains concrete lifecycle/API contracts, but deliberately does not
// pretend installation has been verified until a release engineer pins the
// server/scanner artifacts and bootstraps them against that exact release.
package sonarqube

import (
	"bluntcode/internal/analyzers"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ID = "sonarqube"

type Installer interface {
	Ensure(context.Context) error
	ServerExecutable() string
	ScannerExecutable() string
	Version() string
}
type Runtime interface {
	Validate(context.Context) error
	Environment() map[string]string
}
type Server interface {
	Start(context.Context, map[string]string) error
	Healthy(context.Context) (bool, error)
	Shutdown(context.Context) error
}
type Client interface {
	Bootstrap(context.Context) error
	EnsureProject(context.Context, string, string) error
	Token(context.Context) (string, error)
	WaitForTask(context.Context, string) error
	Issues(context.Context, string) ([]Issue, error)
	Metrics(context.Context, string) ([]Metric, error)
}
type Metric = analyzers.Metric
type Adapter struct {
	Installer      Installer
	Runtime        Runtime
	Server         Server
	Client         Client
	BaseURL        string
	DataDir        string
	StartupTimeout time.Duration
}

func New(i Installer, r Runtime, s Server, c Client) *Adapter {
	// A cold community-server boot (JVM plus embedded search index) routinely
	// takes more than three minutes on consumer Windows hardware, which made
	// healthy installs report "did not become healthy within 3m0s". Ten
	// minutes still leaves room for the analysis itself inside the sonarqube
	// analyzer timeout; warm servers answer the first health check and skip
	// the wait entirely. BLUNTCODE_SONAR_STARTUP_TIMEOUT can tighten or
	// extend the budget per environment.
	return &Adapter{Installer: i, Runtime: r, Server: s, Client: c, BaseURL: "http://127.0.0.1:9000", StartupTimeout: defaultStartupTimeout}
}
func (a *Adapter) ID() string          { return ID }
func (a *Adapter) DisplayName() string { return "SonarQube" }
func (a *Adapter) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguagePython, analyzers.LanguageJavaScript, analyzers.LanguageTypeScript}
}
func (a *Adapter) Check(ctx context.Context, _ analyzers.ToolEnvironment) analyzers.ToolStatus {
	if a.Installer == nil || a.Runtime == nil || a.Server == nil || a.Client == nil {
		return analyzers.ToolStatus{Detail: "managed SonarQube lifecycle is not configured"}
	}
	if err := a.Runtime.Validate(ctx); err != nil {
		return analyzers.ToolStatus{Detail: "runtime unavailable: " + err.Error()}
	}
	if installer, ok := a.Installer.(interface{ Installed() error }); ok {
		if err := installer.Installed(); err != nil {
			return analyzers.ToolStatus{Version: a.Installer.Version(), Detail: "install incomplete: " + err.Error()}
		}
	}
	ok, healthErr := a.Server.Healthy(ctx)
	if healthErr != nil {
		// A connection refusal is expected before the managed local server has
		// started. Run will start it and perform the bounded health wait.
		return analyzers.ToolStatus{Ready: true, Version: a.Installer.Version(), Detail: "server is stopped; it will start for this scan"}
	}
	if !ok {
		return analyzers.ToolStatus{Ready: true, Version: a.Installer.Version(), Detail: "server is stopped; it will start for this scan"}
	}
	return analyzers.ToolStatus{Ready: true, Version: a.Installer.Version()}
}
func (a *Adapter) EnsureInstalled(ctx context.Context, _ analyzers.ToolEnvironment) error {
	if a.Installer == nil || a.Runtime == nil {
		return fmt.Errorf("managed SonarQube installer/runtime is not configured")
	}
	if err := a.Installer.Ensure(ctx); err != nil {
		return fmt.Errorf("install managed SonarQube: %w", err)
	}
	return a.Runtime.Validate(ctx)
}
func (a *Adapter) EnsureRunning(ctx context.Context) error {
	if a.Server == nil || a.Client == nil {
		return fmt.Errorf("managed SonarQube server/client is not configured")
	}
	ok, err := a.Server.Healthy(ctx)
	if err == nil && ok {
		return nil
	}
	if a.Runtime == nil {
		return fmt.Errorf("managed Java runtime unavailable")
	}
	if err := a.Server.Start(ctx, a.Runtime.Environment()); err != nil {
		return fmt.Errorf("start managed SonarQube: %w", err)
	}
	if err := waitForHealthy(ctx, a.Server, a.startupBudget(), startupPollInterval, nil); err != nil {
		return err
	}
	return a.Client.Bootstrap(ctx)
}
func (a *Adapter) Plan(_ context.Context, req analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	if !analyzers.HasLanguage(req.Languages, a.SupportedLanguages()...) {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("sonarqube does not apply")
	}
	if a.Installer == nil {
		return analyzers.AnalyzerPlan{}, fmt.Errorf("managed SonarQube is not configured")
	}
	key := ProjectKey(req.WorkspaceID)
	return analyzers.AnalyzerPlan{AnalyzerID: ID, Version: a.Installer.Version(), Metadata: map[string]any{"project_key": key, "workspace_root": req.WorkspaceRoot, "exclusions": append([]string(nil), req.Exclusions...)}}, nil
}
func (a *Adapter) Run(ctx context.Context, p analyzers.AnalyzerPlan, e analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	if err := a.EnsureInstalled(ctx, analyzers.ToolEnvironment{}); err != nil {
		return analyzers.AnalyzerResult{Plan: p}, err
	}
	if err := a.EnsureRunning(ctx); err != nil {
		return analyzers.AnalyzerResult{Plan: p}, err
	}
	key, _ := p.Metadata["project_key"].(string)
	root, _ := p.Metadata["workspace_root"].(string)
	if err := a.Client.EnsureProject(ctx, key, key); err != nil {
		return analyzers.AnalyzerResult{Plan: p}, err
	}
	token, err := a.Client.Token(ctx)
	if err != nil {
		return analyzers.AnalyzerResult{Plan: p}, err
	}
	props, cleanup, err := ScannerProperties(a.DataDir, root, key, a.BaseURL, token, p.Metadata["exclusions"])
	if err != nil {
		return analyzers.AnalyzerResult{Plan: p}, err
	}
	defer cleanup()
	if e != nil {
		e.Emit(ctx, "analyzer.command_started", map[string]any{"analyzer_id": ID})
	}
	spec := analyzers.ProcessSpec{Executable: a.Installer.ScannerExecutable(), Args: []string{"-Dproject.settings=" + props}, Dir: root, Env: a.Runtime.Environment()}
	p.Commands = []analyzers.ProcessSpec{spec}
	return analyzers.RunDirect(ctx, p, e)
}
func (a *Adapter) Normalize(ctx context.Context, r analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	if r.ExitCode != 0 {
		return nil, nil, fmt.Errorf("sonar-scanner exited %d: %s", r.ExitCode, scannerFailureOutput(string(append(r.Stdout, r.Stderr...))))
	}
	key, _ := r.Plan.Metadata["project_key"].(string)
	task := scannerTask(string(r.Stdout))
	if task != "" {
		if err := a.Client.WaitForTask(ctx, task); err != nil {
			return nil, nil, err
		}
	}
	issues, err := a.Client.Issues(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	metrics, err := a.Client.Metrics(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	out := make([]analyzers.Finding, 0, len(issues))
	for _, x := range issues {
		f := x.Finding()
		f.RelativePath = sonarProjectPath(f.RelativePath, key)
		f.SetFingerprint()
		out = append(out, f)
	}
	return out, metrics, nil
}

// sonarProjectPath strips the "<project-key>:" prefix the issues API puts on
// component keys so reports show workspace-relative paths. The key must be
// trimmed as a whole: Blunt Code project keys contain colons themselves
// ("bluntcode:<workspace-id>"), so splitting at the first colon would leave
// the workspace id glued to every path.
func sonarProjectPath(component, projectKey string) string {
	return strings.TrimPrefix(component, projectKey+":")
}

func scannerFailureOutput(output string) string {
	output = strings.TrimSpace(output)
	if len(output) > 4096 {
		output = output[len(output)-4096:]
	}
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "sonar.token=") {
			lines[i] = "sonar.token=***"
		}
	}
	output = strings.Join(lines, "\n")
	if output == "" {
		return "no scanner output"
	}
	return output
}
func ProjectKey(workspaceID string) string {
	clean := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ':' || r == '_' || r == '-' {
			return r
		}
		return '-'
	}, workspaceID)
	return "bluntcode:" + clean
}
func ScannerProperties(dataDir, root, key, serverURL, token string, exclusions any) (string, func(), error) {
	if dataDir == "" {
		return "", nil, fmt.Errorf("Blunt Code application data directory is required for temporary scanner configuration")
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "tmp"), 0o700); err != nil {
		return "", nil, err
	}
	dir, err := os.MkdirTemp(filepath.Join(dataDir, "tmp"), "sonar-")
	if err != nil {
		return "", nil, err
	}
	p := filepath.Join(dir, "sonar-project.properties")
	var exclusion string
	if xs, ok := exclusions.([]string); ok {
		exclusion = strings.Join(xs, ",")
	}
	content := strings.Join([]string{"sonar.projectKey=" + key, "sonar.projectBaseDir=" + filepath.ToSlash(root), "sonar.sources=.", "sonar.sourceEncoding=UTF-8", "sonar.host.url=" + serverURL, "sonar.token=" + token, "sonar.exclusions=" + exclusion}, "\n") + "\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		os.RemoveAll(dir)
		return "", nil, err
	}
	return p, func() { os.RemoveAll(dir) }, nil
}
func scannerTask(stdout string) string {
	// The compute-engine task id reaches us in two shapes: a debug dump line
	// `ceTaskUrl=<url>`, and the default human-readable INFO line "More about
	// the report processing at <url>". The second is what normal scans emit;
	// missing it skipped WaitForTask entirely, so issues were fetched before
	// the server had processed the report and every scan looked clean.
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimRight(line, "\r")
		if i := strings.Index(line, "ceTaskUrl="); i >= 0 {
			if u, err := url.Parse(strings.TrimSpace(line[i+10:])); err == nil {
				return u.Query().Get("id")
			}
		}
		if i := strings.Index(line, "api/ce/task?id="); i >= 0 {
			if u, err := url.Parse(line[i:]); err == nil {
				return u.Query().Get("id")
			}
		}
	}
	return ""
}

type Issue struct {
	Key, Rule, Severity, Type, Message, Component string
	TextRange                                     *TextRange `json:"textRange"`
}
type TextRange struct {
	StartLine   int `json:"startLine"`
	StartOffset int `json:"startOffset"`
	EndLine     int `json:"endLine"`
	EndOffset   int `json:"endOffset"`
}

func (x Issue) Finding() analyzers.Finding {
	f := analyzers.Finding{AnalyzerID: ID, RuleID: x.Rule, Severity: sonarSeverity(x.Severity), Category: sonarCategory(x.Type), Title: x.Rule, Message: x.Message, RelativePath: x.Component, RawSeverity: x.Severity, Metadata: map[string]any{"issue_key": x.Key, "type": x.Type}}
	if x.TextRange != nil {
		f.StartLine = x.TextRange.StartLine
		f.StartColumn = x.TextRange.StartOffset
		f.EndLine = x.TextRange.EndLine
		f.EndColumn = x.TextRange.EndOffset
	}
	return f
}
func sonarSeverity(s string) analyzers.Severity {
	switch strings.ToUpper(s) {
	case "BLOCKER":
		return analyzers.SeverityCritical
	case "CRITICAL":
		return analyzers.SeverityHigh
	case "MAJOR":
		return analyzers.SeverityMedium
	case "MINOR":
		return analyzers.SeverityLow
	default:
		return analyzers.SeverityInfo
	}
}
func sonarCategory(t string) analyzers.Category {
	switch strings.ToUpper(t) {
	case "BUG":
		return analyzers.CategoryBug
	case "VULNERABILITY":
		return analyzers.CategoryVulnerability
	case "SECURITY_HOTSPOT":
		return analyzers.CategorySecurity
	case "CODE_SMELL":
		return analyzers.CategoryCodeSmell
	default:
		return analyzers.CategoryOther
	}
}

type apiIssueResponse struct {
	Issues []Issue `json:"issues"`
}

package scans

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/config"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/events"
)

type fakeAnalyzer struct{}

func (fakeAnalyzer) ID() string          { return "fake" }
func (fakeAnalyzer) DisplayName() string { return "Fake" }
func (fakeAnalyzer) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguagePython}
}
func (fakeAnalyzer) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	return analyzers.ToolStatus{Ready: true, Version: "test"}
}
func (fakeAnalyzer) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error { return nil }
func (fakeAnalyzer) Plan(context.Context, analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	return analyzers.AnalyzerPlan{AnalyzerID: "fake", Version: "test"}, nil
}
func (fakeAnalyzer) Run(context.Context, analyzers.AnalyzerPlan, analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	return analyzers.AnalyzerResult{}, nil
}
func (fakeAnalyzer) Normalize(context.Context, analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	f := analyzers.Finding{AnalyzerID: "fake", RuleID: "X1", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryCorrectness, Title: "Test finding", Message: "Found a test issue", RelativePath: "main.py", StartLine: 1}
	f.SetFingerprint()
	return []analyzers.Finding{f}, nil, nil
}

func TestPersistsRealNormalizedScanAndMarkdown(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.py"), []byte("x=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	work, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: "Fixture", RootPath: root})
	if err != nil {
		t.Fatal(err)
	}
	registry := analyzers.NewRegistry()
	if err := registry.Register(fakeAnalyzer{}); err != nil {
		t.Fatal(err)
	}
	service := New(db, registry, events.New(), filepath.Join(paths.DataDir, "reports"), paths.ToolsDir, nil)
	scan, err := service.DiscoverAndStart(context.Background(), work, "standard", nil)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, scanErr := db.Scan(context.Background(), scan.ID)
		if scanErr != nil {
			t.Fatal(scanErr)
		}
		if terminal(current.State) {
			if current.State != "completed" {
				t.Fatalf("scan state %q", current.State)
			}
			findings, findErr := db.Findings(context.Background(), scan.ID)
			if findErr != nil || len(findings) != 1 {
				t.Fatalf("findings %v %#v", findErr, findings)
			}
			report, reportErr := db.ReportPath(context.Background(), scan.ID)
			if reportErr != nil {
				t.Fatal(reportErr)
			}
			if _, statErr := os.Stat(report); statErr != nil {
				t.Fatal(statErr)
			}
			if current.Snapshot == nil || current.Snapshot.Profile != "standard" || current.Snapshot.Languages["python"] != 1 || current.Snapshot.AnalyzerVersions["fake"] != "test" {
				t.Fatalf("unexpected scan snapshot: %#v", current.Snapshot)
			}
			var manifestCount int
			if err := db.SQL.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM scan_files WHERE scan_id=? AND selected=1`, scan.ID).Scan(&manifestCount); err != nil || manifestCount != 1 {
				t.Fatalf("scan file manifest: count=%d err=%v", manifestCount, err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("scan did not finish")
}

func TestApplyPathOverridesUsesMostSpecificPath(t *testing.T) {
	files := []core.FileEntry{{RelativePath: "src/drop.py", Selected: true}, {RelativePath: "src/keep.py", Selected: true}, {RelativePath: "main.py", Selected: true}}
	applyPathOverrides(files, []core.PathOverride{{RelativePath: "src", Mode: "exclude"}, {RelativePath: "src/keep.py", Mode: "include"}})
	if files[0].Selected || !files[1].Selected || !files[2].Selected {
		t.Fatalf("unexpected selections: %#v", files)
	}
}

func TestAnalyzerTimeouts(t *testing.T) {
	if analyzerTimeout("ruff") != 10*time.Minute || analyzerTimeout("sonarqube") != 30*time.Minute {
		t.Fatalf("unexpected analyzer timeouts")
	}
}

// recordingAnalyzer captures the ScanRequest each Plan call receives so tests
// can assert what the orchestrator handed to an adapter, including the profile
// and the per-language file list.
type recordingAnalyzer struct {
	id        string
	languages []analyzers.Language
	mu        sync.Mutex
	requests  []analyzers.ScanRequest
}

func (r *recordingAnalyzer) ID() string          { return r.id }
func (r *recordingAnalyzer) DisplayName() string { return r.id }
func (r *recordingAnalyzer) SupportedLanguages() []analyzers.Language {
	if len(r.languages) > 0 {
		return r.languages
	}
	return []analyzers.Language{analyzers.LanguagePython}
}
func (r *recordingAnalyzer) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	return analyzers.ToolStatus{Ready: true, Version: "test"}
}
func (r *recordingAnalyzer) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error {
	return nil
}
func (r *recordingAnalyzer) Plan(_ context.Context, req analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	r.mu.Lock()
	r.requests = append(r.requests, req)
	r.mu.Unlock()
	return analyzers.AnalyzerPlan{AnalyzerID: r.id, Version: "test"}, nil
}
func (r *recordingAnalyzer) Run(context.Context, analyzers.AnalyzerPlan, analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	return analyzers.AnalyzerResult{}, nil
}
func (r *recordingAnalyzer) Normalize(context.Context, analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	return nil, nil, nil
}

func (r *recordingAnalyzer) lastProfile() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.requests) == 0 {
		return ""
	}
	return r.requests[len(r.requests)-1].Profile
}

func (r *recordingAnalyzer) requestCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

func (r *recordingAnalyzer) lastFiles() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.requests) == 0 {
		return nil
	}
	return r.requests[len(r.requests)-1].Files
}

// TestAdaptersReceiveOnlyFilesForTheirLanguages guards the orchestrator's
// per-language file routing. Dogfooding on Blunt Code's own repo found ruff
// being handed the full mixed-language file list, including a committed
// minified JavaScript bundle that ruff then parsed as Python.
func TestAdaptersReceiveOnlyFilesForTheirLanguages(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"main.py", "app.tsx", `static\index-ABC123.js`} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte("x = 1"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	work, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: "Mixed", RootPath: root})
	if err != nil {
		t.Fatal(err)
	}
	registry := analyzers.NewRegistry()
	pythonOnly := &recordingAnalyzer{id: "python-only"}
	webOnly := &recordingAnalyzer{id: "web-only", languages: []analyzers.Language{analyzers.LanguageJavaScript, analyzers.LanguageTypeScript}}
	for _, adapter := range []analyzers.Analyzer{pythonOnly, webOnly} {
		if err := registry.Register(adapter); err != nil {
			t.Fatal(err)
		}
	}
	service := New(db, registry, events.New(), filepath.Join(paths.DataDir, "reports"), paths.ToolsDir, nil)
	scan, err := service.DiscoverAndStart(context.Background(), work, "standard", nil)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, scanErr := db.Scan(context.Background(), scan.ID)
		if scanErr != nil {
			t.Fatal(scanErr)
		}
		if terminal(current.State) {
			if current.State != "completed" {
				t.Fatalf("scan state %q (error: %s)", current.State, current.ErrorSummary)
			}
			if got := pythonOnly.lastFiles(); len(got) != 1 || !strings.HasSuffix(got[0], "main.py") {
				t.Fatalf("python-only adapter files = %#v, want only main.py", got)
			}
			webFiles := webOnly.lastFiles()
			if len(webFiles) != 2 {
				t.Fatalf("web-only adapter files = %#v, want app.tsx and the bundle", webFiles)
			}
			for _, file := range webFiles {
				if strings.HasSuffix(file, ".py") {
					t.Fatalf("web-only adapter received Python file %q", file)
				}
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("scan did not finish")
}

func TestProfileTiersReachAnalyzersAndQuickSkipsHeavyAnalyzers(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.py"), []byte("x=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	work, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: "Profiles", RootPath: root})
	if err != nil {
		t.Fatal(err)
	}
	registry := analyzers.NewRegistry()
	fakes := map[string]*recordingAnalyzer{}
	for _, id := range []string{"ruff", "biome", "semgrep", "sonarqube"} {
		fakes[id] = &recordingAnalyzer{id: id}
		if err := registry.Register(fakes[id]); err != nil {
			t.Fatal(err)
		}
	}
	service := New(db, registry, events.New(), filepath.Join(paths.DataDir, "reports"), paths.ToolsDir, nil)

	runProfile := func(profile string) map[string]bool {
		t.Helper()
		scan, err := service.DiscoverAndStart(context.Background(), work, profile, nil)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			current, scanErr := db.Scan(context.Background(), scan.ID)
			if scanErr != nil {
				t.Fatal(scanErr)
			}
			if terminal(current.State) {
				if current.State != "completed" {
					t.Fatalf("%s scan state %q", profile, current.State)
				}
				runs, runErr := db.AnalyzerRuns(context.Background(), scan.ID)
				if runErr != nil {
					t.Fatal(runErr)
				}
				ran := map[string]bool{}
				for _, run := range runs {
					ran[run.AnalyzerID] = true
				}
				return ran
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("%s scan did not finish", profile)
		return nil
	}

	for _, profile := range []string{analyzers.ProfileStandard, analyzers.ProfileDeep} {
		ran := runProfile(profile)
		if len(ran) != 4 || !ran["ruff"] || !ran["biome"] || !ran["semgrep"] || !ran["sonarqube"] {
			t.Fatalf("%s profile ran %v, want all four analyzers", profile, ran)
		}
		if got := fakes["ruff"].lastProfile(); got != profile {
			t.Fatalf("%s profile reached ruff Plan as %q", profile, got)
		}
	}

	semgrepPlans, sonarPlans := fakes["semgrep"].requestCount(), fakes["sonarqube"].requestCount()
	ran := runProfile(analyzers.ProfileQuick)
	if len(ran) != 2 || !ran["ruff"] || !ran["biome"] {
		t.Fatalf("quick profile ran %v, want ruff and biome only", ran)
	}
	if got := fakes["ruff"].lastProfile(); got != analyzers.ProfileQuick {
		t.Fatalf("quick profile reached ruff Plan as %q", got)
	}
	if fakes["semgrep"].requestCount() != semgrepPlans || fakes["sonarqube"].requestCount() != sonarPlans {
		t.Fatal("quick profile must never plan semgrep or sonarqube")
	}
}

func TestDiagnosticLogRecordsPlannedCommand(t *testing.T) {
	dir := t.TempDir()
	s := &Service{reportsDir: filepath.Join(dir, "reports")}
	result := analyzers.AnalyzerResult{
		Plan: analyzers.AnalyzerPlan{AnalyzerID: "ruff", Commands: []analyzers.ProcessSpec{{
			Executable: filepath.Join(dir, "ruff.exe"),
			Args:       []string{"check", "--output-format", "json", "--no-fix", "--select=E,W,F", filepath.Join(dir, "main.py")},
			Dir:        dir,
		}}},
		Stdout: []byte("[]"),
	}
	s.writeDiagnosticLog("scan-1", "ruff", result)
	content, err := os.ReadFile(filepath.Join(dir, "logs", "scan-1-ruff.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "command:") || !strings.Contains(string(content), "--select=E,W,F") {
		t.Fatalf("diagnostic log does not record the planned command: %q", content)
	}
}

func TestShutdownCancelsActiveScans(t *testing.T) {
	service := &Service{cancels: map[string]context.CancelFunc{}}
	ctx, cancel := context.WithCancel(context.Background())
	service.cancels["scan"] = cancel
	service.Shutdown()
	if ctx.Err() == nil {
		t.Fatal("active scan context was not cancelled")
	}
}

func TestRedactDiagnosticOutput(t *testing.T) {
	got := redactDiagnosticOutput("ok\nsonar.token=private\nAuthorization: Bearer abc\nmessage")
	if strings.Contains(got, "private") || strings.Contains(got, "Bearer abc") || !strings.Contains(got, "message") {
		t.Fatalf("unexpected redaction: %q", got)
	}
}
func terminal(state string) bool {
	return state == "completed" || state == "completed_with_warnings" || state == "failed" || state == "cancelled"
}

// scriptedAnalyzer is a controllable fake for lifecycle race tests. Every gate
// is optional (nil means "do not block"), so a test can freeze a scan at a
// precise point - inside Run, inside Normalize - and then cancel, release, or
// panic it from the test goroutine.
type scriptedAnalyzer struct {
	id               string
	notReady         bool
	panicInRun       bool
	findings         []analyzers.Finding
	runEntered       chan struct{}
	runGate          chan struct{}
	normalizeEntered chan struct{}
	normalizeGate    chan struct{}
}

func (a *scriptedAnalyzer) ID() string          { return a.id }
func (a *scriptedAnalyzer) DisplayName() string { return a.id }
func (a *scriptedAnalyzer) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguagePython}
}
func (a *scriptedAnalyzer) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	if a.notReady {
		return analyzers.ToolStatus{Detail: "scripted analyzer is not installed"}
	}
	return analyzers.ToolStatus{Ready: true, Version: "test"}
}
func (a *scriptedAnalyzer) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error {
	return nil
}
func (a *scriptedAnalyzer) Plan(context.Context, analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	return analyzers.AnalyzerPlan{AnalyzerID: a.id, Version: "test"}, nil
}
func (a *scriptedAnalyzer) Run(context.Context, analyzers.AnalyzerPlan, analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	closeOnce(a.runEntered)
	if a.runGate != nil {
		<-a.runGate
	}
	if a.panicInRun {
		panic("scripted analyzer failure")
	}
	return analyzers.AnalyzerResult{}, nil
}
func (a *scriptedAnalyzer) Normalize(context.Context, analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	closeOnce(a.normalizeEntered)
	if a.normalizeGate != nil {
		<-a.normalizeGate
	}
	return a.findings, nil, nil
}

func closeOnce(ch chan struct{}) {
	if ch == nil {
		return
	}
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func pythonWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.py"), []byte("x=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

// scanFixture wires a real service against a temp SQLite database so tests can
// observe persisted state transitions, not just return values.
type scanFixture struct {
	db      *database.DB
	bus     *events.Bus
	service *Service
	work    core.Workspace
}

func newScanFixture(t *testing.T, adapters ...analyzers.Analyzer) *scanFixture {
	t.Helper()
	return newScanFixtureWithReportsDir(t, "", adapters...)
}

// newScanFixtureWithReportsDir allows pointing the reports directory at a path
// that cannot be created (its parent is a regular file) to simulate report
// generation failure.
func newScanFixtureWithReportsDir(t *testing.T, reportsDir string, adapters ...analyzers.Analyzer) *scanFixture {
	t.Helper()
	root := pythonWorkspace(t)
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	work, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: "Fixture", RootPath: root})
	if err != nil {
		t.Fatal(err)
	}
	registry := analyzers.NewRegistry()
	for _, adapter := range adapters {
		if err := registry.Register(adapter); err != nil {
			t.Fatal(err)
		}
	}
	if reportsDir == "" {
		reportsDir = filepath.Join(paths.DataDir, "reports")
	}
	bus := events.New()
	return &scanFixture{db: db, bus: bus, service: New(db, registry, bus, reportsDir, paths.ToolsDir, nil), work: work}
}

func (f *scanFixture) startScan(t *testing.T) core.Scan {
	t.Helper()
	scan, err := f.service.DiscoverAndStart(context.Background(), f.work, "standard", nil)
	if err != nil {
		t.Fatal(err)
	}
	return scan
}

func (f *scanFixture) waitForTerminal(t *testing.T, scanID string) core.Scan {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		current, err := f.db.Scan(context.Background(), scanID)
		if err != nil {
			t.Fatal(err)
		}
		if terminal(current.State) {
			return current
		}
		time.Sleep(10 * time.Millisecond)
	}
	current, _ := f.db.Scan(context.Background(), scanID)
	t.Fatalf("scan %s never reached a terminal state (last state %q)", scanID, current.State)
	return core.Scan{}
}

// waitTerminalEvent consumes events until a terminal scan event arrives and
// returns its type, proving the SSE pipeline observed the lifecycle end.
func waitTerminalEvent(t *testing.T, ch <-chan events.Event) string {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case event := <-ch:
			if event.Type == "scan.completed" || event.Type == "scan.cancelled" {
				return event.Type
			}
		case <-deadline:
			t.Fatal("no terminal scan event was published")
			return ""
		}
	}
}

// TestCancelDuringFinalAnalyzerNormalizeMarksCancelled pins the tail of the
// analyzer loop: cancellation that lands while the last analyzer normalizes or
// persists results (after the post-Run context check) must still cancel the
// scan instead of completing it.
func TestCancelDuringFinalAnalyzerNormalizeMarksCancelled(t *testing.T) {
	analyzer := &scriptedAnalyzer{id: "fake", normalizeEntered: make(chan struct{}), normalizeGate: make(chan struct{})}
	fixture := newScanFixture(t, analyzer)
	scan := fixture.startScan(t)
	ch, unsubscribe := fixture.bus.Subscribe(scan.ID)
	defer unsubscribe()
	<-analyzer.normalizeEntered
	if err := fixture.service.Cancel(context.Background(), scan.ID); err != nil {
		t.Fatal(err)
	}
	close(analyzer.normalizeGate)
	if final := fixture.waitForTerminal(t, scan.ID); final.State != "cancelled" {
		t.Fatalf("scan state after cancel during final analyzer = %q, want cancelled", final.State)
	}
	if got := waitTerminalEvent(t, ch); got != "scan.cancelled" {
		t.Fatalf("terminal event = %q, want scan.cancelled", got)
	}
}

// TestAnalyzerPanicFailsScanInsteadOfCrashing proves a panic inside an adapter
// is contained: the scan row must land in a terminal failed state and a
// terminal event must be published rather than taking the process down.
func TestAnalyzerPanicFailsScanInsteadOfCrashing(t *testing.T) {
	analyzer := &scriptedAnalyzer{id: "fake", panicInRun: true, runEntered: make(chan struct{})}
	fixture := newScanFixture(t, analyzer)
	scan := fixture.startScan(t)
	ch, unsubscribe := fixture.bus.Subscribe(scan.ID)
	defer unsubscribe()
	<-analyzer.runEntered
	if final := fixture.waitForTerminal(t, scan.ID); final.State != "failed" {
		t.Fatalf("scan state after analyzer panic = %q, want failed", final.State)
	}
	if got := waitTerminalEvent(t, ch); got != "scan.completed" {
		t.Fatalf("terminal event after analyzer panic = %q, want scan.completed", got)
	}
	// The service must remain usable for the next scan after a contained panic.
	replacement := &scriptedAnalyzer{id: "fake"}
	fixture.service.registry = analyzers.NewRegistry()
	if err := fixture.service.registry.Register(replacement); err != nil {
		t.Fatal(err)
	}
	recovery := fixture.startScan(t)
	if final := fixture.waitForTerminal(t, recovery.ID); final.State != "completed" {
		t.Fatalf("scan after contained panic state = %q, want completed", final.State)
	}
}

// TestDoubleCancelIsIdempotentAndThenRejected checks cancel-of-cancel: while
// the scan is winding down a second cancel is a safe no-op, and after the run
// goroutine exits the cancel entry is gone and further cancels error.
func TestDoubleCancelIsIdempotentAndThenRejected(t *testing.T) {
	analyzer := &scriptedAnalyzer{id: "fake", runEntered: make(chan struct{}), runGate: make(chan struct{})}
	fixture := newScanFixture(t, analyzer)
	scan := fixture.startScan(t)
	<-analyzer.runEntered
	if err := fixture.service.Cancel(context.Background(), scan.ID); err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	if err := fixture.service.Cancel(context.Background(), scan.ID); err != nil {
		t.Fatalf("second cancel must be a safe no-op: %v", err)
	}
	close(analyzer.runGate)
	fixture.waitForTerminal(t, scan.ID)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := fixture.service.Cancel(context.Background(), scan.ID); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("canceling a finished scan must return an error")
}

// TestUnwritableReportDirWithFailedAnalyzersStaysFailed covers the state
// precedence bug: when every analyzer failed and the report also cannot be
// written, the scan must remain failed, not silently become
// completed_with_warnings.
func TestUnwritableReportDirWithFailedAnalyzersStaysFailed(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("file, not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	analyzer := &scriptedAnalyzer{id: "fake", notReady: true}
	fixture := newScanFixtureWithReportsDir(t, filepath.Join(blocker, "reports"), analyzer)
	scan := fixture.startScan(t)
	final := fixture.waitForTerminal(t, scan.ID)
	if final.State != "failed" {
		t.Fatalf("scan state = %q, want failed", final.State)
	}
	if final.FinishedAt == nil {
		t.Fatal("failed scan must record a finish time")
	}
}

// TestUnwritableReportDirStillCompletesScan guards the happy path of report
// failure: analyzers that succeeded mean the scan completes with warnings and
// a terminal event, never stuck in a generating state.
func TestUnwritableReportDirStillCompletesScan(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("file, not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	analyzer := &scriptedAnalyzer{id: "fake", findings: []analyzers.Finding{testFinding("fake")}}
	fixture := newScanFixtureWithReportsDir(t, filepath.Join(blocker, "reports"), analyzer)
	scan := fixture.startScan(t)
	ch, unsubscribe := fixture.bus.Subscribe(scan.ID)
	defer unsubscribe()
	final := fixture.waitForTerminal(t, scan.ID)
	if final.State != "completed_with_warnings" {
		t.Fatalf("scan state = %q, want completed_with_warnings", final.State)
	}
	path, err := fixture.db.ReportPath(context.Background(), scan.ID)
	if err != nil || path != "" {
		t.Fatalf("report path = %q, %v; want empty for a failed report write", path, err)
	}
	waitTerminalEvent(t, ch)
}

func testFinding(analyzerID string) analyzers.Finding {
	f := analyzers.Finding{AnalyzerID: analyzerID, RuleID: "X1", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryCorrectness, Title: "Test finding", Message: "Found a test issue", RelativePath: "main.py", StartLine: 1}
	f.SetFingerprint()
	return f
}

// TestPreviousCompletedScanSkipsCancelledScan checks comparison resolution
// across failures: after A completes, B is cancelled mid-run, and C completes,
// C must compare against A - never the cancelled B.
func TestPreviousCompletedScanSkipsCancelledScan(t *testing.T) {
	finder := &scriptedAnalyzer{id: "fake", findings: []analyzers.Finding{testFinding("fake")}}
	fixture := newScanFixture(t, finder)
	scanA := fixture.startScan(t)
	if final := fixture.waitForTerminal(t, scanA.ID); final.State != "completed" {
		t.Fatalf("scan A state = %q", final.State)
	}

	cancellable := &scriptedAnalyzer{id: "fake", runEntered: make(chan struct{}), runGate: make(chan struct{})}
	fixture.service.registry = analyzers.NewRegistry()
	if err := fixture.service.registry.Register(cancellable); err != nil {
		t.Fatal(err)
	}
	scanB := fixture.startScan(t)
	<-cancellable.runEntered
	if err := fixture.service.Cancel(context.Background(), scanB.ID); err != nil {
		t.Fatal(err)
	}
	close(cancellable.runGate)
	if final := fixture.waitForTerminal(t, scanB.ID); final.State != "cancelled" {
		t.Fatalf("scan B state = %q", final.State)
	}

	fixture.service.registry = analyzers.NewRegistry()
	if err := fixture.service.registry.Register(&scriptedAnalyzer{id: "fake", findings: nil}); err != nil {
		t.Fatal(err)
	}
	scanC := fixture.startScan(t)
	if final := fixture.waitForTerminal(t, scanC.ID); final.State != "completed" {
		t.Fatalf("scan C state = %q", final.State)
	}
	previousID, err := fixture.db.PreviousCompletedScanID(context.Background(), fixture.work.ID, scanC.ID)
	if err != nil {
		t.Fatal(err)
	}
	if previousID != scanA.ID {
		t.Fatalf("previous completed scan = %s, want completed scan %s (not cancelled %s)", previousID, scanA.ID, scanB.ID)
	}
	fixed, err := fixture.db.FixedFindings(context.Background(), func() core.Scan {
		current, err := fixture.db.Scan(context.Background(), scanC.ID)
		if err != nil {
			t.Fatal(err)
		}
		return current
	}(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if !fixed.ComparisonAvailable || fixed.Total != 1 {
		t.Fatalf("fixed findings = available:%v total:%d, want A's finding resolved as fixed", fixed.ComparisonAvailable, fixed.Total)
	}
}

// TestTerminalEventEmittedWhenPersistenceFails closes the database while the
// scan is mid-run: even if the final scan update fails, subscribers must still
// receive a terminal event instead of hanging on a stream that never ends.
func TestTerminalEventEmittedWhenPersistenceFails(t *testing.T) {
	analyzer := &scriptedAnalyzer{id: "fake", runEntered: make(chan struct{}), runGate: make(chan struct{})}
	fixture := newScanFixture(t, analyzer)
	scan := fixture.startScan(t)
	ch, unsubscribe := fixture.bus.Subscribe(scan.ID)
	defer unsubscribe()
	<-analyzer.runEntered
	if err := fixture.db.Close(); err != nil {
		t.Fatal(err)
	}
	close(analyzer.runGate)
	if got := waitTerminalEvent(t, ch); got != "scan.completed" {
		t.Fatalf("terminal event = %q, want scan.completed", got)
	}
}

// TestConcurrentScansAcrossWorkspaces exercises the non-DB shared state of the
// service (cancel registry, event bus) with two workspaces scanning at once.
func TestConcurrentScansAcrossWorkspaces(t *testing.T) {
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	registry := analyzers.NewRegistry()
	if err := registry.Register(&scriptedAnalyzer{id: "fake", findings: []analyzers.Finding{testFinding("fake")}}); err != nil {
		t.Fatal(err)
	}
	bus := events.New()
	service := New(db, registry, bus, filepath.Join(paths.DataDir, "reports"), paths.ToolsDir, nil)
	first, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: "One", RootPath: pythonWorkspace(t)})
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: "Two", RootPath: pythonWorkspace(t)})
	if err != nil {
		t.Fatal(err)
	}
	scanOne, err := service.DiscoverAndStart(context.Background(), first, "standard", nil)
	if err != nil {
		t.Fatal(err)
	}
	scanTwo, err := service.DiscoverAndStart(context.Background(), second, "standard", nil)
	if err != nil {
		t.Fatal(err)
	}
	waitTerminal := func(scanID string) core.Scan {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			current, scanErr := db.Scan(context.Background(), scanID)
			if scanErr != nil {
				t.Fatal(scanErr)
			}
			if terminal(current.State) {
				return current
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("scan %s did not finish", scanID)
		return core.Scan{}
	}
	if final := waitTerminal(scanOne.ID); final.State != "completed" {
		t.Fatalf("workspace one scan state = %q", final.State)
	}
	if final := waitTerminal(scanTwo.ID); final.State != "completed" {
		t.Fatalf("workspace two scan state = %q", final.State)
	}
	if err := service.Cancel(context.Background(), scanOne.ID); err == nil {
		t.Fatal("canceling a finished scan must fail")
	}
}

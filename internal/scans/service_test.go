package scans

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

package scans

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/config"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/events"
)

// warnedAnalyzer succeeds but reports degraded coverage, mirroring an adapter
// whose batched output partially failed to parse.
type warnedAnalyzer struct{}

func (warnedAnalyzer) ID() string          { return "warned" }
func (warnedAnalyzer) DisplayName() string { return "Warned" }
func (warnedAnalyzer) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguagePython}
}
func (warnedAnalyzer) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	return analyzers.ToolStatus{Ready: true, Version: "test"}
}
func (warnedAnalyzer) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error { return nil }
func (warnedAnalyzer) Plan(context.Context, analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	return analyzers.AnalyzerPlan{AnalyzerID: "warned", Version: "test"}, nil
}
func (warnedAnalyzer) Run(context.Context, analyzers.AnalyzerPlan, analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	return analyzers.AnalyzerResult{Warnings: []string{"batch 1/2 produced unparseable output; its findings are not included"}}, nil
}
func (warnedAnalyzer) Normalize(context.Context, analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	f := analyzers.Finding{AnalyzerID: "warned", RuleID: "W1", Severity: analyzers.SeverityLow, Category: analyzers.CategoryCorrectness, Title: "Partial", Message: "Partial coverage", RelativePath: "main.py", StartLine: 1}
	f.SetFingerprint()
	return []analyzers.Finding{f}, nil, nil
}

// waitForTerminalState polls the scan row until it reaches any terminal state.
func waitForTerminalState(t *testing.T, db *database.DB, scanID string) core.Scan {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, err := db.Scan(context.Background(), scanID)
		if err != nil {
			t.Fatal(err)
		}
		if terminal(current.State) {
			return current
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("scan did not finish")
	return core.Scan{}
}

func runOutcomeScan(t *testing.T, adapters ...analyzers.Analyzer) (*database.DB, core.Scan) {
	t.Helper()
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
	t.Cleanup(func() { db.Close() })
	work, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: "Outcome", RootPath: root})
	if err != nil {
		t.Fatal(err)
	}
	registry := analyzers.NewRegistry()
	for _, adapter := range adapters {
		if err := registry.Register(adapter); err != nil {
			t.Fatal(err)
		}
	}
	service := New(db, registry, events.New(), filepath.Join(paths.DataDir, "reports"), paths.ToolsDir, nil)
	scan, err := service.DiscoverAndStart(context.Background(), work, "standard", nil)
	if err != nil {
		t.Fatal(err)
	}
	return db, waitForTerminalState(t, db, scan.ID)
}

// TestWarnedRunCompletesWithWarnings pins the honest-outcome contract: a scan
// where an analyzer succeeded with degraded coverage is completed_with_warnings
// (never a clean completed), the warning count is persisted on the analyzer
// run, and the findings that did parse still land.
func TestWarnedRunCompletesWithWarnings(t *testing.T) {
	db, scan := runOutcomeScan(t, fakeAnalyzer{}, warnedAnalyzer{})
	if scan.State != "completed_with_warnings" {
		t.Fatalf("scan state %q, want completed_with_warnings", scan.State)
	}
	runs, err := db.AnalyzerRuns(context.Background(), scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	var warnedSeen bool
	for _, run := range runs {
		if run.AnalyzerID != "warned" {
			continue
		}
		warnedSeen = true
		if run.State != "succeeded" || run.WarningCount != 1 {
			t.Fatalf("warned run = state %q warning_count %d, want succeeded/1", run.State, run.WarningCount)
		}
	}
	if !warnedSeen {
		t.Fatalf("warned analyzer run missing: %#v", runs)
	}
	findings, err := db.Findings(context.Background(), scan.ID)
	if err != nil || len(findings) != 2 {
		t.Fatalf("findings %v %#v", err, findings)
	}
}

// TestOnlyWarnedRunStillCompletesWithWarnings covers a lone analyzer that both
// succeeds and warns: successful > 0 keeps the scan out of failed, but the
// warning still downgrades it from a clean completed.
func TestOnlyWarnedRunStillCompletesWithWarnings(t *testing.T) {
	_, scan := runOutcomeScan(t, warnedAnalyzer{})
	if scan.State != "completed_with_warnings" {
		t.Fatalf("scan state %q, want completed_with_warnings", scan.State)
	}
}

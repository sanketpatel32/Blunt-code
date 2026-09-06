package scans

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/config"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/events"
)

// dependencyOnlyAnalyzer borrows the osv-dependencies inventory entry so the
// orchestrator's dependency-input routing (analyzers.TakesDependencyInputs)
// applies to it, without needing the real scanner executable.
type dependencyOnlyAnalyzer struct{}

func (dependencyOnlyAnalyzer) ID() string          { return "osv-dependencies" }
func (dependencyOnlyAnalyzer) DisplayName() string { return "OSV Scanner" }
func (dependencyOnlyAnalyzer) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguagePython}
}
func (dependencyOnlyAnalyzer) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	return analyzers.ToolStatus{Ready: true, Version: "test"}
}
func (dependencyOnlyAnalyzer) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error { return nil }
func (dependencyOnlyAnalyzer) Plan(context.Context, analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	return analyzers.AnalyzerPlan{AnalyzerID: "osv-dependencies", Version: "test"}, nil
}
func (dependencyOnlyAnalyzer) Run(context.Context, analyzers.AnalyzerPlan, analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	return analyzers.AnalyzerResult{}, nil
}
func (dependencyOnlyAnalyzer) Normalize(context.Context, analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	return nil, nil, nil
}

// TestDependencyAnalyzerRunsOnLockfileOnlyWorkspace pins the IMP-02 routing
// fix: a workspace whose only dependency signal is a lockfile (package.json
// classifies as json; the lockfile itself is artifact-skipped) has no
// language-routable file for a dependency analyzer, yet the scan must still
// run it — dependency inputs, discovered and snapshotted during the walk,
// keep it eligible. The snapshot must also record what was seen.
func TestDependencyAnalyzerRunsOnLockfileOnlyWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package-lock.json"), []byte(`{"lockfileVersion":3}`), 0o600); err != nil {
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
	work, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: "LockfileOnly", RootPath: root})
	if err != nil {
		t.Fatal(err)
	}
	registry := analyzers.NewRegistry()
	if err := registry.Register(dependencyOnlyAnalyzer{}); err != nil {
		t.Fatal(err)
	}
	service := New(db, registry, events.New(), filepath.Join(paths.DataDir, "reports"), paths.ToolsDir, nil)
	scan, err := service.DiscoverAndStart(context.Background(), work, "deep", nil)
	if err != nil {
		t.Fatal(err)
	}
	final := waitForTerminalState(t, db, scan.ID)
	if final.State != "completed" {
		t.Fatalf("scan state %q", final.State)
	}
	runs, err := db.AnalyzerRuns(context.Background(), scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].AnalyzerID != "osv-dependencies" || runs[0].State != "succeeded" {
		t.Fatalf("dependency analyzer must run and succeed on a lockfile-only workspace: %+v", runs)
	}
	if final.Snapshot == nil {
		t.Fatal("snapshot missing")
	}
	found := map[string]bool{}
	for _, input := range final.Snapshot.DependencyInputs {
		found[input] = true
	}
	if !found["package.json"] || !found["package-lock.json"] {
		t.Fatalf("snapshot dependency inputs = %v, want package.json and package-lock.json", final.Snapshot.DependencyInputs)
	}
	if final.Snapshot.SkipCounts == nil || final.Snapshot.SkipCounts["excluded_default"] < 1 {
		t.Fatalf("snapshot skip counts must explain the excluded lockfile: %+v", final.Snapshot.SkipCounts)
	}
}

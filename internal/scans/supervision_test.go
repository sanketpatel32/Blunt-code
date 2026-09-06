package scans

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/config"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/events"
)

// blockingAnalyzer routes to Python files and then parks inside Run until a
// shared channel closes, holding its scan in the running state.
type blockingAnalyzer struct{ release <-chan struct{} }

func (blockingAnalyzer) ID() string          { return "blocking" }
func (blockingAnalyzer) DisplayName() string { return "Blocking" }
func (blockingAnalyzer) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguagePython}
}
func (blockingAnalyzer) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	return analyzers.ToolStatus{Ready: true, Version: "test"}
}
func (blockingAnalyzer) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error {
	return nil
}
func (blockingAnalyzer) Plan(context.Context, analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	return analyzers.AnalyzerPlan{AnalyzerID: "blocking", Version: "test"}, nil
}
func (a blockingAnalyzer) Run(context.Context, analyzers.AnalyzerPlan, analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	<-a.release
	return analyzers.AnalyzerResult{}, nil
}
func (blockingAnalyzer) Normalize(context.Context, analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	return nil, nil, nil
}

// newSupervisionService builds a service with the given adapters around one
// Python workspace, so routed analyzers actually execute.
func newSupervisionService(t *testing.T, adapters ...analyzers.Analyzer) (*database.DB, *Service, core.Workspace) {
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
	work, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: "Supervision", RootPath: root})
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
	return db, service, work
}

// TestConcurrentScanCapRejectsBeyondCapacity pins the machine-load bound:
// while MaxConcurrentScans scans are running, the next start is refused with
// ErrTooManyScans (surfaced as HTTP 503 / CLI exit 3), and capacity returns
// once the running scans finish.
func TestConcurrentScanCapRejectsBeyondCapacity(t *testing.T) {
	release := make(chan struct{})
	db, service, work := newSupervisionService(t, blockingAnalyzer{release: release})
	var ids []string
	for i := 0; i < MaxConcurrentScans; i++ {
		scan, err := service.DiscoverAndStart(context.Background(), work, "standard", nil)
		if err != nil {
			t.Fatalf("start %d: %v", i+1, err)
		}
		ids = append(ids, scan.ID)
	}
	if _, err := service.DiscoverAndStart(context.Background(), work, "standard", nil); !errors.Is(err, ErrTooManyScans) {
		t.Fatalf("start beyond capacity: want ErrTooManyScans, got %v", err)
	}
	close(release)
	for _, id := range ids {
		waitForTerminalState(t, db, id)
	}
	scan, err := service.DiscoverAndStart(context.Background(), work, "standard", nil)
	if err != nil {
		t.Fatalf("start after drain: %v", err)
	}
	waitForTerminalState(t, db, scan.ID)
}

func hashTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestScanLeavesWorkspaceUnchanged pins the read-only source contract with
// evidence instead of intent: every file in the workspace hashes identically
// before and after a completed scan, and the scan writes nothing new into it.
func TestScanLeavesWorkspaceUnchanged(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"main.py":             "x = 1\n",
		"pkg/app.ts":          "export const x = 1;\n",
		"README.md":           "# readme\n",
		"package.json":        `{"name": "fixture"}` + "\n",
		"package-lock.json":   `{"lockfileVersion": 3}` + "\n",
		".bluntcodeignore":    "vendor/\n",
		"vendor/generated.js": "// generated\n",
	}
	for rel, content := range files {
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(rel)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := hashTree(t, root)

	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	work, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: "ReadOnly", RootPath: root})
	if err != nil {
		t.Fatal(err)
	}
	registry := analyzers.NewRegistry()
	for _, adapter := range []analyzers.Analyzer{fakeAnalyzer{}, warnedAnalyzer{}} {
		if err := registry.Register(adapter); err != nil {
			t.Fatal(err)
		}
	}
	service := New(db, registry, events.New(), filepath.Join(paths.DataDir, "reports"), paths.ToolsDir, nil)
	scan, err := service.DiscoverAndStart(context.Background(), work, "standard", nil)
	if err != nil {
		t.Fatal(err)
	}
	final := waitForTerminalState(t, db, scan.ID)
	if final.State != "completed_with_warnings" && final.State != "completed" {
		t.Fatalf("scan state %q, want a successful terminal state", final.State)
	}

	after := hashTree(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("workspace changed during scan:\nbefore=%v\nafter=%v", before, after)
	}
}

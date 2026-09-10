package scans

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/config"
	"bluntcode/internal/database"
	"bluntcode/internal/events"
)

// statusProbeAnalyzer counts Check probes so the AnalyzerStatuses cache is
// observable: every probe is an exec of the real tool binary in production.
type statusProbeAnalyzer struct{ probes atomic.Int64 }

func (a *statusProbeAnalyzer) ID() string          { return "ruff" }
func (a *statusProbeAnalyzer) DisplayName() string { return "Status Probe" }
func (a *statusProbeAnalyzer) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguagePython, analyzers.LanguageGo}
}
func (a *statusProbeAnalyzer) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	a.probes.Add(1)
	return analyzers.ToolStatus{Ready: true, Version: "1.2.3"}
}
func (a *statusProbeAnalyzer) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error {
	return nil
}
func (a *statusProbeAnalyzer) Plan(context.Context, analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	return analyzers.AnalyzerPlan{AnalyzerID: a.ID()}, nil
}
func (a *statusProbeAnalyzer) Run(context.Context, analyzers.AnalyzerPlan, analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	return analyzers.AnalyzerResult{}, nil
}
func (a *statusProbeAnalyzer) Normalize(context.Context, analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	return nil, nil, nil
}

// TestAnalyzerStatusesCachesAndPreservesOrder pins the status snapshot
// contract: one probe per adapter per TTL window, cached snapshots reused
// without re-probing, capability order stable across calls, and a fresh
// probe once the cache expires.
func TestAnalyzerStatusesCachesAndPreservesOrder(t *testing.T) {
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	adapter := &statusProbeAnalyzer{}
	registry := analyzers.NewRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	service := New(db, registry, events.New(), filepath.Join(paths.DataDir, "reports"), paths.ToolsDir, nil)

	first := service.AnalyzerStatuses(context.Background())
	if adapter.probes.Load() != 1 {
		t.Fatalf("first snapshot must probe once, probes=%d", adapter.probes.Load())
	}
	var seen bool
	for _, status := range first {
		if status.ID == "ruff" {
			seen = true
			if !status.Registered || !status.Ready || status.Version != "1.2.3" {
				t.Fatalf("registered row wrong: %#v", status)
			}
		}
	}
	if !seen {
		t.Fatalf("capability row missing: %#v", first)
	}

	second := service.AnalyzerStatuses(context.Background())
	if adapter.probes.Load() != 1 {
		t.Fatalf("cached snapshot must not re-probe within TTL, probes=%d", adapter.probes.Load())
	}
	if len(first) != len(second) {
		t.Fatalf("snapshot size changed: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].ID != second[i].ID || first[i].Version != second[i].Version {
			t.Fatalf("order not preserved at %d: %#v vs %#v", i, first[i], second[i])
		}
	}

	service.statusMu.Lock()
	service.statusCachedAt = time.Now().Add(-2 * analyzerStatusCacheTTL)
	service.statusMu.Unlock()
	_ = service.AnalyzerStatuses(context.Background())
	if adapter.probes.Load() != 2 {
		t.Fatalf("expired cache must re-probe, probes=%d", adapter.probes.Load())
	}
}

package scans

import (
	"bluntcode/internal/analyzers"
	"bluntcode/internal/config"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/events"
	"context"
	"os"
	"path/filepath"
	"testing"
)

type planRuff struct{ fakeAnalyzer }

func (planRuff) ID() string { return "ruff" }

func TestPlanUsesDiscoveryOverridesAndProfileWithoutCreatingScan(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	for _, name := range []string{"keep.py", "drop.py"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x=1"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(ctx, paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	work, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Plan", RootPath: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReplacePathOverrides(ctx, work.ID, []core.PathOverride{{RelativePath: "drop.py", Mode: "exclude"}}); err != nil {
		t.Fatal(err)
	}
	registry := analyzers.NewRegistry()
	if err := registry.Register(planRuff{}); err != nil {
		t.Fatal(err)
	}
	service := New(db, registry, events.New(), paths.ReportsDir, paths.ToolsDir, nil)
	plan, err := service.Plan(ctx, work, "quick", nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CandidateFiles != 2 || plan.SelectedFiles != 1 {
		t.Fatalf("selection = %+v", plan)
	}
	for _, row := range plan.Analyzers {
		if row.ID == "ruff" && (!row.Planned || !row.Ready) {
			t.Fatalf("ruff = %+v", row)
		}
		if row.ID != "ruff" && row.Planned {
			t.Fatalf("unregistered analyzer planned: %+v", row)
		}
	}
	var count int
	if err := db.SQL.QueryRowContext(ctx, "SELECT COUNT(*) FROM scans").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("review created %d scans", count)
	}
	plan, err = service.Plan(ctx, work, "quick", []string{"*.py"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.SelectedFiles != 0 {
		t.Fatalf("excluded files selected: %+v", plan)
	}
	for _, row := range plan.Analyzers {
		if row.Planned {
			t.Fatalf("empty source plan contains %+v", row)
		}
	}
}

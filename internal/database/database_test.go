package database

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
)

func TestMigrationAndWorkspacePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bluntcode.db")
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	created, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: "Sample", RootPath: "C:/sample"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := db.Workspace(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.RootPath != "C:/sample" || got.Name != "Sample" {
		t.Fatalf("unexpected persisted workspace: %#v", got)
	}
}

func TestCreateScanPersistsImmutableSnapshotAndFileManifest(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	work, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: "Sample", RootPath: "C:/sample"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := &core.ScanSnapshot{WorkspaceID: work.ID, WorkspaceRoot: work.RootPath, WorkspaceName: work.Name, Profile: "standard", SelectedFileCount: 1, SelectedFiles: []string{"src/main.py"}, Languages: map[string]int{"python": 1}, AnalyzerVersions: map[string]string{"ruff": "0.16.0"}}
	scan, err := db.CreateScanWithFiles(context.Background(), core.Scan{WorkspaceID: work.ID, State: "queued", Profile: "standard", CandidateFileCount: 1, SelectedFileCount: 1, Snapshot: snapshot}, []core.FileEntry{{RelativePath: "src/main.py", Language: "python", SizeBytes: 12, Selected: true}})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := db.Scan(context.Background(), scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Snapshot == nil || loaded.Snapshot.AnalyzerVersions["ruff"] != "0.16.0" || len(loaded.Snapshot.SelectedFiles) != 1 {
		t.Fatalf("snapshot was not restored: %#v", loaded.Snapshot)
	}
	var path string
	var selected int
	if err := db.SQL.QueryRowContext(context.Background(), `SELECT relative_path,selected FROM scan_files WHERE scan_id=?`, scan.ID).Scan(&path, &selected); err != nil {
		t.Fatal(err)
	}
	if path != "src/main.py" || selected != 1 {
		t.Fatalf("unexpected scan manifest %q/%d", path, selected)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(loaded.SnapshotJSON), &raw); err != nil || raw["workspace_id"] != work.ID {
		t.Fatalf("snapshot JSON is not durable: %v %#v", err, raw)
	}
}

func TestReplacePathOverrides(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	work, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: "Sample", RootPath: "C:/sample"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReplacePathOverrides(context.Background(), work.ID, []core.PathOverride{{RelativePath: "src", Mode: "exclude"}, {RelativePath: "src/keep.py", Mode: "include"}}); err != nil {
		t.Fatal(err)
	}
	overrides, err := db.PathOverrides(context.Background(), work.ID)
	if err != nil || len(overrides) != 2 || overrides[0].Mode != "exclude" || overrides[1].Mode != "include" {
		t.Fatalf("unexpected overrides: %#v (%v)", overrides, err)
	}
	if err := db.ReplacePathOverrides(context.Background(), work.ID, nil); err != nil {
		t.Fatal(err)
	}
	overrides, err = db.PathOverrides(context.Background(), work.ID)
	if err != nil || len(overrides) != 0 {
		t.Fatalf("overrides were not reset: %#v (%v)", overrides, err)
	}
}

func TestAppSettingsDefaultBrowserAndLegacyCompatibility(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	settings, err := db.AppSettings(context.Background())
	if err != nil || !settings.OpenBrowser {
		t.Fatalf("new settings=%#v err=%v", settings, err)
	}
	if _, err := db.SQL.ExecContext(context.Background(), `INSERT INTO settings(key,value_json,updated_at) VALUES('app','{"offline":true}',?)`, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	settings, err = db.AppSettings(context.Background())
	if err != nil || !settings.Offline || !settings.OpenBrowser {
		t.Fatalf("legacy settings=%#v err=%v", settings, err)
	}
}

func TestRecentScansJoinsWorkspaceAndClampsLimit(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	work, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: "Sample", RootPath: "C:/sample"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxRecentScansLimit+5; i++ {
		if _, err := db.CreateScan(context.Background(), core.Scan{WorkspaceID: work.ID, State: "completed"}); err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := db.RecentScans(context.Background(), RecentScansFilter{Limit: MaxRecentScansLimit + 100})
	if err != nil {
		t.Fatal(err)
	}
	if total != MaxRecentScansLimit+5 || len(items) != MaxRecentScansLimit {
		t.Fatalf("limit must clamp to %d: total=%d items=%d", MaxRecentScansLimit, total, len(items))
	}
	for _, item := range items {
		if item.WorkspaceID != work.ID || item.WorkspaceName != "Sample" {
			t.Fatalf("workspace join is wrong: %#v", item)
		}
	}
	items, total, err = db.RecentScans(context.Background(), RecentScansFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if total != MaxRecentScansLimit+5 || len(items) != DefaultRecentScansLimit {
		t.Fatalf("default limit must apply: total=%d items=%d", total, len(items))
	}
	items, total, err = db.RecentScans(context.Background(), RecentScansFilter{State: "running"})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("state filter: items=%#v total=%d err=%v", items, total, err)
	}
	summary, err := db.ScanSummary(context.Background())
	if err != nil || summary.WorkspacesTotal != 1 || summary.WorkspacesScanned != 1 || summary.ScansTotal != MaxRecentScansLimit+5 || summary.ActiveScans != 0 {
		t.Fatalf("summary=%#v err=%v", summary, err)
	}
}

func TestFindingsCSVMatchesPageFiltersAndIgnoresPagination(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	work, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Sample", RootPath: "C:/sample"})
	if err != nil {
		t.Fatal(err)
	}
	makeFinding := func(rule, path, message string, severity analyzers.Severity) analyzers.Finding {
		f := analyzers.Finding{AnalyzerID: "ruff", RuleID: rule, RelativePath: path, Message: message, Severity: severity, Category: analyzers.CategoryCorrectness}
		f.SetFingerprint()
		return f
	}
	persist := makeFinding("PERSIST", "src/persist.py", "same issue", analyzers.SeverityHigh)
	previous, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveAnalyzerResult(ctx, previous.ID, AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{persist}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteScan(ctx, previous.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	current, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	items := []analyzers.Finding{persist,
		makeFinding("H2", "src/h2.py", "high two", analyzers.SeverityHigh),
		makeFinding("M1", "src/m1.py", "medium one", analyzers.SeverityMedium),
		makeFinding("M2", "src/m2.py", "medium two", analyzers.SeverityMedium)}
	if _, err := db.SaveAnalyzerResult(ctx, current.ID, AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, items, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteScan(ctx, current.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}

	page, err := db.FindingsPage(ctx, current, FindingFilter{Limit: 25})
	if err != nil || page.Total != 4 {
		t.Fatalf("page must agree on the matching set: %#v (%v)", page, err)
	}
	all, err := db.FindingsCSV(ctx, current, FindingFilter{})
	if err != nil || len(all) != 4 {
		t.Fatalf("export must return every matching row: %#v (%v)", all, err)
	}
	// Shared ordering: default sort is relative_path ascending.
	if all[0].RuleID != "H2" || all[3].RuleID != "PERSIST" || all[3].Status != "persistent" || all[0].Status != "new" {
		t.Fatalf("export must reuse the page ordering and comparison status: %#v", all)
	}
	high, err := db.FindingsCSV(ctx, current, FindingFilter{Severity: "high"})
	if err != nil || len(high) != 2 {
		t.Fatalf("severity filter: %#v (%v)", high, err)
	}
	unpaged, err := db.FindingsCSV(ctx, current, FindingFilter{Severity: "high", Limit: 1, Offset: 1})
	if err != nil || len(unpaged) != 2 {
		t.Fatalf("limit and offset must be ignored by the export: %#v (%v)", unpaged, err)
	}
	persistent, err := db.FindingsCSV(ctx, current, FindingFilter{Status: "persistent"})
	if err != nil || len(persistent) != 1 || persistent[0].RuleID != "PERSIST" {
		t.Fatalf("status filter: %#v (%v)", persistent, err)
	}
	if _, err := db.FindingsCSV(ctx, current, FindingFilter{Status: "bogus"}); err == nil {
		t.Fatal("invalid status must be rejected")
	}
}

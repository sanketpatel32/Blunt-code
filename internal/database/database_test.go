package database

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

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

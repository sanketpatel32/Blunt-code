package database

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"bluntcode/internal/core"
)

func workspacesPageFixture(t *testing.T) (*DB, map[string]string) {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ids := map[string]string{}
	for _, name := range []string{"alpha-service", "beta-web", "gamma-tools", "delta-fresh"} {
		work, err := db.CreateWorkspace(ctx, core.Workspace{Name: name, RootPath: filepath.Join(t.TempDir(), name)})
		if err != nil {
			t.Fatal(err)
		}
		ids[name] = work.ID
	}
	return db, ids
}

func pageNames(t *testing.T, db *DB, filter WorkspacePageFilter) []string {
	t.Helper()
	items, total, err := db.WorkspacesPage(context.Background(), 1, 25, filter)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, item := range items {
		names = append(names, item.Name)
	}
	if total != len(names) {
		t.Fatalf("total = %d, items = %v", total, names)
	}
	return names
}

func TestWorkspacesPageSearchMatchesNamePathAndLanguages(t *testing.T) {
	db, ids := workspacesPageFixture(t)
	// alpha: Go, newest finished scan. beta: TypeScript finished, then a running
	// re-scan whose snapshot says Rust — the view keeps the finished languages.
	seedPerfScan(t, db, ids["alpha-service"], "scan-alpha", "completed", 1, [5]int{}, `{"languages":{"go":12,"python":0}}`)
	seedPerfScan(t, db, ids["beta-web"], "scan-beta-1", "completed", 2, [5]int{}, `{"languages":{"typescript":30}}`)
	seedPerfScan(t, db, ids["beta-web"], "scan-beta-2", "running", 3, [5]int{}, `{"languages":{"rust":3}}`)
	seedPerfScan(t, db, ids["gamma-tools"], "scan-gamma", "completed", 4, [5]int{}, `not json`)

	cases := []struct {
		filter WorkspacePageFilter
		want   []string
	}{
		{WorkspacePageFilter{Query: "ALPHA", Sort: "name", Order: "asc"}, []string{"alpha-service"}},
		{WorkspacePageFilter{Query: "delta-fresh", Sort: "name", Order: "asc"}, []string{"delta-fresh"}},
		{WorkspacePageFilter{Query: "go", Sort: "name", Order: "asc"}, []string{"alpha-service"}},
		{WorkspacePageFilter{Query: "Script", Sort: "name", Order: "asc"}, []string{"beta-web"}},
		{WorkspacePageFilter{Query: "rust", Sort: "name", Order: "asc"}, []string{}},
		{WorkspacePageFilter{Query: "python", Sort: "name", Order: "asc"}, []string{}},
		// Discovered languages apply only to workspaces without snapshot languages.
		{WorkspacePageFilter{Query: "rust", QueryLanguageIDs: []string{ids["delta-fresh"], ids["alpha-service"], ids["gamma-tools"]}, Sort: "name", Order: "asc"}, []string{"delta-fresh", "gamma-tools"}},
	}
	for _, tc := range cases {
		if got := pageNames(t, db, tc.filter); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("query %q ids %v: got %v, want %v", tc.filter.Query, tc.filter.QueryLanguageIDs, got, tc.want)
		}
	}
}

func TestWorkspacesPageLastScanSortUsesScanTimeNotLastOpened(t *testing.T) {
	db, ids := workspacesPageFixture(t)
	ctx := context.Background()
	seedPerfScan(t, db, ids["alpha-service"], "scan-alpha", "completed", 1, [5]int{}, "")
	seedPerfScan(t, db, ids["beta-web"], "scan-beta", "completed", 5, [5]int{}, "")
	seedPerfScan(t, db, ids["gamma-tools"], "scan-gamma", "running", 3, [5]int{}, "")
	// Opening the oldest-scanned and never-scanned workspaces must not lift them.
	opened := perfBase.Add(48 * time.Hour).UTC().Format(time.RFC3339Nano)
	for _, name := range []string{"alpha-service", "delta-fresh"} {
		if _, err := db.SQL.ExecContext(ctx, `UPDATE workspaces SET last_opened_at=? WHERE id=?`, opened, ids[name]); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := pageNames(t, db, WorkspacePageFilter{Sort: "last_scan", Order: "desc"}), []string{"beta-web", "gamma-tools", "alpha-service", "delta-fresh"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("desc = %v, want %v", got, want)
	}
	if got, want := pageNames(t, db, WorkspacePageFilter{Sort: "last_scan", Order: "asc"}), []string{"alpha-service", "gamma-tools", "beta-web", "delta-fresh"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("asc = %v, want %v", got, want)
	}
}

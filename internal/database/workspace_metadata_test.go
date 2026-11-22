package database

import (
	"context"
	"path/filepath"
	"testing"

	"bluntcode/internal/core"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// git_repository was a dead field — declared, serialized, never populated.
// The flag must round-trip through create and refresh through the discover-
// time setter.
func TestGitRepositoryFlagPersistsAndRefreshes(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	created, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Repo", RootPath: "C:/repo", GitRepository: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.Workspace(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.GitRepository {
		t.Fatal("created workspace lost its git_repository flag")
	}
	// Discover refreshes the flag both ways: a folder that gains .git flips
	// to true, one that loses it flips back.
	if err := db.SetWorkspaceGitRepository(ctx, created.ID, false); err != nil {
		t.Fatal(err)
	}
	got, _ = db.Workspace(ctx, created.ID)
	if got.GitRepository {
		t.Fatal("SetWorkspaceGitRepository(false) did not clear the flag")
	}
	if err := db.SetWorkspaceGitRepository(ctx, created.ID, true); err != nil {
		t.Fatal(err)
	}
	got, _ = db.Workspace(ctx, created.ID)
	if !got.GitRepository {
		t.Fatal("SetWorkspaceGitRepository(true) did not set the flag")
	}
	// Rows pre-dating the migration default to false, not NULL garbage.
	legacy, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Legacy", RootPath: "C:/legacy"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ = db.Workspace(ctx, legacy.ID)
	if got.GitRepository {
		t.Fatal("workspace without an explicit flag must default to false")
	}
}

// last_scan_at was a dead column — read by the workspace list and its sort,
// never written. Every terminal scan outcome stamps it.
func TestTerminalScanStatesStampLastScanAt(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	work, err := db.CreateWorkspace(ctx, core.Workspace{Name: "W", RootPath: "C:/w"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := db.Workspace(ctx, work.ID)
	if got.LastScanAt != nil {
		t.Fatal("fresh workspace must have no last_scan_at")
	}
	scan, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "running"})
	if err != nil {
		t.Fatal(err)
	}
	// A non-terminal transition must NOT stamp — the scan is still going.
	if err := db.UpdateScanState(ctx, scan.ID, "discovering", ""); err != nil {
		t.Fatal(err)
	}
	got, _ = db.Workspace(ctx, work.ID)
	if got.LastScanAt != nil {
		t.Fatal("non-terminal state stamped last_scan_at")
	}
	for _, state := range []string{"completed", "completed_with_warnings", "failed", "cancelled"} {
		if err := db.UpdateScanState(ctx, scan.ID, "running", ""); err != nil {
			t.Fatal(err)
		}
		if err := db.UpdateScanState(ctx, scan.ID, state, ""); err != nil {
			t.Fatal(err)
		}
		got, _ = db.Workspace(ctx, work.ID)
		if got.LastScanAt == nil {
			t.Fatalf("state %q did not stamp last_scan_at", state)
		}
	}
}

func TestCompleteScanStampsLastScanAt(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	work, err := db.CreateWorkspace(ctx, core.Workspace{Name: "W", RootPath: "C:/w"})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "running"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteScan(ctx, scan.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := db.Workspace(ctx, work.ID)
	if got.LastScanAt == nil {
		t.Fatal("CompleteScan did not stamp last_scan_at")
	}
}

func TestMarkInterruptedScansStampsWorkspace(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	work, err := db.CreateWorkspace(ctx, core.Workspace{Name: "W", RootPath: "C:/w"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkInterruptedScans(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := db.Workspace(ctx, work.ID)
	if got.LastScanAt == nil {
		t.Fatal("interrupted scans must stamp their workspace's last_scan_at")
	}
}

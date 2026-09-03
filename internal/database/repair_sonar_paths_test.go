package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
)

func insertRepairScan(t *testing.T, db *DB, workspaceID, root string) string {
	t.Helper()
	snapshot := &core.ScanSnapshot{WorkspaceID: workspaceID, WorkspaceRoot: root, Profile: "standard"}
	scan, err := db.CreateScanWithFiles(context.Background(), core.Scan{WorkspaceID: workspaceID, State: "completed", Profile: "standard", Snapshot: snapshot}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`INSERT INTO analyzer_runs(id,scan_id,analyzer_id,version,state,started_at) VALUES(?,?,?,?,?,?)`,
		"run-"+scan.ID+"-sonarqube", scan.ID, "sonarqube", "test", "succeeded", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	return scan.ID
}

func insertRepairFinding(t *testing.T, db *DB, scanID, id, path, rule, message string) string {
	t.Helper()
	f := analyzers.Finding{AnalyzerID: "sonarqube", RuleID: rule, Message: message, RelativePath: path}
	f.SetFingerprint()
	_, err := db.SQL.Exec(`INSERT INTO findings(id,scan_id,analyzer_run_id,analyzer_id,rule_id,fingerprint,severity,category,title,message,relative_path) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		id, scanID, "run-"+scanID+"-sonarqube", "sonarqube", rule, f.Fingerprint, "high", "maintainability", rule, message, path)
	if err != nil {
		t.Fatal(err)
	}
	return f.Fingerprint
}

func TestRepairSonarqubeFindingPaths(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	work, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: "Sample", RootPath: "C:/sample"})
	if err != nil {
		t.Fatal(err)
	}
	scanID := insertRepairScan(t, db, work.ID, work.RootPath)

	const uuid = "b01a778f-99c6-95c1-0006-f8d4a461e5e6"
	corruptFP := insertRepairFinding(t, db, scanID, "f-1", uuid+":src/pages/Home.tsx", "typescript:S2699", "Add at least one assertion to this test case.")
	cleanFP := insertRepairFinding(t, db, scanID, "f-2", "src/utils/legacy.go", "go:S3776", "Refactor this function.")
	otherFP := insertRepairFinding(t, db, scanID, "f-3", "b01a778f-not-a-uuid:1/x.ts", "ts:S1077", "Not the corruption shape.")
	// A suppression recorded against the corrupt fingerprint must survive.
	if _, err := db.AddSuppression(context.Background(), work.ID, corruptFP, "False positive"); err != nil {
		t.Fatal(err)
	}

	repaired, err := db.RepairSonarqubeFindingPaths(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if repaired != 1 {
		t.Fatalf("repaired %d findings, want 1", repaired)
	}

	var path, fingerprint string
	if err := db.SQL.QueryRow(`SELECT relative_path, fingerprint FROM findings WHERE id = 'f-1'`).Scan(&path, &fingerprint); err != nil {
		t.Fatal(err)
	}
	if path != "src/pages/Home.tsx" {
		t.Fatalf("repaired path = %q", path)
	}
	fixed := analyzers.Finding{AnalyzerID: "sonarqube", RuleID: "typescript:S2699", Message: "Add at least one assertion to this test case.", RelativePath: "src/pages/Home.tsx"}
	fixed.SetFingerprint()
	if fingerprint != fixed.Fingerprint {
		t.Fatalf("repaired fingerprint = %q, want recomputed %q", fingerprint, fixed.Fingerprint)
	}
	suppressed, err := db.SuppressedFingerprints(context.Background(), work.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !suppressed[fixed.Fingerprint] || suppressed[corruptFP] {
		t.Fatalf("suppression not carried to the repaired fingerprint: %v", suppressed)
	}

	// Untouched rows keep their identity.
	if err := db.SQL.QueryRow(`SELECT fingerprint FROM findings WHERE id = 'f-2'`).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	if fingerprint != cleanFP {
		t.Fatalf("clean finding fingerprint changed: %q", fingerprint)
	}
	if err := db.SQL.QueryRow(`SELECT fingerprint FROM findings WHERE id = 'f-3'`).Scan(&fingerprint); err != nil {
		t.Fatal(err)
	}
	if fingerprint != otherFP {
		t.Fatalf("non-matching shape fingerprint changed: %q", fingerprint)
	}

	// Idempotent: a second pass repairs nothing.
	if repaired, err = db.RepairSonarqubeFindingPaths(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repaired != 0 {
		t.Fatalf("second pass repaired %d findings, want 0", repaired)
	}
}

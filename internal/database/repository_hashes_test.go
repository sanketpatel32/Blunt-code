package database

// Tests for the incremental-rescan storage: per-scan file hashes with the
// analyzer identity that produced them, appending reused findings to an
// existing succeeded run, and scan notes.

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
)

func hashFixture(t *testing.T) (*DB, core.Workspace, core.Scan) {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	work, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Hashes", RootPath: "C:/hashes"})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	return db, work, scan
}

func TestScanFileHashesRoundTrip(t *testing.T) {
	db, _, scan := hashFixture(t)
	ctx := context.Background()
	identity := `{"bluntcode_version":"0.3.0","profile":"standard","analyzers":{"fake":"v1"}}`
	hashes := map[string]string{"main.py": "aa", "src/util.py": "bb"}
	if err := db.SaveScanFileHashes(ctx, scan.ID, hashes, identity); err != nil {
		t.Fatal(err)
	}
	loaded, err := db.ScanFileHashes(ctx, scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || loaded["main.py"] != "aa" || loaded["src/util.py"] != "bb" {
		t.Fatalf("loaded hashes = %v, want the saved map", loaded)
	}
	stored, err := db.ScanHashAnalyzerSet(ctx, scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored != identity {
		t.Fatalf("stored identity = %s, want %s", stored, identity)
	}
	// Rewriting replaces instead of colliding on the primary keys.
	if err := db.SaveScanFileHashes(ctx, scan.ID, map[string]string{"main.py": "cc"}, identity); err != nil {
		t.Fatal(err)
	}
	reloaded, err := db.ScanFileHashes(ctx, scan.ID)
	if err != nil || len(reloaded) != 1 || reloaded["main.py"] != "cc" {
		t.Fatalf("reloaded hashes = %v err=%v, want the replaced single row", reloaded, err)
	}
}

func TestScanHashAccessorMissingScan(t *testing.T) {
	db, _, _ := hashFixture(t)
	ctx := context.Background()
	if _, err := db.ScanHashAnalyzerSet(ctx, "no-such-scan"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("identity lookup error = %v, want sql.ErrNoRows", err)
	}
	hashes, err := db.ScanFileHashes(ctx, "no-such-scan")
	if err != nil || len(hashes) != 0 {
		t.Fatalf("hash lookup = %v err=%v, want an empty map without error", hashes, err)
	}
}

func TestAppendReusedFindingsAttachesToSucceededRun(t *testing.T) {
	db, _, scan := hashFixture(t)
	ctx := context.Background()
	fresh := func(rule string) analyzers.Finding {
		f := analyzers.Finding{AnalyzerID: "fake", RuleID: rule, Severity: analyzers.SeverityMedium, Category: analyzers.CategoryCorrectness, Message: "fresh issue", RelativePath: "main.py"}
		f.SetFingerprint()
		return f
	}
	freshFinding := fresh("FRESH")
	if _, err := db.SaveAnalyzerResult(ctx, scan.ID, AnalyzerRunInput{AnalyzerID: "fake", Version: "v1", State: "succeeded"}, []analyzers.Finding{freshFinding}, nil); err != nil {
		t.Fatal(err)
	}
	reusedA, reusedB := fresh("REUSED-A"), fresh("REUSED-B")
	if err := db.AppendReusedFindings(ctx, scan.ID, "fake", []analyzers.Finding{reusedA, reusedB}); err != nil {
		t.Fatal(err)
	}
	findings, err := db.Findings(ctx, scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 3 {
		t.Fatalf("findings = %d, want 3 (one fresh plus two reused)", len(findings))
	}
	fingerprints := map[string]bool{}
	for _, finding := range findings {
		fingerprints[finding.Fingerprint] = true
	}
	if !fingerprints[reusedA.Fingerprint] || !fingerprints[reusedB.Fingerprint] {
		t.Fatal("reused findings must keep their fingerprints")
	}
	runs, err := db.AnalyzerRuns(ctx, scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].FindingCount != 3 {
		t.Fatalf("runs = %#v, want the single run's count to include the reused findings", runs)
	}
}

func TestAppendReusedFindingsRequiresSucceededRun(t *testing.T) {
	db, _, scan := hashFixture(t)
	ctx := context.Background()
	if _, err := db.SaveAnalyzerResult(ctx, scan.ID, AnalyzerRunInput{AnalyzerID: "fake", State: "failed", Error: "boom"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	f := analyzers.Finding{AnalyzerID: "fake", RuleID: "X1", Severity: analyzers.SeverityLow, Category: analyzers.CategoryStyle, Message: "issue", RelativePath: "main.py"}
	f.SetFingerprint()
	if err := db.AppendReusedFindings(ctx, scan.ID, "fake", []analyzers.Finding{f}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("append error = %v, want sql.ErrNoRows without a succeeded run", err)
	}
}

func TestSetScanNoteSurfacesInErrorSummary(t *testing.T) {
	db, _, scan := hashFixture(t)
	ctx := context.Background()
	if err := db.CompleteScan(ctx, scan.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	note := "incremental: reused findings for 3 unchanged file(s), ran analyzers on 1 file(s)"
	if err := db.SetScanNote(ctx, scan.ID, note); err != nil {
		t.Fatal(err)
	}
	current, err := db.Scan(ctx, scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.ErrorSummary != note {
		t.Fatalf("scan note = %q, want %q", current.ErrorSummary, note)
	}
	if current.State != "completed" || current.TotalFindings != 0 {
		t.Fatalf("note write must not disturb the completed scan: state=%q total=%d", current.State, current.TotalFindings)
	}
}

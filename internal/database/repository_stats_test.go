package database

import (
	"context"
	"encoding/json"
	"maps"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
)

// statsScan seeds one scan through the real write path (SaveAnalyzerResult +
// CompleteScan) so the persisted severity split matches production rows, then
// forces started_at/finished_at to a fixed instant so recency assertions never
// depend on wall-clock resolution between inserts. state must be terminal.
func statsScan(t *testing.T, db *DB, workspaceID, state string, when time.Time, findings ...analyzers.Finding) core.Scan {
	t.Helper()
	ctx := context.Background()
	scan, err := db.CreateScan(ctx, core.Scan{WorkspaceID: workspaceID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveAnalyzerResult(ctx, scan.ID, AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, findings, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteScan(ctx, scan.ID, state, ""); err != nil {
		t.Fatal(err)
	}
	stamp := when.UTC().Format(time.RFC3339Nano)
	if _, err := db.SQL.ExecContext(ctx, `UPDATE scans SET started_at=?, finished_at=? WHERE id=?`, stamp, stamp, scan.ID); err != nil {
		t.Fatal(err)
	}
	return scan
}

func statsFinding(rule string, severity analyzers.Severity) analyzers.Finding {
	f := analyzers.Finding{AnalyzerID: "ruff", RuleID: rule, RelativePath: "src/" + rule + ".py", Message: rule, Severity: severity, Category: analyzers.CategoryCorrectness}
	f.SetFingerprint()
	return f
}

// TestGlobalStatsRollsUpLatestCompletedScanPerWorkspace pins the overview's
// semantics against the dashboard summary's: only each workspace's newest
// producing scan feeds the severity rollup (failed and in-flight scans never
// do, though they still count in the scan totals), suppressions are a global
// row count, and workspaces without any scan still count as workspaces.
func TestGlobalStatsRollsUpLatestCompletedScanPerWorkspace(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	base := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)

	alpha, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Alpha", RootPath: "C:/alpha"})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Beta", RootPath: "C:/beta"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Gamma", RootPath: "C:/gamma"}); err != nil {
		t.Fatal(err)
	}

	// Alpha's history: only the newest completed scan may feed the rollup.
	statsScan(t, db, alpha.ID, "completed", base,
		statsFinding("OLD-C", analyzers.SeverityCritical), statsFinding("OLD-L", analyzers.SeverityLow))
	statsScan(t, db, alpha.ID, "completed_with_warnings", base.Add(time.Hour),
		statsFinding("MID-H", analyzers.SeverityHigh), statsFinding("MID-H2", analyzers.SeverityHigh))
	statsScan(t, db, alpha.ID, "completed", base.Add(2*time.Hour),
		statsFinding("NEW-C", analyzers.SeverityCritical), statsFinding("NEW-C2", analyzers.SeverityCritical), statsFinding("NEW-M", analyzers.SeverityMedium))
	// Beta never produced a completed result: the failed scan keeps its counts
	// but must stay out of the rollup, and the running scan is in-flight.
	statsScan(t, db, beta.ID, "failed", base.Add(3*time.Hour),
		statsFinding("FAIL-H", analyzers.SeverityHigh), statsFinding("FAIL-H2", analyzers.SeverityHigh))
	running, err := db.CreateScan(ctx, core.Scan{WorkspaceID: beta.ID, State: "running"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.ExecContext(ctx, `UPDATE scans SET started_at=? WHERE id=?`, base.Add(4*time.Hour).UTC().Format(time.RFC3339Nano), running.ID); err != nil {
		t.Fatal(err)
	}

	// Suppressions are counted across workspaces regardless of scan state;
	// they are added after completion so the seeded severity splits stay exact.
	// Fingerprints are distinct 64-character hex strings, the shape production
	// writes, so every AddSuppression inserts a row rather than upserting.
	for i, work := range []core.Workspace{alpha, beta, alpha} {
		fingerprint := strings.Repeat("a", 63) + string(rune('0'+i))
		if _, err := db.AddSuppression(ctx, work.ID, fingerprint, "wontfix"); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := db.GlobalStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Workspaces != 3 || stats.Suppressions != 3 {
		t.Fatalf("workspace and suppression counters: %#v", stats)
	}
	if stats.Scans.Total != 5 || stats.Scans.Completed != 3 || stats.Scans.Running != 1 {
		t.Fatalf("scan counters (total/completed/running): %#v", stats.Scans)
	}
	wantSeverity := SeverityCounts{Critical: 2, Medium: 1}
	if stats.Findings.Severity != wantSeverity || stats.Findings.Total != 3 {
		t.Fatalf("findings must come from Alpha's newest completed scan only: %#v", stats.Findings)
	}
	// Risk grades bucket each workspace's latest completed scan with the risk
	// endpoint's weights: only Alpha has a completed scan, and its newest one
	// carries 2 critical + 1 medium = 2*10 + 1*2 = 22 points -> grade C.
	// Beta's failed scan and Gamma's missing history never grade.
	wantGrades := map[string]int{"A": 0, "B": 0, "C": 1, "D": 0}
	if !maps.Equal(stats.RiskGrades, wantGrades) {
		t.Fatalf("risk grades = %#v, want %#v", stats.RiskGrades, wantGrades)
	}

	// The rollup must stay in lockstep with the dashboard summary's severity
	// totals; the two endpoints share one notion of "current" findings.
	summary, err := db.ScanSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary.CriticalCount != stats.Findings.Severity.Critical || summary.HighCount != stats.Findings.Severity.High ||
		summary.MediumCount != stats.Findings.Severity.Medium || summary.LowCount != stats.Findings.Severity.Low ||
		summary.InfoCount != stats.Findings.Severity.Info || summary.TotalFindings != stats.Findings.Total {
		t.Fatalf("GlobalStats %#v diverged from ScanSummary severity totals %#v", stats.Findings, summary)
	}
}

// TestGlobalStatsOnEmptyDatabaseReturnsZeroValue pins the empty-database
// contract: every counter is the zero value of one fixed struct, never an
// error or a partially-populated payload, and the risk-grade map serializes
// as a fully-initialized object of zeros — never null — so consumers can
// always index A/B/C/D.
func TestGlobalStatsOnEmptyDatabaseReturnsZeroValue(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stats, err := db.GlobalStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Workspaces != 0 || stats.Scans != (GlobalScans{}) || stats.Findings != (GlobalFindings{}) || stats.Suppressions != 0 {
		t.Fatalf("empty database counters must be zero, got %#v", stats)
	}
	if !maps.Equal(stats.RiskGrades, map[string]int{"A": 0, "B": 0, "C": 0, "D": 0}) {
		t.Fatalf("risk grades must initialize all four keys to zero, got %#v", stats.RiskGrades)
	}
	serialized, err := json.Marshal(stats.RiskGrades)
	if err != nil {
		t.Fatal(err)
	}
	if string(serialized) != `{"A":0,"B":0,"C":0,"D":0}` {
		t.Fatalf("risk grades must serialize as an object of zeros, got %s", serialized)
	}
}

// TestStatsRiskGradeBoundaries pins the grade buckets against the weighted
// score: A<5, B<20, C<50, D at 50 or beyond, using critical=10, high=5,
// medium=2, low=1.
func TestStatsRiskGradeBoundaries(t *testing.T) {
	cases := []struct {
		name                        string
		critical, high, medium, low int
		want                        string
	}{
		{"no findings is a clean A", 0, 0, 0, 0, "A"},
		{"a single medium stays A", 0, 0, 1, 0, "A"},
		{"one high crosses into B", 0, 1, 0, 0, "B"},
		{"one critical is B", 1, 0, 0, 0, "B"},
		{"four highs land exactly on C's boundary", 0, 4, 0, 0, "C"},
		{"two criticals plus a medium match the rollup fixture", 2, 0, 1, 0, "C"},
		{"five criticals reach D", 5, 0, 0, 0, "D"},
		{"lows accumulate too", 0, 0, 2, 1, "B"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := statsRiskGrade(item.critical, item.high, item.medium, item.low); got != item.want {
				t.Fatalf("statsRiskGrade(%d,%d,%d,%d) = %q, want %q", item.critical, item.high, item.medium, item.low, got, item.want)
			}
		})
	}
}

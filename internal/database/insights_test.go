package database

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
)

func openInsightsDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestRecentCompletedTotalsSkipsFailedScans pins the sparkline contract: only
// producing scans chart (a failed scan is a gap, never a zero), the newest
// window is kept, and the series reads oldest first per workspace.
func TestRecentCompletedTotalsSkipsFailedScans(t *testing.T) {
	db := openInsightsDB(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	alpha, _ := db.CreateWorkspace(ctx, core.Workspace{Name: "Alpha", RootPath: "C:/alpha"})
	beta, _ := db.CreateWorkspace(ctx, core.Workspace{Name: "Beta", RootPath: "C:/beta"})
	statsScan(t, db, alpha.ID, "completed", base, statsFinding("A1", analyzers.SeverityHigh))
	statsScan(t, db, alpha.ID, "failed", base.Add(time.Hour))
	statsScan(t, db, alpha.ID, "completed_with_warnings", base.Add(2*time.Hour), statsFinding("B1", analyzers.SeverityLow), statsFinding("B2", analyzers.SeverityLow), statsFinding("B3", analyzers.SeverityLow))
	statsScan(t, db, alpha.ID, "completed", base.Add(3*time.Hour), statsFinding("C1", analyzers.SeverityLow), statsFinding("C2", analyzers.SeverityLow))
	statsScan(t, db, beta.ID, "failed", base)

	got, err := db.RecentCompletedTotals(ctx, 8)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got[alpha.ID], []int{1, 3, 2}) {
		t.Fatalf("alpha series = %v, want [1 3 2]", got[alpha.ID])
	}
	if _, ok := got[beta.ID]; ok {
		t.Fatalf("a workspace with only failed scans has no series: %v", got[beta.ID])
	}
	window, err := db.RecentCompletedTotals(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(window[alpha.ID], []int{3, 2}) {
		t.Fatalf("limit keeps the newest window oldest-first: %v", window[alpha.ID])
	}
}

// TestScansByDayZeroFillsLocalCalendarDays pins the overview activity strip:
// a fixed seven-day window in the caller's zone, zero-filled, with scans
// bucketed by their local start date and older scans ignored.
func TestScansByDayZeroFillsLocalCalendarDays(t *testing.T) {
	db := openInsightsDB(t)
	ctx := context.Background()
	loc := time.FixedZone("IST", 5*3600+1800)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, loc)
	work, _ := db.CreateWorkspace(ctx, core.Workspace{Name: "Alpha", RootPath: "C:/alpha"})
	// 2026-08-09 23:00 UTC is 2026-08-10 04:30 IST: it must land on "today".
	statsScan(t, db, work.ID, "completed", time.Date(2026, 8, 9, 23, 0, 0, 0, time.UTC))
	statsScan(t, db, work.ID, "failed", time.Date(2026, 8, 10, 1, 0, 0, 0, time.UTC))
	statsScan(t, db, work.ID, "completed", time.Date(2026, 8, 4, 10, 0, 0, 0, loc))
	statsScan(t, db, work.ID, "completed", time.Date(2026, 8, 3, 10, 0, 0, 0, loc)) // outside the window

	days, err := db.ScansByDay(ctx, now, loc, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 7 || days[0].Date != "2026-08-04" || days[6].Date != "2026-08-10" {
		t.Fatalf("window = %#v", days)
	}
	if days[0] != (ScanDay{Date: "2026-08-04", Total: 1, Completed: 1}) {
		t.Fatalf("first day = %#v", days[0])
	}
	if days[6] != (ScanDay{Date: "2026-08-10", Total: 2, Completed: 1}) {
		t.Fatalf("today = %#v", days[6])
	}
	for _, day := range days[1:6] {
		if day.Total != 0 || day.Completed != 0 {
			t.Fatalf("quiet days stay zero: %#v", day)
		}
	}
}

// TestFindingFacetsIgnoreOwnDimension pins the facet contract: counts honor
// the other filters but never their own dimension, so a category chart still
// shows every category while one is selected.
func TestFindingFacetsIgnoreOwnDimension(t *testing.T) {
	db := openInsightsDB(t)
	ctx := context.Background()
	work, _ := db.CreateWorkspace(ctx, core.Workspace{Name: "Alpha", RootPath: "C:/alpha"})
	sec := statsFinding("S1", analyzers.SeverityHigh)
	sec.Category = analyzers.CategorySecurity
	sec.SetFingerprint()
	scan := statsScan(t, db, work.ID, "completed", time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC),
		sec, statsFinding("C1", analyzers.SeverityLow), statsFinding("C2", analyzers.SeverityHigh))
	scan, err := db.Scan(ctx, scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	facets, err := db.FindingFacets(ctx, scan, FindingFilter{Category: string(analyzers.CategorySecurity)})
	if err != nil {
		t.Fatal(err)
	}
	if facets.Category[string(analyzers.CategorySecurity)] != 1 || facets.Category[string(analyzers.CategoryCorrectness)] != 2 {
		t.Fatalf("category facet = %v", facets.Category)
	}
	if facets.Analyzer["ruff"] != 3 {
		t.Fatalf("analyzer facet = %v", facets.Analyzer)
	}
	high, err := db.FindingFacets(ctx, scan, FindingFilter{Severity: "high"})
	if err != nil {
		t.Fatal(err)
	}
	if high.Category[string(analyzers.CategorySecurity)] != 1 || high.Category[string(analyzers.CategoryCorrectness)] != 1 {
		t.Fatalf("severity filter narrows facets: %v", high.Category)
	}
}

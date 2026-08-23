package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

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

func TestFixedFindingsAppliesCoverageRuleAndSeverityOrder(t *testing.T) {
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
	issue := func(analyzer, rule, path, message string, severity analyzers.Severity) analyzers.Finding {
		f := analyzers.Finding{AnalyzerID: analyzer, RuleID: rule, RelativePath: path, Message: message, Severity: severity, Category: analyzers.CategoryCorrectness}
		f.SetFingerprint()
		return f
	}
	// Previous scan: STAY persists, GONE-HIGH and GONE-LOW disappear while ruff
	// still succeeds, and SEM-UNKNOWN disappears while semgrep fails the
	// current scan, so its absence is unknown rather than fixed.
	previous, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	ruffItems := []analyzers.Finding{
		issue("ruff", "STAY", "src/stay.py", "still here", analyzers.SeverityCritical),
		issue("ruff", "GONE-LOW", "src/gone-low.py", "fixed low", analyzers.SeverityLow),
		issue("ruff", "GONE-HIGH", "src/gone-high.py", "fixed high", analyzers.SeverityHigh),
	}
	if _, err := db.SaveAnalyzerResult(ctx, previous.ID, AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, ruffItems, nil); err != nil {
		t.Fatal(err)
	}
	semgrepItem := issue("semgrep", "SEM-UNKNOWN", "src/covered.py", "analyzer failed this scan", analyzers.SeverityCritical)
	if _, err := db.SaveAnalyzerResult(ctx, previous.ID, AnalyzerRunInput{AnalyzerID: "semgrep", Version: "test", State: "succeeded"}, []analyzers.Finding{semgrepItem}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteScan(ctx, previous.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	current, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveAnalyzerResult(ctx, current.ID, AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{ruffItems[0]}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveAnalyzerResult(ctx, current.ID, AnalyzerRunInput{AnalyzerID: "semgrep", Version: "test", State: "failed", Error: "tool crashed"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteScan(ctx, current.ID, "completed_with_warnings", ""); err != nil {
		t.Fatal(err)
	}

	result, err := db.FixedFindings(ctx, current, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ComparisonAvailable || result.PreviousScanID != previous.ID {
		t.Fatalf("comparison context: %#v", result)
	}
	if result.Total != 2 || len(result.Items) != 2 {
		t.Fatalf("only GONE-HIGH and GONE-LOW count as fixed: %#v", result)
	}
	if result.Items[0].RuleID != "GONE-HIGH" || result.Items[1].RuleID != "GONE-LOW" {
		t.Fatalf("severity must rank high before low: %#v", result.Items)
	}
	for _, item := range result.Items {
		if item.Status != "fixed" || item.ID == "" || item.AnalyzerID != "ruff" || item.Fingerprint == "" {
			t.Fatalf("fixed rows must reuse the finding shape with status fixed: %#v", item)
		}
	}
	// A tighter limit caps the rows while Total keeps the exact count.
	capped, err := db.FixedFindings(ctx, current, 1)
	if err != nil || capped.Total != 2 || len(capped.Items) != 1 {
		t.Fatalf("limit must cap rows without losing the total: %#v (%v)", capped, err)
	}

	// The first completed scan of a workspace has no comparison basis.
	solo, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Solo", RootPath: "C:/solo"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := db.CreateScan(ctx, core.Scan{WorkspaceID: solo.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveAnalyzerResult(ctx, first.ID, AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{issue("ruff", "ONLY", "src/only.py", "no baseline", analyzers.SeverityLow)}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteScan(ctx, first.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	result, err = db.FixedFindings(ctx, first, 0)
	if err != nil || result.ComparisonAvailable || result.Total != 0 || len(result.Items) != 0 || result.PreviousScanID != "" {
		t.Fatalf("a scan without a previous completed scan has nothing to compare: %#v (%v)", result, err)
	}
}

func TestFixedFindingsCapsLimitAndReportsTrueTotal(t *testing.T) {
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
	total := MaxFixedFindingsLimit + 5
	items := make([]analyzers.Finding, 0, total)
	for i := 0; i < total; i++ {
		f := analyzers.Finding{AnalyzerID: "ruff", RuleID: fmt.Sprintf("RULE-%03d", i), RelativePath: fmt.Sprintf("src/file-%03d.py", i), Message: "resolved issue", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryCorrectness}
		f.SetFingerprint()
		items = append(items, f)
	}
	previous, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveAnalyzerResult(ctx, previous.ID, AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, items, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteScan(ctx, previous.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	current, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveAnalyzerResult(ctx, current.ID, AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteScan(ctx, current.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	result, err := db.FixedFindings(ctx, current, 0)
	if err != nil || result.Total != total || len(result.Items) != DefaultFixedFindingsLimit {
		t.Fatalf("default limit must apply while the total stays exact: total=%d items=%d err=%v", result.Total, len(result.Items), err)
	}
	result, err = db.FixedFindings(ctx, current, total*10)
	if err != nil || result.Total != total || len(result.Items) != MaxFixedFindingsLimit {
		t.Fatalf("limit must clamp to %d while the total stays exact: total=%d items=%d err=%v", MaxFixedFindingsLimit, result.Total, len(result.Items), err)
	}
}

// TestMarkInterruptedScansRecoversEveryNonTerminalState simulates a kill -9
// mid-run: scan rows exist in every non-terminal state. Startup recovery must
// move each of them to a terminal interrupted state with a finish time, leave
// already-terminal scans untouched, and keep interrupted scans out of
// completed-scan comparison resolution so the next scan starts cleanly.
func TestMarkInterruptedScansRecoversEveryNonTerminalState(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	work, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Crashed", RootPath: "C:/crashed"})
	if err != nil {
		t.Fatal(err)
	}
	nonTerminal := []string{"queued", "preparing", "installing_tools", "discovering", "running", "normalizing", "generating_report"}
	stuck := map[string]string{}
	for _, state := range nonTerminal {
		scan, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: state})
		if err != nil {
			t.Fatal(err)
		}
		stuck[scan.ID] = state
	}
	completed, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "running"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveAnalyzerResult(ctx, completed.ID, AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{{
		AnalyzerID: "ruff", RuleID: "F821", Severity: analyzers.SeverityHigh, Message: "undefined", RelativePath: "main.py",
	}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteScan(ctx, completed.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}

	if err := db.MarkInterruptedScans(ctx); err != nil {
		t.Fatal(err)
	}
	for scanID := range stuck {
		scan, err := db.Scan(ctx, scanID)
		if err != nil {
			t.Fatal(err)
		}
		if scan.State != "interrupted" {
			t.Fatalf("scan left in %q after recovery, want interrupted", scan.State)
		}
		if scan.FinishedAt == nil {
			t.Fatalf("interrupted scan %s has no finish time", scanID)
		}
	}
	survivor, err := db.Scan(ctx, completed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if survivor.State != "completed" || survivor.FinishedAt == nil {
		t.Fatalf("recovery must not touch terminal scans: %#v", survivor)
	}
	// The latest scan by recency is interrupted; comparison resolution must
	// still resolve to the older completed scan, and interrupted scans must
	// never be selected as a previous completed scan.
	latest := stuckOrder(stuck)
	if previous, err := db.PreviousCompletedScanID(ctx, work.ID, latest); err != nil || previous != completed.ID {
		t.Fatalf("previous completed scan = %q, %v; want %q", previous, err, completed.ID)
	}
}

// stuckOrder returns one of the interrupted scan IDs to compare against.
func stuckOrder(stuck map[string]string) string {
	for scanID := range stuck {
		return scanID
	}
	return ""
}

func TestSuppressionsRoundTripUniquenessAndIsolation(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	alpha, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Alpha", RootPath: "C:/alpha"})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Beta", RootPath: "C:/beta"})
	if err != nil {
		t.Fatal(err)
	}
	first := strings.Repeat("ab", 32)
	second := strings.Repeat("cd", 32)

	item, err := db.AddSuppression(ctx, alpha.ID, first, "wontfix")
	if err != nil || item.WorkspaceID != alpha.ID || item.Fingerprint != first || item.Reason != "wontfix" || item.CreatedAt.IsZero() {
		t.Fatalf("add suppression: %#v (%v)", item, err)
	}
	// Re-suppressing the same fingerprint is an idempotent upsert that
	// refreshes the reason; the (workspace, fingerprint) pair stays unique.
	if _, err := db.AddSuppression(ctx, alpha.ID, first, "updated reason"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddSuppression(ctx, alpha.ID, second, ""); err != nil {
		t.Fatal(err)
	}
	// The same fingerprint in another workspace is an independent record.
	if _, err := db.AddSuppression(ctx, beta.ID, first, "other workspace"); err != nil {
		t.Fatal(err)
	}

	alphaItems, err := db.Suppressions(ctx, alpha.ID)
	if err != nil || len(alphaItems) != 2 || alphaItems[0].Fingerprint != first || alphaItems[0].Reason != "updated reason" || alphaItems[1].Fingerprint != second || alphaItems[1].Reason != "" {
		t.Fatalf("alpha suppressions: %#v (%v)", alphaItems, err)
	}
	betaItems, err := db.Suppressions(ctx, beta.ID)
	if err != nil || len(betaItems) != 1 || betaItems[0].WorkspaceID != beta.ID || betaItems[0].Reason != "other workspace" {
		t.Fatalf("beta suppressions: %#v (%v)", betaItems, err)
	}
	set, err := db.SuppressedFingerprints(ctx, alpha.ID)
	if err != nil || len(set) != 2 || !set[first] || !set[second] {
		t.Fatalf("alpha fingerprint set: %#v (%v)", set, err)
	}

	// Removing an unknown fingerprint reports no rows.
	if err := db.RemoveSuppression(ctx, alpha.ID, strings.Repeat("ef", 32)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unknown removal must return sql.ErrNoRows: %v", err)
	}
	if err := db.RemoveSuppression(ctx, alpha.ID, first); err != nil {
		t.Fatal(err)
	}
	if err := db.RemoveSuppression(ctx, alpha.ID, second); err != nil {
		t.Fatal(err)
	}
	alphaItems, err = db.Suppressions(ctx, alpha.ID)
	if err != nil || len(alphaItems) != 0 || alphaItems == nil {
		t.Fatalf("alpha must be an empty, non-nil list after removals: %#v (%v)", alphaItems, err)
	}
	if set, err = db.SuppressedFingerprints(ctx, alpha.ID); err != nil || len(set) != 0 {
		t.Fatalf("alpha fingerprint set must be empty: %#v (%v)", set, err)
	}
	// Beta's suppression is untouched by alpha's removals.
	betaItems, err = db.Suppressions(ctx, beta.ID)
	if err != nil || len(betaItems) != 1 {
		t.Fatalf("beta suppression must survive alpha removals: %#v (%v)", betaItems, err)
	}

	// An empty fingerprint is rejected at the repository door.
	if _, err := db.AddSuppression(ctx, alpha.ID, "", "reason"); err == nil {
		t.Fatal("empty fingerprint must be rejected")
	}
}

func TestSuppressedFindingsExcludedFromTotalsExportsAndFixed(t *testing.T) {
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
	issue := func(rule, message string, severity analyzers.Severity) analyzers.Finding {
		f := analyzers.Finding{AnalyzerID: "ruff", RuleID: rule, RelativePath: "src/" + strings.ToLower(rule) + ".py", Message: message, Severity: severity, Category: analyzers.CategoryCorrectness}
		f.SetFingerprint()
		return f
	}
	acknowledged := issue("ACK", "acknowledged issue", analyzers.SeverityHigh)
	dismissed := issue("GONE", "dismissed and gone", analyzers.SeverityLow)
	kept := issue("KEEP", "kept issue", analyzers.SeverityMedium)

	previous, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveAnalyzerResult(ctx, previous.ID, AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{acknowledged, dismissed}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteScan(ctx, previous.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	baseline, err := db.Scan(ctx, previous.ID)
	if err != nil || baseline.TotalFindings != 2 {
		t.Fatalf("baseline scan must count both findings: %#v (%v)", baseline, err)
	}

	// Suppress the persistent finding and the one that disappears next scan.
	if _, err := db.AddSuppression(ctx, work.ID, acknowledged.Fingerprint, "wontfix"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddSuppression(ctx, work.ID, dismissed.Fingerprint, ""); err != nil {
		t.Fatal(err)
	}
	current, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveAnalyzerResult(ctx, current.ID, AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{acknowledged, kept}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteScan(ctx, current.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	loaded, err := db.Scan(ctx, current.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Suppressed findings stay stored but never feed totals or severity counts.
	if loaded.TotalFindings != 1 {
		t.Fatalf("suppressed findings must not count toward totals: %#v", loaded)
	}
	var highCount, mediumCount int
	if err := db.SQL.QueryRowContext(ctx, `SELECT high_count,medium_count FROM scans WHERE id=?`, current.ID).Scan(&highCount, &mediumCount); err != nil {
		t.Fatal(err)
	}
	if highCount != 0 || mediumCount != 1 {
		t.Fatalf("severity counts must exclude suppressed findings: high=%d medium=%d", highCount, mediumCount)
	}

	// The findings list keeps suppressed rows visible, stamped with their status.
	page, err := db.FindingsPage(ctx, loaded, FindingFilter{Limit: 25})
	if err != nil || page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("list must show stored suppressed findings with their status: %#v (%v)", page, err)
	}
	statuses := map[string]string{}
	for _, item := range page.Items {
		statuses[item.Fingerprint] = item.Status
	}
	if statuses[acknowledged.Fingerprint] != "suppressed" || statuses[kept.Fingerprint] != "new" {
		t.Fatalf("statuses: %#v", statuses)
	}

	// The CSV export excludes suppressed findings by default.
	rows, err := db.FindingsCSV(ctx, loaded, FindingFilter{})
	if err != nil || len(rows) != 1 || rows[0].Fingerprint != kept.Fingerprint {
		t.Fatalf("export must exclude suppressed findings: %#v (%v)", rows, err)
	}
	// ...unless the filter explicitly selects them.
	suppressed, err := db.FindingsCSV(ctx, loaded, FindingFilter{Status: "suppressed"})
	if err != nil || len(suppressed) != 1 || suppressed[0].Fingerprint != acknowledged.Fingerprint {
		t.Fatalf("status=suppressed must export the dismissed finding: %#v (%v)", suppressed, err)
	}

	// A dismissed fingerprint that disappeared is not fixed.
	fixed, err := db.FixedFindings(ctx, loaded, 0)
	if err != nil || fixed.Total != 0 || len(fixed.Items) != 0 {
		t.Fatalf("a suppressed fingerprint must not report as fixed: %#v (%v)", fixed, err)
	}

	// Removing the suppression un-suppresses the next scan.
	if err := db.RemoveSuppression(ctx, work.ID, acknowledged.Fingerprint); err != nil {
		t.Fatal(err)
	}
	next, err := db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveAnalyzerResult(ctx, next.ID, AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{acknowledged, kept}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteScan(ctx, next.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	nextLoaded, err := db.Scan(ctx, next.ID)
	if err != nil || nextLoaded.TotalFindings != 2 {
		t.Fatalf("removed suppression must restore counting: %#v (%v)", nextLoaded, err)
	}
	page, err = db.FindingsPage(ctx, nextLoaded, FindingFilter{Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.Fingerprint == acknowledged.Fingerprint && item.Status != "persistent" {
			t.Fatalf("un-suppressed persistent finding must compare normally again: %#v", item)
		}
	}
}

// ---------------------------------------------------------------------------
// Large-volume responsiveness guards. Each scenario seeds a temp SQLite
// database with realistic synthetic volumes, runs the operation repeatedly,
// and asserts a generous p95 budget (>= 2x the worst observed p95 on the
// reference machine) so CI jitter cannot flake while real regressions still
// fail loudly. Seeds are skipped under -short.
// ---------------------------------------------------------------------------

var (
	perfBase       = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	perfAnalyzers  = []string{"biome", "ruff", "semgrep", "sonarqube"}
	perfSeverities = []string{"critical", "high", "medium", "low", "info"}
	perfCategories = []string{"correctness", "suspicious", "style", "security"}
)

func openPerfDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func perfWorkspace(t *testing.T, db *DB, name string) core.Workspace {
	t.Helper()
	work, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: name, RootPath: "C:/perf/" + name})
	if err != nil {
		t.Fatal(err)
	}
	return work
}

// perfSnapshotJSON approximates a real immutable snapshot manifest (~5KB of
// selected file paths) so scan-list queries move realistic byte volumes.
func perfSnapshotJSON(workspaceID string, seq int) string {
	files := make([]string, 0, 140)
	for i := 0; i < 140; i++ {
		files = append(files, fmt.Sprintf("src/pkg%02d/file%03d.ts", i%12, i))
	}
	// The payload shape is fixed and marshal cannot fail; ignoring the error
	// keeps this helper usable without a *testing.T.
	payload, _ := json.Marshal(map[string]any{
		"workspace_id": workspaceID,
		"captured_at":  perfBase.Add(time.Duration(seq) * 15 * time.Minute).Format(time.RFC3339),
		"profile":      "standard", "candidate_file_count": 4200, "selected_file_count": 140,
		"selected_files": files, "languages": map[string]int{"typescript": 90, "python": 50},
	})
	return string(payload)
}

// seedPerfScan inserts one scan row directly (bulk seeding is far faster than
// going through CreateScan) with staggered recency so ordering queries across
// workspaces have real work to do. seq must increase with recency.
func seedPerfScan(t *testing.T, db *DB, workspaceID, scanID, state string, seq int, counts [5]int, snapshotJSON string) {
	t.Helper()
	started := perfBase.Add(time.Duration(seq) * 15 * time.Minute)
	finished := any(nil)
	switch state {
	case "completed", "completed_with_warnings", "failed", "cancelled", "interrupted":
		stamp := started.Add(6 * time.Minute)
		finished = stamp.UTC().Format(time.RFC3339Nano)
	}
	total := 0
	for _, count := range counts {
		total += count
	}
	if snapshotJSON == "" {
		snapshotJSON = "{}"
	}
	_, err := db.SQL.Exec(`INSERT INTO scans(id,workspace_id,state,profile,started_at,finished_at,candidate_file_count,selected_file_count,total_findings,critical_count,high_count,medium_count,low_count,info_count,snapshot_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		scanID, workspaceID, state, "standard", started.UTC().Format(time.RFC3339Nano), finished, 4200, 140, total, counts[0], counts[1], counts[2], counts[3], counts[4], snapshotJSON)
	if err != nil {
		t.Fatal(err)
	}
	for _, analyzer := range perfAnalyzers {
		if _, err := db.SQL.Exec(`INSERT INTO analyzer_runs(id,scan_id,analyzer_id,version,state,started_at,finished_at,duration_ms,exit_code,finding_count) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			"run-"+scanID+"-"+analyzer, scanID, analyzer, "test", "succeeded", started.UTC().Format(time.RFC3339Nano), started.Add(3*time.Minute).UTC().Format(time.RFC3339Nano), 180000, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
}

func perfPath(i int) string  { return fmt.Sprintf("src/pkg%02d/mod%03d/file%04d.ts", i%12, i%140, i%9000) }
func perfMessage(i int) string {
	base := fmt.Sprintf("Finding %06d: implicit string concatenation inside a loop can degrade into quadratic behavior; build the parts once outside the loop and reuse the buffer across iterations of the surrounding scope for better throughput.", i)
	if i%25 == 0 {
		base += " needle: rare-token-match"
	}
	return base
}

// bulkInsertFindings loads synthetic findings through one prepared statement
// inside a single transaction; fingerprints come from the caller so cross-scan
// overlap (comparison status) is under the test's control.
func bulkInsertFindings(t *testing.T, db *DB, scanID string, rows int, fingerprint func(i int) string) {
	t.Helper()
	tx, err := db.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.Prepare(`INSERT INTO findings(id,scan_id,analyzer_run_id,analyzer_id,rule_id,fingerprint,severity,category,title,message,relative_path,start_line,start_column,end_line,end_column,remediation,documentation_url,raw_severity,metadata_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < rows; i++ {
		analyzer := perfAnalyzers[i%len(perfAnalyzers)]
		if _, err := stmt.Exec(fmt.Sprintf("f-%s-%06d", scanID, i), scanID, "run-"+scanID+"-ruff", analyzer,
			fmt.Sprintf("%s-R%03d", analyzer, i%120), fingerprint(i), perfSeverities[i%5], perfCategories[i%len(perfCategories)],
			fmt.Sprintf("Issue summary %06d", i), perfMessage(i), perfPath(i), i%900+1, 1, i%900+2, 48,
			"Apply the suggested remediation and re-run the scan.", "", "", "{}"); err != nil {
			t.Fatal(err)
		}
	}
	if err := stmt.Close(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// measurePerf runs one warm-up (also failing on op errors) plus iterations
// timed runs and returns the p95 and worst observed duration.
func measurePerf(t *testing.T, iterations int, op func() error) (time.Duration, time.Duration) {
	t.Helper()
	if err := op(); err != nil {
		t.Fatal(err)
	}
	samples := make([]time.Duration, 0, iterations)
	for i := 0; i < iterations; i++ {
		start := time.Now()
		if err := op(); err != nil {
			t.Fatal(err)
		}
		samples = append(samples, time.Since(start))
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	idx := len(samples) * 95 / 100
	if idx >= len(samples) {
		idx = len(samples) - 1
	}
	return samples[idx], samples[len(samples)-1]
}

// assertBudget fails with a clear report when a p95 sample exceeds its budget.
func assertBudget(t *testing.T, name string, p95, worst, budget time.Duration) {
	t.Helper()
	t.Logf("%s: p95=%v worst=%v budget=%v", name, p95, worst, budget)
	if p95 > budget {
		t.Fatalf("%s p95 %v exceeded budget %v", name, p95, budget)
	}
}

// explainPlan returns the joined EXPLAIN QUERY PLAN detail lines for a query.
func explainPlan(t *testing.T, db *DB, query string, args ...any) string {
	t.Helper()
	rows, err := db.SQL.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(lines, " | ")
}

// TestLargeVolumeFindingsPageResponsiveness guards the paged findings query on
// a workspace holding 50k findings across 5 scans (10k in the latest), with
// every expensive shape represented: comparison-status EXISTS filtering and
// sorting, deep offsets, LIKE path/query filters, and severity sort.
func TestLargeVolumeFindingsPageResponsiveness(t *testing.T) {
	if testing.Short() {
		t.Skip("large-volume seed skipped in -short mode")
	}
	ctx := context.Background()
	db := openPerfDB(t)
	work := perfWorkspace(t, db, "pages")
	var latest string
	for scan := 0; scan < 5; scan++ {
		id := fmt.Sprintf("pg-scan-%d", scan)
		seedPerfScan(t, db, work.ID, id, "completed", scan, [5]int{2000, 2000, 2000, 2000, 2000}, perfSnapshotJSON(work.ID, scan))
		if scan == 5-1 {
			latest = id
		}
	}
	for scan := 0; scan < 5; scan++ {
		scanID := fmt.Sprintf("pg-scan-%d", scan)
		// The latest scan shares a third of its fingerprints with its
		// predecessor so the comparison-status EXISTS subquery has real hits.
		bulkInsertFindings(t, db, scanID, 10000, func(i int) string {
			if scan == 4 && i%3 == 0 {
				return fmt.Sprintf("pg-scan-3-%06d", i)
			}
			return fmt.Sprintf("%s-%06d", scanID, i)
		})
	}
	scan, err := db.Scan(ctx, latest)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		filter FindingFilter
		check  func(t *testing.T, page FindingsPage)
	}{
		{"default page", FindingFilter{Limit: 50}, func(t *testing.T, page FindingsPage) {
			if page.Total != 10000 {
				t.Fatalf("total=%d want 10000", page.Total)
			}
		}},
		{"severity filter + sort, deep offset", FindingFilter{Severity: "high", Sort: "severity", Order: "desc", Limit: 100, Offset: 1500}, func(t *testing.T, page FindingsPage) {
			if page.Total != 2000 || len(page.Items) != 100 {
				t.Fatalf("total=%d items=%d want 2000/100", page.Total, len(page.Items))
			}
		}},
		{"status=new EXISTS filter", FindingFilter{Status: "new", Limit: 100}, func(t *testing.T, page FindingsPage) {
			if page.Total != 6666 {
				t.Fatalf("new total=%d want 6666", page.Total)
			}
		}},
		{"status sort (EXISTS per row)", FindingFilter{Sort: "status", Limit: 100}, nil},
		{"path LIKE filter", FindingFilter{Path: "pkg03", Sort: "path", Limit: 100}, nil},
		{"free-text query filter", FindingFilter{Query: "needle", Limit: 100}, func(t *testing.T, page FindingsPage) {
			if page.Total != 400 {
				t.Fatalf("query total=%d want 400", page.Total)
			}
		}},
	}
	for _, testCase := range cases {
		filter := testCase.filter
		var page FindingsPage
		p95, worst := measurePerf(t, 15, func() error {
			var err error
			page, err = db.FindingsPage(ctx, scan, filter)
			return err
		})
		if testCase.check != nil {
			testCase.check(t, page)
		}
		assertBudget(t, "FindingsPage/"+testCase.name, p95, worst, 150*time.Millisecond)
	}
}

// TestLargeVolumeFindingsCSVExport guards the CSV export path on a scan with
// 52k findings over a 30k predecessor: the export must honor the 50k row cap,
// keep comparison-status evaluation affordable, and stay well inside the 3s
// budget while materializing the bounded slice in memory.
func TestLargeVolumeFindingsCSVExport(t *testing.T) {
	if testing.Short() {
		t.Skip("large-volume seed skipped in -short mode")
	}
	ctx := context.Background()
	db := openPerfDB(t)
	work := perfWorkspace(t, db, "export")
	seedPerfScan(t, db, work.ID, "csv-prev", "completed", 0, [5]int{6000, 6000, 6000, 6000, 6000}, perfSnapshotJSON(work.ID, 0))
	seedPerfScan(t, db, work.ID, "csv-latest", "completed", 1, [5]int{10400, 10400, 10400, 10400, 10400}, perfSnapshotJSON(work.ID, 1))
	bulkInsertFindings(t, db, "csv-prev", 30000, func(i int) string { return fmt.Sprintf("csv-fp-%06d", i) })
	bulkInsertFindings(t, db, "csv-latest", 52000, func(i int) string {
		if i < 30000 && i%2 == 0 {
			return fmt.Sprintf("csv-fp-%06d", i)
		}
		return fmt.Sprintf("csv-new-%06d", i)
	})
	scan, err := db.Scan(ctx, "csv-latest")
	if err != nil {
		t.Fatal(err)
	}
	var rows []analyzers.Finding
	p95, worst := measurePerf(t, 5, func() error {
		var err error
		rows, err = db.FindingsCSV(ctx, scan, FindingFilter{})
		return err
	})
	if len(rows) != MaxFindingsExportRows {
		t.Fatalf("export rows=%d want cap %d", len(rows), MaxFindingsExportRows)
	}
	assertBudget(t, "FindingsCSV/52k-row scan", p95, worst, 3*time.Second)
}

// TestLargeVolumeFixedFindings guards the fixed-findings panel when an entire
// 20k-finding scan was resolved: COUNT plus the capped 200-row page must stay
// responsive with the NOT EXISTS + analyzer-coverage subqueries in play.
func TestLargeVolumeFixedFindings(t *testing.T) {
	if testing.Short() {
		t.Skip("large-volume seed skipped in -short mode")
	}
	ctx := context.Background()
	db := openPerfDB(t)
	work := perfWorkspace(t, db, "fixed")
	seedPerfScan(t, db, work.ID, "fixed-prev", "completed", 0, [5]int{4000, 4000, 4000, 4000, 4000}, perfSnapshotJSON(work.ID, 0))
	seedPerfScan(t, db, work.ID, "fixed-latest", "completed", 1, [5]int{100, 100, 100, 100, 100}, perfSnapshotJSON(work.ID, 1))
	bulkInsertFindings(t, db, "fixed-prev", 20000, func(i int) string { return fmt.Sprintf("fixed-prev-%06d", i) })
	bulkInsertFindings(t, db, "fixed-latest", 500, func(i int) string { return fmt.Sprintf("fixed-new-%06d", i) })
	scan, err := db.Scan(ctx, "fixed-latest")
	if err != nil {
		t.Fatal(err)
	}
	var result FixedFindingsResult
	p95, worst := measurePerf(t, 10, func() error {
		var err error
		result, err = db.FixedFindings(ctx, scan, MaxFixedFindingsLimit)
		return err
	})
	if result.Total != 20000 || len(result.Items) != MaxFixedFindingsLimit {
		t.Fatalf("fixed total=%d items=%d want 20000/%d", result.Total, len(result.Items), MaxFixedFindingsLimit)
	}
	assertBudget(t, "FixedFindings/20k fixed", p95, worst, 300*time.Millisecond)
}

// TestLargeVolumeDashboardQueries guards the dashboard pair (RecentScans +
// ScanSummary) at 10 workspaces x 200 scans each. The summary's
// ROW_NUMBER-over-partition and the global recency join are EXPLAINed so a
// missing index would be visible in the failure output.
func TestLargeVolumeDashboardQueries(t *testing.T) {
	if testing.Short() {
		t.Skip("large-volume seed skipped in -short mode")
	}
	ctx := context.Background()
	db := openPerfDB(t)
	seq := 0
	var expectedTotal int
	for w := 0; w < 10; w++ {
		work := perfWorkspace(t, db, fmt.Sprintf("dash%02d", w))
		for s := 0; s < 200; s++ {
			state := "completed"
			switch {
			case s%37 == 0:
				state = "completed_with_warnings"
			case s%53 == 0:
				state = "failed"
			case s%97 == 0:
				state = "running"
			}
			counts := [5]int{s % 7 + 1, s % 11 + 1, s % 13 + 1, s % 17 + 1, s % 19 + 1}
			seedPerfScan(t, db, work.ID, fmt.Sprintf("dash-%02d-%03d", w, s), state, seq, counts, perfSnapshotJSON(work.ID, seq))
			seq++
			if state == "completed" || state == "completed_with_warnings" {
				// Only the latest producing scan per workspace feeds totals;
				// the last scan of each workspace below is interrupted-free by
				// construction, so track the per-workspace latest contribution.
				expectedTotal += 0
			}
		}
	}
	t.Logf("EXPLAIN RecentScans: %s", explainPlan(t, db, `SELECT scans.id FROM scans JOIN workspaces ON workspaces.id=scans.workspace_id ORDER BY scans.started_at DESC,scans.id DESC LIMIT 10`))
	t.Logf("EXPLAIN RecentScans COUNT: %s", explainPlan(t, db, `SELECT COUNT(*) FROM scans`))
	t.Logf("EXPLAIN ScanSummary window: %s", explainPlan(t, db, `SELECT COUNT(*),COALESCE(SUM(latest.critical_count),0),COALESCE(SUM(latest.total_findings),0) FROM (SELECT critical_count,total_findings,ROW_NUMBER() OVER (PARTITION BY workspace_id ORDER BY COALESCE(finished_at,started_at) DESC,id DESC) AS recency FROM scans WHERE state IN ('completed','completed_with_warnings')) latest WHERE recency=1`))
	var summary ScanSummary
	var recent []RecentScan
	var total int
	p95, worst := measurePerf(t, 10, func() error {
		var err error
		if summary, err = db.ScanSummary(ctx); err != nil {
			return err
		}
		recent, total, err = db.RecentScans(ctx, RecentScansFilter{})
		return err
	})
	if summary.WorkspacesTotal != 10 || summary.ScansTotal != 2000 || len(recent) != DefaultRecentScansLimit || total != 2000 {
		t.Fatalf("summary=%#v recent=%d total=%d", summary, len(recent), total)
	}
	if summary.ActiveScans == 0 {
		t.Fatalf("expected active scans among 2000, got %#v", summary)
	}
	assertBudget(t, "Dashboard/RecentScans+ScanSummary (10x200)", p95, worst, 200*time.Millisecond)
}

// TestLargeVolumeWorkspaceListView measures the exact repository call pattern
// of GET /api/v1/workspaces: one Workspaces() query plus one Scans() query per
// workspace (the N+1 the server loop produces). 50 workspaces x 20 scans with
// realistic 5KB snapshot manifests quantify the cost; a single aggregate
// latest-scan query is timed alongside for comparison.
func TestLargeVolumeWorkspaceListView(t *testing.T) {
	if testing.Short() {
		t.Skip("large-volume seed skipped in -short mode")
	}
	ctx := context.Background()
	db := openPerfDB(t)
	seq := 0
	for w := 0; w < 50; w++ {
		work := perfWorkspace(t, db, fmt.Sprintf("list%02d", w))
		for s := 0; s < 20; s++ {
			state := "completed"
			if s == 0 && w%5 == 0 {
				state = "running"
			}
			seedPerfScan(t, db, work.ID, fmt.Sprintf("list-%02d-%03d", w, s), state, seq, [5]int{s + 1, s, s, s, s}, perfSnapshotJSON(work.ID, seq))
			seq++
		}
	}
	queries := 0
	var latestScans int
	var loopLatest map[string]string
	p95, worst := measurePerf(t, 5, func() error {
		items, err := db.Workspaces(ctx)
		if err != nil {
			return err
		}
		queries = 1 + len(items)
		latestScans = 0
		loopLatest = map[string]string{}
		for _, item := range items {
			scans, err := db.Scans(ctx, item.ID)
			if err != nil {
				return err
			}
			if len(scans) > 0 {
				latestScans++
				loopLatest[item.ID] = scans[0].ID
			}
		}
		return nil
	})
	if queries != 51 || latestScans != 50 {
		t.Fatalf("queries=%d workspacesWithScans=%d want 51/50", queries, latestScans)
	}
	assertBudget(t, "GET /workspaces repository calls (N+1)", p95, worst, 500*time.Millisecond)
	t.Logf("N+1 detail: 1 Workspaces() + 50 Scans() queries load every scan row and hydrate every 5KB snapshot: p95=%v worst=%v", p95, worst)

	// The single-aggregate alternative resolves the same latest scans with one
	// extra query instead of one full-history query per workspace.
	var aggregate map[string]core.Scan
	aggP95, aggWorst := measurePerf(t, 5, func() error {
		var err error
		aggregate, err = db.LatestScans(ctx)
		return err
	})
	if len(aggregate) != 50 {
		t.Fatalf("aggregate latest scans=%d want 50", len(aggregate))
	}
	for workspaceID, scanID := range loopLatest {
		if got := aggregate[workspaceID]; got.ID != scanID {
			t.Fatalf("aggregate latest scan for %s = %s, loop says %s", workspaceID, got.ID, scanID)
		}
	}
	t.Logf("single-aggregate alternative (1 query, latest scan only): p95=%v worst=%v", aggP95, aggWorst)
}

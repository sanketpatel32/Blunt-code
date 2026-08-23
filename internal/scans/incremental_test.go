package scans

// Tests for incremental rescans (ScanOptions.Incremental): hash round-trips,
// the full-vs-incremental equivalence invariant over an unchanged workspace,
// per-file recomputation, analyzer-identity invalidation, removal, suppression
// and comparison semantics on reused findings, and the silent degradation
// paths back to a full scan.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/config"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/events"
)

// fileEchoAnalyzer is a deterministic per-file fake: Normalize emits one
// finding per file of the most recent Plan call, with the file's content in
// the message so editing a file changes the finding's fingerprint. Tests can
// assert exactly which files reached the analyzer and bump its version to
// invalidate reuse.
type fileEchoAnalyzer struct {
	id string
	mu sync.Mutex
	// version is reported by Check and Plan; changing it models a tool
	// upgrade, which must force a full re-run.
	version string
	planned [][]plannedFile
}

// plannedFile keeps both forms of one planned file: the absolute path to read
// the content from and the workspace-relative path findings carry.
type plannedFile struct {
	rel, abs string
}

func (a *fileEchoAnalyzer) ID() string          { return a.id }
func (a *fileEchoAnalyzer) DisplayName() string { return a.id }
func (a *fileEchoAnalyzer) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguagePython}
}
func (a *fileEchoAnalyzer) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return analyzers.ToolStatus{Ready: true, Version: a.version}
}
func (a *fileEchoAnalyzer) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error {
	return nil
}
func (a *fileEchoAnalyzer) Plan(_ context.Context, req analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	files := make([]plannedFile, 0, len(req.Files))
	for _, abs := range req.Files {
		rel, err := filepath.Rel(req.WorkspaceRoot, abs)
		if err != nil {
			return analyzers.AnalyzerPlan{}, err
		}
		files = append(files, plannedFile{rel: filepath.ToSlash(rel), abs: abs})
	}
	a.mu.Lock()
	a.planned = append(a.planned, files)
	version := a.version
	a.mu.Unlock()
	return analyzers.AnalyzerPlan{AnalyzerID: a.id, Version: version}, nil
}
func (a *fileEchoAnalyzer) Run(context.Context, analyzers.AnalyzerPlan, analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	return analyzers.AnalyzerResult{}, nil
}
func (a *fileEchoAnalyzer) Normalize(context.Context, analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	a.mu.Lock()
	var last []plannedFile
	if len(a.planned) > 0 {
		last = a.planned[len(a.planned)-1]
	}
	a.mu.Unlock()
	findings := make([]analyzers.Finding, 0, len(last))
	for _, file := range last {
		content, err := os.ReadFile(file.abs)
		if err != nil {
			return nil, nil, err
		}
		finding := analyzers.Finding{
			AnalyzerID: a.id, RuleID: "X1", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryCorrectness,
			Title: "Echo finding", Message: fmt.Sprintf("issue in %s: %s", file.rel, content), RelativePath: file.rel, StartLine: 1,
		}
		finding.SetFingerprint()
		findings = append(findings, finding)
	}
	return findings, nil, nil
}

func (a *fileEchoAnalyzer) setVersion(version string) {
	a.mu.Lock()
	a.version = version
	a.mu.Unlock()
}

func (a *fileEchoAnalyzer) planCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.planned)
}

// lastPlanned returns the relative paths of the most recent Plan call.
func (a *fileEchoAnalyzer) lastPlanned() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.planned) == 0 {
		return nil
	}
	last := a.planned[len(a.planned)-1]
	rels := make([]string, 0, len(last))
	for _, file := range last {
		rels = append(rels, file.rel)
	}
	return rels
}

// newIncrementalFixture wires a scanFixture whose workspace holds the given
// files (relative slash paths) and whose registry holds the given adapters.
func newIncrementalFixture(t *testing.T, adapters []analyzers.Analyzer, files map[string]string) *scanFixture {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	work, err := db.CreateWorkspace(context.Background(), core.Workspace{Name: "Incremental", RootPath: root})
	if err != nil {
		t.Fatal(err)
	}
	registry := analyzers.NewRegistry()
	for _, adapter := range adapters {
		if err := registry.Register(adapter); err != nil {
			t.Fatal(err)
		}
	}
	bus := events.New()
	return &scanFixture{db: db, bus: bus, service: New(db, registry, bus, filepath.Join(paths.DataDir, "reports"), paths.ToolsDir, nil), work: work}
}

func writeFixtureFile(t *testing.T, fixture *scanFixture, rel, content string) {
	t.Helper()
	full := filepath.Join(fixture.work.RootPath, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func scanFingerprints(t *testing.T, fixture *scanFixture, scanID string) []string {
	t.Helper()
	findings, err := fixture.db.Findings(context.Background(), scanID)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(findings))
	for _, finding := range findings {
		out = append(out, finding.Fingerprint)
	}
	sort.Strings(out)
	return out
}

func scanFindingByPath(t *testing.T, fixture *scanFixture, scanID, path string) analyzers.Finding {
	t.Helper()
	findings, err := fixture.db.Findings(context.Background(), scanID)
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range findings {
		if finding.RelativePath == path {
			return finding
		}
	}
	t.Fatalf("scan %s has no finding for %s", scanID, path)
	return analyzers.Finding{}
}

// TestIncrementalHashRoundTripWritesAndReuses covers the storage contract: a
// completed scan records content hashes plus its analyzer identity, and the
// next incremental scan over an unchanged workspace reuses the findings
// verbatim without planning the analyzer again.
func TestIncrementalHashRoundTripWritesAndReuses(t *testing.T) {
	analyzer := &fileEchoAnalyzer{id: "fake", version: "v1"}
	fixture := newIncrementalFixture(t, []analyzers.Analyzer{analyzer}, map[string]string{"a.py": "x=1\n", "b.py": "y=2\n"})
	ctx := context.Background()

	first := fixture.startScanWithOptions(t, ScanOptions{Incremental: true}) // no previous scan: full run
	if final := fixture.waitForTerminal(t, first.ID); final.State != "completed" {
		t.Fatalf("first scan state = %q (error: %s)", final.State, final.ErrorSummary)
	}
	if planned := analyzer.lastPlanned(); len(planned) != 2 {
		t.Fatalf("first scan planned files = %v, want both files", planned)
	}
	hashes, err := fixture.db.ScanFileHashes(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 2 {
		t.Fatalf("recorded hashes = %v, want one per selected file", hashes)
	}
	for path, hash := range hashes {
		if len(hash) != 64 { // sha256 hex
			t.Fatalf("hash for %s = %q, want a sha256 hex digest", path, hash)
		}
	}
	identity, err := fixture.db.ScanHashAnalyzerSet(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(identity, `"fake":"v1"`) || !strings.Contains(identity, `"profile":"standard"`) {
		t.Fatalf("recorded analyzer identity = %s, want the analyzer id/version and profile", identity)
	}

	second := fixture.startScanWithOptions(t, ScanOptions{Incremental: true})
	final := fixture.waitForTerminal(t, second.ID)
	if final.State != "completed" {
		t.Fatalf("second scan state = %q (error: %s)", final.State, final.ErrorSummary)
	}
	if got := analyzer.planCount(); got != 1 {
		t.Fatalf("analyzer planned %d times across both scans, want 1 (unchanged workspace must not re-plan)", got)
	}
	if got, want := scanFingerprints(t, fixture, second.ID), scanFingerprints(t, fixture, first.ID); !equalStrings(got, want) {
		t.Fatalf("reused fingerprints = %v, want the first scan's %v", got, want)
	}
	runs, err := fixture.db.AnalyzerRuns(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].State != "succeeded" || runs[0].FindingCount != 2 || runs[0].AnalyzerID != "fake" {
		t.Fatalf("reused analyzer runs = %#v, want one succeeded fake run carrying both findings", runs)
	}
	if final.TotalFindings != 2 {
		t.Fatalf("second scan total findings = %d, want 2", final.TotalFindings)
	}
	if !strings.Contains(final.ErrorSummary, "incremental: reused findings for 2 unchanged file(s), ran analyzers on 0 file(s)") {
		t.Fatalf("scan note = %q, want the incremental reuse note", final.ErrorSummary)
	}
	secondHashes, err := fixture.db.ScanFileHashes(ctx, second.ID)
	if err != nil || len(secondHashes) != 2 {
		t.Fatalf("second scan hashes = %v err=%v, want the full two-file base recorded again", secondHashes, err)
	}
}

// TestIncrementalUnchangedWorkspaceEqualsFullScan is the core invariant: over
// an unchanged workspace, an incremental scan and a full scan produce the same
// fingerprints and totals.
func TestIncrementalUnchangedWorkspaceEqualsFullScan(t *testing.T) {
	analyzer := &fileEchoAnalyzer{id: "fake", version: "v1"}
	fixture := newIncrementalFixture(t, []analyzers.Analyzer{analyzer}, map[string]string{"a.py": "x=1\n", "b.py": "y=2\n"})

	full := fixture.startScanWithOptions(t, ScanOptions{}) // the default: a full scan
	fullFinal := fixture.waitForTerminal(t, full.ID)
	if fullFinal.State != "completed" {
		t.Fatalf("full scan state = %q", fullFinal.State)
	}
	incremental := fixture.startScanWithOptions(t, ScanOptions{Incremental: true})
	incFinal := fixture.waitForTerminal(t, incremental.ID)
	if incFinal.State != "completed" {
		t.Fatalf("incremental scan state = %q (error: %s)", incFinal.State, incFinal.ErrorSummary)
	}
	if got, want := scanFingerprints(t, fixture, incremental.ID), scanFingerprints(t, fixture, full.ID); !equalStrings(got, want) {
		t.Fatalf("incremental fingerprints = %v, want the full scan's %v", got, want)
	}
	if incFinal.TotalFindings != fullFinal.TotalFindings {
		t.Fatalf("incremental total = %d, want the full scan's %d", incFinal.TotalFindings, fullFinal.TotalFindings)
	}
}

// TestIncrementalChangedFileRecomputesOnlyThatFile pins the file scoping: the
// analyzer receives exactly the changed file, that file's findings recompute
// with a new fingerprint, and the untouched file's findings are reused
// verbatim.
func TestIncrementalChangedFileRecomputesOnlyThatFile(t *testing.T) {
	analyzer := &fileEchoAnalyzer{id: "fake", version: "v1"}
	fixture := newIncrementalFixture(t, []analyzers.Analyzer{analyzer}, map[string]string{"a.py": "x=1\n", "b.py": "y=2\n"})

	first := fixture.startScanWithOptions(t, ScanOptions{})
	if final := fixture.waitForTerminal(t, first.ID); final.State != "completed" {
		t.Fatalf("first scan state = %q", final.State)
	}
	beforeA := scanFindingByPath(t, fixture, first.ID, "a.py")
	beforeB := scanFindingByPath(t, fixture, first.ID, "b.py")

	writeFixtureFile(t, fixture, "a.py", "x=1+41\n")
	second := fixture.startScanWithOptions(t, ScanOptions{Incremental: true})
	final := fixture.waitForTerminal(t, second.ID)
	if final.State != "completed" {
		t.Fatalf("second scan state = %q (error: %s)", final.State, final.ErrorSummary)
	}
	if planned := analyzer.lastPlanned(); len(planned) != 1 || planned[0] != "a.py" {
		t.Fatalf("analyzer files = %v, want exactly a.py", planned)
	}
	afterA := scanFindingByPath(t, fixture, second.ID, "a.py")
	if afterA.Fingerprint == beforeA.Fingerprint {
		t.Fatal("changed file's finding fingerprint must change with its content")
	}
	afterB := scanFindingByPath(t, fixture, second.ID, "b.py")
	if afterB.Fingerprint != beforeB.Fingerprint {
		t.Fatal("unchanged file's finding fingerprint must be reused verbatim")
	}
	runs, err := fixture.db.AnalyzerRuns(context.Background(), second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].FindingCount != 2 {
		t.Fatalf("analyzer runs = %#v, want one run whose count covers the fresh and the reused finding", runs)
	}
	if !strings.Contains(final.ErrorSummary, "reused findings for 1 unchanged file(s), ran analyzers on 1 file(s)") {
		t.Fatalf("scan note = %q", final.ErrorSummary)
	}
}

// TestIncrementalAnalyzerVersionChangeForcesFullScan: bumping the analyzer
// version changes the recorded identity, so nothing may be reused and every
// file re-runs.
func TestIncrementalAnalyzerVersionChangeForcesFullScan(t *testing.T) {
	analyzer := &fileEchoAnalyzer{id: "fake", version: "v1"}
	fixture := newIncrementalFixture(t, []analyzers.Analyzer{analyzer}, map[string]string{"a.py": "x=1\n", "b.py": "y=2\n"})

	first := fixture.startScanWithOptions(t, ScanOptions{})
	if final := fixture.waitForTerminal(t, first.ID); final.State != "completed" {
		t.Fatalf("first scan state = %q", final.State)
	}
	analyzer.setVersion("v2")
	second := fixture.startScanWithOptions(t, ScanOptions{Incremental: true})
	if final := fixture.waitForTerminal(t, second.ID); final.State != "completed" {
		t.Fatalf("second scan state = %q (error: %s)", final.State, final.ErrorSummary)
	}
	if got := analyzer.planCount(); got != 2 {
		t.Fatalf("analyzer planned %d times, want 2 (version change must force a full re-run)", got)
	}
	if planned := analyzer.lastPlanned(); len(planned) != 2 {
		t.Fatalf("full re-run files = %v, want both files", planned)
	}
	if final := fixture.waitForTerminal(t, second.ID); strings.Contains(final.ErrorSummary, "incremental:") {
		t.Fatalf("degraded scan must not carry the incremental note: %q", final.ErrorSummary)
	}
}

// TestIncrementalProfileChangeForcesFullScan: the profile is part of the
// analyzer identity (deep widens ruff's rules), so a standard-to-deep change
// refuses reuse even with identical versions and files.
func TestIncrementalProfileChangeForcesFullScan(t *testing.T) {
	analyzer := &fileEchoAnalyzer{id: "fake", version: "v1"}
	fixture := newIncrementalFixture(t, []analyzers.Analyzer{analyzer}, map[string]string{"a.py": "x=1\n"})

	first := fixture.startScanWithOptions(t, ScanOptions{})
	if final := fixture.waitForTerminal(t, first.ID); final.State != "completed" {
		t.Fatalf("first scan state = %q", final.State)
	}
	second, err := fixture.service.DiscoverAndStartWithOptions(context.Background(), fixture.work, "deep", nil, ScanOptions{Incremental: true})
	if err != nil {
		t.Fatal(err)
	}
	if final := fixture.waitForTerminal(t, second.ID); final.State != "completed" {
		t.Fatalf("deep scan state = %q (error: %s)", final.State, final.ErrorSummary)
	}
	if got := analyzer.planCount(); got != 2 {
		t.Fatalf("analyzer planned %d times, want 2 (profile change must force a full re-run)", got)
	}
}

// TestIncrementalRemovedFileDropsFindings: a file that disappeared since the
// previous scan contributes nothing - its findings are neither re-analyzed nor
// copied.
func TestIncrementalRemovedFileDropsFindings(t *testing.T) {
	analyzer := &fileEchoAnalyzer{id: "fake", version: "v1"}
	fixture := newIncrementalFixture(t, []analyzers.Analyzer{analyzer}, map[string]string{"a.py": "x=1\n", "b.py": "y=2\n"})

	first := fixture.startScanWithOptions(t, ScanOptions{})
	if final := fixture.waitForTerminal(t, first.ID); final.State != "completed" {
		t.Fatalf("first scan state = %q", final.State)
	}
	if err := os.Remove(filepath.Join(fixture.work.RootPath, "a.py")); err != nil {
		t.Fatal(err)
	}
	second := fixture.startScanWithOptions(t, ScanOptions{Incremental: true})
	final := fixture.waitForTerminal(t, second.ID)
	if final.State != "completed" {
		t.Fatalf("second scan state = %q (error: %s)", final.State, final.ErrorSummary)
	}
	if got := analyzer.planCount(); got != 1 {
		t.Fatalf("analyzer planned %d times, want 1 (the surviving file is unchanged and reused)", got)
	}
	findings, err := fixture.db.Findings(context.Background(), second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].RelativePath != "b.py" {
		t.Fatalf("findings after removal = %#v, want only b.py's reused finding", findings)
	}
	if final.TotalFindings != 1 {
		t.Fatalf("total findings = %d, want 1", final.TotalFindings)
	}
}

// TestIncrementalSuppressionAndComparisonSurviveReuse: reused findings keep
// their fingerprints, so a dismissal still excludes the finding from totals
// and the new/fixed/persistent comparison still classifies it persistent.
func TestIncrementalSuppressionAndComparisonSurviveReuse(t *testing.T) {
	analyzer := &fileEchoAnalyzer{id: "fake", version: "v1"}
	fixture := newIncrementalFixture(t, []analyzers.Analyzer{analyzer}, map[string]string{"a.py": "x=1\n", "b.py": "y=2\n"})
	ctx := context.Background()

	first := fixture.startScanWithOptions(t, ScanOptions{})
	if final := fixture.waitForTerminal(t, first.ID); final.State != "completed" {
		t.Fatalf("first scan state = %q", final.State)
	}
	dismissed := scanFindingByPath(t, fixture, first.ID, "a.py")
	if _, err := fixture.db.AddSuppression(ctx, fixture.work.ID, dismissed.Fingerprint, "wontfix"); err != nil {
		t.Fatal(err)
	}

	second := fixture.startScanWithOptions(t, ScanOptions{Incremental: true})
	final := fixture.waitForTerminal(t, second.ID)
	if final.State != "completed" {
		t.Fatalf("second scan state = %q (error: %s)", final.State, final.ErrorSummary)
	}
	// The reused finding stays stored but leaves the totals.
	stored, err := fixture.db.Findings(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored findings = %d, want 2 (suppressed findings remain stored)", len(stored))
	}
	if final.TotalFindings != 1 {
		t.Fatalf("total findings = %d, want 1 (suppression must exclude the reused finding)", final.TotalFindings)
	}
	// The comparison machinery is fingerprint-based and coverage comes from
	// the reused run row, so b.py's finding is persistent - not new, not
	// fixed, and the dismissed one is invisible on both sides.
	suppressed, err := fixture.db.SuppressedFingerprints(ctx, fixture.work.ID)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := fixture.db.SuccessfulAnalyzerIDs(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !coverage["fake"] {
		t.Fatalf("coverage = %v, want the reused analyzer counted as succeeded", coverage)
	}
	firstFindings, err := fixture.db.Findings(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	diff := Compare(FilterSuppressed(stored, suppressed), FilterSuppressed(firstFindings, suppressed), coverage)
	if len(diff.Persistent) != 1 || len(diff.New) != 0 || len(diff.Fixed) != 0 {
		t.Fatalf("comparison = new:%d fixed:%d persistent:%d, want 0/0/1 (persistent b.py)", len(diff.New), len(diff.Fixed), len(diff.Persistent))
	}
}

// TestIncrementalWithoutPreviousScanRunsFull: the very first scan of a
// workspace (the --watch case) has nothing to reuse and must run as a plain
// full scan without errors or notes.
func TestIncrementalWithoutPreviousScanRunsFull(t *testing.T) {
	analyzer := &fileEchoAnalyzer{id: "fake", version: "v1"}
	fixture := newIncrementalFixture(t, []analyzers.Analyzer{analyzer}, map[string]string{"a.py": "x=1\n", "b.py": "y=2\n"})

	scan := fixture.startScanWithOptions(t, ScanOptions{Incremental: true})
	final := fixture.waitForTerminal(t, scan.ID)
	if final.State != "completed" {
		t.Fatalf("scan state = %q (error: %s)", final.State, final.ErrorSummary)
	}
	if planned := analyzer.lastPlanned(); len(planned) != 2 {
		t.Fatalf("first scan planned files = %v, want both files", planned)
	}
	if final.ErrorSummary != "" {
		t.Fatalf("degraded scan note = %q, want empty", final.ErrorSummary)
	}
}

// TestIncrementalWithMissingHashesFallsBackToFull: a previous completed scan
// without recorded hashes (it predates the feature or the rows were lost) is
// not a reuse base; the scan silently runs full.
func TestIncrementalWithMissingHashesFallsBackToFull(t *testing.T) {
	analyzer := &fileEchoAnalyzer{id: "fake", version: "v1"}
	fixture := newIncrementalFixture(t, []analyzers.Analyzer{analyzer}, map[string]string{"a.py": "x=1\n", "b.py": "y=2\n"})
	ctx := context.Background()

	first := fixture.startScanWithOptions(t, ScanOptions{})
	if final := fixture.waitForTerminal(t, first.ID); final.State != "completed" {
		t.Fatalf("first scan state = %q", final.State)
	}
	if _, err := fixture.db.SQL.ExecContext(ctx, `DELETE FROM scan_file_hashes WHERE scan_id=?`, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.db.SQL.ExecContext(ctx, `DELETE FROM scan_hash_meta WHERE scan_id=?`, first.ID); err != nil {
		t.Fatal(err)
	}

	second := fixture.startScanWithOptions(t, ScanOptions{Incremental: true})
	final := fixture.waitForTerminal(t, second.ID)
	if final.State != "completed" {
		t.Fatalf("second scan state = %q (error: %s)", final.State, final.ErrorSummary)
	}
	if got := analyzer.planCount(); got != 2 {
		t.Fatalf("analyzer planned %d times, want 2 (missing hashes must force a full re-run)", got)
	}
	if planned := analyzer.lastPlanned(); len(planned) != 2 {
		t.Fatalf("full re-run files = %v, want both files", planned)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

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
	diff := Compare(FilterSuppressed(stored, suppressed), FilterSuppressed(firstFindings, suppressed), NewComparisonCoverage(coverage, nil))
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

// The reuse identity must now carry the provenance contract (IMP-09): the
// config digest and schema versions are recorded beside the analyzer set, so
// two scans are only comparable under identical shaping configuration and
// finding-identity schemas.
func TestIncrementalIdentityCarriesProvenance(t *testing.T) {
	analyzer := &fileEchoAnalyzer{id: "fake", version: "v1"}
	fixture := newIncrementalFixture(t, []analyzers.Analyzer{analyzer}, map[string]string{"a.py": "x=1\n"})

	first := fixture.startScanWithOptions(t, ScanOptions{})
	if final := fixture.waitForTerminal(t, first.ID); final.State != "completed" {
		t.Fatalf("first scan state = %q", first.State)
	}
	identity, err := fixture.db.ScanHashAnalyzerSet(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"config_digest"`, `"fingerprint_version":` + fmt.Sprint(analyzers.FingerprintVersion), `"discovery_policy_version"`} {
		if !strings.Contains(identity, want) {
			t.Fatalf("recorded identity %s lacks %s", identity, want)
		}
	}
}

// Identities recorded before IMP-09 (no provenance fields) must not match a
// modern identity: findings produced under an unstated configuration are not
// comparable, so reuse refuses and the scan runs full.
func TestIncrementalLegacyIdentityForcesFullScan(t *testing.T) {
	analyzer := &fileEchoAnalyzer{id: "fake", version: "v1"}
	fixture := newIncrementalFixture(t, []analyzers.Analyzer{analyzer}, map[string]string{"a.py": "x=1\n", "b.py": "y=2\n"})

	first := fixture.startScanWithOptions(t, ScanOptions{})
	if final := fixture.waitForTerminal(t, first.ID); final.State != "completed" {
		t.Fatalf("first scan state = %q", first.State)
	}
	legacy := fmt.Sprintf(`{"bluntcode_version":%q,"profile":"standard","analyzers":{"fake":"v1"}}`, bluntCodeVersion)
	if _, err := fixture.db.SQL.ExecContext(context.Background(), `UPDATE scan_hash_meta SET analyzers_json=? WHERE scan_id=?`, legacy, first.ID); err != nil {
		t.Fatal(err)
	}
	second := fixture.startScanWithOptions(t, ScanOptions{Incremental: true})
	if final := fixture.waitForTerminal(t, second.ID); final.State != "completed" {
		t.Fatalf("second scan state = %q (error: %s)", final.State, final.ErrorSummary)
	}
	if got := analyzer.planCount(); got != 2 {
		t.Fatalf("analyzer planned %d times, want 2 (a legacy identity must force a full re-run)", got)
	}
}

// Changing the shaping configuration between scans (a workspace rule) changes
// the ConfigDigest and must refuse reuse: findings produced under different
// rules are not comparable.
func TestIncrementalConfigChangeForcesFullScan(t *testing.T) {
	analyzer := &fileEchoAnalyzer{id: "fake", version: "v1"}
	fixture := newIncrementalFixture(t, []analyzers.Analyzer{analyzer}, map[string]string{"a.py": "x=1\n", "b.py": "y=2\n"})

	first := fixture.startScanWithOptions(t, ScanOptions{})
	if final := fixture.waitForTerminal(t, first.ID); final.State != "completed" {
		t.Fatalf("first scan state = %q", first.State)
	}
	rule := core.WorkspaceRule{WorkspaceID: fixture.work.ID, RuleType: "exclude", Pattern: "legacy/", Source: "user", Enabled: true}
	if err := fixture.db.ReplaceUserRules(context.Background(), fixture.work.ID, []core.WorkspaceRule{rule}); err != nil {
		t.Fatal(err)
	}
	second := fixture.startScanWithOptions(t, ScanOptions{Incremental: true})
	if final := fixture.waitForTerminal(t, second.ID); final.State != "completed" {
		t.Fatalf("second scan state = %q (error: %s)", final.State, final.ErrorSummary)
	}
	if got := analyzer.planCount(); got != 2 {
		t.Fatalf("analyzer planned %d times, want 2 (a config change must force a full re-run)", got)
	}
}

// walkerAnalyzer simulates a workspace-scope analyzer (the gitleaks shape): it
// records what it was handed but reports findings for every Python file in the
// workspace by walking the root itself — its output always covers the whole
// tree no matter which files the orchestrator passed.
type walkerAnalyzer struct {
	mu      sync.Mutex
	version string
	root    string
	planned [][]string
}

func (a *walkerAnalyzer) ID() string          { return "gitleaks-secrets" }
func (a *walkerAnalyzer) DisplayName() string { return "Walker" }
func (a *walkerAnalyzer) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguagePython}
}
func (a *walkerAnalyzer) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return analyzers.ToolStatus{Ready: true, Version: a.version}
}
func (a *walkerAnalyzer) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error { return nil }
func (a *walkerAnalyzer) Plan(_ context.Context, req analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	rels := make([]string, 0, len(req.Files))
	for _, abs := range req.Files {
		rel, err := filepath.Rel(req.WorkspaceRoot, abs)
		if err != nil {
			return analyzers.AnalyzerPlan{}, err
		}
		rels = append(rels, filepath.ToSlash(rel))
	}
	a.mu.Lock()
	a.root = req.WorkspaceRoot
	a.planned = append(a.planned, rels)
	a.mu.Unlock()
	return analyzers.AnalyzerPlan{AnalyzerID: a.ID(), Version: a.version}, nil
}
func (a *walkerAnalyzer) Run(context.Context, analyzers.AnalyzerPlan, analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	return analyzers.AnalyzerResult{}, nil
}
func (a *walkerAnalyzer) Normalize(context.Context, analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	a.mu.Lock()
	root := a.root
	a.mu.Unlock()
	var findings []analyzers.Finding
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".py") {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		finding := analyzers.Finding{
			AnalyzerID: a.ID(), RuleID: "W1", Severity: analyzers.SeverityLow, Category: analyzers.CategorySecurity,
			Message: "walker: " + string(content), RelativePath: filepath.ToSlash(rel), StartLine: 1,
		}
		finding.SetFingerprint()
		findings = append(findings, finding)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return findings, nil, nil
}

func (a *walkerAnalyzer) planCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.planned)
}

func (a *walkerAnalyzer) lastPlanned() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.planned) == 0 {
		return nil
	}
	return a.planned[len(a.planned)-1]
}

// TestIncrementalWorkspaceWalkerRerunsWholeTreeAndNeverDoubles pins the
// workspace-scope reuse contract: once anything changed, a directory-walking
// analyzer is handed the full selection (not the changed subset), its fresh
// output is the whole-tree truth, and unchanged-file findings are never
// appended on top. Without this, a re-run walker's report would double every
// unchanged finding. A later no-change scan copies the walker wholesale.
func TestIncrementalWorkspaceWalkerRerunsWholeTreeAndNeverDoubles(t *testing.T) {
	walker := &walkerAnalyzer{version: "v1"}
	fixture := newIncrementalFixture(t, []analyzers.Analyzer{walker}, map[string]string{"a.py": "x=1\n", "b.py": "y=2\n"})

	first := fixture.startScanWithOptions(t, ScanOptions{})
	firstFinal := fixture.waitForTerminal(t, first.ID)
	if firstFinal.State != "completed" {
		t.Fatalf("first scan state = %q (error: %s)", firstFinal.State, firstFinal.ErrorSummary)
	}
	before := scanFingerprints(t, fixture, first.ID)
	if len(before) != 2 {
		t.Fatalf("first scan findings = %v, want one per file", before)
	}

	writeFixtureFile(t, fixture, "a.py", "x=42\n")
	second := fixture.startScanWithOptions(t, ScanOptions{Incremental: true})
	secondFinal := fixture.waitForTerminal(t, second.ID)
	if secondFinal.State != "completed" {
		t.Fatalf("second scan state = %q (error: %s)", secondFinal.State, secondFinal.ErrorSummary)
	}
	if got := walker.planCount(); got != 2 {
		t.Fatalf("walker planned %d times, want 2 (any change re-runs a workspace-scope analyzer)", got)
	}
	if handed := walker.lastPlanned(); !equalStrings(handed, []string{"a.py", "b.py"}) {
		t.Fatalf("walker handed files = %v, want the FULL selection (routing must not narrow walkers)", handed)
	}
	after := scanFingerprints(t, fixture, second.ID)
	if len(after) != 2 {
		t.Fatalf("incremental walker findings = %d (%v), want exactly 2 — never the fresh report plus reused copies", len(after), after)
	}
	if secondFinal.TotalFindings != 2 {
		t.Fatalf("second scan total = %d, want 2", secondFinal.TotalFindings)
	}
	// The reuse manifest is recorded on the snapshot: one changed file, one
	// reused, and the walker counted as freshly run (not copied wholesale).
	if manifest := secondFinal.Snapshot.Incremental; manifest == nil {
		t.Fatal("incremental scan records no reuse manifest on the snapshot")
	} else {
		if manifest.ReusedFromScanID != first.ID || manifest.ChangedFileCount != 1 || manifest.ReusedFileCount != 1 {
			t.Fatalf("reuse manifest = %+v", manifest)
		}
		if len(manifest.RanAnalyzers) != 1 || manifest.RanAnalyzers[0] != walker.ID() {
			t.Fatalf("manifest ran analyzers = %v, want the walker", manifest.RanAnalyzers)
		}
		if len(manifest.ReusedAnalyzers) != 0 {
			t.Fatalf("manifest reused analyzers = %v, want none (the walker re-ran)", manifest.ReusedAnalyzers)
		}
	}

	// With no further changes the walker is copied wholesale: no re-plan and
	// identical fingerprints.
	third := fixture.startScanWithOptions(t, ScanOptions{Incremental: true})
	thirdFinal := fixture.waitForTerminal(t, third.ID)
	if thirdFinal.State != "completed" {
		t.Fatalf("third scan state = %q (error: %s)", thirdFinal.State, thirdFinal.ErrorSummary)
	}
	if got := walker.planCount(); got != 2 {
		t.Fatalf("walker planned %d times, want 2 (unchanged workspace must copy the walker wholesale)", got)
	}
	if got, want := scanFingerprints(t, fixture, third.ID), after; !equalStrings(got, want) {
		t.Fatalf("third scan fingerprints = %v, want %v", got, want)
	}
}

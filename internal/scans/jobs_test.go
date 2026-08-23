package scans

// Tests for the optional analyzer parallelism bound (ScanOptions.Jobs): the
// default stays sequential, a bound never runs more analyzers than allowed,
// persistence is identical either way, and cancellation still stops queued
// runs.

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
)

// runTracker records concurrency and ordering across analyzer runs so tests
// can pin scheduling semantics without depending on wall-clock timing beyond
// a sleep that widens the overlap window.
type runTracker struct {
	mu      sync.Mutex
	current int
	peak    int
	runs    map[string]int
	order   []string
}

func newRunTracker() *runTracker { return &runTracker{runs: map[string]int{}} }

func (t *runTracker) enter(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current++
	if t.current > t.peak {
		t.peak = t.current
	}
	t.runs[id]++
	t.order = append(t.order, id+"+")
}

func (t *runTracker) exit(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current--
	t.order = append(t.order, id+"-")
}

func (t *runTracker) peakConcurrency() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.peak
}

func (t *runTracker) runCount(id string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.runs[id]
}

func (t *runTracker) runOrder() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.order...)
}

// countingAnalyzer reports every run to a shared runTracker, sleeps to widen
// the concurrency window, and can produce a finding or panic like the
// scriptedAnalyzer in service_test.go.
type countingAnalyzer struct {
	id         string
	tracker    *runTracker
	sleep      time.Duration
	findings   bool
	panicInRun bool
}

func (a *countingAnalyzer) ID() string          { return a.id }
func (a *countingAnalyzer) DisplayName() string { return a.id }
func (a *countingAnalyzer) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguagePython}
}
func (a *countingAnalyzer) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	return analyzers.ToolStatus{Ready: true, Version: "test"}
}
func (a *countingAnalyzer) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error {
	return nil
}
func (a *countingAnalyzer) Plan(context.Context, analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	return analyzers.AnalyzerPlan{AnalyzerID: a.id, Version: "test"}, nil
}
func (a *countingAnalyzer) Run(context.Context, analyzers.AnalyzerPlan, analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	a.tracker.enter(a.id)
	if a.sleep > 0 {
		time.Sleep(a.sleep)
	}
	a.tracker.exit(a.id)
	if a.panicInRun {
		panic("counting analyzer failure")
	}
	return analyzers.AnalyzerResult{}, nil
}
func (a *countingAnalyzer) Normalize(context.Context, analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	if a.findings {
		return []analyzers.Finding{testFinding(a.id)}, nil, nil
	}
	return nil, nil, nil
}

func newCountingAnalyzers(tracker *runTracker, sleep time.Duration, findings bool, ids ...string) []analyzers.Analyzer {
	adapters := make([]analyzers.Analyzer, 0, len(ids))
	for _, id := range ids {
		adapters = append(adapters, &countingAnalyzer{id: id, tracker: tracker, sleep: sleep, findings: findings})
	}
	return adapters
}

func (f *scanFixture) startScanWithOptions(t *testing.T, opts ScanOptions) core.Scan {
	t.Helper()
	scan, err := f.service.DiscoverAndStartWithOptions(context.Background(), f.work, "standard", nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	return scan
}

// scanRunStates maps analyzer ID to persisted run state for one scan.
func scanRunStates(t *testing.T, fixture *scanFixture, scanID string) map[string]string {
	t.Helper()
	runs, err := fixture.db.AnalyzerRuns(context.Background(), scanID)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, run := range runs {
		states[run.AnalyzerID] = run.State
	}
	return states
}

func scanFindingCount(t *testing.T, fixture *scanFixture, scanID string) int {
	t.Helper()
	findings, err := fixture.db.Findings(context.Background(), scanID)
	if err != nil {
		t.Fatal(err)
	}
	return len(findings)
}

// TestJobsBoundsConcurrentAnalyzerRuns pins the bound itself: with Jobs=2 and
// four analyzers, no more than two ever run at once, yet the runs genuinely
// overlap (the bound is exercised, not just unused).
func TestJobsBoundsConcurrentAnalyzerRuns(t *testing.T) {
	tracker := newRunTracker()
	fixture := newScanFixture(t, newCountingAnalyzers(tracker, 100*time.Millisecond, true, "one", "two", "three", "four")...)
	scan := fixture.startScanWithOptions(t, ScanOptions{Jobs: 2})
	if final := fixture.waitForTerminal(t, scan.ID); final.State != "completed" {
		t.Fatalf("scan state = %q, want completed", final.State)
	}
	if peak := tracker.peakConcurrency(); peak > 2 {
		t.Fatalf("peak concurrency = %d, want at most 2 under Jobs=2", peak)
	}
	if peak := tracker.peakConcurrency(); peak < 2 {
		t.Fatalf("peak concurrency = %d, want 2 (analyzers must actually overlap)", peak)
	}
	states := scanRunStates(t, fixture, scan.ID)
	if len(states) != 4 {
		t.Fatalf("persisted runs = %v, want all four analyzers", states)
	}
	for id, state := range states {
		if state != "succeeded" {
			t.Fatalf("analyzer %s run state = %q, want succeeded", id, state)
		}
	}
	if count := scanFindingCount(t, fixture, scan.ID); count != 4 {
		t.Fatalf("findings = %d, want 4 (one per analyzer)", count)
	}
}

// TestDefaultJobsKeepsSequentialAnalyzerExecution encodes the historical
// scheduling semantics: without a jobs bound, analyzers run strictly one at a
// time, in registry order, each finishing before the next starts.
func TestDefaultJobsKeepsSequentialAnalyzerExecution(t *testing.T) {
	tracker := newRunTracker()
	fixture := newScanFixture(t, newCountingAnalyzers(tracker, 5*time.Millisecond, false, "one", "two", "three", "four")...)
	scan := fixture.startScan(t) // the default entry point, no options
	if final := fixture.waitForTerminal(t, scan.ID); final.State != "completed" {
		t.Fatalf("scan state = %q, want completed", final.State)
	}
	if peak := tracker.peakConcurrency(); peak != 1 {
		t.Fatalf("peak concurrency = %d, want 1 (sequential default)", peak)
	}
	want := []string{"one+", "one-", "two+", "two-", "three+", "three-", "four+", "four-"}
	if got := tracker.runOrder(); !reflect.DeepEqual(got, want) {
		t.Fatalf("run order = %v, want strict registry-order sequencing %v", got, want)
	}
}

// TestBoundedScanPersistsSameRunsAndFindingsAsDefault checks the persistence
// invariants: a bounded scan and a default scan of the same workspace persist
// the same analyzer run states and the same findings.
func TestBoundedScanPersistsSameRunsAndFindingsAsDefault(t *testing.T) {
	tracker := newRunTracker()
	fixture := newScanFixture(t, newCountingAnalyzers(tracker, 15*time.Millisecond, true, "one", "two", "three", "four")...)

	defaultScan := fixture.startScan(t)
	if final := fixture.waitForTerminal(t, defaultScan.ID); final.State != "completed" {
		t.Fatalf("default scan state = %q, want completed", final.State)
	}
	defaultStates := scanRunStates(t, fixture, defaultScan.ID)
	defaultFindings := scanFindingCount(t, fixture, defaultScan.ID)

	boundedScan := fixture.startScanWithOptions(t, ScanOptions{Jobs: 2})
	if final := fixture.waitForTerminal(t, boundedScan.ID); final.State != "completed" {
		t.Fatalf("bounded scan state = %q, want completed", final.State)
	}
	boundedStates := scanRunStates(t, fixture, boundedScan.ID)
	if !reflect.DeepEqual(boundedStates, defaultStates) {
		t.Fatalf("bounded run states = %v, want the same as default %v", boundedStates, defaultStates)
	}
	if count := scanFindingCount(t, fixture, boundedScan.ID); count != defaultFindings {
		t.Fatalf("bounded findings = %d, want %d like the default scan", count, defaultFindings)
	}
	for _, id := range []string{"one", "two", "three", "four"} {
		if got := tracker.runCount(id); got != 2 {
			t.Fatalf("analyzer %s ran %d times across two scans, want 2", id, got)
		}
	}
}

// TestCancelWithJobsBoundStopsQueuedAnalyzers cancels a Jobs=1 scan while one
// analyzer holds the single slot: the scan must reach the cancelled terminal
// state promptly and the analyzer queued behind the bound must never start.
func TestCancelWithJobsBoundStopsQueuedAnalyzers(t *testing.T) {
	first := &scriptedAnalyzer{id: "first", runEntered: make(chan struct{}), runGate: make(chan struct{})}
	second := &scriptedAnalyzer{id: "second", runEntered: make(chan struct{}), runGate: make(chan struct{})}
	fixture := newScanFixture(t, first, second)
	scan := fixture.startScanWithOptions(t, ScanOptions{Jobs: 1})
	// Whichever analyzer grabbed the single slot is blocked inside Run; the
	// other one is provably queued behind the semaphore.
	var holder, queued *scriptedAnalyzer
	select {
	case <-first.runEntered:
		holder, queued = first, second
	case <-second.runEntered:
		holder, queued = second, first
	}
	if err := fixture.service.Cancel(context.Background(), scan.ID); err != nil {
		t.Fatal(err)
	}
	close(holder.runGate)
	if final := fixture.waitForTerminal(t, scan.ID); final.State != "cancelled" {
		t.Fatalf("scan state = %q, want cancelled", final.State)
	}
	// The terminal state is only written after every worker returned, so this
	// check is race-free: a queued run must not have started at all.
	select {
	case <-queued.runEntered:
		t.Fatal("analyzer queued behind the jobs bound started despite cancellation")
	default:
	}
}

// TestAnalyzerPanicUnderJobsBoundFailsScanInsteadOfCrashing is the bounded
// counterpart of TestAnalyzerPanicFailsScanInsteadOfCrashing: a panic inside a
// worker goroutine is contained, the scan lands in failed, and a terminal
// event is published.
func TestAnalyzerPanicUnderJobsBoundFailsScanInsteadOfCrashing(t *testing.T) {
	tracker := newRunTracker()
	panicking := &countingAnalyzer{id: "boom", tracker: tracker, panicInRun: true}
	healthy := &countingAnalyzer{id: "fine", tracker: tracker, findings: true}
	fixture := newScanFixture(t, panicking, healthy)
	scan := fixture.startScanWithOptions(t, ScanOptions{Jobs: 2})
	ch, unsubscribe := fixture.bus.Subscribe(scan.ID)
	defer unsubscribe()
	if final := fixture.waitForTerminal(t, scan.ID); final.State != "failed" {
		t.Fatalf("scan state after worker panic = %q, want failed", final.State)
	}
	if got := waitTerminalEvent(t, ch); got != "scan.completed" {
		t.Fatalf("terminal event after worker panic = %q, want scan.completed", got)
	}
}

package main

// Tests for `bluntcode scan --watch`: the change-detection pieces (the
// snapshot builder honoring discovery's excludes, the snapshot diff, and the
// debouncer state machine), the --watch flag wiring (including the
// --format github rejection), and the watch loop itself with a fake scan
// runner, a real temp workspace, and compressed timing constants.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Change detection: snapshots and diffs ----------------------------------

func writeWatchFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildWatchSnapshotHonorsDiscoveryExcludes(t *testing.T) {
	root := t.TempDir()
	writeWatchFile(t, root, "main.py", "x = 1\n")
	writeWatchFile(t, root, "util.ts", "export const x = 1;\n")
	writeWatchFile(t, root, "notes.txt", "not a source file\n")
	writeWatchFile(t, root, "node_modules/bundled.js", "x")
	writeWatchFile(t, root, "dist/out.js", "x")
	writeWatchFile(t, root, "gen/generated.py", "x = 2\n")
	writeWatchFile(t, root, "ignored.py", "x = 3\n")
	writeWatchFile(t, root, ".bluntcodeignore", "ignored.py\n")
	snapshot, err := buildWatchSnapshot(context.Background(), root, []string{"gen/**"})
	if err != nil {
		t.Fatal(err)
	}
	// Only the two source files discovery would select are fingerprinted:
	// default excludes skip node_modules/dist, the user pattern skips gen/**,
	// the ignore file skips ignored.py, and non-source files never appear.
	want := []string{"main.py", "util.ts"}
	got := make([]string, 0, len(snapshot))
	for path := range snapshot {
		got = append(got, path)
	}
	if len(snapshot) != len(want) {
		t.Fatalf("snapshot = %v, want exactly %v", got, want)
	}
	for _, path := range want {
		stat, ok := snapshot[path]
		if !ok {
			t.Fatalf("snapshot missing %q: %v", path, got)
		}
		if stat.Size == 0 {
			t.Errorf("%q fingerprinted with size 0", path)
		}
		if stat.ModTime.IsZero() {
			t.Errorf("%q fingerprinted without modtime", path)
		}
	}
}

func TestDiffWatchSnapshotsSets(t *testing.T) {
	stamp := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	before := watchSnapshot{
		"same.py":    {Size: 10, ModTime: stamp},
		"grew.py":    {Size: 10, ModTime: stamp},
		"touched.py": {Size: 10, ModTime: stamp},
		"gone.py":    {Size: 10, ModTime: stamp},
	}
	after := watchSnapshot{
		"same.py":    {Size: 10, ModTime: stamp},
		"grew.py":    {Size: 42, ModTime: stamp},                // content changed
		"touched.py": {Size: 10, ModTime: stamp.Add(time.Hour)}, // mtime-only change
		"new.py":     {Size: 1, ModTime: stamp},                 // added
	}
	diff := diffWatchSnapshots(before, after)
	if !reflect.DeepEqual(diff.Added, []string{"new.py"}) {
		t.Errorf("Added = %v", diff.Added)
	}
	if !reflect.DeepEqual(diff.Changed, []string{"grew.py", "touched.py"}) {
		t.Errorf("Changed = %v", diff.Changed)
	}
	if !reflect.DeepEqual(diff.Removed, []string{"gone.py"}) {
		t.Errorf("Removed = %v", diff.Removed)
	}
	if diff.Count() != 4 {
		t.Errorf("Count = %d, want 4", diff.Count())
	}
	if !diffWatchSnapshots(before, before).Empty() {
		t.Errorf("identical snapshots must diff empty")
	}
}

func TestDiffWatchSnapshotsFromDiskAddModifyTouchDelete(t *testing.T) {
	root := t.TempDir()
	writeWatchFile(t, root, "main.py", "x = 1\n")
	writeWatchFile(t, root, "keep.ts", "export const a = 1;\n")
	writeWatchFile(t, root, "gone.ts", "export const b = 2;\n")
	before, err := buildWatchSnapshot(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	writeWatchFile(t, root, "main.py", "x = 1 + 41\n") // content change
	writeWatchFile(t, root, "added.py", "y = 2\n")     // new file
	if err := os.Remove(filepath.Join(root, "gone.ts")); err != nil {
		t.Fatal(err)
	}
	// mtime-only touch: identical content, clearly different timestamp.
	touched := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, "keep.ts"), touched, touched); err != nil {
		t.Fatal(err)
	}
	after, err := buildWatchSnapshot(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	diff := diffWatchSnapshots(before, after)
	if !reflect.DeepEqual(diff.Added, []string{"added.py"}) {
		t.Errorf("Added = %v", diff.Added)
	}
	if !reflect.DeepEqual(diff.Changed, []string{"keep.ts", "main.py"}) {
		t.Errorf("Changed = %v (a bare touch must count)", diff.Changed)
	}
	if !reflect.DeepEqual(diff.Removed, []string{"gone.ts"}) {
		t.Errorf("Removed = %v", diff.Removed)
	}
	if diff.Count() != 4 {
		t.Errorf("Count = %d, want 4", diff.Count())
	}
}

// --- Debouncer state machine --------------------------------------------------

func TestWatchDebouncerFiresAfterQuietWindow(t *testing.T) {
	deb := watchDebouncer{quietWindow: 1500 * time.Millisecond}
	start := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	if deb.observe(start, true) {
		t.Fatal("fired on the change itself")
	}
	if deb.observe(start.Add(1*time.Second), false) {
		t.Fatal("fired before the quiet window elapsed")
	}
	if !deb.observe(start.Add(2*time.Second), false) {
		t.Fatal("did not fire after the quiet window elapsed")
	}
	if deb.observe(start.Add(3*time.Second), false) {
		t.Fatal("fired again while idle")
	}
}

func TestWatchDebouncerContinuedChangesPushFireOut(t *testing.T) {
	deb := watchDebouncer{quietWindow: 1500 * time.Millisecond}
	start := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	deb.observe(start, true)
	if deb.observe(start.Add(1400*time.Millisecond), false) {
		t.Fatal("fired too early")
	}
	// A new change inside the window restarts the quiet countdown.
	deb.observe(start.Add(1400*time.Millisecond), true)
	if deb.observe(start.Add(2800*time.Millisecond), false) {
		t.Fatal("fired before the window restarted after a new change")
	}
	if !deb.observe(start.Add(2900*time.Millisecond), false) {
		t.Fatal("did not fire 1.5s after the last change")
	}
}

func TestWatchDebouncerResetSuppressesFire(t *testing.T) {
	deb := watchDebouncer{quietWindow: time.Second}
	start := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	deb.observe(start, true)
	deb.reset() // a started rescan supersedes the pending debounce
	for offset := time.Duration(1); offset <= 5; offset++ {
		if deb.observe(start.Add(offset*time.Second), false) {
			t.Fatalf("fired at +%s after reset without a new change", offset*time.Second)
		}
	}
	deb.observe(start.Add(6*time.Second), true)
	if !deb.observe(start.Add(8*time.Second), false) {
		t.Fatal("did not re-arm after reset and a fresh change")
	}
}

// --- Flag wiring: --watch -----------------------------------------------------

func TestParseScanFlagsWatchFlag(t *testing.T) {
	cases := []struct {
		name         string
		args         []string
		wantWatch    bool
		wantGate     bool
		wantBaseline string
		wantFormat   string
	}{
		{"watch alone", []string{"--watch", `C:\proj`}, true, false, "", ""},
		{"watch with gate flags", []string{"--watch", "--fail-on", "high+", "--max-findings", "5", `C:\proj`}, true, true, "", ""},
		{"watch with text format", []string{"--watch", "--format", "text", `C:\proj`}, true, false, "", "text"},
		{"watch with json format", []string{"--watch", "--format", "json", `C:\proj`}, true, false, "", "json"},
		{"watch with baseline", []string{"--watch", "--baseline", "scan-123", `C:\proj`}, true, false, "scan-123", ""},
		{"watch after the path works", []string{`C:\proj`, "--watch"}, true, false, "", ""},
		{"absent stays one-shot", []string{`C:\proj`}, false, false, "", ""},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			var errOut bytes.Buffer
			cfg, err := parseScanFlags(item.args, &errOut)
			if err != nil {
				t.Fatalf("parse: %v (stderr: %s)", err, errOut.String())
			}
			if cfg.watch != item.wantWatch {
				t.Errorf("watch = %v, want %v", cfg.watch, item.wantWatch)
			}
			if cfg.gate.Enabled() != item.wantGate {
				t.Errorf("gate enabled = %v, want %v", cfg.gate.Enabled(), item.wantGate)
			}
			if cfg.baseline != item.wantBaseline {
				t.Errorf("baseline = %q, want %q", cfg.baseline, item.wantBaseline)
			}
			if cfg.format != item.wantFormat {
				t.Errorf("format = %q, want %q", cfg.format, item.wantFormat)
			}
		})
	}
}

func TestParseScanFlagsRejectsWatchWithGitHubFormat(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		message string
	}{
		{"watch then github", []string{"--watch", "--format", "github", `C:\proj`}, "--watch cannot be combined with --format github"},
		{"github then watch", []string{"--format", "github", "--watch", `C:\proj`}, "--watch cannot be combined with --format github"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			var errOut bytes.Buffer
			_, err := parseScanFlags(item.args, &errOut)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), item.message) {
				t.Fatalf("error = %q, want substring %q", err.Error(), item.message)
			}
			if !strings.Contains(errOut.String(), "usage: bluntcode scan") {
				t.Fatalf("usage not printed: %q", errOut.String())
			}
		})
	}
}

func TestRunScanCommandRejectsWatchWithGitHubFormatExitTwo(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runScanCommand([]string{"--watch", "--format", "github", `C:\proj`}, &out, &errOut); code != 2 {
		t.Fatalf("exit code = %d", code)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q", out.String())
	}
	if !strings.Contains(errOut.String(), "--watch cannot be combined with --format github") {
		t.Fatalf("reason missing: %q", errOut.String())
	}
}

// --- Watch loop integration (fake scan runner, real temp workspace) ----------

// watchFake is a scripted scan runner: it counts invocations and returns the
// scripted result per call. Scripts end the loop by returning interruptSeen.
type watchFake struct {
	mu     sync.Mutex
	calls  int
	script func(call int) scanRunResult
}

func (f *watchFake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *watchFake) run(interrupts <-chan os.Signal) scanRunResult {
	f.mu.Lock()
	f.calls++
	call := f.calls
	f.mu.Unlock()
	return f.script(call)
}

// startWatchLoop runs the loop with compressed timings and returns its exit
// code. Tests always terminate it via an interrupt-flagged scan result or the
// interrupt channel, so the goroutine cannot outlive the test.
func startWatchLoop(t *testing.T, env watchEnv) <-chan int {
	t.Helper()
	result := make(chan int, 1)
	go func() {
		result <- runWatchLoop(env, watchLoopOptions{pollInterval: 5 * time.Millisecond, quietWindow: 2 * time.Millisecond})
	}()
	return result
}

func waitUntil(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func watchTestEnv(t *testing.T, root string, fake *watchFake, interrupt chan os.Signal) (watchEnv, *bytes.Buffer) {
	t.Helper()
	var errOut bytes.Buffer
	var signals <-chan os.Signal
	if interrupt != nil {
		signals = interrupt
	}
	return watchEnv{
		stderr:    &errOut,
		interrupt: signals,
		snapshot:  func() (watchSnapshot, error) { return buildWatchSnapshot(context.Background(), root, nil) },
		runScan:   fake.run,
	}, &errOut
}

// TestWatchLoopRunsFirstScanAndRescansOnChanges covers the core contract: the
// first scan runs immediately, each settled change triggers exactly one
// rescan with a header reporting the files changed since the last scan, and an
// interrupt-flagged scan result exits 130.
func TestWatchLoopRunsFirstScanAndRescansOnChanges(t *testing.T) {
	root := t.TempDir()
	writeWatchFile(t, root, "app.py", "x = 1\n")
	fake := &watchFake{script: func(call int) scanRunResult {
		if call >= 3 {
			return scanRunResult{code: 130, interruptSeen: true}
		}
		return scanRunResult{code: 0}
	}}
	env, errOut := watchTestEnv(t, root, fake, nil)
	result := startWatchLoop(t, env)
	waitUntil(t, "the immediate first scan", func() bool { return fake.count() >= 1 })

	writeWatchFile(t, root, "added.py", "y = 2\n")
	waitUntil(t, "rescan after the first change", func() bool { return fake.count() >= 2 })

	writeWatchFile(t, root, "app.py", "x = 1 + 41\n")
	waitUntil(t, "rescan after the second change", func() bool { return fake.count() >= 3 })

	select {
	case code := <-result:
		if code != 130 {
			t.Fatalf("exit code = %d, want 130", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not exit after the interrupt-flagged scan")
	}
	if got := strings.Count(errOut.String(), "rescan: 1 file(s) changed"); got != 2 {
		t.Fatalf("rescan headers = %d, want 2 (one per change): %q", got, errOut.String())
	}
	if strings.Count(errOut.String(), "rescan:") != 2 {
		t.Fatalf("unexpected rescan headers: %q", errOut.String())
	}
}

// TestWatchLoopQueuesExactlyOneRescanDuringScan pins the queueing rule: a scan
// blocks while several poll ticks observe the same change, and exactly one
// rescan fires when the scan finishes — not one per observing tick.
func TestWatchLoopQueuesExactlyOneRescanDuringScan(t *testing.T) {
	root := t.TempDir()
	writeWatchFile(t, root, "app.py", "x = 1\n")
	release := make(chan struct{})
	fake := &watchFake{script: func(call int) scanRunResult {
		if call == 1 {
			<-release // the first scan stays in flight while changes land
			return scanRunResult{code: 0}
		}
		return scanRunResult{code: 130, interruptSeen: true}
	}}
	env, errOut := watchTestEnv(t, root, fake, nil)
	result := startWatchLoop(t, env)
	waitUntil(t, "the blocked first scan", func() bool { return fake.count() >= 1 })

	writeWatchFile(t, root, "added.py", "y = 2\n")
	time.Sleep(30 * time.Millisecond) // several 5ms ticks observe the change mid-scan
	close(release)
	waitUntil(t, "the queued rescan", func() bool { return fake.count() >= 2 })

	select {
	case code := <-result:
		if code != 130 {
			t.Fatalf("exit code = %d, want 130", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not exit after the interrupt-flagged rescan")
	}
	if fake.count() != 2 {
		t.Fatalf("scan invocations = %d, want exactly 2 (one queued rescan)", fake.count())
	}
	if got := strings.Count(errOut.String(), "rescan: 1 file(s) changed"); got != 1 {
		t.Fatalf("rescan headers = %d, want 1: %q", got, errOut.String())
	}
}

// TestWatchLoopScanFailureDoesNotStopWatching: a failed scan (non-zero result,
// e.g. a start or summary error) is survived — the loop keeps watching and
// still rescans on the next change.
func TestWatchLoopScanFailureDoesNotStopWatching(t *testing.T) {
	root := t.TempDir()
	writeWatchFile(t, root, "app.py", "x = 1\n")
	fake := &watchFake{script: func(call int) scanRunResult {
		if call == 1 {
			return scanRunResult{code: 1}
		}
		return scanRunResult{code: 130, interruptSeen: true}
	}}
	env, _ := watchTestEnv(t, root, fake, nil)
	result := startWatchLoop(t, env)
	waitUntil(t, "the failing first scan", func() bool { return fake.count() >= 1 })
	time.Sleep(15 * time.Millisecond) // let the loop settle into idle watching

	writeWatchFile(t, root, "added.py", "y = 2\n")
	waitUntil(t, "rescan after a change despite the earlier failure", func() bool { return fake.count() >= 2 })

	select {
	case code := <-result:
		if code != 130 {
			t.Fatalf("exit code = %d, want 130", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not exit after the interrupt-flagged rescan")
	}
}

// TestWatchLoopInterruptWhileIdleExits130: between scans a single Ctrl+C
// stops the loop promptly with the historical interrupt exit code.
func TestWatchLoopInterruptWhileIdleExits130(t *testing.T) {
	root := t.TempDir()
	writeWatchFile(t, root, "app.py", "x = 1\n")
	fake := &watchFake{script: func(call int) scanRunResult { return scanRunResult{code: 0} }}
	interrupt := make(chan os.Signal, 1)
	env, _ := watchTestEnv(t, root, fake, interrupt)
	result := startWatchLoop(t, env)
	waitUntil(t, "the first scan", func() bool { return fake.count() >= 1 })
	time.Sleep(20 * time.Millisecond) // the loop is now idle, polling

	interrupt <- os.Interrupt
	select {
	case code := <-result:
		if code != 130 {
			t.Fatalf("exit code = %d, want 130", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("idle interrupt did not stop the loop")
	}
}

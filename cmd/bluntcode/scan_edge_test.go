package main

// Edge-case regression tests for `bluntcode scan`: argument order, validation
// error reporting, the terminal-state event loop (including the Ctrl+C
// double-tap), JSON output across every terminal state, and workspace reuse
// for junction and case-variant paths. All tests run offline: no analyzer
// downloads, no network.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"bluntcode/internal/config"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/events"
	"bluntcode/internal/instance"
	"bluntcode/internal/reports"
)

// --- Flag parsing: order, separators, and validation messages -------------

// TestParseScanFlagsAcceptsFlagsAfterPath pins the documented usage order
// `bluntcode scan <path> --json`: flag.FlagSet stops at the first positional
// argument, so the parser must restart after each collected path.
func TestParseScanFlagsAcceptsFlagsAfterPath(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want scanConfig
	}{
		{
			"flags after path",
			[]string{`C:\my proj`, "--json", "--profile", "quick", "--timeout", "5m"},
			scanConfig{path: `C:\my proj`, profile: "quick", json: true, timeout: 5 * time.Minute},
		},
		{
			"flags before path",
			[]string{"--json", "--profile", "quick", "--timeout", "5m", `C:\my proj`},
			scanConfig{path: `C:\my proj`, profile: "quick", json: true, timeout: 5 * time.Minute},
		},
		{
			"flags around path",
			[]string{"--quiet", `C:\my proj`, "--json"},
			scanConfig{path: `C:\my proj`, profile: "standard", json: true, timeout: scanDefaultTimeout, quiet: true},
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			var errOut bytes.Buffer
			cfg, err := parseScanFlags(item.args, &errOut)
			if err != nil {
				t.Fatalf("parse: %v (stderr: %s)", err, errOut.String())
			}
			if cfg != item.want {
				t.Fatalf("cfg = %#v, want %#v", cfg, item.want)
			}
		})
	}
}

func TestParseScanFlagsAcceptsDoubleDashSeparator(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseScanFlags([]string{"--quiet", "--", `C:\odd dir`}, &errOut)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.path != `C:\odd dir` || !cfg.quiet {
		t.Fatalf("cfg = %#v", cfg)
	}
}

func TestParseScanFlagsHugeButValidTimeoutAccepted(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseScanFlags([]string{"--timeout", "99999h", `C:\proj`}, &errOut)
	if err != nil {
		t.Fatalf("99999h rejected: %v", err)
	}
	if cfg.timeout != 99999*time.Hour {
		t.Fatalf("timeout = %s", cfg.timeout)
	}
}

func TestParseScanFlagsRejectsOverflowingTimeout(t *testing.T) {
	var errOut bytes.Buffer
	if _, err := parseScanFlags([]string{"--timeout", "3000000000h", `C:\proj`}, &errOut); err == nil {
		t.Fatal("overflowing duration accepted")
	}
	if !strings.Contains(errOut.String(), "usage: bluntcode scan") {
		t.Fatalf("usage not printed: %q", errOut.String())
	}
}

// TestParseScanFlagsPrintsReason pins the fix for swallowed validation
// errors: the specific reason must reach stderr, not just the usage block.
func TestParseScanFlagsPrintsReason(t *testing.T) {
	cases := []struct {
		args    []string
		message string
	}{
		{nil, "exactly one workspace path is required"},
		{[]string{`C:\one`, `C:\two`}, "exactly one workspace path is required"},
		{[]string{"--timeout", "0", `C:\proj`}, "timeout must be a positive duration"},
		{[]string{"--timeout", "-5m", `C:\proj`}, "timeout must be a positive duration"},
		{[]string{"--profile", "thorough", `C:\proj`}, "profile must be quick, standard, or deep"},
	}
	for _, item := range cases {
		var errOut bytes.Buffer
		_, err := parseScanFlags(item.args, &errOut)
		if err == nil {
			t.Fatalf("%v: expected error", item.args)
		}
		if !strings.Contains(errOut.String(), item.message) {
			t.Errorf("%v: reason %q missing from stderr: %q", item.args, item.message, errOut.String())
		}
		if !strings.Contains(errOut.String(), "usage: bluntcode scan") {
			t.Errorf("%v: usage not printed: %q", item.args, errOut.String())
		}
	}
}

// TestRunScanCommandBadFlagExitAndMessage verifies end to end that a bad
// invocation exits 2 with the reason on stderr and nothing on stdout.
func TestRunScanCommandBadFlagExitAndMessage(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runScanCommand([]string{`C:\one`, `C:\two`}, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit code = %d", code)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q", out.String())
	}
	if !strings.Contains(errOut.String(), "exactly one workspace path is required") {
		t.Fatalf("reason missing: %q", errOut.String())
	}
}

// --- The terminal-state event loop -----------------------------------------

// awaitInput builds a deterministic scanWaitInput. Nil channels never fire,
// so tests enable only the inputs they exercise.
func awaitInput(eventCh <-chan events.Event) scanWaitInput {
	return scanWaitInput{
		events:       eventCh,
		interrupts:   nil,
		timeout:      nil,
		poll:         func() (string, bool) { return "running", false },
		cancel:       func() {},
		timeoutLimit: time.Minute,
		quiet:        false,
		errOut:       io.Discard,
		pollEvery:    time.Hour,
		cancelGrace:  time.Hour,
	}
}

func TestAwaitScanTerminalCompletedEvents(t *testing.T) {
	eventCh := make(chan events.Event, 4)
	eventCh <- events.Event{Type: "analyzer.failed", Data: map[string]any{"analyzer_id": "ruff", "error": "offline"}}
	eventCh <- events.Event{Type: "scan.completed", Data: map[string]any{"state": "completed_with_warnings"}}
	outcome := awaitScanTerminal(awaitInput(eventCh))
	if outcome.finalState != "completed_with_warnings" {
		t.Fatalf("finalState = %q", outcome.finalState)
	}
	if outcome.interruptSeen || outcome.timedOut || outcome.exitNow {
		t.Fatalf("outcome = %+v", outcome)
	}

	eventCh = make(chan events.Event, 1)
	eventCh <- events.Event{Type: "scan.completed"}
	outcome = awaitScanTerminal(awaitInput(eventCh))
	if outcome.finalState != "completed" {
		t.Fatalf("default finalState = %q", outcome.finalState)
	}

	eventCh = make(chan events.Event, 1)
	eventCh <- events.Event{Type: "scan.cancelled"}
	outcome = awaitScanTerminal(awaitInput(eventCh))
	if outcome.finalState != "cancelled" {
		t.Fatalf("cancelled finalState = %q", outcome.finalState)
	}
}

// TestAwaitScanTerminalCompletedEventRendersProgress checks that progress
// events reaching the loop are printed (non-quiet) through the same seam.
func TestAwaitScanTerminalCompletedEventRendersProgress(t *testing.T) {
	eventCh := make(chan events.Event, 2)
	eventCh <- events.Event{Type: "analyzer.failed", Data: map[string]any{"analyzer_id": "ruff", "error": "not installed"}}
	eventCh <- events.Event{Type: "scan.completed", Data: map[string]any{"state": "failed"}}
	in := awaitInput(eventCh)
	var errOut bytes.Buffer
	in.errOut = &errOut
	awaitScanTerminal(in)
	if !strings.Contains(errOut.String(), "[ruff] FAILED: not installed") {
		t.Fatalf("progress not rendered: %q", errOut.String())
	}
}

func TestAwaitScanTerminalPollFallback(t *testing.T) {
	in := awaitInput(nil)
	in.pollEvery = time.Millisecond
	in.poll = func() (string, bool) { return "failed", true }
	outcome := awaitScanTerminal(in)
	if outcome.finalState != "failed" {
		t.Fatalf("finalState = %q", outcome.finalState)
	}
}

// TestAwaitScanTerminalTimeoutCancels uses a closed (permanently ready)
// timeout channel plus grace expiry so the timeout branch must run before any
// terminal state, deterministically.
func TestAwaitScanTerminalTimeoutCancels(t *testing.T) {
	in := awaitInput(nil) // no event delivery: only the timeout path can finish
	timeout := make(chan time.Time)
	close(timeout) // fires immediately and stays ready; must still cancel once
	in.timeout = timeout
	in.cancelGrace = 5 * time.Millisecond
	cancels := 0
	in.cancel = func() { cancels++ }
	var errOut bytes.Buffer
	in.errOut = &errOut
	outcome := awaitScanTerminal(in)
	if outcome.timedOut != true || outcome.finalState != "cancelled" {
		t.Fatalf("outcome = %+v", outcome)
	}
	if cancels != 1 {
		t.Fatalf("cancel called %d times", cancels)
	}
	if !strings.Contains(errOut.String(), "timeout after 1m0s: cancelling scan") {
		t.Fatalf("timeout message missing: %q", errOut.String())
	}

	// Quiet mode must suppress the timeout progress line.
	in2 := awaitInput(nil)
	timeout2 := make(chan time.Time)
	close(timeout2)
	in2.timeout = timeout2
	in2.cancelGrace = 5 * time.Millisecond
	in2.quiet = true
	var quietOut bytes.Buffer
	in2.errOut = &quietOut
	awaitScanTerminal(in2)
	if quietOut.Len() != 0 {
		t.Fatalf("quiet mode wrote to stderr: %q", quietOut.String())
	}
}

func TestAwaitScanTerminalTimeoutGraceExpiry(t *testing.T) {
	in := awaitInput(nil)
	timeout := make(chan time.Time)
	close(timeout)
	in.timeout = timeout
	in.cancelGrace = 5 * time.Millisecond
	outcome := awaitScanTerminal(in)
	if outcome.finalState != "cancelled" || !outcome.timedOut {
		t.Fatalf("outcome = %+v", outcome)
	}
}

// TestAwaitScanTerminalSingleInterrupt sequences the interrupt before the
// terminal event so the select never races between two ready channels.
func TestAwaitScanTerminalSingleInterrupt(t *testing.T) {
	eventCh := make(chan events.Event) // unbuffered: delivery is test-driven
	in := awaitInput(eventCh)
	interrupts := make(chan os.Signal, 1)
	in.interrupts = interrupts
	cancels := 0
	cancelCalled := make(chan struct{})
	in.cancel = func() { cancels++; close(cancelCalled) }
	interrupts <- os.Interrupt
	done := make(chan scanWaitOutcome, 1)
	go func() { done <- awaitScanTerminal(in) }()
	select {
	case <-cancelCalled:
	case <-time.After(5 * time.Second):
		t.Fatal("first interrupt did not request cancellation")
	}
	eventCh <- events.Event{Type: "scan.cancelled"}
	select {
	case outcome := <-done:
		if !outcome.interruptSeen || outcome.exitNow || cancels != 1 {
			t.Fatalf("outcome = %+v cancels = %d", outcome, cancels)
		}
		if outcome.finalState != "cancelled" {
			t.Fatalf("finalState = %q", outcome.finalState)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("terminal event did not finish the wait loop")
	}
}

// TestAwaitScanTerminalDoubleInterruptExitsImmediately pins the Ctrl+C
// double-tap: the second press must exit immediately (exitNow) without a
// terminal scan state, while both presses are consumed from a buffer of two.
func TestAwaitScanTerminalDoubleInterruptExitsImmediately(t *testing.T) {
	in := awaitInput(nil)
	interrupts := make(chan os.Signal, 2)
	in.interrupts = interrupts
	cancels := 0
	in.cancel = func() { cancels++ }
	interrupts <- os.Interrupt
	interrupts <- os.Interrupt
	var errOut bytes.Buffer
	in.errOut = &errOut
	done := make(chan scanWaitOutcome, 1)
	go func() { done <- awaitScanTerminal(in) }()
	select {
	case outcome := <-done:
		if !outcome.exitNow {
			t.Fatalf("second interrupt did not exit immediately: %+v", outcome)
		}
		if outcome.interruptSeen != true || cancels != 1 {
			t.Fatalf("outcome = %+v cancels = %d", outcome, cancels)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("double interrupt did not terminate the wait loop")
	}
	if strings.Count(errOut.String(), "interrupt received") != 2 {
		t.Fatalf("interrupt lines = %q", errOut.String())
	}
}

// TestAwaitScanTerminalInterruptAfterTimeoutExitsImmediately: a timeout that
// already requested cancellation turns the next Ctrl+C into an immediate exit.
// The timeout is observed first (via the cancel callback) so the select never
// races the two ready channels.
func TestAwaitScanTerminalInterruptAfterTimeoutExitsImmediately(t *testing.T) {
	in := awaitInput(nil)
	timeout := make(chan time.Time)
	close(timeout)
	in.timeout = timeout
	in.cancelGrace = time.Hour // grace must NOT expire first
	cancelCalled := make(chan struct{})
	in.cancel = func() { close(cancelCalled) }
	interrupts := make(chan os.Signal, 1)
	in.interrupts = interrupts
	done := make(chan scanWaitOutcome, 1)
	go func() { done <- awaitScanTerminal(in) }()
	select {
	case <-cancelCalled:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout did not request cancellation")
	}
	interrupts <- os.Interrupt
	select {
	case outcome := <-done:
		if !outcome.exitNow {
			t.Fatalf("interrupt after timeout did not exit: %+v", outcome)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("interrupt after timeout did not terminate the wait loop")
	}
}

func TestAwaitScanTerminalQuietSuppressesInterruptLine(t *testing.T) {
	eventCh := make(chan events.Event) // unbuffered: delivery is test-driven
	in := awaitInput(eventCh)
	in.quiet = true
	interrupts := make(chan os.Signal, 1)
	in.interrupts = interrupts
	var errOut bytes.Buffer
	in.errOut = &errOut
	cancelCalled := make(chan struct{})
	in.cancel = func() { close(cancelCalled) }
	interrupts <- os.Interrupt
	done := make(chan scanWaitOutcome, 1)
	go func() { done <- awaitScanTerminal(in) }()
	select {
	case <-cancelCalled:
	case <-time.After(5 * time.Second):
		t.Fatal("interrupt did not request cancellation")
	}
	eventCh <- events.Event{Type: "scan.cancelled"}
	<-done
	if errOut.Len() != 0 {
		t.Fatalf("quiet mode wrote to stderr: %q", errOut.String())
	}
}

// --- JSON output across every terminal state --------------------------------

// TestWriteScanJSONAllTerminalStates parses the JSON output for every terminal
// state and asserts the invariants CI consumers depend on: valid JSON (which
// encoding/json refuses to produce for NaN/Inf), RFC3339 timestamps, and an
// always-present severity object.
func TestWriteScanJSONAllTerminalStates(t *testing.T) {
	for _, state := range []string{"completed", "completed_with_warnings", "failed", "cancelled", "interrupted"} {
		t.Run(state, func(t *testing.T) {
			s := sampleScanSummary()
			s.state = state
			var out bytes.Buffer
			if err := writeScanJSON(&out, s); err != nil {
				t.Fatalf("writeScanJSON: %v", err)
			}
			raw := out.String()
			for _, banned := range []string{"NaN", "Infinity", "-Infinity"} {
				if strings.Contains(raw, banned) {
					t.Fatalf("%s in JSON output: %s", banned, raw)
				}
			}
			var decoded map[string]any
			if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
				t.Fatalf("invalid JSON: %v\n%s", err, raw)
			}
			if decoded["state"] != state {
				t.Fatalf("state = %v", decoded["state"])
			}
			severity, ok := decoded["severity"].(map[string]any)
			if !ok {
				t.Fatalf("severity object missing: %s", raw)
			}
			for _, key := range []string{"critical", "high", "medium", "low", "info", "total"} {
				if _, ok := severity[key]; !ok {
					t.Fatalf("severity.%s missing: %s", key, raw)
				}
			}
			var typed scanResultJSON
			if err := json.Unmarshal([]byte(raw), &typed); err != nil {
				t.Fatalf("decode typed: %v", err)
			}
			if _, err := time.Parse(time.RFC3339, typed.StartedAt); err != nil {
				t.Fatalf("started_at %q not RFC3339: %v", typed.StartedAt, err)
			}
			if _, err := time.Parse(time.RFC3339, typed.FinishedAt); err != nil {
				t.Fatalf("finished_at %q not RFC3339: %v", typed.FinishedAt, err)
			}
			if typed.Severity.Total != typed.TotalFindings {
				t.Fatalf("severity.total %d != total_findings %d", typed.Severity.Total, typed.TotalFindings)
			}
		})
	}
}

// TestWriteScanJSONEmptyRunListAndZeroTimes pins the empty-state shape: no
// null arrays and empty strings (never zero-time noise) for missing stamps.
func TestWriteScanJSONEmptyRunListAndZeroTimes(t *testing.T) {
	s := sampleScanSummary()
	s.state = "interrupted"
	s.runs = nil
	s.startedAt = time.Time{}
	s.finishedAt = time.Time{}
	s.reportPath = ""
	var out bytes.Buffer
	if err := writeScanJSON(&out, s); err != nil {
		t.Fatal(err)
	}
	var decoded scanResultJSON
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if decoded.StartedAt != "" || decoded.FinishedAt != "" {
		t.Fatalf("zero times must emit empty strings: %+v", decoded)
	}
	if decoded.Analyzers == nil || len(decoded.Analyzers) != 0 {
		t.Fatalf("analyzers must serialize as [] not null: %#v", decoded.Analyzers)
	}
	if decoded.Severity.Total != 21 {
		t.Fatalf("severity.total = %d", decoded.Severity.Total)
	}
}

// --- Junction and case-insensitive workspace reuse --------------------------

// makeJunction creates an NTFS junction (no admin rights required) and skips
// the test when the platform or filesystem cannot.
func makeJunction(t *testing.T, link, target string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("junctions are a Windows filesystem feature")
	}
	if err := exec.Command("cmd", "/c", "mklink", "/J", link, target).Run(); err != nil {
		t.Skipf("cannot create junction: %v", err)
	}
}

func TestResolveJunctionsIdentityAndMissing(t *testing.T) {
	dir := t.TempDir()
	if got := resolveJunctions(dir); got != dir {
		t.Fatalf("plain dir changed: %q -> %q", dir, got)
	}
	missing := filepath.Join(dir, "does", "not", "exist")
	if got := resolveJunctions(missing); got != missing {
		t.Fatalf("missing dir changed: %q -> %q", missing, got)
	}
	if got := resolveJunctions(""); got != "" {
		t.Fatalf("empty changed: %q", got)
	}
}

func TestResolveJunctionsResolvesJunction(t *testing.T) {
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(target, "pkg"), 0o700); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	link := filepath.Join(base, "alias")
	makeJunction(t, link, target)
	if got := resolveJunctions(link); got != target {
		t.Fatalf("junction resolved to %q, want %q", got, target)
	}
	nested := filepath.Join(link, "pkg")
	if got := resolveJunctions(nested); got != filepath.Join(target, "pkg") {
		t.Fatalf("nested junction resolved to %q", got)
	}
}

func openTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "ws.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestEnsureWorkspaceReusesSameRoot(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()
	first, err := ensureWorkspace(ctx, db, dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ensureWorkspace(ctx, db, dir)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("same path created a second workspace: %s vs %s", first.ID, second.ID)
	}
	all, err := db.Workspaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("workspace rows = %d, want 1", len(all))
	}
}

// TestEnsureWorkspaceReusesCaseVariant pins Windows case-insensitive lookup:
// a stored root spelled with different case must be reused, not duplicated.
func TestEnsureWorkspaceReusesCaseVariant(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive paths are a Windows behavior")
	}
	db := openTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()
	// Seed a row whose stored root differs in case from the on-disk spelling
	// (the residual case filepath.EvalSymlinks does not unify).
	seeded, err := db.CreateWorkspace(ctx, core.Workspace{Name: "cased", RootPath: strings.ToLower(dir)})
	if err != nil {
		t.Fatal(err)
	}
	reused, err := ensureWorkspace(ctx, db, dir)
	if err != nil {
		t.Fatal(err)
	}
	if reused.ID != seeded.ID {
		t.Fatalf("case variant created a second workspace: %s vs %s", reused.ID, seeded.ID)
	}
}

// TestEnsureWorkspaceReusesJunctionTarget pins junction-aware reuse: scanning
// through a junction must reuse the target's workspace row.
func TestEnsureWorkspaceReusesJunctionTarget(t *testing.T) {
	target := t.TempDir()
	base := t.TempDir()
	link := filepath.Join(base, "alias")
	makeJunction(t, link, target)
	db := openTestDB(t)
	ctx := context.Background()
	direct, err := ensureWorkspace(ctx, db, target)
	if err != nil {
		t.Fatal(err)
	}
	viaJunction, err := ensureWorkspace(ctx, db, link)
	if err != nil {
		t.Fatal(err)
	}
	if viaJunction.ID != direct.ID {
		t.Fatalf("junction created a second workspace: %s vs %s", viaJunction.ID, direct.ID)
	}
	if viaJunction.RootPath != direct.RootPath {
		t.Fatalf("junction workspace root %q != target root %q", viaJunction.RootPath, direct.RootPath)
	}
}

// --- Full command runs (isolated data directory, offline mode) --------------

// isolateDataDir points LOCALAPPDATA at a fresh directory, seeds offline mode,
// and returns the data dir path, so runScanCommand never touches the real user
// data directory, contends for the real instance lock, or downloads tools.
func isolateDataDir(t *testing.T) config.Paths {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("openCore requires managed SonarQube (Windows-only)")
	}
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	paths, err := config.NewPaths(filepath.Join(base, "BluntCode"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAppSettings(context.Background(), database.AppSettings{Offline: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return paths
}

// sampleWorkspace writes a small Python/TypeScript workspace and returns it.
func sampleWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.py"), []byte("import os\nx = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "util.ts"), []byte("export const x: number = 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestRunScanCommandOfflineMissingToolsFailsHonestly covers the offline
// no-tools matrix end to end: every analyzer fails cleanly with an offline
// message, the scan state is failed, the exit code is 1 (never a silent 0),
// and the JSON summary reflects it. It also exercises flags-after-path
// ordering and quiet stderr.
func TestRunScanCommandOfflineMissingToolsFailsHonestly(t *testing.T) {
	paths := isolateDataDir(t)
	ws := sampleWorkspace(t)
	var out, errOut bytes.Buffer
	code := runScanCommand([]string{ws, "--json", "--quiet"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (offline scan must not exit 0)", code)
	}
	if errOut.Len() != 0 {
		t.Fatalf("quiet mode wrote to stderr: %q", errOut.String())
	}
	var result scanResultJSON
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON summary: %v\n%s", err, out.String())
	}
	if result.State != "failed" {
		t.Fatalf("state = %q, want failed (no analyzer can succeed offline)", result.State)
	}
	if result.TimedOut {
		t.Fatalf("timed_out = true")
	}
	if len(result.Analyzers) == 0 {
		t.Fatalf("analyzer runs missing from summary")
	}
	for _, run := range result.Analyzers {
		if run.State == "succeeded" {
			t.Fatalf("analyzer %s succeeded offline", run.ID)
		}
		if !strings.Contains(run.Error, "offline mode is enabled") {
			t.Fatalf("analyzer %s error = %q, want offline explanation", run.ID, run.Error)
		}
	}
	if result.Severity.Total != result.TotalFindings {
		t.Fatalf("severity.total %d != total_findings %d", result.Severity.Total, result.TotalFindings)
	}

	// Re-scanning the same workspace (uppercased path) must reuse it.
	var out2, errOut2 bytes.Buffer
	code = runScanCommand([]string{"--quiet", "--json", strings.ToUpper(ws)}, &out2, &errOut2)
	if code != 1 {
		t.Fatalf("second scan exit code = %d", code)
	}
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	all, err := db.Workspaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("workspace rows = %d, want 1 (reuse across case variants)", len(all))
	}
}

// TestRunScanCommandInvalidPathsAreClean covers non-existent and file paths:
// exit 1 with a one-line message, no panic, no stack, nothing on stdout.
func TestRunScanCommandInvalidPathsAreClean(t *testing.T) {
	isolateDataDir(t)
	var out, errOut bytes.Buffer
	code := runScanCommand([]string{filepath.Join(t.TempDir(), "missing")}, &out, &errOut)
	if code != 1 {
		t.Fatalf("non-existent path exit code = %d", code)
	}
	if out.Len() != 0 || !strings.Contains(errOut.String(), "bluntcode scan: workspace path") {
		t.Fatalf("non-existent path output = %q / %q", out.String(), errOut.String())
	}

	var out2, errOut2 bytes.Buffer
	file := filepath.Join(t.TempDir(), "file.py")
	if err := os.WriteFile(file, []byte("x=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code = runScanCommand([]string{file}, &out2, &errOut2)
	if code != 1 {
		t.Fatalf("file path exit code = %d", code)
	}
	if !strings.Contains(errOut2.String(), "workspace path must be a directory") {
		t.Fatalf("file path message = %q", errOut2.String())
	}
}

// TestRunScanCommandInstanceLockContended pins the single-instance error path:
// with the data-directory lock held by another process-ish holder, the scan
// exits 1 with the dedicated message.
func TestRunScanCommandInstanceLockContended(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("named-mutex lock is Windows-only")
	}
	paths := isolateDataDir(t)
	guard, err := instance.Acquire(paths.DataDir)
	if err != nil {
		t.Fatalf("acquire lock for test: %v", err)
	}
	defer guard.Close()
	var out, errOut bytes.Buffer
	code := runScanCommand([]string{sampleWorkspace(t), "--quiet"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("contended lock exit code = %d", code)
	}
	if !strings.Contains(errOut.String(), "another Blunt Code process") {
		t.Fatalf("lock message = %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q", out.String())
	}
}

// --- Misc: duration formatting and interrupted human summary ----------------

func TestWriteScanHumanInterruptedState(t *testing.T) {
	s := sampleScanSummary()
	s.state = "interrupted"
	s.errorSummary = "Blunt Code stopped before this scan finished."
	s.runs = []reports.Run{}
	var out bytes.Buffer
	writeScanHuman(&out, s)
	if !strings.Contains(out.String(), "interrupted in") {
		t.Fatalf("interrupted label missing: %q", out.String())
	}
	if !strings.Contains(out.String(), "Error: Blunt Code stopped") {
		t.Fatalf("error line missing: %q", out.String())
	}
}

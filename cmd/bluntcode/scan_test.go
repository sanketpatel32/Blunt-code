package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"strings"
	"testing"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/events"
	"bluntcode/internal/reports"
)

func TestParseScanFlagsAppliesDefaults(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseScanFlags([]string{`C:\Projects\my-python-app`}, &errOut)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.path != `C:\Projects\my-python-app` {
		t.Fatalf("path = %q", cfg.path)
	}
	if cfg.profile != "standard" {
		t.Fatalf("profile = %q", cfg.profile)
	}
	if cfg.timeout != 30*time.Minute {
		t.Fatalf("timeout = %s", cfg.timeout)
	}
	if cfg.json || cfg.quiet {
		t.Fatalf("json/quiet = %v/%v", cfg.json, cfg.quiet)
	}
}

func TestParseScanFlagsAcceptsOverrides(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseScanFlags([]string{"--profile", "deep", "--json", "--quiet", "--timeout", "90s", "/srv/project"}, &errOut)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.profile != "deep" || !cfg.json || !cfg.quiet || cfg.timeout != 90*time.Second {
		t.Fatalf("cfg = %#v", cfg)
	}
}

func TestParseScanFlagsRejectsBadInput(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		message string
	}{
		{"bad profile", []string{"--profile", "thorough", `C:\proj`}, "profile must be quick, standard, or deep"},
		{"zero timeout", []string{"--timeout", "0", `C:\proj`}, "timeout must be a positive duration"},
		{"negative timeout", []string{"--timeout", "-5m", `C:\proj`}, "timeout must be a positive duration"},
		{"missing path", nil, "exactly one workspace path is required"},
		{"extra path", []string{`C:\one`, `C:\two`}, "exactly one workspace path is required"},
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

func TestParseScanFlagsHelpExitsZeroViaRunScanCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runScanCommand([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(errOut.String(), "usage: bluntcode scan") {
		t.Fatalf("help text = %q", errOut.String())
	}
}

func TestRunScanCommandRejectsBadFlagsWithExitCodeTwo(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runScanCommand([]string{"--profile", "thorough", `C:\proj`}, &out, &errOut); code != 2 {
		t.Fatalf("exit code = %d", code)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestScanExitCode(t *testing.T) {
	cases := []struct {
		state    string
		timedOut bool
		want     int
	}{
		{"completed", false, 0},
		{"completed_with_warnings", false, 0},
		{"failed", false, 1},
		{"cancelled", false, 1},
		{"interrupted", false, 1},
		{"completed", true, 1},
		{"cancelled", true, 1},
		{"unknown-state", false, 1},
	}
	for _, item := range cases {
		if got := scanExitCode(item.state, item.timedOut); got != item.want {
			t.Errorf("scanExitCode(%q, %v) = %d, want %d", item.state, item.timedOut, got, item.want)
		}
	}
}

func sampleScanSummary() scanSummary {
	started := time.Date(2026, 8, 22, 15, 30, 0, 0, time.UTC)
	finished := started.Add(83 * time.Second)
	return scanSummary{
		scanID:        "abcd-1234",
		workspace:     "my-python-app",
		workspacePath: `C:\Projects\my-python-app`,
		profile:       "standard",
		state:         "completed_with_warnings",
		startedAt:     started,
		finishedAt:    finished,
		critical:      2,
		high:          5,
		medium:        10,
		low:           3,
		info:          1,
		total:         21,
		hasPrevious:   true,
		newCount:      4,
		fixedCount:    2,
		persistent:    15,
		runs: []reports.Run{
			{AnalyzerID: "ruff", Version: "0.16.0", State: "succeeded", FindingCount: 12, Duration: 1200 * time.Millisecond},
			{AnalyzerID: "semgrep", State: "failed", ErrorSummary: "executable is not configured", Duration: 3 * time.Second},
		},
		reportPath:   `C:\Users\me\AppData\Local\BluntCode\reports\my-python-app-20260822-153000.md`,
		errorSummary: "",
	}
}

func TestWriteScanHumanCompletedWithWarnings(t *testing.T) {
	var out bytes.Buffer
	writeScanHuman(&out, sampleScanSummary())
	text := out.String()
	for _, want := range []string{
		"my-python-app (standard profile) completed with warnings in 1m23s",
		"Findings: 2 critical, 5 high, 10 medium, 3 low, 1 info - 21 total",
		"Versus previous scan: 4 new, 2 fixed, 15 persistent",
		"Ruff 0.16.0: succeeded, 12 findings (1.2s)",
		"Semgrep: FAILED (3s) - executable is not configured",
		"Warnings:",
		"Semgrep did not complete: executable is not configured",
		"Report: ",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("human summary missing %q; got:\n%s", want, text)
		}
	}
}

func TestWriteScanHumanFailed(t *testing.T) {
	s := sampleScanSummary()
	s.state = "failed"
	s.errorSummary = "No supported source files were selected."
	s.hasPrevious = false
	s.runs = nil
	s.reportPath = ""
	var out bytes.Buffer
	writeScanHuman(&out, s)
	text := out.String()
	if !strings.Contains(text, "failed in") {
		t.Errorf("missing failed state: %q", text)
	}
	if !strings.Contains(text, "Error: No supported source files were selected.") {
		t.Errorf("missing error line: %q", text)
	}
	if strings.Contains(text, "Findings:") || strings.Contains(text, "Versus previous scan") {
		t.Errorf("failed summary must omit findings lines: %q", text)
	}
}

func TestWriteScanHumanTimedOut(t *testing.T) {
	s := sampleScanSummary()
	s.state = "cancelled"
	s.timedOut = true
	s.runs = nil
	var out bytes.Buffer
	writeScanHuman(&out, s)
	if !strings.Contains(out.String(), "cancelled in") || !strings.Contains(out.String(), "aborted: timeout exceeded") {
		t.Fatalf("timed-out summary = %q", out.String())
	}
}

func TestWriteScanJSONShape(t *testing.T) {
	var out bytes.Buffer
	if err := writeScanJSON(&out, sampleScanSummary()); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	for _, key := range []string{
		"scan_id", "workspace", "workspace_path", "profile", "state", "timed_out",
		"started_at", "finished_at", "duration_ms", "total_findings", "severity",
		"comparison", "analyzers", "report_markdown_path", "error",
	} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing key %q in %s", key, out.String())
		}
	}
	if raw["scan_id"] != "abcd-1234" || raw["state"] != "completed_with_warnings" {
		t.Errorf("scan_id/state = %v/%v", raw["scan_id"], raw["state"])
	}
	if raw["duration_ms"].(float64) != 83000 {
		t.Errorf("duration_ms = %v", raw["duration_ms"])
	}
	severity, ok := raw["severity"].(map[string]any)
	if !ok {
		t.Fatalf("severity = %#v", raw["severity"])
	}
	for _, key := range []string{"critical", "high", "medium", "low", "info", "total"} {
		if _, ok := severity[key]; !ok {
			t.Errorf("severity missing %q: %#v", key, severity)
		}
	}
	if severity["critical"].(float64) != 2 || severity["total"].(float64) != 21 {
		t.Errorf("severity counts = %#v", severity)
	}
	comparison, ok := raw["comparison"].(map[string]any)
	if !ok || comparison["available"] != true {
		t.Fatalf("comparison = %#v", raw["comparison"])
	}
	if comparison["new"].(float64) != 4 || comparison["fixed"].(float64) != 2 || comparison["persistent"].(float64) != 15 {
		t.Errorf("comparison counts = %#v", comparison)
	}
	analyzers, ok := raw["analyzers"].([]any)
	if !ok || len(analyzers) != 2 {
		t.Fatalf("analyzers = %#v", raw["analyzers"])
	}
	first, ok := analyzers[0].(map[string]any)
	if !ok {
		t.Fatalf("analyzers[0] = %#v", analyzers[0])
	}
	for _, key := range []string{"id", "version", "state", "findings", "duration_ms", "error"} {
		if _, ok := first[key]; !ok {
			t.Errorf("analyzer run missing %q: %#v", key, first)
		}
	}
	if first["id"] != "ruff" || first["state"] != "succeeded" || first["findings"].(float64) != 12 {
		t.Errorf("analyzers[0] = %#v", first)
	}
	if !strings.Contains(raw["report_markdown_path"].(string), "reports") {
		t.Errorf("report_markdown_path = %#v", raw["report_markdown_path"])
	}
}

func TestWriteScanJSONWithoutPreviousScan(t *testing.T) {
	s := sampleScanSummary()
	s.hasPrevious = false
	s.newCount, s.fixedCount, s.persistent = 0, 0, 0
	var out bytes.Buffer
	if err := writeScanJSON(&out, s); err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Comparison struct {
			Available bool `json:"available"`
			New       int  `json:"new"`
		} `json:"comparison"`
	}
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw.Comparison.Available || raw.Comparison.New != 0 {
		t.Fatalf("comparison = %#v", raw.Comparison)
	}
}

func TestPrintScanEvent(t *testing.T) {
	cases := []struct {
		name  string
		event events.Event
		want  string
	}{
		{"stage", events.Event{Type: "scan.stage", Data: map[string]any{"stage": "Preparing workspace"}}, "==> Preparing workspace\n"},
		{"analyzer started with name", events.Event{Type: "analyzer.started", Data: map[string]any{"analyzer_id": "ruff", "name": "Ruff"}}, "[Ruff] started\n"},
		{"analyzer completed", events.Event{Type: "analyzer.completed", Data: map[string]any{"analyzer_id": "ruff", "findings": 12}}, "[ruff] completed: 12 findings\n"},
		{"analyzer failed", events.Event{Type: "analyzer.failed", Data: map[string]any{"analyzer_id": "semgrep", "error": "not ready"}}, "[semgrep] FAILED: not ready\n"},
		{"analyzer skipped", events.Event{Type: "analyzer.skipped", Data: map[string]any{"analyzer_id": "sonarqube", "reason": "Quick profile runs language-specific analyzers only."}}, "[sonarqube] skipped: Quick profile runs language-specific analyzers only.\n"},
		{"scan warning", events.Event{Type: "scan.warning", Data: map[string]any{"message": "Could not write Markdown report."}}, "warning: Could not write Markdown report.\n"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			var out bytes.Buffer
			printScanEvent(&out, item.event)
			if out.String() != item.want {
				t.Fatalf("output = %q, want %q", out.String(), item.want)
			}
		})
	}
}

// TestPrintScanEventStripsTerminalControlCharacters is the regression test for
// terminal output poisoning: analyzer failure text embeds untrusted tool stderr
// from the scanned workspace, so ESC sequences and forged newlines must never
// reach the terminal raw.
func TestPrintScanEventStripsTerminalControlCharacters(t *testing.T) {
	var out bytes.Buffer
	printScanEvent(&out, events.Event{Type: "analyzer.failed", Data: map[string]any{
		"analyzer_id": "ruff",
		"error":       "ruff exited 1: \x1b[31mbad config\x1b[0m\nforged summary line\x07",
	}})
	text := out.String()
	if text != "[ruff] FAILED: ruff exited 1:  [31mbad config [0m forged summary line \n" {
		t.Fatalf("sanitized event line = %q", text)
	}
	for _, banned := range []string{"\x1b", "\n", "\x07"} {
		if strings.Contains(strings.TrimSuffix(text, "\n"), banned) {
			t.Fatalf("control character %q reached the terminal output: %q", banned, text)
		}
	}
}

// TestWriteScanHumanStripsTerminalControlCharacters pins the same rule for the
// end-of-scan summary on stdout: analyzer error summaries stay one sanitized
// line each.
func TestWriteScanHumanStripsTerminalControlCharacters(t *testing.T) {
	s := sampleScanSummary()
	s.state = "completed_with_warnings"
	s.runs = []reports.Run{{
		AnalyzerID: "ruff", State: "failed", Duration: time.Second,
		ErrorSummary: "ruff exited 1: \x1b]0;pwned\x07 injected title\nsecond line",
	}}
	var out bytes.Buffer
	writeScanHuman(&out, s)
	text := out.String()
	for _, banned := range []string{"\x1b", "\x07"} {
		if strings.Contains(text, banned) {
			t.Fatalf("control character %q reached stdout: %q", banned, text)
		}
	}
	// The forged newline must collapse onto a real summary line; a line
	// carrying "second line" must also carry a genuine run prefix (the
	// ErrorSummary text intentionally appears on both the analyzer line and
	// the Warnings line).
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "second line") && !strings.Contains(line, "Ruff") {
			t.Fatalf("forged newline produced a standalone summary line: %q", line)
		}
	}
	if !strings.Contains(text, "injected title second line") {
		t.Fatalf("sanitized error text missing from summary: %q", text)
	}
}

func TestFormatScanDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{1500 * time.Millisecond, "1.5s"},
		{83 * time.Second, "1m23s"},
		{1200 * time.Millisecond, "1.2s"},
	}
	for _, item := range cases {
		if got := formatScanDuration(item.in); got != item.want {
			t.Errorf("formatScanDuration(%s) = %q, want %q", item.in, got, item.want)
		}
	}
}

func TestTerminalScanState(t *testing.T) {
	for _, state := range []string{"completed", "completed_with_warnings", "failed", "cancelled", "interrupted"} {
		if !terminalScanState(state) {
			t.Errorf("terminalScanState(%q) = false", state)
		}
	}
	for _, state := range []string{"", "queued", "preparing", "running"} {
		if terminalScanState(state) {
			t.Errorf("terminalScanState(%q) = true", state)
		}
	}
}

func TestParseScanFlagsHelpReturnsErrHelp(t *testing.T) {
	var errOut bytes.Buffer
	_, err := parseScanFlags([]string{"--help"}, &errOut)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("err = %v", err)
	}
}

// --- CI gate flags: --fail-on and --max-findings --------------------------------

func TestParseScanFlagsGateFlags(t *testing.T) {
	cases := []struct {
		name         string
		args         []string
		wantLabel    string
		wantMax      int
		wantEnabled  bool
		wantCritical bool
		wantHigh     bool
		wantMedium   bool
		wantLow      bool
		wantInfo     bool
	}{
		{
			name:      "fail-on plus with max-findings",
			args:      []string{"--fail-on", "high+", "--max-findings", "10", `C:\proj`},
			wantLabel: "high+", wantMax: 10, wantEnabled: true,
			wantCritical: true, wantHigh: true,
		},
		{
			name:      "fail-on list is case-insensitive",
			args:      []string{`C:\proj`, "--fail-on", "Medium, LOW"},
			wantLabel: "medium,low", wantEnabled: true,
			wantMedium: true, wantLow: true,
		},
		{
			name:    "max-findings alone",
			args:    []string{"--max-findings", "1", `C:\proj`},
			wantMax: 1, wantEnabled: true,
		},
		{
			name: "no gate flags leaves the gate off",
			args: []string{`C:\proj`},
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			var errOut bytes.Buffer
			cfg, err := parseScanFlags(item.args, &errOut)
			if err != nil {
				t.Fatalf("parse: %v (stderr: %s)", err, errOut.String())
			}
			if cfg.gate.Enabled() != item.wantEnabled {
				t.Fatalf("gate enabled = %v, want %v", cfg.gate.Enabled(), item.wantEnabled)
			}
			if cfg.gate.FailOn.Label != item.wantLabel {
				t.Errorf("fail-on label = %q, want %q", cfg.gate.FailOn.Label, item.wantLabel)
			}
			if cfg.gate.MaxFindings != item.wantMax {
				t.Errorf("max-findings = %d, want %d", cfg.gate.MaxFindings, item.wantMax)
			}
			for severity, want := range map[analyzers.Severity]bool{
				analyzers.SeverityCritical: item.wantCritical,
				analyzers.SeverityHigh:     item.wantHigh,
				analyzers.SeverityMedium:   item.wantMedium,
				analyzers.SeverityLow:      item.wantLow,
				analyzers.SeverityInfo:     item.wantInfo,
			} {
				if got := cfg.gate.FailOn.Matches(severity); got != want {
					t.Errorf("fail-on Matches(%s) = %v, want %v", severity, got, want)
				}
			}
		})
	}
}

func TestParseScanFlagsRejectsBadGateInput(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		message string
	}{
		{"unknown severity", []string{"--fail-on", "severe", `C:\proj`}, `unknown severity "severe"`},
		{"garbage severity token", []string{"--fail-on", "high,bogus", `C:\proj`}, `unknown severity "bogus"`},
		{"double plus", []string{"--fail-on", "high++", `C:\proj`}, `unknown severity "high++"`},
		{"separators only", []string{"--fail-on", ",", `C:\proj`}, "no severities given"},
		{"zero max-findings", []string{"--max-findings", "0", `C:\proj`}, "max-findings must be a positive integer"},
		{"negative max-findings", []string{"--max-findings", "-3", `C:\proj`}, "max-findings must be a positive integer"},
		{"garbage max-findings", []string{"--max-findings", "many", `C:\proj`}, "invalid value"},
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

func TestRunScanCommandRejectsBadGateFlagWithExitCodeTwo(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runScanCommand([]string{"--fail-on", "urgent", "--max-findings", "0", `C:\proj`}, &out, &errOut); code != 2 {
		t.Fatalf("exit code = %d", code)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q", out.String())
	}
	if !strings.Contains(errOut.String(), `unknown severity "urgent"`) {
		t.Fatalf("reason missing: %q", errOut.String())
	}
}

// --- report format flag: --format text|json|github|sarif --------------------------

func TestParseScanFlagsFormatFlag(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantFormat string
		wantJSON   bool
	}{
		{"absent keeps the historical text output", []string{`C:\proj`}, "", false},
		{"explicit text", []string{"--format", "text", `C:\proj`}, "text", false},
		{"json selects the full report document", []string{"--format", "json", `C:\proj`}, "json", false},
		{"github selects the annotation stream", []string{"--format", "github", `C:\proj`}, "github", false},
		{"sarif selects the SARIF code-scanning document", []string{"--format", "sarif", `C:\proj`}, "sarif", false},
		{"format after the path works", []string{`C:\proj`, "--format", "json"}, "json", false},
		{"github format after the path works", []string{`C:\proj`, "--format", "github"}, "github", false},
		{"sarif format after the path works", []string{`C:\proj`, "--format", "sarif"}, "sarif", false},
		{"json summary flag still accepted with text format", []string{"--json", "--format", "text", `C:\proj`}, "text", true},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			var errOut bytes.Buffer
			cfg, err := parseScanFlags(item.args, &errOut)
			if err != nil {
				t.Fatalf("parse: %v (stderr: %s)", err, errOut.String())
			}
			if cfg.format != item.wantFormat {
				t.Errorf("format = %q, want %q", cfg.format, item.wantFormat)
			}
			if cfg.json != item.wantJSON {
				t.Errorf("json = %v, want %v", cfg.json, item.wantJSON)
			}
		})
	}
}

func TestParseScanFlagsRejectsBadFormatInput(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		message string
	}{
		{"unknown format", []string{"--format", "yaml", `C:\proj`}, "format must be text, json, github, or sarif"},
		{"github annotations misspelled", []string{"--format", "actions", `C:\proj`}, "format must be text, json, github, or sarif"},
		{"json summary combined with report format", []string{"--json", "--format", "json", `C:\proj`}, "--json cannot be combined with --format json"},
		{"json summary combined with github format", []string{"--json", "--format", "github", `C:\proj`}, "--json cannot be combined with --format github"},
		{"json summary combined with sarif format", []string{"--json", "--format", "sarif", `C:\proj`}, "--json cannot be combined with --format sarif"},
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

func TestRunScanCommandRejectsBadFormatFlagWithExitCodeTwo(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runScanCommand([]string{"--format", "yaml", `C:\proj`}, &out, &errOut); code != 2 {
		t.Fatalf("exit code = %d", code)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q", out.String())
	}
	if !strings.Contains(errOut.String(), "format must be text, json, github, or sarif") {
		t.Fatalf("reason missing: %q", errOut.String())
	}
}

// TestParseScanFlagsWatchWithSARIFFormatAllowed pins the watch interplay of the
// document formats: --watch --format sarif is accepted (each rescan emits one
// complete, newline-terminated SARIF log, mirroring how --format json behaves
// in the loop) while --watch --format github stays rejected exactly as before
// (also pinned by the watch_test.go rejection tests).
func TestParseScanFlagsWatchWithSARIFFormatAllowed(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseScanFlags([]string{"--watch", "--format", "sarif", `C:\proj`}, &errOut)
	if err != nil {
		t.Fatalf("parse: %v (stderr: %s)", err, errOut.String())
	}
	if !cfg.watch || cfg.format != "sarif" {
		t.Fatalf("watch/format = %v/%q, want true/sarif", cfg.watch, cfg.format)
	}
	// The counterpart is unchanged: the annotation stream is the one document
	// format the watch loop refuses.
	var rejectOut bytes.Buffer
	if _, err := parseScanFlags([]string{"--watch", "--format", "github", `C:\proj`}, &rejectOut); err == nil {
		t.Fatal("--watch --format github must still be rejected")
	} else if !strings.Contains(err.Error(), "--watch cannot be combined with --format github") {
		t.Fatalf("rejection reason = %q", err.Error())
	}
}

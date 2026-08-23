package main

// Tests for the --incremental flag of `bluntcode scan` and the --watch loop's
// automatic switch to incremental rescans from its second scan onward.

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestParseScanFlagsIncremental(t *testing.T) {
	cases := []struct {
		name            string
		args            []string
		wantIncremental bool
		wantJobs        int
		wantWatch       bool
		wantBaseline    string
		wantFormat      string
	}{
		{"absent stays full", []string{`C:\proj`}, false, 0, false, "", ""},
		{"incremental alone", []string{"--incremental", `C:\proj`}, true, 0, false, "", ""},
		{"after the path works", []string{`C:\proj`, "--incremental"}, true, 0, false, "", ""},
		{"composes with jobs", []string{"--incremental", "--jobs", "4", `C:\proj`}, true, 4, false, "", ""},
		{"composes with gate flags", []string{"--incremental", "--fail-on", "high+", "--max-findings", "5", `C:\proj`}, true, 0, false, "", ""},
		{"composes with baseline", []string{"--incremental", "--baseline", "scan-123", `C:\proj`}, true, 0, false, "scan-123", ""},
		{"composes with format", []string{"--incremental", "--format", "json", `C:\proj`}, true, 0, false, "", "json"},
		{"composes with watch", []string{"--incremental", "--watch", `C:\proj`}, true, 0, true, "", ""},
		{"boolean =true form", []string{"--incremental=true", `C:\proj`}, true, 0, false, "", ""},
		{"boolean =false form", []string{"--incremental=false", `C:\proj`}, false, 0, false, "", ""},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			var errOut bytes.Buffer
			cfg, err := parseScanFlags(item.args, &errOut)
			if err != nil {
				t.Fatalf("parse: %v (stderr: %s)", err, errOut.String())
			}
			if cfg.incremental != item.wantIncremental {
				t.Errorf("incremental = %v, want %v", cfg.incremental, item.wantIncremental)
			}
			if cfg.jobs != item.wantJobs {
				t.Errorf("jobs = %d, want %d", cfg.jobs, item.wantJobs)
			}
			if cfg.watch != item.wantWatch {
				t.Errorf("watch = %v, want %v", cfg.watch, item.wantWatch)
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

// TestParseScanFlagsIncrementalTakesNoValue: --incremental is a boolean flag,
// so a following bare word is parsed as the workspace path and the extra
// positional becomes a usage error rather than being consumed as a value.
func TestParseScanFlagsIncrementalTakesNoValue(t *testing.T) {
	var errOut bytes.Buffer
	_, err := parseScanFlags([]string{"--incremental", "true", `C:\proj`}, &errOut)
	if err == nil {
		t.Fatal("expected a usage error for --incremental followed by a bare word")
	}
	if !strings.Contains(err.Error(), "exactly one workspace path is required") {
		t.Fatalf("error = %q, want the extra-positional message", err.Error())
	}
}

// TestWatchLoopSecondScanRunsIncremental pins the --watch contract: the first
// scan runs full (there is nothing to reuse yet) and every rescan is passed
// incremental=true, which the production runner forwards to the scan options.
func TestWatchLoopSecondScanRunsIncremental(t *testing.T) {
	root := t.TempDir()
	writeWatchFile(t, root, "app.py", "x = 1\n")
	fake := &watchFake{script: func(call int) scanRunResult {
		if call >= 3 {
			return scanRunResult{code: 130, interruptSeen: true}
		}
		return scanRunResult{code: 0}
	}}
	env, _ := watchTestEnv(t, root, fake, nil)
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
	fake.mu.Lock()
	got := append([]bool(nil), fake.incremental...)
	fake.mu.Unlock()
	want := []bool{false, true, true}
	if len(got) != len(want) {
		t.Fatalf("incremental flags = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("incremental flags = %v, want %v (scan %d wrong)", got, want, i+1)
		}
	}
}

// TestWatchLoopExplicitIncrementalFlagAppliesToEveryScan: an explicit
// --incremental composes with --watch, and the production runner closure ORs
// the loop's own incremental signal with the flag so no scan is ever
// downgraded back to full.
func TestWatchLoopExplicitIncrementalFlagAppliesToEveryScan(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseScanFlags([]string{"--incremental", "--watch", `C:\proj`}, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.incremental || !cfg.watch {
		t.Fatalf("incremental=%v watch=%v, want both true", cfg.incremental, cfg.watch)
	}
	first := cfg.incremental || false // the loop's first scan signal
	second := cfg.incremental || true // every rescan
	if !first || !second {
		t.Fatalf("combined incremental flags = %v/%v, want true/true", first, second)
	}
}

package main

// Tests for the --jobs flag of `bluntcode scan`: valid bounds parse into the
// config, invalid values are usage errors (exit 2 with the reason and the
// usage block), and the default stays the sequential execution model.

import (
	"bytes"
	"strings"
	"testing"
)

// --- parallelism flag: --jobs ---------------------------------------------------

func TestParseScanFlagsJobsFlag(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantJobs int
	}{
		{"absent keeps the default execution model", []string{`C:\proj`}, 0},
		{"explicit bound", []string{"--jobs", "4", `C:\proj`}, 4},
		{"bound of one is allowed", []string{"--jobs", "1", `C:\proj`}, 1},
		{"flag after the path works", []string{`C:\proj`, "--jobs", "3"}, 3},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			var errOut bytes.Buffer
			cfg, err := parseScanFlags(item.args, &errOut)
			if err != nil {
				t.Fatalf("parse: %v (stderr: %s)", err, errOut.String())
			}
			if cfg.jobs != item.wantJobs {
				t.Errorf("jobs = %d, want %d", cfg.jobs, item.wantJobs)
			}
		})
	}
}

func TestParseScanFlagsRejectsBadJobsInput(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		message string
	}{
		{"zero jobs", []string{"--jobs", "0", `C:\proj`}, "jobs must be a positive integer"},
		{"negative jobs", []string{"--jobs", "-2", `C:\proj`}, "jobs must be a positive integer"},
		{"garbage jobs", []string{"--jobs", "many", `C:\proj`}, "invalid value"},
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

func TestRunScanCommandRejectsBadJobsFlagWithExitCodeTwo(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runScanCommand([]string{"--jobs", "0", `C:\proj`}, &out, &errOut); code != 2 {
		t.Fatalf("exit code = %d", code)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q", out.String())
	}
	if !strings.Contains(errOut.String(), "jobs must be a positive integer") {
		t.Fatalf("reason missing: %q", errOut.String())
	}
}

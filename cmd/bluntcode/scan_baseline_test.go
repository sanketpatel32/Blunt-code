package main

// Baseline diff mode for `bluntcode scan`: --baseline <scan-id-or-sarif>
// flag parsing, reference resolution (SARIF file or previous scan ID), and the
// usage-style exit-2 path for unknown IDs and unreadable files. All tests run
// offline; nothing downloads tools.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/reports"
)

// --- Flag parsing: --baseline with and without gate flags --------------------

func TestParseScanFlagsBaseline(t *testing.T) {
	cases := []struct {
		name         string
		args         []string
		wantBaseline string
		wantGateOn   bool
		wantMax      int
	}{
		{
			name:         "baseline with both gate flags",
			args:         []string{"--baseline", `C:\ci\baseline.sarif`, "--fail-on", "high+", "--max-findings", "5", `C:\proj`},
			wantBaseline: `C:\ci\baseline.sarif`, wantGateOn: true, wantMax: 5,
		},
		{
			name:         "baseline and gate flags after the path",
			args:         []string{`C:\proj`, "--baseline", "scan-123", "--fail-on", "medium"},
			wantBaseline: "scan-123", wantGateOn: true,
		},
		{
			name:         "baseline without gate flags is allowed",
			args:         []string{"--baseline", "scan-123", `C:\proj`},
			wantBaseline: "scan-123",
		},
		{
			name:       "no baseline flag leaves it empty while gate flags still work",
			args:       []string{"--fail-on", "high", `C:\proj`},
			wantGateOn: true,
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			var errOut bytes.Buffer
			cfg, err := parseScanFlags(item.args, &errOut)
			if err != nil {
				t.Fatalf("parse: %v (stderr: %s)", err, errOut.String())
			}
			if cfg.baseline != item.wantBaseline {
				t.Errorf("baseline = %q, want %q", cfg.baseline, item.wantBaseline)
			}
			if cfg.gate.Enabled() != item.wantGateOn {
				t.Errorf("gate enabled = %v, want %v", cfg.gate.Enabled(), item.wantGateOn)
			}
			if cfg.gate.MaxFindings != item.wantMax {
				t.Errorf("max-findings = %d, want %d", cfg.gate.MaxFindings, item.wantMax)
			}
		})
	}
}

func TestParseScanFlagsRejectsEmptyBaseline(t *testing.T) {
	var errOut bytes.Buffer
	_, err := parseScanFlags([]string{"--baseline", "", `C:\proj`}, &errOut)
	if err == nil {
		t.Fatal("empty --baseline accepted")
	}
	if !strings.Contains(err.Error(), "baseline must be a scan ID or the path of a SARIF file") {
		t.Fatalf("error = %q", err.Error())
	}
	if !strings.Contains(errOut.String(), "usage: bluntcode scan") {
		t.Fatalf("usage not printed: %q", errOut.String())
	}
}

// --- Baseline reference resolution -------------------------------------------

// seedBaselineScan persists one completed scan with the given findings for the
// workspace and returns its scan ID.
func seedBaselineScan(t *testing.T, db *database.DB, workspaceID string, findings []analyzers.Finding) string {
	t.Helper()
	scan, err := db.CreateScan(context.Background(), core.Scan{WorkspaceID: workspaceID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveAnalyzerResult(context.Background(), scan.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, findings, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteScan(context.Background(), scan.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	return scan.ID
}

func baselineFindings(rules ...string) []analyzers.Finding {
	findings := make([]analyzers.Finding, 0, len(rules))
	for _, rule := range rules {
		f := analyzers.Finding{AnalyzerID: "ruff", RuleID: rule, RelativePath: "src/a.py", Message: "issue " + rule, Severity: analyzers.SeverityHigh, Category: analyzers.CategoryCorrectness}
		f.SetFingerprint()
		findings = append(findings, f)
	}
	return findings
}

// writeSARIFFixture marshals findings through the report export and writes
// them to a file with LF endings so the bytes match a real export.
func writeSARIFFixture(t *testing.T, findings []analyzers.Finding) string {
	t.Helper()
	data, err := json.Marshal(reports.SARIF(reports.Build(reports.Input{Findings: findings})))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "baseline.sarif")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadScanBaseline(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	work, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Sample", RootPath: "C:/sample"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := db.CreateWorkspace(ctx, core.Workspace{Name: "Other", RootPath: "C:/other"})
	if err != nil {
		t.Fatal(err)
	}
	findings := baselineFindings("F401", "E501")
	scanID := seedBaselineScan(t, db, work.ID, findings)
	fingerprints := map[string]bool{}
	for _, f := range findings {
		fingerprints[f.Fingerprint] = true
	}

	t.Run("sarif path", func(t *testing.T) {
		baseline, err := loadScanBaseline(ctx, db, writeSARIFFixture(t, findings), work.ID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if baseline.Len() != len(findings) {
			t.Fatalf("Len = %d, want %d", baseline.Len(), len(findings))
		}
		for fingerprint := range fingerprints {
			if !baseline.Known(fingerprint) {
				t.Errorf("Known(%s) = false for SARIF baseline", fingerprint)
			}
		}
	})

	t.Run("scan id", func(t *testing.T) {
		baseline, err := loadScanBaseline(ctx, db, scanID, work.ID)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if baseline.Len() != len(findings) {
			t.Fatalf("Len = %d, want %d", baseline.Len(), len(findings))
		}
		for fingerprint := range fingerprints {
			if !baseline.Known(fingerprint) {
				t.Errorf("Known(%s) = false for scan-id baseline", fingerprint)
			}
		}
	})

	t.Run("scan id of another workspace is rejected", func(t *testing.T) {
		foreign := seedBaselineScan(t, db, other.ID, findings)
		_, err := loadScanBaseline(ctx, db, foreign, work.ID)
		if err == nil || !strings.Contains(err.Error(), "different workspace") {
			t.Fatalf("err = %v, want different-workspace rejection", err)
		}
	})

	t.Run("unknown reference", func(t *testing.T) {
		_, err := loadScanBaseline(ctx, db, "no-such-id-or-file", work.ID)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("err = %v, want not-found rejection", err)
		}
	})

	t.Run("invalid sarif file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.sarif")
		if err := os.WriteFile(path, []byte("definitely not sarif\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := loadScanBaseline(ctx, db, path, work.ID)
		if err == nil || !strings.Contains(err.Error(), "invalid SARIF") {
			t.Fatalf("err = %v, want invalid-SARIF rejection", err)
		}
	})

	t.Run("directory path", func(t *testing.T) {
		_, err := loadScanBaseline(ctx, db, t.TempDir(), work.ID)
		if err == nil || !strings.Contains(err.Error(), "is a directory") {
			t.Fatalf("err = %v, want directory rejection", err)
		}
	})
}

// --- End to end: bad baselines exit 2 before any scan runs -------------------

// TestRunScanCommandBaselineErrorsExitTwo pins the CI contract: an unresolvable
// --baseline is a usage error (exit 2) reported before the scan starts, with
// nothing on stdout. The unknown-ID case also proves a bare scan ID that is not
// a file is treated as a scan reference, not a path.
func TestRunScanCommandBaselineErrorsExitTwo(t *testing.T) {
	isolateDataDir(t)
	ws := sampleWorkspace(t)
	cases := []struct {
		name     string
		baseline string
		message  string
	}{
		{"unknown scan id", "no-such-scan-id", "not found"},
		{"invalid sarif file", writeSARIFFixtureInvalid(t), "invalid SARIF"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := runScanCommand([]string{ws, "--quiet", "--baseline", item.baseline}, &out, &errOut)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2 (stderr: %s)", code, errOut.String())
			}
			if out.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", out.String())
			}
			if !strings.Contains(errOut.String(), item.message) {
				t.Fatalf("reason %q missing from stderr: %q", item.message, errOut.String())
			}
			if !strings.Contains(errOut.String(), "usage: bluntcode scan") {
				t.Fatalf("usage line missing: %q", errOut.String())
			}
		})
	}
}

// writeSARIFFixtureInvalid writes an unreadable baseline file.
func writeSARIFFixtureInvalid(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "broken.sarif")
	if err := os.WriteFile(path, []byte("<not json>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestRunScanCommandBaselineSummaryLine covers the stderr baseline summary with
// a valid SARIF baseline while offline: the scan still runs (and fails offline
// with exit 1), and the baseline line reports the scan's findings without
// changing the exit code by itself. Offline analyzers produce no findings, so
// both counters are zero — the point here is that the line appears and the run
// proceeds instead of dying at baseline resolution.
func TestRunScanCommandBaselineSummaryLine(t *testing.T) {
	isolateDataDir(t)
	ws := sampleWorkspace(t)
	sarif := writeSARIFFixture(t, baselineFindings("F401"))
	var out, errOut bytes.Buffer
	code := runScanCommand([]string{ws, "--quiet", "--json", "--baseline", sarif}, &out, &errOut)
	if code != 1 {
		t.Fatalf("offline scan exit code = %d, want 1 (stderr: %s)", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "baseline: 0 known finding(s) excluded from gate, 0 new finding(s)") {
		t.Fatalf("baseline summary line missing: %q", errOut.String())
	}
}

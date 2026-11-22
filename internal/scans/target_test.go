package scans

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/analyzers/dastprobe"
)

func TestNormalizeScanTarget(t *testing.T) {
	if got, err := NormalizeScanTarget(analyzers.ProfileStandard, ""); err != nil || got != "" {
		t.Errorf("empty target must be valid for every profile: %q, %v", got, err)
	}
	if _, err := NormalizeScanTarget(analyzers.ProfileDeep, "http://localhost:3000"); !errors.Is(err, ErrTargetNeedsPentest) {
		t.Errorf("deep + target = %v, want ErrTargetNeedsPentest", err)
	}
	if got, err := NormalizeScanTarget(analyzers.ProfilePentest, "localhost:3000"); err != nil || got != "http://localhost:3000" {
		t.Errorf("pentest target = %q, %v; want http://localhost:3000", got, err)
	}
	if _, err := NormalizeScanTarget(analyzers.ProfilePentest, "ftp://x"); err == nil {
		t.Error("non-HTTP target must be rejected")
	}
}

// TestPentestScanWithoutTargetSkipsDAST pins the honest default: a pentest
// scan with no target runs the static analyzers, leaves dast-probe out of
// expected coverage, and records no dast-probe run.
func TestPentestScanWithoutTargetSkipsDAST(t *testing.T) {
	db, service, work := newSupervisionService(t, blockingAnalyzer{release: released()}, dastprobe.New())
	scan, err := service.DiscoverAndStart(context.Background(), work, analyzers.ProfilePentest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(scan.Snapshot.EnabledAnalyzers, dastprobe.ID) {
		t.Errorf("dast-probe expected without a target: %v", scan.Snapshot.EnabledAnalyzers)
	}
	waitForTerminalState(t, db, scan.ID)
	runs, err := db.AnalyzerRuns(context.Background(), scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range runs {
		if run.AnalyzerID == dastprobe.ID {
			t.Errorf("dast-probe ran without a target: %+v", run)
		}
	}
}

// TestPentestScanWithTargetRunsDAST pins issue #24: a pentest scan given a
// target performs real HTTP probing and persists its findings.
func TestPentestScanWithTargetRunsDAST(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	db, service, work := newSupervisionService(t, blockingAnalyzer{release: released()}, dastprobe.New())
	scan, err := service.DiscoverAndStartWithOptions(context.Background(), work, analyzers.ProfilePentest, nil, ScanOptions{TargetURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if scan.Snapshot.TargetURL != server.URL || !slices.Contains(scan.Snapshot.EnabledAnalyzers, dastprobe.ID) {
		t.Errorf("snapshot target=%q analyzers=%v; want the target recorded and dast-probe expected", scan.Snapshot.TargetURL, scan.Snapshot.EnabledAnalyzers)
	}
	waitForTerminalState(t, db, scan.ID)
	if hits == 0 {
		t.Fatal("pentest scan with a target sent no HTTP request")
	}
	findings, err := db.Findings(context.Background(), scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	var dastFindings int
	for _, f := range findings {
		if f.AnalyzerID == dastprobe.ID {
			dastFindings++
		}
	}
	if dastFindings == 0 {
		t.Errorf("no dast-probe findings persisted for a target missing every security header; got %d findings", len(findings))
	}
}

func released() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

// TestPentestTargetRunsWithoutSourceFiles: a DAST target is scan signal on
// its own, so an empty workspace still gets probed instead of failing.
func TestPentestTargetRunsWithoutSourceFiles(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	db, service, work := newSupervisionService(t, dastprobe.New())
	if err := os.Remove(filepath.Join(work.RootPath, "main.py")); err != nil {
		t.Fatal(err)
	}
	scan, err := service.DiscoverAndStartWithOptions(context.Background(), work, analyzers.ProfilePentest, nil, ScanOptions{TargetURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	final := waitForTerminalState(t, db, scan.ID)
	if final.State == "failed" || hits == 0 {
		t.Errorf("empty workspace with a target: state=%s hits=%d; want the probe to run", final.State, hits)
	}
}

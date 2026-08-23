package scans

import (
	"context"
	"os"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/database"
)

func suppressionTestFinding(rule, message string, severity analyzers.Severity) analyzers.Finding {
	f := analyzers.Finding{AnalyzerID: "fake", RuleID: rule, Message: message, Severity: severity, Category: analyzers.CategoryCorrectness, RelativePath: "main.py"}
	f.SetFingerprint()
	return f
}

func TestFilterSuppressed(t *testing.T) {
	dismissed := suppressionTestFinding("DISMISSED", "dismiss me", analyzers.SeverityHigh)
	kept := suppressionTestFinding("KEPT", "keep me", analyzers.SeverityMedium)
	findings := []analyzers.Finding{dismissed, kept, dismissed}
	suppressed := map[string]bool{dismissed.Fingerprint: true}
	got := FilterSuppressed(findings, suppressed)
	if len(got) != 1 || got[0].Fingerprint != kept.Fingerprint {
		t.Fatalf("only unsuppressed findings may survive: %#v", got)
	}
	// An empty suppression set is the identity.
	if got := FilterSuppressed(findings, nil); len(got) != 3 {
		t.Fatalf("empty suppression set must keep every finding: %#v", got)
	}
	// A fully suppressed input yields an empty, non-nil slice.
	if got := FilterSuppressed(findings, map[string]bool{dismissed.Fingerprint: true, kept.Fingerprint: true}); len(got) != 0 || got == nil {
		t.Fatalf("fully suppressed input must yield an empty slice: %#v", got)
	}
}

// TestEvaluateGateSkipsSuppressedFindings pins the defensive exclusion: callers
// filter suppressed findings before the gate, so a stray suppressed row must
// never count toward --fail-on or --max-findings.
func TestEvaluateGateSkipsSuppressedFindings(t *testing.T) {
	spec, err := ParseSeveritySpec("high+")
	if err != nil {
		t.Fatal(err)
	}
	findings := []analyzers.Finding{
		{Severity: analyzers.SeverityCritical, Status: StatusSuppressed},
		{Severity: analyzers.SeverityHigh, Status: StatusSuppressed},
		{Severity: analyzers.SeverityLow},
	}
	result := EvaluateGate(findings, GateConfig{FailOn: spec, MaxFindings: 1})
	if result.Failed || result.OpenTotal != 1 || result.ThresholdHits != 0 {
		t.Fatalf("suppressed findings must not trip the gate: %#v", result)
	}
}

// TestSuppressionFlowThroughScanPipeline covers the full "ignore this finding
// forever" workflow against a real service and database: after a fingerprint
// is suppressed, the next scan still stores the matching finding but excludes
// it from the scan totals and the Markdown report, the findings list stamps it
// suppressed, and removing the suppression restores normal counting.
func TestSuppressionFlowThroughScanPipeline(t *testing.T) {
	ctx := context.Background()
	dismissed := suppressionTestFinding("DISMISSED", "dismiss me", analyzers.SeverityHigh)
	kept := suppressionTestFinding("KEPT", "keep me", analyzers.SeverityMedium)
	analyzer := &scriptedAnalyzer{id: "fake", findings: []analyzers.Finding{dismissed, kept}}
	fixture := newScanFixture(t, analyzer)

	first := fixture.waitForTerminal(t, fixture.startScan(t).ID)
	if first.State != "completed" || first.TotalFindings != 2 {
		t.Fatalf("baseline scan: state=%q total=%d", first.State, first.TotalFindings)
	}

	if _, err := fixture.db.AddSuppression(ctx, fixture.work.ID, dismissed.Fingerprint, "wontfix"); err != nil {
		t.Fatal(err)
	}
	second := fixture.waitForTerminal(t, fixture.startScan(t).ID)
	if second.State != "completed" || second.TotalFindings != 1 {
		t.Fatalf("suppressed finding must not count toward totals: state=%q total=%d", second.State, second.TotalFindings)
	}
	// The finding is still stored so the UI can show it with its status.
	stored, err := fixture.db.Findings(ctx, second.ID)
	if err != nil || len(stored) != 2 {
		t.Fatalf("suppressed findings must stay stored: %#v (%v)", stored, err)
	}
	page, err := fixture.db.FindingsPage(ctx, second, database.FindingFilter{Limit: 25})
	if err != nil || page.Total != 2 {
		t.Fatalf("findings list must keep suppressed rows visible: %#v (%v)", page, err)
	}
	statuses := map[string]string{}
	for _, item := range page.Items {
		statuses[item.Fingerprint] = item.Status
	}
	if statuses[dismissed.Fingerprint] != StatusSuppressed || statuses[kept.Fingerprint] != "persistent" {
		t.Fatalf("statuses after suppression: %#v", statuses)
	}
	// The scan-time Markdown report carries only the kept finding.
	path, err := fixture.db.ReportPath(ctx, second.ID)
	if err != nil || path == "" {
		t.Fatalf("report path: %q (%v)", path, err)
	}
	markdown, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(markdown), "keep me") || strings.Contains(string(markdown), "dismiss me") {
		t.Fatalf("markdown report must exclude the suppressed finding")
	}

	// Removing the suppression un-suppresses the next scan.
	if err := fixture.db.RemoveSuppression(ctx, fixture.work.ID, dismissed.Fingerprint); err != nil {
		t.Fatal(err)
	}
	third := fixture.waitForTerminal(t, fixture.startScan(t).ID)
	if third.State != "completed" || third.TotalFindings != 2 {
		t.Fatalf("removed suppression must restore counting: state=%q total=%d", third.State, third.TotalFindings)
	}
	page, err = fixture.db.FindingsPage(ctx, third, database.FindingFilter{Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.Fingerprint == dismissed.Fingerprint && item.Status != "persistent" {
			t.Fatalf("un-suppressed finding must compare normally again: %#v", item)
		}
	}
}

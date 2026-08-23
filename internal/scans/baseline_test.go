package scans

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/reports"
)

// baselineFinding builds a finding with just the fields the baseline and gate
// read: a pinned fingerprint plus severity.
func baselineFinding(severity analyzers.Severity, fingerprint string) analyzers.Finding {
	return analyzers.Finding{AnalyzerID: "ruff", RuleID: "R", Severity: severity, Fingerprint: fingerprint}
}

// sarifFor renders findings as a SARIF 2.1.0 log just like the report export
// does, so baseline loading is tested against real writer output.
func sarifFor(t *testing.T, findings []analyzers.Finding) *bytes.Reader {
	t.Helper()
	data, err := json.Marshal(reports.SARIF(reports.Build(reports.Input{Findings: findings})))
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(data)
}

func TestBaselineFromFindings(t *testing.T) {
	findings := []analyzers.Finding{
		baselineFinding(analyzers.SeverityHigh, "aaa"),
		baselineFinding(analyzers.SeverityLow, "bbb"),
		baselineFinding(analyzers.SeverityLow, "aaa"),                      // duplicate fingerprint collapses
		{AnalyzerID: "ruff", RuleID: "R", Severity: analyzers.SeverityLow}, // no fingerprint: ignored
	}
	baseline := BaselineFromFindings(findings)
	if baseline.Len() != 2 {
		t.Fatalf("Len = %d, want 2", baseline.Len())
	}
	for _, fingerprint := range []string{"aaa", "bbb"} {
		if !baseline.Known(fingerprint) {
			t.Errorf("Known(%s) = false", fingerprint)
		}
	}
	if baseline.Known("ccc") {
		t.Error("Known(ccc) = true")
	}
	if baseline.Known("") {
		t.Error("the empty fingerprint must never be known")
	}
}

func TestBaselineFromSARIFRoundTrip(t *testing.T) {
	findings := []analyzers.Finding{
		baselineFinding(analyzers.SeverityHigh, "aaa"),
		baselineFinding(analyzers.SeverityMedium, "bbb"),
	}
	baseline, err := BaselineFromSARIF(sarifFor(t, findings))
	if err != nil {
		t.Fatalf("from SARIF: %v", err)
	}
	if baseline.Len() != 2 {
		t.Fatalf("Len = %d, want 2", baseline.Len())
	}
	for _, fingerprint := range []string{"aaa", "bbb"} {
		if !baseline.Known(fingerprint) {
			t.Errorf("Known(%s) = false after SARIF round-trip", fingerprint)
		}
	}
}

func TestBaselineFromSARIFRejectsInvalidInput(t *testing.T) {
	for _, log := range []string{"not json", `{"version":"2.0.0","runs":[]}`} {
		if _, err := BaselineFromSARIF(strings.NewReader(log)); err == nil {
			t.Errorf("%q accepted as a baseline", log)
		}
	}
}

func TestBaselineSplit(t *testing.T) {
	cases := []struct {
		name      string
		baseline  Baseline
		findings  []analyzers.Finding
		wantKnown []string
		wantFresh []string
	}{
		{
			name:      "mixed",
			baseline:  BaselineFromFindings([]analyzers.Finding{baselineFinding(analyzers.SeverityHigh, "old-1"), baselineFinding(analyzers.SeverityHigh, "old-2")}),
			findings:  []analyzers.Finding{baselineFinding(analyzers.SeverityHigh, "old-1"), baselineFinding(analyzers.SeverityHigh, "new-1"), baselineFinding(analyzers.SeverityLow, "old-2")},
			wantKnown: []string{"old-1", "old-2"},
			wantFresh: []string{"new-1"},
		},
		{
			name:      "all known",
			baseline:  BaselineFromFindings([]analyzers.Finding{baselineFinding(analyzers.SeverityHigh, "x")}),
			findings:  []analyzers.Finding{baselineFinding(analyzers.SeverityHigh, "x")},
			wantKnown: []string{"x"},
		},
		{
			name:      "empty baseline treats everything as new",
			baseline:  BaselineFromFindings(nil),
			findings:  []analyzers.Finding{baselineFinding(analyzers.SeverityHigh, "x"), baselineFinding(analyzers.SeverityLow, "y")},
			wantFresh: []string{"x", "y"},
		},
		{
			name:      "zero Baseline treats everything as new",
			baseline:  Baseline{},
			findings:  []analyzers.Finding{baselineFinding(analyzers.SeverityHigh, "x")},
			wantFresh: []string{"x"},
		},
		{
			name:     "no findings",
			baseline: BaselineFromFindings([]analyzers.Finding{baselineFinding(analyzers.SeverityHigh, "x")}),
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			known, fresh := item.baseline.Split(item.findings)
			collect := func(fs []analyzers.Finding) []string {
				out := []string{}
				for _, f := range fs {
					out = append(out, f.Fingerprint)
				}
				return out
			}
			if got := collect(known); strings.Join(got, ",") != strings.Join(item.wantKnown, ",") {
				t.Errorf("known = %v, want %v", got, item.wantKnown)
			}
			if got := collect(fresh); strings.Join(got, ",") != strings.Join(item.wantFresh, ",") {
				t.Errorf("fresh = %v, want %v", got, item.wantFresh)
			}
		})
	}
}

// TestBaselineGateCountsOnlyNewFindings exercises the exact composition the
// CLI wires: split against the baseline first, then EvaluateGate on the fresh
// half alone. Known findings must not trip --fail-on or --max-findings; an
// empty baseline must reproduce the gate-free-baseline behavior exactly.
func TestBaselineGateCountsOnlyNewFindings(t *testing.T) {
	highPlus := mustSpec(t, "high+")
	known := []analyzers.Finding{
		baselineFinding(analyzers.SeverityCritical, "known-crit"),
		baselineFinding(analyzers.SeverityHigh, "known-high"),
		baselineFinding(analyzers.SeverityLow, "known-low"),
	}
	cases := []struct {
		name        string
		baseline    Baseline
		findings    []analyzers.Finding
		gate        GateConfig
		wantFailed  bool
		wantHits    int
		wantOpen    int
		wantMessage string
	}{
		{
			name:       "known findings excluded from the threshold",
			baseline:   BaselineFromFindings(known),
			findings:   known,
			gate:       GateConfig{FailOn: highPlus},
			wantFailed: false, wantHits: 0, wantOpen: 0,
		},
		{
			name:       "new finding trips the threshold",
			baseline:   BaselineFromFindings(known),
			findings:   append(append([]analyzers.Finding{}, known...), baselineFinding(analyzers.SeverityHigh, "new-high")),
			gate:       GateConfig{FailOn: highPlus},
			wantFailed: true, wantHits: 1, wantOpen: 1,
			wantMessage: "fail: 1 finding(s) at or above high (gate: --fail-on high+)",
		},
		{
			name:       "threshold counts only new findings when several are known",
			baseline:   BaselineFromFindings(known),
			findings:   append(append([]analyzers.Finding{}, known...), baselineFinding(analyzers.SeverityCritical, "new-crit"), baselineFinding(analyzers.SeverityHigh, "new-high")),
			gate:       GateConfig{FailOn: highPlus},
			wantFailed: true, wantHits: 2, wantOpen: 2,
			wantMessage: "fail: 2 finding(s) at or above high (gate: --fail-on high+)",
		},
		{
			name:       "max-findings counts only new findings",
			baseline:   BaselineFromFindings(known),
			findings:   append(append([]analyzers.Finding{}, known...), baselineFinding(analyzers.SeverityLow, "new-1"), baselineFinding(analyzers.SeverityLow, "new-2")),
			gate:       GateConfig{MaxFindings: 2},
			wantFailed: false, wantHits: 0, wantOpen: 2,
		},
		{
			name:       "max-findings trips on new findings alone",
			baseline:   BaselineFromFindings(known),
			findings:   append(append([]analyzers.Finding{}, known...), baselineFinding(analyzers.SeverityLow, "new-1"), baselineFinding(analyzers.SeverityLow, "new-2"), baselineFinding(analyzers.SeverityLow, "new-3")),
			gate:       GateConfig{MaxFindings: 2},
			wantFailed: true, wantHits: 0, wantOpen: 3,
			wantMessage: "fail: 3 finding(s) total exceeds the maximum of 2 (gate: --max-findings 2)",
		},
		{
			name:       "empty baseline behaves like no baseline",
			baseline:   BaselineFromFindings(nil),
			findings:   known,
			gate:       GateConfig{FailOn: highPlus, MaxFindings: 2},
			wantFailed: true, wantHits: 2, wantOpen: 3,
			wantMessage: "fail: 2 finding(s) at or above high; 3 finding(s) total exceeds the maximum of 2 (gate: --fail-on high+, --max-findings 2)",
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, fresh := item.baseline.Split(item.findings)
			result := EvaluateGate(fresh, item.gate)
			if result.Failed != item.wantFailed {
				t.Errorf("Failed = %v, want %v", result.Failed, item.wantFailed)
			}
			if result.ThresholdHits != item.wantHits {
				t.Errorf("ThresholdHits = %d, want %d", result.ThresholdHits, item.wantHits)
			}
			if result.OpenTotal != item.wantOpen {
				t.Errorf("OpenTotal = %d, want %d", result.OpenTotal, item.wantOpen)
			}
			if message := result.FailureMessage(); message != item.wantMessage {
				t.Errorf("FailureMessage() = %q, want %q", message, item.wantMessage)
			}
		})
	}
}

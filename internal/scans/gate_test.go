package scans

import (
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

// gateFindings builds findings with just the fields the gate reads.
func gateFindings(statuses map[int]string, severities ...analyzers.Severity) []analyzers.Finding {
	findings := make([]analyzers.Finding, 0, len(severities))
	for i, severity := range severities {
		findings = append(findings, analyzers.Finding{Severity: severity, Status: statuses[i]})
	}
	return findings
}

func TestParseSeveritySpec(t *testing.T) {
	all := []analyzers.Severity{analyzers.SeverityCritical, analyzers.SeverityHigh, analyzers.SeverityMedium, analyzers.SeverityLow, analyzers.SeverityInfo}
	cases := []struct {
		name string
		spec string
		want map[analyzers.Severity]bool
		// wantLabel is the canonical rendering echoed in gate messages.
		wantLabel string
	}{
		{"single severity", "high", map[analyzers.Severity]bool{analyzers.SeverityHigh: true}, "high"},
		{"case insensitive", "HIGH", map[analyzers.Severity]bool{analyzers.SeverityHigh: true}, "high"},
		{"mixed case with spaces", "  Medium ,  LOW ", map[analyzers.Severity]bool{analyzers.SeverityMedium: true, analyzers.SeverityLow: true}, "medium,low"},
		{"plus shorthand", "high+", map[analyzers.Severity]bool{analyzers.SeverityCritical: true, analyzers.SeverityHigh: true}, "high+"},
		{"plus shorthand medium", "MEDIUM+", map[analyzers.Severity]bool{analyzers.SeverityCritical: true, analyzers.SeverityHigh: true, analyzers.SeverityMedium: true}, "medium+"},
		{"plus shorthand covers everything at info", "info+", map[analyzers.Severity]bool{analyzers.SeverityCritical: true, analyzers.SeverityHigh: true, analyzers.SeverityMedium: true, analyzers.SeverityLow: true, analyzers.SeverityInfo: true}, "info+"},
		{"explicit list", "medium,low", map[analyzers.Severity]bool{analyzers.SeverityMedium: true, analyzers.SeverityLow: true}, "medium,low"},
		{"plus combined with explicit", "high+,low", map[analyzers.Severity]bool{analyzers.SeverityCritical: true, analyzers.SeverityHigh: true, analyzers.SeverityLow: true}, "critical,high,low"},
		{"duplicates collapse", "high,high", map[analyzers.Severity]bool{analyzers.SeverityHigh: true}, "high"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			spec, err := ParseSeveritySpec(item.spec)
			if err != nil {
				t.Fatalf("parse %q: %v", item.spec, err)
			}
			if spec.Label != item.wantLabel {
				t.Errorf("label = %q, want %q", spec.Label, item.wantLabel)
			}
			for _, severity := range all {
				if got := spec.Matches(severity); got != item.want[severity] {
					t.Errorf("Matches(%s) = %v, want %v", severity, got, item.want[severity])
				}
			}
		})
	}
}

func TestParseSeveritySpecRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name string
		spec string
		want string
	}{
		{"unknown severity", "bogus", `unknown severity "bogus"`},
		{"unknown severity in list", "high,bogus", `unknown severity "bogus"`},
		{"double plus", "high++", `unknown severity "high++"`},
		{"bare plus", "+", `unknown severity "+"`},
		{"empty spec", "", "no severities given"},
		{"separators only", " , , ", "no severities given"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := ParseSeveritySpec(item.spec)
			if err == nil {
				t.Fatalf("parse %q accepted", item.spec)
			}
			if !strings.Contains(err.Error(), item.want) {
				t.Fatalf("error = %q, want substring %q", err.Error(), item.want)
			}
		})
	}
}

func TestEvaluateGate(t *testing.T) {
	high := mustSpec(t, "high")
	highPlus := mustSpec(t, "high+")
	cases := []struct {
		name        string
		findings    []analyzers.Finding
		gate        GateConfig
		wantFailed  bool
		wantHits    int
		wantOpen    int
		wantMessage string
	}{
		{
			name:       "threshold hit",
			findings:   gateFindings(nil, analyzers.SeverityHigh, analyzers.SeverityLow),
			gate:       GateConfig{FailOn: highPlus},
			wantFailed: true, wantHits: 1, wantOpen: 2,
			wantMessage: "fail: 1 finding(s) at or above high (gate: --fail-on high+)",
		},
		{
			name:       "threshold miss passes",
			findings:   gateFindings(nil, analyzers.SeverityLow, analyzers.SeverityInfo),
			gate:       GateConfig{FailOn: highPlus},
			wantFailed: false, wantHits: 0, wantOpen: 2,
		},
		{
			name:       "exact set does not include higher severities",
			findings:   gateFindings(nil, analyzers.SeverityCritical),
			gate:       GateConfig{FailOn: high},
			wantFailed: false, wantHits: 0, wantOpen: 1,
		},
		{
			name:       "fixed findings excluded from threshold",
			findings:   gateFindings(map[int]string{0: "fixed"}, analyzers.SeverityHigh, analyzers.SeverityHigh),
			gate:       GateConfig{FailOn: high},
			wantFailed: true, wantHits: 1, wantOpen: 1,
			wantMessage: "fail: 1 finding(s) of severity high (gate: --fail-on high)",
		},
		{
			name:       "fixed findings excluded from max",
			findings:   gateFindings(map[int]string{0: "fixed"}, analyzers.SeverityCritical, analyzers.SeverityLow, analyzers.SeverityLow),
			gate:       GateConfig{MaxFindings: 2},
			wantFailed: false, wantHits: 0, wantOpen: 2,
		},
		{
			name:       "new and persistent statuses count",
			findings:   gateFindings(map[int]string{0: "new", 1: "persistent", 2: "fixed"}, analyzers.SeverityHigh, analyzers.SeverityHigh, analyzers.SeverityHigh),
			gate:       GateConfig{FailOn: high},
			wantFailed: true, wantHits: 2, wantOpen: 2,
			wantMessage: "fail: 2 finding(s) of severity high (gate: --fail-on high)",
		},
		{
			name:       "max findings exceeded",
			findings:   gateFindings(nil, analyzers.SeverityLow, analyzers.SeverityLow, analyzers.SeverityLow),
			gate:       GateConfig{MaxFindings: 2},
			wantFailed: true, wantHits: 0, wantOpen: 3,
			wantMessage: "fail: 3 finding(s) total exceeds the maximum of 2 (gate: --max-findings 2)",
		},
		{
			name:       "max findings at the limit passes",
			findings:   gateFindings(nil, analyzers.SeverityLow, analyzers.SeverityLow),
			gate:       GateConfig{MaxFindings: 2},
			wantFailed: false, wantHits: 0, wantOpen: 2,
		},
		{
			name:       "both flags either can trip",
			findings:   gateFindings(nil, analyzers.SeverityLow, analyzers.SeverityLow, analyzers.SeverityLow),
			gate:       GateConfig{FailOn: high, MaxFindings: 2},
			wantFailed: true, wantHits: 0, wantOpen: 3,
			wantMessage: "fail: 3 finding(s) total exceeds the maximum of 2 (gate: --fail-on high, --max-findings 2)",
		},
		{
			name:       "both flags both tripped",
			findings:   gateFindings(nil, analyzers.SeverityCritical, analyzers.SeverityLow, analyzers.SeverityLow),
			gate:       GateConfig{FailOn: highPlus, MaxFindings: 2},
			wantFailed: true, wantHits: 1, wantOpen: 3,
			wantMessage: "fail: 1 finding(s) at or above high; 3 finding(s) total exceeds the maximum of 2 (gate: --fail-on high+, --max-findings 2)",
		},
		{
			name:       "no flags always passes",
			findings:   gateFindings(nil, analyzers.SeverityCritical, analyzers.SeverityHigh, analyzers.SeverityMedium),
			gate:       GateConfig{},
			wantFailed: false, wantHits: 0, wantOpen: 3,
		},
		{
			name:       "no findings passes",
			findings:   nil,
			gate:       GateConfig{FailOn: highPlus, MaxFindings: 1},
			wantFailed: false, wantHits: 0, wantOpen: 0,
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			result := EvaluateGate(item.findings, item.gate)
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

func TestGateConfigEnabled(t *testing.T) {
	if (GateConfig{}).Enabled() {
		t.Error("zero gate must be disabled")
	}
	if !(GateConfig{FailOn: mustSpec(t, "low")}).Enabled() {
		t.Error("fail-on alone must enable the gate")
	}
	if !(GateConfig{MaxFindings: 1}).Enabled() {
		t.Error("max-findings alone must enable the gate")
	}
}

func mustSpec(t *testing.T, spec string) SeveritySpec {
	t.Helper()
	parsed, err := ParseSeveritySpec(spec)
	if err != nil {
		t.Fatalf("parse %q: %v", spec, err)
	}
	return parsed
}

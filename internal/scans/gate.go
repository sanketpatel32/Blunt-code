package scans

import (
	"fmt"
	"strings"

	"bluntcode/internal/analyzers"
)

// CI gate for `bluntcode scan`: --fail-on (severity threshold) and
// --max-findings (total cap). Everything here is pure so the CLI wiring in
// cmd/bluntcode stays free of decision logic.

// gateSeverityOrder lists the gate severities from most to least severe; the
// index doubles as the rank used for the "X and above" (+) expansion.
var gateSeverityOrder = []analyzers.Severity{
	analyzers.SeverityCritical,
	analyzers.SeverityHigh,
	analyzers.SeverityMedium,
	analyzers.SeverityLow,
	analyzers.SeverityInfo,
}

// statusFixed mirrors the comparison status the database pins on rows of the
// fixed-findings view (internal/database). Rows loaded for a scan itself carry
// no status, but the gate excludes "fixed" defensively so a mixed list of open
// and resolved findings can never trip — or pass — on resolved issues.
// StatusSuppressed is excluded for the same reason: callers filter suppressed
// findings before the gate, and a stray suppressed row must never count.
const statusFixed = "fixed"

// SeveritySpec is a parsed --fail-on value: a severity set plus the canonical
// label echoed back in gate messages. The set is a bitmask over
// gateSeverityOrder (bit i = rank i), keeping the struct comparable.
type SeveritySpec struct {
	mask  uint8
	Label string
}

// Matches reports whether the severity belongs to the set.
func (s SeveritySpec) Matches(severity analyzers.Severity) bool {
	for i, candidate := range gateSeverityOrder {
		if candidate == severity {
			return s.mask&(1<<i) != 0
		}
	}
	return false
}

// Empty reports whether the set selects no severity at all.
func (s SeveritySpec) Empty() bool { return s.mask == 0 }

// ParseSeveritySpec parses a comma-separated severity list, case-insensitive,
// where a trailing "+" on a name means "this severity and above" (for example
// high+ = critical and high). Whitespace around names is ignored.
func ParseSeveritySpec(spec string) (SeveritySpec, error) {
	parsed := SeveritySpec{}
	for _, field := range strings.Split(spec, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		plus := strings.HasSuffix(field, "+")
		name := strings.ToLower(strings.TrimSuffix(field, "+"))
		rank := -1
		for i, candidate := range gateSeverityOrder {
			if string(candidate) == name {
				rank = i
				break
			}
		}
		if rank < 0 {
			return SeveritySpec{}, fmt.Errorf("unknown severity %q (valid: critical, high, medium, low, info)", field)
		}
		if plus {
			parsed.mask |= (1 << (rank + 1)) - 1 // this severity and everything above
		} else {
			parsed.mask |= 1 << rank
		}
	}
	if parsed.mask == 0 {
		return SeveritySpec{}, fmt.Errorf("no severities given (valid: critical, high, medium, low, info)")
	}
	parsed.Label = canonicalSeverityLabel(parsed.mask)
	return parsed, nil
}

// canonicalSeverityLabel renders a mask as "base+" when it is exactly the
// "base and above" closure, otherwise as the member names in severity order
// (for example "medium,low").
func canonicalSeverityLabel(mask uint8) string {
	closure := uint8(0)
	for i, severity := range gateSeverityOrder {
		closure |= 1 << i
		if mask == closure {
			return string(severity) + "+"
		}
	}
	names := make([]string, 0, len(gateSeverityOrder))
	for i, severity := range gateSeverityOrder {
		if mask&(1<<i) != 0 {
			names = append(names, string(severity))
		}
	}
	return strings.Join(names, ",")
}

// describe renders the human phrase for gate failure messages: closures read
// "at or above high", explicit sets read "of severity medium, low".
func (s SeveritySpec) describe() string {
	closure := uint8(0)
	for i, severity := range gateSeverityOrder {
		closure |= 1 << i
		if s.mask == closure {
			return "at or above " + string(severity)
		}
	}
	names := make([]string, 0, len(gateSeverityOrder))
	for i, severity := range gateSeverityOrder {
		if s.mask&(1<<i) != 0 {
			names = append(names, string(severity))
		}
	}
	return "of severity " + strings.Join(names, ", ")
}

// GateConfig carries the parsed CI gate flags of `bluntcode scan`. The zero
// value disables the gate entirely.
type GateConfig struct {
	FailOn      SeveritySpec
	MaxFindings int // 0 = no maximum
}

// Enabled reports whether either gate flag was supplied.
func (g GateConfig) Enabled() bool { return !g.FailOn.Empty() || g.MaxFindings > 0 }

// GateResult is the gate decision for one scan.
type GateResult struct {
	Gate          GateConfig
	Failed        bool
	ThresholdHits int // open findings whose severity is in the --fail-on set
	OpenTotal     int // open findings counted toward --max-findings
}

// EvaluateGate counts a scan's open findings against the gate. Findings whose
// Status is "fixed" are resolved issues that only exist in the comparison
// view, and "suppressed" findings were dismissed via fingerprint suppression;
// both are excluded so the gate counts problems the scan still reports. The
// gate trips when any open finding matches the --fail-on set or the open
// total exceeds --max-findings.
func EvaluateGate(findings []analyzers.Finding, gate GateConfig) GateResult {
	result := GateResult{Gate: gate}
	for _, finding := range findings {
		if finding.Status == statusFixed || finding.Status == StatusSuppressed {
			continue
		}
		result.OpenTotal++
		if gate.FailOn.Matches(finding.Severity) {
			result.ThresholdHits++
		}
	}
	if !gate.FailOn.Empty() && result.ThresholdHits > 0 {
		result.Failed = true
	}
	if gate.MaxFindings > 0 && result.OpenTotal > gate.MaxFindings {
		result.Failed = true
	}
	return result
}

// FailureMessage renders the one-line stderr explanation of a tripped gate,
// for example `fail: 12 finding(s) at or above high (gate: --fail-on high+)`.
// It returns the empty string for a passing gate.
func (r GateResult) FailureMessage() string {
	if !r.Failed {
		return ""
	}
	var clauses, gates []string
	if !r.Gate.FailOn.Empty() && r.ThresholdHits > 0 {
		clauses = append(clauses, fmt.Sprintf("%d finding(s) %s", r.ThresholdHits, r.Gate.FailOn.describe()))
	}
	if r.Gate.MaxFindings > 0 && r.OpenTotal > r.Gate.MaxFindings {
		clauses = append(clauses, fmt.Sprintf("%d finding(s) total exceeds the maximum of %d", r.OpenTotal, r.Gate.MaxFindings))
	}
	if !r.Gate.FailOn.Empty() {
		gates = append(gates, "--fail-on "+r.Gate.FailOn.Label)
	}
	if r.Gate.MaxFindings > 0 {
		gates = append(gates, fmt.Sprintf("--max-findings %d", r.Gate.MaxFindings))
	}
	return "fail: " + strings.Join(clauses, "; ") + " (gate: " + strings.Join(gates, ", ") + ")"
}

package scans

import (
	"io"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/reports"
)

// Baseline support for the `bluntcode scan` CI gate: findings whose
// fingerprints the baseline already knows are excluded from gate counting, so
// a team can adopt --fail-on/--max-findings without failing on pre-existing
// debt. Everything here is pure; the CLI resolves the baseline reference
// (scan ID or SARIF path) and applies Split before EvaluateGate.

// Baseline is a set of finding fingerprints treated as already known. The
// zero value knows nothing, which makes every finding new — exactly the
// historical gate behavior.
type Baseline struct {
	fingerprints map[string]bool
}

// BaselineFromFindings builds a baseline from a previous scan's findings.
// Findings without a fingerprint are ignored: they can never be matched.
func BaselineFromFindings(findings []analyzers.Finding) Baseline {
	set := make(map[string]bool, len(findings))
	for _, finding := range findings {
		if finding.Fingerprint != "" {
			set[finding.Fingerprint] = true
		}
	}
	return Baseline{fingerprints: set}
}

// BaselineFromSARIF builds a baseline from a SARIF 2.1.0 log — Blunt Code's own
// export shape (reports.SARIF emits each finding's fingerprint) or any
// third-party log whose results carry fingerprints.
func BaselineFromSARIF(r io.Reader) (Baseline, error) {
	results, err := reports.ReadSARIF(r)
	if err != nil {
		return Baseline{}, err
	}
	set := make(map[string]bool, len(results))
	for _, result := range results {
		if result.Fingerprint != "" {
			set[result.Fingerprint] = true
		}
	}
	return Baseline{fingerprints: set}, nil
}

// Len reports how many distinct fingerprints the baseline knows.
func (b Baseline) Len() int { return len(b.fingerprints) }

// Known reports whether a fingerprint is part of the baseline. An empty
// fingerprint is never known.
func (b Baseline) Known(fingerprint string) bool {
	return fingerprint != "" && b.fingerprints[fingerprint]
}

// Split partitions findings into known (their fingerprints are in the
// baseline) and fresh (new since the baseline). The zero Baseline puts
// everything in fresh. Resolution status is not considered here; fixed rows
// only exist in comparison views and EvaluateGate excludes them defensively
// from whichever side they land on.
func (b Baseline) Split(findings []analyzers.Finding) (known, fresh []analyzers.Finding) {
	for _, finding := range findings {
		if b.Known(finding.Fingerprint) {
			known = append(known, finding)
			continue
		}
		fresh = append(fresh, finding)
	}
	return known, fresh
}

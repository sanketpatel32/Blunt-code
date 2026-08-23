package scans

import "bluntcode/internal/analyzers"

// Fingerprint-based suppression shared by the scan pipeline, report model
// builders, and the CI gate. The database stores one suppression per
// (workspace, fingerprint); everything here is pure so every consumer applies
// the identical rule.

// StatusSuppressed is the status findings carry when their fingerprint is
// suppressed for the workspace. It outranks the comparison statuses: a
// dismissed finding is never reported as new or persistent.
const StatusSuppressed = "suppressed"

// FilterSuppressed removes findings whose fingerprints are in the suppression
// set, preserving order. It is the single filter point every report/model
// builder (scan-time Markdown, the API report model, the CLI JSON document)
// and the CLI CI gate pass through, so no renderer or gate can count a
// dismissed finding. Suppressed findings remain stored — the findings list
// surfaces them with StatusSuppressed via the database status expression.
func FilterSuppressed(findings []analyzers.Finding, suppressed map[string]bool) []analyzers.Finding {
	if len(suppressed) == 0 {
		return findings
	}
	out := make([]analyzers.Finding, 0, len(findings))
	for _, finding := range findings {
		if suppressed[finding.Fingerprint] {
			continue
		}
		out = append(out, finding)
	}
	return out
}

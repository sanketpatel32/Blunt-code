package reports

import (
	"bytes"
	"encoding/json"
)

// JSONL renders the findings as newline-delimited JSON: one self-contained
// finding object per line, LF-terminated, ready for log pipelines and tools
// like jq. The scan header block of the JSON export is intentionally absent —
// consumers that need it should read findings.json instead. Ordering matches
// the JSON export (path, line, rule) and each row carries the same derived
// new/persistent status the other exports show. Suppressed filtering is the
// caller's responsibility, mirroring CSV.
func JSONL(m Model) []byte {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	persistent, appeared := comparisonFingerprints(m.Comparison)
	for _, f := range sortedFindingsJSON(m.Findings) {
		if len(persistent) > 0 || len(appeared) > 0 {
			f.Status = derivedStatus(f, persistent, appeared)
		}
		if err := encoder.Encode(f); err != nil {
			break // bytes.Buffer writes never fail; unreachable by construction
		}
	}
	return buf.Bytes()
}

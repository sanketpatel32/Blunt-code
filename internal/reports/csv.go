package reports

import (
	"bytes"
	"encoding/csv"
	"strconv"
)

// csvBOM prefixes CSV exports so Excel on Windows detects UTF-8.
const csvBOM = "\xef\xbb\xbf"

// CSVHeader is the fixed export column order; CSV and the row writer must stay in sync.
var CSVHeader = []string{"severity", "category", "analyzer", "rule_id", "title", "message", "file", "line", "column", "end_line", "status", "remediation", "documentation_url"}

// csvCell neutralizes CSV formula injection and strips hostile control bytes.
func csvCell(value string) string {
	value = scrubControls(value)
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	}
	return value
}

func csvNumber(value int) string {
	if value == 0 {
		return ""
	}
	return strconv.Itoa(value)
}

// CSV renders the report model as a spreadsheet-friendly CSV with UTF-8 BOM.
// It mirrors the API GET /api/v1/scans/{id}/findings.csv export: same header,
// same cell transforms, same status derivation, LF terminators. Suppressed
// filtering is the caller's responsibility (the CLI's summary findings are
// already filtered).
func CSV(m Model) []byte {
	var buf bytes.Buffer
	_, _ = buf.WriteString(csvBOM)
	w := csv.NewWriter(&buf)
	_ = w.Write(CSVHeader)
	persistent, appeared := comparisonFingerprints(m.Comparison)
	for _, f := range m.Findings {
		status := derivedStatus(f, persistent, appeared)
		_ = w.Write([]string{
			csvCell(string(f.Severity)), csvCell(string(f.Category)), csvCell(f.AnalyzerID), csvCell(f.RuleID), csvCell(f.Title), csvCell(f.Message), csvCell(f.RelativePath),
			csvNumber(f.StartLine), csvNumber(f.StartColumn), csvNumber(f.EndLine), csvCell(status), csvCell(f.Remediation), csvCell(f.DocumentationURL),
		})
	}
	w.Flush()
	return buf.Bytes()
}

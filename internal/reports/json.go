package reports

import (
	"bluntcode/internal/analyzers"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The JSON export is the machine-readable twin of the Markdown report: the
// same model rendered as a stable, versioned document for CI tooling and
// editor integrations. schemaVersion must be bumped whenever a field is
// removed or changes meaning; purely additive fields keep version 1.
const (
	JSONSchema        = "bluntcode/scan-report"
	JSONSchemaVersion = 1
)

// The document is built only from structs with fixed field order plus sorted
// slices (never maps), so two renders of identical input are byte-identical
// and consumers can diff exports without normalizing first.
type jsonReport struct {
	Schema        string             `json:"schema"`
	SchemaVersion int                `json:"schemaVersion"`
	Workspace     jsonWorkspace      `json:"workspace"`
	Scan          jsonScan           `json:"scan"`
	Files         jsonFiles          `json:"files"`
	Severity      jsonSeverityCounts `json:"severity"`
	Analyzers     []jsonAnalyzerRun  `json:"analyzers"`
	Findings      []jsonFinding      `json:"findings"`
	Metrics       []jsonMetric       `json:"metrics"`
	Comparison    jsonComparison     `json:"comparison"`
	Warnings      []string           `json:"warnings"`
}
type jsonWorkspace struct {
	Name string `json:"name"`
	Path string `json:"path"`
}
type jsonScan struct {
	ID               string `json:"id"`
	Profile          string `json:"profile"`
	State            string `json:"state"`
	StartedAt        string `json:"started_at"`
	FinishedAt       string `json:"finished_at"`
	BluntCodeVersion string `json:"bluntcode_version,omitempty"`
}
type jsonFiles struct {
	Candidate int `json:"candidate"`
	Selected  int `json:"selected"`
	Skipped   int `json:"skipped"`
}
type jsonSeverityCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
	Total    int `json:"total"`
}
type jsonAnalyzerRun struct {
	ID         string `json:"id"`
	Version    string `json:"version,omitempty"`
	State      string `json:"state"`
	Findings   int    `json:"findings"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}
type jsonFinding struct {
	Severity         string `json:"severity"`
	Category         string `json:"category"`
	AnalyzerID       string `json:"analyzer_id"`
	RuleID           string `json:"rule_id"`
	Title            string `json:"title"`
	Message          string `json:"message"`
	File             string `json:"file"`
	StartLine        int    `json:"start_line,omitempty"`
	StartColumn      int    `json:"start_column,omitempty"`
	EndLine          int    `json:"end_line,omitempty"`
	EndColumn        int    `json:"end_column,omitempty"`
	Status           string `json:"status,omitempty"`
	RawSeverity      string `json:"raw_severity,omitempty"`
	Remediation      string `json:"remediation,omitempty"`
	DocumentationURL string `json:"documentation_url,omitempty"`
	Fingerprint      string `json:"fingerprint,omitempty"`
}
type jsonMetric struct {
	AnalyzerID string  `json:"analyzer_id"`
	Key        string  `json:"key"`
	Label      string  `json:"label,omitempty"`
	Value      float64 `json:"value"`
	Unit       string  `json:"unit,omitempty"`
}
type jsonComparison struct {
	New        int `json:"new"`
	Fixed      int `json:"fixed"`
	Persistent int `json:"persistent"`
}

// JSON renders the report model as the versioned JSON scan document served by
// GET /api/v1/scans/{id}/findings.json and printed by `bluntcode scan
// --format json`. Like Markdown and HTML it is infallible: the document holds
// only strings, numbers, and slices thereof, which json.MarshalIndent always
// accepts (the fallback below keeps that guarantee even if a future field
// breaks it).
//
// Determinism: findings are sorted by path, start line, then rule id, metrics
// by analyzer and key, and severity counts are a fixed struct rather than a
// map, so identical input yields identical bytes.
//
// Analyzer-derived strings pass through scrubControls exactly like the SARIF
// and HTML exports: encoding/json escapes the C0 controls but writes DEL
// through raw, so a hostile finding message must not reach the export
// unescaped. Tab, LF, and CR survive because JSON quotes them losslessly.
func JSON(m Model) []byte {
	doc := jsonReport{
		Schema:        JSONSchema,
		SchemaVersion: JSONSchemaVersion,
		Workspace:     jsonWorkspace{Name: scrubControls(m.WorkspaceName), Path: scrubControls(m.WorkspacePath)},
		Scan: jsonScan{
			ID: scrubControls(m.ScanID), Profile: scrubControls(m.Profile), State: scrubControls(m.State),
			StartedAt: jsonTime(m.StartedAt), FinishedAt: jsonTime(m.FinishedAt), BluntCodeVersion: scrubControls(m.BluntCodeVersion),
		},
		Files:      jsonFiles{Candidate: len(m.Files) + len(m.SkippedFiles), Selected: len(m.Files), Skipped: len(m.SkippedFiles)},
		Severity:   severityCountsJSON(m),
		Analyzers:  make([]jsonAnalyzerRun, 0, len(m.Runs)),
		Findings:   make([]jsonFinding, 0, len(m.Findings)),
		Metrics:    make([]jsonMetric, 0, len(m.Metrics)),
		Comparison: jsonComparison{New: len(m.Comparison.New), Fixed: len(m.Comparison.Fixed), Persistent: len(m.Comparison.Persistent)},
		Warnings:   make([]string, 0, len(m.Warnings)),
	}
	for _, r := range m.Runs {
		doc.Analyzers = append(doc.Analyzers, jsonAnalyzerRun{
			ID: scrubControls(r.AnalyzerID), Version: scrubControls(r.Version), State: scrubControls(r.State),
			Findings: r.FindingCount, DurationMS: r.Duration.Milliseconds(), Error: scrubControls(strings.TrimSpace(r.ErrorSummary)),
		})
	}
	persistent, appeared := comparisonFingerprints(m.Comparison)
	for _, f := range sortedFindingsJSON(m.Findings) {
		doc.Findings = append(doc.Findings, jsonFinding{
			Severity: string(f.Severity), Category: string(f.Category),
			AnalyzerID: scrubControls(f.AnalyzerID), RuleID: scrubControls(f.RuleID),
			Title: scrubControls(f.Title), Message: scrubControls(f.Message), File: scrubControls(jsonFilePath(f.RelativePath)),
			StartLine: f.StartLine, StartColumn: f.StartColumn, EndLine: f.EndLine, EndColumn: f.EndColumn,
			Status: derivedStatus(f, persistent, appeared), RawSeverity: scrubControls(f.RawSeverity),
			Remediation: scrubControls(strings.TrimSpace(f.Remediation)), DocumentationURL: scrubControls(strings.TrimSpace(f.DocumentationURL)),
			Fingerprint: scrubControls(strings.TrimSpace(f.Fingerprint)),
		})
	}
	for _, metric := range sortedMetrics(m.Metrics) {
		doc.Metrics = append(doc.Metrics, jsonMetric{
			AnalyzerID: scrubControls(metric.AnalyzerID), Key: scrubControls(metric.Key), Label: scrubControls(metric.Label),
			Value: metric.Value, Unit: scrubControls(metric.Unit),
		})
	}
	for _, warning := range m.Warnings {
		doc.Warnings = append(doc.Warnings, scrubControls(warning))
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		// Unreachable with the field types above; kept so JSON stays infallible
		// like Markdown and HTML no matter how the document evolves.
		return []byte("{\n  \"schema\": \"bluntcode/scan-report\",\n  \"schemaVersion\": 1,\n  \"findings\": []\n}\n")
	}
	// json.MarshalIndent emits LF-only indentation and no trailing newline;
	// the export is one JSON document per file terminated by a single LF.
	return append(data, '\n')
}

// severityCountsJSON projects the model's severity map onto the fixed struct
// so the document always carries all five counts in a stable order.
func severityCountsJSON(m Model) jsonSeverityCounts {
	return jsonSeverityCounts{
		Critical: m.Counts[analyzers.SeverityCritical],
		High:     m.Counts[analyzers.SeverityHigh],
		Medium:   m.Counts[analyzers.SeverityMedium],
		Low:      m.Counts[analyzers.SeverityLow],
		Info:     m.Counts[analyzers.SeverityInfo],
		Total:    len(m.Findings),
	}
}

// sortedMetrics orders metrics by analyzer id then key so identical input
// renders identical bytes regardless of how the metrics slice was assembled.
func sortedMetrics(metrics []analyzers.Metric) []analyzers.Metric {
	out := append([]analyzers.Metric(nil), metrics...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AnalyzerID != out[j].AnalyzerID {
			return out[i].AnalyzerID < out[j].AnalyzerID
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// sortedFindingsJSON orders findings by the file value the export actually
// carries (slash-normalized), then start line, then rule id. Sorting on the
// normalized path matters: raw stored paths may mix separators, and '\' sorts
// after '/' would make the exported order disagree with the exported files.
func sortedFindingsJSON(findings []analyzers.Finding) []analyzers.Finding {
	out := append([]analyzers.Finding(nil), findings...)
	sort.SliceStable(out, func(i, j int) bool {
		pathI, pathJ := jsonFilePath(out[i].RelativePath), jsonFilePath(out[j].RelativePath)
		if pathI != pathJ {
			return pathI < pathJ
		}
		if out[i].StartLine != out[j].StartLine {
			return out[i].StartLine < out[j].StartLine
		}
		return out[i].RuleID < out[j].RuleID
	})
	return out
}

// comparisonFingerprints indexes the comparison buckets a finding's status is
// derived from: persistent holds the fingerprints the previous completed scan
// also had, and appeared holds every fingerprint the comparison classified at
// all. The lookup sets are built per render, never serialized, so the document
// itself stays map-free and byte-stable.
func comparisonFingerprints(comparison Comparison) (persistent, appeared map[string]bool) {
	persistent = make(map[string]bool, len(comparison.Persistent))
	appeared = make(map[string]bool, len(comparison.New)+len(comparison.Persistent))
	for _, f := range comparison.Persistent {
		persistent[f.Fingerprint] = true
		appeared[f.Fingerprint] = true
	}
	for _, f := range comparison.New {
		appeared[f.Fingerprint] = true
	}
	return persistent, appeared
}

// derivedStatus fills a finding's comparison status when the loader did not
// populate it. The loaders behind the Markdown/SARIF/JSON model return
// findings without a status column, but the model already carries the
// comparison buckets, so the same rule the CSV export's SQL applies —
// persistent when a fingerprint also existed in the previous completed scan,
// new otherwise — is derived here instead of a second query. An explicit
// Status always wins; without a fingerprint or comparison the field stays
// empty rather than guessed.
func derivedStatus(f analyzers.Finding, persistent, appeared map[string]bool) string {
	if f.Status != "" {
		return scrubControls(f.Status)
	}
	if f.Fingerprint == "" || !appeared[f.Fingerprint] {
		return ""
	}
	if persistent[f.Fingerprint] {
		return "persistent"
	}
	return "new"
}

// jsonTime renders report timestamps as RFC 3339 UTC strings, matching the
// CLI summary's convention; a zero time exports as an empty string.
func jsonTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// jsonFilePath normalizes a stored relative path to forward slashes without a
// leading "./" so the document is platform-stable, mirroring the SARIF and
// Markdown exports.
func jsonFilePath(path string) string {
	return strings.TrimPrefix(filepath.ToSlash(path), "./")
}

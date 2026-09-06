// Package reports builds one analyzer-independent model for the UI and
// Markdown exporter. It never parses analyzer output.
package reports

import (
	"bluntcode/internal/analyzers"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Run struct {
	AnalyzerID, DisplayName, Version, State, ErrorSummary string
	FindingCount                                          int
	// WarningCount counts degradations that kept the run from full coverage
	// (unparseable output batches); a warned run is not a clean run.
	WarningCount int
	Duration     time.Duration
}
type Comparison struct {
	New, Fixed, Persistent []analyzers.Finding
	UnknownAnalyzerIDs     []string
}
type Input struct {
	WorkspaceName, WorkspacePath, ScanID, Profile, BluntCodeVersion string
	// State is the terminal scan state (for example "completed"); it feeds the
	// JSON export's scan block and is not rendered by the other formats.
	State string
	// Provenance carries the run manifest (IMP-07): digests of the analyzed
	// bytes and shaping configuration, schema versions, platform, and drift.
	// Nil for reports built before snapshots recorded it; renderers skip it.
	Provenance            *Provenance
	StartedAt, FinishedAt time.Time
	Files                 []string
	SkippedFiles          []string
	Findings              []analyzers.Finding
	Metrics               []analyzers.Metric
	Runs                  []Run
	Comparison            Comparison
}

// Provenance is the compact identity of one scan run: what bytes were
// analyzed, what configuration shaped the selection, which schema versions
// classified inputs and identified findings, and where it ran. Two scans
// with equal provenance are comparable; anything that differs is visible
// instead of implied. Absolute paths and full file lists stay out on
// purpose — the digests stand in for them.
type Provenance struct {
	InputDigest            string
	ConfigDigest           string
	GitCommit              string
	GitDirty               bool
	DiscoveryPolicyVersion int
	FingerprintVersion     int
	Platform               map[string]string
	DriftDetected          bool
}
type Model struct {
	Input
	Counts   map[analyzers.Severity]int
	Partial  bool
	Warnings []string
	Summary  string
	Priority []analyzers.Finding
}

func Build(in Input) Model {
	counts := map[analyzers.Severity]int{}
	for _, f := range in.Findings {
		counts[f.Severity]++
	}
	m := Model{Input: in, Counts: counts, Warnings: []string{}}
	for _, r := range in.Runs {
		if r.State != "completed" && r.State != "success" && r.State != "succeeded" {
			m.Partial = true
			m.Warnings = append(m.Warnings, fmt.Sprintf("%s: %s", display(r), strings.TrimSpace(r.ErrorSummary)))
		}
	}
	for _, id := range in.Comparison.UnknownAnalyzerIDs {
		m.Warnings = append(m.Warnings, fmt.Sprintf("%s did not produce a valid current result; its previous findings are not considered fixed.", id))
	}
	if in.Provenance != nil && in.Provenance.DriftDetected {
		m.Warnings = append(m.Warnings, "Workspace content changed during the scan; results may mix two versions of the tree.")
	}
	m.Priority = priority(in.Findings)
	m.Summary = summary(counts, m.Partial)
	return m
}
func display(r Run) string {
	if r.DisplayName != "" {
		return r.DisplayName
	}
	return r.AnalyzerID
}
func summary(c map[analyzers.Severity]int, partial bool) string {
	var base string
	switch {
	case c[analyzers.SeverityCritical] > 0:
		base = fmt.Sprintf("Immediate attention recommended: %d critical findings were reported by the analyzers that completed.", c[analyzers.SeverityCritical])
	case c[analyzers.SeverityHigh] > 0:
		base = fmt.Sprintf("The project contains %d high-severity findings that should be prioritized.", c[analyzers.SeverityHigh])
	default:
		base = "No critical findings were reported by the analyzers that completed. Review medium and low findings for maintainability improvements."
	}
	if partial {
		return base + " Analysis completeness is partial."
	}
	return base
}
func priority(in []analyzers.Finding) []analyzers.Finding {
	out := append([]analyzers.Finding(nil), in...)
	rank := map[analyzers.Severity]int{analyzers.SeverityCritical: 0, analyzers.SeverityHigh: 1, analyzers.SeverityMedium: 2, analyzers.SeverityLow: 3, analyzers.SeverityInfo: 4}
	sort.SliceStable(out, func(i, j int) bool {
		if rank[out[i].Severity] != rank[out[j].Severity] {
			return rank[out[i].Severity] < rank[out[j].Severity]
		}
		return out[i].RelativePath < out[j].RelativePath
	})
	if len(out) > 20 {
		return out[:20]
	}
	return out
}

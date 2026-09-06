package scans

import (
	"sort"

	"bluntcode/internal/analyzers"
)

// ComparisonCoverage states what the current scan actually evaluated: which
// analyzers completed a successful run, and which files were part of the
// evaluated input set (normalized workspace-relative paths). A previous
// finding is classified Fixed only when both its analyzer succeeded and its
// file was in scope; anything less is NotEvaluated, never a silent fix.
type ComparisonCoverage struct {
	Succeeded      map[string]bool
	EvaluatedFiles map[string]bool
}

// NewComparisonCoverage builds coverage from the successful analyzer ids and
// the workspace-relative selected paths of the current scan. An empty path
// list leaves EvaluatedFiles nil: the comparison then falls back to the
// analyzer-only contract instead of declaring every file out of scope.
func NewComparisonCoverage(succeeded map[string]bool, selectedRelPaths []string) ComparisonCoverage {
	var evaluated map[string]bool
	for _, path := range selectedRelPaths {
		if path == "" {
			continue
		}
		if evaluated == nil {
			evaluated = make(map[string]bool, len(selectedRelPaths))
		}
		evaluated[normalizeReusePath(path)] = true
	}
	return ComparisonCoverage{Succeeded: succeeded, EvaluatedFiles: evaluated}
}

type Comparison struct {
	New, Fixed, Persistent []analyzers.Finding
	// NotEvaluatedAnalyzerIDs lists analyzers whose previous findings could
	// not be classified: the analyzer did not complete a successful run in
	// this scan, or the finding's file was not part of this scan's inputs
	// (excluded, deselected, or deleted-and-renamed-away).
	NotEvaluatedAnalyzerIDs []string
}

// Compare is coverage-aware: previous findings are fixed only when that
// exact analyzer completed successfully in the current scan AND the finding's
// file was among the inputs it evaluated. Everything else lands in
// NotEvaluatedAnalyzerIDs so a partial scan cannot launder old findings as
// fixed.
func Compare(current, previous []analyzers.Finding, coverage ComparisonCoverage) Comparison {
	old := map[string]analyzers.Finding{}
	for _, finding := range previous {
		old[finding.Fingerprint] = finding
	}
	now := map[string]analyzers.Finding{}
	for _, finding := range current {
		now[finding.Fingerprint] = finding
	}
	result := Comparison{}
	notEvaluated := map[string]bool{}
	for fingerprint, finding := range now {
		if _, ok := old[fingerprint]; ok {
			result.Persistent = append(result.Persistent, finding)
		} else {
			result.New = append(result.New, finding)
		}
	}
	for fingerprint, finding := range old {
		if _, ok := now[fingerprint]; ok {
			continue
		}
		if coverage.Succeeded[finding.AnalyzerID] && coverage.evaluated(finding) {
			result.Fixed = append(result.Fixed, finding)
		} else {
			notEvaluated[finding.AnalyzerID] = true
		}
	}
	for id := range notEvaluated {
		result.NotEvaluatedAnalyzerIDs = append(result.NotEvaluatedAnalyzerIDs, id)
	}
	sort.Strings(result.NotEvaluatedAnalyzerIDs)
	return result
}

// evaluated reports whether a previous finding's location was part of this
// scan's input set. Project-level (path-less) findings follow their analyzer:
// a successful run re-evaluated the whole project, so they can be fixed;
// per-file findings additionally need their file selected. A nil
// EvaluatedFiles (older callers, tests) keeps the analyzer-only contract.
func (c ComparisonCoverage) evaluated(finding analyzers.Finding) bool {
	if finding.RelativePath == "" || c.EvaluatedFiles == nil {
		return true
	}
	return c.EvaluatedFiles[normalizeReusePath(finding.RelativePath)]
}

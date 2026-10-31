package reports

import (
	"fmt"

	"bluntcode/internal/analyzers"
)

// Grade computes the weighted risk score and letter the app shows (web
// lib/risk.ts and the API risk profile): critical ×10, high ×5, medium ×2,
// low ×1; A <5, B <20, C <50, D otherwise.
func Grade(counts map[analyzers.Severity]int) (score int, grade, label string) {
	score = counts[analyzers.SeverityCritical]*10 + counts[analyzers.SeverityHigh]*5 + counts[analyzers.SeverityMedium]*2 + counts[analyzers.SeverityLow]
	switch {
	case score < 5:
		return score, "A", "Low risk"
	case score < 20:
		return score, "B", "Moderate risk"
	case score < 50:
		return score, "C", "High risk"
	default:
		return score, "D", "Critical risk"
	}
}

// CoverageLine is the app's one coverage sentence, "7 of 9 analyzers ran",
// with the runs that did not finish cleanly called out.
func CoverageLine(runs []Run) string {
	if len(runs) == 0 {
		return ""
	}
	succeeded, warned := 0, 0
	for _, r := range runs {
		if r.State == "succeeded" {
			succeeded++
			if r.WarningCount > 0 {
				warned++
			}
		}
	}
	noun := "analyzers"
	if len(runs) == 1 {
		noun = "analyzer"
	}
	line := fmt.Sprintf("%d of %d %s ran", succeeded, len(runs), noun)
	if failed := len(runs) - succeeded; failed > 0 {
		line += fmt.Sprintf(" · %d didn't run", failed)
	}
	if warned > 0 {
		line += fmt.Sprintf(" · %d with warnings", warned)
	}
	return line
}

// coverageComplete reports whether every analyzer ran cleanly.
func coverageComplete(runs []Run) bool {
	for _, r := range runs {
		if r.State != "succeeded" || r.WarningCount > 0 {
			return false
		}
	}
	return len(runs) > 0
}

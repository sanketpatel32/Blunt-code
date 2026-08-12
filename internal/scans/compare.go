package scans

import "bluntcode/internal/analyzers"

type Comparison struct {
	New, Fixed, Persistent []analyzers.Finding
	UnknownAnalyzerIDs     []string
}

// Compare is coverage-aware: previous findings are fixed only when that exact
// analyzer completed successfully in the current scan.
func Compare(current, previous []analyzers.Finding, succeeded map[string]bool) Comparison {
	old := map[string]analyzers.Finding{}
	for _, finding := range previous {
		old[finding.Fingerprint] = finding
	}
	now := map[string]analyzers.Finding{}
	for _, finding := range current {
		now[finding.Fingerprint] = finding
	}
	result := Comparison{}
	unknown := map[string]bool{}
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
		if succeeded[finding.AnalyzerID] {
			result.Fixed = append(result.Fixed, finding)
		} else {
			unknown[finding.AnalyzerID] = true
		}
	}
	for _, id := range []string{"ruff", "biome", "semgrep", "sonarqube"} {
		if unknown[id] {
			result.UnknownAnalyzerIDs = append(result.UnknownAnalyzerIDs, id)
		}
	}
	return result
}

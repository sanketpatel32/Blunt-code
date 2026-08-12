package scans

import (
	"bluntcode/internal/analyzers"
	"testing"
)

func finding(id, fingerprint string) analyzers.Finding {
	return analyzers.Finding{AnalyzerID: id, Fingerprint: fingerprint}
}
func TestComparisonDoesNotMarkFailedAnalyzerFindingFixed(t *testing.T) {
	result := Compare([]analyzers.Finding{finding("ruff", "same"), finding("ruff", "new")}, []analyzers.Finding{finding("ruff", "same"), finding("sonarqube", "sonar-old")}, map[string]bool{"ruff": true})
	if len(result.New) != 1 || len(result.Persistent) != 1 || len(result.Fixed) != 0 || len(result.UnknownAnalyzerIDs) != 1 || result.UnknownAnalyzerIDs[0] != "sonarqube" {
		t.Fatalf("unexpected comparison %#v", result)
	}
}

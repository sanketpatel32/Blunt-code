package reports

import (
	"reflect"
	"testing"
)

// TestBuildWarningsOneLinePerFailedAnalyzer: the stored "ruff: ruff: …" echo
// collapses to "Ruff: …", and a failed analyzer is not repeated as "could
// not be fully evaluated".
func TestBuildWarningsOneLinePerFailedAnalyzer(t *testing.T) {
	m := Build(Input{
		Runs: []Run{
			{AnalyzerID: "ruff", DisplayName: "Ruff", State: "failed", ErrorSummary: "ruff: ruff: No pinned managed artifact is configured."},
			{AnalyzerID: "biome", DisplayName: "Biome", State: "succeeded"},
		},
		Comparison: Comparison{NotEvaluatedAnalyzerIDs: []string{"ruff", "semgrep"}},
	})
	want := []string{
		"Ruff: No pinned managed artifact is configured.",
		"semgrep could not be fully evaluated in this scan (analyzer did not complete successfully, or some of its files were not part of the scan); its previous findings are not considered fixed.",
	}
	if !reflect.DeepEqual(m.Warnings, want) {
		t.Fatalf("warnings = %#v", m.Warnings)
	}
}

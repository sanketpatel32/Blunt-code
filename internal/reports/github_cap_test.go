package reports

import (
	"bytes"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

// TestGitHubAnnotationsWithCapOverridesDisplayLimit proves the custom cap
// changes how many annotations are shown per level and that the truncation
// notice still appears, while the default entry point keeps the historic 10.
func TestGitHubAnnotationsWithCapOverridesDisplayLimit(t *testing.T) {
	newFinding := func(i int) analyzers.Finding {
		return analyzers.Finding{AnalyzerID: "ruff", RuleID: "F401", Severity: analyzers.SeverityCritical, Message: "m", RelativePath: "src/a.py", StartLine: i + 1}
	}
	in := Input{WorkspaceName: "Cap", ScanID: "scan-cap"}
	for i := 0; i < 12; i++ {
		in.Findings = append(in.Findings, newFinding(i))
	}
	model := Build(in)

	capped := GitHubAnnotationsWithCap(model, 3)
	count := bytes.Count(capped, []byte("::error "))
	if count != 3 {
		t.Fatalf("custom cap must show exactly 3 error annotations, got %d", count)
	}
	if !strings.Contains(string(capped), "::notice ") {
		t.Fatalf("truncation notice missing under a custom cap:\n%s", capped)
	}

	defaultOut := GitHubAnnotations(model)
	if got := bytes.Count(defaultOut, []byte("::error ")); got != 10 {
		t.Fatalf("default entry point must keep the historic cap of 10, got %d", got)
	}
}

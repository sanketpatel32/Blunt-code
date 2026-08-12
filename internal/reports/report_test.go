package reports

import (
	"bluntcode/internal/analyzers"
	"strings"
	"testing"
	"time"
)

func TestMarkdownMarksPartialAndRedactsPath(t *testing.T) {
	m := Build(Input{WorkspaceName: "Demo", WorkspacePath: `C:\private`, StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Findings: []analyzers.Finding{{AnalyzerID: "ruff", RuleID: "F401", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryCorrectness, Title: "Unused import", Message: "unused", RelativePath: "src/a.py", StartLine: 2}}, Runs: []Run{{AnalyzerID: "sonarqube", State: "failed", ErrorSummary: "startup timeout"}}})
	s := Markdown(m)
	if !strings.Contains(s, "| Analysis completeness | PARTIAL |") || strings.Contains(s, `C:\private`) {
		t.Fatalf("unexpected markdown: %s", s)
	}
}

func TestSucceededRunIsNotPartial(t *testing.T) {
	m := Build(Input{Runs: []Run{{AnalyzerID: "ruff", State: "succeeded"}}})
	if m.Partial || len(m.Warnings) != 0 {
		t.Fatalf("successful run should not be partial: %#v", m)
	}
}

func TestMarkdownUsesQualityAndFindingsTables(t *testing.T) {
	m := Build(Input{
		WorkspaceName: "Demo", StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Profile: "standard",
		Files: []string{"src/a.ts"}, Metrics: []analyzers.Metric{
			{AnalyzerID: "sonarqube", Key: "ncloc", Value: 42}, {AnalyzerID: "sonarqube", Key: "security_rating", Value: 1}, {AnalyzerID: "sonarqube", Key: "coverage", Value: 87.5},
		},
		Findings: []analyzers.Finding{{AnalyzerID: "sonarqube", RuleID: "typescript:S1", RawSeverity: "MAJOR", Category: analyzers.CategoryCodeSmell, Message: "Use a | b", RelativePath: "src/a.ts", StartLine: 8, Metadata: map[string]any{"type": "CODE_SMELL"}}},
		Runs:     []Run{{AnalyzerID: "sonarqube", State: "succeeded", Version: "26.8", FindingCount: 1, Duration: time.Second}},
	})
	s := Markdown(m)
	for _, want := range []string{
		"## Quality Metrics Summary", "| **Size** | Lines of Code (LOC) | 42 |", "|  | Security Rating | A |", "| **Coverage & Duplication** | Coverage | 87.5% |",
		"## Open Findings", "| sonarqube | MAJOR | CODE_SMELL | src/a.ts | 8 | typescript:S1 | Use a \\| b |", "## Security Findings", "[OK] No security findings reported.", "## Analyzer Summary",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in markdown:\n%s", want, s)
		}
	}
}

package reports

import (
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

func TestMarkdownLeadsWithVerdictAndDropsPlaceholderMetrics(t *testing.T) {
	m := Build(Input{WorkspaceName: "Demo",
		Findings: []analyzers.Finding{{Severity: analyzers.SeverityHigh, RuleID: "a"}, {Severity: analyzers.SeverityHigh, RuleID: "b"}, {Severity: analyzers.SeverityLow, RuleID: "c"}},
		Runs:     []Run{{AnalyzerID: "ruff", State: "succeeded"}, {AnalyzerID: "sonarqube", State: "failed"}}})
	out := Markdown(m)
	for _, want := range []string{"## Summary", "**Grade B · Moderate risk** (risk score 11) — provisional", "3 findings: 0 critical, 2 high, 0 medium, 1 low, 0 info.", "1 of 2 analyzers ran · 1 didn't run."} {
		if !strings.Contains(out, want) {
			t.Fatalf("markdown missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Rating | N/A") || strings.Contains(out, "Coverage | N/A") || strings.Contains(out, "Lines of Code") {
		t.Fatalf("placeholder metric rows survived:\n%s", out[:min(len(out), 2000)])
	}
	html := string(HTML(m))
	for _, want := range []string{"brand-mark", "Grade B", "1 of 2 analyzers ran", "grade-provisional"} {
		if !strings.Contains(html, want) {
			t.Fatalf("html missing %q", want)
		}
	}
}

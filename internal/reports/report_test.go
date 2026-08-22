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

// TestMarkdownStripsControlCharacters is the regression test for control
// character injection into report.md. Analyzer-derived text must never carry
// raw C0 controls (ESC/ANSI sequences, BEL, NUL) or DEL into the export, and
// CRLF must never forge new markdown rows. Markdown link and HTML syntax in
// messages are accepted by design for the local single-user report; the test
// pins that such content stays inert inside a single table cell.
func TestMarkdownStripsControlCharacters(t *testing.T) {
	m := Build(Input{
		WorkspaceName: "Demo", StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Findings: []analyzers.Finding{{
			AnalyzerID: "ruff", RuleID: "F401", Severity: analyzers.SeverityHigh, Category: analyzers.CategorySecurity,
			Message:      "\x1b[31malarm\x1b[0m bell\x07 nul\x00\r\n| forged | row | [click](https://evil.example)",
			RelativePath: "src/a.py", StartLine: 3,
		}},
	})
	s := Markdown(m)
	for _, banned := range []string{"\x1b", "\x07", "\x00", "\x7f", "\r"} {
		if strings.Contains(s, banned) {
			t.Fatalf("control character %q survived into the markdown:\n%q", banned, s)
		}
	}
	if strings.Contains(s, "| forged |") {
		t.Fatalf("unescaped pipe forged markdown structure:\n%s", s)
	}
	if !strings.Contains(s, "[click](https://evil.example)") {
		t.Fatalf("benign link syntax must stay visible as inert text inside its cell:\n%s", s)
	}
	// The CRLF-forged content must stay on the same rendered row as the rest of
	// the message instead of becoming an attacker-controlled table row.
	sameLine := false
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "forged") {
			if !strings.Contains(line, "alarm") || !strings.Contains(line, "[click]") {
				t.Fatalf("forged fragment escaped its row:\n%q", line)
			}
			sameLine = true
		}
	}
	if !sameLine {
		t.Fatalf("message content missing from markdown:\n%s", s)
	}
}

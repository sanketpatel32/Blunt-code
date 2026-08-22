package reports

import (
	"bluntcode/internal/analyzers"
	"strings"
	"testing"
	"time"
)

func renderHTML(t *testing.T, in Input) string {
	t.Helper()
	return string(HTML(Build(in)))
}

func TestHTMLDocumentIsComplete(t *testing.T) {
	doc := renderHTML(t, Input{WorkspaceName: "Demo", StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)})
	if !strings.HasPrefix(doc, "<!DOCTYPE html>") {
		t.Fatalf("document must start with a doctype: %q", doc[:min(len(doc), 60)])
	}
	for _, want := range []string{
		`<html lang="en">`, `<meta charset="utf-8">`, `<meta name="viewport" content="width=device-width, initial-scale=1">`,
		"<title>Blunt Code report — Demo — 2026-01-02</title>", "</head>", "</body>", "</html>",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("missing %q in document:\n%s", want, doc)
		}
	}
}

func TestHTMLEscapesAllUserControlledStrings(t *testing.T) {
	doc := renderHTML(t, Input{
		WorkspaceName: `Acme<script>alert(1)</script>`,
		Findings: []analyzers.Finding{{
			AnalyzerID: "ruff", RuleID: `<script>alert(1)</script>`, Severity: analyzers.SeverityHigh,
			Message: `msg <script>alert(1)</script> body`, RelativePath: `src/<script>alert(1)</script>.py`, StartLine: 4,
		}},
	})
	if strings.Contains(doc, "<script") {
		t.Fatalf("raw script markup must never survive into the document:\n%s", doc)
	}
	escaped := "&lt;script&gt;alert(1)&lt;/script&gt;"
	for _, want := range []string{
		"Acme" + escaped,           // workspace name in title and header
		"msg " + escaped + " body", // finding message
		escaped + ".py",            // file path
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("missing escaped form %q in document:\n%s", want, doc)
		}
	}
	if strings.Count(doc, escaped) < 4 {
		t.Fatalf("workspace name, rule id, message, and path must each render escaped:\n%s", doc)
	}
}

func TestHTMLEmptyStateWhenNoFindings(t *testing.T) {
	doc := renderHTML(t, Input{WorkspaceName: "Demo"})
	if !strings.Contains(doc, "No findings — nice work.") {
		t.Fatalf("zero-findings report must carry the empty state sentence:\n%s", doc)
	}
}

func TestHTMLSeverityBadgePerSeverity(t *testing.T) {
	findings := []analyzers.Finding{
		{AnalyzerID: "ruff", RuleID: "C1", Severity: analyzers.SeverityCritical, Message: "m", RelativePath: "a.py"},
		{AnalyzerID: "ruff", RuleID: "H1", Severity: analyzers.SeverityHigh, Message: "m", RelativePath: "a.py"},
		{AnalyzerID: "ruff", RuleID: "M1", Severity: analyzers.SeverityMedium, Message: "m", RelativePath: "a.py"},
		{AnalyzerID: "ruff", RuleID: "L1", Severity: analyzers.SeverityLow, Message: "m", RelativePath: "a.py"},
		{AnalyzerID: "ruff", RuleID: "I1", Severity: analyzers.SeverityInfo, Message: "m", RelativePath: "a.py"},
		{AnalyzerID: "ruff", RuleID: "X1", Severity: analyzers.Severity("bogus"), Message: "m", RelativePath: "a.py"},
	}
	doc := renderHTML(t, Input{Findings: findings})
	for _, class := range []string{"critical", "high", "medium", "low", "info"} {
		if !strings.Contains(doc, `class="badge badge-`+class+`"`) {
			t.Fatalf("severity %s must render its badge class:\n%s", class, doc)
		}
		if !strings.Contains(doc, `class="stat stat-`+class+`"`) {
			t.Fatalf("severity %s must render its summary stat class:\n%s", class, doc)
		}
	}
	if strings.Contains(doc, "badge-bogus") || strings.Contains(doc, "stat-bogus") {
		t.Fatalf("an unmapped severity must fall back to a fixed class, not become one:\n%s", doc)
	}
}

func TestHTMLPrintAndResponsiveStyles(t *testing.T) {
	doc := renderHTML(t, Input{WorkspaceName: "Demo"})
	for _, want := range []string{"@media print", "page-break-inside:avoid", ".table-wrap{overflow-x:auto", "@media (max-width:40rem)"} {
		if !strings.Contains(doc, want) {
			t.Fatalf("missing style rule %q:\n%s", want, doc)
		}
	}
}

func TestHTMLHeaderFooterAndSections(t *testing.T) {
	doc := renderHTML(t, Input{
		WorkspaceName: "Demo", WorkspacePath: `C:\code\demo`, Profile: "deep", BluntCodeVersion: "1.2.3",
		StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), FinishedAt: time.Date(2026, 1, 2, 3, 4, 12, 0, time.UTC),
		Findings: []analyzers.Finding{{
			AnalyzerID: "semgrep", RuleID: "S1", Severity: analyzers.SeverityHigh, Title: "Hardcoded credential",
			Message: "credential in source", Remediation: "Move it to an environment variable.",
			RelativePath: "src/a.py", StartLine: 7, DocumentationURL: "https://example.com/docs/S1",
		}},
		Runs: []Run{{AnalyzerID: "sonarqube", State: "failed", ErrorSummary: "startup timeout"}},
	})
	for _, want := range []string{
		`<strong>Root:</strong> C:\code\demo`, `<strong>Profile:</strong> deep`,
		`<strong>Started:</strong> 2026-01-02 03:04:05 UTC`, `<strong>Finished:</strong> 2026-01-02 03:04:12 UTC`,
		"Generated locally by Blunt Code v1.2.3 — code never left this computer.",
		"Warnings — incomplete analysis", "sonarqube: startup timeout",
		`<a href="https://example.com/docs/S1" rel="noopener noreferrer" target="_blank">S1</a>`,
		"<strong>Fix:</strong> Move it to an environment variable.",
		`<code class="path">src/a.py:7</code>`, `<span class="tag">semgrep</span>`, "Hardcoded credential",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("missing %q in document:\n%s", want, doc)
		}
	}
	bare := renderHTML(t, Input{WorkspaceName: "Demo"})
	if !strings.Contains(bare, "Generated locally by Blunt Code — code never left this computer.") {
		t.Fatalf("version must be omitted when the scan carries none:\n%s", bare)
	}
}

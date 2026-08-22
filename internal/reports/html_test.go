package reports

import (
	"bluntcode/internal/analyzers"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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

// TestHTMLNeutralizesHostileURLsAndMarkup pins html/template contextual
// escaping against payloads a hostile scanned repository could plant in
// analyzer output: scheme-abusing documentation URLs, attribute-breaking
// quotes and newlines in file paths, and event-handler markup in titles.
// Every interpolation must stay inert; unsafe hrefs must be replaced by the
// html/template URL filter failsafe (#ZgotunZSet).
func TestHTMLNeutralizesHostileURLsAndMarkup(t *testing.T) {
	doc := renderHTML(t, Input{WorkspaceName: "Demo", Findings: []analyzers.Finding{
		{
			AnalyzerID: "ruff", RuleID: `a"href="javascript:alert(1)`, Severity: analyzers.SeverityHigh,
			Message: `</td></tr><script>alert(1)</script>`, RelativePath: "src/plain.py",
			DocumentationURL: "javascript:alert(1)",
		},
		{
			AnalyzerID: "ruff", RuleID: "DATA-URI", Severity: analyzers.SeverityHigh,
			Message: "data uri in docs link", RelativePath: "src/plain2.py",
			DocumentationURL: "data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==",
		},
		{
			AnalyzerID: "ruff", RuleID: "CONTROL-SCHEME", Severity: analyzers.SeverityHigh,
			Message: "scheme smuggled behind control bytes", RelativePath: "src/plain3.py",
			DocumentationURL: " \x01jav\tascript:alert(1)",
		},
		{
			AnalyzerID: "ruff", RuleID: "ATTR-BREAK", Severity: analyzers.SeverityMedium,
			Title:        `<img src=x onerror=alert(1)>`,
			Message:      "attribute breaking path below",
			RelativePath: "src/\" onmouseover=\"alert(1)\nhidden.py",
		},
	}})
	if strings.Contains(doc, `href="javascript`) || strings.Contains(doc, `href="data:`) {
		t.Fatalf("an executable URL scheme survived the href context:\n%s", doc)
	}
	if !strings.Contains(doc, `href="#ZgotmplZ"`) {
		t.Fatalf("unsafe documentation URLs must be replaced by the html/template filter failsafe:\n%s", doc)
	}
	for _, banned := range []string{"<script", "<img", `onmouseover="`, `href="a"`} {
		if strings.Contains(doc, banned) {
			t.Fatalf("raw markup %q leaked into the document:\n%s", banned, doc)
		}
	}
	// Quotes and newlines in the path must render as inert escaped text.
	if !strings.Contains(doc, "src/&#34; onmouseover=&#34;alert(1)") {
		t.Fatalf("attribute-breaking path must render escaped inside its cell:\n%s", doc)
	}
	if !strings.Contains(doc, "&lt;img src=x onerror=alert(1)&gt;") {
		t.Fatalf("event-handler title must render as escaped text:\n%s", doc)
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

// htmlTextEscape mirrors html/template's text-context escaping (verified
// against the standard library: it escapes & < > ' " + — the last one to
// defeat UTF-7 style attacks — and nothing else) so tests can assert the
// exact escaped form hostile payloads must take inside the document.
var htmlTextEscape = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&#34;",
	"'", "&#39;",
	"+", "&#43;",
)

// scrubbed is the test's independent statement of the control-character
// contract: every C0 control except tab, LF, and CR, plus DEL, becomes a
// space before text reaches the template.
func scrubbed(s string) string {
	return strings.Map(func(r rune) rune {
		if (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0x7f {
			return ' '
		}
		return r
	}, s)
}

// TestHTMLSurvivesHostileCorpus round-trips the hostile fixture table through
// the HTML export: markup payloads must render as inert escaped text, control
// bytes must never appear raw (html/template passes NUL, BEL, and ANSI
// escapes through untouched in text contexts), the findings table must keep
// exactly one row and one severity badge per finding, and every message must
// remain present and legible.
func TestHTMLSurvivesHostileCorpus(t *testing.T) {
	corpus := hostileCorpus()
	doc := string(HTML(Build(Input{
		WorkspaceName: "Hostile \"Name\" <script>&'+'", StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Findings: corpus, Runs: []Run{hostileRun()},
	})))

	if !utf8.ValidString(doc) {
		t.Fatal("HTML output must be valid UTF-8")
	}
	// onerror=/onmouseover= appear only as escaped inert text (e.g. inside
	// &lt;img ...&gt;), which is why the banned set uses the raw markup and
	// attribute-assignment forms that would indicate a breakout.
	for _, banned := range []string{"\x00", "\x07", "\x0b", "\x0c", "\x1b", "\x7f", "<script", "<img", `onmouseover="`, `onerror="`, `href="javascript`, `href="data:`} {
		if strings.Contains(doc, banned) {
			t.Fatalf("raw %q leaked into the document:\n%s", banned, doc)
		}
	}

	// Structure: one badge and one table row per finding, plus the two table
	// headers and the one analyzer run row.
	if got := strings.Count(doc, `class="badge badge-`); got != len(corpus) {
		t.Fatalf("expected %d severity badges, got %d", len(corpus), got)
	}
	if got := strings.Count(doc, "<tr>"); got != len(corpus)+3 {
		t.Fatalf("expected %d table rows (%d findings + 2 headers + 1 run), got %d", len(corpus)+3, len(corpus), got)
	}

	// Hostile documentation URL must hit the template's URL filter failsafe.
	if !strings.Contains(doc, `href="#ZgotmplZ"`) {
		t.Fatalf("unsafe documentation URL must be replaced by the filter failsafe:\n%s", doc)
	}

	// Escaped forms: the markup payloads render as text, exactly.
	for _, want := range []string{
		htmlTextEscape.Replace(`<script>alert(1)</script> & <img src=x onerror=alert(1)> &amp; &#60;b&#62;`),
		htmlTextEscape.Replace(`100% #1 'single' "double" +plus.py`),
		htmlTextEscape.Replace(`https://evil.example/payload.py`),
		htmlTextEscape.Replace(`Wörkspâce <script>`),
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("missing escaped payload %q:\n%s", want, doc)
		}
	}

	// Every non-empty corpus message survives, scrubbed and escaped, in its cell.
	for _, f := range corpus {
		if f.Message == "" {
			continue
		}
		want := htmlTextEscape.Replace(scrubbed(f.Message))
		if !strings.Contains(doc, want) {
			t.Fatalf("message for %q missing or mangled; want %q:\n%s", f.RuleID, want, doc)
		}
	}

	// Severity is exported normalized: the raw analyzer casing is display-only
	// metadata that must not leak into the badge.
	if !strings.Contains(doc, ">high</span>") {
		t.Fatalf("normalized severity must drive the badge text:\n%s", doc)
	}
	if strings.Contains(doc, "HIGH!!!") {
		t.Fatalf("raw analyzer severity casing leaked into the HTML:\n%s", doc)
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

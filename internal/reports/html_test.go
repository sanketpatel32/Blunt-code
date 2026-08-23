package reports

import (
	"bluntcode/internal/analyzers"
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func renderHTML(t *testing.T, in Input) string {
	t.Helper()
	return string(HTML(Build(in)))
}

// assertSingleScript pins the document's one legitimate inline script: the
// report may carry exactly one <script> element (its own static filter code)
// and one closing tag, so any hostile payload that opens or closes a script
// context stands out immediately.
func assertSingleScript(t *testing.T, doc string) {
	t.Helper()
	if got := strings.Count(doc, "<script"); got != 1 {
		t.Fatalf("document must contain exactly one script element (the report's own), found %d:\n%s", got, doc)
	}
	if got := strings.Count(doc, "</script>"); got != 1 {
		t.Fatalf("document must contain exactly one closing script tag, found %d:\n%s", got, doc)
	}
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
	assertSingleScript(t, doc)
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
	// Filter controls are server-rendered only when there is something to
	// filter; an empty report must not ship the widgets or their script hooks.
	for _, banned := range []string{`id="filter-status"`, `id="filter-search"`, `data-sev="`, `<details class="file-group"`} {
		if strings.Contains(doc, banned) {
			t.Fatalf("filter markup %q must not render without findings:\n%s", banned, doc)
		}
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
	assertSingleScript(t, doc)
	if strings.Contains(doc, `href="javascript`) || strings.Contains(doc, `href="data:`) {
		t.Fatalf("an executable URL scheme survived the href context:\n%s", doc)
	}
	if !strings.Contains(doc, `href="#ZgotmplZ"`) {
		t.Fatalf("unsafe documentation URLs must be replaced by the html/template filter failsafe:\n%s", doc)
	}
	for _, banned := range []string{"<img", `onmouseover="`, `onerror="`, `href="a"`} {
		if strings.Contains(doc, banned) {
			t.Fatalf("raw markup %q leaked into the document:\n%s", banned, doc)
		}
	}
	// Quotes and newlines in the path must render as inert escaped text, both
	// in the group heading and in the row's data-file attribute.
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
	// The bogus-severity finding must still be filterable: its row maps to the
	// fixed low class, never to an attacker-named one.
	if !strings.Contains(doc, `<tr data-sev="low" data-an="ruff" data-rule="X1"`) {
		t.Fatalf("unmapped severity must fall back to the low filter class on its row:\n%s", doc)
	}
}

func TestHTMLPrintAndResponsiveStyles(t *testing.T) {
	doc := renderHTML(t, Input{WorkspaceName: "Demo"})
	for _, want := range []string{
		"@media print", "page-break-inside:avoid", ".table-wrap{overflow-x:auto", "@media (max-width:40rem)",
		".filters{display:none}",                         // filters are screen-only UI
		"body.is-printing tr[hidden]{display:table-row}", // print class forces every row visible
		"body.is-printing details.file-group[hidden]{display:block}",
		":focus-visible{outline:2px solid #1d4ed8", // keyboard focus states for the filter controls
		"tr[hidden]{display:none}",                 // filtered rows hide in screen context
	} {
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

// htmlGroupCount mirrors the renderer's file grouping rule so structure tests
// can compute the expected number of file sections from the corpus itself.
func htmlGroupCount(findings []analyzers.Finding) int {
	seen := map[string]bool{}
	for _, f := range findings {
		path := strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(f.RelativePath)), "./")
		if path == "" || path == "." {
			path = "project"
		}
		seen[path] = true
	}
	return len(seen)
}

// TestHTMLSurvivesHostileCorpus round-trips the hostile fixture table through
// the HTML export: markup payloads must render as inert escaped text, control
// bytes must never appear raw (html/template passes NUL, BEL, and ANSI
// escapes through untouched in text contexts), the findings must keep exactly
// one row, badge, and filterable data-sev row per finding, and every message
// must remain present and legible.
func TestHTMLSurvivesHostileCorpus(t *testing.T) {
	corpus := hostileCorpus()
	doc := string(HTML(Build(Input{
		WorkspaceName: "Hostile \"Name\" <script>&'+'", StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Findings: corpus, Runs: []Run{hostileRun()},
	})))

	if !utf8.ValidString(doc) {
		t.Fatal("HTML output must be valid UTF-8")
	}
	assertSingleScript(t, doc)
	// onerror=/onmouseover= appear only as escaped inert text (e.g. inside
	// &lt;img ...&gt;), which is why the banned set uses the raw markup and
	// attribute-assignment forms that would indicate a breakout.
	for _, banned := range []string{"\x00", "\x07", "\x0b", "\x0c", "\x1b", "\x7f", "<img", `onmouseover="`, `onerror="`, `href="javascript`, `href="data:`} {
		if strings.Contains(doc, banned) {
			t.Fatalf("raw %q leaked into the document:\n%s", banned, doc)
		}
	}

	// Structure: one badge, one filterable row, and one file group set per
	// finding, plus one header row per table and the single analyzer run row.
	groupCount := htmlGroupCount(corpus)
	if got := strings.Count(doc, `class="badge badge-`); got != len(corpus) {
		t.Fatalf("expected %d severity badges, got %d", len(corpus), got)
	}
	if got := strings.Count(doc, `<tr data-sev=`); got != len(corpus) {
		t.Fatalf("expected one filterable row per finding (%d), got %d", len(corpus), got)
	}
	if got := strings.Count(doc, `<details class="file-group"`); got != groupCount {
		t.Fatalf("expected %d file groups, got %d", groupCount, got)
	}
	if got := strings.Count(doc, "<tr>"); got != groupCount+2 {
		t.Fatalf("expected %d structural table rows (%d group headers + analyzer header + run row), got %d", groupCount+2, groupCount, got)
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
		`<code class="path">src/a.py</code>`, `<span class="tag">semgrep</span>`, "Hardcoded credential",
		`<td class="num">7</td>`,
		`<option value="semgrep">semgrep</option>`, `<option value="sonarqube">sonarqube</option>`,
		`Showing 1 of 1 findings`,
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

// TestHTMLFilterControlsWithCountsAndDataAttributes pins the server-rendered
// filter UI: every severity chip carries its live-updating count, the analyzer
// dropdown lists exactly the analyzers present in runs and findings (sorted,
// deduplicated), the search box and clear action exist, and every finding row
// exports the escaped data-* metadata the script filters on.
func TestHTMLFilterControlsWithCountsAndDataAttributes(t *testing.T) {
	doc := renderHTML(t, Input{WorkspaceName: "Demo", Findings: []analyzers.Finding{
		{AnalyzerID: "ruff", RuleID: "H1", Severity: analyzers.SeverityHigh, Message: "first", RelativePath: "src/b.py", StartLine: 3},
		{AnalyzerID: "ruff", RuleID: "H2", Severity: analyzers.SeverityHigh, Message: "second", RelativePath: "src/a.py"},
		{AnalyzerID: "biome", RuleID: "M1", Severity: analyzers.SeverityMedium, Message: "third", RelativePath: "src/a.py", StartLine: 9},
		{AnalyzerID: "semgrep", RuleID: "C1", Severity: analyzers.SeverityCritical, Message: "fourth", RelativePath: "src/z.py"},
	}, Runs: []Run{{AnalyzerID: "ruff", State: "succeeded"}}})
	for _, want := range []string{
		`<div class="filter-group" role="group" aria-label="Filter findings by severity">`,
		`data-sev="critical" aria-pressed="true">Critical <span class="chip-count">1</span>`,
		`data-sev="high" aria-pressed="true">High <span class="chip-count">2</span>`,
		`data-sev="medium" aria-pressed="true">Medium <span class="chip-count">1</span>`,
		`data-sev="low" aria-pressed="true">Low <span class="chip-count">0</span>`,
		`data-sev="info" aria-pressed="true">Info <span class="chip-count">0</span>`,
		`<label class="filter-label" for="filter-analyzer">Analyzer</label>`,
		`<option value="">All analyzers</option>`,
		`<option value="biome">biome</option>`,
		`<option value="ruff">ruff</option>`,
		`<option value="semgrep">semgrep</option>`,
		`<label class="filter-label" for="filter-search">Search</label>`,
		`<input type="search" id="filter-search" placeholder="Rule, message, or file" autocomplete="off">`,
		`<button type="button" class="filter-clear" id="filter-clear">Clear filters</button>`,
		`<p class="filter-status" id="filter-status" role="status" aria-live="polite">Showing 4 of 4 findings</p>`,
		`<p class="empty" id="filter-no-match" hidden>No findings match the current filters.</p>`,
		`<tr data-sev="high" data-an="ruff" data-rule="H1" data-file="src/b.py">`,
		`<tr data-sev="high" data-an="ruff" data-rule="H2" data-file="src/a.py">`,
		`<tr data-sev="medium" data-an="biome" data-rule="M1" data-file="src/a.py">`,
		`<tr data-sev="critical" data-an="semgrep" data-rule="C1" data-file="src/z.py">`,
		`<details class="file-group" data-file="src/a.py" open>`,
		`<span class="group-count">2 findings</span>`,
		`<span class="group-count">1 finding</span>`,
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("missing filter markup %q in document:\n%s", want, doc)
		}
	}
	// Groups render sorted by path so identical input is byte-stable.
	a, b, z := strings.Index(doc, `data-file="src/a.py"`), strings.Index(doc, `data-file="src/b.py"`), strings.Index(doc, `data-file="src/z.py"`)
	if !(0 <= a && a < b && b < z) {
		t.Fatalf("file groups must render sorted by path (a=%d b=%d z=%d):\n%s", a, b, z, doc)
	}
	// Analyzers absent from both runs and findings must not appear as options.
	if strings.Contains(doc, `<option value="sonarqube"`) {
		t.Fatalf("analyzer dropdown must only list analyzers present in the report:\n%s", doc)
	}
}

// TestHTMLScriptIsInlineSelfContainedAndPrintAware proves the report loads no
// external resources: one attribute-less script element carrying the static
// htmlScript constant verbatim, no link/import/src/url references, no network
// or storage APIs, and print wiring that shows every row when printing.
func TestHTMLScriptIsInlineSelfContainedAndPrintAware(t *testing.T) {
	doc := renderHTML(t, Input{WorkspaceName: "Demo"})
	assertSingleScript(t, doc)
	if strings.Contains(doc, "<script ") {
		t.Fatalf("the script element must not carry attributes such as src:\n%s", doc)
	}
	if !strings.Contains(doc, htmlScript) {
		t.Fatalf("the embedded script body must be the htmlScript constant, verbatim:\n%s", doc)
	}
	if !strings.Contains(doc, "\n</script>\n</body>") {
		t.Fatalf("the script must sit inline at the end of the document body:\n%s", doc)
	}
	for _, banned := range []string{
		"http://", "https://", "<link", "@import", " src=", "srcset=", "crossorigin=",
		"url(", "fetch(", "XMLHttpRequest", "localStorage", "sessionStorage", "eval(", "import(",
	} {
		if strings.Contains(doc, banned) {
			t.Fatalf("self-contained document must not reference or load external resources (%q found):\n%s", banned, doc)
		}
	}
	for _, want := range []string{"beforeprint", "afterprint", "is-printing"} {
		if !strings.Contains(doc, want) {
			t.Fatalf("print wiring %q missing from the document:\n%s", want, doc)
		}
	}
}

// TestHTMLFilterMarkupSurvivesScriptInjection feeds the exact payloads that
// could try to escape the filter metadata contexts: a message and rule id
// carrying <script>, an early </script>, quotes in an analyzer id, and control
// bytes. Everything must render escaped (attributes included), the document
// must keep exactly its one script element, and the row must stay structurally
// filterable so the filters keep working.
func TestHTMLFilterMarkupSurvivesScriptInjection(t *testing.T) {
	doc := renderHTML(t, Input{WorkspaceName: "Demo", Findings: []analyzers.Finding{{
		AnalyzerID:       `evil"an`,
		RuleID:           `</script><script>alert(1)</script>`,
		Severity:         analyzers.SeverityHigh,
		Message:          "</script><script>alert(1)</script> \"quoted\" 'also' \x00bell\x07esc\x1b[31m",
		RelativePath:     `src/<script>alert(1)</script>.py`,
		DocumentationURL: "javascript:alert(1)",
	}}, Runs: []Run{{AnalyzerID: `evil"an`, State: "failed", ErrorSummary: "boom"}}})

	assertSingleScript(t, doc)
	if strings.Contains(doc, "<script>alert") {
		t.Fatalf("hostile payload opened an executable script context:\n%s", doc)
	}
	for _, banned := range []string{"\x00", "\x07", "\x1b", `data-an="evil"`, `value="evil"`} {
		if strings.Contains(doc, banned) {
			t.Fatalf("raw %q leaked into the document:\n%s", banned, doc)
		}
	}
	for _, want := range []string{
		`data-rule="&lt;/script&gt;&lt;script&gt;alert(1)&lt;/script&gt;"`,
		`data-file="src/&lt;script&gt;alert(1)&lt;/script&gt;.py"`,
		`data-an="evil&#34;an"`,
		htmlTextEscape.Replace("</script><script>alert(1)</script> \"quoted\" 'also'  bell esc [31m"),
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("missing escaped filter metadata %q:\n%s", want, doc)
		}
	}
	// The hostile row keeps its filter wiring: severity, analyzer, and file all
	// survive as escaped attributes on the same row the script toggles.
	if !strings.Contains(doc, `<tr data-sev="high" data-an="evil&#34;an" data-rule="&lt;/script&gt;&lt;script&gt;alert(1)&lt;/script&gt;" data-file="src/&lt;script&gt;alert(1)&lt;/script&gt;.py">`) {
		t.Fatalf("hostile finding must remain one filterable row:\n%s", doc)
	}
	// The script body itself is the untouched constant.
	if !strings.Contains(doc, htmlScript) {
		t.Fatalf("the embedded script body must remain the static htmlScript constant:\n%s", doc)
	}
}

// TestHTMLIsByteStableForIdenticalInput renders the same input twice and
// requires byte-identical output: the report derives no timestamps, random
// ids, or map-ordered markup, so archiving and diffing exports is reliable.
func TestHTMLIsByteStableForIdenticalInput(t *testing.T) {
	in := Input{
		WorkspaceName: "Demo", Profile: "quick", BluntCodeVersion: "9.9.9",
		StartedAt: time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC), FinishedAt: time.Date(2026, 3, 4, 5, 6, 9, 0, time.UTC),
		Findings: []analyzers.Finding{
			{AnalyzerID: "biome", RuleID: "b1", Severity: analyzers.SeverityLow, Message: "low", RelativePath: "x/y.ts"},
			{AnalyzerID: "ruff", RuleID: "r1", Severity: analyzers.SeverityCritical, Message: "crit", RelativePath: "a.py", StartLine: 2},
			{AnalyzerID: "ruff", RuleID: "r2", Severity: analyzers.SeverityCritical, Message: "crit deux", RelativePath: "a.py"},
		},
		Runs: []Run{{AnalyzerID: "ruff", State: "succeeded", FindingCount: 2, Duration: 1500 * time.Millisecond}},
	}
	first, second := HTML(Build(in)), HTML(Build(in))
	if !bytes.Equal(first, second) {
		t.Fatalf("two renders of identical input differ:\n%s\n---\n%s", first, second)
	}
	if strings.Contains(string(first), "\r") {
		t.Fatalf("document must use LF line endings only")
	}
}

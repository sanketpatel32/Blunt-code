package reports

import (
	"bluntcode/internal/analyzers"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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

// hostileCorpus is the adversarial fixture table every export format must
// survive. Analyzer output is untrusted: a hostile scanned repository can
// plant every one of these payloads in a finding. Each entry names the attack
// it represents; the per-format corpus tests prove the payload cannot corrupt
// the export's structure, forge new structure, or lose data.
func hostileCorpus() []analyzers.Finding {
	return []analyzers.Finding{
		{ // BMP + astral planes, CJK, RTL scripts with bidi marks, combining characters
			AnalyzerID: "ruff", RuleID: "unicode-bmp-astral", Severity: analyzers.SeverityHigh, Category: analyzers.CategorySecurity,
			Title:        "Ünïcode ✨ 中文タイトル مرحبا שלום",
			Message:      "BMP ✆ CJK 日本語 RTL مرحبا bidi \u202b\u202e and emoji 🚨🚀 ZWJ 👨‍👩‍👧‍👦 combining e\u0301\u0301clat",
			RelativePath: "src/ünïcode/日本語/مرحبا/coffee☕.py", StartLine: 3, StartColumn: 4, EndLine: 3, EndColumn: 9,
		},
		{ // LF, CRLF, and bare CR in every text field
			AnalyzerID: "ruff", RuleID: "newlines-lf-crlf-cr", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryBug,
			Title: "title\ntitle2\r\ntitle3\rtitle4", Message: "line1\nline2\r\nline3\rline4\n\rmixed",
			RelativePath: "dir/new\nline\r\nname.py", Remediation: "fix\r\nme\nplease",
		},
		{ // pipes try to forge markdown table columns and separator rows
			AnalyzerID: "ruff", RuleID: "pipes-forgery", Severity: analyzers.SeverityCritical, Category: analyzers.CategorySecurity,
			Title: "| Title | Forge |", Message: "a | b || c |:---| extra | column | forge",
			RelativePath: "dir|pipe.py", Remediation: "|remediation|",
		},
		{ // backticks try to break markdown code spans
			AnalyzerID: "ruff", RuleID: "backticks", Severity: analyzers.SeverityLow, Category: analyzers.CategoryStyle,
			Message: "`` `code` ``` tick", RelativePath: "back`tick`.py",
		},
		{ // HTML and script markup must render as inert text
			AnalyzerID: "semgrep", RuleID: "html-markup", Severity: analyzers.SeverityHigh, Category: analyzers.CategoryVulnerability,
			Title:        `</tr></td><script>alert("xss")</script>`,
			Message:      `<script>alert(1)</script> & <img src=x onerror=alert(1)> &amp; &#60;b&#62;`,
			RelativePath: "src/<b>bold</b>.py",
		},
		{ // JSON-breaking quotes, backslashes, and brace soup
			AnalyzerID: "ruff", RuleID: "json-breaking", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryBug,
			Message: `quotes " backslash \ escaped \" and {"json":"like\"structure"}`, RelativePath: `weird"path\name.py`,
		},
		{ // ATX headings try to corrupt section structure
			AnalyzerID: "ruff", RuleID: "heading-forgery", Severity: analyzers.SeverityInfo, Category: analyzers.CategoryStyle,
			Title: "# Not a heading", Message: "### Forged H3\n# Forged H1\n###### deep heading", RelativePath: "heading.py",
		},
		{ // leading/trailing whitespace and tabs
			AnalyzerID: "ruff", RuleID: "whitespace-edges", Severity: analyzers.SeverityLow, Category: analyzers.CategoryStyle,
			Title: "\ttabbed title\t", Message: "   leading and trailing   ", RelativePath: "  spaced path  ",
		},
		{ // very long single token
			AnalyzerID: "ruff", RuleID: "long-token", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryMaintainability,
			Message: strings.Repeat("A", 10_000) + "☕", RelativePath: "long.py",
		},
		{ // file paths that look like absolute URIs
			AnalyzerID: "ruff", RuleID: "url-like-path", Severity: analyzers.SeverityLow, Category: analyzers.CategoryStyle,
			Message: "path looks like a URL", RelativePath: "https://evil.example/payload.py",
		},
		{AnalyzerID: "ruff", RuleID: "file-url-path", Severity: analyzers.SeverityLow, Category: analyzers.CategoryStyle,
			Message: "path looks like a file URL", RelativePath: "file:///C:/Users/x/file.py"},
		{ // percent, hash, quotes, spaces, plus in one path
			AnalyzerID: "ruff", RuleID: "path-zoo", Severity: analyzers.SeverityInfo, Category: analyzers.CategoryStyle,
			Message: "zoo", RelativePath: `100% #1 'single' "double" +plus.py`,
		},
		{ // Windows separators
			AnalyzerID: "biome", RuleID: "windows-path", Severity: analyzers.SeverityLow, Category: analyzers.CategoryStyle,
			Message: "windows", RelativePath: `dir\sub dir\file name.py`,
		},
		{AnalyzerID: "biome", RuleID: "rule:with:colons", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryBug,
			Message: "first occurrence", RelativePath: "a.py", DocumentationURL: "https://first.example/rule"},
		{AnalyzerID: "ruff", RuleID: "rule:with:colons", Severity: analyzers.SeverityLow, Category: analyzers.CategoryStyle,
			Message: "second occurrence with the SAME hostile rule id", RelativePath: "b.py", DocumentationURL: "https://evil.example/rule"},
		{AnalyzerID: "ruff", RuleID: "rule with spaces and ‼️", Severity: analyzers.SeverityInfo, Category: analyzers.CategoryStyle,
			Message: "spaced rule", RelativePath: "c.py"},
		{ // empty strings in every optional field
			AnalyzerID: "ruff", RuleID: "empty-fields", Severity: analyzers.SeverityInfo, Category: analyzers.CategoryOther,
			Title: "", Message: "", RelativePath: "empty.py", Remediation: "", DocumentationURL: "",
		},
		{ // raw analyzer severity casing must not leak where normalized is expected
			AnalyzerID: "sonarqube", RuleID: "raw-severity-casing", Severity: analyzers.SeverityHigh, RawSeverity: "HIGH!!!",
			Category: analyzers.CategoryCodeSmell, Message: "raw severity kept for display", RelativePath: "raw.py",
		},
		{ // start column without a start line (invalid SARIF region input)
			AnalyzerID: "ruff", RuleID: "column-without-line", Severity: analyzers.SeverityInfo, Category: analyzers.CategoryStyle,
			Message: "column only", RelativePath: "col.py", StartColumn: 7,
		},
		{ // end positions before start positions (invalid SARIF region input)
			AnalyzerID: "ruff", RuleID: "inverted-region", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryBug,
			Message: "inverted", RelativePath: "inv.py", StartLine: 10, StartColumn: 8, EndLine: 5, EndColumn: 3,
		},
		{ // NUL, BEL, ANSI escapes, DEL, vertical tab, form feed
			AnalyzerID: "ruff", RuleID: "nul-and-controls", Severity: analyzers.SeverityHigh, Category: analyzers.CategorySecurity,
			Message: "nul\x00bell\x07esc\x1b[31mred\x1b[0m del\x7f vertical\x0btab\x0c", RelativePath: "ctrl.py",
		},
		{ // hostile documentation URL
			AnalyzerID: "semgrep", RuleID: "javascript-docs-url", Severity: analyzers.SeverityMedium, Category: analyzers.CategorySecurity,
			Message: "hostile docs link", RelativePath: "docs.py", DocumentationURL: "javascript:alert(1)//\x00",
		},
	}
}

// hostileRun is the analyzer-run counterpart of hostileCorpus.
func hostileRun() Run {
	return Run{AnalyzerID: "sonarqube", DisplayName: "Wörkspâce <script>", State: "failed\t\x1b", Version: "1<2&3", ErrorSummary: "err\x00or\r\nmulti\nline", FindingCount: 3, Duration: time.Second}
}

// countUnescapedPipes counts pipes that act as GFM table delimiters on a line:
// a pipe preceded by a backslash is escaped markup, not a delimiter. The
// markdown builder emits structural pipes surrounded by spaces and escaped
// pipes as \|, so this counts exactly the structural delimiters.
func countUnescapedPipes(line string) int {
	count := 0
	for i := 0; i < len(line); i++ {
		if line[i] == '|' && (i == 0 || line[i-1] != '\\') {
			count++
		}
	}
	return count
}

// TestMarkdownSurvivesHostileCorpus round-trips the hostile fixture table
// through the Markdown export. A message containing pipes or newlines must
// never break out of its table cell to forge columns, rows, or headings, and
// the payloads must stay legible as escaped text.
func TestMarkdownSurvivesHostileCorpus(t *testing.T) {
	corpus := hostileCorpus()
	m := Build(Input{
		WorkspaceName: "Hostile \"Name\" <script>", StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Findings: corpus, Runs: []Run{hostileRun()},
	})
	s := Markdown(m)

	if !utf8.ValidString(s) {
		t.Fatal("markdown must be valid UTF-8")
	}
	for _, banned := range []string{"\x00", "\x07", "\x0b", "\x0c", "\x1b", "\x7f", "\r"} {
		if strings.Contains(s, banned) {
			t.Fatalf("control character %q survived into the markdown:\n%q", banned, s)
		}
	}

	// Section structure: the only ATX headings are the report's own. A
	// message like "\n# Forged H1" must not introduce a heading.
	allowed := map[string]bool{
		"# Blunt Code Analysis Report - ":     false,
		"## Quality Metrics Summary":          false,
		"## Open Findings":                    false,
		"## Security Findings":                false,
		"## Change Since Previous Scan":       false,
		"## Scan Details":                     false,
		"## Analyzer Summary":                 false,
		"## Warnings and Incomplete Analysis": false,
		"## Report Notes":                     false,
	}
	headings := 0
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(line, "#") {
			continue
		}
		headings++
		matched := false
		for prefix := range allowed {
			if strings.HasPrefix(line, prefix) {
				allowed[prefix] = true
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("forged or unexpected heading line %q", line)
		}
	}
	if headings != len(allowed) {
		t.Fatalf("expected exactly %d structural headings, got %d", len(allowed), headings)
	}

	// Findings table structure: every line in the Open Findings section is one
	// of the 7-column table lines, and there are exactly 2 + len(corpus) of
	// them, so no payload forged or swallowed rows.
	lines := strings.Split(s, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "## Open Findings") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatal("Open Findings section missing")
	}
	tableLines := 0
	for _, line := range lines[start+1:] {
		if strings.HasPrefix(line, "## ") {
			break
		}
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			t.Fatalf("unexpected non-table line in findings section: %q", line)
		}
		if pipes := countUnescapedPipes(line); pipes != 8 {
			t.Fatalf("table line must carry exactly 8 structural pipes (7 columns), got %d: %q", pipes, line)
		}
		tableLines++
	}
	if tableLines != 2+len(corpus) {
		t.Fatalf("findings table must have a header, separator, and one row per finding: got %d lines for %d findings", tableLines, len(corpus))
	}

	// Payload visibility: hostile text stays present, escaped.
	for _, want := range []string{
		`a \| b \|\| c \|:---\| extra \| column \| forge`, // pipes escaped, still one cell
		"line1 line2  line3 line4  mixed",                 // newlines collapsed inside the cell
		"\\`\\` \\`code\\` \\`\\`\\` tick",                // backticks escaped
		`quotes " backslash \\ escaped \\" and {"json":"like\\"structure"}`,
		"### Forged H3 # Forged H1 ###### deep heading", // heading syntax inert on one line
		strings.Repeat("A", 10_000) + "☕",               // long token intact
		"e\u0301\u0301clat",                             // combining characters intact
		"bidi \u202b\u202e",                             // bidi marks survive
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing escaped payload %q in markdown:\n%s", want, s)
		}
	}
	// The analyzer summary section keeps its 6-column shape despite the hostile run.
	summaryStart := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "## Analyzer Summary") {
			summaryStart = i
			break
		}
	}
	if summaryStart < 0 {
		t.Fatal("Analyzer Summary section missing")
	}
	summaryLines := 0
	runRow := false
	for _, line := range lines[summaryStart+1:] {
		if strings.HasPrefix(line, "## ") {
			break
		}
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			t.Fatalf("unexpected non-table line in analyzer summary: %q", line)
		}
		if pipes := countUnescapedPipes(line); pipes != 7 {
			t.Fatalf("analyzer summary line must carry 7 structural pipes (6 columns), got %d: %q", pipes, line)
		}
		if strings.Contains(line, `Wörkspâce <script>`) {
			runRow = true
			if !strings.Contains(line, "err or  multi line") {
				t.Fatalf("hostile run data missing from analyzer summary: %q", line)
			}
		}
		summaryLines++
	}
	if summaryLines != 3 || !runRow { // header, separator, and the single hostile run row
		t.Fatalf("analyzer summary must have header, separator, and one run row: got %d lines, run row found: %v", summaryLines, runRow)
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

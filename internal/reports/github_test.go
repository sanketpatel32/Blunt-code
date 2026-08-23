package reports

import (
	"bluntcode/internal/analyzers"
	"bytes"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// githubRender builds and renders a one-finding model and returns the first
// (annotation) line, so escaping cases can assert exact workflow commands.
func githubFirstLine(t *testing.T, in Input) string {
	t.Helper()
	out := string(GitHubAnnotations(Build(in)))
	end := strings.IndexByte(out, '\n')
	if end < 0 {
		t.Fatalf("render has no annotation line:\n%s", out)
	}
	return out[:end]
}

// githubAnnotationLines returns only the workflow-command lines of a render,
// dropping the plain-text summary tail.
func githubAnnotationLines(out string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if strings.HasPrefix(line, "::") {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestGitHubAnnotationsRendersOneWorkflowCommandPerFinding(t *testing.T) {
	out := string(GitHubAnnotations(Build(Input{
		WorkspaceName: "Demo", ScanID: "scan-1",
		Findings: []analyzers.Finding{{
			AnalyzerID: "ruff", RuleID: "F401", Severity: analyzers.SeverityHigh, Category: analyzers.CategoryCorrectness,
			Message: "unused import", RelativePath: "src/a.py", StartLine: 3, StartColumn: 7,
		}},
	})))
	want := "::error file=src/a.py,line=3,col=7,title=ruff/F401::unused import\n" +
		"Blunt Code scan of Demo (scan scan-1): 0 critical, 1 high, 0 medium, 0 low, 0 info - 1 total.\n"
	if out != want {
		t.Fatalf("output = %q\nwant %q", out, want)
	}
}

// TestGitHubAnnotationsEscapesPerWorkflowCommandSpec pins the percent-escape
// set: properties (file, title) must never carry the `,`, `:`, or `%`
// delimiters, and the message segment must stay single-line.
func TestGitHubAnnotationsEscapesPerWorkflowCommandSpec(t *testing.T) {
	cases := []struct {
		name     string
		finding  analyzers.Finding
		wantLine string
	}{
		{"message newline and CR", analyzers.Finding{AnalyzerID: "ruff", RuleID: "R", Severity: analyzers.SeverityMedium,
			Message: "line1\nline2\rline3", RelativePath: "a.py"},
			"::warning file=a.py,title=ruff/R::line1%0Aline2%0Dline3"},
		{"message comma and colon pass through", analyzers.Finding{AnalyzerID: "ruff", RuleID: "R", Severity: analyzers.SeverityMedium,
			Message: "first, second: third", RelativePath: "a.py"},
			"::warning file=a.py,title=ruff/R::first, second: third"},
		{"message percent is encoded first", analyzers.Finding{AnalyzerID: "ruff", RuleID: "R", Severity: analyzers.SeverityMedium,
			Message: "100% of %0A", RelativePath: "a.py"},
			"::warning file=a.py,title=ruff/R::100%25 of %250A"},
		{"message control bytes are scrubbed then encoded", analyzers.Finding{AnalyzerID: "ruff", RuleID: "R", Severity: analyzers.SeverityMedium,
			Message: "nul\x00bel\x07esc\x1b[0mdel\x7fvtab\x0b tab\tlf\n", RelativePath: "a.py"},
			"::warning file=a.py,title=ruff/R::nul bel esc [0mdel vtab  tab%09lf"},
		{"title comma is encoded", analyzers.Finding{AnalyzerID: "ruff", RuleID: "lint,comma", Severity: analyzers.SeverityMedium,
			Message: "m", RelativePath: "a.py"},
			"::warning file=a.py,title=ruff/lint%2Ccomma::m"},
		{"title colon is encoded", analyzers.Finding{AnalyzerID: "ruff", RuleID: "colon:rule", Severity: analyzers.SeverityMedium,
			Message: "m", RelativePath: "a.py"},
			"::warning file=a.py,title=ruff/colon%3Arule::m"},
		{"title slash survives", analyzers.Finding{AnalyzerID: "semgrep", RuleID: "go/lang/injection", Severity: analyzers.SeverityCritical,
			Message: "m", RelativePath: "a.go"},
			"::error file=a.go,title=semgrep/go/lang/injection::m"},
		{"path backslashes become slashes", analyzers.Finding{AnalyzerID: "biome", RuleID: "R", Severity: analyzers.SeverityMedium,
			Message: "m", RelativePath: `pkg\cmd\tool main.go`},
			"::warning file=pkg/cmd/tool main.go,title=biome/R::m"},
		{"path comma and percent are encoded", analyzers.Finding{AnalyzerID: "ruff", RuleID: "R", Severity: analyzers.SeverityMedium,
			Message: "m", RelativePath: "a%b,c.py"},
			"::warning file=a%25b%2Cc.py,title=ruff/R::m"},
		{"unicode message survives verbatim", analyzers.Finding{AnalyzerID: "ruff", RuleID: "R", Severity: analyzers.SeverityMedium,
			Message: "日本語のメッセージ ☕ naïve", RelativePath: "a.py"},
			"::warning file=a.py,title=ruff/R::日本語のメッセージ ☕ naïve"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got := githubFirstLine(t, Input{WorkspaceName: "Esc", ScanID: "scan-e", Findings: []analyzers.Finding{item.finding}})
			if got != item.wantLine {
				t.Fatalf("annotation = %q\nwant        %q", got, item.wantLine)
			}
			if strings.Contains(got, `\`) {
				t.Fatalf("annotation must not carry a raw backslash: %q", got)
			}
		})
	}
}

// TestGitHubAnnotationsLevelMapping covers every normalized severity plus the
// unknown fallback: critical/high error, medium warning, low/info notice, and
// anything unmapped stays visible as a warning (sarifLevel's policy).
func TestGitHubAnnotationsLevelMapping(t *testing.T) {
	cases := []struct {
		severity analyzers.Severity
		prefix   string
	}{
		{analyzers.SeverityCritical, "::error "},
		{analyzers.SeverityHigh, "::error "},
		{analyzers.SeverityMedium, "::warning "},
		{analyzers.SeverityLow, "::notice "},
		{analyzers.SeverityInfo, "::notice "},
		{analyzers.Severity("catastrophic"), "::warning "},
	}
	for _, item := range cases {
		t.Run(string(item.severity), func(t *testing.T) {
			got := githubFirstLine(t, Input{Findings: []analyzers.Finding{{
				AnalyzerID: "ruff", RuleID: "R", Severity: item.severity, Message: "m", RelativePath: "a.py",
			}}})
			if !strings.HasPrefix(got, item.prefix) {
				t.Fatalf("severity %q must render %q, got %q", item.severity, item.prefix, got)
			}
		})
	}
}

// TestGitHubAnnotationsPositionHandling pins the zero-position conventions:
// zero line/column means "not stored" (csvNumber/JSON/SARIF treat it the
// same) and is omitted; a column without a line floors the line to 1 because
// col alone is not expressible.
func TestGitHubAnnotationsPositionHandling(t *testing.T) {
	cases := []struct {
		name     string
		finding  analyzers.Finding
		wantLine string
	}{
		{"line and column", analyzers.Finding{AnalyzerID: "ruff", RuleID: "R", Severity: analyzers.SeverityHigh,
			Message: "m", RelativePath: "a.py", StartLine: 3, StartColumn: 7},
			"::error file=a.py,line=3,col=7,title=ruff/R::m"},
		{"column absent omits col", analyzers.Finding{AnalyzerID: "ruff", RuleID: "R", Severity: analyzers.SeverityHigh,
			Message: "m", RelativePath: "a.py", StartLine: 3},
			"::error file=a.py,line=3,title=ruff/R::m"},
		{"no positions omits line and col", analyzers.Finding{AnalyzerID: "ruff", RuleID: "R", Severity: analyzers.SeverityHigh,
			Message: "m", RelativePath: "a.py"},
			"::error file=a.py,title=ruff/R::m"},
		{"column without line floors line to 1", analyzers.Finding{AnalyzerID: "ruff", RuleID: "R", Severity: analyzers.SeverityHigh,
			Message: "m", RelativePath: "a.py", StartColumn: 7},
			"::error file=a.py,line=1,col=7,title=ruff/R::m"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got := githubFirstLine(t, Input{Findings: []analyzers.Finding{item.finding}})
			if got != item.wantLine {
				t.Fatalf("annotation = %q\nwant        %q", got, item.wantLine)
			}
		})
	}
}

// TestGitHubAnnotationsOrdersAndCapsDeterministically pins the export order
// (slash-normalized path, start line, rule id — the JSON export's sort) and
// byte stability: shuffled input must render identical output.
func TestGitHubAnnotationsOrdersAndCapsDeterministically(t *testing.T) {
	newFinding := func(analyzer, rule, path string, severity analyzers.Severity, line int) analyzers.Finding {
		return analyzers.Finding{AnalyzerID: analyzer, RuleID: rule, Severity: severity, Message: "m", RelativePath: path, StartLine: line}
	}
	findings := []analyzers.Finding{
		newFinding("biome", "b2", "src/z.ts", analyzers.SeverityMedium, 9),
		newFinding("ruff", "F401", `src\a.py`, analyzers.SeverityHigh, 3),
		newFinding("ruff", "A01", "src/a.py", analyzers.SeverityHigh, 10),
		newFinding("ruff", "A00", "src/a.py", analyzers.SeverityCritical, 3),
	}
	in := Input{WorkspaceName: "Order", ScanID: "scan-o", Findings: findings}
	base := string(GitHubAnnotations(Build(in)))
	shuffled := in
	shuffled.Findings = []analyzers.Finding{findings[2], findings[0], findings[3], findings[1]}
	if base != string(GitHubAnnotations(Build(shuffled))) {
		t.Fatalf("shuffled findings must render byte-identical output:\n%s", base)
	}
	lines := githubAnnotationLines(base)
	want := []string{
		"::error file=src/a.py,line=3,title=ruff/A00::m",
		"::error file=src/a.py,line=3,title=ruff/F401::m",
		"::error file=src/a.py,line=10,title=ruff/A01::m",
		"::warning file=src/z.ts,line=9,title=biome/b2::m",
	}
	if len(lines) != len(want) {
		t.Fatalf("annotation lines = %#v\nwant %d lines", lines, len(want))
	}
	for i, wantLine := range want {
		if lines[i] != wantLine {
			t.Fatalf("annotation[%d] = %q, want %q", i, lines[i], wantLine)
		}
	}
	if !strings.HasSuffix(base, "\n") || strings.HasSuffix(base, "\n\n") {
		t.Fatalf("output must end with exactly one trailing LF:\n%q", base[len(base)-32:])
	}
	if strings.Contains(base, "\r") {
		t.Fatal("output must use LF line endings only")
	}
}

// TestGitHubAnnotationsTruncatesAtTenPerType pins the GitHub display limit —
// 10 error, 10 warning, 10 notice annotations per step — including the
// severity-first selection (criticals on late paths beat highs on early
// paths), the reserved notice slot for the truncation notice, and the full
// counts in both the truncation notice and the summary.
func TestGitHubAnnotationsTruncatesAtTenPerType(t *testing.T) {
	finding := func(rule, path string, severity analyzers.Severity) analyzers.Finding {
		return analyzers.Finding{AnalyzerID: "ruff", RuleID: rule, Severity: severity, Message: "m", RelativePath: path, StartLine: 1}
	}
	var findings []analyzers.Finding
	// 12 error findings: 7 highs on the earliest paths, 5 criticals on the
	// latest ones — selection must keep all criticals and drop 2 highs.
	for i := 1; i <= 7; i++ {
		findings = append(findings, finding("H", fmt.Sprintf("a%02d-high.py", i), analyzers.SeverityHigh))
	}
	for i := 1; i <= 5; i++ {
		findings = append(findings, finding("C", fmt.Sprintf("z%02d-critical.py", i), analyzers.SeverityCritical))
	}
	// 11 medium warnings.
	for i := 1; i <= 11; i++ {
		findings = append(findings, finding("M", fmt.Sprintf("m%02d-warning.py", i), analyzers.SeverityMedium))
	}
	// 15 notice findings: 13 infos then 2 lows — the severity-first cut keeps
	// both lows plus 7 infos.
	for i := 1; i <= 13; i++ {
		findings = append(findings, finding("I", fmt.Sprintf("i%02d-info.py", i), analyzers.SeverityInfo))
	}
	findings = append(findings, finding("L", "l01-low.py", analyzers.SeverityLow), finding("L", "l02-low.py", analyzers.SeverityLow))

	out := string(GitHubAnnotations(Build(Input{WorkspaceName: "Full", ScanID: "scan-full", Findings: findings})))
	var errors, warnings, notices, truncations int
	for _, line := range githubAnnotationLines(out) {
		switch {
		case strings.HasPrefix(line, "::error "):
			errors++
		case strings.HasPrefix(line, "::warning "):
			warnings++
		case strings.HasPrefix(line, "::notice ::Blunt Code truncated"):
			truncations++
		case strings.HasPrefix(line, "::notice "):
			notices++
		}
	}
	if errors != 10 {
		t.Fatalf("error annotations = %d, want 10 (GitHub's per-step cap)", errors)
	}
	if warnings != 10 {
		t.Fatalf("warning annotations = %d, want 10 (GitHub's per-step cap)", warnings)
	}
	if notices != 9 || truncations != 1 {
		t.Fatalf("finding notices = %d, truncation notices = %d; want 9 + 1 so the truncation notice fits the cap", notices, truncations)
	}
	if !strings.Contains(out, "this scan reported 5 critical, 7 high, 11 medium, 2 low, 13 info - 38 total") {
		t.Fatalf("truncation notice must carry the full counts:\n%s", out)
	}
	// Severity-first selection: every critical (latest paths) survives while
	// the last two highs (earliest paths of the error group) are dropped.
	for _, path := range []string{"z01-critical.py", "z05-critical.py", "a05-high.py"} {
		if !strings.Contains(out, "file="+path+",") {
			t.Fatalf("survivor %s missing from the truncated output:\n%s", path, out)
		}
	}
	for _, dropped := range []string{"a06-high.py", "a07-high.py", "m11-warning.py", "i08-info.py"} {
		if strings.Contains(out, "file="+dropped+",") {
			t.Fatalf("dropped finding %s must not be annotated:\n%s", dropped, out)
		}
	}
	if !strings.Contains(out, "file=l01-low.py,") || !strings.Contains(out, "file=i07-info.py,") {
		t.Fatalf("low-severity notices must outrank info notices in the cut:\n%s", out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	summary := lines[len(lines)-1]
	if strings.HasPrefix(summary, "::") {
		t.Fatalf("the final line must be the plain-text summary, not a command:\n%s", summary)
	}
	if summary != "Blunt Code scan of Full (scan scan-full): 5 critical, 7 high, 11 medium, 2 low, 13 info - 38 total." {
		t.Fatalf("summary line = %q", summary)
	}
}

// TestGitHubAnnotationsEmptyFindingsPrintsSummaryOnly pins that a clean scan
// emits no workflow commands at all — just the plain-text summary.
func TestGitHubAnnotationsEmptyFindingsPrintsSummaryOnly(t *testing.T) {
	out := string(GitHubAnnotations(Build(Input{WorkspaceName: "Clean", ScanID: "scan-clean"})))
	want := "Blunt Code scan of Clean (scan scan-clean): 0 critical, 0 high, 0 medium, 0 low, 0 info - 0 total.\n"
	if out != want {
		t.Fatalf("output = %q\nwant %q", out, want)
	}
}

// TestGitHubAnnotationsSurvivesHostileCorpus round-trips the shared
// adversarial fixture table through the export: no raw hostile control bytes
// or forged line breaks may survive into a command, the output stays valid
// UTF-8, and every line is either a workflow command or the summary tail.
// The corpus holds more than 10 notice-severity findings, so it doubles as a
// hostile-input truncation exercise.
func TestGitHubAnnotationsSurvivesHostileCorpus(t *testing.T) {
	body := GitHubAnnotations(Build(Input{
		WorkspaceName: "Hostile \"Name\" <script>", ScanID: "scan-hostile\x00",
		Findings: hostileCorpus(), Runs: []Run{hostileRun()},
	}))
	if !utf8.Valid(body) {
		t.Fatal("annotation stream must be valid UTF-8")
	}
	for _, banned := range []string{"\x00", "\x07", "\x0b", "\x0c", "\x1b", "\x7f", "\r"} {
		if bytes.Contains(body, []byte(banned)) {
			t.Fatalf("control character %q survived into the export", banned)
		}
	}
	sawTruncation, sawSummary := false, false
	for _, line := range strings.Split(strings.TrimSuffix(string(body), "\n"), "\n") {
		if line == "" {
			t.Fatalf("blank line in export:\n%s", body)
		}
		if strings.HasPrefix(line, "::notice ::Blunt Code truncated") {
			sawTruncation = true
			continue
		}
		if strings.HasPrefix(line, "::") {
			// One command per line: the data segment never carries a raw
			// newline, so a finding cannot forge a second command.
			if !strings.HasPrefix(line, "::error ") && !strings.HasPrefix(line, "::warning ") && !strings.HasPrefix(line, "::notice ") {
				t.Fatalf("malformed workflow command %q", line)
			}
			if !strings.Contains(line, "::") || strings.Count(line, "::") < 2 {
				t.Fatalf("command missing its data separator: %q", line)
			}
			continue
		}
		sawSummary = true
	}
	if !sawTruncation {
		t.Fatalf("corpus holds more than 10 notice findings; the truncation notice is missing:\n%s", body)
	}
	if !sawSummary {
		t.Fatalf("plain-text summary missing:\n%s", body)
	}
}

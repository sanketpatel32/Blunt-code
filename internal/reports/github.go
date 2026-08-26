package reports

import (
	"bluntcode/internal/analyzers"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// GitHub's Actions runner displays at most 10 annotations of each type (error,
// warning, notice) per workflow step; every further annotation command of that
// type is parsed but silently dropped instead of queued. Rendering more than
// that per type wastes output and hides the tail of the list, so the export
// caps itself and says so (see githubAnnotationCap).
const githubAnnotationCap = 10

// GitHubAnnotations renders the report model as a stream of GitHub Actions
// workflow commands — one annotation per finding — aimed at `bluntcode scan
// --format github` in CI. Each finding becomes
//
//	::error file=src/a.py,line=3,col=7,title=ruff/F401::unused import
//
// which the Actions runner turns into an inline PR annotation on the changed
// file. The commands must be the step's stdout, which is why the CLI writes
// this format to stdout while gate and baseline summaries keep going to
// stderr.
//
// Truncation: GitHub renders at most 10 error, 10 warning, and 10 notice
// annotations per step (githubAnnotationCap), dropping the rest. When a type
// overflows, the renderer keeps that type's most important findings —
// severity first, then the export's path/line/rule order — and closes with
// one ::notice carrying the full counts. That truncation notice is itself a
// notice, so whenever truncation will happen findings-derived notices are
// capped one slot lower (9), keeping the truncation notice inside GitHub's
// budget.
//
// The final line is a plain-text (non-command) summary with the workspace,
// scan id, and every severity count rather than another ::notice: plain text
// never competes for an annotation slot and is still useful in bare logs.
//
// Determinism and conventions mirror the JSON export: findings are ordered by
// slash-normalized path, start line, then rule id (sortedFindingsJSON), file
// paths use forward slashes without a leading "./" (jsonFilePath), and the
// output is LF-only with a single trailing newline. Like every export,
// suppressed findings never reach the model in the first place.
func GitHubAnnotations(m Model) []byte {
	return githubAnnotations(m, githubAnnotationCap)
}

// GitHubAnnotationsWithCap renders the annotation stream with a custom
// per-type display cap (GitHub's own hard limit is 10 per type per step; the
// cap here only decides how many are *shown* before the truncation notice).
func GitHubAnnotationsWithCap(m Model, cap int) []byte {
	if cap < 1 {
		cap = githubAnnotationCap
	}
	return githubAnnotations(m, cap)
}

func githubAnnotations(m Model, annotationCap int) []byte {
	findings := sortedFindingsJSON(m.Findings)
	levels := make([]string, len(findings))
	counts := map[string]int{"error": 0, "warning": 0, "notice": 0}
	for i, f := range findings {
		levels[i] = githubLevel(f.Severity)
		counts[levels[i]]++
	}
	// A truncation notice is emitted exactly when any type overflows the
	// display cap; reserve one notice slot for it in that case.
	noticeCap := annotationCap
	truncated := counts["error"] > annotationCap || counts["warning"] > annotationCap || counts["notice"] > annotationCap
	if truncated {
		noticeCap = annotationCap - 1
	}
	caps := map[string]int{"error": annotationCap, "warning": annotationCap, "notice": noticeCap}
	// Selection: per overflowing type keep the top findings by severity, then
	// path — the candidates are already in the export's (path, line, rule)
	// order, so a stable sort on severity alone yields exactly that tiebreak.
	// Survivors are emitted in the export order below.
	kept := make([]bool, len(findings))
	byLevel := map[string][]int{"error": {}, "warning": {}, "notice": {}}
	for i := range findings {
		byLevel[levels[i]] = append(byLevel[levels[i]], i)
	}
	for level, candidates := range byLevel {
		if len(candidates) <= caps[level] {
			for _, i := range candidates {
				kept[i] = true
			}
			continue
		}
		ordered := append([]int(nil), candidates...)
		sort.SliceStable(ordered, func(a, b int) bool {
			return githubSeverityRank(findings[ordered[a]].Severity) < githubSeverityRank(findings[ordered[b]].Severity)
		})
		for _, i := range ordered[:caps[level]] {
			kept[i] = true
		}
	}
	var b strings.Builder
	for i, f := range findings {
		if !kept[i] {
			continue
		}
		b.WriteString(githubCommand(f, levels[i]))
		b.WriteByte('\n')
	}
	if truncated {
		b.WriteString("::notice ::Blunt Code truncated its GitHub annotations to GitHub's display limit (10 per error/warning/notice per step); this scan reported " +
			githubSeverityPhrase(m) + ".\n")
	}
	b.WriteString("Blunt Code scan of " + scrubControls(m.WorkspaceName) + " (scan " + scrubControls(m.ScanID) + "): " +
		githubSeverityPhrase(m) + ".\n")
	return []byte(b.String())
}

// githubCommand renders one finding as a workflow command line. Property
// order is fixed (file, line, col, title) so identical findings always render
// identical bytes; the message is the command's data segment.
func githubCommand(f analyzers.Finding, level string) string {
	var props []string
	if path := jsonFilePath(f.RelativePath); path != "" {
		props = append(props, "file="+githubEscapeProperty(path))
	}
	line, col := githubPosition(f)
	if line > 0 {
		props = append(props, "line="+strconv.Itoa(line))
		if col > 0 {
			props = append(props, "col="+strconv.Itoa(col))
		}
	}
	props = append(props, "title="+githubEscapeProperty(f.AnalyzerID+"/"+f.RuleID))
	return "::" + level + " " + strings.Join(props, ",") + "::" + githubEscapeData(scrubControls(strings.TrimSpace(f.Message)))
}

// githubPosition projects a finding's stored line/column onto the annotation
// protocol. Zero means "not stored" everywhere else in the exports
// (csvNumber's empty cell, the JSON document's omitempty fields, SARIF's
// dropped region), so a zero line or column is omitted here too — GitHub
// renders an annotation without a line on the file's first line by itself.
// The one floor: a column without a line cannot be expressed (col is only
// meaningful together with line), so a finding that stored a column but no
// line is pinned to line 1 rather than losing the column silently.
func githubPosition(f analyzers.Finding) (line, col int) {
	line, col = f.StartLine, f.StartColumn
	if line <= 0 && col > 0 {
		line = 1
	}
	return line, col
}

// githubLevel maps the five normalized severities onto the three annotation
// types. Unlike the SARIF projection (medium and low both warning), low rides
// with info as notice: an annotation per low-severity style nit would crowd
// out the notices that matter in the PR view. An unmapped severity stays
// visible as a warning, the same policy sarifLevel applies.
func githubLevel(severity analyzers.Severity) string {
	switch severity {
	case analyzers.SeverityCritical, analyzers.SeverityHigh:
		return "error"
	case analyzers.SeverityMedium:
		return "warning"
	case analyzers.SeverityLow, analyzers.SeverityInfo:
		return "notice"
	default:
		return "warning"
	}
}

// githubSeverityRank orders severities for the truncation cut; unknown
// severities sort last behind the five normalized ones.
func githubSeverityRank(severity analyzers.Severity) int {
	switch severity {
	case analyzers.SeverityCritical:
		return 0
	case analyzers.SeverityHigh:
		return 1
	case analyzers.SeverityMedium:
		return 2
	case analyzers.SeverityLow:
		return 3
	case analyzers.SeverityInfo:
		return 4
	default:
		return 5
	}
}

// githubSeverityPhrase renders the model's severity counts as the shared tail
// of the truncation notice and the summary line, matching the phrasing of the
// CLI's human summary ("2 critical, 1 high, ... - 7 total").
func githubSeverityPhrase(m Model) string {
	return strconv.Itoa(m.Counts[analyzers.SeverityCritical]) + " critical, " +
		strconv.Itoa(m.Counts[analyzers.SeverityHigh]) + " high, " +
		strconv.Itoa(m.Counts[analyzers.SeverityMedium]) + " medium, " +
		strconv.Itoa(m.Counts[analyzers.SeverityLow]) + " low, " +
		strconv.Itoa(m.Counts[analyzers.SeverityInfo]) + " info - " +
		strconv.Itoa(len(m.Findings)) + " total"
}

// githubEscapeData percent-encodes the characters that would break the
// command's data (message) segment: annotations are single-line, so LF and CR
// become %0A and %0D, and a literal % is encoded first so the runner cannot
// decode a forged %0A smuggled through a raw percent sign. Commas and colons
// stay: the data segment runs to end of line and delimits nothing.
//
// Text reaches the escaper after scrubControls, which replaces the hostile
// C0 controls and DEL with spaces (the same scrub every other export applies)
// so an encoded NUL-style control can never re-enter GitHub's annotation
// record. Iterating runes also rewrites invalid UTF-8 as U+FFFD, keeping the
// output valid UTF-8.
func githubEscapeData(s string) string {
	return githubEscape(s, false)
}

// githubEscapeProperty additionally encodes the property delimiters: within
// the `file={...},line={...}` segment a comma would start a new property and
// a colon would split its name from its value, so both (and the percent that
// could forge either) are percent-encoded — the same escape set the actions
// toolkit's escapeProperty applies.
func githubEscapeProperty(s string) string {
	return githubEscape(s, true)
}

func githubEscape(s string, property bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '%':
			b.WriteString("%25")
		case r == '\r':
			b.WriteString("%0D")
		case r == '\n':
			b.WriteString("%0A")
		case r < 0x20 || r == 0x7f:
			// Remaining control bytes (tab and anything else scrubControls
			// kept); uppercase zero-padded percent-hex like the toolkit's.
			b.WriteString(fmt.Sprintf("%%%02X", r))
		case property && r == ':':
			b.WriteString("%3A")
		case property && r == ',':
			b.WriteString("%2C")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

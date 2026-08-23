package analyzers

import "strings"

// Inline "bluntcode:ignore" directives: in-source suppression for the
// BUILT-IN analyzers (secrets and todo). A false positive — a documented
// example key in a test fixture, a TODO marker kept on purpose — can be
// dismissed at the site, in source, reviewable in git. This complements the
// DB-stored suppression workflow; it replaces nothing. External tools keep
// their own mechanisms (ruff "# noqa", biome "// biome-ignore", semgrep
// "# nosemgrep") and never see these directives.
//
// # Syntax — this comment is the canonical documentation
//
// A line comment containing the exact token bluntcode:ignore, matched
// case-sensitively anywhere in the line (so prose like "BluntCode:Ignore"
// never fires). The comment leader is whatever the file's language uses;
// matching is language-agnostic and simply searches the line for the token:
//
//	# bluntcode:ignore
//	// bluntcode:ignore
//	<!-- bluntcode:ignore -->
//	/* bluntcode:ignore */
//
// What may follow the token, separated by spaces:
//
//   - an optional single rule id targets the directive at exactly one rule.
//     One directive names one rule; multiple rule ids are not supported —
//     write one directive per rule:
//
//	# bluntcode:ignore secrets.aws-access-key-id
//
//   - an optional reason, introduced by the literal marker "reason:", is
//     free text to end of line. It exists so users can annotate; it is not
//     parsed beyond being skipped and is recorded nowhere:
//
//	// bluntcode:ignore reason: example key for docs
//	/* bluntcode:ignore todo.fixme reason: accepted until v2 */
//
// A bare directive (nothing but an optional reason after the token)
// suppresses ANY built-in finding it applies to. A word that is neither a
// "reason:" marker nor a rule id is taken as a rule id that matches
// nothing, so the directive is inert — a typo fails safe (the finding
// stays visible) rather than silently suppressing.
//
// # Placement
//
// SAME-LINE: a directive on the finding's line suppresses that finding.
// PREVIOUS-LINE: a directive on the line immediately above — contiguous,
// no blank line between — suppresses findings on the next line. Nothing
// further: no multi-line ranges, no file-level pragma (v1 stays small).
// Only the first directive token on a line counts.
//
// # Scope and pipeline shape
//
// Suppression applies per finding, in Normalize, and only for the built-in
// adapters that carry directives: Run parses the two relevant lines into
// the JSON diagnostic envelope (as IgnoreDirective values, never as raw
// source text — a secrets finding's own line contains the very secret
// being reported) and Normalize drops matching diagnostics before findings
// are built. A directive therefore never affects another analyzer's
// findings, even on the same line. Suppression happens before
// fingerprinting, so surviving findings keep exactly the fingerprints they
// would have had without any directive nearby.

// IgnoreDirectiveToken is the single canonical inline suppression marker.
const IgnoreDirectiveToken = "bluntcode:ignore"

// IgnoreDirective is one parsed inline directive. Rule is the targeted rule
// id; the zero value is a bare directive that applies to any built-in
// finding, so it marshals compactly and round-trips through the adapters'
// JSON envelopes. A nil *IgnoreDirective means the line carried no
// directive at all.
type IgnoreDirective struct {
	Rule string `json:"rule,omitempty"`
}

// matches reports whether the directive suppresses a finding of ruleID.
// nil (no directive on the line) suppresses nothing.
func (d *IgnoreDirective) matches(ruleID string) bool {
	return d != nil && (d.Rule == "" || d.Rule == ruleID)
}

// ParseIgnoreDirective reports the directive carried by one line of source.
// found is false when the line does not contain the token; rule is the
// targeted rule id, empty for a bare directive. The token is matched
// case-sensitively anywhere in the line, and only its first occurrence
// counts.
func ParseIgnoreDirective(line string) (rule string, found bool) {
	index := strings.Index(line, IgnoreDirectiveToken)
	if index < 0 {
		return "", false
	}
	rest := strings.TrimLeft(line[index+len(IgnoreDirectiveToken):], " \t")
	word := rest
	if end := strings.IndexAny(rest, " \t"); end >= 0 {
		word = rest[:end]
	}
	if strings.HasPrefix(word, "reason:") {
		return "", true
	}
	// Comment closers glued to the token ("-->", "*/", trailing "//" or "#")
	// must not turn a bare directive into a rule id named "-->"; real rule
	// ids never end with these characters, so trimming them is safe.
	word = strings.TrimRight(word, " \t\r#*/>-")
	if word == "" {
		return "", true
	}
	return word, true
}

// IgnoreContextLines returns the source text of the 1-based line and of the
// line immediately above it. prevLine is empty for line 1, and both are
// empty when line is out of range. Trailing CR is stripped so CRLF files
// behave like LF files.
func IgnoreContextLines(data []byte, line int) (text, prevLine string) {
	if line < 1 {
		return "", ""
	}
	current := 1
	for start := 0; start < len(data); {
		end := start
		for end < len(data) && data[end] != '\n' {
			end++
		}
		contentEnd := end
		if contentEnd > start && data[contentEnd-1] == '\r' {
			contentEnd--
		}
		if current == line {
			return string(data[start:contentEnd]), prevLine
		}
		prevLine = string(data[start:contentEnd])
		current++
		start = end + 1
	}
	return "", ""
}

// directivesFromLines parses the directives carried by a finding's own line
// and its previous line.
func directivesFromLines(line, prevLine string) (same, prev *IgnoreDirective) {
	if rule, found := ParseIgnoreDirective(line); found {
		same = &IgnoreDirective{Rule: rule}
	}
	if rule, found := ParseIgnoreDirective(prevLine); found {
		prev = &IgnoreDirective{Rule: rule}
	}
	return same, prev
}

// IgnoreDirectivesAt parses the inline directives relevant to a finding on
// the given 1-based line of data: same is the directive on the finding's
// own line, prev the one on the line immediately above; either is nil when
// that line carries no directive. Built-in adapters call this at Run time
// so Normalize stays pure and raw source lines — which for the secrets
// analyzer contain the very secret being reported — never travel in a
// serialized result.
func IgnoreDirectivesAt(data []byte, line int) (same, prev *IgnoreDirective) {
	lineText, prevText := IgnoreContextLines(data, line)
	return directivesFromLines(lineText, prevText)
}

// IgnoreSuppressedDirectives is the envelope form of the check: it answers
// from directives parsed at Run time (what an adapter's Normalize holds)
// instead of raw source lines. same is the directive on the finding's line,
// prev the one on the line above; either may be nil.
func IgnoreSuppressedDirectives(same, prev *IgnoreDirective, ruleID string) bool {
	return same.matches(ruleID) || prev.matches(ruleID)
}

// IgnoreSuppressed reports whether a built-in finding with the given rule id
// is suppressed by inline directives: it checks the finding's own line
// (same-line directive) and the line immediately above it (previous-line
// directive). prevLine is empty for a first-line finding, and a blank
// previous line naturally breaks the chain because it carries no token.
// The check is pure: rule id plus two lines of text in, boolean out.
func IgnoreSuppressed(ruleID, line, prevLine string) bool {
	same, prev := directivesFromLines(line, prevLine)
	return IgnoreSuppressedDirectives(same, prev, ruleID)
}

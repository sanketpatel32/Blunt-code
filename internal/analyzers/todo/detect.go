package todo

// Pure detection engine for the built-in TODO/FIXME comment tracker. Everything
// in this file operates on byte slices held in memory and never touches the
// network, the filesystem, or an external process, so the detector is fully
// testable and safe to run inside the scan pipeline.
//
// Rule IDs are STABLE public identifiers: finding fingerprints hash them, so
// renaming a rule would orphan every stored finding. The scheme is
// todo.<marker> and the set is fixed unless a rule is retired deliberately.

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Scanning hygiene limits. Files larger than maxScanBytes are read only up to
// the cap, files containing a NUL byte in their first binarySniffBytes bytes
// are treated as binary and skipped, and detection results are capped per file
// and per run so a pathological input cannot flood a scan.
const (
	maxScanBytes       = 1 << 20 // 1 MiB
	binarySniffBytes   = 8 << 10 // 8 KiB
	maxFindingsPerFile = 200
	maxFindingsPerRun  = 5000
)

// Message hygiene: the comment text that follows a marker is trimmed and then
// truncated to maxMessageRunes runes (an ellipsis marks the cut), so a finding
// never carries a whole minified line into a report.
const maxMessageRunes = 200

// Stable rule identifiers, one per tracked marker word.
const (
	ruleTODO  = "todo.todo"
	ruleFIXME = "todo.fixme"
	ruleHACK  = "todo.hack"
	ruleXXX   = "todo.xxx"
	ruleBUG   = "todo.bug"
)

// rulesByMarker maps each marker word to its stable rule id. Matching is
// case-sensitive on purpose: `todo` in prose or a lowercase identifier is not
// procrastination debt, and accepting it would flood scans with noise.
var rulesByMarker = map[string]string{
	"TODO":  ruleTODO,
	"FIXME": ruleFIXME,
	"HACK":  ruleHACK,
	"XXX":   ruleXXX,
	"BUG":   ruleBUG,
}

// markerRe matches the tracked markers with word boundaries on both sides, so
// markers that are part of a longer identifier (TODOS, TODONT, TODO_LIST,
// BUGFIX) never fire. \b in RE2 is the ASCII word boundary, which is exactly
// the letters/digits/underscore set identifiers are built from.
var markerRe = regexp.MustCompile(`\b(TODO|FIXME|HACK|XXX|BUG)\b`)

// match is one raw detection: the marker occurrence plus everything a finding
// needs. The message never carries the raw line beyond the marker and its
// trailing comment text.
type match struct {
	rule    string
	marker  string
	line    int // 1-based line of the marker
	column  int // 1-based rune column of the marker
	message string
}

// detect runs the marker scan over one file's content, line by line. A marker
// counts only when it is followed by a colon, whitespace, or the end of the
// line: `TODO:` and `TODO refactor` are debt, `TODOS` and `TODOING` are not.
func detect(data []byte) []match {
	var out []match
	lineNo, lineStart := 1, 0
	for lineStart < len(data) {
		lineEnd := lineStart
		for lineEnd < len(data) && data[lineEnd] != '\n' {
			lineEnd++
		}
		// A trailing CR (CRLF files) is part of the line terminator, not text,
		// so a bare marker right before \r\n still counts as end of line.
		contentEnd := lineEnd
		if contentEnd > lineStart && data[contentEnd-1] == '\r' {
			contentEnd--
		}
		out = append(out, detectLine(lineNo, data[lineStart:contentEnd])...)
		lineNo++
		lineStart = lineEnd + 1
	}
	return out
}

// detectLine reports every qualifying marker occurrence on one line; each
// occurrence yields exactly one match.
func detectLine(lineNo int, line []byte) []match {
	var out []match
	for _, loc := range markerRe.FindAllIndex(line, -1) {
		marker := string(line[loc[0]:loc[1]])
		if !followerCountsAsDebt(line, loc[1]) {
			continue
		}
		out = append(out, match{
			rule:    rulesByMarker[marker],
			marker:  marker,
			line:    lineNo,
			column:  runeColumn(line, loc[0]),
			message: messageFor(line, loc[0]),
		})
	}
	return out
}

// followerCountsAsDebt accepts the tracked-marker shapes: a colon (TODO:),
// whitespace before more text (TODO refactor), or nothing at all (a bare TODO
// at the end of the line). Anything else — punctuation like `TODO.` or
// `TODO(name)` — means the word appeared without the marker convention, so it
// is not counted. A marker inside a string literal can still match; that is
// accepted noise for a line-oriented scan.
func followerCountsAsDebt(line []byte, markerEnd int) bool {
	if markerEnd >= len(line) {
		return true
	}
	switch line[markerEnd] {
	case ':', ' ', '\t':
		return true
	}
	return false
}

// messageFor builds the finding text: the marker plus the comment text after
// it, with control bytes scrubbed to spaces, trimmed, and truncated to
// maxMessageRunes runes. Nothing before the marker on the line is copied.
func messageFor(line []byte, markerStart int) string {
	var b strings.Builder
	for _, r := range string(line[markerStart:]) {
		if r < 0x20 || r == 0x7f {
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
	}
	text := strings.TrimSpace(b.String())
	runes := []rune(text)
	if len(runes) <= maxMessageRunes {
		return text
	}
	return string(runes[:maxMessageRunes]) + "…"
}

// runeColumn translates a byte offset inside a line into a 1-based rune
// column, so a marker after emoji or other multibyte text still points where
// a human reads it.
func runeColumn(line []byte, offset int) int {
	column := 1
	for i := 0; i < offset; {
		_, size := utf8.DecodeRune(line[i:])
		column++
		i += size
	}
	return column
}

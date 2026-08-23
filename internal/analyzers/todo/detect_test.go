package todo

// Pure detector tests. The inputs double as fixtures proving that word
// boundaries, case sensitivity, and the follower check keep identifiers,
// prose, and non-marker punctuation from firing.

import (
	"fmt"
	"strings"
	"testing"
)

func rulesOf(ms []match) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.rule)
	}
	return out
}

func TestMarkerDetection(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantRules []string
	}{
		// Every tracked marker fires in its natural comment shape.
		{"todo colon", "// TODO: refactor this", []string{ruleTODO}},
		{"fixme hash comment", "# FIXME: broken on Windows", []string{ruleFIXME}},
		{"hack block comment", "/* HACK: until the API lands */", []string{ruleHACK}},
		{"xxx marker", "// XXX: not sure about this", []string{ruleXXX}},
		{"bug marker", "// BUG: racy under load", []string{ruleBUG}},
		// Follower shapes: colon, whitespace before text, bare at end of line.
		{"todo followed by space", "// TODO refactor later", []string{ruleTODO}},
		{"todo followed by tab", "// TODO\tbatch these", []string{ruleTODO}},
		{"fixme bare at end of line", "process(); // FIXME", []string{ruleFIXME}},
		{"todo colon at end of line", "// TODO:", []string{ruleTODO}},
		{"trailing space after marker", "// HACK ", []string{ruleHACK}},
		{"bare marker as whole line", "TODO", []string{ruleTODO}},
		{"crlf bare marker at end of line", "// TODO\r\nx = 1\n", []string{ruleTODO}},
		// Word boundaries on both sides: longer identifiers never fire.
		{"todos plural", "// TODOS: the list", nil},
		{"todont", "// TODONT", nil},
		{"todo underscore suffix", "// TODO_LIST", nil},
		{"todo digits suffix", "// TODO123", nil},
		{"todo verb suffix", "// TODOING it now", nil},
		{"marker embedded in word start", "// XTODO: nope", nil},
		{"marker embedded with underscore prefix", "// _FIXME: nope", nil},
		{"bugfix is not a marker pair", "// BUGFIX: not tracked", nil},
		// Case sensitivity: lowercase prose is not a marker.
		{"lowercase todo", "// todo: lowercase prose", nil},
		{"lowercase fixme", "// fixme later", nil},
		{"mixed case Todo", "// Todo: sentence start", nil},
		{"mixed case Bug", "// Bug: prose", nil},
		// Follower must be a colon, whitespace, or end of line.
		{"todo followed by parenthesis", "// TODO(name): attribution style", nil},
		{"todo followed by period", "// TODO.", nil},
		{"todo followed by hyphen", "// TODO-maybe", nil},
		// A marker inside a string literal is accepted noise: the scan is
		// line-oriented by design.
		{"marker inside string literal", `const label = "TODO: ship it";`, []string{ruleTODO}},
		// Multiple markers on one line each fire.
		{"two markers one line", "// TODO: alpha FIXME: beta", []string{ruleTODO, ruleFIXME}},
		{"repeated same marker one line", "// TODO first TODO second", []string{ruleTODO, ruleTODO}},
		// Multi-line placement.
		{"markers on separate lines", "// TODO: one\nx = 1\n// FIXME: two\n", []string{ruleTODO, ruleFIXME}},
		{"marker on last line without newline", "// TODO: tail", []string{ruleTODO}},
		{"no markers", "package main\n\nfunc main() {}\n", nil},
		{"empty input", "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := rulesOf(detect([]byte(c.input)))
			if len(got) != len(c.wantRules) {
				t.Fatalf("detect(%q) rules = %v, want %v", c.input, got, c.wantRules)
			}
			for i := range got {
				if got[i] != c.wantRules[i] {
					t.Fatalf("detect(%q) rules = %v, want %v", c.input, got, c.wantRules)
				}
			}
		})
	}
}

func TestMarkerLinesAreOneBasedPerOccurrence(t *testing.T) {
	data := []byte("# TODO: first\nprint(1)\n# TODO: second\nprint(2)\n# TODO: third\n")
	ms := detect(data)
	if len(ms) != 3 {
		t.Fatalf("detect returned %d matches, want 3: %+v", len(ms), ms)
	}
	for i, want := range []int{1, 3, 5} {
		if ms[i].line != want {
			t.Fatalf("match %d line = %d, want %d", i, ms[i].line, want)
		}
	}
}

func TestMatchPositionAndMessage(t *testing.T) {
	data := []byte("import os\n  // FIXME: refactor this before release\nx = 1\n")
	ms := detect(data)
	if len(ms) != 1 {
		t.Fatalf("detect returned %d matches, want 1: %+v", len(ms), ms)
	}
	m := ms[0]
	if m.marker != "FIXME" {
		t.Fatalf("marker = %q, want FIXME", m.marker)
	}
	if m.line != 2 {
		t.Fatalf("line = %d, want 2", m.line)
	}
	// Runes on line 2: space, space, /, /, space, then F at column 6.
	if m.column != 6 {
		t.Fatalf("column = %d, want 6", m.column)
	}
	if m.message != "FIXME: refactor this before release" {
		t.Fatalf("message = %q, want the marker plus the comment text", m.message)
	}
	// The message must not carry the line before the marker.
	if strings.Contains(m.message, "//") {
		t.Fatalf("message leaks the comment prefix: %q", m.message)
	}
}

func TestColumnCountsRunesNotBytes(t *testing.T) {
	// One 4-byte emoji up front: byte offsets and rune columns diverge.
	line := []byte("/* 🎉 TODO */")
	ms := detectLine(1, line)
	if len(ms) != 1 {
		t.Fatalf("detectLine returned %d matches, want 1: %+v", len(ms), ms)
	}
	// Runes: / * space 🎉 space, then T at column 6 (bytes would say 9).
	if ms[0].column != 6 {
		t.Fatalf("column = %d, want 6 (runes, not bytes)", ms[0].column)
	}
}

func TestMessageTruncatesToLimit(t *testing.T) {
	long := strings.Repeat("a", maxMessageRunes+50)
	ms := detectLine(1, []byte("// TODO: "+long))
	if len(ms) != 1 {
		t.Fatalf("got %d matches, want 1", len(ms))
	}
	msg := ms[0].message
	if n := len([]rune(msg)); n != maxMessageRunes+1 { // cap plus the ellipsis
		t.Fatalf("message has %d runes, want %d", n, maxMessageRunes+1)
	}
	if !strings.HasSuffix(msg, "…") {
		t.Fatalf("truncated message must end with an ellipsis: %q…", msg[:32])
	}
	if want := "TODO: " + strings.Repeat("a", maxMessageRunes-len("TODO: ")); !strings.HasPrefix(msg, want) {
		t.Fatal("truncated message lost the marker or leading text")
	}
	// Exactly at the cap: no ellipsis.
	exact := strings.Repeat("b", maxMessageRunes-len("TODO: "))
	ms = detectLine(1, []byte("// TODO: "+exact))
	if len(ms) != 1 || ms[0].message != "TODO: "+exact {
		t.Fatalf("at-cap message = %q, want exactly the capped text with no ellipsis", ms[0].message)
	}
}

func TestMessageTruncationIsRuneSafe(t *testing.T) {
	// Multibyte runes after the marker: truncation must cut whole runes.
	text := strings.Repeat("é", maxMessageRunes+50)
	ms := detectLine(1, []byte("// TODO: "+text))
	if len(ms) != 1 {
		t.Fatalf("got %d matches, want 1", len(ms))
	}
	if n := len([]rune(ms[0].message)); n != maxMessageRunes+1 {
		t.Fatalf("message has %d runes, want %d", n, maxMessageRunes+1)
	}
	if !strings.HasSuffix(ms[0].message, "é…") {
		t.Fatal("truncated message mangled the final rune")
	}
}

func TestMessageScrubsControlBytes(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{"bell and stx and nul scrubbed", "// TODO:\x07bell\x02done\x00", "TODO: bell done"},
		{"del scrubbed", "// HACK: a\x7fb", "HACK: a b"},
		{"tab becomes space", "// TODO:\tspaced\t", "TODO: spaced"},
		{"only control bytes after marker", "// XXX:\x01", "XXX:"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ms := detectLine(1, []byte(c.line))
			if len(ms) != 1 {
				t.Fatalf("got %d matches, want 1: %+v", len(ms), ms)
			}
			if ms[0].message != c.want {
				t.Fatalf("message = %q, want %q", ms[0].message, c.want)
			}
			for _, r := range ms[0].message {
				if r < 0x20 || r == 0x7f {
					t.Fatalf("message still contains control rune %q: %q", r, ms[0].message)
				}
			}
		})
	}
}

func TestBareMarkerKeepsUsableMessage(t *testing.T) {
	// A bare marker at end of line still yields a non-empty message so the
	// finding passes validation.
	ms := detectLine(1, []byte("process(); // FIXME"))
	if len(ms) != 1 {
		t.Fatalf("got %d matches, want 1", len(ms))
	}
	if ms[0].message != "FIXME" {
		t.Fatalf("message = %q, want %q", ms[0].message, "FIXME")
	}
}

func TestPerFileCapInsideScanFile(t *testing.T) {
	var lines []string
	for i := 0; i < maxFindingsPerFile+10; i++ {
		lines = append(lines, fmt.Sprintf("// TODO: item %d", i))
	}
	diagnostics, truncated := scanFile("cap.go", []byte(strings.Join(lines, "\n")+"\n"))
	if !truncated {
		t.Fatal("scanFile did not report truncation past the per-file cap")
	}
	if len(diagnostics) != maxFindingsPerFile {
		t.Fatalf("scanFile returned %d diagnostics, want the cap of %d", len(diagnostics), maxFindingsPerFile)
	}
}

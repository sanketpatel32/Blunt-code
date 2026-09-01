package analyzers

// Unit tests for the inline bluntcode:ignore directive machinery — the pure
// core shared by the built-in secrets and todo analyzers. End-to-end
// coverage (fixture files scanned through Run -> Normalize) lives in those
// adapters' packages; everything here drives the pure functions directly.

import "testing"

func TestParseIgnoreDirective(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		rule  string
		found bool
	}{
		{"hash leader, bare", "# bluntcode:ignore", "", true},
		{"slash leader, bare, padded", "  //   bluntcode:ignore  ", "", true},
		{"html leader, bare", "<!-- bluntcode:ignore -->", "", true},
		{"block leader, bare", "/* bluntcode:ignore */", "", true},
		{"rule id after the token", "# bluntcode:ignore secrets.aws-access-key-id", "secrets.aws-access-key-id", true},
		{"rule id with a reason", "// bluntcode:ignore todo.fixme reason: accepted until v2", "todo.fixme", true},
		{"reason marker makes the directive bare", "<!-- bluntcode:ignore reason: example key for docs -->", "", true},
		{"reason marker without a space", "/* bluntcode:ignore reason:docs */", "", true},
		{"tight html closer is tolerated", "<!-- bluntcode:ignore todo.fixme-->", "todo.fixme", true},
		{"tight block closer is tolerated", "/* bluntcode:ignore secrets.jwt*/", "secrets.jwt", true},
		{"token anywhere in the line", "# TODO: revisit; bluntcode:ignore todo.todo", "todo.todo", true},
		{"unknown word is a rule id that matches nothing (fail safe)", "# bluntcode:ignore oops", "oops", true},
		{"crlf line ending is stripped", "# bluntcode:ignore todo.todo\r", "todo.todo", true},
		{"mixed-case prose does not contain the token", "// BluntCode:Ignore the noise", "", false},
		{"upper-case prose does not contain the token", "# BLUNTCODE:IGNORE", "", false},
		{"no directive", "just a comment", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule, found := ParseIgnoreDirective(tc.line)
			if found != tc.found || rule != tc.rule {
				t.Fatalf("ParseIgnoreDirective(%q) = (%q, %v), want (%q, %v)", tc.line, rule, found, tc.rule, tc.found)
			}
		})
	}
}

func TestIgnoreSuppressed(t *testing.T) {
	const aws = "secrets.aws-access-key-id"
	cases := []struct {
		name     string
		ruleID   string
		line     string
		prevLine string
		want     bool
	}{
		{"bare directive on the finding's line", aws, `aws_key = "AKIA` + `1234567890ABCDEF" # bluntcode:ignore`, "", true},
		{"bare directive on the previous line", "todo.todo", "x = 1", "# bluntcode:ignore", true},
		{"targeted directive on the finding's line, matching rule", "secrets.jwt", `token := "eyJ` + `hbGci.x.y" // bluntcode:ignore secrets.jwt`, "", true},
		{"targeted directive on the previous line, matching rule", "todo.fixme", "# FIXME: handle empty input", "# bluntcode:ignore todo.fixme", true},
		{"targeted directive on the finding's line, different rule", aws, `aws_key = "AKIA` + `1234567890ABCDEF" # bluntcode:ignore secrets.jwt`, "", false},
		{"targeted directive on the previous line, different rule", "todo.fixme", "# FIXME: handle empty input", "# bluntcode:ignore todo.hack", false},
		{"directive naming another analyzer's rule leaves the finding visible", "todo.todo", "// TODO: x // bluntcode:ignore secrets.aws-access-key-id", "", false},
		{"unknown word after the directive is inert, not bare", "todo.todo", "# TODO: x", "# bluntcode:ignore oops", false},
		{"mixed-case prose never matches", "secrets.jwt", "// BluntCode:Ignore this line, please", "", false},
		{"upper-case prose never matches", "todo.todo", "x = 1", "# BLUNTCODE:IGNORE", false},
		{"first line has no previous line", "todo.todo", "# TODO: first", "", false},
		{"whitespace-only previous line breaks the chain", "todo.todo", "# TODO: far", "   ", false},
		{"reason tolerated after a bare directive", aws, "x = 1", "<!-- bluntcode:ignore reason: example key for docs -->", true},
		{"reason tolerated after a rule id", "todo.fixme", "# FIXME: x", "/* bluntcode:ignore todo.fixme reason: accepted until v2 */", true},
		{"hash leader recognized", "secrets.jwt", `token = "x" # bluntcode:ignore`, "", true},
		{"slash leader recognized", "todo.hack", "// HACK: x // bluntcode:ignore", "", true},
		{"html leader recognized", "todo.todo", "TODO: x <!-- bluntcode:ignore -->", "", true},
		{"block leader recognized", "todo.xxx", "/* XXX */ bluntcode:ignore", "", true},
		{"no directive at all", "secrets.jwt", `token = "abc"`, "# just a comment", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IgnoreSuppressed(tc.ruleID, tc.line, tc.prevLine); got != tc.want {
				t.Fatalf("IgnoreSuppressed(%q, %q, %q) = %v, want %v", tc.ruleID, tc.line, tc.prevLine, got, tc.want)
			}
		})
	}
}

func TestIgnoreSuppressedDirectivesNilMeansNoDirective(t *testing.T) {
	bare := &IgnoreDirective{}
	targeted := &IgnoreDirective{Rule: "todo.fixme"}
	if IgnoreSuppressedDirectives(nil, nil, "todo.fixme") {
		t.Fatal("no directives must not suppress")
	}
	if !IgnoreSuppressedDirectives(bare, nil, "secrets.jwt") {
		t.Fatal("a bare directive suppresses any rule")
	}
	if !IgnoreSuppressedDirectives(nil, targeted, "todo.fixme") {
		t.Fatal("a matching targeted directive suppresses")
	}
	if IgnoreSuppressedDirectives(nil, targeted, "todo.hack") {
		t.Fatal("a mismatched targeted directive must not suppress")
	}
}

func TestIgnoreDirectivesAt(t *testing.T) {
	data := []byte("# plain\n# bluntcode:ignore todo.todo\n# TODO: x\n")
	same, prev := IgnoreDirectivesAt(data, 3)
	if same != nil {
		t.Fatalf("line 3 carries no directive, got same = %+v", same)
	}
	if prev == nil || prev.Rule != "todo.todo" {
		t.Fatalf("line 2 carries a directive targeting todo.todo, got prev = %+v", prev)
	}
	same, prev = IgnoreDirectivesAt(data, 1)
	if same != nil || prev != nil {
		t.Fatalf("line 1 has neither a directive nor a previous line, got (%+v, %+v)", same, prev)
	}
}

func TestIgnoreContextLines(t *testing.T) {
	cases := []struct {
		name     string
		data     string
		line     int
		wantText string
		wantPrev string
	}{
		{"first line has no previous", "one\ntwo\n", 1, "one", ""},
		{"middle line", "one\ntwo\nthree\n", 2, "two", "one"},
		{"last line with terminator", "one\ntwo\n", 2, "two", "one"},
		{"line beyond end of file", "one\n", 2, "", ""},
		{"zero line", "one\n", 0, "", ""},
		{"negative line", "one\n", -3, "", ""},
		{"crlf terminators are stripped", "a\r\nb\r\n", 2, "b", "a"},
		{"crlf on the first line", "a\r\nb", 1, "a", ""},
		{"no trailing newline", "x\ny", 2, "y", "x"},
		{"empty file", "", 1, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, prev := IgnoreContextLines([]byte(tc.data), tc.line)
			if text != tc.wantText || prev != tc.wantPrev {
				t.Fatalf("IgnoreContextLines(%q, %d) = (%q, %q), want (%q, %q)", tc.data, tc.line, text, prev, tc.wantText, tc.wantPrev)
			}
		})
	}
}

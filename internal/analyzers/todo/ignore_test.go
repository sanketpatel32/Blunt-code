package todo

// End-to-end coverage for inline bluntcode:ignore directives: real marker
// detections in fixture files, scanned through Plan -> Run -> Normalize.
// The pure directive check is unit-tested in internal/analyzers; these
// tests prove the directives Run parses travel through the JSON envelope
// and that Normalize drops exactly the suppressed findings. Fixture files
// are written with explicit \n line endings so expectations are
// byte-stable on Windows.

import (
	"context"
	"testing"

	"bluntcode/internal/analyzers"
)

func TestRunNormalizeInlineIgnoreSuppressesFindings(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		content string
	}{
		{
			name:    "same-line bare directive",
			file:    "app.py",
			content: "# TODO: example kept on purpose bluntcode:ignore\n",
		},
		{
			name:    "same-line rule-targeted directive",
			file:    "ui.tsx",
			content: "// FIXME: placeholder // bluntcode:ignore todo.fixme\n",
		},
		{
			name:    "previous-line rule-targeted directive in an html comment",
			file:    "notes.md",
			content: "<!-- bluntcode:ignore todo.todo reason: documented sample -->\nTODO: marker in docs\n",
		},
		{
			name:    "previous-line bare directive",
			file:    "main.go",
			content: "// bluntcode:ignore\n// TODO: wire up flags\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFile(t, dir, tc.file, tc.content)
			adapter := New()
			ctx := context.Background()
			plan, err := adapter.Plan(ctx, analyzers.ScanRequest{WorkspaceRoot: dir, Files: []string{path}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := adapter.Run(ctx, plan, nil)
			if err != nil {
				t.Fatal(err)
			}
			findings, _, err := adapter.Normalize(ctx, result)
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != 0 {
				t.Fatalf("got %d findings, want 0 (the directive must suppress): %+v", len(findings), findings)
			}
		})
	}
}

// TestInlineIgnoreWrongRuleKeepsSurvivingFinding pins the regression the
// feature must not cause: a directive naming a different rule leaves the
// finding reported with a byte-identical fingerprint and message — inline
// ignore handling must be invisible to surviving findings.
func TestInlineIgnoreWrongRuleKeepsSurvivingFinding(t *testing.T) {
	scan := func(content string) analyzers.Finding {
		dir := t.TempDir()
		// The same relative path in every root, so fingerprints are
		// comparable; the directive line shifts the finding's line number,
		// which fingerprints deliberately do not include.
		path := writeFile(t, dir, "app.py", content)
		adapter := New()
		ctx := context.Background()
		plan, err := adapter.Plan(ctx, analyzers.ScanRequest{WorkspaceRoot: dir, Files: []string{path}})
		if err != nil {
			t.Fatal(err)
		}
		result, err := adapter.Run(ctx, plan, nil)
		if err != nil {
			t.Fatal(err)
		}
		findings, _, err := adapter.Normalize(ctx, result)
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 1 {
			t.Fatalf("got %d findings, want exactly 1: %+v", len(findings), findings)
		}
		return findings[0]
	}
	surviving := scan("# bluntcode:ignore todo.hack\n# FIXME: handle empty input\n")
	baseline := scan("# FIXME: handle empty input\n")
	if surviving.RuleID != ruleFIXME {
		t.Fatalf("rule = %s, want %s", surviving.RuleID, ruleFIXME)
	}
	if surviving.Fingerprint != baseline.Fingerprint {
		t.Fatalf("inline ignore handling changed the surviving fingerprint: %q vs baseline %q", surviving.Fingerprint, baseline.Fingerprint)
	}
	if surviving.Message != baseline.Message {
		t.Fatalf("inline ignore handling changed the surviving message: %q vs baseline %q", surviving.Message, baseline.Message)
	}
}

func TestInlineIgnoreBlankLineBreaksPreviousLine(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "app.py", "# bluntcode:ignore\n\n# TODO: too far below\n")
	plan := analyzers.AnalyzerPlan{AnalyzerID: ID, Metadata: map[string]any{planKeyFiles: []string{path}, planKeyRoot: dir}}
	result, err := New().Run(context.Background(), plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	findings, _, err := New().Normalize(context.Background(), result)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1 (a blank line breaks the previous-line directive): %+v", len(findings), findings)
	}
	if findings[0].RuleID != ruleTODO || findings[0].StartLine != 3 {
		t.Fatalf("finding rule/line = %s/%d, want %s/3", findings[0].RuleID, findings[0].StartLine, ruleTODO)
	}
}

// TestInlineIgnoreSuppressesOnlyTargetedFinding proves the per-finding
// selectivity end to end: the TODO under a matching directive disappears
// while the FIXME below — whose own previous line carries no directive —
// is still reported with its message intact.
func TestInlineIgnoreSuppressesOnlyTargetedFinding(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "mixed.go",
		"// bluntcode:ignore todo.todo\n"+
			"// TODO: suppressed by the directive above\n"+
			"// FIXME: still reported\n")
	plan := analyzers.AnalyzerPlan{AnalyzerID: ID, Metadata: map[string]any{planKeyFiles: []string{path}, planKeyRoot: dir}}
	result, err := New().Run(context.Background(), plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	findings, _, err := New().Normalize(context.Background(), result)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want exactly 1 (the untargeted FIXME): %+v", len(findings), findings)
	}
	if findings[0].RuleID != ruleFIXME || findings[0].StartLine != 3 {
		t.Fatalf("finding rule/line = %s/%d, want %s/3", findings[0].RuleID, findings[0].StartLine, ruleFIXME)
	}
	if findings[0].Message != "FIXME: still reported" {
		t.Fatalf("surviving message = %q, want the untouched marker text", findings[0].Message)
	}
}

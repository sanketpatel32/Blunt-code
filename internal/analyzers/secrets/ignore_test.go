package secrets

// End-to-end coverage for inline bluntcode:ignore directives: real
// detections in fixture files, scanned through Plan -> Run -> Normalize.
// The pure directive check is unit-tested in internal/analyzers; these
// tests prove the directives Run parses travel through the JSON envelope
// and that Normalize drops exactly the suppressed findings. Fixture files
// are written with explicit \n line endings so expectations are
// byte-stable on Windows.

import (
	"context"
	"strings"
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
			file:    "aws.py",
			content: "aws_key = \"AKIA1234567890ABCDEF\" # bluntcode:ignore\n",
		},
		{
			name:    "same-line bare directive with a reason",
			file:    "cfg.ts",
			content: "token := \"dfe3a91b77214f0c\" // bluntcode:ignore reason: fixture value\n",
		},
		{
			name:    "previous-line rule-targeted directive in an html comment",
			file:    "sample.md",
			content: "<!-- bluntcode:ignore secrets.aws-access-key-id reason: documented example -->\nAKIA1234567890ABCDEF\n",
		},
		{
			name:    "previous-line bare directive",
			file:    "deploy.sh",
			content: "# bluntcode:ignore\nexport api_key = \"dfe3a91b77214f0c\"\n",
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
		path := writeFile(t, dir, "aws.py", content)
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
	surviving := scan("# bluntcode:ignore secrets.jwt\naws_key = \"AKIA1234567890ABCDEF\"\n")
	baseline := scan("aws_key = \"AKIA1234567890ABCDEF\"\n")
	if surviving.RuleID != ruleAWSAccessKey {
		t.Fatalf("rule = %s, want %s", surviving.RuleID, ruleAWSAccessKey)
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
	path := writeFile(t, dir, "cfg.py", "# bluntcode:ignore\n\naws_key = \"AKIA1234567890ABCDEF\"\n")
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
	if findings[0].RuleID != ruleAWSAccessKey || findings[0].StartLine != 3 {
		t.Fatalf("finding rule/line = %s/%d, want %s/3", findings[0].RuleID, findings[0].StartLine, ruleAWSAccessKey)
	}
}

// TestInlineIgnoreSuppressesOnlyTargetedFinding proves the per-finding
// selectivity end to end: the AWS key under a matching directive disappears
// while the GitHub token two lines below — whose own previous line carries
// no directive — is still reported.
func TestInlineIgnoreSuppressesOnlyTargetedFinding(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "two.py",
		"# bluntcode:ignore secrets.aws-access-key-id\n"+
			"aws_key = \"AKIA1234567890ABCDEF\"\n"+
			"github_key = \"ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789\"\n")
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
		t.Fatalf("got %d findings, want exactly 1 (the untargeted token): %+v", len(findings), findings)
	}
	if findings[0].RuleID != ruleGitHubToken || findings[0].StartLine != 3 {
		t.Fatalf("finding rule/line = %s/%d, want %s/3", findings[0].RuleID, findings[0].StartLine, ruleGitHubToken)
	}
}

// TestInlineIgnoreLeavesEnvelopeRedacted guards the envelope design: the
// source lines Run parses are never serialized, so the directive fields
// cannot smuggle the reported secret back into Stdout.
func TestInlineIgnoreLeavesEnvelopeRedacted(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "redact.py", "# bluntcode:ignore secrets.jwt\naws_key = \"AKIA1234567890ABCDEF\"\n")
	plan := analyzers.AnalyzerPlan{AnalyzerID: ID, Metadata: map[string]any{planKeyFiles: []string{path}, planKeyRoot: dir}}
	result, err := New().Run(context.Background(), plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stdout := string(result.Stdout); strings.Contains(stdout, "AKIA1234567890ABCDEF") {
		t.Fatalf("envelope leaks the full secret: %q", stdout)
	}
	findings, _, err := New().Normalize(context.Background(), result)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1 (wrong-rule directive is inert): %+v", len(findings), findings)
	}
}

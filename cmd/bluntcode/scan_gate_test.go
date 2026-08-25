package main

import (
	"bytes"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

func gateFinding(analyzer string, severity analyzers.Severity, category analyzers.Category, path string) analyzers.Finding {
	f := analyzers.Finding{AnalyzerID: analyzer, RuleID: "R1", RelativePath: path, Message: "m", Severity: severity, Category: category}
	f.SetFingerprint()
	return f
}

func TestScopeGateFindingsFiltersByAnalyzerAndCategory(t *testing.T) {
	findings := []analyzers.Finding{
		gateFinding("semgrep", analyzers.SeverityCritical, analyzers.CategorySecurity, "a.py"),
		gateFinding("ruff", analyzers.SeverityHigh, analyzers.CategoryCorrectness, "b.py"),
		gateFinding("secrets", analyzers.SeverityHigh, analyzers.CategorySecurity, "c.env"),
	}

	all := scopeGateFindings(findings, nil, nil)
	if len(all) != 3 {
		t.Fatalf("empty scoping must keep everything: %d", len(all))
	}
	securityOnly := scopeGateFindings(findings, map[string]bool{"semgrep": true}, map[string]bool{"security": true})
	if len(securityOnly) != 1 {
		t.Fatalf("analyzer+category scoping must intersect both allow-lists: %#v", securityOnly)
	}
	for _, f := range securityOnly {
		if f.AnalyzerID != "semgrep" || f.Category != analyzers.CategorySecurity {
			t.Fatalf("leaked finding outside gate scope: %#v", f)
		}
	}
	caseInsensitive := scopeGateFindings(findings, map[string]bool{"SEMGREP": true, "secrets": true}, nil)
	if len(caseInsensitive) != 2 {
		t.Fatalf("analyzer scoping must be case-insensitive: %d", len(caseInsensitive))
	}
	if got := scopeGateFindings(findings, map[string]bool{"nothing": true}, nil); len(got) != 0 {
		t.Fatalf("unknown analyzer must scope to zero: %d", len(got))
	}
}

func TestWriteTopFindingsOrdersBySeverityThenPath(t *testing.T) {
	var out bytes.Buffer
	findings := []analyzers.Finding{
		gateFinding("ruff", analyzers.SeverityLow, analyzers.CategoryStyle, "z.py"),
		gateFinding("semgrep", analyzers.SeverityCritical, analyzers.CategorySecurity, "m.py"),
		gateFinding("ruff", analyzers.SeverityMedium, analyzers.CategoryCorrectness, "a.py"),
	}
	writeTopFindings(&out, findings, 3)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 || lines[0] != "Top findings:" {
		t.Fatalf("unexpected output shape: %q", out.String())
	}
	if !strings.Contains(lines[1], "CRITICAL") || !strings.Contains(lines[1], "m.py") {
		t.Fatalf("critical must lead: %q", lines[1])
	}
	if !strings.Contains(lines[3], "z.py") {
		t.Fatalf("low must be last: %q", lines[3])
	}
	// A cap shorter than the finding list truncates.
	out.Reset()
	writeTopFindings(&out, findings, 1)
	if strings.Count(out.String(), "\n") != 2 || !strings.Contains(out.String(), "CRITICAL") {
		t.Fatalf("cap must truncate to the most serious only: %q", out.String())
	}
	// No findings means no section at all.
	out.Reset()
	writeTopFindings(&out, nil, 3)
	if out.Len() != 0 {
		t.Fatalf("empty findings must print nothing: %q", out.String())
	}
}

func TestParseScanFlagsWatchTuning(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseScanFlags([]string{"--watch", "--watch-poll", "500ms", "--watch-quiet", "2s", `C:\proj`}, &errOut)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.watchPoll != 500*1_000_000 || cfg.watchQuiet != 2_000_000_000 {
		t.Fatalf("watch tuning not parsed: poll=%v quiet=%v", cfg.watchPoll, cfg.watchQuiet)
	}
	if _, err := parseScanFlags([]string{"--watch-poll", "10ms", `C:\proj`}, &errOut); err == nil {
		t.Fatalf("too-small poll must be rejected")
	}
	if _, err := parseScanFlags([]string{"--watch-quiet", "5m", `C:\proj`}, &errOut); err == nil {
		t.Fatalf("oversized quiet window must be rejected")
	}
	if _, err := parseScanFlags([]string{"--watch", "--watch-poll", "4s", "--watch-quiet", "100ms", `C:\proj`}, &errOut); err == nil {
		t.Fatalf("quiet window below a quarter of the poll must be rejected")
	}
}

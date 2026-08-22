package ruff

import (
	"bluntcode/internal/analyzers"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNormalizeFixture(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "fixtures", "ruff", "diagnostics.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := New("ruff.exe", "test").Normalize(context.Background(), analyzers.AnalyzerResult{Stdout: b, ExitCode: 1, Plan: analyzers.AnalyzerPlan{Commands: []analyzers.ProcessSpec{{Dir: "."}}}})
	if err != nil || len(got) != 1 || got[0].RuleID != "F401" || got[0].Category != analyzers.CategoryCorrectness {
		t.Fatalf("got %#v, err %v", got, err)
	}
}

func TestPlanAddsExtendedSelectOnlyForDeepProfile(t *testing.T) {
	adapter := New("ruff.exe", "test")
	base := analyzers.ScanRequest{WorkspaceRoot: `C:\ws`, Files: []string{`C:\ws\main.py`}, Languages: []analyzers.Language{analyzers.LanguagePython}}

	for _, profile := range []string{"", analyzers.ProfileStandard, analyzers.ProfileQuick} {
		req := base
		req.Profile = profile
		plan, err := adapter.Plan(context.Background(), req)
		if err != nil {
			t.Fatalf("profile %q: %v", profile, err)
		}
		want := []string{"check", "--output-format", "json", "--no-fix", `C:\ws\main.py`}
		if !reflect.DeepEqual(plan.Commands[0].Args, want) {
			t.Fatalf("profile %q args = %#v, want %#v (non-deep must stay identical to the default invocation)", profile, plan.Commands[0].Args, want)
		}
	}

	req := base
	req.Profile = analyzers.ProfileDeep
	plan, err := adapter.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"check", "--output-format", "json", "--no-fix", "--select=" + deepSelect, `C:\ws\main.py`}
	if !reflect.DeepEqual(plan.Commands[0].Args, want) {
		t.Fatalf("deep args = %#v, want %#v", plan.Commands[0].Args, want)
	}
}

func TestClassificationCoversDeepRulePrefixes(t *testing.T) {
	cases := []struct {
		code     string
		severity analyzers.Severity
		category analyzers.Category
	}{
		// Reachable in every profile; must stay unchanged.
		{"F401", analyzers.SeverityMedium, analyzers.CategoryCorrectness},
		{"E501", analyzers.SeverityMedium, analyzers.CategoryStyle},
		{"S101", analyzers.SeverityHigh, analyzers.CategorySecurity},
		// Reachable only under deep's --select set.
		{"SIM101", analyzers.SeverityLow, analyzers.CategoryMaintainability}, // SIM must not hit the "S" bandit branch
		{"B008", analyzers.SeverityMedium, analyzers.CategoryCorrectness},
		{"W291", analyzers.SeverityLow, analyzers.CategoryStyle},
		{"C401", analyzers.SeverityLow, analyzers.CategoryMaintainability},
		{"RET504", analyzers.SeverityLow, analyzers.CategoryMaintainability},
		{"ARG001", analyzers.SeverityLow, analyzers.CategoryMaintainability},
		{"PLR0911", analyzers.SeverityLow, analyzers.CategoryMaintainability},
	}
	for _, c := range cases {
		if got := severity(c.code); got != c.severity {
			t.Fatalf("severity(%s) = %s, want %s", c.code, got, c.severity)
		}
		if got := category(c.code); got != c.category {
			t.Fatalf("category(%s) = %s, want %s", c.code, got, c.category)
		}
	}
}

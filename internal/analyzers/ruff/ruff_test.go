package ruff

import (
	"bluntcode/internal/analyzers"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestPlanFiltersToPythonFilesOnly(t *testing.T) {
	adapter := New("ruff.exe", "test")
	req := analyzers.ScanRequest{
		WorkspaceRoot: `C:\ws`,
		Languages:     []analyzers.Language{analyzers.LanguagePython, analyzers.LanguageJavaScript, analyzers.LanguageTypeScript},
		Files: []string{
			`C:\ws\main.py`,
			`C:\ws\app.tsx`,
			`C:\ws\static\assets\index-ABC123.js`,
			`C:\ws\typed.pyi`,
			`C:\ws\bundle.min.js`,
		},
	}
	plan, err := adapter.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"check", "--output-format", "json", "--no-fix", `C:\ws\main.py`, `C:\ws\typed.pyi`}
	if !reflect.DeepEqual(plan.Commands[0].Args, want) {
		t.Fatalf("args = %#v, want %#v (non-Python files must never reach ruff)", plan.Commands[0].Args, want)
	}
}

func TestPlanRejectsSelectionWithoutPythonFiles(t *testing.T) {
	adapter := New("ruff.exe", "test")
	req := analyzers.ScanRequest{
		WorkspaceRoot: `C:\ws`,
		// A stale caller may claim Python is present while listing only web
		// files; the file list is authoritative, so Plan must refuse rather
		// than invoke a bare `ruff check` that would scan its working dir.
		Languages: []analyzers.Language{analyzers.LanguagePython},
		Files:     []string{`C:\ws\app.ts`, `C:\ws\app.js`},
	}
	if _, err := adapter.Plan(context.Background(), req); err == nil {
		t.Fatal("expected error when no Python files are in the selection")
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

func TestPlanBatchesLargeWorkspaceFileLists(t *testing.T) {
	// 692 real-world Python paths overflowed Windows' 32,767-character
	// process command line ("filename or extension is too long"), so Plan
	// must split file arguments exactly like the biome and semgrep adapters.
	files := make([]string, 692)
	for i := range files {
		files[i] = `C:\Users\sanpa\OneDrive\Desktop\Claire\claire-backend\src\some\deeply\nested\package\module_file_with_a_long_name_` + strings.Repeat("x", 4) + fmt.Sprint(i) + ".py"
	}
	adapter := New("ruff.exe", "test")
	plan, err := adapter.Plan(context.Background(), analyzers.ScanRequest{WorkspaceRoot: `C:\ws`, Files: files, Languages: []analyzers.Language{analyzers.LanguagePython}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commands) < 2 {
		t.Fatalf("expected multiple batches for a large file list, got %d command(s)", len(plan.Commands))
	}
	seen := map[string]bool{}
	total := 0
	for _, command := range plan.Commands {
		for _, arg := range command.Args[4:] { // skip check --output-format json --no-fix
			if !strings.HasSuffix(arg, ".py") {
				t.Fatalf("non-Python argument reached ruff: %s", arg)
			}
			if seen[arg] {
				t.Fatalf("file assigned to more than one batch: %s", arg)
			}
			seen[arg] = true
			total++
		}
	}
	if total != len(files) {
		t.Fatalf("batched %d files, want all %d", total, len(files))
	}
}

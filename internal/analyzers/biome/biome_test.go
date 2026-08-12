package biome

import (
	"bluntcode/internal/analyzers"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeFixture(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "fixtures", "biome", "diagnostics.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := New("biome.exe", "test").Normalize(context.Background(), analyzers.AnalyzerResult{Stdout: b, ExitCode: 1, Plan: analyzers.AnalyzerPlan{Commands: []analyzers.ProcessSpec{{Dir: "."}}}})
	if err != nil || len(got) != 1 || got[0].RuleID == "" {
		t.Fatalf("got %#v, err %v", got, err)
	}
}

func TestPlanBatchesLargeWorkspaceFileLists(t *testing.T) {
	files := make([]string, 428)
	for i := range files {
		files[i] = `C:\workspace\src\` + strings.Repeat("long-name-", 10) + "file.ts"
	}
	plan, err := New("biome.exe", "test").Plan(context.Background(), analyzers.ScanRequest{WorkspaceRoot: `C:\workspace`, Files: files, Languages: []analyzers.Language{analyzers.LanguageTypeScript}})
	if err != nil || len(plan.Commands) < 2 {
		t.Fatalf("plan=%#v err=%v", plan, err)
	}
	for _, command := range plan.Commands {
		length := 0
		for _, arg := range command.Args {
			length += len(arg) + 3
		}
		if length > analyzers.MaxCommandArgumentCharacters {
			t.Fatalf("command is %d characters", length)
		}
	}
}

func TestNormalizeUsesRuleFallbackForEmptyMessage(t *testing.T) {
	got, _, err := New("biome.exe", "test").Normalize(context.Background(), analyzers.AnalyzerResult{Stdout: []byte(`{"diagnostics":[{"category":"lint/security/noGlobalEval","severity":"error","location":{"path":"a.ts"}}]}`), ExitCode: 1})
	if err != nil || len(got) != 1 || got[0].Message != "Biome reported noGlobalEval." {
		t.Fatalf("got %#v, err %v", got, err)
	}
}

func TestNormalizeConvertsBiomeSpanToSourcePosition(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.ts"), []byte("one\nvalue\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"diagnostics":[{"category":"lint/suspicious/noExplicitAny","severity":"warning","description":"Avoid any.","location":{"path":"src/main.ts","span":{"start":4,"end":9}}}]}`)
	got, _, err := New("biome.exe", "test").Normalize(context.Background(), analyzers.AnalyzerResult{Stdout: raw, ExitCode: 1, Plan: analyzers.AnalyzerPlan{Commands: []analyzers.ProcessSpec{{Dir: root}}}})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %#v, err %v", got, err)
	}
	if got[0].StartLine != 2 || got[0].StartColumn != 1 || got[0].EndLine != 2 || got[0].EndColumn != 6 {
		t.Fatalf("unexpected position: %#v", got[0])
	}
}

func TestNormalizeUsesBiomeLineLocation(t *testing.T) {
	raw := []byte(`{"diagnostics":[{"category":"lint/complexity/noExtraBooleanCast","severity":"info","message":"Avoid redundant Boolean call","location":{"path":"src/main.ts","start":{"line":14,"column":7},"end":{"line":14,"column":29}}}]}`)
	got, _, err := New("biome.exe", "test").Normalize(context.Background(), analyzers.AnalyzerResult{Stdout: raw, ExitCode: 1})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %#v, err %v", got, err)
	}
	if got[0].Message != "Avoid redundant Boolean call" || got[0].StartLine != 14 || got[0].StartColumn != 7 || got[0].EndLine != 14 || got[0].EndColumn != 29 {
		t.Fatalf("unexpected normalized finding: %#v", got[0])
	}
}

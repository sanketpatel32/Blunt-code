package ruff

import (
	"bluntcode/internal/analyzers"
	"context"
	"os"
	"path/filepath"
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

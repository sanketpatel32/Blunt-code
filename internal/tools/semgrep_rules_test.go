package tools

import (
	"os"
	"path/filepath"
	"testing"

	analyzerssemgrep "bluntcode/internal/analyzers/semgrep"
)

// The runtime rulepack is maintained inside the semgrep adapter package; this
// test guards the wiring so scans always extract exactly that pack.
func TestExtractSemgrepRulesWritesBundledRulepack(t *testing.T) {
	dir := t.TempDir()
	if err := ExtractSemgrepRules(dir); err != nil {
		t.Fatalf("extract rules: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, analyzerssemgrep.RulesFileName))
	if err != nil {
		t.Fatalf("read extracted rules: %v", err)
	}
	if want := analyzerssemgrep.RulesYAML(); string(got) != want {
		t.Fatalf("extracted rulepack does not match semgrep.RulesYAML(): got %d bytes, want %d bytes", len(got), len(want))
	}
	version, err := os.ReadFile(filepath.Join(dir, "RULES_VERSION"))
	if err != nil {
		t.Fatalf("read rules version: %v", err)
	}
	if want := SemgrepRulesVersion + "\n"; string(version) != want {
		t.Fatalf("RULES_VERSION = %q, want %q", string(version), want)
	}
}

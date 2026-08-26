package reports

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

func TestJSONLOneObjectPerLineInExportOrder(t *testing.T) {
	findings := []analyzers.Finding{
		{AnalyzerID: "ruff", RuleID: "F401", Severity: analyzers.SeverityHigh, Message: "second", RelativePath: "b.py", StartLine: 2},
		{AnalyzerID: "ruff", RuleID: "F401", Severity: analyzers.SeverityHigh, Message: "first", RelativePath: "a.py", StartLine: 1},
	}
	for i := range findings {
		findings[i].SetFingerprint()
	}
	got := JSONL(Build(Input{Findings: findings}))
	lines := strings.Split(strings.TrimRight(string(got), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two lines, got %d", len(lines))
	}
	if !bytes.HasSuffix(got, []byte("\n")) {
		t.Fatalf("last line must be LF-terminated")
	}
	var first analyzers.Finding
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("line 0 is not valid JSON: %v", err)
	}
	if first.RelativePath != "a.py" || first.Message != "first" {
		t.Fatalf("export order must put a.py first: %#v", first)
	}
}

func TestJSONLNoFindingsRendersEmptyOutput(t *testing.T) {
	if got := JSONL(Build(Input{})); len(got) != 0 {
		t.Fatalf("empty model must render zero bytes, got %q", got)
	}
}

func TestJSONLRowsCarryDerivedStatus(t *testing.T) {
	old := analyzers.Finding{AnalyzerID: "ruff", RuleID: "P", Severity: analyzers.SeverityLow, Message: "m", RelativePath: "p.py"}
	old.SetFingerprint()
	cur := old // same fingerprint -> persistent in the current scan too
	in := Input{Findings: []analyzers.Finding{cur}, Comparison: Comparison{Persistent: []analyzers.Finding{old}}}
	lines := strings.Split(strings.TrimRight(string(JSONL(Build(in))), "\n"), "\n")
	var f analyzers.Finding
	if err := json.Unmarshal([]byte(lines[0]), &f); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if f.Status != "persistent" {
		t.Fatalf("comparison status must ride on the row, got %q", f.Status)
	}
}

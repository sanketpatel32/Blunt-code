package reports

import (
	"bytes"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

func findingForCSV(severity analyzers.Severity, rule, message string) analyzers.Finding {
	f := analyzers.Finding{AnalyzerID: "ruff", RuleID: rule, RelativePath: "src/x.py", Message: message, Severity: severity, Category: analyzers.CategoryCorrectness}
	f.SetFingerprint()
	return f
}

func TestCSVEmbedsBOMHeaderAndEscapesFormulas(t *testing.T) {
	hostile := findingForCSV(analyzers.SeverityHigh, "E1", "=cmd|' /C calc'!A0")
	model := Model{}
	model.Findings = []analyzers.Finding{hostile}
	got := CSV(model)
	if !bytes.HasPrefix(got, []byte(csvBOM)) {
		t.Fatalf("CSV must start with the UTF-8 BOM")
	}
	text := string(got)
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if lines[0] != csvBOM+strings.Join(CSVHeader, ",") {
		t.Fatalf("header mismatch: %q", lines[0])
	}
	if len(lines) != 2 {
		t.Fatalf("expected header plus one data row, got %d lines", len(lines))
	}
	if !strings.Contains(lines[1], "'=cmd") {
		t.Fatalf("formula prefix must be neutralized with a leading apostrophe: %q", lines[1])
	}
}

func TestCSVEmptyModelProducesHeaderOnly(t *testing.T) {
	got := CSV(Model{})
	lines := strings.Split(strings.TrimRight(string(got), "\n"), "\n")
	if len(lines) != 1 || !bytes.HasPrefix(got, []byte(csvBOM)) {
		t.Fatalf("empty model must render exactly the BOM+header line")
	}
}

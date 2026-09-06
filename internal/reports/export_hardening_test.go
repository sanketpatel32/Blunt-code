package reports

// IMP-12 acceptance: one malicious corpus is pushed through every exporter,
// and each exporter's own contract is asserted — HTML contextual escaping
// (script terminators, unsafe URL schemes, no network-loaded resources),
// CSV formula neutralization, GitHub workflow-command neutralization,
// JSON/JSONL/SARIF parse-back validity, control-byte scrubbing, cross-format
// total reconciliation with the normalized model, and atomic artifact writes.

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

// hostileModel is the shared malicious fixture: HTML/script terminators,
// unsafe documentation schemes, CSV formula payloads, workflow-command
// forgery (line feeds, colons, percent-encoding), raw C0/DEL controls, and
// traversal-style paths.
func hostileModel() Model {
	findings := []analyzers.Finding{
		{
			AnalyzerID: "evil", RuleID: "</script><img src=x onerror=alert(1)>", Severity: analyzers.SeverityHigh,
			Message:          "msg </script>\x00\x1b[31m\x7f with controls",
			Title:            `=HYPERLINK("http://evil","click")`,
			RelativePath:     `..\..\windows\system32\config`,
			DocumentationURL: "javascript:alert(document.domain)",
			StartLine:        3, StartColumn: 1,
		},
		{
			AnalyzerID: "evil", RuleID: "::error ::forged workflow command", Severity: analyzers.SeverityLow,
			Message:          "line1\nline2 %0A ::debug forged\r\n::error file=x",
			RelativePath:     "+SUM(1,2)@cmd|' /C calc'!A0",
			DocumentationURL: "data:text/html,<script>x</script>",
			StartLine:        4,
		},
	}
	return Build(Input{
		WorkspaceName: "Hostile <b>name</b>", WorkspacePath: `C:\hostile`, ScanID: "scan-1", Profile: "standard",
		Findings: findings, Runs: []Run{{AnalyzerID: "evil", Version: "1", State: "succeeded", FindingCount: 2}},
		Comparison: Comparison{New: findings},
	})
}

func TestHTMLExportNeutralizesScriptTerminatorsAndUnsafeURLs(t *testing.T) {
	document := string(HTML(hostileModel()))
	if strings.Contains(document, "</script><img") {
		t.Fatal("the injected script-terminator sequence survived unescaped into the HTML document")
	}
	if !strings.Contains(document, "&lt;/script&gt;") {
		t.Fatal("the hostile rule id should appear only in HTML-escaped form")
	}
	if strings.Contains(document, "javascript:alert") || strings.Contains(document, "data:text/html") {
		t.Fatal("unsafe documentation URL scheme survived into the HTML document")
	}
	if !strings.Contains(document, "#ZgotmplZ") {
		t.Fatal("html/template should have rewritten the unsafe documentation URL")
	}
	if strings.Contains(document, "\x00") || strings.Contains(document, "\x1b") || strings.Contains(document, "\x7f") {
		t.Fatal("raw control bytes survived into the HTML document")
	}
	// Standalone rendering with networking disabled: the document loads no
	// external stylesheets, scripts, images, or fonts.
	for _, ref := range []string{"src=\"http", "src='http", "href=\"http", "href='http", "<link", "@import", "url(http"} {
		if strings.Contains(document, ref) {
			t.Fatalf("document references an external resource (%s) and cannot render offline", ref)
		}
	}
}

func TestCSVExportNeutralizesFormulasAndControls(t *testing.T) {
	raw := CSV(hostileModel())
	if !bytes.HasPrefix(raw, []byte(csvBOM)) {
		t.Fatal("CSV export lost its UTF-8 BOM")
	}
	if bytes.ContainsAny(raw, "\x00\x1b\x7f") {
		t.Fatal("raw control bytes survived into the CSV export")
	}
	records, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, []byte(csvBOM)))).ReadAll()
	if err != nil {
		t.Fatalf("CSV export does not parse: %v", err)
	}
	if len(records) != 1+len(hostileModel().Findings) {
		t.Fatalf("CSV records = %d, want header + 2 findings", len(records))
	}
	var formula, injection bool
	for _, rec := range records[1:] {
		// title is column 4, file is column 6 (CSVHeader order).
		if strings.HasPrefix(rec[4], "'=HYPERLINK") {
			formula = true
		}
		if strings.HasPrefix(rec[6], "'+SUM(1,2)") {
			injection = true
		}
		if strings.HasPrefix(rec[4], "=HYPERLINK") || strings.HasPrefix(rec[6], "+SUM(1,2)") {
			t.Fatalf("formula-like cell reached the CSV unneutralized: %q", rec)
		}
	}
	if !formula || !injection {
		t.Fatalf("expected both formula payloads in the export (formula=%v injection=%v)", formula, injection)
	}
}

func TestGitHubExportNeutralizesWorkflowCommands(t *testing.T) {
	document := string(GitHubAnnotations(hostileModel()))
	if bytes.ContainsAny([]byte(document), "\x00\x1b\x7f") {
		t.Fatal("raw control bytes survived into annotations")
	}
	var errorLines, noticeLines, warningLines int
	for _, line := range strings.Split(document, "\n") {
		switch {
		case strings.HasPrefix(line, "::error"):
			errorLines++
		case strings.HasPrefix(line, "::notice"):
			noticeLines++
		case strings.HasPrefix(line, "::warning"):
			warningLines++
		}
	}
	// high → error, low → notice for this corpus.
	if errorLines != 1 || noticeLines != 1 || warningLines != 0 {
		t.Fatalf("annotation levels wrong: %d error, %d notice, %d warning:\n%s", errorLines, noticeLines, warningLines, document)
	}
	// A literal % is encoded first (%25), so the smuggled "%0A" cannot be
	// decoded back into a line feed by the runner; the real newlines in the
	// message become %0A but stay inside the single annotation line.
	if !strings.Contains(document, "%250A") || !strings.Contains(document, "line1%0Aline2") {
		t.Fatalf("newline/percent forging not neutralized:\n%s", document)
	}
	// Colons inside the property segment are %3A-encoded, so the hostile
	// rule id "::error ::forged..." cannot forge command structure there.
	if !strings.Contains(document, "%3A%3Aerror") {
		t.Fatalf("property-segment colons not encoded:\n%s", document)
	}
	lines := strings.Split(strings.TrimRight(document, "\n"), "\n")
	if !strings.Contains(lines[len(lines)-1], "2 total") {
		t.Fatalf("summary line does not reconcile with the model: %q", lines[len(lines)-1])
	}
}

// Every machine-readable export must parse back cleanly and reconcile its
// totals with the normalized model.
func TestJSONExportsParseBackAndReconcile(t *testing.T) {
	m := hostileModel()
	var doc struct {
		Findings  []map[string]any `json:"findings"`
		Severity  map[string]int   `json:"severity"`
		Analyzers []struct {
			ID       string `json:"id"`
			State    string `json:"state"`
			Findings int    `json:"findings"`
		} `json:"analyzers"`
		Comparison struct {
			New        int `json:"new"`
			Fixed      int `json:"fixed"`
			Persistent int `json:"persistent"`
		} `json:"comparison"`
	}
	if err := json.Unmarshal(JSON(m), &doc); err != nil {
		t.Fatalf("JSON export does not parse: %v", err)
	}
	if len(doc.Findings) != len(m.Findings) {
		t.Fatalf("JSON findings = %d, want %d", len(doc.Findings), len(m.Findings))
	}
	if total, ok := doc.Severity["total"]; !ok || total != len(m.Findings) {
		t.Fatalf("severity total = %v (present=%v), want %d (full map: %v)", doc.Severity, ok, len(m.Findings), doc.Severity)
	}
	sum := doc.Severity["critical"] + doc.Severity["high"] + doc.Severity["medium"] + doc.Severity["low"] + doc.Severity["info"]
	if sum != len(m.Findings) {
		t.Fatalf("severity counts (%v) do not sum to %d findings", doc.Severity, len(m.Findings))
	}
	if doc.Severity["high"] != 1 || doc.Severity["low"] != 1 {
		t.Fatalf("per-severity counts wrong: %v", doc.Severity)
	}
	if doc.Comparison.New != len(m.Comparison.New) || doc.Comparison.Fixed != 0 || doc.Comparison.Persistent != 0 {
		t.Fatalf("comparison totals do not reconcile: %+v", doc.Comparison)
	}
	if len(doc.Analyzers) != 1 || doc.Analyzers[0].Findings != len(m.Findings) {
		t.Fatalf("analyzer run counts do not reconcile: %+v", doc.Analyzers)
	}
	if bytes.ContainsAny(JSON(m), "\x00\x1b\x7f") {
		t.Fatal("raw control bytes survived into the JSON export")
	}

	var rows int
	for _, line := range bytes.Split(bytes.TrimSpace(JSONL(m)), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal(line, &row); err != nil {
			t.Fatalf("JSONL row does not parse: %v (%s)", err, line)
		}
		if _, ok := row["rule_id"]; !ok {
			t.Fatalf("JSONL row lacks rule_id: %s", line)
		}
		rows++
	}
	if rows != len(m.Findings) {
		t.Fatalf("JSONL rows = %d, want %d", rows, len(m.Findings))
	}
}

func TestSARIFExportParsesBackAndAllowsOnlyHTTPLinks(t *testing.T) {
	var log struct {
		Runs []struct {
			Tool struct {
				Driver struct {
					Rules []struct {
						ID      string `json:"id"`
						HelpURI string `json:"helpUri"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []map[string]any `json:"results"`
		} `json:"runs"`
	}
	sarif := SARIFBytes(hostileModel())
	if err := json.Unmarshal(sarif, &log); err != nil {
		t.Fatalf("SARIF export does not parse: %v", err)
	}
	if len(log.Runs) != 1 || len(log.Runs[0].Results) != 2 {
		t.Fatalf("SARIF shape wrong: %d runs, %d results", len(log.Runs), len(log.Runs[0].Results))
	}
	for _, rule := range log.Runs[0].Tool.Driver.Rules {
		if rule.HelpURI != "" && !strings.HasPrefix(rule.HelpURI, "https://") && !strings.HasPrefix(rule.HelpURI, "http://") {
			t.Fatalf("rule %q carries a non-http help URI %q", rule.ID, rule.HelpURI)
		}
	}
	if bytes.ContainsAny(sarif, "\x00\x1b\x7f") {
		t.Fatal("raw control bytes survived into the SARIF export")
	}
}

// A failed atomic write leaves the previous artifact untouched with no
// temporary litter behind; a successful write replaces the file in full.
func TestWriteFileAtomicNeverLeavesTruncatedArtifacts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.md")
	if err := WriteBytesAtomic(path, []byte("complete previous report"), 0o600); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("disk went away mid-write")
	if err := WriteFileAtomic(path, 0o600, func(w io.Writer) error {
		if _, err := w.Write([]byte("half-writte")); err != nil {
			return err
		}
		return boom
	}); !errors.Is(err, boom) {
		t.Fatalf("write error = %v, want the callback error", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "complete previous report" {
		t.Fatalf("failed write damaged the previous artifact: %v %q", err, got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary litter left behind: %d entries", len(entries))
	}
	if err := WriteBytesAtomic(path, []byte("fresh full report"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "fresh full report" {
		t.Fatal("successful write did not replace the artifact")
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		t.Fatal("artifact is not a regular file after atomic write")
	}
}

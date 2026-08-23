package reports

import (
	"bluntcode/internal/analyzers"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// jsonTestInput is a compact but complete report input: every optional field
// populated, both a successful and a failed analyzer run, a comparison, and
// findings whose (path, line, rule) sort keys are all distinct so shuffled
// input must render byte-identical output.
func jsonTestInput() Input {
	started := time.Date(2026, 8, 24, 9, 30, 0, 0, time.UTC)
	finished := started.Add(42 * time.Second)
	findings := []analyzers.Finding{
		{AnalyzerID: "biome", RuleID: "lint/b", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryStyle,
			Title: "Beta", Message: "beta issue", RelativePath: "src/z.ts",
			StartLine: 9, StartColumn: 1, EndLine: 9, EndColumn: 8, Status: "new", Fingerprint: "fp-b",
			Remediation: "Rename it", DocumentationURL: "https://biomejs.dev/lint/b"},
		{AnalyzerID: "ruff", RuleID: "F401", Severity: analyzers.SeverityHigh, Category: analyzers.CategoryCorrectness,
			Title: "Unused import", Message: "unused import", RelativePath: `src\a.py`,
			StartLine: 3, Status: "persistent", Fingerprint: "fp-a"},
		{AnalyzerID: "semgrep", RuleID: "sec-1", Severity: analyzers.SeverityCritical, Category: analyzers.CategorySecurity,
			Message: "hardcoded secret", RelativePath: "src/m.py", StartLine: 3, Status: "new", Fingerprint: "fp-c"},
	}
	return Input{
		WorkspaceName: "Demo", WorkspacePath: `C:\Projects\demo`, ScanID: "scan-1", Profile: "standard",
		State: "completed_with_warnings", BluntCodeVersion: "1.2.3",
		StartedAt: started, FinishedAt: finished,
		Files:        []string{"src/a.py", "src/m.py", "src/z.ts"},
		SkippedFiles: []string{"", "", ""}, // counts only, like the loaders' placeholders
		Findings:     findings,
		Metrics: []analyzers.Metric{
			{AnalyzerID: "sonarqube", Key: "ncloc", Label: "Lines of Code", Value: 42},
			{AnalyzerID: "ruff", Key: "violations", Value: 2},
		},
		Runs: []Run{
			{AnalyzerID: "ruff", Version: "0.16.0", State: "succeeded", FindingCount: 2, Duration: 1500 * time.Millisecond},
			{AnalyzerID: "semgrep", State: "failed", ErrorSummary: " not configured ", FindingCount: 1, Duration: 2 * time.Second},
		},
		Comparison: Comparison{New: []analyzers.Finding{findings[2]}, Persistent: []analyzers.Finding{findings[1]}},
	}
}

// jsonDocument is the decode target mirroring the export shape; the test
// asserts every field the document promises consumers.
type jsonDocument struct {
	Schema        string `json:"schema"`
	SchemaVersion int    `json:"schemaVersion"`
	Workspace     struct {
		Name string `json:"name"`
		Path string `json:"path"`
	} `json:"workspace"`
	Scan struct {
		ID               string `json:"id"`
		Profile          string `json:"profile"`
		State            string `json:"state"`
		StartedAt        string `json:"started_at"`
		FinishedAt       string `json:"finished_at"`
		BluntCodeVersion string `json:"bluntcode_version"`
	} `json:"scan"`
	Files struct {
		Candidate int `json:"candidate"`
		Selected  int `json:"selected"`
		Skipped   int `json:"skipped"`
	} `json:"files"`
	Severity struct {
		Critical int `json:"critical"`
		High     int `json:"high"`
		Medium   int `json:"medium"`
		Low      int `json:"low"`
		Info     int `json:"info"`
		Total    int `json:"total"`
	} `json:"severity"`
	Analyzers []struct {
		ID         string `json:"id"`
		Version    string `json:"version"`
		State      string `json:"state"`
		Findings   int    `json:"findings"`
		DurationMS int64  `json:"duration_ms"`
		Error      string `json:"error"`
	} `json:"analyzers"`
	Findings []struct {
		Severity         string `json:"severity"`
		Category         string `json:"category"`
		AnalyzerID       string `json:"analyzer_id"`
		RuleID           string `json:"rule_id"`
		Title            string `json:"title"`
		Message          string `json:"message"`
		File             string `json:"file"`
		StartLine        int    `json:"start_line"`
		StartColumn      int    `json:"start_column"`
		EndLine          int    `json:"end_line"`
		EndColumn        int    `json:"end_column"`
		Status           string `json:"status"`
		RawSeverity      string `json:"raw_severity"`
		Remediation      string `json:"remediation"`
		DocumentationURL string `json:"documentation_url"`
		Fingerprint      string `json:"fingerprint"`
	} `json:"findings"`
	Metrics []struct {
		AnalyzerID string  `json:"analyzer_id"`
		Key        string  `json:"key"`
		Label      string  `json:"label"`
		Value      float64 `json:"value"`
	} `json:"metrics"`
	Comparison struct {
		New        int `json:"new"`
		Fixed      int `json:"fixed"`
		Persistent int `json:"persistent"`
	} `json:"comparison"`
	Warnings []string `json:"warnings"`
}

func TestJSONRendersVersionedDocumentWithEveryFindingField(t *testing.T) {
	var doc jsonDocument
	body := JSON(Build(jsonTestInput()))
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, body)
	}
	if doc.Schema != "bluntcode/scan-report" || doc.SchemaVersion != 1 {
		t.Fatalf("schema marker = %q v%d", doc.Schema, doc.SchemaVersion)
	}
	if doc.Workspace.Name != "Demo" || doc.Workspace.Path != `C:\Projects\demo` {
		t.Fatalf("workspace = %#v", doc.Workspace)
	}
	if doc.Scan.ID != "scan-1" || doc.Scan.Profile != "standard" || doc.Scan.State != "completed_with_warnings" {
		t.Fatalf("scan = %#v", doc.Scan)
	}
	if doc.Scan.StartedAt != "2026-08-24T09:30:00Z" || doc.Scan.FinishedAt != "2026-08-24T09:30:42Z" {
		t.Fatalf("scan timestamps = %q / %q", doc.Scan.StartedAt, doc.Scan.FinishedAt)
	}
	if doc.Scan.BluntCodeVersion != "1.2.3" {
		t.Fatalf("bluntcode_version = %q", doc.Scan.BluntCodeVersion)
	}
	if doc.Files.Candidate != 6 || doc.Files.Selected != 3 || doc.Files.Skipped != 3 {
		t.Fatalf("file counts = %#v", doc.Files)
	}
	if doc.Severity.Critical != 1 || doc.Severity.High != 1 || doc.Severity.Medium != 1 || doc.Severity.Low != 0 || doc.Severity.Info != 0 || doc.Severity.Total != 3 {
		t.Fatalf("severity counts = %#v", doc.Severity)
	}
	if len(doc.Analyzers) != 2 {
		t.Fatalf("analyzers = %#v", doc.Analyzers)
	}
	ruff, semgrep := doc.Analyzers[0], doc.Analyzers[1]
	if ruff.ID != "ruff" || ruff.Version != "0.16.0" || ruff.State != "succeeded" || ruff.Findings != 2 || ruff.DurationMS != 1500 || ruff.Error != "" {
		t.Fatalf("ruff run = %#v", ruff)
	}
	if semgrep.ID != "semgrep" || semgrep.State != "failed" || semgrep.Findings != 1 || semgrep.DurationMS != 2000 || semgrep.Error != "not configured" {
		t.Fatalf("semgrep run = %#v", semgrep)
	}
	if len(doc.Findings) != 3 {
		t.Fatalf("findings = %#v", doc.Findings)
	}
	first, second, third := doc.Findings[0], doc.Findings[1], doc.Findings[2]
	if first.File != "src/a.py" || first.RuleID != "F401" || first.AnalyzerID != "ruff" || first.Severity != "high" ||
		first.Category != "correctness" || first.Title != "Unused import" || first.Message != "unused import" ||
		first.StartLine != 3 || first.StartColumn != 0 || first.Status != "persistent" || first.Fingerprint != "fp-a" {
		t.Fatalf("first finding = %#v", first)
	}
	if second.File != "src/m.py" || second.RuleID != "sec-1" || second.Severity != "critical" || second.Category != "security" || second.Status != "new" || second.Fingerprint != "fp-c" {
		t.Fatalf("second finding = %#v", second)
	}
	if third.File != "src/z.ts" || third.RuleID != "lint/b" || third.Title != "Beta" || third.Remediation != "Rename it" ||
		third.DocumentationURL != "https://biomejs.dev/lint/b" || third.StartColumn != 1 || third.EndLine != 9 || third.EndColumn != 8 || third.Fingerprint != "fp-b" {
		t.Fatalf("third finding = %#v", third)
	}
	if len(doc.Metrics) != 2 || doc.Metrics[0].AnalyzerID != "ruff" || doc.Metrics[0].Key != "violations" ||
		doc.Metrics[1].AnalyzerID != "sonarqube" || doc.Metrics[1].Key != "ncloc" || doc.Metrics[1].Label != "Lines of Code" || doc.Metrics[1].Value != 42 {
		t.Fatalf("metrics = %#v", doc.Metrics)
	}
	if doc.Comparison.New != 1 || doc.Comparison.Fixed != 0 || doc.Comparison.Persistent != 1 {
		t.Fatalf("comparison = %#v", doc.Comparison)
	}
	if len(doc.Warnings) != 1 || !strings.Contains(doc.Warnings[0], "semgrep") {
		t.Fatalf("warnings = %#v", doc.Warnings)
	}
}

func TestJSONIsByteStableForIdenticalInput(t *testing.T) {
	first := JSON(Build(jsonTestInput()))
	second := JSON(Build(jsonTestInput()))
	if !bytes.Equal(first, second) {
		t.Fatalf("two renders of identical input differ:\n%s\n---\n%s", first, second)
	}
	if !strings.HasPrefix(string(first), "{\n  \"schema\": \"bluntcode/scan-report\"") {
		t.Fatalf("document must open with the schema marker at two-space indent:\n%s", first)
	}
	if !strings.Contains(string(first), "\n  \"schemaVersion\": 1,") {
		t.Fatalf("schemaVersion must be a two-space-indented top-level key:\n%s", first)
	}
	if !strings.HasSuffix(string(first), "}\n") {
		t.Fatalf("document must end with a single trailing LF:\n%q", string(first)[max(0, len(first)-32):])
	}
	if strings.Contains(string(first), "\r") {
		t.Fatalf("document must use LF line endings only")
	}
}

func TestJSONSortsFindingsRegardlessOfInputOrder(t *testing.T) {
	base := JSON(Build(jsonTestInput()))
	shuffledInput := jsonTestInput()
	shuffledInput.Findings = []analyzers.Finding{shuffledInput.Findings[2], shuffledInput.Findings[0], shuffledInput.Findings[1]}
	shuffled := JSON(Build(shuffledInput))
	if !bytes.Equal(base, shuffled) {
		t.Fatalf("shuffled findings must render byte-identical output:\n%s\n---\n%s", base, shuffled)
	}
	var doc jsonDocument
	if err := json.Unmarshal(base, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	wantOrder := []string{"src/a.py", "src/m.py", "src/z.ts"}
	for i, want := range wantOrder {
		if doc.Findings[i].File != want {
			t.Fatalf("findings[%d].file = %q, want %q (order: %v)", i, doc.Findings[i].File, want, wantOrder)
		}
	}
}

// TestJSONSurvivesHostileCorpus round-trips the shared adversarial fixture
// table through the JSON export. The document must stay valid UTF-8 and
// parseable, keep every finding, and never carry raw hostile control bytes
// (encoding/json escapes the C0 controls but writes DEL through raw, which
// scrubControls must neutralize before marshaling).
func TestJSONSurvivesHostileCorpus(t *testing.T) {
	corpus := hostileCorpus()
	body := JSON(Build(Input{
		WorkspaceName: "Hostile \"Name\" <script>", StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Findings: corpus, Runs: []Run{hostileRun()},
	}))
	if !utf8.Valid(body) {
		t.Fatal("JSON export must be valid UTF-8")
	}
	for _, banned := range []string{"\x00", "\x07", "\x0b", "\x0c", "\x1b", "\x7f"} {
		if bytes.Contains(body, []byte(banned)) {
			t.Fatalf("control character %q survived into the export", banned)
		}
	}
	var doc struct {
		Findings []struct {
			RuleID string `json:"rule_id"`
			File   string `json:"file"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, body)
	}
	if len(doc.Findings) != len(corpus) {
		t.Fatalf("every corpus finding must be exported: %d of %d", len(doc.Findings), len(corpus))
	}
	for _, finding := range doc.Findings {
		if finding.RuleID == "" || finding.File == "" {
			t.Fatalf("finding lost its rule id or file: %#v", finding)
		}
	}
}

func TestJSONEmptyListsAreArraysNotNull(t *testing.T) {
	body := string(JSON(Build(Input{WorkspaceName: "Empty"})))
	for _, want := range []string{`"analyzers": []`, `"findings": []`, `"metrics": []`, `"warnings": []`} {
		if !strings.Contains(body, want) {
			t.Fatalf("%s missing (lists must serialize as empty arrays, never null):\n%s", want, body)
		}
	}
}

// TestJSONDerivesComparisonStatus pins how a finding's status is projected
// when the loader behind the report model returns no status column: the
// comparison buckets decide (persistent when the previous completed scan had
// the fingerprint, new when the comparison classified it), an explicit Status
// always wins, and a finding outside any comparison keeps an empty status
// instead of a guess.
func TestJSONDerivesComparisonStatus(t *testing.T) {
	persistent := analyzers.Finding{AnalyzerID: "ruff", RuleID: "P", Severity: analyzers.SeverityHigh, Message: "kept", RelativePath: "a.py", Fingerprint: "fp-kept"}
	fresh := analyzers.Finding{AnalyzerID: "ruff", RuleID: "N", Severity: analyzers.SeverityMedium, Message: "added", RelativePath: "b.py", Fingerprint: "fp-added"}
	explicit := analyzers.Finding{AnalyzerID: "ruff", RuleID: "E", Severity: analyzers.SeverityLow, Message: "preset", RelativePath: "c.py", Fingerprint: "fp-preset", Status: "suppressed"}
	outside := analyzers.Finding{AnalyzerID: "ruff", RuleID: "U", Severity: analyzers.SeverityInfo, Message: "unknown", RelativePath: "d.py"}
	body := JSON(Build(Input{
		WorkspaceName: "Derive",
		Findings:      []analyzers.Finding{persistent, fresh, explicit, outside},
		Comparison:    Comparison{New: []analyzers.Finding{fresh}, Persistent: []analyzers.Finding{persistent}},
	}))
	var doc struct {
		Findings []struct {
			RuleID string `json:"rule_id"`
			Status string `json:"status"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, body)
	}
	byRule := map[string]string{}
	for _, f := range doc.Findings {
		byRule[f.RuleID] = f.Status
	}
	if byRule["P"] != "persistent" {
		t.Fatalf("fingerprint in previous scan must derive persistent: %#v", byRule)
	}
	if byRule["N"] != "new" {
		t.Fatalf("fingerprint absent from previous scan must derive new: %#v", byRule)
	}
	if byRule["E"] != "suppressed" {
		t.Fatalf("explicit status must win over derivation: %#v", byRule)
	}
	if byRule["U"] != "" {
		t.Fatalf("finding outside any comparison must keep an empty status: %#v", byRule)
	}
}

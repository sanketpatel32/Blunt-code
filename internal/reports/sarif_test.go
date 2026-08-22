package reports

import (
	"bluntcode/internal/analyzers"
	"encoding/json"
	"strings"
	"testing"
)

// sarifDocument marshals a model to SARIF and decodes it back into generic
// maps, mirroring what a SARIF consumer parses, so tests can assert both
// present and absent keys.
func sarifDocument(t *testing.T, in Input) map[string]any {
	t.Helper()
	data, err := json.Marshal(SARIF(Build(in)))
	if err != nil {
		t.Fatal(err)
	}
	var log map[string]any
	if err := json.Unmarshal(data, &log); err != nil {
		t.Fatal(err)
	}
	return log
}

func sarifFirstRun(t *testing.T, log map[string]any) map[string]any {
	t.Helper()
	runs, ok := log["runs"].([]any)
	if !ok || len(runs) != 1 {
		t.Fatalf("expected exactly one run: %#v", log)
	}
	run, ok := runs[0].(map[string]any)
	if !ok {
		t.Fatalf("run is not an object: %#v", runs[0])
	}
	return run
}

func sarifAllResults(t *testing.T, run map[string]any) []map[string]any {
	t.Helper()
	items, ok := run["results"].([]any)
	if !ok {
		t.Fatalf("results missing: %#v", run)
	}
	results := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("result is not an object: %#v", item)
		}
		results = append(results, result)
	}
	return results
}

func sarifDriverOf(t *testing.T, run map[string]any) map[string]any {
	t.Helper()
	tool, ok := run["tool"].(map[string]any)
	if !ok {
		t.Fatalf("tool missing: %#v", run)
	}
	driver, ok := tool["driver"].(map[string]any)
	if !ok {
		t.Fatalf("driver missing: %#v", tool)
	}
	return driver
}

func TestSARIFHeaderAndDriverMetadata(t *testing.T) {
	log := sarifDocument(t, Input{BluntCodeVersion: "1.2.3", Findings: []analyzers.Finding{{
		AnalyzerID: "ruff", RuleID: "F401", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryCorrectness,
		Title: "Unused import", Message: "unused", RelativePath: "src/a.py",
		DocumentationURL: "https://example.com/docs/F401",
	}}})
	if log["$schema"] != SARIFSchemaURI || log["version"] != "2.1.0" {
		t.Fatalf("unexpected header: %#v", log)
	}
	driver := sarifDriverOf(t, sarifFirstRun(t, log))
	if driver["name"] != "Blunt Code" || driver["version"] != "1.2.3" || driver["informationUri"] != "https://github.com/sanketpatel32/Blunt-code" {
		t.Fatalf("unexpected driver: %#v", driver)
	}
	rules, ok := driver["rules"].([]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("unexpected rules: %#v", driver["rules"])
	}
	rule := rules[0].(map[string]any)
	if rule["id"] != "F401" || rule["helpUri"] != "https://example.com/docs/F401" {
		t.Fatalf("unexpected rule: %#v", rule)
	}
	if description, ok := rule["shortDescription"].(map[string]any); !ok || description["text"] != "Unused import" {
		t.Fatalf("unexpected shortDescription: %#v", rule["shortDescription"])
	}
}

func TestSARIFDriverVersionOmittedWhenUnknown(t *testing.T) {
	log := sarifDocument(t, Input{})
	driver := sarifDriverOf(t, sarifFirstRun(t, log))
	if _, present := driver["version"]; present {
		t.Fatalf("version must be omitted when the scan carries none: %#v", driver)
	}
}

func TestSARIFMapsEverySeverityToALevel(t *testing.T) {
	findings := []analyzers.Finding{
		{AnalyzerID: "ruff", RuleID: "C1", Severity: analyzers.SeverityCritical, Message: "m", RelativePath: "a.py"},
		{AnalyzerID: "ruff", RuleID: "H1", Severity: analyzers.SeverityHigh, Message: "m", RelativePath: "a.py"},
		{AnalyzerID: "ruff", RuleID: "M1", Severity: analyzers.SeverityMedium, Message: "m", RelativePath: "a.py"},
		{AnalyzerID: "ruff", RuleID: "L1", Severity: analyzers.SeverityLow, Message: "m", RelativePath: "a.py"},
		{AnalyzerID: "ruff", RuleID: "I1", Severity: analyzers.SeverityInfo, Message: "m", RelativePath: "a.py"},
		{AnalyzerID: "ruff", RuleID: "X1", Severity: analyzers.Severity("bogus"), Message: "m", RelativePath: "a.py"},
	}
	results := sarifAllResults(t, sarifFirstRun(t, sarifDocument(t, Input{Findings: findings})))
	want := map[string]string{"C1": "error", "H1": "error", "M1": "warning", "L1": "warning", "I1": "note", "X1": "warning"}
	seen := map[string]string{}
	for _, result := range results {
		seen[result["ruleId"].(string)] = result["level"].(string)
	}
	if len(seen) != len(want) {
		t.Fatalf("unexpected result count: %#v", seen)
	}
	for rule, level := range want {
		if seen[rule] != level {
			t.Fatalf("rule %s: level %q, want %q", rule, seen[rule], level)
		}
	}
}

func TestSARIFDeduplicatesRulesAndKeepsRuleIndexesCorrect(t *testing.T) {
	findings := []analyzers.Finding{
		{AnalyzerID: "ruff", RuleID: "F401", Severity: analyzers.SeverityHigh, Message: "first", RelativePath: "a.py"},
		{AnalyzerID: "biome", RuleID: "lint/a", Severity: analyzers.SeverityLow, Message: "second", RelativePath: "b.ts"},
		{AnalyzerID: "ruff", RuleID: "F401", Severity: analyzers.SeverityLow, Message: "third", RelativePath: "c.py"},
	}
	log := sarifDocument(t, Input{Findings: findings})
	driver := sarifDriverOf(t, sarifFirstRun(t, log))
	rules, ok := driver["rules"].([]any)
	if !ok || len(rules) != 2 {
		t.Fatalf("rules must deduplicate by rule id: %#v", driver["rules"])
	}
	results := sarifAllResults(t, sarifFirstRun(t, log))
	if len(results) != 3 {
		t.Fatalf("every finding stays a result: %#v", results)
	}
	// rules[0] must be F401 (first seen) and rules[1] lint/a, with both F401
	// results pointing at index 0 and the biome result at index 1.
	for i, want := range []struct {
		rule  string
		index float64
	}{{"F401", 0}, {"lint/a", 1}, {"F401", 0}} {
		if results[i]["ruleId"] != want.rule || results[i]["ruleIndex"] != want.index {
			t.Fatalf("result %d: %#v", i, results[i])
		}
	}
}

func TestSARIFRegionOmitsUnsetPositionedFields(t *testing.T) {
	results := sarifAllResults(t, sarifFirstRun(t, sarifDocument(t, Input{Findings: []analyzers.Finding{{
		AnalyzerID: "ruff", RuleID: "F401", Severity: analyzers.SeverityMedium, Message: "m", RelativePath: "src/a.py", StartLine: 7,
	}}})))
	if len(results) != 1 {
		t.Fatalf("unexpected results: %#v", results)
	}
	location := results[0]["locations"].([]any)[0].(map[string]any)["physicalLocation"].(map[string]any)
	region := location["region"].(map[string]any)
	if region["startLine"] != float64(7) {
		t.Fatalf("startLine must survive: %#v", region)
	}
	for _, key := range []string{"startColumn", "endLine", "endColumn"} {
		if _, present := region[key]; present {
			t.Fatalf("region key %s must be omitted when unset: %#v", key, region)
		}
	}
}

func TestSARIFArtifactURIsUseForwardSlashesAndEncoding(t *testing.T) {
	results := sarifAllResults(t, sarifFirstRun(t, sarifDocument(t, Input{Findings: []analyzers.Finding{
		{AnalyzerID: "ruff", RuleID: "F401", Severity: analyzers.SeverityMedium, Message: "m", RelativePath: `src\nested\a file.py`},
		{AnalyzerID: "ruff", RuleID: "F402", Severity: analyzers.SeverityMedium, Message: "m", RelativePath: "/src/absolute.py"},
	}})))
	if len(results) != 2 {
		t.Fatalf("unexpected results: %#v", results)
	}
	uri := func(i int) string {
		return results[i]["locations"].([]any)[0].(map[string]any)["physicalLocation"].(map[string]any)["artifactLocation"].(map[string]any)["uri"].(string)
	}
	if uri(0) != "src/nested/a%20file.py" {
		t.Fatalf("backslashes must become slashes and spaces must be encoded: %q", uri(0))
	}
	if uri(1) != "src/absolute.py" {
		t.Fatalf("URIs stay relative without a leading slash: %q", uri(1))
	}
}

func TestSARIFAppendsRemediationAsSecondSentence(t *testing.T) {
	results := sarifAllResults(t, sarifFirstRun(t, sarifDocument(t, Input{Findings: []analyzers.Finding{
		{AnalyzerID: "semgrep", RuleID: "S1", Severity: analyzers.SeverityHigh, Message: "Hardcoded credential detected", RelativePath: "a.py", Remediation: "Move it to an environment variable."},
		{AnalyzerID: "semgrep", RuleID: "S2", Severity: analyzers.SeverityHigh, Message: "Already complete.", RelativePath: "b.py", Remediation: "Do this next."},
	}})))
	first := results[0]["message"].(map[string]any)["text"].(string)
	if first != "Hardcoded credential detected. Move it to an environment variable." {
		t.Fatalf("remediation must be appended as a second sentence: %q", first)
	}
	second := results[1]["message"].(map[string]any)["text"].(string)
	if second != "Already complete. Do this next." {
		t.Fatalf("an existing sentence must not gain a second period: %q", second)
	}
}

func TestSARIFProjectLevelFindingHasNoLocations(t *testing.T) {
	results := sarifAllResults(t, sarifFirstRun(t, sarifDocument(t, Input{Findings: []analyzers.Finding{
		{AnalyzerID: "ruff", RuleID: "F401", Severity: analyzers.SeverityMedium, Message: "m", RelativePath: "src/a.py"},
		{AnalyzerID: "sonarqube", RuleID: "project:coverage", Severity: analyzers.SeverityInfo, Message: "coverage below threshold"},
	}})))
	if len(results) != 2 {
		t.Fatalf("unexpected results: %#v", results)
	}
	if _, present := results[0]["locations"]; !present {
		t.Fatalf("file-backed finding must keep its location: %#v", results[0])
	}
	if _, present := results[1]["locations"]; present {
		t.Fatalf("project-level finding must omit locations: %#v", results[1])
	}
	if !strings.Contains(results[1]["ruleId"].(string), "project") {
		t.Fatalf("unexpected project result: %#v", results[1])
	}
}

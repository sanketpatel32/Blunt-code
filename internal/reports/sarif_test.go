package reports

import (
	"bluntcode/internal/analyzers"
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
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

// TestSARIFDuplicateRuleIDKeepsFirstHelpURI pins the dedup behavior when the
// same rule id arrives with different documentation URLs (possible when two
// analyzers, or a hostile scanned repo's custom rules, reuse one id). The first
// non-empty helpUri wins deterministically and every result's ruleIndex stays
// bound to that single descriptor, so later findings can never swap the link a
// user clicks.
func TestSARIFDuplicateRuleIDKeepsFirstHelpURI(t *testing.T) {
	findings := []analyzers.Finding{
		{AnalyzerID: "ruff", RuleID: "DUP", Severity: analyzers.SeverityHigh, Message: "no url yet", RelativePath: "a.py"},
		{AnalyzerID: "biome", RuleID: "DUP", Severity: analyzers.SeverityLow, Message: "first url", RelativePath: "b.ts", DocumentationURL: "https://first.example/DUP"},
		{AnalyzerID: "semgrep", RuleID: "DUP", Severity: analyzers.SeverityInfo, Message: "later conflicting url", RelativePath: "c.js", DocumentationURL: "https://evil.example/DUP"},
	}
	log := sarifDocument(t, Input{Findings: findings})
	driver := sarifDriverOf(t, sarifFirstRun(t, log))
	rules, ok := driver["rules"].([]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("one rule id must stay one descriptor: %#v", driver["rules"])
	}
	rule := rules[0].(map[string]any)
	if rule["helpUri"] != "https://first.example/DUP" {
		t.Fatalf("first non-empty helpUri must win: %#v", rule)
	}
	results := sarifAllResults(t, sarifFirstRun(t, log))
	if len(results) != 3 {
		t.Fatalf("every finding stays a result: %#v", results)
	}
	for i, result := range results {
		if result["ruleIndex"] != float64(0) {
			t.Fatalf("result %d must point at the single descriptor: %#v", i, result)
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

// TestSARIFSurvivesHostileCorpus round-trips the hostile fixture table through
// the SARIF export. Every finding must survive as a result with its exact rule
// id and message text, every ruleIndex must resolve to a descriptor whose id
// matches the result's ruleId, artifact URIs must decode back to the original
// path, and regions must satisfy the SARIF 2.1.0 constraints no matter what
// the analyzer stored.
func TestSARIFSurvivesHostileCorpus(t *testing.T) {
	corpus := hostileCorpus()
	log := sarifDocument(t, Input{Findings: corpus})
	data, err := json.Marshal(SARIF(Build(Input{Findings: corpus})))
	if err != nil {
		t.Fatal(err)
	}

	// Byte level: valid UTF-8, non-ASCII kept raw (JSON escapes only markup
	// characters and U+2028/U+2029), and no raw control bytes anywhere.
	if !utf8.Valid(data) {
		t.Fatal("SARIF output must be valid UTF-8")
	}
	for _, raw := range []string{"\x00", "\x07", "\x1b", "\x7f"} {
		if strings.Contains(string(data), raw) {
			t.Fatalf("raw control byte %q survived into the SARIF bytes", raw)
		}
	}
	for _, want := range []string{"🚨🚀", "\u202b\u202e", "e\u0301\u0301clat"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("non-ASCII payload %q must stay raw UTF-8 in the JSON: %s", want, data)
		}
	}

	if log["$schema"] != SARIFSchemaURI || log["version"] != "2.1.0" {
		t.Fatalf("unexpected header: %#v", log)
	}
	run := sarifFirstRun(t, log)
	driver := sarifDriverOf(t, run)
	rules, _ := driver["rules"].([]any)
	results := sarifAllResults(t, run)
	if len(results) != len(corpus) {
		t.Fatalf("every finding stays a result: got %d for %d findings", len(results), len(corpus))
	}

	uniqueRules := map[string]bool{}
	for i, result := range results {
		want := corpus[i]
		if result["ruleId"] != want.RuleID {
			t.Fatalf("result %d ruleId %q, want the exact id %q", i, result["ruleId"], want.RuleID)
		}
		// ruleIndex must resolve to a descriptor carrying this result's ruleId.
		index := int(result["ruleIndex"].(float64))
		if index < 0 || index >= len(rules) {
			t.Fatalf("result %d ruleIndex %d out of range (%d rules)", i, index, len(rules))
		}
		if id := rules[index].(map[string]any)["id"]; id != want.RuleID {
			t.Fatalf("result %d ruleIndex %d resolves to %v, want %q", i, index, id, want.RuleID)
		}
		uniqueRules[want.RuleID] = true

		// Message text must be the exact source composition after the export's
		// only transformation: hostile control bytes become spaces (the same
		// scrubbed() contract the HTML corpus test models).
		expected := scrubbed(strings.TrimSpace(want.Message))
		if remediation := scrubbed(strings.TrimSpace(want.Remediation)); remediation != "" {
			if expected == "" {
				expected = remediation
			} else if strings.HasSuffix(expected, ".") || strings.HasSuffix(expected, "!") || strings.HasSuffix(expected, "?") {
				expected = expected + " " + remediation
			} else {
				expected = expected + ". " + remediation
			}
		}
		if text := result["message"].(map[string]any)["text"]; text != expected {
			t.Fatalf("result %d (%s) message %q, want %q", i, want.RuleID, text, expected)
		}

		// Region validity: SARIF 2.1.0 requires startColumn to accompany
		// startLine, endLine >= startLine, and endColumn only with endLine.
		locations, _ := result["locations"].([]any)
		if len(locations) != 1 {
			t.Fatalf("result %d must carry exactly one location: %#v", i, result["locations"])
		}
		physical := locations[0].(map[string]any)["physicalLocation"].(map[string]any)
		region, _ := physical["region"].(map[string]any)
		startLine, hasStartLine := region["startLine"]
		startColumn, hasStartColumn := region["startColumn"]
		endLine, hasEndLine := region["endLine"]
		endColumn, hasEndColumn := region["endColumn"]
		if hasStartColumn && !hasStartLine {
			t.Fatalf("result %d region has startColumn without startLine: %#v", i, region)
		}
		if hasEndLine {
			if !hasStartLine || endLine.(float64) < startLine.(float64) {
				t.Fatalf("result %d region endLine %v precedes startLine %v: %#v", i, endLine, startLine, region)
			}
		}
		if hasEndColumn && !hasEndLine {
			t.Fatalf("result %d region has endColumn without endLine: %#v", i, region)
		}
		if hasEndColumn && hasStartColumn && hasEndLine && endLine.(float64) == startLine.(float64) && endColumn.(float64) < startColumn.(float64) {
			t.Fatalf("result %d region same-line endColumn %v precedes startColumn %v: %#v", i, endColumn, startColumn, region)
		}

		// Artifact URI must decode back to the workspace-relative path.
		uri := physical["artifactLocation"].(map[string]any)["uri"].(string)
		decoded, err := url.PathUnescape(uri)
		if err != nil {
			t.Fatalf("result %d artifact uri %q does not decode: %v", i, uri, err)
		}
		expectedPath := strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(want.RelativePath)), "./")
		expectedPath = strings.TrimPrefix(expectedPath, "/")
		if decoded != expectedPath {
			t.Fatalf("result %d artifact uri %q decodes to %q, want %q", i, uri, decoded, expectedPath)
		}
	}
	if len(rules) != len(uniqueRules) {
		t.Fatalf("rules must deduplicate by exact id: %d rules for %d unique ids", len(rules), len(uniqueRules))
	}
}

// TestSARIFRegionSanitizesInvalidCoordinates pins the region rules for the
// two malformed-coordinate shapes a hostile or buggy analyzer can produce:
// a start column without a start line, and end positions before their starts.
// Contradictory coordinates are dropped, never invented.
func TestSARIFRegionSanitizesInvalidCoordinates(t *testing.T) {
	results := sarifAllResults(t, sarifFirstRun(t, sarifDocument(t, Input{Findings: hostileCorpus()})))
	byRule := map[string]map[string]any{}
	for _, result := range results {
		byRule[result["ruleId"].(string)] = result["locations"].([]any)[0].(map[string]any)["physicalLocation"].(map[string]any)["region"].(map[string]any)
	}
	if region := byRule["column-without-line"]; len(region) != 0 {
		t.Fatalf("a start column without a start line must drop every region coordinate: %#v", region)
	}
	region := byRule["inverted-region"]
	if region["startLine"] != float64(10) || region["startColumn"] != float64(8) {
		t.Fatalf("valid start coordinates must survive: %#v", region)
	}
	if _, present := region["endLine"]; present {
		t.Fatalf("endLine before startLine must be dropped: %#v", region)
	}
	if _, present := region["endColumn"]; present {
		t.Fatalf("endColumn of a dropped end must go with it: %#v", region)
	}
	region = byRule["unicode-bmp-astral"]
	if region["startLine"] != float64(3) || region["startColumn"] != float64(4) || region["endLine"] != float64(3) || region["endColumn"] != float64(9) {
		t.Fatalf("a fully valid region must survive untouched: %#v", region)
	}
}

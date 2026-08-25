package semgrep

import (
	"strings"
	"testing"
)

func TestBundledRulepackParsesAndValidates(t *testing.T) {
	pack := RulesYAML()
	if pack == "" {
		t.Fatal("bundled rulepack is empty")
	}
	if strings.ContainsRune(pack, '\r') {
		t.Fatal("bundled rulepack must keep LF line endings")
	}
	rules, err := parseRulepack(pack)
	if err != nil {
		t.Fatalf("parse bundled rulepack: %v", err)
	}
	if err := validateRulepack(rules); err != nil {
		t.Fatalf("validate bundled rulepack: %v", err)
	}
	covered := map[string]bool{}
	for _, r := range rules {
		for _, lang := range r.Languages {
			covered[lang] = true
		}
	}
	for _, lang := range []string{"python", "javascript", "typescript"} {
		if !covered[lang] {
			t.Errorf("bundled rulepack has no rule covering %s", lang)
		}
	}
}

func TestBundledRulepackMinimumRuleCount(t *testing.T) {
	rules, err := parseRulepack(RulesYAML())
	if err != nil {
		t.Fatal(err)
	}
	const minRules = 25
	if len(rules) < minRules {
		t.Fatalf("bundled rulepack shrank to %d rules, expected at least %d", len(rules), minRules)
	}
}

// The tools package extracts this pack at managed-setup time; the wiring test
// in internal/tools asserts the extracted bytes equal RulesYAML().
func TestRulesYAMLIsStableAcrossCalls(t *testing.T) {
	first, second := RulesYAML(), RulesYAML()
	if first != second {
		t.Fatal("RulesYAML returned different content across calls")
	}
	if strings.TrimSpace(first) == "" {
		t.Fatal("RulesYAML returned an empty rulepack")
	}
}

func TestParseRulepackParsesPackConstructs(t *testing.T) {
	const sample = `rules:
  - id: blunt-code.python.sample
    languages: [python, javascript]
    severity: WARNING
    message: "sample: \"quoted\" text"
    metadata:
      category: security
    patterns:
      - pattern-either:
          - pattern: eval($X)
          - pattern: "exec(\"...\")"
      - pattern-not: "os.system(\"...\")"
      - metavariable-regex:
          metavariable: $X
          regex: "(?i)^keep"
  - id: blunt-code.python.only-pattern
    languages: [python]
    severity: INFO
    message: plain message
    pattern: eval(...)
`
	rules, err := parseRulepack(sample)
	if err != nil {
		t.Fatalf("parse sample rulepack: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(rules))
	}
	first := rules[0]
	if first.ID != "blunt-code.python.sample" || first.Severity != "WARNING" {
		t.Fatalf("unexpected header fields: %#v", first)
	}
	if want := `sample: "quoted" text`; first.Message != want {
		t.Fatalf("message = %q, want %q", first.Message, want)
	}
	if len(first.Languages) != 2 || first.Languages[0] != "python" || first.Languages[1] != "javascript" {
		t.Fatalf("languages = %#v", first.Languages)
	}
	if first.Category != "security" {
		t.Fatalf("category = %q, want security", first.Category)
	}
	if first.Pattern != "" || first.Patterns != 3 {
		t.Fatalf("pattern body = %q with %d patterns entries, want empty pattern and 3 entries", first.Pattern, first.Patterns)
	}
	second := rules[1]
	if second.Pattern != "eval(...)" || second.Patterns != 0 {
		t.Fatalf("unexpected pattern body: %q with %d entries", second.Pattern, second.Patterns)
	}
	if _, err := parseRulepack("rules: 5"); err == nil {
		t.Fatal("expected error for non-list rules value")
	}
}

func TestValidateRulepackRejectsMalformedRules(t *testing.T) {
	valid := rule{ID: "blunt-code.python.x", Message: "message", Severity: "ERROR", Languages: []string{"python"}, Pattern: "eval(...)"}
	mutate := func(f func(*rule)) []rule {
		r := valid
		f(&r)
		return []rule{r}
	}
	cases := []struct {
		name    string
		rules   []rule
		wantErr string
	}{
		{"empty pack", nil, "no rules"},
		{"empty id", mutate(func(r *rule) { r.ID = "" }), "empty id"},
		{"wrong id prefix", mutate(func(r *rule) { r.ID = "semgrep.python.x" }), "prefix"},
		{"empty message", mutate(func(r *rule) { r.Message = "" }), "empty message"},
		{"bad severity", mutate(func(r *rule) { r.Severity = "high" }), "severity"},
		{"no languages", mutate(func(r *rule) { r.Languages = nil }), "no languages"},
		{"unknown language", mutate(func(r *rule) { r.Languages = []string{"go"} }), "language"},
		{"unknown category", mutate(func(r *rule) { r.Category = "style" }), "category"},
		{"no pattern body", mutate(func(r *rule) { r.Pattern = "" }), "pattern"},
		{"duplicate ids", []rule{valid, valid}, "duplicate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRulepack(tc.rules)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

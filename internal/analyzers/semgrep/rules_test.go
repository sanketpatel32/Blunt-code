package semgrep

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// rule is the subset of the semgrep rule schema the bundled pack relies on.
// The adapter maps severity ERROR/WARNING/INFO onto Blunt Code
// high/medium/info findings and metadata.category onto Blunt Code
// categories, so both fields are validated below.
type rule struct {
	ID           string
	Message      string
	Severity     string
	Languages    []string
	Category     string
	Pattern      string
	Patterns     int
	PatternRegex string
}

var (
	rulepackLanguages  = map[string]bool{"python": true, "javascript": true, "typescript": true}
	rulepackSeverities = map[string]bool{"ERROR": true, "WARNING": true, "INFO": true}
	rulepackCategories = map[string]bool{"security": true, "vulnerability": true, "correctness": true}
)

// validateRulepack enforces the invariants every bundled rule must satisfy so
// a bad edit fails in tests instead of surfacing as semgrep runtime errors.
func validateRulepack(rules []rule) error {
	if len(rules) == 0 {
		return fmt.Errorf("rulepack defines no rules")
	}
	seen := make(map[string]struct{}, len(rules))
	for i, r := range rules {
		at := fmt.Sprintf("rule %d (%q)", i+1, r.ID)
		if r.ID == "" {
			return fmt.Errorf("%s: empty id", at)
		}
		if !strings.HasPrefix(r.ID, "blunt-code.") {
			return fmt.Errorf("%s: id must use the blunt-code.<language>.<rule> prefix", at)
		}
		if _, dup := seen[r.ID]; dup {
			return fmt.Errorf("%s: duplicate id", at)
		}
		seen[r.ID] = struct{}{}
		if r.Message == "" {
			return fmt.Errorf("%s: empty message", at)
		}
		if !rulepackSeverities[r.Severity] {
			return fmt.Errorf("%s: severity %q must be one of ERROR, WARNING, INFO", at, r.Severity)
		}
		if len(r.Languages) == 0 {
			return fmt.Errorf("%s: no languages", at)
		}
		for _, lang := range r.Languages {
			if !rulepackLanguages[lang] {
				return fmt.Errorf("%s: unsupported language %q", at, lang)
			}
		}
		if r.Category != "" && !rulepackCategories[r.Category] {
			return fmt.Errorf("%s: unknown metadata category %q", at, r.Category)
		}
		if r.Pattern == "" && r.Patterns == 0 && r.PatternRegex == "" {
			return fmt.Errorf("%s: needs a pattern, patterns, pattern-either, or pattern-regex body", at)
		}
	}
	return nil
}

// parseRulepack decodes the rulepack with a real YAML parser (yaml.v3 is a
// port of libyaml, which semgrep itself uses), so values libyaml rejects —
// such as plain scalars containing ": " — fail here too.
func parseRulepack(src string) ([]rule, error) {
	var pack struct {
		Rules []struct {
			ID            string   `yaml:"id"`
			Message       string   `yaml:"message"`
			Severity      string   `yaml:"severity"`
			Languages     []string `yaml:"languages"`
			Pattern       string   `yaml:"pattern"`
			PatternRegex  string   `yaml:"pattern-regex"`
			Patterns      []any    `yaml:"patterns"`
			PatternEither []any    `yaml:"pattern-either"`
			Metadata      struct {
				Category string `yaml:"category"`
			} `yaml:"metadata"`
		} `yaml:"rules"`
	}
	if err := yaml.Unmarshal([]byte(src), &pack); err != nil {
		return nil, fmt.Errorf("rulepack: %w", err)
	}
	if len(pack.Rules) == 0 {
		return nil, fmt.Errorf("rulepack: rules must be a non-empty list")
	}
	rules := make([]rule, 0, len(pack.Rules))
	for _, r := range pack.Rules {
		rules = append(rules, rule{
			ID:           r.ID,
			Message:      r.Message,
			Severity:     r.Severity,
			Languages:    r.Languages,
			Category:     r.Metadata.Category,
			Pattern:      r.Pattern,
			Patterns:     len(r.Patterns) + len(r.PatternEither),
			PatternRegex: r.PatternRegex,
		})
	}
	return rules, nil
}

func TestBundledRulepackParsesAndValidates(t *testing.T) {
	pack := RulesYAML()
	if pack == "" {
		t.Fatal("bundled rulepack is empty")
	}
	if strings.ContainsRune(pack, '\r') {
		t.Fatal("bundled rulepack must keep LF line endings")
	}
	// The "<... X ... />" idiom is invalid semgrep JavaScript grammar: semgrep
	// rejects the whole config file for it, silently killing every rule. The
	// working form is "<$TAG ... X ...>...</$TAG>". This raw check guards the
	// grammar class a YAML parser cannot see.
	if strings.Contains(pack, "<... ") || strings.Contains(pack, " ... />") {
		t.Fatal(`bundled rulepack uses the invalid "<... ... />" pattern idiom`)
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
	// libyaml rejects a plain scalar containing ": " as an ambiguous mapping;
	// semgrep parses the pack with libyaml, so the pack must quote such values
	// and the parser must reject them the same way.
	const colonPlain = `rules:
  - id: blunt-code.python.x
    languages: [python]
    severity: ERROR
    message: m
    pattern: <... dangerouslySetInnerHTML = { __html: $V } ... />
`
	if _, err := parseRulepack(colonPlain); err == nil {
		t.Fatal("expected error for a plain scalar containing ': '")
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

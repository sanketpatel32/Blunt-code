package semgrep

import (
	_ "embed"
	"fmt"
	"strings"
)

// RulesFileName is the file name the bundled local semgrep rulepack ships
// under. The maintained pack lives in rules/ beside this file and is the
// single source of truth: the tools package extracts these bytes into the
// managed Semgrep directory during setup (see tools.ExtractSemgrepRules).
const RulesFileName = "blunt-code-local.yaml"

//go:embed rules/blunt-code-local.yaml
var bundledRules string

// RulesYAML returns the bundled local semgrep rulepack. Rule ids are part of
// finding fingerprints, so ids must stay stable once shipped.
func RulesYAML() string { return bundledRules }

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

// parseRulepack parses the rulepack YAML. The pack is generated in a fixed
// YAML subset (block mappings and sequences, flow sequences of scalars,
// quoted or plain scalars), which this small purpose-built parser covers
// without pulling a YAML dependency into the module.
func parseRulepack(src string) ([]rule, error) {
	root, err := parseYAML(src)
	if err != nil {
		return nil, err
	}
	entries := root.value("rules").list()
	if entries == nil {
		return nil, fmt.Errorf("rulepack: rules must be a non-empty list")
	}
	rules := make([]rule, 0, len(entries))
	for i, entry := range entries {
		if entry == nil || !entry.isMap {
			return nil, fmt.Errorf("rulepack: rule %d must be a mapping", i+1)
		}
		r := rule{
			ID:           entry.value("id").text(),
			Message:      entry.value("message").text(),
			Severity:     entry.value("severity").text(),
			Pattern:      entry.value("pattern").text(),
			PatternRegex: entry.value("pattern-regex").text(),
		}
		for _, lang := range entry.value("languages").list() {
			r.Languages = append(r.Languages, lang.text())
		}
		r.Patterns = len(entry.value("patterns").list()) + len(entry.value("pattern-either").list())
		r.Category = entry.value("metadata").value("category").text()
		rules = append(rules, r)
	}
	return rules, nil
}

// yamlNode is a node of the YAML subset understood by parseYAML.
type yamlNode struct {
	scalar   string
	isScalar bool
	isSeq    bool
	seq      []*yamlNode
	isMap    bool
	keys     []string
	values   []*yamlNode
}

// value returns the mapping value for key, or nil when absent.
func (n *yamlNode) value(key string) *yamlNode {
	if n == nil || !n.isMap {
		return nil
	}
	for i, k := range n.keys {
		if k == key {
			return n.values[i]
		}
	}
	return nil
}

// text returns the scalar text, or the empty string for non-scalars.
func (n *yamlNode) text() string {
	if n == nil || !n.isScalar {
		return ""
	}
	return n.scalar
}

// list returns the sequence entries, or nil for non-sequences.
func (n *yamlNode) list() []*yamlNode {
	if n == nil || !n.isSeq {
		return nil
	}
	return n.seq
}

type yamlLine struct {
	indent int
	text   string
}

// parseYAML parses src and requires that every line belongs to the tree.
func parseYAML(src string) (*yamlNode, error) {
	lines := yamlLines(src)
	if len(lines) == 0 {
		return nil, fmt.Errorf("yaml: empty document")
	}
	root, next, err := parseBlock(lines, 0, lines[0].indent)
	if err != nil {
		return nil, err
	}
	if next != len(lines) {
		return nil, fmt.Errorf("yaml: unexpected content %q", lines[next].text)
	}
	return root, nil
}

// yamlLines strips comments and blank lines and records indentation.
func yamlLines(src string) []yamlLine {
	var out []yamlLine
	for _, raw := range strings.Split(src, "\n") {
		line := stripYAMLComment(raw)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		out = append(out, yamlLine{indent: indent, text: trimmed})
	}
	return out
}

// stripYAMLComment removes a trailing comment that starts outside quotes.
func stripYAMLComment(line string) string {
	inSingle, inDouble := false, false
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle && !(i > 0 && line[i-1] == '\\'):
			inDouble = !inDouble
		case c == '#' && !inSingle && !inDouble:
			if i == 0 || line[i-1] == ' ' || line[i-1] == '\t' {
				return line[:i]
			}
		}
	}
	return line
}

func isSeqItem(text string) bool {
	return text == "-" || strings.HasPrefix(text, "- ")
}

// parseBlock parses the mapping or sequence that starts at lines[i] with the
// given indentation and returns the index of the first line beyond it.
func parseBlock(lines []yamlLine, i, indent int) (*yamlNode, int, error) {
	if i >= len(lines) {
		return nil, i, fmt.Errorf("yaml: unexpected end of document")
	}
	if isSeqItem(lines[i].text) {
		return parseSeqBlock(lines, i, indent)
	}
	return parseMapBlock(lines, i, indent)
}

func parseSeqBlock(lines []yamlLine, i, indent int) (*yamlNode, int, error) {
	node := &yamlNode{isSeq: true}
	for i < len(lines) && lines[i].indent == indent && isSeqItem(lines[i].text) {
		rest := strings.TrimSpace(lines[i].text[1:])
		if rest == "" {
			child, next, err := parseNestedBlock(lines, i+1, indent)
			if err != nil {
				return nil, i, err
			}
			node.seq = append(node.seq, child)
			i = next
			continue
		}
		// The item content starts on the dash line; parse it as a block whose
		// base indentation is the column where the content begins.
		base := indent + 1 + (len(lines[i].text) - 1 - len(strings.TrimLeft(lines[i].text[1:], " ")))
		sub := make([]yamlLine, 0, len(lines)-i)
		sub = append(sub, yamlLine{indent: base, text: rest})
		sub = append(sub, lines[i+1:]...)
		child, consumed, err := parseBlock(sub, 0, base)
		if err != nil {
			return nil, i, err
		}
		node.seq = append(node.seq, child)
		i += consumed
	}
	return node, i, nil
}

func parseMapBlock(lines []yamlLine, i, indent int) (*yamlNode, int, error) {
	node := &yamlNode{isMap: true}
	for i < len(lines) && lines[i].indent == indent {
		key, value, ok := splitKeyValue(lines[i].text)
		if !ok {
			return nil, i, fmt.Errorf("yaml: expected key-value line, got %q", lines[i].text)
		}
		var child *yamlNode
		if value == "" {
			if i+1 < len(lines) && lines[i+1].indent > indent {
				var next int
				var err error
				child, next, err = parseBlock(lines, i+1, lines[i+1].indent)
				if err != nil {
					return nil, i, err
				}
				i = next
			} else {
				child = &yamlNode{isScalar: true}
				i++
			}
		} else {
			child = parseInlineValue(value)
			i++
		}
		node.keys = append(node.keys, key)
		node.values = append(node.values, child)
	}
	if len(node.keys) == 0 {
		return nil, i, fmt.Errorf("yaml: empty mapping")
	}
	return node, i, nil
}

// parseNestedBlock parses the block that belongs to a bare dash and returns
// the index of the first line beyond it.
func parseNestedBlock(lines []yamlLine, i, parentIndent int) (*yamlNode, int, error) {
	if i >= len(lines) || lines[i].indent <= parentIndent {
		return &yamlNode{isScalar: true}, i, nil
	}
	return parseBlock(lines, i, lines[i].indent)
}

// splitKeyValue splits "key: value" on the first unquoted colon.
func splitKeyValue(text string) (key, value string, ok bool) {
	inSingle, inDouble := false, false
	for i := 0; i < len(text); i++ {
		switch c := text[i]; {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle && !(i > 0 && text[i-1] == '\\'):
			inDouble = !inDouble
		case c == ':' && !inSingle && !inDouble:
			if i == len(text)-1 {
				return strings.TrimSpace(text[:i]), "", true
			}
			if text[i+1] == ' ' || text[i+1] == '\t' {
				return strings.TrimSpace(text[:i]), strings.TrimSpace(text[i+1:]), true
			}
		}
	}
	return "", "", false
}

// parseInlineValue parses a scalar or a flow sequence such as [a, b].
func parseInlineValue(v string) *yamlNode {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "[") {
		if !strings.HasSuffix(v, "]") {
			return nil
		}
		node := &yamlNode{isSeq: true}
		inner := strings.TrimSpace(v[1 : len(v)-1])
		if inner == "" {
			return node
		}
		for _, part := range splitFlowItems(inner) {
			node.seq = append(node.seq, parseInlineValue(part))
		}
		return node
	}
	return parseScalar(v)
}

func parseScalar(s string) *yamlNode {
	node := &yamlNode{isScalar: true}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		node.scalar = unescapeDoubleQuoted(s[1 : len(s)-1])
		return node
	}
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		node.scalar = strings.ReplaceAll(s[1:len(s)-1], "''", "'")
		return node
	}
	node.scalar = s
	return node
}

func unescapeDoubleQuoted(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// splitFlowItems splits a flow sequence body on top-level commas.
func splitFlowItems(s string) []string {
	var parts []string
	inSingle, inDouble := false, false
	start := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle && !(i > 0 && s[i-1] == '\\'):
			inDouble = !inDouble
		case c == ',' && !inSingle && !inDouble:
			parts = append(parts, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	return append(parts, strings.TrimSpace(s[start:]))
}

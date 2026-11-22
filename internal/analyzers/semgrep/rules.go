package semgrep

import _ "embed"

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

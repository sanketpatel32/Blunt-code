package tools

import (
	"fmt"
	"os"
	"path/filepath"

	analyzerssemgrep "bluntcode/internal/analyzers/semgrep"
)

// SemgrepRulesVersion identifies the local rules copied during managed setup.
// These rules are authored by Blunt Code, not fetched from the Semgrep registry.
// Bumped to 2.0.0 when the bundled pack grew from 2 to 20 rules so existing
// installations re-extract it.
const SemgrepRulesVersion = "3.0.0"

// ExtractSemgrepRules writes the bundled local rules atomically into the
// managed Semgrep directory. A successful scan only needs this local copy.
// The pack itself lives with the semgrep adapter
// (internal/analyzers/semgrep/rules) so its parser and validation tests keep
// a single source of truth.
func ExtractSemgrepRules(destination string) error {
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	if err := writePrivateFile(filepath.Join(destination, analyzerssemgrep.RulesFileName), []byte(analyzerssemgrep.RulesYAML())); err != nil {
		return err
	}
	return writePrivateFile(filepath.Join(destination, "RULES_VERSION"), []byte(SemgrepRulesVersion+"\n"))
}

func writePrivateFile(path string, content []byte) error {
	stage := path + ".new"
	if err := os.WriteFile(stage, content, 0o600); err != nil {
		return err
	}
	if err := os.Rename(stage, path); err != nil {
		return fmt.Errorf("install %s: %w", filepath.Base(path), err)
	}
	return nil
}

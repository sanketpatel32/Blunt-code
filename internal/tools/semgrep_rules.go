package tools

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// SemgrepRulesVersion identifies the local rules copied during managed setup.
// These rules are authored by Blunt Code, not fetched from the Semgrep registry.
const SemgrepRulesVersion = "1.0.0"

//go:embed semgrep-rules/blunt-code-local.yaml
var bundledSemgrepRules []byte

// ExtractSemgrepRules writes the bundled local rules atomically into the
// managed Semgrep directory. A successful scan only needs this local copy.
func ExtractSemgrepRules(destination string) error {
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	if err := writePrivateFile(filepath.Join(destination, "blunt-code-local.yaml"), bundledSemgrepRules); err != nil {
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

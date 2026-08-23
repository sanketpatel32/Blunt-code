//go:build windows

package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bluntcode/internal/instance"
)

// The repair pass must refuse to touch a data directory another Blunt Code
// process owns, exactly like `bluntcode scan` refuses to start against it.
// The lock is simulated the same way internal/instance's own tests do: a
// guard acquired in-process before Run.
func TestFixRefusesRepairsWhileDataDirIsInUse(t *testing.T) {
	paths, service := newFixFixture(t)
	rulesDir := plantSemgrepInstall(t, service, "1.0.0")
	leftover := filepath.Join(paths.ToolsDir, "ruff", "0.16.0", "ruff.exe.new")
	if err := os.MkdirAll(filepath.Dir(leftover), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(leftover, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	guard, err := instance.Acquire(paths.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()

	result := runFix(t, paths, service, true)
	semgrep := find(result, "Semgrep")
	if semgrep.Status != StatusWarn || semgrep.Repair == nil || semgrep.Repair.State != RepairSkipped {
		t.Fatalf("semgrep check = %#v", semgrep)
	}
	if !strings.Contains(semgrep.Repair.Action, "already using the data directory") && !strings.Contains(semgrep.Repair.Action, "data directory") {
		t.Fatalf("skip reason = %q", semgrep.Repair.Action)
	}
	repairs := find(result, "Repairs")
	if repairs.Status != StatusWarn || !strings.Contains(repairs.Detail, "no repairs applied") {
		t.Fatalf("repairs summary = %#v", repairs)
	}
	if row := find(result, "Install staging leftovers"); row.Status != StatusSkip {
		t.Fatalf("staging row = %#v", row)
	}
	if _, err := os.Stat(leftover); err != nil {
		t.Fatalf("locked run must not delete leftovers: %v", err)
	}
	version, err := os.ReadFile(filepath.Join(rulesDir, "RULES_VERSION"))
	if err != nil || string(version) != "1.0.0\n" {
		t.Fatalf("locked run must not rewrite rules: %q, %v", string(version), err)
	}
}

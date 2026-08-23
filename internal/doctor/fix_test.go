package doctor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	analyzerssemgrep "bluntcode/internal/analyzers/semgrep"
	"bluntcode/internal/config"
	"bluntcode/internal/tools"
)

// testManifest pins one artifact per managed tool for the current platform so
// the repair pass sees the same "referenced" versions as a real install.
func testManifest(t *testing.T) tools.Manifest {
	t.Helper()
	plat := runtime.GOOS + "-" + runtime.GOARCH
	sha := func(char string) string { return strings.Repeat(char, 64) }
	return tools.Manifest{Artifacts: []tools.Artifact{
		{ToolID: "ruff", Version: "0.16.0", Platform: plat, SourceURL: "https://example.test/ruff.exe", SHA256: sha("1"), ArchiveType: "exe", Executable: "ruff.exe"},
		{ToolID: "biome", Version: "2.5.6", Platform: plat, SourceURL: "https://example.test/biome.exe", SHA256: sha("2"), ArchiveType: "exe", Executable: "biome.exe"},
		{ToolID: "uv", Version: "0.11.16", Platform: plat, SourceURL: "https://example.test/uv.zip", SHA256: sha("3"), ArchiveType: "zip", Executable: "uv.exe"},
		{ToolID: "semgrep", Version: "1.172.0", Platform: plat, SourceURL: "https://example.test/semgrep-1.172.0-cp310-cp310-win_amd64.whl", SHA256: sha("4"), ArchiveType: "wheel", Executable: "semgrep.exe", InstallKind: "uv_tool", Package: "semgrep==1.172.0"},
		{ToolID: "sonarqube-server", Version: "10.9.1", Platform: plat, SourceURL: "https://example.test/sonarqube.zip", SHA256: sha("5"), ArchiveType: "zip", Executable: "bin/windows-x86-64/Run.bat"},
		{ToolID: "sonar-scanner", Version: "7.1.0", Platform: plat, SourceURL: "https://example.test/sonar-scanner.zip", SHA256: sha("6"), ArchiveType: "zip", Executable: "bin/sonar-scanner.bat"},
		{ToolID: "java", Version: "21.0.8", Platform: plat, SourceURL: "https://example.test/java.zip", SHA256: sha("7"), ArchiveType: "zip", Executable: "bin/java.exe"},
	}}
}

func newFixFixture(t *testing.T) (config.Paths, *tools.Service) {
	t.Helper()
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return paths, tools.NewService(paths.ToolsDir, testManifest(t), true)
}

// plantSemgrepInstall writes a managed Semgrep executable plus local rules
// carrying the given version marker, the state an outdated or interrupted
// rules extraction leaves behind.
func plantSemgrepInstall(t *testing.T, service *tools.Service, rulesVersion string) string {
	t.Helper()
	paths, ok := service.SemgrepPaths()
	if !ok {
		t.Fatal("semgrep paths unavailable")
	}
	if err := os.MkdirAll(paths.RulesDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Executable, []byte("managed semgrep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.RulesDir, analyzerssemgrep.RulesFileName), []byte("rules: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.RulesDir, "RULES_VERSION"), []byte(rulesVersion+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return paths.RulesDir
}

func runFix(t *testing.T, paths config.Paths, service *tools.Service, fix bool) Result {
	t.Helper()
	return Run(context.Background(), Options{Version: "test", Paths: paths, Tools: service, Fix: fix})
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
		t.Fatalf("%s still present: %v", path, err)
	}
}

func TestFixReextractsStaleSemgrepRules(t *testing.T) {
	paths, service := newFixFixture(t)
	rulesDir := plantSemgrepInstall(t, service, "1.0.0")

	result := runFix(t, paths, service, true)
	check := find(result, "Semgrep")
	if check.Status != StatusOK || check.Repair == nil || check.Repair.State != RepairFixed {
		t.Fatalf("semgrep check = %#v", check)
	}
	if !strings.Contains(check.Repair.Action, tools.SemgrepRulesVersion) {
		t.Fatalf("repair action = %q", check.Repair.Action)
	}
	version, err := os.ReadFile(filepath.Join(rulesDir, "RULES_VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	if want := tools.SemgrepRulesVersion + "\n"; string(version) != want {
		t.Fatalf("RULES_VERSION = %q, want %q", string(version), want)
	}
	rules, err := os.ReadFile(filepath.Join(rulesDir, analyzerssemgrep.RulesFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(rules) != analyzerssemgrep.RulesYAML() {
		t.Fatalf("restored rules do not match the bundled rulepack (%d bytes)", len(rules))
	}

	// Idempotent: the second run finds nothing left to repair.
	second := runFix(t, paths, service, true)
	recheck := find(second, "Semgrep")
	if recheck.Status != StatusOK || recheck.Repair != nil {
		t.Fatalf("second run = %#v", recheck)
	}
	if staging := find(second, "Install staging leftovers"); staging.Status != StatusOK || staging.Repair != nil {
		t.Fatalf("second-run staging = %#v", staging)
	}
}

func TestFixRestoresMissingSemgrepRulesFile(t *testing.T) {
	paths, service := newFixFixture(t)
	rulesDir := plantSemgrepInstall(t, service, tools.SemgrepRulesVersion)
	if err := os.Remove(filepath.Join(rulesDir, analyzerssemgrep.RulesFileName)); err != nil {
		t.Fatal(err)
	}

	result := runFix(t, paths, service, true)
	check := find(result, "Semgrep")
	if check.Status != StatusOK || check.Repair == nil || check.Repair.State != RepairFixed {
		t.Fatalf("semgrep check = %#v", check)
	}
	if _, err := os.Stat(filepath.Join(rulesDir, analyzerssemgrep.RulesFileName)); err != nil {
		t.Fatalf("rules file not restored: %v", err)
	}
}

func TestFixRemovesInterruptedInstallLeftovers(t *testing.T) {
	paths, service := newFixFixture(t)
	toolVersion := filepath.Join(paths.ToolsDir, "ruff", "0.16.0")
	if err := os.MkdirAll(toolVersion, 0o700); err != nil {
		t.Fatal(err)
	}
	finalBinary := filepath.Join(toolVersion, "ruff.exe")
	if err := os.WriteFile(finalBinary, []byte("ruff"), 0o700); err != nil {
		t.Fatal(err)
	}
	stagedBinary := finalBinary + ".new"
	if err := os.WriteFile(stagedBinary, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	extractionStage := filepath.Join(paths.ToolsDir, "biome", "2.5.6.new-1234567890")
	if err := os.MkdirAll(extractionStage, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extractionStage, "biome.exe"), []byte("partial"), 0o700); err != nil {
		t.Fatal(err)
	}
	partialDownload := filepath.Join(paths.ToolsDir, ".downloads", "ruff-1234567890")
	if err := os.MkdirAll(filepath.Dir(partialDownload), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(partialDownload, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	staleBackup := filepath.Join(paths.ToolsDir, "ruff", "0.16.0.previous")
	if err := os.MkdirAll(staleBackup, 0o700); err != nil {
		t.Fatal(err)
	}
	// A backup whose final directory never appeared may be the only copy:
	// it must be kept and reported, never deleted.
	lonelyBackup := filepath.Join(paths.ToolsDir, "biome", "2.5.6.previous")
	if err := os.MkdirAll(lonelyBackup, 0o700); err != nil {
		t.Fatal(err)
	}
	sonarScratch := filepath.Join(paths.DataDir, "tmp", "sonar-987654321")
	if err := os.MkdirAll(sonarScratch, 0o700); err != nil {
		t.Fatal(err)
	}

	result := runFix(t, paths, service, true)
	row := find(result, "Install staging leftovers")
	if row.Status != StatusOK || row.Repair == nil || row.Repair.State != RepairFixed {
		t.Fatalf("staging row = %#v", row)
	}
	for _, gone := range []string{stagedBinary, extractionStage, partialDownload, staleBackup, sonarScratch} {
		mustNotExist(t, gone)
	}
	if _, err := os.Stat(finalBinary); err != nil {
		t.Fatalf("legitimate binary removed: %v", err)
	}
	if _, err := os.Stat(lonelyBackup); err != nil {
		t.Fatalf("backup without final directory must be kept: %v", err)
	}
	for _, name := range []string{"0.16.0.previous", "sonar-987654321", "2.5.6.previous"} {
		if !strings.Contains(row.Detail, name) {
			t.Fatalf("staging detail %q lacks %q", row.Detail, name)
		}
	}

	// Idempotent: the second run removes nothing new.
	second := runFix(t, paths, service, true)
	stagingAgain := find(second, "Install staging leftovers")
	if stagingAgain.Repair != nil && stagingAgain.Repair.State == RepairFixed {
		t.Fatalf("second run must not remove anything: %#v", stagingAgain)
	}
}

func TestFixRecreatesMissingDataDirectories(t *testing.T) {
	paths, service := newFixFixture(t)
	for _, dir := range []string{paths.TempDir, paths.LogsDir} {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
	}

	// Without --fix the diagnosis stays read-only.
	plain := runFix(t, paths, service, false)
	if row := find(plain, "Data directory"); row.Status != StatusFail || row.Repair != nil {
		t.Fatalf("diagnostics must stay read-only: %#v", row)
	}

	result := runFix(t, paths, service, true)
	row := find(result, "Data directory")
	if row.Status != StatusOK || row.Repair == nil || row.Repair.State != RepairFixed {
		t.Fatalf("data directory = %#v", row)
	}
	if !result.OK || result.ExitCode() != 0 {
		t.Fatalf("repaired run must pass: ok=%v", result.OK)
	}
	for _, dir := range []string{paths.TempDir, paths.LogsDir} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("%s not recreated: %v", dir, err)
		}
	}
}

func TestFixReportsOrphanedToolVersionsWithoutDeleting(t *testing.T) {
	paths, service := newFixFixture(t)
	orphans := []string{
		filepath.Join(paths.ToolsDir, "ruff", "0.99.0"),
		filepath.Join(paths.ToolsDir, "legacytool"),
	}
	for _, dir := range orphans {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	result := runFix(t, paths, service, true)
	row := find(result, "Orphaned tool versions")
	if row.Status != StatusWarn || row.Repair == nil || row.Repair.State != RepairManual {
		t.Fatalf("orphan row = %#v", row)
	}
	for _, dir := range orphans {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("%s must not be deleted: %v", dir, err)
		}
		if !strings.Contains(row.Detail, filepath.Base(dir)) {
			t.Fatalf("orphan detail %q lacks %q", row.Detail, filepath.Base(dir))
		}
	}
}

func TestFixMarksMissingManagedToolsAsReinstallRequired(t *testing.T) {
	paths, service := newFixFixture(t) // manifest present, nothing installed

	result := runFix(t, paths, service, true)
	for _, name := range []string{"Ruff", "Biome", "SonarQube installation", "SonarScanner", "Managed Java runtime"} {
		row := find(result, name)
		if row.Status != StatusWarn {
			t.Fatalf("%s status = %#v", name, row)
		}
		if row.Repair == nil || row.Repair.State != RepairReinstall {
			t.Fatalf("%s repair = %#v", name, row.Repair)
		}
		if !strings.Contains(row.Repair.Action, "bluntcode scan") {
			t.Fatalf("%s action = %q", name, row.Repair.Action)
		}
	}
	if entries, err := os.ReadDir(paths.ToolsDir); err == nil && len(entries) > 0 {
		t.Fatalf("doctor must not install anything, tools dir has %d entries", len(entries))
	}
}

func TestFixJSONOnlyAddsRepairFields(t *testing.T) {
	paths, service := newFixFixture(t)
	plantSemgrepInstall(t, service, "1.0.0")

	fixed, err := json.Marshal(runFix(t, paths, service, true))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fixed), `"repair"`) || !strings.Contains(string(fixed), `"state":"fixed"`) {
		t.Fatalf("fix run JSON lacks repair fields: %s", fixed)
	}

	plain, err := json.Marshal(runFix(t, paths, service, false))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "repair") {
		t.Fatalf("plain run JSON must stay unchanged: %s", plain)
	}
	var document map[string]any
	if err := json.Unmarshal(plain, &document); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"version", "ok", "checks"} {
		if _, ok := document[key]; !ok {
			t.Fatalf("plain JSON lost key %q", key)
		}
	}
}

func TestFixHumanOutputMentionsRepairs(t *testing.T) {
	paths, service := newFixFixture(t)
	plantSemgrepInstall(t, service, "1.0.0")

	var output strings.Builder
	runFix(t, paths, service, true).WriteHuman(&output)
	text := output.String()
	if !strings.Contains(text, "Semgrep: OK") {
		t.Fatalf("semgrep not repaired in human output: %q", text)
	}
	if !strings.Contains(text, "fixed: re-extracted semgrep rules ("+tools.SemgrepRulesVersion+")") {
		t.Fatalf("repair phrase missing: %q", text)
	}
	if !strings.Contains(text, "Repairs: OK") {
		t.Fatalf("repairs summary missing: %q", text)
	}
}

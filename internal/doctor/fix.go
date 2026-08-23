package doctor

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	analyzerssemgrep "bluntcode/internal/analyzers/semgrep"
	"bluntcode/internal/config"
	"bluntcode/internal/instance"
	"bluntcode/internal/tools"
)

// Repair states reported per check by `bluntcode doctor --fix`.
const (
	// RepairFixed marks a repair that was applied and resolved the finding.
	RepairFixed = "fixed"
	// RepairFailed marks a repair that was attempted but did not resolve it.
	RepairFailed = "failed"
	// RepairSkipped marks a repair that was refused (data directory in use
	// or unusable); no state was changed.
	RepairSkipped = "skipped"
	// RepairManual marks a finding reported for manual cleanup because
	// deleting it automatically would not be safe.
	RepairManual = "manual"
	// RepairReinstall marks a finding doctor cannot repair offline.
	RepairReinstall = "reinstall"
)

// reinstallAction explains the only supported way to restore a missing
// managed tool. Doctor never downloads (it may run offline), but every scan
// calls tools.Service.Ensure before running an analyzer, so the next online
// scan reinstalls the pinned artifact automatically.
const reinstallAction = "doctor never downloads; the next online scan reinstalls this managed tool automatically (`bluntcode scan <path>`)"

// Repair records what the `--fix` pass did about one check. It is only ever
// set when fixing is enabled, so plain diagnostics serialize byte-for-byte as
// before (the JSON field is omitted entirely).
type Repair struct {
	State  string `json:"state"`
	Action string `json:"action,omitempty"`
}

// phrase renders the repair for the human report in doctor's established
// voice ("fixed: re-extracted semgrep rules (2.0.0)").
func (r Repair) phrase() string {
	prefix := ""
	switch r.State {
	case RepairFixed:
		prefix = "fixed"
	case RepairFailed:
		prefix = "repair failed"
	case RepairSkipped:
		prefix = "skipped"
	case RepairManual:
		prefix = "manual cleanup"
	case RepairReinstall:
		prefix = "reinstall required"
	}
	if r.Action == "" {
		return prefix
	}
	return prefix + ": " + r.Action
}

// applyRepairs runs after diagnosis when Options.Fix is set. Every repair is
// local, mechanical, and idempotent, and doctor still never downloads or
// starts anything. Repairs that change state require exclusive access to the
// data directory, so the pass first acquires the same single-instance lock
// the app and scan commands use; while another Blunt Code process holds it,
// every repair is refused and reported as skipped instead.
func applyRepairs(options Options, result *Result) {
	if options.Paths.DataDir == "" || options.Paths.TempDir == "" {
		refuseRepairs(result, "application paths are not configured")
		return
	}
	guard, err := instance.Acquire(options.Paths.DataDir)
	if err != nil {
		reason := "another Blunt Code process (app or scan) is using the data directory; close it and re-run doctor --fix"
		if !errors.Is(err, instance.ErrAlreadyRunning) {
			reason = "data-directory lock is unavailable: " + err.Error()
		}
		refuseRepairs(result, reason)
		return
	}
	defer guard.Close()
	repairDataDirectory(options.Paths, result.Checks)
	repairToolChecks(options.Tools, result.Checks)
	result.add(stagingLeftoversCheck(sweepStagingFiles(options.Paths), options.Paths.DataDir))
	result.add(orphanedVersionsCheck(options.Tools, options.Paths))
	result.add(repairsSummary(result.Checks))
}

// refuseRepairs reports every pending repair as skipped without touching the
// filesystem, so a locked data directory can never end up half-repaired.
func refuseRepairs(result *Result, reason string) {
	for i := range result.Checks {
		if result.Checks[i].Status == StatusOK || !repairableName(result.Checks[i].Name) {
			continue
		}
		skipped := Repair{State: RepairSkipped, Action: reason}
		result.Checks[i].Repair = &skipped
	}
	skippedRow := func(name string) Check {
		return Check{Name: name, Status: StatusSkip, Detail: "not inspected", Hard: false, Repair: &Repair{State: RepairSkipped, Action: reason}}
	}
	result.add(skippedRow("Install staging leftovers"))
	result.add(skippedRow("Orphaned tool versions"))
	result.add(Check{Name: "Repairs", Status: StatusWarn, Detail: "no repairs applied; " + reason, Hard: false})
}

// repairableName lists the checks the fix pass knows how to act on; every
// other check (disk space, database, loopback) is diagnosis-only because no
// mechanical local repair exists for it.
func repairableName(name string) bool {
	switch name {
	case "Data directory", "Semgrep", "Ruff", "Biome", "SonarQube installation", "SonarScanner", "Managed Java runtime":
		return true
	}
	return false
}

// repairDataDirectory recreates the directories the app expects and retests
// writability. ReportsDir is deliberately not recreated: config.NewPaths
// keeps it lazy so a fresh install never carries an empty reports folder.
func repairDataDirectory(paths config.Paths, checks []Check) {
	for i := range checks {
		if checks[i].Name != "Data directory" || checks[i].Status == StatusOK {
			continue
		}
		if err := recreateExpectedDirectories(paths); err != nil {
			checks[i].Repair = &Repair{State: RepairFailed, Action: "recreating data directories failed: " + err.Error()}
			continue
		}
		recheck := dataDirectoryCheck(paths)
		if recheck.Status == StatusOK {
			checks[i].Status = StatusOK
			checks[i].Detail = recheck.Detail
			checks[i].Repair = &Repair{State: RepairFixed, Action: "recreated missing data directories"}
			continue
		}
		checks[i].Detail = recheck.Detail
		checks[i].Repair = &Repair{State: RepairFailed, Action: "recreated the expected directories, but the data directory is still not writable"}
	}
}

func recreateExpectedDirectories(paths config.Paths) error {
	for _, dir := range []string{paths.DataDir, paths.LogsDir, paths.TempDir, paths.ToolsDir} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// repairToolChecks repairs the managed-tool rows. Only the bundled Semgrep
// rules are repairable offline (they ship inside the binary); a missing
// managed binary needs a network reinstall, which is left to the next scan.
func repairToolChecks(service *tools.Service, checks []Check) {
	if service == nil {
		return
	}
	for i := range checks {
		check := &checks[i]
		if check.Status == StatusOK {
			continue
		}
		switch check.Name {
		case "Semgrep":
			repairSemgrepRules(service, check)
		case "Ruff", "Biome":
			if status := service.Status(strings.ToLower(check.Name)); !status.Ready && status.CanInstall {
				check.Repair = &Repair{State: RepairReinstall, Action: reinstallAction}
			}
		case "SonarQube installation":
			if status := service.Status("sonarqube"); !status.Ready && status.CanInstall {
				check.Repair = &Repair{State: RepairReinstall, Action: reinstallAction}
			}
		case "SonarScanner", "Managed Java runtime":
			if _, ok := service.SonarQubeArtifacts(); ok {
				check.Repair = &Repair{State: RepairReinstall, Action: reinstallAction}
			}
		}
	}
}

// repairSemgrepRules re-extracts the bundled rulepack exactly like the tools
// service does during an offline scan. It only runs when the managed Semgrep
// executable itself is present: rules for a tool that is not installed are
// restored by the install the next scan performs anyway.
func repairSemgrepRules(service *tools.Service, check *Check) {
	paths, ok := service.SemgrepPaths()
	if !ok {
		return
	}
	if _, err := os.Stat(paths.Executable); err != nil {
		check.Repair = &Repair{State: RepairReinstall, Action: reinstallAction}
		return
	}
	if semgrepRulesCurrent(paths.RulesDir) {
		return
	}
	if err := tools.ExtractSemgrepRules(paths.RulesDir); err != nil {
		check.Repair = &Repair{State: RepairFailed, Action: "re-extracting the bundled rules failed: " + err.Error()}
		return
	}
	status := service.Status("semgrep")
	if !status.Ready {
		check.Repair = &Repair{State: RepairFailed, Action: "re-extracted the bundled rules, but Semgrep still reports: " + strings.TrimSpace(status.Detail)}
		return
	}
	check.Status = StatusOK
	check.Detail = toolDetail(status)
	check.Repair = &Repair{State: RepairFixed, Action: "re-extracted semgrep rules (" + tools.SemgrepRulesVersion + ")"}
}

// semgrepRulesCurrent mirrors the tools service's private rules verification
// (a current RULES_VERSION marker plus the bundled rules file) so doctor only
// repairs a rules directory the service itself would reject.
func semgrepRulesCurrent(rulesDir string) bool {
	version, err := os.ReadFile(filepath.Join(rulesDir, "RULES_VERSION"))
	if err != nil || string(version) != tools.SemgrepRulesVersion+"\n" {
		return false
	}
	_, err = os.Stat(filepath.Join(rulesDir, analyzerssemgrep.RulesFileName))
	return err == nil
}

// stagingSweep collects and removes interrupted-install leftovers. Names
// follow the installer exactly: staged files end in `.new` (writePrivateFile
// and Manager.InstallExecutable), zip extraction stages are
// `<version>.new-<digits>` directories (Manager.installZIP), completed swaps
// leave `<version>.previous` backups (replaceDirectory), partial downloads
// are CreateTemp files inside the tools-root `.downloads` folder
// (Manager.Download), and Sonar scanner scratch lives in `<data>/tmp/sonar-*`.
type stagingSweep struct {
	removed []string
	manual  []string
	failed  []string
}

func sweepStagingFiles(paths config.Paths) stagingSweep {
	var sweep stagingSweep
	var remove []string
	if paths.ToolsDir != "" {
		_ = filepath.WalkDir(paths.ToolsDir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || entry == nil || path == paths.ToolsDir {
				return nil // unreadable subtrees are skipped, never fatal
			}
			name := entry.Name()
			if entry.IsDir() && name == ".downloads" && filepath.Dir(path) == paths.ToolsDir {
				// Only Manager.Download ever writes here, and it removes its
				// CreateTemp partial on success; under the instance lock
				// every remaining file is an interrupted download.
				if files, err := os.ReadDir(path); err == nil {
					for _, file := range files {
						if !file.IsDir() {
							remove = append(remove, filepath.Join(path, file.Name()))
						}
					}
				}
				return fs.SkipDir
			}
			if strings.HasSuffix(name, ".new") {
				remove = append(remove, path)
				if entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if entry.IsDir() && stagedExtractionDir(name) {
				remove = append(remove, path)
				return fs.SkipDir
			}
			if entry.IsDir() && strings.HasSuffix(name, ".previous") {
				// The installer deletes the backup once the swap completes,
				// so a leftover next to an existing final directory is
				// stale. Without the final directory the backup may be the
				// only copy, so it is kept and reported instead.
				final := filepath.Join(filepath.Dir(path), strings.TrimSuffix(name, ".previous"))
				if _, err := os.Stat(final); err == nil {
					remove = append(remove, path)
				} else {
					sweep.manual = append(sweep.manual, path)
				}
				return fs.SkipDir
			}
			return nil
		})
	}
	// The Sonar adapter removes its scanner scratch directory after every
	// run, so leftovers under <data>/tmp belong to an interrupted scan.
	if paths.DataDir != "" {
		if entries, err := os.ReadDir(filepath.Join(paths.DataDir, "tmp")); err == nil {
			for _, entry := range entries {
				if entry.IsDir() && strings.HasPrefix(entry.Name(), "sonar-") {
					remove = append(remove, filepath.Join(paths.DataDir, "tmp", entry.Name()))
				}
			}
		}
	}
	for _, path := range remove {
		if err := os.RemoveAll(path); err != nil {
			sweep.failed = append(sweep.failed, path+" ("+err.Error()+")")
			continue
		}
		sweep.removed = append(sweep.removed, path)
	}
	sort.Strings(sweep.removed)
	sort.Strings(sweep.manual)
	return sweep
}

// stagedExtractionDir matches the `<version>.new-<digits>` directories the
// zip installer creates with os.MkdirTemp.
func stagedExtractionDir(name string) bool {
	index := strings.LastIndex(name, ".new-")
	if index <= 0 {
		return false
	}
	suffix := name[index+len(".new-"):]
	if suffix == "" {
		return false
	}
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// isStagingEntryName identifies installer staging names so the orphan scan
// never mistakes a (possibly kept) staging entry for an abandoned version.
func isStagingEntryName(name string) bool {
	return name == ".downloads" || strings.HasSuffix(name, ".new") || strings.HasSuffix(name, ".previous") || stagedExtractionDir(name)
}

func stagingLeftoversCheck(sweep stagingSweep, dataDir string) Check {
	check := Check{Name: "Install staging leftovers", Hard: false}
	var details []string
	if len(sweep.removed) > 0 {
		details = append(details, "removed "+plural(len(sweep.removed), "leftover", "leftovers")+": "+relativeNames(dataDir, sweep.removed))
	}
	if len(sweep.failed) > 0 {
		details = append(details, "could not remove "+plural(len(sweep.failed), "leftover", "leftovers")+": "+strings.Join(sweep.failed, ", "))
	}
	if len(sweep.manual) > 0 {
		details = append(details, "kept unowned "+plural(len(sweep.manual), "backup", "backups")+" for manual cleanup: "+relativeNames(dataDir, sweep.manual))
	}
	switch {
	case len(sweep.failed) > 0:
		check.Status = StatusWarn
		check.Repair = &Repair{State: RepairFailed, Action: "removal failed; close any program locking the tools directory and re-run doctor --fix"}
	case len(sweep.removed) > 0:
		check.Status = StatusOK
		check.Repair = &Repair{State: RepairFixed, Action: "deleted " + plural(len(sweep.removed), "interrupted-install leftover", "interrupted-install leftovers")}
	case len(sweep.manual) > 0:
		check.Status = StatusOK
		check.Repair = &Repair{State: RepairManual, Action: "delete the kept backup directories manually once their tool version is not needed"}
	default:
		check.Status = StatusOK
		details = []string{"none found"}
	}
	check.Detail = strings.Join(details, "; ")
	return check
}

// orphanedVersionsCheck reports tool version directories no manifest
// artifact references anymore. Deleting them is deliberately left to the
// user: doctor has no journal of completed installs, so "unreferenced" is a
// strong hint, not proof of abandonment.
func orphanedVersionsCheck(service *tools.Service, paths config.Paths) Check {
	check := Check{Name: "Orphaned tool versions", Hard: false}
	if service == nil {
		check.Status = StatusSkip
		check.Detail = "tool manifest is unavailable; not inspected"
		return check
	}
	orphans := findOrphanedToolVersions(service, paths.ToolsDir)
	if len(orphans) == 0 {
		check.Status = StatusOK
		check.Detail = "none found"
		return check
	}
	check.Status = StatusWarn
	check.Detail = plural(len(orphans), "directory", "directories") + " not referenced by the pinned manifest: " + relativeNames(paths.DataDir, orphans)
	check.Repair = &Repair{State: RepairManual, Action: "delete these directories manually to reclaim space; doctor will not remove unreferenced tool versions"}
	return check
}

func findOrphanedToolVersions(service *tools.Service, toolsDir string) []string {
	if toolsDir == "" {
		return nil
	}
	referenced := make(map[string]map[string]bool)
	for _, artifact := range service.Manager.Manifest.Artifacts {
		versions, ok := referenced[artifact.ToolID]
		if !ok {
			versions = make(map[string]bool)
			referenced[artifact.ToolID] = versions
		}
		versions[artifact.Version] = true
	}
	toolDirs, err := os.ReadDir(toolsDir)
	if err != nil {
		return nil
	}
	var orphans []string
	for _, toolDir := range toolDirs {
		if !toolDir.IsDir() || toolDir.Name() == ".downloads" {
			continue
		}
		path := filepath.Join(toolsDir, toolDir.Name())
		versions, known := referenced[toolDir.Name()]
		if !known {
			orphans = append(orphans, path)
			continue
		}
		children, err := os.ReadDir(path)
		if err != nil {
			continue
		}
		for _, child := range children {
			if child.IsDir() && !isStagingEntryName(child.Name()) && !versions[child.Name()] {
				orphans = append(orphans, filepath.Join(path, child.Name()))
			}
		}
	}
	sort.Strings(orphans)
	return orphans
}

// repairsSummary closes the --fix report with one line describing what the
// pass did (or why it refused to act).
func repairsSummary(checks []Check) Check {
	counts := make(map[string]int)
	for _, check := range checks {
		if check.Repair != nil {
			counts[check.Repair.State]++
		}
	}
	check := Check{Name: "Repairs", Hard: false}
	var parts []string
	if n := counts[RepairFixed]; n > 0 {
		parts = append(parts, plural(n, "repair", "repairs")+" applied")
	}
	if n := counts[RepairManual]; n > 0 {
		parts = append(parts, plural(n, "item", "items")+" left for manual cleanup")
	}
	if n := counts[RepairReinstall]; n > 0 {
		parts = append(parts, plural(n, "managed tool", "managed tools")+" awaiting reinstall on the next online scan")
	}
	if n := counts[RepairFailed]; n > 0 {
		parts = append(parts, plural(n, "repair", "repairs")+" failed")
	}
	if len(parts) == 0 {
		check.Status = StatusOK
		check.Detail = "nothing to fix"
		return check
	}
	check.Status = StatusOK
	if counts[RepairFailed] > 0 {
		check.Status = StatusWarn
	}
	check.Detail = strings.Join(parts, "; ")
	return check
}

func plural(count int, singular, pluralWord string) string {
	if count == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(count) + " " + pluralWord
}

// relativeNames shortens absolute paths under base for compact output.
func relativeNames(base string, paths []string) string {
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		rel, err := filepath.Rel(base, path)
		if err != nil {
			rel = path
		}
		names = append(names, filepath.ToSlash(rel))
	}
	return strings.Join(names, ", ")
}

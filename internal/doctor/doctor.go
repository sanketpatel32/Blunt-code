// Package doctor implements local-only command-line diagnostics.
package doctor

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"bluntcode/internal/config"
	"bluntcode/internal/database"
	"bluntcode/internal/tools"
)

type Status string

const (
	StatusOK   Status = "ok"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

type Check struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail,omitempty"`
	Hard   bool   `json:"hard"`
	// Repair records what the `--fix` pass did about this check. It is only
	// ever set when fixing is enabled, so plain diagnostics (human and JSON)
	// serialize exactly as before. See fix.go.
	Repair *Repair `json:"repair,omitempty"`
}

type Result struct {
	Version string  `json:"version"`
	OK      bool    `json:"ok"`
	Checks  []Check `json:"checks"`
}

type Options struct {
	Version string
	Paths   config.Paths
	Tools   *tools.Service
	// Fix enables the repair pass that follows diagnosis. Repairs are local
	// and mechanical only (missing directories, stale bundled Semgrep rules,
	// interrupted-install leftovers): doctor still never downloads, installs,
	// or starts anything, and repairs are refused while another Blunt Code
	// process holds the data-directory lock.
	Fix bool
}

// Run only reads installed-tool state and performs local filesystem, SQLite,
// and loopback bind checks. It does not download, install, start analyzers, or
// send a request anywhere. With Fix set, a repair pass follows diagnosis; it
// changes only local files under the single-instance data-directory lock and
// keeps the same no-network guarantee.
func Run(ctx context.Context, options Options) Result {
	result := Result{Version: options.Version}
	result.add(Check{Name: "Blunt Code", Status: StatusOK, Detail: options.Version, Hard: true})
	result.add(dataDirectoryCheck(options.Paths))
	result.add(diskSpaceCheck(options.Paths))
	result.add(databaseCheck(ctx, options.Paths))
	result.add(loopbackCheck())
	result.addTools(options.Tools)
	if options.Fix {
		applyRepairs(options, &result)
	}
	result.OK = true
	for _, check := range result.Checks {
		if check.Hard && check.Status == StatusFail {
			result.OK = false
		}
	}
	return result
}

func diskSpaceCheck(paths config.Paths) Check {
	free, err := freeSpace(paths.DataDir)
	if err != nil {
		return Check{Name: "Disk space", Status: StatusWarn, Detail: "could not determine free space: " + err.Error(), Hard: false}
	}
	const minimum = uint64(2 << 30)
	detail := fmt.Sprintf("%.1f GB free", float64(free)/(1<<30))
	if free < minimum {
		return Check{Name: "Disk space", Status: StatusWarn, Detail: detail + "; at least 2 GB is recommended for managed tools", Hard: false}
	}
	return Check{Name: "Disk space", Status: StatusOK, Detail: detail, Hard: false}
}

func (r *Result) add(values ...Check) { r.Checks = append(r.Checks, values...) }

func dataDirectoryCheck(paths config.Paths) Check {
	if paths.DataDir == "" || paths.TempDir == "" {
		return Check{Name: "Data directory", Status: StatusFail, Detail: "application paths are not configured", Hard: true}
	}
	file, err := os.CreateTemp(paths.TempDir, "doctor-*")
	if err != nil {
		return Check{Name: "Data directory", Status: StatusFail, Detail: "not writable: " + err.Error(), Hard: true}
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.WriteString("blunt-code-doctor\n"); err != nil {
		_ = file.Close()
		return Check{Name: "Data directory", Status: StatusFail, Detail: "write test failed: " + err.Error(), Hard: true}
	}
	if err := file.Close(); err != nil {
		return Check{Name: "Data directory", Status: StatusFail, Detail: "write test close failed: " + err.Error(), Hard: true}
	}
	return Check{Name: "Data directory", Status: StatusOK, Detail: paths.DataDir, Hard: true}
}

func databaseCheck(ctx context.Context, paths config.Paths) Check {
	if paths.DBPath == "" {
		return Check{Name: "Database and migrations", Status: StatusFail, Detail: "database path is not configured", Hard: true}
	}
	db, err := database.Open(ctx, paths.DBPath)
	if err != nil {
		return Check{Name: "Database and migrations", Status: StatusFail, Detail: err.Error(), Hard: true}
	}
	defer db.Close()
	var migrationCount int
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		return Check{Name: "Database and migrations", Status: StatusFail, Detail: err.Error(), Hard: true}
	}
	return Check{Name: "Database and migrations", Status: StatusOK, Detail: fmt.Sprintf("%d migration(s) available", migrationCount), Hard: true}
}

func loopbackCheck() Check {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return Check{Name: "Loopback port", Status: StatusFail, Detail: err.Error(), Hard: true}
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		return Check{Name: "Loopback port", Status: StatusFail, Detail: err.Error(), Hard: true}
	}
	return Check{Name: "Loopback port", Status: StatusOK, Detail: address, Hard: true}
}

func (r *Result) addTools(service *tools.Service) {
	if service == nil {
		r.add(Check{Name: "Managed tools", Status: StatusWarn, Detail: "tool manifest is unavailable", Hard: false})
		return
	}
	byID := make(map[string]tools.Status)
	for _, status := range service.All() {
		byID[status.ID] = status
	}
	for _, item := range []struct{ id, label string }{{"ruff", "Ruff"}, {"biome", "Biome"}, {"semgrep", "Semgrep"}} {
		r.add(toolCheck(item.label, byID[item.id]))
	}
	r.add(sonarChecks(service, byID["sonarqube"])...)
}

func toolCheck(name string, status tools.Status) Check {
	if status.Ready {
		return Check{Name: name, Status: StatusOK, Detail: toolDetail(status), Hard: false}
	}
	return Check{Name: name, Status: StatusWarn, Detail: toolDetail(status), Hard: false}
}

func sonarChecks(service *tools.Service, status tools.Status) []Check {
	installation := toolCheck("SonarQube installation", status)
	if status.Ready {
		installation.Detail = "server, scanner, and Java are installed"
	}
	checks := []Check{installation}
	artifacts, ok := service.SonarQubeArtifacts()
	if !ok {
		return append(checks,
			Check{Name: "SonarScanner", Status: StatusWarn, Detail: "pinned artifact is unavailable", Hard: false},
			Check{Name: "Managed Java runtime", Status: StatusWarn, Detail: "pinned artifact is unavailable", Hard: false},
			Check{Name: "SonarQube startup", Status: StatusSkip, Detail: "not started by a no-network diagnostic", Hard: false},
		)
	}
	ready := make(map[string]bool, len(artifacts))
	for _, artifact := range artifacts {
		ready[artifact.ToolID] = service.Manager.IsReady(artifact)
	}
	checks = append(checks,
		artifactCheck("SonarScanner", ready["sonar-scanner"]),
		artifactCheck("Managed Java runtime", ready["java"]),
		Check{Name: "SonarQube startup", Status: StatusSkip, Detail: "not started by a no-network diagnostic", Hard: false},
	)
	return checks
}

func artifactCheck(name string, ready bool) Check {
	if ready {
		return Check{Name: name, Status: StatusOK, Detail: "ready", Hard: false}
	}
	return Check{Name: name, Status: StatusWarn, Detail: "pinned managed artifact is not installed", Hard: false}
}

func toolDetail(status tools.Status) string {
	detail := strings.TrimSpace(status.Detail)
	if status.Version != "" {
		if detail == "" {
			return "version " + status.Version
		}
		return "version " + status.Version + "; " + detail
	}
	return detail
}

func (r Result) ExitCode() int {
	if r.OK {
		return 0
	}
	return 1
}

func (r Result) WriteHuman(w io.Writer) {
	state := "OK"
	if !r.OK {
		state = "FAIL"
	}
	fmt.Fprintf(w, "Blunt Code: %s\n", state)
	for _, check := range r.Checks {
		if check.Name == "Blunt Code" {
			continue
		}
		line := fmt.Sprintf("%s: %s", check.Name, strings.ToUpper(string(check.Status)))
		if check.Detail != "" {
			line += " — " + check.Detail
		}
		if check.Repair != nil {
			line += "; " + check.Repair.phrase()
		}
		fmt.Fprintln(w, line)
	}
}

// DatabasePath is intentionally exposed only for focused tests and never
// printed by the JSON protocol beyond the configured data-directory check.
func DatabasePath(paths config.Paths) string { return filepath.Clean(paths.DBPath) }

package main

// `bluntcode config` prints the effective, read-only configuration report:
// resolved paths, environment overrides currently in effect, persisted app
// settings, and managed-tool install state. The command never creates
// directories, never acquires the single-instance data-directory lock, and
// never writes settings, so it is safe to run alongside a live Blunt Code app
// — the same property `doctor` diagnostics have (doctor takes the lock only
// for --fix).

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bluntcode/internal/config"
	"bluntcode/internal/database"
	"bluntcode/internal/tools"
)

const configUsage = "usage: bluntcode config [--json]"

// sonarStartupTimeoutEnv mirrors the override parsed by
// internal/analyzers/sonarqube: unparseable or non-positive values are
// ignored with a warning, so the built-in default (10m for the managed
// server) applies.
const sonarStartupTimeoutEnv = "BLUNTCODE_SONAR_STARTUP_TIMEOUT"

// configFlags is the validated command line of `bluntcode config`.
type configFlags struct {
	json bool
}

// parseConfigFlags parses the `bluntcode config` command line, mirroring
// parseDoctorFlags: --json is a boolean flag, parse failures and any
// positional argument are usage errors.
func parseConfigFlags(args []string, errOut io.Writer) (configFlags, error) {
	flags := flag.NewFlagSet("config", flag.ContinueOnError)
	flags.SetOutput(errOut)
	var cfg configFlags
	flags.BoolVar(&cfg.json, "json", false, "write the effective configuration as JSON")
	if err := flags.Parse(args); err != nil {
		return configFlags{}, err
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(errOut, configUsage)
		return configFlags{}, fmt.Errorf("config takes no positional arguments")
	}
	return cfg, nil
}

func runConfig(args []string) {
	cfg, err := parseConfigFlags(args, os.Stderr)
	if err != nil {
		os.Exit(2)
	}
	paths, err := resolveReportPaths(os.Getenv)
	if err != nil {
		fatal(err)
	}
	manifest, manifestErr := tools.DefaultManifest()
	report := buildConfigReport(context.Background(), configReportInputs{
		Version:    version,
		Paths:      paths,
		Getenv:     os.Getenv,
		Manifest:   manifest,
		ManifestOK: manifestErr == nil,
		OpenDB:     database.Open,
	})
	if cfg.json {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fatal(fmt.Errorf("write configuration report: %w", err))
		}
		return
	}
	report.WriteHuman(os.Stdout)
}

// resolveReportPaths resolves exactly the paths config.DefaultPaths resolves —
// LOCALAPPDATA (then the user cache directory) plus the Blunt Code data
// directory — but never creates them. A read-only report must describe a
// machine where Blunt Code has never run without materializing directories;
// the layout half is kept identical to internal/config by test.
func resolveReportPaths(getenv func(string) string) (config.Paths, error) {
	base := getenv("LOCALAPPDATA")
	if base == "" {
		userCache, err := os.UserCacheDir()
		if err != nil {
			return config.Paths{}, fmt.Errorf("resolve local application data directory: %w", err)
		}
		base = userCache
	}
	return layoutPaths(filepath.Join(base, config.AppName))
}

// layoutPaths is config.NewPaths minus the MkdirAll pass.
func layoutPaths(dataDir string) (config.Paths, error) {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return config.Paths{}, fmt.Errorf("normalize data directory: %w", err)
	}
	p := config.Paths{DataDir: filepath.Clean(abs)}
	p.DBPath = filepath.Join(p.DataDir, "bluntcode.db")
	p.LogsDir = filepath.Join(p.DataDir, "logs")
	p.TempDir = filepath.Join(p.DataDir, "temp")
	p.ToolsDir = filepath.Join(p.DataDir, "tools")
	p.ReportsDir = filepath.Join(p.DataDir, "reports")
	return p, nil
}

// pathInfo is one resolved path plus a stat-based existence flag.
type pathInfo struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

func newPathInfo(path string) pathInfo {
	_, err := os.Stat(path)
	return pathInfo{Path: path, Exists: err == nil}
}

// envVarInfo describes one environment override currently in effect. Valid is
// false with a Note when the value is set but the consumer will warn and fall
// back to its built-in default.
type envVarInfo struct {
	Name         string  `json:"name"`
	Set          bool    `json:"set"`
	Valid        bool    `json:"valid"`
	ValueSeconds float64 `json:"value_seconds"`
	RawValue     string  `json:"raw_value,omitempty"`
	Note         string  `json:"note,omitempty"`
}

// sonarStartupTimeoutOverride reads BLUNTCODE_SONAR_STARTUP_TIMEOUT with the
// same acceptance rules as sonarqube.startupTimeoutFromEnv: trimmed-empty is
// unset, and a value that does not parse as a positive Go duration is invalid
// (the analyzer warns and keeps its default).
func sonarStartupTimeoutOverride(getenv func(string) string) envVarInfo {
	info := envVarInfo{Name: sonarStartupTimeoutEnv}
	raw := strings.TrimSpace(getenv(sonarStartupTimeoutEnv))
	if raw == "" {
		info.Note = "not set; managed server default 10m applies"
		return info
	}
	info.Set = true
	info.RawValue = raw
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		info.Note = "invalid (will warn and use default)"
		return info
	}
	info.Valid = true
	info.ValueSeconds = value.Seconds()
	return info
}

// appSettingsInfo reports the persisted app settings: straight from the
// database when readable, the effective defaults when no database exists yet,
// or an unavailable marker.
type appSettingsInfo struct {
	Available   bool   `json:"available"`
	Source      string `json:"source"` // database, defaults, or unavailable
	Offline     bool   `json:"offline"`
	OpenBrowser bool   `json:"open_browser"`
	Note        string `json:"note,omitempty"`
}

// dbOpener opens the settings database. It is injected so tests can simulate
// the database being unreadable while the app is running.
type dbOpener func(ctx context.Context, path string) (*database.DB, error)

// appSettingsReport reads the persisted app settings WITHOUT acquiring the
// single-instance data-directory lock, exactly like doctor's database check:
// SQLite's busy timeout absorbs a running app's brief write locks, so a
// read-only consumer never needs the lock. The database file is stat-ed
// first so a fresh install is never created by a read-only report. If
// opening still fails, the only realistic cause is the running app holding
// SQLite past the busy timeout, so that is what the report says.
func appSettingsReport(ctx context.Context, dbPath string, open dbOpener) appSettingsInfo {
	if _, err := os.Stat(dbPath); err != nil {
		// Matches database.AppSettings' no-row default: existing installs keep
		// the historical launch behavior of opening a browser.
		return appSettingsInfo{
			Source:      "defaults",
			OpenBrowser: true,
			Note:        "database not created yet; effective defaults shown",
		}
	}
	db, err := open(ctx, dbPath)
	if err != nil {
		return appSettingsInfo{Source: "unavailable", Note: "app is running"}
	}
	defer db.Close()
	settings, err := db.AppSettings(ctx)
	if err != nil {
		return appSettingsInfo{Source: "unavailable", Note: "app is running"}
	}
	return appSettingsInfo{Available: true, Source: "database", Offline: settings.Offline, OpenBrowser: settings.OpenBrowser}
}

// toolInfo is one managed tool's pinned version and whether that version is
// installed on disk.
type toolInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Installed bool   `json:"installed"`
}

type managedToolsInfo struct {
	Available bool       `json:"available"`
	Note      string     `json:"note,omitempty"`
	Tools     []toolInfo `json:"tools"`
}

// managedToolsReport summarizes installed state per managed tool id by reusing
// the tools service over the embedded manifest, exactly like doctor: every
// probe is a stat against the pinned tools-dir layout
// (tools/<id>/<version>/<executable>), never a download or install.
func managedToolsReport(root string, manifest tools.Manifest, ok bool) managedToolsInfo {
	if !ok {
		return managedToolsInfo{Note: "embedded tool manifest could not be loaded"}
	}
	statuses := tools.NewService(root, manifest, true).All()
	info := managedToolsInfo{Available: true, Tools: make([]toolInfo, 0, len(statuses))}
	for _, status := range statuses {
		info.Tools = append(info.Tools, toolInfo{
			ID:        status.ID,
			Name:      status.Name,
			Version:   status.Version,
			Installed: status.Ready,
		})
	}
	return info
}

// configReportPaths is the fixed set of resolved paths the report covers.
type configReportPaths struct {
	DataDir  pathInfo `json:"data_dir"`
	Database pathInfo `json:"database"`
	ToolsDir pathInfo `json:"tools_dir"`
	Reports  pathInfo `json:"reports_dir"`
	Logs     pathInfo `json:"logs_dir"`
	Temp     pathInfo `json:"temp_dir"`
}

type configEnvironment struct {
	SonarStartupTimeout envVarInfo `json:"sonar_startup_timeout"`
}

// configReport is the whole effective-configuration report. Every section is
// a fixed struct field (never a map), so the JSON key set and order are
// stable for scripts.
type configReport struct {
	Version      string            `json:"version"`
	Paths        configReportPaths `json:"paths"`
	Environment  configEnvironment `json:"environment"`
	AppSettings  appSettingsInfo   `json:"app_settings"`
	ManagedTools managedToolsInfo  `json:"managed_tools"`
}

// configReportInputs carries the injectable environment of a report build:
// the environment lookup and database opener are os.Getenv and database.Open
// in production and fakes in tests.
type configReportInputs struct {
	Version    string
	Paths      config.Paths
	Getenv     func(string) string
	Manifest   tools.Manifest
	ManifestOK bool
	OpenDB     dbOpener
}

func buildConfigReport(ctx context.Context, in configReportInputs) configReport {
	if in.Getenv == nil {
		in.Getenv = os.Getenv
	}
	if in.OpenDB == nil {
		in.OpenDB = database.Open
	}
	return configReport{
		Version: in.Version,
		Paths: configReportPaths{
			DataDir:  newPathInfo(in.Paths.DataDir),
			Database: newPathInfo(in.Paths.DBPath),
			ToolsDir: newPathInfo(in.Paths.ToolsDir),
			Reports:  newPathInfo(in.Paths.ReportsDir),
			Logs:     newPathInfo(in.Paths.LogsDir),
			Temp:     newPathInfo(in.Paths.TempDir),
		},
		Environment:  configEnvironment{SonarStartupTimeout: sonarStartupTimeoutOverride(in.Getenv)},
		AppSettings:  appSettingsReport(ctx, in.Paths.DBPath, in.OpenDB),
		ManagedTools: managedToolsReport(in.Paths.ToolsDir, in.Manifest, in.ManifestOK),
	}
}

// WriteHuman renders the report for people; the JSON encoder renders the same
// data for scripts.
func (r configReport) WriteHuman(w io.Writer) {
	fmt.Fprintf(w, "Blunt Code %s\n", r.Version)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Paths:")
	for _, row := range []struct {
		label string
		info  pathInfo
	}{
		{"data directory", r.Paths.DataDir},
		{"database", r.Paths.Database},
		{"tools directory", r.Paths.ToolsDir},
		{"reports directory", r.Paths.Reports},
		{"logs directory", r.Paths.Logs},
		{"temp directory", r.Paths.Temp},
	} {
		fmt.Fprintf(w, "  %s: %s (%s)\n", row.label, row.info.Path, existence(row.info.Exists))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Environment overrides:")
	fmt.Fprintf(w, "  %s: %s\n", r.Environment.SonarStartupTimeout.Name, r.Environment.SonarStartupTimeout.human())
	fmt.Fprintln(w)
	r.AppSettings.writeHuman(w)
	fmt.Fprintln(w)
	r.ManagedTools.writeHuman(w)
}

func existence(exists bool) string {
	if exists {
		return "exists"
	}
	return "missing"
}

func (e envVarInfo) human() string {
	switch {
	case !e.Set:
		return "not set"
	case e.Valid:
		parsed := time.Duration(e.ValueSeconds * float64(time.Second)).Round(time.Millisecond)
		return fmt.Sprintf("%s (parsed duration %s)", e.RawValue, parsed)
	default:
		return fmt.Sprintf("%q is invalid (will warn and use default)", e.RawValue)
	}
}

func (a appSettingsInfo) writeHuman(w io.Writer) {
	if a.Source == "unavailable" {
		// The spec'd support line: reading never needs the instance lock, so
		// this only happens when the running app holds SQLite past the busy
		// timeout. The unknown values are deliberately not guessed.
		fmt.Fprintf(w, "app settings: unavailable (%s)\n", a.Note)
		return
	}
	if !a.Available {
		fmt.Fprintln(w, "App settings (defaults; database not created yet):")
	} else {
		fmt.Fprintln(w, "App settings:")
	}
	fmt.Fprintf(w, "  offline mode: %s\n", onOff(a.Offline))
	fmt.Fprintf(w, "  open browser on start: %s\n", onOff(a.OpenBrowser))
}

func (m managedToolsInfo) writeHuman(w io.Writer) {
	fmt.Fprintln(w, "Managed tools:")
	if !m.Available {
		fmt.Fprintf(w, "  unavailable (%s)\n", m.Note)
		return
	}
	for _, tool := range m.Tools {
		state := "not installed"
		if tool.Installed {
			state = "installed"
		}
		version := tool.Version
		if version == "" {
			version = "unpinned"
		}
		fmt.Fprintf(w, "  %s %s: %s\n", tool.Name, version, state)
	}
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

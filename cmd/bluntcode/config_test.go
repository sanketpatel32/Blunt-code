package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"bluntcode/internal/config"
	"bluntcode/internal/database"
	"bluntcode/internal/tools"
)

func fakeEnv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestParseConfigFlagsDefaultsToReport(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseConfigFlags(nil, &errOut)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.json {
		t.Fatalf("cfg = %#v", cfg)
	}
	if errOut.Len() != 0 {
		t.Fatalf("unexpected output: %q", errOut.String())
	}
}

func TestParseConfigFlagsAcceptsJSON(t *testing.T) {
	var errOut bytes.Buffer
	cfg, err := parseConfigFlags([]string{"--json"}, &errOut)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.json {
		t.Fatalf("cfg = %#v", cfg)
	}
	if errOut.Len() != 0 {
		t.Fatalf("unexpected output: %q", errOut.String())
	}
}

func TestParseConfigFlagsJSONTakesNoValue(t *testing.T) {
	var errOut bytes.Buffer
	if _, err := parseConfigFlags([]string{"--json", "true"}, &errOut); err == nil {
		t.Fatal("--json must not consume a value; a trailing argument is a usage error")
	}
	if !strings.Contains(errOut.String(), configUsage) {
		t.Fatalf("usage not printed: %q", errOut.String())
	}
	var valueErrOut bytes.Buffer
	if _, err := parseConfigFlags([]string{"--json=maybe"}, &valueErrOut); err == nil {
		t.Fatal("--json accepts only boolean values")
	}
}

func TestParseConfigFlagsRejectsUnknownInput(t *testing.T) {
	var flagErrOut bytes.Buffer
	if _, err := parseConfigFlags([]string{"--bogus"}, &flagErrOut); err == nil {
		t.Fatal("unknown flag must error")
	}
	var argErrOut bytes.Buffer
	if _, err := parseConfigFlags([]string{"--json", "extra"}, &argErrOut); err == nil {
		t.Fatal("positional argument must error")
	}
	if !strings.Contains(argErrOut.String(), configUsage) {
		t.Fatalf("usage not printed: %q", argErrOut.String())
	}
}

// layoutPaths must stay byte-for-byte identical to config.NewPaths (minus its
// directory creation) or the report would describe different paths than the
// app uses.
func TestLayoutPathsMirrorsInternalConfig(t *testing.T) {
	base := t.TempDir()
	want, err := config.NewPaths(base)
	if err != nil {
		t.Fatal(err)
	}
	got, err := layoutPaths(base)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("layoutPaths = %#v, want config.NewPaths %#v", got, want)
	}
}

// resolveReportPaths must derive the data directory from LOCALAPPDATA without
// creating anything, even on a machine where Blunt Code has never run.
func TestResolveReportPathsUsesLocalAppDataWithoutCreating(t *testing.T) {
	base := t.TempDir()
	paths, err := resolveReportPaths(fakeEnv(map[string]string{"LOCALAPPDATA": base}))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, config.AppName)
	if paths.DataDir != want {
		t.Fatalf("data dir = %q, want %q", paths.DataDir, want)
	}
	for name, path := range map[string]string{
		"database":    paths.DBPath,
		"logs dir":    paths.LogsDir,
		"temp dir":    paths.TempDir,
		"tools dir":   paths.ToolsDir,
		"reports dir": paths.ReportsDir,
	} {
		if !strings.HasPrefix(path, want+string(filepath.Separator)) {
			t.Fatalf("%s = %q, want below %q", name, path, want)
		}
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("resolveReportPaths created %s (%q); the report must be read-only", name, path)
		}
	}
}

func TestBuildConfigReportPathExistenceMarkers(t *testing.T) {
	base := t.TempDir()
	paths, err := layoutPaths(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{paths.DataDir, paths.ToolsDir, paths.LogsDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	report := buildConfigReport(context.Background(), configReportInputs{
		Version: "test",
		Paths:   paths,
		Getenv:  fakeEnv(nil),
	})
	if !report.Paths.DataDir.Exists || !report.Paths.ToolsDir.Exists || !report.Paths.Logs.Exists {
		t.Fatalf("created paths must be reported as existing: %#v", report.Paths)
	}
	if report.Paths.Database.Exists || report.Paths.Reports.Exists || report.Paths.Temp.Exists {
		t.Fatalf("missing paths must be reported as missing: %#v", report.Paths)
	}
	var out bytes.Buffer
	report.WriteHuman(&out)
	for _, want := range []string{"(exists)", "(missing)", paths.DBPath, paths.ReportsDir} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("human report missing %q:\n%s", want, out.String())
		}
	}
}

func TestConfigReportSonarStartupTimeout(t *testing.T) {
	cases := []struct {
		name      string
		value     string
		set       bool
		valid     bool
		seconds   float64
		humanWant string
	}{
		{name: "unset", humanWant: "not set"},
		{name: "valid", value: "45s", set: true, valid: true, seconds: 45, humanWant: "parsed duration 45s"},
		{name: "nonpositive is invalid", value: "-3m", set: true, humanWant: "invalid (will warn and use default)"},
		{name: "unparseable is invalid", value: "soon", set: true, humanWant: "invalid (will warn and use default)"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			paths, err := layoutPaths(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			report := buildConfigReport(context.Background(), configReportInputs{
				Version: "test",
				Paths:   paths,
				Getenv:  fakeEnv(map[string]string{sonarStartupTimeoutEnv: item.value}),
			})
			env := report.Environment.SonarStartupTimeout
			if env.Name != sonarStartupTimeoutEnv || env.Set != item.set || env.Valid != item.valid || env.ValueSeconds != item.seconds {
				t.Fatalf("env info = %#v", env)
			}
			var out bytes.Buffer
			report.WriteHuman(&out)
			if !strings.Contains(out.String(), item.humanWant) {
				t.Fatalf("human report missing %q:\n%s", item.humanWant, out.String())
			}
		})
	}
}

func TestConfigReportAppSettingsFromDatabase(t *testing.T) {
	base := t.TempDir()
	db, err := database.Open(context.Background(), filepath.Join(base, "bluntcode.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAppSettings(context.Background(), database.AppSettings{Offline: true, OpenBrowser: false}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	paths, err := layoutPaths(base)
	if err != nil {
		t.Fatal(err)
	}
	report := buildConfigReport(context.Background(), configReportInputs{
		Version: "test",
		Paths:   paths,
		Getenv:  fakeEnv(nil),
		OpenDB:  database.Open,
	})
	settings := report.AppSettings
	if !settings.Available || settings.Source != "database" || !settings.Offline || settings.OpenBrowser {
		t.Fatalf("app settings = %#v", settings)
	}
	var out bytes.Buffer
	report.WriteHuman(&out)
	for _, want := range []string{"offline mode: on", "open browser on start: off"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("human report missing %q:\n%s", want, out.String())
		}
	}
}

func TestConfigReportAppSettingsDefaultsWithoutDatabase(t *testing.T) {
	paths, err := layoutPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	report := buildConfigReport(context.Background(), configReportInputs{
		Version: "test",
		Paths:   paths,
		Getenv:  fakeEnv(nil),
	})
	settings := report.AppSettings
	if settings.Available || settings.Source != "defaults" || settings.Offline || !settings.OpenBrowser {
		t.Fatalf("app settings = %#v", settings)
	}
	var out bytes.Buffer
	report.WriteHuman(&out)
	if !strings.Contains(out.String(), "database not created yet") {
		t.Fatalf("human report missing defaults note:\n%s", out.String())
	}
}

// The lock-safe read never takes the instance mutex; the only way database
// access fails in production is the running app holding SQLite past the
// configured busy timeout. Holding a real >5s SQLite lock here would slow the
// suite for the same branch, so the opener is injected instead (the same
// branch doctor's database check exercises for real).
func TestConfigReportAppSettingsUnavailableWhileAppRuns(t *testing.T) {
	paths, err := layoutPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// The database file must exist for the report to attempt opening it.
	if err := os.WriteFile(paths.DBPath, []byte("database file"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := buildConfigReport(context.Background(), configReportInputs{
		Version: "test",
		Paths:   paths,
		Getenv:  fakeEnv(nil),
		OpenDB: func(context.Context, string) (*database.DB, error) {
			return nil, context.DeadlineExceeded
		},
	})
	settings := report.AppSettings
	if settings.Available || settings.Source != "unavailable" || settings.Note != "app is running" {
		t.Fatalf("app settings = %#v", settings)
	}
	var out bytes.Buffer
	report.WriteHuman(&out)
	if !strings.Contains(out.String(), "app settings: unavailable (app is running)") {
		t.Fatalf("human report missing unavailable note:\n%s", out.String())
	}
}

func TestConfigReportManagedTools(t *testing.T) {
	manifest, err := tools.DefaultManifest()
	if err != nil {
		t.Fatal(err)
	}
	ruff, ok := manifest.Find("ruff", runtime.GOOS+"-"+runtime.GOARCH)
	if !ok {
		t.Skip("embedded manifest has no ruff artifact for this platform")
	}
	paths, err := layoutPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Plant the pinned ruff layout (tools/<id>/<version>/<executable>) and
	// leave everything else missing.
	target := filepath.Join(paths.ToolsDir, ruff.ToolID, ruff.Version, ruff.Executable)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("stub"), 0o700); err != nil {
		t.Fatal(err)
	}
	report := buildConfigReport(context.Background(), configReportInputs{
		Version:    "test",
		Paths:      paths,
		Getenv:     fakeEnv(nil),
		Manifest:   manifest,
		ManifestOK: true,
	})
	if !report.ManagedTools.Available {
		t.Fatalf("managed tools = %#v", report.ManagedTools)
	}
	byID := map[string]toolInfo{}
	for _, tool := range report.ManagedTools.Tools {
		byID[tool.ID] = tool
	}
	if tool := byID["ruff"]; !tool.Installed || tool.Version != ruff.Version {
		t.Fatalf("ruff = %#v, want installed %s", tool, ruff.Version)
	}
	if tool := byID["biome"]; tool.Installed {
		t.Fatalf("biome = %#v, want not installed", tool)
	}
	var out bytes.Buffer
	report.WriteHuman(&out)
	if !strings.Contains(out.String(), "Ruff "+ruff.Version+": installed") {
		t.Fatalf("human report missing ruff line:\n%s", out.String())
	}
	if !strings.Contains(out.String(), ": not installed") {
		t.Fatalf("human report missing not-installed marker:\n%s", out.String())
	}
}

// topLevelKeys decodes the key order of the first JSON object in data, so the
// test can pin the report's stable field order, not just its key set.
func topLevelKeys(t *testing.T, data []byte) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		t.Fatalf("read object start: %v", err)
	}
	if tok != json.Delim('{') {
		t.Fatalf("report is not a JSON object: %v", tok)
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("read key: %v", err)
		}
		key, ok := tok.(string)
		if !ok {
			t.Fatalf("key token = %#v", tok)
		}
		keys = append(keys, key)
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatalf("skip value of %s: %v", key, err)
		}
	}
	return keys
}

func objectKeys(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("unmarshal object: %v", err)
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func assertKeys(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
}

func TestConfigReportJSONShapeIsStable(t *testing.T) {
	paths, err := layoutPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manifest, manifestErr := tools.DefaultManifest()
	if manifestErr != nil {
		t.Fatal(manifestErr)
	}
	report := buildConfigReport(context.Background(), configReportInputs{
		Version:    "shape-test",
		Paths:      paths,
		Getenv:     fakeEnv(map[string]string{sonarStartupTimeoutEnv: "45s"}),
		Manifest:   manifest,
		ManifestOK: true,
	})
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	assertKeys(t, topLevelKeys(t, data), []string{"version", "paths", "environment", "app_settings", "managed_tools"})
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatal(err)
	}
	assertKeys(t, objectKeys(t, top["paths"]), []string{"data_dir", "database", "logs_dir", "reports_dir", "temp_dir", "tools_dir"})
	for _, key := range []string{"data_dir", "database", "logs_dir", "reports_dir", "temp_dir", "tools_dir"} {
		var section map[string]json.RawMessage
		if err := json.Unmarshal(top["paths"], &section); err != nil {
			t.Fatal(err)
		}
		assertKeys(t, objectKeys(t, section[key]), []string{"exists", "path"})
	}
	var environment map[string]json.RawMessage
	if err := json.Unmarshal(top["environment"], &environment); err != nil {
		t.Fatal(err)
	}
	assertKeys(t, objectKeys(t, environment["sonar_startup_timeout"]), []string{"name", "raw_value", "set", "valid", "value_seconds"})
	assertKeys(t, objectKeys(t, top["app_settings"]), []string{"available", "note", "offline", "open_browser", "source"})
	assertKeys(t, objectKeys(t, top["managed_tools"]), []string{"available", "tools"})
	var toolsSection struct {
		Tools []map[string]json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(top["managed_tools"], &toolsSection); err != nil {
		t.Fatal(err)
	}
	if len(toolsSection.Tools) == 0 {
		t.Fatal("managed tools list must not be empty with a loaded manifest")
	}
	for _, tool := range toolsSection.Tools {
		keys := objectKeys(t, mustMarshal(t, tool))
		assertKeys(t, keys, []string{"id", "installed", "name", "version"})
	}
}

func mustMarshal(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

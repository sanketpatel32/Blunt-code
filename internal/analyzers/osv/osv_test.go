package osv

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

func TestIdentity(t *testing.T) {
	adapter := New("osv-scanner.exe", "2.5.1")
	if adapter.ID() != "osv-dependencies" {
		t.Fatalf("ID() = %q", adapter.ID())
	}
	if adapter.DisplayName() != "OSV Scanner" {
		t.Fatalf("DisplayName() = %q", adapter.DisplayName())
	}
	// Every supported language must exist in the shared enum's routable set;
	// a typo here would silently unregister the analyzer from those tiers.
	want := []analyzers.Language{
		analyzers.LanguagePython, analyzers.LanguageJavaScript, analyzers.LanguageTypeScript,
		analyzers.LanguageGo, analyzers.LanguageJava, analyzers.LanguageCSharp,
		analyzers.LanguagePHP, analyzers.LanguageRuby, analyzers.LanguageRust,
	}
	if got := adapter.SupportedLanguages(); !reflect.DeepEqual(got, want) {
		t.Fatalf("SupportedLanguages() = %#v, want %#v", got, want)
	}
}

func TestCheckReportsReadiness(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "osv-scanner.exe")
	status := New(exe, "2.5.1").Check(context.Background(), analyzers.ToolEnvironment{})
	if status.Ready || status.Detail == "" || status.Version != "2.5.1" {
		t.Fatalf("missing executable must not be ready: %#v", status)
	}
	if err := os.WriteFile(exe, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	status = New(exe, "2.5.1").Check(context.Background(), analyzers.ToolEnvironment{})
	if !status.Ready || status.Detail != "" {
		t.Fatalf("existing executable must be ready: %#v", status)
	}
}

func TestPlanIsDeepOnly(t *testing.T) {
	adapter := New(`C:\BluntCode\tools\osv-scanner\2.5.1\osv-scanner.exe`, "2.5.1")
	base := analyzers.ScanRequest{
		WorkspaceRoot: `C:\ws`,
		Files:         []string{`C:\ws\package-lock.json`},
		Languages:     []analyzers.Language{analyzers.LanguageJavaScript},
	}
	// The empty profile behaves exactly like standard everywhere, so it must
	// skip too. Quick never reaches the adapter (orchestration gates it), but
	// Plan has to stay correct if called directly.
	for _, profile := range []string{"", analyzers.ProfileStandard, analyzers.ProfileQuick} {
		req := base
		req.Profile = profile
		plan, err := adapter.Plan(context.Background(), req)
		if err != nil {
			t.Fatalf("profile %q: %v", profile, err)
		}
		if len(plan.Commands) != 0 {
			t.Fatalf("profile %q planned %d commands, want a command-less no-op plan", profile, len(plan.Commands))
		}
		if plan.AnalyzerID != ID || plan.Metadata["reason"] != "deep_only" {
			t.Fatalf("profile %q plan = %#v", profile, plan)
		}
		// The no-op plan must survive the whole pipeline: Run treats it as a
		// deliberate skip and Normalize yields nothing, which is what the
		// executor records as a succeeded run with zero findings.
		result, err := adapter.Run(context.Background(), plan, nil)
		if err != nil || result.ExitCode != 0 {
			t.Fatalf("profile %q run: result=%#v err=%v", profile, result, err)
		}
		findings, _, err := adapter.Normalize(context.Background(), result)
		if err != nil || len(findings) != 0 {
			t.Fatalf("profile %q normalize: findings=%d err=%v", profile, len(findings), err)
		}
	}

	req := base
	req.Profile = analyzers.ProfileDeep
	plan, err := adapter.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commands) != 1 {
		t.Fatalf("deep planned %d commands, want exactly one", len(plan.Commands))
	}
	command := plan.Commands[0]
	if command.Executable != adapter.Executable || command.Dir != `C:\ws` {
		t.Fatalf("unexpected process: %#v", command)
	}
	want := []string{"scan", "source", "--format", "json", "--recursive", "--no-ignore", "--allow-no-lockfiles", "."}
	if !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("deep args = %#v, want %#v", command.Args, want)
	}
	if len(command.Env) != 0 {
		t.Fatalf("online plan must not set environment, got %#v", command.Env)
	}
}

func TestPlanOfflineModeAndManagedCache(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "osv-db")
	adapter := New("osv-scanner.exe", "2.5.1")
	adapter.Offline = true
	adapter.DBCacheDir = cache
	plan, err := adapter.Plan(context.Background(), analyzers.ScanRequest{
		WorkspaceRoot: `C:\ws`,
		Languages:     []analyzers.Language{analyzers.LanguagePython},
		Profile:       analyzers.ProfileDeep,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"scan", "source", "--format", "json", "--recursive", "--no-ignore", "--allow-no-lockfiles",
		"--offline", "--offline-vulnerabilities", ".",
	}
	if !reflect.DeepEqual(plan.Commands[0].Args, want) {
		t.Fatalf("offline args = %#v, want %#v", plan.Commands[0].Args, want)
	}
	if got := plan.Commands[0].Env["OSV_SCANNER_LOCAL_DB_CACHE_DIRECTORY"]; got != cache {
		t.Fatalf("cache env = %q, want %q", got, cache)
	}
	if plan.Metadata["offline"] != true {
		t.Fatalf("plan metadata = %#v", plan.Metadata)
	}
}

func TestPlanRefusesUnusableRequests(t *testing.T) {
	adapter := New("osv-scanner.exe", "2.5.1")
	// No workspace root means "." would resolve against Blunt Code's own
	// process directory and scan the wrong tree.
	if _, err := adapter.Plan(context.Background(), analyzers.ScanRequest{Languages: []analyzers.Language{analyzers.LanguageGo}, Profile: analyzers.ProfileDeep}); err == nil {
		t.Fatal("expected error without a workspace root")
	}
	// Language gating mirrors semgrep: no dependency-manifest language in the
	// selection means the analyzer does not apply.
	if _, err := adapter.Plan(context.Background(), analyzers.ScanRequest{WorkspaceRoot: `C:\ws`, Languages: []analyzers.Language{analyzers.LanguageCSS, analyzers.LanguageSQL}, Profile: analyzers.ProfileDeep}); err == nil {
		t.Fatal("expected error without a supported language")
	}
}

// reportWorkspaceRoot extracts the directory the real report was produced in,
// keeping the fixture byte-for-byte real (machine-specific absolute source
// paths included) while the assertions stay machine-independent.
func reportWorkspaceRoot(t *testing.T, b []byte) string {
	t.Helper()
	var probe struct {
		Results []struct {
			Source struct {
				Path string `json:"path"`
			} `json:"source"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &probe); err != nil || len(probe.Results) == 0 {
		t.Fatalf("probe captured report: %v", err)
	}
	return filepath.Dir(probe.Results[0].Source.Path)
}

func TestNormalizeRealReport(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "real-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Captured from `osv-scanner scan source --format json --offline
	// --recursive --no-ignore --allow-no-lockfiles .` (v2.5.1) over a fixture
	// workspace pinning lodash 4.17.15 (package-lock.json) and
	// requests 2.19.1 (requirements.txt): 6 + 10 advisories, exit code 1.
	root := reportWorkspaceRoot(t, b)
	got, _, err := New("osv-scanner.exe", "2.5.1").Normalize(context.Background(), analyzers.AnalyzerResult{
		Stdout:   b,
		Stderr:   []byte("Scanning dir .\n"),
		ExitCode: 1,
		Plan:     analyzers.AnalyzerPlan{Commands: []analyzers.ProcessSpec{{Dir: root}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 16 {
		t.Fatalf("normalized %d findings, want 16 (6 lodash + 10 requests)", len(got))
	}
	bySeverity := map[analyzers.Severity]int{}
	byRule := map[string]analyzers.Finding{}
	for _, f := range got {
		bySeverity[f.Severity]++
		byRule[f.RuleID] = f
		if err := analyzers.ValidateFinding(f); err != nil {
			t.Fatalf("finding %s: %v", f.RuleID, err)
		}
		if f.Fingerprint == "" {
			t.Fatalf("finding %s has no fingerprint", f.RuleID)
		}
		if f.Category != analyzers.CategoryVulnerability {
			t.Fatalf("finding %s category = %s", f.RuleID, f.Category)
		}
		if f.StartLine != 0 {
			t.Fatalf("manifest findings carry no line data: %s has %d", f.RuleID, f.StartLine)
		}
	}
	if bySeverity[analyzers.SeverityHigh] != 5 || bySeverity[analyzers.SeverityMedium] != 11 {
		t.Fatalf("severity spread = %#v, want 5 high and 11 medium", bySeverity)
	}
	lodash, ok := byRule["GHSA-p6mc-m468-83gw"]
	if !ok {
		t.Fatal("lodash prototype-pollution advisory missing from findings")
	}
	if lodash.Severity != analyzers.SeverityHigh || lodash.RelativePath != "package-lock.json" {
		t.Fatalf("lodash finding = %#v", lodash)
	}
	if !strings.Contains(lodash.Message, "lodash@4.17.15") || !strings.Contains(lodash.Message, "Prototype Pollution") {
		t.Fatalf("message = %q", lodash.Message)
	}
	if lodash.DocumentationURL != "https://osv.dev/GHSA-p6mc-m468-83gw" {
		t.Fatalf("documentation url = %q", lodash.DocumentationURL)
	}
	// PYSEC records in the offline databases ship no database_specific
	// severity at all; the alias group's max CVSS score (7.5 here) must lift
	// requests' 2018 advisory to high.
	requests, ok := byRule["PYSEC-2018-28"]
	if !ok {
		t.Fatal("requests PYSEC advisory missing from findings")
	}
	if requests.Severity != analyzers.SeverityHigh || requests.RelativePath != "requirements.txt" {
		t.Fatalf("requests finding = %#v", requests)
	}
	if requests.Metadata["cvss_max"] != "7.5" {
		t.Fatalf("requests cvss_max = %#v, want 7.5", requests.Metadata["cvss_max"])
	}
}

func TestNormalizeDeduplicatesPerPackageAndAdvisory(t *testing.T) {
	// requirements.txt can surface twice (direct lockfile read plus extracted
	// inventory); the same package+version+advisory must collapse to one
	// finding while a second package keeps its own.
	captured := `{
	  "results": [
	    {"source": {"path": "C:/ws/requirements.txt", "type": "lockfile"},
	     "packages": [{"package": {"name": "requests", "version": "2.19.1", "ecosystem": "PyPI"},
	                   "vulnerabilities": [{"id": "PYSEC-2018-28", "summary": "Redirect with auth"}]}]},
	    {"source": {"path": "C:/ws/requirements.txt", "type": "unknown"},
	     "packages": [{"package": {"name": "requests", "version": "2.19.1", "ecosystem": "PyPI"},
	                   "vulnerabilities": [{"id": "PYSEC-2018-28", "summary": "Redirect with auth"}]},
	                  {"package": {"name": "urllib3", "version": "1.23", "ecosystem": "PyPI"},
	                   "vulnerabilities": [{"id": "PYSEC-2018-28", "summary": "Redirect with auth"}]}]}
	  ]
	}`
	got, _, err := New("osv-scanner.exe", "2.5.1").Normalize(context.Background(), analyzers.AnalyzerResult{
		Stdout:   []byte(captured),
		ExitCode: 1,
		Plan:     analyzers.AnalyzerPlan{Commands: []analyzers.ProcessSpec{{Dir: `C:\ws`}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("normalized %d findings, want 2 after dedup", len(got))
	}
}

func TestNormalizeHandlesCleanAndFailedRuns(t *testing.T) {
	adapter := New("osv-scanner.exe", "2.5.1")
	// --allow-no-lockfiles yields exit 0 with "results": null on workspaces
	// without manifests; that is a successful, finding-free run.
	got, _, err := adapter.Normalize(context.Background(), analyzers.AnalyzerResult{
		Stdout:   []byte(`{"results": null, "experimental_config": {}}`),
		ExitCode: 0,
		Plan:     analyzers.AnalyzerPlan{Commands: []analyzers.ProcessSpec{{Dir: `C:\ws`}}},
	})
	if err != nil || len(got) != 0 {
		t.Fatalf("results:null -> findings=%d err=%v", len(got), err)
	}
	// 128 is a real failure (for example scanning without --allow-no-lockfiles
	// in a tree with no package sources); it must surface as an error.
	_, _, err = adapter.Normalize(context.Background(), analyzers.AnalyzerResult{
		ExitCode: 128, Stderr: []byte("No package sources found, --help for usage information.\n"),
	})
	if err == nil || !strings.Contains(err.Error(), "osv-scanner exited 128") {
		t.Fatalf("err = %v, want exit-code error", err)
	}
}

func TestSeverityMapping(t *testing.T) {
	vuln := func(severity string) vulnerability {
		v := vulnerability{ID: "GHSA-test"}
		v.DatabaseSpecific.Severity = severity
		return v
	}
	group := func(score string) []groupEntry {
		return []groupEntry{{IDs: []string{"GHSA-test"}, MaxSeverity: score}}
	}
	cases := []struct {
		name     string
		v        vulnerability
		groups   []groupEntry
		want     analyzers.Severity
		wantCVSS string
	}{
		{"GHSA critical label", vuln("CRITICAL"), nil, analyzers.SeverityCritical, ""},
		{"GHSA high label", vuln("HIGH"), nil, analyzers.SeverityHigh, ""},
		{"GHSA moderate label", vuln("MODERATE"), nil, analyzers.SeverityMedium, ""},
		{"plain medium label", vuln("MEDIUM"), nil, analyzers.SeverityMedium, ""},
		{"GHSA low label", vuln("LOW"), nil, analyzers.SeverityLow, ""},
		{"numeric severity string", vuln("7.5"), nil, analyzers.SeverityHigh, "7.5"},
		{"numeric severity critical", vuln("9.8"), nil, analyzers.SeverityCritical, "9.8"},
		{"group score critical", vuln(""), group("9.1"), analyzers.SeverityCritical, "9.1"},
		{"group score medium", vuln(""), group("5.3"), analyzers.SeverityMedium, "5.3"},
		{"group score low", vuln(""), group("0.5"), analyzers.SeverityLow, "0.5"},
		{"missing everywhere falls back to low", vuln(""), nil, analyzers.SeverityLow, ""},
		{"unrecognized label falls back to low", vuln("SUPER BAD"), nil, analyzers.SeverityLow, ""},
	}
	for _, c := range cases {
		sev, _, cvss := severity(c.v, c.groups)
		if sev != c.want || cvss != c.wantCVSS {
			t.Fatalf("%s: severity=%s cvss=%q, want %s/%q", c.name, sev, cvss, c.want, c.wantCVSS)
		}
	}
}

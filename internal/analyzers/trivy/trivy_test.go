package trivy

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

// realReport is the unedited JSON produced by trivy 0.74.0 (windows-amd64)
// over a fixture workspace containing a bad-practice Dockerfile (FROM
// ubuntu:18.04, root user, apt-get without --no-install-recommends), a
// terraform S3 bucket with acl = "public-read", a package-lock.json pinning
// lodash 4.17.15, and a creds.txt holding an AWS-shaped access key ID.
func loadRealReport(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "real-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func normalizeRealReport(t *testing.T) []analyzers.Finding {
	t.Helper()
	adapter := New("trivy.exe", "0.74.0")
	got, _, err := adapter.Normalize(context.Background(), analyzers.AnalyzerResult{
		Stdout:   loadRealReport(t),
		ExitCode: 0,
		Plan:     analyzers.AnalyzerPlan{AnalyzerID: ID, Commands: []analyzers.ProcessSpec{{Dir: `C:\ws`}}},
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	return got
}

func findByRule(t *testing.T, findings []analyzers.Finding, path, rule string) analyzers.Finding {
	t.Helper()
	for _, f := range findings {
		if f.RelativePath == path && f.RuleID == rule {
			return f
		}
	}
	t.Fatalf("no %s finding in %s", rule, path)
	return analyzers.Finding{}
}

func TestNormalizeRealReportCountsAndCategories(t *testing.T) {
	got := normalizeRealReport(t)
	// 3 Dockerfile + 9 terraform misconfigurations, 7 lodash CVEs, 1 secret.
	if len(got) != 20 {
		t.Fatalf("got %d findings, want 20", len(got))
	}
	perPath := map[string]int{}
	categories := map[analyzers.Category]int{}
	for _, f := range got {
		perPath[f.RelativePath]++
		categories[f.Category]++
		if f.AnalyzerID != ID {
			t.Fatalf("finding %s has analyzer %q", f.RuleID, f.AnalyzerID)
		}
		if err := analyzers.ValidateFinding(f); err != nil {
			t.Fatalf("finding %s in %s invalid: %v", f.RuleID, f.RelativePath, err)
		}
		if f.Fingerprint == "" {
			t.Fatalf("finding %s in %s has no fingerprint", f.RuleID, f.RelativePath)
		}
	}
	for path, want := range map[string]int{"Dockerfile": 3, "main.tf": 9, "creds.txt": 1, "package-lock.json": 7} {
		if perPath[path] != want {
			t.Fatalf("path %s: %d findings, want %d (all: %#v)", path, perPath[path], want, perPath)
		}
	}
	if categories[analyzers.CategorySecurity] != 13 || categories[analyzers.CategoryVulnerability] != 7 {
		t.Fatalf("categories = %#v, want 13 security (misconfig+secret) and 7 vulnerability", categories)
	}
}

func TestNormalizeRealReportDockerfileMisconfigurations(t *testing.T) {
	got := normalizeRealReport(t)
	root := findByRule(t, got, "Dockerfile", "DS-0002")
	if root.Severity != analyzers.SeverityHigh || root.Category != analyzers.CategorySecurity {
		t.Fatalf("DS-0002 severity/category = %s/%s", root.Severity, root.Category)
	}
	// Module-level checks carry no CauseMetadata line; the finding must keep
	// zero lines rather than inventing one.
	if root.StartLine != 0 || root.EndLine != 0 {
		t.Fatalf("DS-0002 lines = %d-%d, want 0-0", root.StartLine, root.EndLine)
	}
	if !strings.HasPrefix(root.Message, "DS-0002: Image user should not be 'root'") {
		t.Fatalf("DS-0002 message = %q, want ID + Title + Description prefix", root.Message)
	}
	if !strings.Contains(root.Remediation, "USER") {
		t.Fatalf("DS-0002 remediation = %q, want the Resolution text", root.Remediation)
	}
	aptGet := findByRule(t, got, "Dockerfile", "DS-0029")
	if aptGet.StartLine != 2 || aptGet.EndLine != 2 {
		t.Fatalf("DS-0029 lines = %d-%d, want 2-2 (the apt-get RUN line)", aptGet.StartLine, aptGet.EndLine)
	}
	if len(root.Message) > maxMessageRunes+3 {
		t.Fatalf("DS-0002 message not truncated: %d runes", len(root.Message))
	}
}

func TestNormalizeRealReportTerraformMisconfigurations(t *testing.T) {
	got := normalizeRealReport(t)
	// AWS-0092 is the public-ACL check; it points at the acl line (3).
	public := findByRule(t, got, "main.tf", "AWS-0092")
	if public.Severity != analyzers.SeverityHigh || public.Category != analyzers.CategorySecurity {
		t.Fatalf("AWS-0092 severity/category = %s/%s", public.Severity, public.Category)
	}
	if public.StartLine != 3 || public.EndLine != 3 {
		t.Fatalf("AWS-0092 lines = %d-%d, want 3-3 (acl = \"public-read\")", public.StartLine, public.EndLine)
	}
	if public.DocumentationURL == "" {
		t.Fatal("AWS-0092 has no documentation URL")
	}
}

func TestNormalizeRealReportVulnerabilities(t *testing.T) {
	got := normalizeRealReport(t)
	var vulns []analyzers.Finding
	for _, f := range got {
		if f.RelativePath == "package-lock.json" {
			vulns = append(vulns, f)
		}
	}
	if len(vulns) != 7 {
		t.Fatalf("got %d lodash vulnerabilities, want 7", len(vulns))
	}
	for _, f := range vulns {
		if f.Category != analyzers.CategoryVulnerability {
			t.Fatalf("%s category = %s, want vulnerability", f.RuleID, f.Category)
		}
		// Message is vulnID + pkg@version + title for every CVE.
		if !strings.Contains(f.Message, f.RuleID+": lodash@4.17.15 - ") {
			t.Fatalf("%s message = %q, want CVE + lodash@4.17.15 + title", f.RuleID, f.Message)
		}
	}
	prototype := findByRule(t, got, "package-lock.json", "CVE-2020-8203")
	if prototype.Severity != analyzers.SeverityHigh {
		t.Fatalf("CVE-2020-8203 severity = %s, want high", prototype.Severity)
	}
	if prototype.Remediation != "Upgrade lodash to 4.17.19" {
		t.Fatalf("CVE-2020-8203 remediation = %q", prototype.Remediation)
	}
}

func TestNormalizeRealReportSecretNeverCarriesMatchedValue(t *testing.T) {
	got := normalizeRealReport(t)
	secret := findByRule(t, got, "creds.txt", "aws-access-key-id")
	if secret.Severity != analyzers.SeverityCritical || secret.Category != analyzers.CategorySecurity {
		t.Fatalf("secret severity/category = %s/%s", secret.Severity, secret.Category)
	}
	if secret.StartLine != 1 || secret.EndLine != 1 {
		t.Fatalf("secret lines = %d-%d, want 1-1", secret.StartLine, secret.EndLine)
	}
	if !strings.Contains(secret.Message, "AWS Access Key ID") {
		t.Fatalf("secret message = %q, want the rule title", secret.Message)
	}
	// Belt and braces: the matched key material must not surface anywhere in
	// any finding, whatever trivy put into its raw Match field.
	for _, f := range got {
		for _, field := range []string{f.Title, f.Message, f.Remediation, f.RuleID} {
			if strings.Contains(field, "AKIA") {
				t.Fatalf("raw secret material leaked into finding %s: %q", f.RuleID, field)
			}
		}
		for _, v := range f.Metadata {
			if s, ok := v.(string); ok && strings.Contains(s, "AKIA") {
				t.Fatalf("raw secret material leaked into finding %s metadata", f.RuleID)
			}
		}
	}
}

func TestNormalizeRejectsFatalExit(t *testing.T) {
	adapter := New("trivy.exe", "0.74.0")
	_, _, err := adapter.Normalize(context.Background(), analyzers.AnalyzerResult{
		ExitCode: 1,
		Stderr:   []byte("FATAL Fatal error, run error: init error: DB error: --skip-db-update cannot be specified on the first run"),
	})
	if err == nil || !strings.Contains(err.Error(), "trivy exited 1") {
		t.Fatalf("err = %v, want fatal-exit error", err)
	}
}

func TestIdentityAndLanguages(t *testing.T) {
	adapter := New("trivy.exe", "0.74.0")
	if adapter.ID() != "container-trivy" {
		t.Fatalf("ID = %q", adapter.ID())
	}
	if adapter.DisplayName() != "Trivy" {
		t.Fatalf("DisplayName = %q", adapter.DisplayName())
	}
	want := []analyzers.Language{analyzers.LanguageDockerfile, analyzers.LanguageYAML, analyzers.LanguageJSON, analyzers.LanguageTOML}
	if !reflect.DeepEqual(adapter.SupportedLanguages(), want) {
		t.Fatalf("SupportedLanguages = %#v, want %#v", adapter.SupportedLanguages(), want)
	}
}

func TestCheckNotReadyAndReady(t *testing.T) {
	missing := New(filepath.Join(t.TempDir(), "trivy.exe"), "0.74.0")
	if status := missing.Check(context.Background(), analyzers.ToolEnvironment{}); status.Ready || status.Detail == "" {
		t.Fatalf("missing executable: %#v, want not ready with detail", status)
	}
	exe := filepath.Join(t.TempDir(), "trivy.exe")
	if err := os.WriteFile(exe, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	status := New(exe, "0.74.0").Check(context.Background(), analyzers.ToolEnvironment{})
	if !status.Ready || status.Version != "0.74.0" {
		t.Fatalf("present executable: %#v, want ready with version", status)
	}
}

func TestPlanIsDeepProfileOnly(t *testing.T) {
	adapter := New("trivy.exe", "0.74.0")
	// Pin the cache so Plan's warm-DB probe never reads the developer's real
	// trivy cache (tests must stay hermetic and deterministic).
	adapter.CacheDir = t.TempDir()
	base := analyzers.ScanRequest{
		WorkspaceRoot: `C:\ws`,
		Files:         []string{`C:\ws\Dockerfile`, `C:\ws\package-lock.json`},
		Languages:     []analyzers.Language{analyzers.LanguageDockerfile, analyzers.LanguageJSON},
	}
	for _, profile := range []string{"", analyzers.ProfileQuick, analyzers.ProfileStandard} {
		req := base
		req.Profile = profile
		if _, err := adapter.Plan(context.Background(), req); err == nil || !strings.Contains(err.Error(), "does not apply") {
			t.Fatalf("profile %q: err = %v, want the does-not-apply skip error", profile, err)
		}
	}
	req := base
	req.Profile = analyzers.ProfileDeep
	plan, err := adapter.Plan(context.Background(), req)
	if err != nil {
		t.Fatalf("deep profile: %v", err)
	}
	defer os.Remove(plan.Metadata[planKeyOutput].(string))
	if len(plan.Commands) != 1 || plan.AnalyzerID != ID || plan.Version != "0.74.0" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestPlanArgumentsColdAndWarmCache(t *testing.T) {
	adapter := New("trivy.exe", "0.74.0")
	adapter.CacheDir = t.TempDir()
	req := analyzers.ScanRequest{
		WorkspaceRoot: `C:\ws`,
		Profile:       analyzers.ProfileDeep,
		Files:         []string{`C:\ws\Dockerfile`, `C:\ws\package-lock.json`},
		Languages:     []analyzers.Language{analyzers.LanguageDockerfile, analyzers.LanguageJSON},
	}

	// Cold cache: trivy must download its DB, so --skip-db-update must not
	// appear (on a cold cache the flag makes trivy exit 1 with no report).
	plan, err := adapter.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	outputPath := plan.Metadata[planKeyOutput].(string)
	defer os.Remove(outputPath)
	wantCold := []string{"fs", "--format", "json", "--output", outputPath, "--scanners", "vuln,secret,misconfig", "."}
	if !reflect.DeepEqual(plan.Commands[0].Args, wantCold) {
		t.Fatalf("cold args = %#v, want %#v", plan.Commands[0].Args, wantCold)
	}
	if plan.Commands[0].Dir != `C:\ws` {
		t.Fatalf("Dir = %q", plan.Commands[0].Dir)
	}
	// A pinned CacheDir always travels as TRIVY_CACHE_DIR so the child
	// process and the adapter's warm probe look at the same directory.
	if got := plan.Commands[0].Env["TRIVY_CACHE_DIR"]; got != adapter.CacheDir {
		t.Fatalf("cold TRIVY_CACHE_DIR = %q, want %q", got, adapter.CacheDir)
	}
	if plan.Metadata[planKeyWarmDB] != false {
		t.Fatalf("db_cache_warm = %v, want false", plan.Metadata[planKeyWarmDB])
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("output file was not reserved: %v", err)
	}

	// Warm cache: db/metadata.json + a non-empty trivy.db exist, so the plan
	// pins TRIVY_CACHE_DIR and skips the DB freshness check.
	if err := os.MkdirAll(filepath.Join(adapter.CacheDir, "db"), 0o700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(adapter.CacheDir, "db", "metadata.json"), []byte(`{"Version":2}`), 0o644)
	os.WriteFile(filepath.Join(adapter.CacheDir, "db", "trivy.db"), []byte("x"), 0o644)
	plan, err = adapter.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(plan.Metadata[planKeyOutput].(string))
	wantWarm := []string{"fs", "--format", "json", "--output", plan.Metadata[planKeyOutput].(string), "--scanners", "vuln,secret,misconfig", "--skip-db-update", "."}
	if !reflect.DeepEqual(plan.Commands[0].Args, wantWarm) {
		t.Fatalf("warm args = %#v, want %#v", plan.Commands[0].Args, wantWarm)
	}
	if got := plan.Commands[0].Env["TRIVY_CACHE_DIR"]; got != adapter.CacheDir {
		t.Fatalf("TRIVY_CACHE_DIR = %q, want %q", got, adapter.CacheDir)
	}
	if plan.Metadata[planKeyWarmDB] != true {
		t.Fatalf("db_cache_warm = %v, want true", plan.Metadata[planKeyWarmDB])
	}
}

func TestPlanRejectsSelectionWithoutManifestFiles(t *testing.T) {
	adapter := New("trivy.exe", "0.74.0")
	adapter.CacheDir = t.TempDir()
	req := analyzers.ScanRequest{
		WorkspaceRoot: `C:\ws`,
		Profile:       analyzers.ProfileDeep,
		// A stale caller may claim manifests exist while listing only code;
		// the file list is authoritative, so Plan must refuse rather than
		// launch a whole-workspace trivy sweep.
		Languages: []analyzers.Language{analyzers.LanguageDockerfile},
		Files:     []string{`C:\ws\main.py`, `C:\ws\main.go`},
	}
	if _, err := adapter.Plan(context.Background(), req); err == nil {
		t.Fatal("expected error when no manifest files are in the selection")
	}
}

func TestRunLoadsAndRemovesOutputFile(t *testing.T) {
	adapter := New(`Z:\definitely\missing\trivy.exe`, "0.74.0")
	adapter.CacheDir = t.TempDir()
	plan, err := adapter.Plan(context.Background(), analyzers.ScanRequest{
		WorkspaceRoot: `C:\ws`,
		Profile:       analyzers.ProfileDeep,
		Files:         []string{`C:\ws\Dockerfile`},
		Languages:     []analyzers.Language{analyzers.LanguageDockerfile},
	})
	if err != nil {
		t.Fatal(err)
	}
	outputPath := plan.Metadata[planKeyOutput].(string)
	if err := os.WriteFile(outputPath, []byte(`{"SchemaVersion":2,"Results":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// The executable cannot start, so Run fails; the reserved report file
	// must still be read into Stdout and cleaned up.
	result, err := adapter.Run(context.Background(), plan, nil)
	if err == nil {
		t.Fatal("expected start failure for a missing executable")
	}
	if string(result.Stdout) != `{"SchemaVersion":2,"Results":[]}` {
		t.Fatalf("Stdout = %q, want the report file content", result.Stdout)
	}
	if _, statErr := os.Stat(outputPath); statErr == nil {
		t.Fatal("output file was not removed after Run")
	}
}

func TestSeverityMapping(t *testing.T) {
	cases := map[string]analyzers.Severity{
		"CRITICAL": analyzers.SeverityCritical,
		"HIGH":     analyzers.SeverityHigh,
		"MEDIUM":   analyzers.SeverityMedium,
		"LOW":      analyzers.SeverityLow,
		"UNKNOWN":  analyzers.SeverityInfo,
		"":         analyzers.SeverityInfo,
	}
	for raw, want := range cases {
		if got := severity(raw); got != want {
			t.Fatalf("severity(%q) = %s, want %s", raw, got, want)
		}
	}
}

func TestRelativeAndTruncate(t *testing.T) {
	plan := analyzers.AnalyzerPlan{Commands: []analyzers.ProcessSpec{{Dir: `C:\ws`}}}
	for path, want := range map[string]string{
		"Dockerfile":       "Dockerfile",
		"sub\\main.tf":     "sub/main.tf",
		`C:\ws\a\k8s.yaml`: "a/k8s.yaml",
		".":                ".",
	} {
		if got := relative(plan, path); got != want {
			t.Fatalf("relative(%q) = %q, want %q", path, got, want)
		}
	}
	if got := truncate(strings.Repeat("x", 600), maxMessageRunes); len([]rune(got)) != maxMessageRunes+3 || !strings.HasSuffix(got, "...") {
		t.Fatalf("truncate produced %d runes without ellipsis", len(got))
	}
	if got := truncate("short", maxMessageRunes); got != "short" {
		t.Fatalf("truncate altered a short string: %q", got)
	}
}

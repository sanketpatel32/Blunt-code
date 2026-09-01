package checkov

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"bluntcode/internal/analyzers"
)

func TestIdentity(t *testing.T) {
	adapter := New(`C:\BluntCode\tools\checkov\3.3.16\env\tools\checkov\Scripts\python.exe`, "3.3.16")
	if adapter.ID() != "iac-checkov" || adapter.DisplayName() != "Checkov" {
		t.Fatalf("id=%q display=%q", adapter.ID(), adapter.DisplayName())
	}
	want := []analyzers.Language{analyzers.LanguageYAML, analyzers.LanguageJSON, analyzers.LanguageDockerfile}
	if got := adapter.SupportedLanguages(); !reflect.DeepEqual(got, want) {
		t.Fatalf("languages = %#v, want %#v", got, want)
	}
}

func TestCheckNotReadyWhenExecutableMissing(t *testing.T) {
	adapter := New(filepath.Join(t.TempDir(), "missing", "python.exe"), "3.3.16")
	status := adapter.Check(context.Background(), analyzers.ToolEnvironment{})
	if status.Ready || status.Version != "3.3.16" || status.Detail == "" {
		t.Fatalf("status = %#v", status)
	}
}

func TestEnsureInstalledRequiresExecutable(t *testing.T) {
	if err := New("", "3.3.16").EnsureInstalled(context.Background(), analyzers.ToolEnvironment{}); err == nil {
		t.Fatal("empty executable must be refused")
	}
	if err := New("python.exe", "3.3.16").EnsureInstalled(context.Background(), analyzers.ToolEnvironment{}); err != nil {
		t.Fatal(err)
	}
}

func TestPlanRequiresDeepProfile(t *testing.T) {
	adapter := New(`C:\BluntCode\tools\checkov\3.3.16\env\tools\checkov\Scripts\python.exe`, "3.3.16")
	req := analyzers.ScanRequest{
		WorkspaceRoot: `C:\source`,
		WorkspaceID:   "w1",
		ScanID:        "scan-1",
		Files:         []string{`C:\source\Dockerfile`},
		Languages:     []analyzers.Language{analyzers.LanguageDockerfile},
	}
	for _, profile := range []string{"", analyzers.ProfileQuick, analyzers.ProfileStandard} {
		req.Profile = profile
		if _, err := adapter.Plan(context.Background(), req); err == nil {
			t.Fatalf("profile %q must skip checkov", profile)
		}
	}
	req.Profile = analyzers.ProfileDeep
	if _, err := adapter.Plan(context.Background(), req); err != nil {
		t.Fatal(err)
	}
}

func TestPlanScansWorkspaceDirectoryOnce(t *testing.T) {
	adapter := New(`C:\BluntCode\tools\checkov\3.3.16\env\tools\checkov\Scripts\python.exe`, "3.3.16")
	req := analyzers.ScanRequest{
		WorkspaceRoot: `C:\source`,
		WorkspaceID:   "w1",
		ScanID:        "scan-1",
		// main.tf and app.py are not routable languages; Dockerfile and the
		// kubernetes manifest keep the analyzer selected.
		Files:     []string{`C:\source\main.tf`, `C:\source\Dockerfile`, `C:\source\app.py`, `C:\source\deploy\k8s.yaml`},
		Languages: []analyzers.Language{analyzers.LanguageDockerfile, analyzers.LanguageYAML},
		Profile:   analyzers.ProfileDeep,
	}
	plan, err := adapter.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.AnalyzerID != ID || plan.Version != "3.3.16" || len(plan.Commands) != 1 {
		t.Fatalf("plan = %#v", plan)
	}
	command := plan.Commands[0]
	if command.Executable != adapter.Executable || command.Dir != `C:\source` {
		t.Fatalf("unexpected managed process: %#v", command)
	}
	outputDir := filepath.Join(os.TempDir(), outputDirPrefix+"scan-1")
	want := []string{
		"-m", "checkov.main",
		"-d", `C:\source`,
		"-o", "json",
		"--output-file-path", outputDir,
		"--framework", frameworks,
		"--quiet",
	}
	if !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("args = %#v, want %#v", command.Args, want)
	}
	for _, key := range []string{"PYTHONPATH", "PYTHONHOME", "CKV_API_TOKEN", "BC_API_KEY"} {
		if value, ok := command.Env[key]; !ok || value != "" {
			t.Fatalf("%s should be neutralized, got %q (present %v)", key, value, ok)
		}
	}
	if plan.Metadata["output_dir"] != outputDir || plan.Metadata["frameworks"] != frameworks {
		t.Fatalf("metadata = %#v", plan.Metadata)
	}
}

func TestPlanSkipsWorkspacesWithoutRoutableFiles(t *testing.T) {
	// Terraform has no Language enum value yet, so a pure-.tf workspace is
	// not routed to checkov today even on the deep profile.
	adapter := New("python.exe", "3.3.16")
	req := analyzers.ScanRequest{
		WorkspaceRoot: `C:\tf-only`,
		Files:         []string{`C:\tf-only\main.tf`},
		Profile:       analyzers.ProfileDeep,
	}
	if _, err := adapter.Plan(context.Background(), req); err == nil {
		t.Fatal("plan must refuse a workspace with no routable IaC files")
	}
}

func TestRunRejectsUnexpectedPlanShapes(t *testing.T) {
	adapter := New("python.exe", "3.3.16")
	zero := analyzers.AnalyzerPlan{AnalyzerID: ID, Metadata: map[string]any{"output_dir": "unused"}}
	if _, err := adapter.Run(context.Background(), zero, nil); err == nil {
		t.Fatal("run must refuse a plan without commands")
	}
	two := zero
	two.Commands = []analyzers.ProcessSpec{{Executable: "python.exe"}, {Executable: "python.exe"}}
	if _, err := adapter.Run(context.Background(), two, nil); err == nil {
		t.Fatal("run must refuse a plan with more than one command")
	}
}

// TestNormalizeRealReport parses the captured output of a real checkov
// 3.3.16 scan (testdata/real-report.json) over a fixture with a
// public-read S3 bucket, an unencrypted EBS volume, and a root Dockerfile:
// 10 terraform failures plus 2 dockerfile failures.
func TestNormalizeRealReport(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "real-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	adapter := New("python.exe", "3.3.16")
	findings, metrics, err := adapter.Normalize(context.Background(), analyzers.AnalyzerResult{
		Stdout:   b,
		Stderr:   []byte(""),
		ExitCode: 1,
		Plan:     analyzers.AnalyzerPlan{AnalyzerID: ID, Commands: []analyzers.ProcessSpec{{Dir: `C:\ws`}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 12 {
		t.Fatalf("findings = %d, want 12", len(findings))
	}
	byRule := make(map[string]analyzers.Finding, len(findings))
	fingerprints := make(map[string]bool, len(findings))
	for _, f := range findings {
		byRule[f.RuleID] = f
		if fingerprints[f.Fingerprint] {
			t.Fatalf("duplicate fingerprint for %s", f.RuleID)
		}
		fingerprints[f.Fingerprint] = true
		if err := analyzers.ValidateFinding(f); err != nil {
			t.Fatalf("%s: %v", f.RuleID, err)
		}
		if f.AnalyzerID != ID || f.Category != analyzers.CategorySecurity {
			t.Fatalf("%s: analyzer=%q category=%q", f.RuleID, f.AnalyzerID, f.Category)
		}
	}
	public := byRule["CKV_AWS_20"]
	if public.Message != "CKV_AWS_20: S3 Bucket has an ACL defined which allows public READ access." ||
		public.RelativePath != "main.tf" || public.StartLine != 1 || public.EndLine != 4 ||
		public.Severity != analyzers.SeverityMedium || public.DocumentationURL == "" {
		t.Fatalf("public-read finding = %#v", public)
	}
	ebs := byRule["CKV_AWS_3"]
	if ebs.RelativePath != "main.tf" || ebs.StartLine != 6 || ebs.EndLine != 10 || ebs.Severity != analyzers.SeverityMedium {
		t.Fatalf("unencrypted EBS finding = %#v", ebs)
	}
	root := byRule["CKV_DOCKER_8"]
	if root.RelativePath != "Dockerfile" || root.StartLine != 4 || root.Severity != analyzers.SeverityMedium {
		t.Fatalf("root-user finding = %#v", root)
	}
	// Offline output carries no per-check severity; the informational
	// missing-HEALTHCHECK check opts down to low, everything else defaults
	// to medium.
	if byRule["CKV_DOCKER_2"].Severity != analyzers.SeverityLow {
		t.Fatalf("CKV_DOCKER_2 = %#v", byRule["CKV_DOCKER_2"])
	}
	wantMetrics := map[string]float64{
		"checks_failed":     12,
		"checks_passed":     26, // 4 terraform + 22 dockerfile
		"resources_scanned": 3,  // 2 terraform resources + 1 dockerfile stage
	}
	if len(metrics) != len(wantMetrics) {
		t.Fatalf("metrics = %#v", metrics)
	}
	for _, m := range metrics {
		if wantMetrics[m.Key] != m.Value {
			t.Fatalf("metric %s = %v, want %v", m.Key, m.Value, wantMetrics[m.Key])
		}
	}
}

func TestNormalizeRejectsFatalExitCode(t *testing.T) {
	adapter := New("python.exe", "3.3.16")
	if _, _, err := adapter.Normalize(context.Background(), analyzers.AnalyzerResult{ExitCode: 2, Stderr: []byte("boom")}); err == nil {
		t.Fatal("exit code 2 must be an error")
	}
}

func TestNormalizeAcceptsSingleReportObject(t *testing.T) {
	// Older checkov releases emitted one report object rather than an array.
	adapter := New("python.exe", "3.3.16")
	findings, _, err := adapter.Normalize(context.Background(), analyzers.AnalyzerResult{
		Stdout:   []byte(`{"check_type":"dockerfile","results":{"failed_checks":[{"check_id":"CKV_DOCKER_8","check_name":"Ensure the last USER is not root","file_path":"/Dockerfile","file_line_range":[4,4],"resource":"/Dockerfile.USER"}]},"summary":{"passed":0,"failed":1,"skipped":0,"resource_count":1}}`),
		ExitCode: 1,
	})
	if err != nil || len(findings) != 1 || findings[0].RelativePath != "Dockerfile" || findings[0].StartLine != 4 {
		t.Fatalf("findings = %#v, err %v", findings, err)
	}
}

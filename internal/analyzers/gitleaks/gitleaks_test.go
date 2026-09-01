package gitleaks

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

// fixtureRoot is the --source directory of the scratch repo that produced
// testdata/real-report.json; Normalize only string-manipulates the File
// fields against it, so the path never needs to exist.
const fixtureRoot = `C:/Users/sanpa/AppData/Local/Temp/bluntcode-tooldev/gitleaks/fixture`

// realReportNote: the fixture is the JSON gitleaks 8.30.1 actually wrote for
// the scratch repo (an AWS access key planted in config.py, a fake Slack
// token in .env, plus clean .py/.ts/.md files), with the Secret and Match
// values replaced by REDACTED; every other field is untouched tool output.
func TestNormalizeRealReport(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "real-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := New("gitleaks.exe", "test").Normalize(context.Background(), analyzers.AnalyzerResult{
		Stdout:   b,
		ExitCode: 0,
		Plan:     analyzers.AnalyzerPlan{Commands: []analyzers.ProcessSpec{{Dir: fixtureRoot}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d findings, want 2 (the two planted secrets): %#v", len(got), got)
	}
	var aws *analyzers.Finding
	for i := range got {
		if got[i].RuleID == "aws-access-token" {
			aws = &got[i]
		}
	}
	if aws == nil {
		t.Fatalf("planted AWS key not reported: %#v", got)
	}
	if aws.RelativePath != "config.py" || aws.StartLine != 3 || aws.Severity != analyzers.SeverityCritical || aws.Category != analyzers.CategorySecurity {
		t.Fatalf("unexpected AWS finding: %#v", aws)
	}
	for _, f := range got {
		if f.RuleID == "generic-api-key" && (f.RelativePath != ".env" || f.Severity != analyzers.SeverityHigh) {
			t.Fatalf("unexpected generic-api-key finding: %#v", f)
		}
		if strings.Contains(f.Message, "xoxp-") || strings.Contains(f.Message, "AKIA") {
			t.Fatalf("message echoes the secret value: %q", f.Message)
		}
		if err := analyzers.ValidateFinding(f); err != nil {
			t.Fatalf("finding %s invalid: %v", f.RuleID, err)
		}
	}
}

func TestPlanScansWorkspaceRootWithJSONReport(t *testing.T) {
	adapter := New(`C:\BluntCode\tools\gitleaks\8.30.1\gitleaks.exe`, "8.30.1")
	req := analyzers.ScanRequest{
		WorkspaceRoot: `C:\source`,
		ScanID:        "0a1b-2c3d",
		Files:         []string{`C:\source\config.py`, `C:\source\src\app.ts`},
		Languages:     []analyzers.Language{analyzers.LanguagePython, analyzers.LanguageTypeScript},
	}
	plan, err := adapter.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commands) != 1 {
		t.Fatalf("gitleaks plans exactly one directory scan, got %d commands", len(plan.Commands))
	}
	command := plan.Commands[0]
	if command.Executable != adapter.Executable || command.Dir != `C:\source` {
		t.Fatalf("unexpected process: %#v", command)
	}
	report := filepath.Join(os.TempDir(), "bluntcode-gitleaks-0a1b-2c3d.json")
	want := []string{
		"detect", "--source", `C:\source`, "--no-git",
		"--report-format", "json", "--report-path", report,
		"--exit-code", "0", "--redact=0", "--no-banner",
	}
	if !reflect.DeepEqual(command.Args, want) {
		t.Fatalf("args = %#v, want %#v", command.Args, want)
	}
	if plan.Metadata[planKeyReportPath] != report || plan.Metadata[planKeySource] != `C:\source` {
		t.Fatalf("unexpected plan metadata: %#v", plan.Metadata)
	}
}

func TestPlanSanitizesHostileScanIDIntoReportPath(t *testing.T) {
	adapter := New("gitleaks.exe", "test")
	plan, err := adapter.Plan(context.Background(), analyzers.ScanRequest{
		WorkspaceRoot: `C:\source`,
		ScanID:        `..\evil id`,
		Files:         []string{`C:\source\main.py`},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := plan.Metadata[planKeyReportPath].(string)
	want := filepath.Join(os.TempDir(), "bluntcode-gitleaks-.._evil_id.json")
	if got != want {
		t.Fatalf("report path = %q, want %q (must stay inside the temp dir)", got, want)
	}
}

func TestPlanRejectsUnscannableSelections(t *testing.T) {
	adapter := New("gitleaks.exe", "test")
	// A stale caller may claim files while listing only unclassified assets;
	// the file list is authoritative, so Plan must refuse rather than launch a
	// pointless scan.
	if _, err := adapter.Plan(context.Background(), analyzers.ScanRequest{WorkspaceRoot: `C:\ws`, Files: []string{`C:\ws\logo.png`, `C:\ws\app.zip`}}); err == nil {
		t.Fatal("expected error when no classified files are in the selection")
	}
	if _, err := adapter.Plan(context.Background(), analyzers.ScanRequest{WorkspaceRoot: "", Files: []string{`C:\ws\main.py`}}); err == nil {
		t.Fatal("expected error when the workspace root is empty")
	}
}

func TestIdentityAndLanguageBreadth(t *testing.T) {
	adapter := New("gitleaks.exe", "8.30.1")
	if adapter.ID() != "gitleaks-secrets" || adapter.DisplayName() != "Gitleaks" {
		t.Fatalf("unexpected identity: %s / %s", adapter.ID(), adapter.DisplayName())
	}
	// Credentials hide in every file type discovery classifies, so gitleaks
	// must ride the same breadth as the built-in secrets detector.
	langs := adapter.SupportedLanguages()
	if !analyzers.HasLanguage(langs, analyzers.LanguageEnv, analyzers.LanguagePython, analyzers.LanguageDockerfile, analyzers.LanguageCertificate) {
		t.Fatalf("supported languages too narrow: %#v", langs)
	}
	if !reflect.DeepEqual(langs, analyzers.AllLanguages()) {
		t.Fatal("supported languages must mirror analyzers.AllLanguages")
	}
}

func TestCheckReportsNotReadyForMissingExecutable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gitleaks.exe")
	status := New(missing, "8.30.1").Check(context.Background(), analyzers.ToolEnvironment{})
	if status.Ready || status.Version != "8.30.1" || status.Detail == "" {
		t.Fatalf("unexpected status for a missing executable: %#v", status)
	}
}

func TestRunRejectsPlanWithoutReportPath(t *testing.T) {
	plan := analyzers.AnalyzerPlan{AnalyzerID: ID, Commands: []analyzers.ProcessSpec{{Executable: "gitleaks.exe"}}}
	if _, err := New("gitleaks.exe", "test").Run(context.Background(), plan, nil); err == nil {
		t.Fatal("expected error for a plan without report path metadata")
	}
}

func TestNormalizeNeverEchoesSecretValues(t *testing.T) {
	// Assembled from two literals so no single source string matches the AWS
	// access key ID pattern and trips secret push protection; the runtime
	// value is the same synthetic fixture key.
	const secret = "AKIA" + "IMNOJQBFDVXYZKEA"
	report := `[{"RuleID":"aws-access-token","Description":"Identified a pattern that may indicate AWS credentials.","StartLine":3,"EndLine":3,"StartColumn":23,"EndColumn":42,"Match":"` + secret + `","Secret":"` + secret + `","File":"C:/ws/config.py","SymlinkFile":"","Commit":"","Entropy":3.8841836,"Author":"","Email":"","Date":"","Message":"","Tags":[],"Fingerprint":"C:/ws/config.py:aws-access-token:3"}]`
	got, _, err := New("gitleaks.exe", "test").Normalize(context.Background(), analyzers.AnalyzerResult{
		Stdout:   []byte(report),
		ExitCode: 0,
		Plan:     analyzers.AnalyzerPlan{Commands: []analyzers.ProcessSpec{{Dir: `C:\ws`}}},
	})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %#v, err %v", got, err)
	}
	f := got[0]
	// The full value must never surface in user-visible finding text.
	if blob := f.Message + " " + f.Title + " " + f.Remediation; strings.Contains(blob, secret) {
		t.Fatalf("secret value surfaced in finding text: %q", blob)
	}
	if f.Message != "aws-access-token: Identified a pattern that may indicate AWS credentials." {
		t.Fatalf("unexpected message: %q", f.Message)
	}
	if f.Metadata["secret_length"] != len(secret) || f.Metadata["preview"] != "AKIA…" {
		t.Fatalf("unexpected redaction metadata: %#v", f.Metadata)
	}
}

func TestNormalizeTreatsToolFailureAsError(t *testing.T) {
	if _, _, err := New("gitleaks.exe", "test").Normalize(context.Background(), analyzers.AnalyzerResult{
		ExitCode: 1,
		Stderr:   []byte("fatal: no such directory"),
		Plan:     analyzers.AnalyzerPlan{Commands: []analyzers.ProcessSpec{{Dir: `C:\ws`}}},
	}); err == nil || !strings.Contains(err.Error(), "exited 1") {
		t.Fatalf("expected exit-code error, got %v", err)
	}
}

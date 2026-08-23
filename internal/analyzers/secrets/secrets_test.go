package secrets

// Adapter contract tests: Check readiness, Plan selection, the Run ->
// Normalize pipeline, redaction in messages, and the scanning hygiene limits
// (binary skip, size cap, per-file cap, total cap). Fixture files are written
// with explicit \n line endings so expectations are byte-stable on Windows.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/discovery"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckAlwaysReady(t *testing.T) {
	status := New().Check(context.Background(), analyzers.ToolEnvironment{})
	if !status.Ready {
		t.Fatalf("built-in detector Check must always be ready, got %+v", status)
	}
	if status.Version == "" {
		t.Fatal("Check must report a version")
	}
}

// TestSupportedLanguagesCoverDiscovery keeps the invariant that secrets sees
// everything discovery classifies: every extension in the classifier's table,
// plus the .env dotfile and Dockerfile basename forms, must be routable to
// this adapter. New languages added to discovery fail here until declared.
func TestSupportedLanguagesCoverDiscovery(t *testing.T) {
	langs := New().SupportedLanguages()
	for ext, lang := range discovery.ExtensionLanguages() {
		if !analyzers.HasLanguage(langs, analyzers.Language(lang)) {
			t.Fatalf("SupportedLanguages %v is missing %s (extension %s)", langs, lang, ext)
		}
	}
	for _, path := range []string{".env", ".env.local", "Dockerfile", "ci/Dockerfile.prod"} {
		lang := discovery.Language(path)
		if lang == "" {
			t.Fatalf("%s is not classified by discovery; the fixture is wrong", path)
		}
		if !analyzers.HasLanguage(langs, analyzers.Language(lang)) {
			t.Fatalf("SupportedLanguages %v is missing %s (basename form %s)", langs, lang, path)
		}
	}
}

func TestPlanSelectsAllLanguageFiles(t *testing.T) {
	adapter := New()
	req := analyzers.ScanRequest{
		WorkspaceRoot: `C:\ws`,
		Languages:     adapter.SupportedLanguages(),
		Files: []string{
			`C:\ws\main.py`,
			`C:\ws\app.tsx`,
			`C:\ws\main.go`,
			`C:\ws\config.env`,  // classified as env; credentials hide here
			`C:\ws\.env`,        // dotfile form of env
			`C:\ws\server.pem`,  // classified as certificate
			`C:\ws\Dockerfile`,  // basename form of dockerfile
			`C:\ws\logo.png`,    // never classified by discovery; filtered like ruff does
			`C:\ws\archive.zip`, // ditto
		},
	}
	plan, err := adapter.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := plan.Metadata[planKeyFiles].([]string)
	want := []string{
		`C:\ws\main.py`, `C:\ws\app.tsx`, `C:\ws\main.go`,
		`C:\ws\config.env`, `C:\ws\.env`, `C:\ws\server.pem`, `C:\ws\Dockerfile`,
	}
	if len(files) != len(want) {
		t.Fatalf("plan files = %v, want %v", files, want)
	}
	for i := range want {
		if files[i] != want[i] {
			t.Fatalf("plan files = %v, want %v", files, want)
		}
	}
	if plan.AnalyzerID != ID || len(plan.Commands) != 0 {
		t.Fatalf("plan must be in-process: id %q, commands %d", plan.AnalyzerID, len(plan.Commands))
	}
}

func TestPlanRejectsEmptySelection(t *testing.T) {
	adapter := New()
	req := analyzers.ScanRequest{WorkspaceRoot: `C:\ws`, Files: []string{`C:\ws\logo.png`, `C:\ws\archive.zip`}}
	if _, err := adapter.Plan(context.Background(), req); err == nil {
		t.Fatal("expected error when the selection has no supported files")
	}
}

func TestRunPlanWithoutSelectionFails(t *testing.T) {
	if _, err := New().Run(context.Background(), analyzers.AnalyzerPlan{AnalyzerID: ID}, nil); err == nil {
		t.Fatal("Run must reject a plan without its file selection")
	}
}

func TestRunNormalizeContract(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "aws.py", "aws_key = \"AKIA1234567890ABCDEF\"\n")
	// The value starts at column 11 on line 2 (t1 o2 k3 e4 n5 space6 :7 =8 space9 quote10).
	writeFile(t, dir, "cfg.ts", "export const region = \"eu-west-1\";\ntoken := \"dfe3a91b77214f0c\"\n")

	adapter := New()
	ctx := context.Background()
	plan, err := adapter.Plan(ctx, analyzers.ScanRequest{WorkspaceRoot: dir, Files: []string{filepath.Join(dir, "aws.py"), filepath.Join(dir, "cfg.ts")}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Run(ctx, plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("built-in run exit code = %d, want 0", result.ExitCode)
	}
	findings, metrics, err := adapter.Normalize(ctx, result)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2: %+v", len(findings), findings)
	}

	byPath := map[string]analyzers.Finding{}
	for _, f := range findings {
		byPath[f.RelativePath] = f
		if err := analyzers.ValidateFinding(f); err != nil {
			t.Fatalf("finding fails validation: %v", err)
		}
		if f.Fingerprint == "" {
			t.Fatalf("finding %s has no fingerprint", f.RuleID)
		}
		if f.Category != analyzers.CategorySecurity {
			t.Fatalf("category = %s, want security", f.Category)
		}
		if f.Remediation == "" {
			t.Fatal("remediation must be populated")
		}
	}

	aws := byPath["aws.py"]
	if aws.RuleID != ruleAWSAccessKey || aws.Severity != analyzers.SeverityHigh {
		t.Fatalf("aws finding rule/severity = %s/%s, want %s/high", aws.RuleID, aws.Severity, ruleAWSAccessKey)
	}
	if aws.StartLine != 1 || aws.StartColumn != 12 {
		t.Fatalf("aws position = (%d,%d), want (1,12) — the secret inside the quotes", aws.StartLine, aws.StartColumn)
	}
	if strings.Contains(aws.Message, "AKIA1234567890ABCDEF") {
		t.Fatalf("message leaks the full secret: %q", aws.Message)
	}
	if !strings.Contains(aws.Message, "AKIA") || !strings.Contains(aws.Message, "20 characters") {
		t.Fatalf("message lacks the redacted preview or length: %q", aws.Message)
	}

	generic := byPath["cfg.ts"]
	if generic.RuleID != ruleGenericAssign || generic.Severity != analyzers.SeverityMedium {
		t.Fatalf("generic finding rule/severity = %s/%s, want %s/medium", generic.RuleID, generic.Severity, ruleGenericAssign)
	}
	if generic.StartLine != 2 || generic.StartColumn != 11 || generic.EndLine != 2 || generic.EndColumn != 27 {
		t.Fatalf("generic position = (%d,%d)-(%d,%d), want (2,11)-(2,27)", generic.StartLine, generic.StartColumn, generic.EndLine, generic.EndColumn)
	}
	if strings.Contains(generic.Message, "dfe3a91b77214f0c") {
		t.Fatalf("message leaks the full secret: %q", generic.Message)
	}
	if !strings.Contains(generic.Message, `"token"`) {
		t.Fatalf("message should name the assignment key: %q", generic.Message)
	}

	filesScanned := false
	for _, m := range metrics {
		if m.AnalyzerID != ID {
			t.Fatalf("metric has analyzer %q, want %s", m.AnalyzerID, ID)
		}
		if m.Key == "files_scanned" && m.Value == 2 {
			filesScanned = true
		}
	}
	if !filesScanned {
		t.Fatalf("metrics missing files_scanned=2: %+v", metrics)
	}
}

// TestRunScansDotenvAndDockerfile is the feature-level proof for the
// broadened routing: a committed AWS key in a .env dotfile and a hardcoded
// token in a Dockerfile are found end to end through Plan -> Run ->
// Normalize, file types the detector could never receive before.
func TestRunScansDotenvAndDockerfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env", "AWS_ACCESS_KEY_ID=AKIA1234567890ABCDEF\n")
	writeFile(t, dir, "Dockerfile", "ENV token = \"dfe3a91b77214f0c\"\n")

	adapter := New()
	ctx := context.Background()
	plan, err := adapter.Plan(ctx, analyzers.ScanRequest{
		WorkspaceRoot: dir,
		Files:         []string{filepath.Join(dir, ".env"), filepath.Join(dir, "Dockerfile")},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Run(ctx, plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	findings, _, err := adapter.Normalize(ctx, result)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2 (one per file): %+v", len(findings), findings)
	}
	byPath := map[string]analyzers.Finding{}
	for _, f := range findings {
		byPath[f.RelativePath] = f
	}
	if f := byPath[".env"]; f.RuleID != ruleAWSAccessKey || f.StartLine != 1 {
		t.Fatalf(".env finding rule/line = %s/%d, want %s/1", f.RuleID, f.StartLine, ruleAWSAccessKey)
	}
	if f := byPath["Dockerfile"]; f.RuleID != ruleGenericAssign {
		t.Fatalf("Dockerfile finding rule = %s, want %s", f.RuleID, ruleGenericAssign)
	}
}

func TestNormalizeEmptyStdoutAndBadExit(t *testing.T) {
	adapter := New()
	ctx := context.Background()
	findings, metrics, err := adapter.Normalize(ctx, analyzers.AnalyzerResult{})
	if err != nil || findings != nil || metrics != nil {
		t.Fatalf("empty result: got (%v, %v, %v), want nils", findings, metrics, err)
	}
	if _, _, err := adapter.Normalize(ctx, analyzers.AnalyzerResult{ExitCode: 2, Stdout: []byte("{}")}); err == nil {
		t.Fatal("non-zero exit code must be an error")
	}
}

func TestRunSkipsBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	// NUL byte inside the first 8 KiB marks the file binary; the planted key
	// after it must never be reported.
	binary := append([]byte{0x00, 0x01, 0x02}, []byte("\naws_key = \"AKIA1234567890ABCDEF\"\n")...)
	path := writeFile(t, dir, "blob.py", string(binary))
	plan := analyzers.AnalyzerPlan{AnalyzerID: ID, Metadata: map[string]any{planKeyFiles: []string{path}, planKeyRoot: dir}}
	result, err := New().Run(context.Background(), plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	findings, metrics, err := New().Normalize(context.Background(), result)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("binary file produced %d findings, want 0", len(findings))
	}
	for _, m := range metrics {
		if m.Key == "files_skipped_binary" && m.Value != 1 {
			t.Fatalf("files_skipped_binary = %v, want 1", m.Value)
		}
	}
}

func TestRunReadsAtMostOneMiB(t *testing.T) {
	dir := t.TempDir()
	near := "aws_key = \"AKIA1234567890ABCDE1\"\n"
	filler := strings.Repeat("# padding line without any secrets at all\n", 40000) // ~1.6 MiB
	far := "aws_key = \"AKIA1234567890ABCDE2\"\n"
	path := writeFile(t, dir, "big.py", near+filler+far)
	if size := len(near + filler + far); size <= maxScanBytes {
		t.Fatalf("fixture is %d bytes, want more than the %d cap", size, maxScanBytes)
	}
	plan := analyzers.AnalyzerPlan{AnalyzerID: ID, Metadata: map[string]any{planKeyFiles: []string{path}, planKeyRoot: dir}}
	result, err := New().Run(context.Background(), plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	findings, _, err := New().Normalize(context.Background(), result)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want exactly 1 (the key inside the cap)", len(findings))
	}
	if findings[0].StartLine != 1 {
		t.Fatalf("finding line = %d, want 1", findings[0].StartLine)
	}
	if strings.Contains(findings[0].Message, "ABCDE2") {
		t.Fatal("found the secret planted beyond the read cap")
	}
}

func TestRunPerFileCapAddsNote(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 0; i < maxFindingsPerFile+10; i++ {
		lines = append(lines, "aws_key = \""+awsKeyForIndex(i)+"\"")
	}
	path := writeFile(t, dir, "flood.py", strings.Join(lines, "\n")+"\n")
	plan := analyzers.AnalyzerPlan{AnalyzerID: ID, Metadata: map[string]any{planKeyFiles: []string{path}, planKeyRoot: dir}}
	result, err := New().Run(context.Background(), plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	var env rawEnvelope
	if err := json.Unmarshal(result.Stdout, &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Diagnostics) != maxFindingsPerFile {
		t.Fatalf("got %d diagnostics, want the per-file cap of %d", len(env.Diagnostics), maxFindingsPerFile)
	}
	if !hasNote(env.Notes, "truncated at the 50-finding per-file limit") {
		t.Fatalf("missing per-file truncation note: %v", env.Notes)
	}
}

func TestRunTotalCapAddsTruncationDiagnostic(t *testing.T) {
	dir := t.TempDir()
	var files []string
	// 105 files x 50 capped findings each exceeds the 5000-finding run cap.
	for i := 0; i < maxFindingsPerRun/maxFindingsPerFile+5; i++ {
		var lines []string
		for j := 0; j < maxFindingsPerFile; j++ {
			lines = append(lines, "aws_key = \""+awsKeyForIndex(i*1000+j)+"\"")
		}
		files = append(files, writeFile(t, dir, "flood"+string(rune('a'+i%26))+string(rune('a'+i/26))+".py", strings.Join(lines, "\n")+"\n"))
	}
	plan := analyzers.AnalyzerPlan{AnalyzerID: ID, Metadata: map[string]any{planKeyFiles: files, planKeyRoot: dir}}
	result, err := New().Run(context.Background(), plan, nil)
	if err != nil {
		t.Fatal(err)
	}
	var env rawEnvelope
	if err := json.Unmarshal(result.Stdout, &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Diagnostics) != maxFindingsPerRun {
		t.Fatalf("got %d diagnostics, want the run cap of %d", len(env.Diagnostics), maxFindingsPerRun)
	}
	if !env.Truncated || !hasNote(env.Notes, "scan truncated at the 5000-finding limit") {
		t.Fatalf("missing total truncation diagnostic: truncated=%v notes=%v", env.Truncated, env.Notes)
	}
}

func TestRunStopsOnCancelledContext(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "one.py", "aws_key = \"AKIA1234567890ABCDEF\"\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	plan := analyzers.AnalyzerPlan{AnalyzerID: ID, Metadata: map[string]any{planKeyFiles: []string{path}, planKeyRoot: dir}}
	if _, err := New().Run(ctx, plan, nil); err == nil {
		t.Fatal("Run must stop when the context is cancelled")
	}
}

func awsKeyForIndex(i int) string {
	return "AKIA" + fmt.Sprintf("%016X", i)
}

func hasNote(notes []string, fragment string) bool {
	for _, n := range notes {
		if strings.Contains(n, fragment) {
			return true
		}
	}
	return false
}

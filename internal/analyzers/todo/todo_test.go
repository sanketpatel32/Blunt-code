package todo

// Adapter contract tests: Check readiness, Plan selection, the Run ->
// Normalize pipeline, and the scanning hygiene limits (binary skip, size cap,
// per-file cap, total cap). Fixture files are written with explicit \n line
// endings so expectations are byte-stable on Windows.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
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
		t.Fatalf("built-in tracker Check must always be ready, got %+v", status)
	}
	if status.Version == "" {
		t.Fatal("Check must report a version")
	}
}

func TestSupportedLanguagesCoverDiscovery(t *testing.T) {
	langs := New().SupportedLanguages()
	for _, want := range []analyzers.Language{analyzers.LanguagePython, analyzers.LanguageJavaScript, analyzers.LanguageTypeScript} {
		if !analyzers.HasLanguage(langs, want) {
			t.Fatalf("SupportedLanguages %v is missing %s", langs, want)
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
			`C:\ws\bundle.mjs`,
			`C:\ws\deploy.pyi`,
			`C:\ws\README.md`,  // never classified by discovery; filtered like ruff does
			`C:\ws\config.env`, // ditto
		},
	}
	plan, err := adapter.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := plan.Metadata[planKeyFiles].([]string)
	want := []string{`C:\ws\main.py`, `C:\ws\app.tsx`, `C:\ws\bundle.mjs`, `C:\ws\deploy.pyi`}
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
	req := analyzers.ScanRequest{WorkspaceRoot: `C:\ws`, Files: []string{`C:\ws\README.md`}}
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
	writeFile(t, dir, "app.py", "import os\n# TODO: add docstring\n# FIXME: handle empty input\n")
	writeFile(t, dir, "ui.tsx", "export const x = 1; // HACK: placeholder\n// XXX: verify types\n// BUG: off by one\n")

	adapter := New()
	ctx := context.Background()
	plan, err := adapter.Plan(ctx, analyzers.ScanRequest{WorkspaceRoot: dir, Files: []string{filepath.Join(dir, "app.py"), filepath.Join(dir, "ui.tsx")}})
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
	if len(findings) != 5 {
		t.Fatalf("got %d findings, want 5: %+v", len(findings), findings)
	}

	byRule := map[string]analyzers.Finding{}
	for _, f := range findings {
		byRule[f.RuleID] = f
		if err := analyzers.ValidateFinding(f); err != nil {
			t.Fatalf("finding fails validation: %v", err)
		}
		if f.AnalyzerID != ID {
			t.Fatalf("analyzer id = %q, want %s", f.AnalyzerID, ID)
		}
		if f.Fingerprint == "" {
			t.Fatalf("finding %s has no fingerprint", f.RuleID)
		}
		if f.Category != analyzers.CategoryMaintainability {
			t.Fatalf("category = %s, want maintainability", f.Category)
		}
		if f.Remediation == "" {
			t.Fatal("remediation must be populated")
		}
	}

	wantSeverity := map[string]analyzers.Severity{
		ruleTODO:  analyzers.SeverityLow,
		ruleFIXME: analyzers.SeverityMedium,
		ruleHACK:  analyzers.SeverityLow,
		ruleXXX:   analyzers.SeverityLow,
		ruleBUG:   analyzers.SeverityMedium,
	}
	for rule, want := range wantSeverity {
		f, ok := byRule[rule]
		if !ok {
			t.Fatalf("missing finding for rule %s", rule)
		}
		if f.Severity != want {
			t.Fatalf("rule %s severity = %s, want %s", rule, f.Severity, want)
		}
		marker := strings.ToUpper(strings.TrimPrefix(rule, ID+"."))
		if !strings.HasPrefix(f.Message, marker) {
			t.Fatalf("rule %s message must lead with the marker: %q", rule, f.Message)
		}
	}

	todo := byRule[ruleTODO]
	if todo.RelativePath != "app.py" || todo.StartLine != 2 || todo.StartColumn != 3 {
		t.Fatalf("todo finding path/position = %s (%d,%d), want app.py (2,3)", todo.RelativePath, todo.StartLine, todo.StartColumn)
	}
	if todo.Message != "TODO: add docstring" {
		t.Fatalf("todo message = %q", todo.Message)
	}
	fixme := byRule[ruleFIXME]
	if fixme.StartLine != 3 || fixme.StartColumn != 3 || fixme.EndLine != 3 || fixme.EndColumn != 8 {
		t.Fatalf("fixme position = (%d,%d)-(%d,%d), want (3,3)-(3,8)", fixme.StartLine, fixme.StartColumn, fixme.EndLine, fixme.EndColumn)
	}
	bug := byRule[ruleBUG]
	if bug.RelativePath != "ui.tsx" || bug.StartLine != 3 {
		t.Fatalf("bug finding path/line = %s/%d, want ui.tsx/3", bug.RelativePath, bug.StartLine)
	}
	if bug.Title != "BUG comment marker" {
		t.Fatalf("title = %q, want a marker-based title", bug.Title)
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
	// NUL byte inside the first 8 KiB marks the file binary; the planted
	// marker after it must never be reported.
	binary := append([]byte{0x00, 0x01, 0x02}, []byte("\n# TODO: never reported\n")...)
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
	near := "// TODO: inside cap\n"
	filler := strings.Repeat("# padding line without any tracked words at all\n", 40000) // ~1.6 MiB
	far := "// TODO: beyond cap\n"
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
		t.Fatalf("got %d findings, want exactly 1 (the marker inside the cap)", len(findings))
	}
	if findings[0].StartLine != 1 {
		t.Fatalf("finding line = %d, want 1", findings[0].StartLine)
	}
	if strings.Contains(findings[0].Message, "beyond cap") {
		t.Fatal("found the marker planted beyond the read cap")
	}
}

func TestRunPerFileCapAddsNote(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 0; i < maxFindingsPerFile+10; i++ {
		lines = append(lines, "// TODO: item")
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
	if !hasNote(env.Notes, "truncated at the 200-finding per-file limit") {
		t.Fatalf("missing per-file truncation note: %v", env.Notes)
	}
}

func TestRunTotalCapAddsTruncationDiagnostic(t *testing.T) {
	dir := t.TempDir()
	var files []string
	// 26 files x 200 capped findings each exceeds the 5000-finding run cap.
	for i := 0; i < maxFindingsPerRun/maxFindingsPerFile+1; i++ {
		var lines []string
		for j := 0; j < maxFindingsPerFile; j++ {
			lines = append(lines, "// TODO: item")
		}
		files = append(files, writeFile(t, dir, "flood"+string(rune('a'+i))+".py", strings.Join(lines, "\n")+"\n"))
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
	path := writeFile(t, dir, "one.py", "# TODO: single\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	plan := analyzers.AnalyzerPlan{AnalyzerID: ID, Metadata: map[string]any{planKeyFiles: []string{path}, planKeyRoot: dir}}
	if _, err := New().Run(ctx, plan, nil); err == nil {
		t.Fatal("Run must stop when the context is cancelled")
	}
}

func hasNote(notes []string, fragment string) bool {
	for _, n := range notes {
		if strings.Contains(n, fragment) {
			return true
		}
	}
	return false
}

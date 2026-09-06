package analyzers

import (
	"context"
	"strings"
	"testing"

	"bluntcode/internal/discovery"
)

// TestLanguageClassificationMirrorsDiscovery guards the deliberate
// duplication between this package's languageExtensions/languageOfPath and
// discovery's classifier: both must map the same extensions to the same
// language names — and apply the same .env*/Dockerfile basename rules — or
// per-adapter file filtering would disagree with how files were discovered.
// discovery does not import this package, so importing it here cannot cycle.
func TestLanguageClassificationMirrorsDiscovery(t *testing.T) {
	discoveryExts := discovery.ExtensionLanguages()
	if len(discoveryExts) <= 10 {
		t.Fatal("discovery extension table unexpectedly small; the mirror assumption is wrong")
	}
	for ext, want := range discoveryExts {
		if got := languageExtensions[ext]; got != Language(want) {
			t.Errorf("extension %s: analyzers classifies %q, discovery %q", ext, got, want)
		}
	}
	for ext := range languageExtensions {
		if _, ok := discoveryExts[ext]; !ok {
			t.Errorf("extension %s: analyzers classifies it but discovery does not", ext)
		}
	}
	for _, path := range []string{".env", ".env.local", "Dockerfile", "ci/Dockerfile.prod", "photo.png", "Makefile", "main.tf", "vars.tfvars", "policy.hcl", "LICENSE", "LICENSE.md", "COPYING.LESSER", "NOTICE", "LICENSE-MIT", "license-checker.config.json"} {
		if got, want := languageOfPath(path), Language(discovery.Language(path)); got != want {
			t.Errorf("path %q: analyzers classifies %q, discovery %q", path, got, want)
		}
	}
	// AllLanguages must cover every language discovery can produce.
	all := AllLanguages()
	for ext, lang := range discoveryExts {
		if !HasLanguage(all, Language(lang)) {
			t.Errorf("AllLanguages is missing %s (extension %s)", lang, ext)
		}
	}
	for _, lang := range []Language{LanguageDockerfile} {
		if !HasLanguage(all, lang) {
			t.Errorf("AllLanguages is missing %s (basename form)", lang)
		}
	}
}

func TestFingerprintIgnoresPositionAndMessageSpacing(t *testing.T) {
	a := Finding{AnalyzerID: "ruff", RuleID: "F401", RelativePath: "src\\app.py", Message: " Imported   module is unused ", StartLine: 1}
	a.SetFingerprint()
	b := a
	b.RelativePath = "src/app.py"
	b.Message = "imported module is unused"
	b.StartLine = 99
	b.SetFingerprint()
	if a.Fingerprint != b.Fingerprint {
		t.Fatal("fingerprint should survive a line move and path separators")
	}
}

func TestSetFingerprintsGivesDuplicatesDistinctIdentities(t *testing.T) {
	dupe := func(line, col int) Finding {
		return Finding{AnalyzerID: "todo", RuleID: "todo", RelativePath: "notes.txt", Message: "TODO fix", StartLine: line, StartColumn: col}
	}
	findings := []Finding{dupe(3, 1), dupe(3, 1), dupe(7, 5), {AnalyzerID: "ruff", RuleID: "F401", RelativePath: "app.py", Message: "unused"}}
	SetFingerprints(findings)
	seen := map[string]bool{}
	for _, f := range findings {
		if seen[f.Fingerprint] {
			t.Fatalf("duplicate fingerprint after assignment: %s", f.Fingerprint)
		}
		seen[f.Fingerprint] = true
	}
	// Occurrence 1 keeps the V1 base identity so suppressions and baselines
	// recorded before V2 still match the first occurrence.
	base := dupe(3, 1)
	base.SetFingerprint()
	if findings[0].Fingerprint != base.Fingerprint {
		t.Fatalf("first occurrence must keep the base fingerprint: %s vs %s", findings[0].Fingerprint, base.Fingerprint)
	}
	if findings[1].Fingerprint == base.Fingerprint {
		t.Fatal("second occurrence must be re-fingerprinted")
	}
	// The unrelated finding is untouched by the dedup machinery (base value).
	solo := findings[3]
	solo.SetFingerprint()
	if findings[3].Fingerprint != solo.Fingerprint {
		t.Fatal("unique findings keep their base fingerprint")
	}
}

// Assignment is stable under input reordering: the occurrence order is derived
// from position, not slice order, so two runs of the same analyzer produce the
// same fingerprint per occurrence.
func TestSetFingerprintsStableUnderReorder(t *testing.T) {
	mk := func(line int) Finding {
		return Finding{AnalyzerID: "biome", RuleID: "noUnusedVars", RelativePath: "a.ts", Message: "x is unused", StartLine: line}
	}
	forward := []Finding{mk(1), mk(2), mk(3)}
	backward := []Finding{mk(3), mk(1), mk(2)}
	SetFingerprints(forward)
	SetFingerprints(backward)
	byLine := func(list []Finding, line int) string {
		for _, f := range list {
			if f.StartLine == line {
				return f.Fingerprint
			}
		}
		return ""
	}
	for _, line := range []int{1, 2, 3} {
		if byLine(forward, line) != byLine(backward, line) {
			t.Fatalf("line %d got different fingerprints depending on slice order", line)
		}
	}
}

func TestRunDirectCapsOutput(t *testing.T) {
	result, err := RunDirect(context.Background(), AnalyzerPlan{AnalyzerID: "test", Commands: []ProcessSpec{{Executable: "powershell.exe", Args: []string{"-NoProfile", "-Command", "[Console]::Out.Write('x' * (9MB))"}}}}, nil)
	if err == nil || !result.OutputTruncated || len(result.Stdout) != 8<<20 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestMergedEnvRemovesEmptyOverride(t *testing.T) {
	values := strings.Join(mergedEnv([]string{"SONAR_TOKEN=external", "PATH=C:\\Windows"}, map[string]string{"SONAR_TOKEN": "", "JAVA_HOME": "C:\\java"}), "\n")
	if strings.Contains(values, "SONAR_TOKEN=") || !strings.Contains(values, "JAVA_HOME=C:\\java") {
		t.Fatalf("unexpected merged environment: %s", values)
	}
}

func TestFileArgumentBatchesStayWithinWindowsCommandLimit(t *testing.T) {
	files := make([]string, 428)
	for i := range files {
		files[i] = `C:\workspace\src\` + strings.Repeat("long-name-", 10) + "file.ts"
	}
	batches := FileArgumentBatches([]string{"scan", "--json"}, files)
	if len(batches) < 2 {
		t.Fatalf("expected multiple batches, got %d", len(batches))
	}
	var count int
	for _, args := range batches {
		if got := commandArgumentLength(args); got > MaxCommandArgumentCharacters {
			t.Fatalf("batch is %d characters", got)
		}
		count += len(args) - 2
	}
	if count != len(files) {
		t.Fatalf("batched %d files, want %d", count, len(files))
	}
}

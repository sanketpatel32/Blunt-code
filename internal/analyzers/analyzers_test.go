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
	for _, path := range []string{".env", ".env.local", "Dockerfile", "ci/Dockerfile.prod", "photo.png", "Makefile"} {
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

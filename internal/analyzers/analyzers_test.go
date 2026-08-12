package analyzers

import (
	"context"
	"strings"
	"testing"
)

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

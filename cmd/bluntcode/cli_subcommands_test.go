package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIDocs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLIDocs(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runCLIDocs with no args returned %d", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "BLUNT CODE CLI REFERENCE MANUAL") {
		t.Errorf("expected reference manual title, got:\n%s", out)
	}
	if !strings.Contains(out, "COMMAND CATEGORIES") {
		t.Errorf("expected command categories, got:\n%s", out)
	}

	for _, cmd := range []string{"scan", "workspace", "findings", "history", "report", "suppress", "rules", "tools", "pentest", "stats", "doctor", "config", "agent"} {
		stdout.Reset()
		code = runCLIDocs([]string{cmd}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("runCLIDocs(%q) returned %d", cmd, code)
		}
		if stdout.Len() == 0 {
			t.Errorf("runCLIDocs(%q) produced no output", cmd)
		}
	}

	stdout.Reset()
	stderr.Reset()
	code = runCLIDocs([]string{"nonexistent-command"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("expected exit 2 for unknown command, got %d", code)
	}
}

func TestCommandHelpOutputs(t *testing.T) {
	tests := []struct {
		name string
		fn   func([]string, *bytes.Buffer, *bytes.Buffer) int
		want string
	}{
		{"workspace", func(args []string, o, e *bytes.Buffer) int { return runWorkspace(args, o, e) }, "Blunt Code workspace management"},
		{"findings", func(args []string, o, e *bytes.Buffer) int { return runFindings(args, o, e) }, "Blunt Code findings search and inspection"},
		{"history", func(args []string, o, e *bytes.Buffer) int { return runHistory(args, o, e) }, "Blunt Code scan history and comparison"},
		{"report", func(args []string, o, e *bytes.Buffer) int { return runReport(args, o, e) }, "Blunt Code report exporter"},
		{"suppress", func(args []string, o, e *bytes.Buffer) int { return runSuppress(args, o, e) }, "Blunt Code finding suppressions management"},
		{"rules", func(args []string, o, e *bytes.Buffer) int { return runRules(args, o, e) }, "Blunt Code workspace analysis rules and path overrides"},
		{"tools", func(args []string, o, e *bytes.Buffer) int { return runTools(args, o, e) }, "Blunt Code managed analyzer toolchain"},
		{"pentest", func(args []string, o, e *bytes.Buffer) int { return runPentest(args, o, e) }, "Blunt Code dynamic pentest and DAST security probing"},
		{"stats", func(args []string, o, e *bytes.Buffer) int { return runStats(args, o, e) }, "Blunt Code statistics, severity trends, and risk metrics"},
		{"update", func(args []string, o, e *bytes.Buffer) int { return runUpdate(args, o, e) }, "Blunt Code update check"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := tc.fn([]string{"--help"}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("%s --help returned %d", tc.name, code)
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Errorf("%s --help output missing %q:\n%s", tc.name, tc.want, stdout.String())
			}
		})
	}
}

func TestCLISubcommandsIntegration(t *testing.T) {
	tempDir := t.TempDir()
	origLocal := os.Getenv("LOCALAPPDATA")
	t.Setenv("LOCALAPPDATA", tempDir)
	_ = origLocal

	// Create a dummy workspace folder
	dummyWork := filepath.Join(tempDir, "test-workspace")
	if err := os.MkdirAll(filepath.Join(dummyWork, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dummyWork, "src", "main.py"), []byte("print('hello')\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer

	// 1. workspace add
	code := runWorkspace([]string{"add", dummyWork, "--name", "my-test-app"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("workspace add failed (%d): %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "my-test-app") {
		t.Errorf("workspace add output missing name: %s", stdout.String())
	}

	// 2. workspace list
	stdout.Reset()
	code = runWorkspace([]string{"list"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("workspace list failed (%d): %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "my-test-app") {
		t.Errorf("workspace list output missing my-test-app: %s", stdout.String())
	}

	// 3. workspace show
	stdout.Reset()
	code = runWorkspace([]string{"show", "my-test-app"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("workspace show failed (%d): %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "my-test-app") {
		t.Errorf("workspace show output missing my-test-app: %s", stdout.String())
	}

	// 4. workspace tags
	stdout.Reset()
	code = runWorkspace([]string{"tags", "my-test-app", "--set", "test,cli"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("workspace tags failed (%d): %s", code, stderr.String())
	}
	stdout.Reset()
	code = runWorkspace([]string{"tags", "my-test-app"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "cli") || !strings.Contains(stdout.String(), "test") {
		t.Errorf("workspace tags get output: %s", stdout.String())
	}

	// 5. workspace tree
	stdout.Reset()
	code = runWorkspace([]string{"tree", "my-test-app", "--path", "src"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("workspace tree failed (%d): %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "main.py") {
		t.Errorf("workspace tree output missing main.py: %s", stdout.String())
	}

	// 6. rules overrides
	stdout.Reset()
	code = runRules([]string{"overrides", "my-test-app", "--set", "dist/**"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("rules overrides set failed (%d): %s", code, stderr.String())
	}
	stdout.Reset()
	code = runRules([]string{"overrides", "my-test-app"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "dist/**") {
		t.Errorf("rules overrides get output: %s", stdout.String())
	}

	// 7. suppress add, list, remove
	stdout.Reset()
	code = runSuppress([]string{"add", "my-test-app", "--fingerprint", "deadbeef0123456789abcdef", "--reason", "false positive"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("suppress add failed (%d): %s", code, stderr.String())
	}
	stdout.Reset()
	code = runSuppress([]string{"list", "my-test-app"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "deadbeef0123456789abcdef") {
		t.Errorf("suppress list output: %s", stdout.String())
	}
	stdout.Reset()
	code = runSuppress([]string{"remove", "my-test-app", "--fingerprint", "deadbeef0123456789abcdef"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("suppress remove failed (%d): %s", code, stderr.String())
	}

	// 8. stats
	stdout.Reset()
	code = runStats([]string{"my-test-app"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("stats failed (%d): %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "my-test-app") {
		t.Errorf("stats output: %s", stdout.String())
	}

	// 9. workspace delete
	stdout.Reset()
	code = runWorkspace([]string{"delete", "my-test-app"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("workspace delete failed (%d): %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Deleted workspace") {
		t.Errorf("workspace delete output: %s", stdout.String())
	}
}

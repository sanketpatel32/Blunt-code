package gitleaks

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

func TestPlanPinsConfigAndIsolatesWorkingDirectoryByDefault(t *testing.T) {
	root := t.TempDir()
	adapter := New("gitleaks", "8.30.1")
	plan, err := adapter.Plan(context.Background(), analyzers.ScanRequest{ScanID: "pin", WorkspaceRoot: root, Files: []string{filepath.Join(root, "a.go")}, Languages: []analyzers.Language{analyzers.LanguageGo}})
	if err != nil {
		t.Fatal(err)
	}
	command := plan.Commands[0]
	workDir, _ := plan.Metadata[planKeyWorkDir].(string)
	defer os.RemoveAll(workDir)
	if workDir == "" || command.Dir != workDir || strings.HasPrefix(workDir, root) {
		t.Fatalf("Dir = %q, work dir = %q: gitleaks must not run from the workspace", command.Dir, workDir)
	}
	config := flagValue(command.Args, "--config")
	if config == "" || strings.HasPrefix(config, root) {
		t.Fatalf("--config = %q, want a Blunt Code-owned file", config)
	}
	if data, err := os.ReadFile(config); err != nil || string(data) != pinnedConfig {
		t.Fatalf("pinned config = %q, %v", data, err)
	}
	if flagValue(command.Args, "--source") != root {
		t.Fatalf("--source must still be the workspace: %#v", command.Args)
	}
}

func TestIgnoreTargetsResolvesFingerprintsInsideRoot(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("sub/creds.txt", "x")
	write("other.env", "y")
	write(ignoreFileName, strings.Join([]string{
		"# comment",
		filepath.Join(root, "sub", "creds.txt") + ":aws-access-token:1",
		"abc1234:other.env:generic-api-key:3",
		"sub/creds.txt:github-pat:2",
		"../escape.txt:rule:1",
		"missing.txt:rule:1",
		".gitleaksignore:rule:1",
		"garbage",
	}, "\n"))
	got := ignoreTargets(root)
	want := []string{"sub/creds.txt", "other.env"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
}

func TestMergeReportsReplacesStagedFiles(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "ws")
	stage := filepath.Join(string(filepath.Separator), "stage")
	mainReport, _ := json.Marshal([]map[string]any{
		{"File": filepath.Join(root, "sub", "creds.txt"), "RuleID": "aws-access-token", "StartLine": 1},
		{"File": filepath.Join(root, "keep.txt"), "RuleID": "generic-api-key", "StartLine": 4},
	})
	stagedFile := filepath.Join(stage, "sub", "creds.txt")
	stagedReport, _ := json.Marshal([]map[string]any{
		{"File": stagedFile, "RuleID": "aws-access-token", "StartLine": 1, "Fingerprint": stagedFile + ":aws-access-token:1"},
		{"File": stagedFile, "RuleID": "github-pat", "StartLine": 2, "Fingerprint": stagedFile + ":github-pat:2"},
	})
	merged, err := mergeReports(mainReport, stagedReport, root, stage, []string{"sub/creds.txt"})
	if err != nil {
		t.Fatal(err)
	}
	var got []rawFinding
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("merged = %s, want keep.txt plus both staged findings", merged)
	}
	mapped := filepath.Join(root, "sub", "creds.txt")
	for _, f := range got[1:] {
		if f.File != mapped || !strings.HasPrefix(f.Fingerprint, mapped+":") {
			t.Fatalf("staged finding not mapped back to the workspace: %#v", f)
		}
	}
}

func TestPlanTrustedKeepsNativeConfigDiscovery(t *testing.T) {
	root := t.TempDir()
	plan, err := New("gitleaks", "8.30.1").Plan(context.Background(), analyzers.ScanRequest{TrustRepoConfig: true, WorkspaceRoot: root, Files: []string{filepath.Join(root, "a.go")}, Languages: []analyzers.Language{analyzers.LanguageGo}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Commands[0].Dir != root || flagValue(plan.Commands[0].Args, "--config") != "" {
		t.Fatalf("trusted plan must keep gitleaks' own config discovery: %#v", plan.Commands[0])
	}
}

func flagValue(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

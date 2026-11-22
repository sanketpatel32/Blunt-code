package trivy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

func TestPlanPinsConfigAndIsolatesWorkingDirectoryByDefault(t *testing.T) {
	root := t.TempDir()
	adapter := New("trivy", "0.74.0")
	adapter.CacheDir = filepath.Join(t.TempDir(), "cache")
	plan, err := adapter.Plan(context.Background(), analyzers.ScanRequest{WorkspaceRoot: root, Files: []string{filepath.Join(root, "Dockerfile")}, Languages: []analyzers.Language{analyzers.LanguageDockerfile}, Profile: analyzers.ProfileDeep})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(plan.Metadata[planKeyOutput].(string))
	workDir, _ := plan.Metadata[planKeyWorkDir].(string)
	defer os.RemoveAll(workDir)
	command := plan.Commands[0]
	if workDir == "" || command.Dir != workDir || strings.HasPrefix(workDir, root) {
		t.Fatalf("Dir = %q: trivy must not run from the workspace", command.Dir)
	}
	if command.Args[len(command.Args)-1] != root {
		t.Fatalf("target = %q, want the workspace root", command.Args[len(command.Args)-1])
	}
	for flag, mustExist := range map[string]bool{"--config": true, "--ignorefile": true, "--secret-config": false} {
		value := flagValue(command.Args, flag)
		if value == "" || !strings.HasPrefix(value, workDir) {
			t.Fatalf("%s = %q, want a path in the isolated dir", flag, value)
		}
		if _, err := os.Stat(value); (err == nil) != mustExist {
			t.Fatalf("%s %q existence = %v, want %v", flag, value, err == nil, mustExist)
		}
	}
	if got := relative(plan, filepath.Join(root, "sub", "Dockerfile")); got != "sub/Dockerfile" {
		t.Fatalf("relative = %q", got)
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

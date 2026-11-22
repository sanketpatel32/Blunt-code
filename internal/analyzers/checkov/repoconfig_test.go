package checkov

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
)

func TestPlanAvoidsCheckovDefaultConfigLookupByDefault(t *testing.T) {
	root := t.TempDir()
	plan, err := New("python", "3.3.16").Plan(context.Background(), analyzers.ScanRequest{ScanID: "pin", WorkspaceRoot: root, Files: []string{filepath.Join(root, "Dockerfile")}, Languages: []analyzers.Language{analyzers.LanguageDockerfile}, Profile: analyzers.ProfileDeep})
	if err != nil {
		t.Fatal(err)
	}
	workDir, _ := plan.Metadata[planKeyWorkDir].(string)
	defer os.RemoveAll(workDir)
	command := plan.Commands[0]
	if workDir == "" || command.Dir != workDir || strings.HasPrefix(workDir, root) {
		t.Fatalf("Dir = %q: checkov must not run from the workspace", command.Dir)
	}
	// checkov's get_default_config_paths reads .checkov.yaml after every
	// separate -d/--directory token; only the joined form avoids it.
	for _, arg := range command.Args {
		if arg == "-d" || arg == "--directory" {
			t.Fatalf("args %#v use a separate directory token", command.Args)
		}
	}
	found := false
	for _, arg := range command.Args {
		found = found || arg == "--directory="+root
	}
	if !found {
		t.Fatalf("args %#v do not scan the workspace", command.Args)
	}
	if got := relative(plan.Metadata[planKeyRoot].(string), filepath.Join(root, "main.tf")); got != "main.tf" {
		t.Fatalf("relative = %q", got)
	}
}

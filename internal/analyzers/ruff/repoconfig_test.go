package ruff

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"bluntcode/internal/analyzers"
)

func TestPlanIsolatesRuffFromRepoConfigByDefault(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "app.py")
	if err := os.WriteFile(file, []byte("import os\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, trust := range []bool{false, true} {
		plan, err := New("ruff", "0.16.0").Plan(context.Background(), analyzers.ScanRequest{WorkspaceRoot: root, Files: []string{file}, Languages: []analyzers.Language{analyzers.LanguagePython}, TrustRepoConfig: trust})
		if err != nil {
			t.Fatal(err)
		}
		isolated := false
		for _, arg := range plan.Commands[0].Args {
			isolated = isolated || arg == "--isolated"
		}
		if isolated == trust {
			t.Fatalf("trust=%v: --isolated present=%v in %#v", trust, isolated, plan.Commands[0].Args)
		}
	}
}

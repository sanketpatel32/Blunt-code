package analyzers

import (
	"path/filepath"
	"testing"
)

func TestRelativePath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	cases := []struct {
		name, root, path, want string
	}{
		{"inside root", root, filepath.Join(root, "src", "app.py"), "src/app.py"},
		{"root itself", root, root, "."},
		{"empty root keeps path", "", filepath.Join("src", "app.py"), "src/app.py"},
		{"unrelatable keeps path", root, filepath.Join("src", "app.py"), "src/app.py"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RelativePath(tc.root, tc.path); got != tc.want {
				t.Fatalf("RelativePath(%q, %q) = %q, want %q", tc.root, tc.path, got, tc.want)
			}
		})
	}
}

func TestPlanRelativePath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "repo")
	file := filepath.Join(dir, "pkg", "main.py")
	if got := PlanRelativePath(AnalyzerPlan{Commands: []ProcessSpec{{Dir: dir}}}, file); got != "pkg/main.py" {
		t.Fatalf("with command dir = %q, want pkg/main.py", got)
	}
	if got := PlanRelativePath(AnalyzerPlan{}, file); got != filepath.ToSlash(file) {
		t.Fatalf("without commands = %q, want %q", got, filepath.ToSlash(file))
	}
}

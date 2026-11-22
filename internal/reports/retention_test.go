package reports

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveFilesDeletesReportsInsideReportsDir(t *testing.T) {
	dir := t.TempDir()
	report := filepath.Join(dir, "blunt-code-x.md")
	if err := os.WriteFile(report, []byte("# r"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "already-gone.md")
	if err := RemoveFiles(dir, []string{report, missing, ""}); err != nil {
		t.Fatalf("RemoveFiles: %v", err)
	}
	if _, err := os.Stat(report); !os.IsNotExist(err) {
		t.Fatalf("report still on disk: %v", err)
	}
}

func TestRemoveFilesRefusesPathsOutsideReportsDir(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "important.md")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "sub", "nested.md")
	if err := os.MkdirAll(filepath.Dir(nested), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveFiles(dir, []string{outside, nested}); err == nil {
		t.Fatal("RemoveFiles accepted paths outside the reports directory")
	}
	for _, path := range []string{outside, nested} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s was removed: %v", path, err)
		}
	}
}

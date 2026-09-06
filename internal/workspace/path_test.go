package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateRelativePathRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	if _, err := ValidateRelativePath(root, "..\\escape"); err == nil {
		t.Fatal("traversal accepted")
	}
	if _, err := ValidateRelativePath(root, filepath.Join("..", "escape")); err == nil {
		t.Fatal("traversal accepted")
	}
}

// TestValidateRelativePathResolvesJunctionEscapes pins the containment fix: a
// junction inside the workspace that points outside must not validate paths
// beneath it, even though the path is lexically inside root and the endpoint
// is a file (EvalSymlinks alone leaves junctions unresolved).
func TestValidateRelativePathResolvesJunctionEscapes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are a Windows reparse-point behavior")
	}
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("classified"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
		t.Skipf("cannot create junction: %v: %s", err, out)
	}

	if _, err := ValidateRelativePath(root, filepath.Join("linked", "secret.txt")); err == nil {
		t.Fatal("path through an escaping junction must be rejected")
	}
	if _, err := ValidateRelativePath(root, "linked"); err == nil {
		t.Fatal("the escaping junction directory itself must be rejected")
	}

	// A junction that stays inside root is legitimate project structure and
	// must keep validating (before junction resolution ran first, these died
	// with a bogus "resolve requested path" error because EvalSymlinks
	// cannot walk through a junction).
	inner := filepath.Join(root, "packages")
	if err := os.MkdirAll(inner, 0o700); err != nil {
		t.Fatal(err)
	}
	innerFile := filepath.Join(inner, "notes.md")
	if err := os.WriteFile(innerFile, []byte("shared"), 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", alias, inner).CombinedOutput(); err != nil {
		t.Skipf("cannot create junction: %v: %s", err, out)
	}
	if clean, err := ValidateRelativePath(root, filepath.Join("alias", "notes.md")); err != nil || clean != filepath.Join("alias", "notes.md") {
		t.Fatalf("path through an internal junction must validate: %q %v", clean, err)
	}

	inside := filepath.Join(root, "readme.md")
	if err := os.WriteFile(inside, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if clean, err := ValidateRelativePath(root, "readme.md"); err != nil || clean != "readme.md" {
		t.Fatalf("a regular inside file must validate: %q %v", clean, err)
	}
}

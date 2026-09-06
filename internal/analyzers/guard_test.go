package analyzers_test

// Containment tests for the shared RelativeInside guard that adapters apply
// before handing discovered files to external tools or reading them
// in-process. The lexically-inside-but-physically-outside junction case is
// the one that motivated the guard: a reparse point inside the workspace can
// alias any directory on disk, so containment must hold after resolution, not
// just on the raw string.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"bluntcode/internal/analyzers"
)

func TestRelativeInsideContainment(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "app.py")
	if err := os.WriteFile(inside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "app.py")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !analyzers.RelativeInside(root, inside) {
		t.Fatal("a file inside root must pass")
	}
	if analyzers.RelativeInside(root, outside) {
		t.Fatal("a file outside root must be dropped")
	}
	// Non-existent paths cannot leak anything; the lexical verdict stands.
	if !analyzers.RelativeInside(root, filepath.Join(root, "gone.py")) {
		t.Fatal("a non-existent path under root must fail open")
	}
	if !analyzers.RelativeInside("", inside) {
		t.Fatal("an empty root disables containment by contract")
	}
}

func TestRelativeInsideResolvesJunctionEscapes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are a Windows reparse-point behavior")
	}
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "app.py")
	if err := os.WriteFile(secret, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
		t.Skipf("cannot create junction: %v: %s", err, out)
	}

	if analyzers.RelativeInside(root, filepath.Join(link, "app.py")) {
		t.Fatal("a path through an escaping junction must be dropped")
	}
	if analyzers.RelativeInside(root, link) {
		// The junction directory itself resolves outside root, so it must be
		// dropped too — but the file case above is the load-bearing one; keep
		// both pinned.
		t.Fatal("an escaping junction directory must be dropped")
	}
}

func TestFilesForLanguagesInsideCombinesRoutingAndContainment(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junction fixture requires Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	insidePython := filepath.Join(root, "app.py")
	insideText := filepath.Join(root, "notes.txt")
	for _, file := range []string{insidePython, insideText, filepath.Join(outside, "app.py")} {
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(root, "linked")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, outside).CombinedOutput(); err != nil {
		t.Skipf("cannot create junction: %v: %s", err, out)
	}

	files := []string{
		insidePython,
		insideText,
		filepath.Join(outside, "app.py"),
		filepath.Join(link, "app.py"),
	}
	got := analyzers.FilesForLanguagesInside(root, files, analyzers.LanguagePython)
	if len(got) != 1 || got[0] != insidePython {
		t.Fatalf("want only the inside Python file, got %v", got)
	}
}

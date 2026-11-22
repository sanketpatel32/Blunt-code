package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsGitRepositoryDetectsDirectoryEntry(t *testing.T) {
	root := t.TempDir()
	if IsGitRepository(root) {
		t.Fatal("plain directory must not be detected as a git repository")
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !IsGitRepository(root) {
		t.Fatal("directory with .git entry must be detected as a git repository")
	}
}

func TestIsGitRepositoryDetectsWorktreeFile(t *testing.T) {
	root := t.TempDir()
	// Linked worktrees and submodules carry a `.git` file, not a directory.
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: elsewhere"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsGitRepository(root) {
		t.Fatal(".git file (linked worktree) must count as a git repository")
	}
}

func TestIsGitRepositoryFollowsParentEnclosure(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "src", "pkg")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	// Scanning a folder inside a repo still has git provenance — matching
	// `git rev-parse --is-inside-work-tree`, which climbs to the parent.
	if !IsGitRepository(nested) {
		t.Fatal("nested folder inside a working tree must be detected")
	}
}

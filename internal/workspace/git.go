package workspace

import (
	"os"
	"path/filepath"
)

// IsGitRepository reports whether root is inside a git working tree — the
// same answer `git rev-parse --is-inside-work-tree` gives, which is what
// scan provenance already relies on. It is a filesystem probe, not a `git`
// subprocess: `.git` exists as a directory in a normal clone and as a file
// (`gitdir: <path>`) in a linked worktree or submodule checkout, so walking
// ancestors for either covers regular, nested, and linked layouts while
// keeping workspace creation free of a PATH dependency. A bare repository's
// root IS its git directory and is intentionally not detected: it has no
// working tree to scan.
func IsGitRepository(root string) bool {
	dir, err := filepath.Abs(root)
	if err != nil {
		dir = root
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

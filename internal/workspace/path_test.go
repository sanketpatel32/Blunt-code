package workspace

import (
	"path/filepath"
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

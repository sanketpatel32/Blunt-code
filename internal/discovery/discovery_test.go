package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSkipsDefaultsAndDetectsLanguages(t *testing.T) {
	root := t.TempDir()
	for path := range map[string]string{"main.py": "x", "ui/app.tsx": "x", "node_modules/no.js": "x", "dist/out.js": "x", "a.min.js": "x"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Discover(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 2 || got.Languages["python"] != 1 || got.Languages["typescript"] != 1 {
		t.Fatalf("unexpected discovery: %+v", got)
	}
}
func TestLanguage(t *testing.T) {
	if Language("A.TS") != "typescript" || Language("a.txt") != "" {
		t.Fatal("language detection wrong")
	}
}

func TestTreeReturnsOnlyImmediateSafeChildren(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "nested.py"), []byte("x=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.py"), []byte("x=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0o700); err != nil {
		t.Fatal(err)
	}
	items, err := Tree(context.Background(), root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].RelativePath != "main.py" || items[0].IsDir || items[1].RelativePath != "src" || !items[1].IsDir {
		t.Fatalf("unexpected root tree: %#v", items)
	}
	items, err = Tree(context.Background(), root, "src", nil)
	if err != nil || len(items) != 1 || items[0].RelativePath != "src/nested.py" {
		t.Fatalf("unexpected child tree: %#v (%v)", items, err)
	}
}

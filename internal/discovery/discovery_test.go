package discovery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"bluntcode/internal/core"
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

// ---------------------------------------------------------------------------
// Large-volume responsiveness guards. Realistic filesystem shapes (a wide
// directory for the tree endpoint, a deep nesting chain for discovery) with
// generous budgets so CI jitter cannot flake while real regressions fail.
// Seeds are skipped under -short.
// ---------------------------------------------------------------------------

// measureDiscovery runs one warm-up (also failing on op errors) plus timed
// iterations and returns the p95 and worst observed duration.
func measureDiscovery(t *testing.T, iterations int, op func() error) (time.Duration, time.Duration) {
	t.Helper()
	if err := op(); err != nil {
		t.Fatal(err)
	}
	samples := make([]time.Duration, 0, iterations)
	for i := 0; i < iterations; i++ {
		start := time.Now()
		if err := op(); err != nil {
			t.Fatal(err)
		}
		samples = append(samples, time.Since(start))
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	idx := len(samples) * 95 / 100
	if idx >= len(samples) {
		idx = len(samples) - 1
	}
	return samples[idx], samples[len(samples)-1]
}

func assertDiscoveryBudget(t *testing.T, name string, p95, worst, budget time.Duration) {
	t.Helper()
	t.Logf("%s: p95=%v worst=%v budget=%v", name, p95, worst, budget)
	if p95 > budget {
		t.Fatalf("%s p95 %v exceeded budget %v", name, p95, budget)
	}
}

// TestTreeLargeDirectoryResponsiveness guards the workspace tree endpoint's
// data path: one directory holding 2,000 child entries (mixed files and
// folders) must list immediate children without walking anything deeper.
func TestTreeLargeDirectoryResponsiveness(t *testing.T) {
	if testing.Short() {
		t.Skip("large-volume seed skipped in -short mode")
	}
	root := t.TempDir()
	wide := filepath.Join(root, "wide")
	if err := os.MkdirAll(wide, 0o700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2000; i++ {
		name := fmt.Sprintf("file%04d.ts", i)
		if i%10 == 0 {
			name = fmt.Sprintf("dir%04d", i)
			if err := os.Mkdir(filepath.Join(wide, name), 0o700); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(filepath.Join(wide, name), []byte("export const x = 1;\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var items []core.FileEntry
	p95, worst := measureDiscovery(t, 10, func() error {
		var err error
		items, err = Tree(context.Background(), root, "wide", nil)
		return err
	})
	// 1,800 files + 200 directories survive filtering; the skipped entries are
	// only the non-source file extensions (Language returns "" for them).
	if len(items) == 0 {
		t.Fatal("tree returned no children")
	}
	if len(items) != 1800+200 {
		t.Fatalf("tree children=%d want 2000 (files+dirs)", len(items))
	}
	assertDiscoveryBudget(t, "Tree/2000 child entries", p95, worst, 150*time.Millisecond)
}

// TestDiscoverDeepTreeResponsiveness guards the discovery walk on a depth-15
// nesting chain with several files per level plus skipped heavy directories,
// approximating a hostile monorepo layout.
func TestDiscoverDeepTreeResponsiveness(t *testing.T) {
	if testing.Short() {
		t.Skip("large-volume seed skipped in -short mode")
	}
	root := t.TempDir()
	depth := 15
	dir := root
	for level := 0; level < depth; level++ {
		dir = filepath.Join(dir, fmt.Sprintf("level%02d", level))
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		for file := 0; file < 12; file++ {
			ext := ".py"
			if file%2 == 0 {
				ext = ".ts"
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("mod%02d%s", file, ext)), []byte("x = 1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if level%5 == 0 {
			// Skipped-by-default directories that must not be entered.
			if err := os.MkdirAll(filepath.Join(dir, "node_modules", "pkg", "deep"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "node_modules", "pkg", "deep", "bundled.js"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	var result Result
	p95, worst := measureDiscovery(t, 5, func() error {
		var err error
		result, err = Discover(context.Background(), root, nil)
		return err
	})
	if len(result.Files) != depth*12 {
		t.Fatalf("discovered files=%d want %d", len(result.Files), depth*12)
	}
	if result.Languages["python"] != depth*6 || result.Languages["typescript"] != depth*6 {
		t.Fatalf("language counts wrong: %#v", result.Languages)
	}
	assertDiscoveryBudget(t, "Discover/depth-15 tree", p95, worst, 300*time.Millisecond)
}

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
	if Language("A.TS") != "typescript" || Language("a.bin") != "" {
		t.Fatal("language detection wrong")
	}
}

// TestLanguageClassificationTable pins the full extension map: the original
// python/javascript/typescript names are asserted exactly (they are load-
// bearing for routing and fingerprints), every broadened extension maps to
// its language case-insensitively, and unknown extensions stay "" so those
// files remain non-candidates exactly as before the broadening.
func TestLanguageClassificationTable(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		// Original trio, unchanged.
		{"main.py", "python"}, {"stub.pyi", "python"},
		{"app.ts", "typescript"}, {"ui.tsx", "typescript"}, {"mod.mts", "typescript"}, {"mod.cts", "typescript"},
		{"index.js", "javascript"}, {"comp.jsx", "javascript"}, {"lib.mjs", "javascript"}, {"lib.cjs", "javascript"},
		// Code.
		{"main.go", "go"}, {"App.java", "java"}, {"Main.kt", "kotlin"}, {"build.gradle.kts", "kotlin"},
		{"Program.cs", "csharp"}, {"main.c", "c"}, {"header.h", "c"},
		{"impl.cpp", "cpp"}, {"header.hpp", "cpp"}, {"source.cc", "cpp"},
		{"app.rb", "ruby"}, {"index.php", "php"}, {"main.rs", "rust"},
		{"View.swift", "swift"}, {"Job.scala", "scala"},
		{"AppDelegate.m", "objective-c"}, {"Bridge.mm", "objective-c"},
		{"App.vue", "vue"}, {"Widget.svelte", "svelte"},
		// Web and data.
		{"style.css", "css"}, {"style.scss", "scss"}, {"style.less", "less"},
		{"page.html", "html"}, {"page.htm", "html"},
		{"data.json", "json"}, {"tsconfig.jsonc", "json"},
		{"ci.yaml", "yaml"}, {"cfg.yml", "yaml"},
		{"pyproject.toml", "toml"}, {"pom.xml", "xml"}, {"query.sql", "sql"}, {"schema.graphql", "graphql"},
		// Shell and scripts.
		{"run.sh", "shell"}, {"run.bash", "shell"}, {"run.zsh", "shell"},
		{"task.ps1", "powershell"}, {"build.bat", "batch"}, {"win.cmd", "batch"},
		// Config, docs, credentials.
		{"README.md", "markdown"}, {"NOTES.markdown", "markdown"}, {"notes.txt", "text"},
		{"app.ini", "ini"}, {"app.cfg", "ini"}, {"app.conf", "ini"}, {"app.properties", "properties"},
		{"config.env", "env"}, {"server.pem", "certificate"}, {"id_rsa.key", "certificate"}, {"id_rsa.pub", "certificate"},
		// Case-insensitive matching, old and new alike.
		{"A.TS", "typescript"}, {"MAIN.GO", "go"}, {"STYLE.SCSS", "scss"}, {"TASK.PS1", "powershell"},
		// Unknown extensions and basenames stay unclassified.
		{"photo.png", ""}, {"PHOTO.PNG", ""}, {"archive.zip", ""}, {"binary.exe", ""}, {"data.bin", ""},
		{"Makefile", ""}, {"app.dockerfile", ""}, {"mydockerfile", ""}, {".envrc", ""},
	}
	for _, tc := range cases {
		if got := Language(tc.path); got != tc.want {
			t.Errorf("Language(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// TestLanguageDotfilesAndDockerfile pins the basename rules: dotfile env
// variants and extension-less Dockerfiles are candidates, extension matching
// keeps precedence over basename matching (dockerfile.go is Go source), and
// look-alike names stay out.
func TestLanguageDotfilesAndDockerfile(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{".env", "env"},
		{".env.local", "env"},
		{".env.production", "env"},
		{".ENV", "env"},
		{".Env.Production", "env"},
		{"nested/.env", "env"},
		{"Dockerfile", "dockerfile"},
		{"Dockerfile.dev", "dockerfile"},
		{"dockerfile", "dockerfile"},
		{"DOCKERFILE", "dockerfile"},
		{"ci/Dockerfile.prod", "dockerfile"},
		{"dockerfile.go", "go"},    // extension wins over basename
		{"compose.dockerfile", ""}, // basename not anchored at "dockerfile"
		{".envrc", ""},             // ".env." prefix does not swallow .envrc
		{"env", ""},                // bare name is not a dotfile form
	}
	for _, tc := range cases {
		if got := Language(tc.path); got != tc.want {
			t.Errorf("Language(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// TestDiscoverClassifiesConfigAndCredentialCandidates proves the broadened
// classifier changes candidacy end to end: .env dotfiles, certificates,
// Dockerfiles, shell scripts, and markdown become scan candidates, while
// binaries, assets, and the pre-existing suffix exclusions stay out.
func TestDiscoverClassifiesConfigAndCredentialCandidates(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"main.py":         "x",
		".env":            "x",
		".env.local":      "x",
		"server.pem":      "x",
		"Dockerfile":      "x",
		"deploy/build.sh": "x",
		"README.md":       "x",
		"logo.png":        "x",
		"photo.jpg":       "x",
		"app.min.js":      "x",
		"bundle.map":      "x",
		"module.pyc":      "x",
	}
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Discover(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, file := range got.Files {
		paths[file.RelativePath] = true
	}
	want := map[string]bool{
		"main.py": true, ".env": true, ".env.local": true, "server.pem": true,
		"Dockerfile": true, "deploy/build.sh": true, "README.md": true,
	}
	if len(paths) != len(want) {
		t.Fatalf("discovered %v, want exactly %v", paths, want)
	}
	for path := range want {
		if !paths[path] {
			t.Fatalf("%s missing from discovery: %v", path, paths)
		}
	}
	for _, lang := range []string{"python", "env", "certificate", "dockerfile", "shell", "markdown"} {
		if got.Languages[lang] == 0 {
			t.Fatalf("language %s missing from counts: %v", lang, got.Languages)
		}
	}
	if got.Languages["python"] != 1 || got.Languages["env"] != 2 {
		t.Fatalf("language counts wrong: %v", got.Languages)
	}
}

// TestDiscoverExcludesStillApplyToNewFileTypes pins that the exclusion
// machinery (DB user excludes and the committed .bluntcodeignore) filters
// the newly classified file types exactly like it always filtered source
// files.
func TestDiscoverExcludesStillApplyToNewFileTypes(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"main.go":           "x",
		"cert.pem":          "x",
		"keys/local.key":    "x",
		".env":              "x",
		".env.local":        "x",
		"deploy/Dockerfile": "x",
		"notes.md":          "x",
	}
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, IgnoreFileName), []byte("notes.md\nkeys/**\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Discover(context.Background(), root, []string{"*.pem", ".env"})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, file := range got.Files {
		paths[file.RelativePath] = true
	}
	for _, excluded := range []string{"cert.pem", ".env", "keys/local.key", "notes.md"} {
		if paths[excluded] {
			t.Fatalf("%s should be excluded (user pattern or ignore file), got %v", excluded, paths)
		}
	}
	// The user's ".env" pattern is a basename match: it hides the dotfile
	// itself but not the .env.local variant or the config.env extension form.
	for _, included := range []string{"main.go", ".env.local", "deploy/Dockerfile"} {
		if !paths[included] {
			t.Fatalf("%s missing from discovery: %v", included, paths)
		}
	}
	if len(paths) != 3 {
		t.Fatalf("discovered %v, want exactly the three surviving files", paths)
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

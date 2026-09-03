package discovery

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestArtifactFileClassification pins the smart-skip file table: generated
// and minified names must be excluded wherever they sit, while names a human
// plausibly gave a hand-written file stay candidates.
func TestArtifactFileClassification(t *testing.T) {
	excluded := []string{
		// Hash-suffixed bundler output, including this repository's own
		// compiled web bundle names (Vite, esbuild, webpack shapes — even
		// digit-less and hyphen-bearing base64url hashes).
		"index-CKc0XBc7.js", "index-CxIJRw4I.js", "main.6f8f4f2a.js", "styles-Dw2bJdXo.css",
		"chunk-charts-BdSwwqVo.js", "chunk-editor-C-LEM0mN.js",
		// Minified, chunked, and bundled output without a hash.
		"app.min.js", "app.min.css", "app.min.mjs", "app.chunk.js", "app.bundle.css",
		"vendors~main.js", "vendor.js", "vendors.js", "0.js", "2.chunk.js",
		// Lockfiles and single-line build metadata.
		"package-lock.json", "pnpm-lock.yaml", "yarn.lock", "data.tsbuildinfo",
		// Generated code.
		"api.pb.go", "svc_pb2.py", "ui.generated.ts", "Form.designer.cs",
		// The legacy suffix set must keep working through the new table.
		"a.map", "x.pyc",
	}
	for _, name := range excluded {
		if !DefaultExcluded(filepath.FromSlash(name), false) {
			t.Errorf("%q should be excluded as generated output", name)
		}
	}
	kept := []string{
		"main.ts", "service2.js", "user-service2.js", "index.test.js", "package.json",
		"use-auth-hook.js", "create-store.js", "vite.config.js", "moment.js",
		"locales.js", "2024-report.js", "App.cs", "chart-123456.js",
		// Whole-stem PascalCase/camelCase names are hand-written files; the
		// digit-less hash shape must only fire after a separator.
		"DarkMode.js", "MyComponent.tsx", "useAuthHook.ts",
	}
	for _, name := range kept {
		if DefaultExcluded(filepath.FromSlash(name), false) {
			t.Errorf("%q is a plausible hand-written file and must stay a candidate", name)
		}
	}
}

// TestArtifactDirectoryClassification pins the smart-skip directory table:
// every ecosystem's build, dependency, cache, and log directories are
// pruned, while first-party source directory names are never matched.
func TestArtifactDirectoryClassification(t *testing.T) {
	excluded := []string{
		"target", "vendor", "obj", "Pods", "DerivedData", ".svelte-kit", ".output",
		".astro", ".vite", ".turbo", ".pnpm-store", ".yarn", ".tox", "site-packages",
		".terraform", "playwright-report", "test-results", "logs", "log", "_build",
		"foo-1.2.3.egg-info",
		// Case-insensitive, like the historical set.
		"NODE_MODULES", "Dist",
	}
	for _, name := range excluded {
		if !DefaultExcluded(filepath.FromSlash(name), true) {
			t.Errorf("directory %q should be excluded as generated", name)
		}
	}
	kept := []string{"src", "cmd", "internal", "bin", "packages", "web", "docs", "deps"}
	for _, name := range kept {
		if DefaultExcluded(filepath.FromSlash(name), true) {
			t.Errorf("directory %q is a plausible source directory and must stay visible", name)
		}
	}
}

// TestIsArtifactPathMatrix pins the finding-filter flavor: artifact
// directories and generated file names anywhere are artifacts, but lockfile
// paths are not — dependency analyzers (osv, trivy) report them and those
// findings must survive.
func TestIsArtifactPathMatrix(t *testing.T) {
	artifacts := []string{
		"node_modules/left-pad/index.js",
		"dist/assets/app.js",
		"static/assets/index-CKc0XBc7.js",
		"target/debug/main.rs",
		"vendor/autoload.php",
		"logs/deploy.txt",
	}
	for _, path := range artifacts {
		if !IsArtifactPath(path) {
			t.Errorf("%q should be classified as an artifact path", path)
		}
	}
	live := []string{
		"src/main.py", "web/src/App.tsx", "package.json", "cmd/tool/main.go",
		"package-lock.json", "pnpm-lock.yaml", "go.sum", "",
	}
	for _, path := range live {
		if IsArtifactPath(path) {
			t.Errorf("%q must not be classified as an artifact path", path)
		}
	}
}

func TestHashLikeSegment(t *testing.T) {
	hashes := []string{"CKc0XBc7", "6f8f4f2a", "Dw2bJdXo", "BdSwwqVo", "C-LEM0mN", "abc123def", "1234567", "deadBEEF9"}
	for _, segment := range hashes {
		if !hashLikeSegment(segment, true) {
			t.Errorf("%q looks like a bundler hash and should match", segment)
		}
	}
	names := []string{"service2", "component", "locales", "v2", "polyfill", "123456", "auth"}
	for _, segment := range names {
		if hashLikeSegment(segment, true) {
			t.Errorf("%q is a plausible name fragment and must not match", segment)
		}
	}
	// A digit-less mixed-case shape only counts after a separator; as a
	// whole stem it is a PascalCase source file (DarkMode.js). A digit-
	// bearing shape matches even bare (Dw2bJdXo.js is a hash, not a name).
	if hashLikeSegment("BdSwwqVo", false) {
		t.Error("digit-less hash shape must not match a whole stem")
	}
	if !hashLikeSegment("Dw2bJdXo", false) {
		t.Error("digit-bearing mixed-case shape should match a whole stem")
	}
	if hashLikeSegment("DarkMode", false) || hashLikeSegment("service2", false) {
		t.Error("whole-stem hand-written names must never match")
	}
}

func TestLooksMinified(t *testing.T) {
	if !LooksMinified([]byte(strings.Repeat("a", minifiedLineLength) + "\nshort")) {
		t.Fatal("a minified-length line must be detected")
	}
	if LooksMinified([]byte(strings.Repeat("a", minifiedLineLength-1) + "\nshort")) {
		t.Fatal("just under the threshold must pass")
	}
	if !LooksMinified([]byte(strings.Repeat("a", minifiedLineLength+200))) {
		t.Fatal("a long line without a trailing newline is still minified")
	}
	if LooksMinified(nil) {
		t.Fatal("empty head is not minified")
	}
}

// TestDiscoverSkipsGeneratedContent covers the content and size heuristics:
// a minified bundle under a source-looking name is skipped once it is large
// enough to sniff, ordinary large source stays, and oversized single files
// are refused outright.
func TestDiscoverSkipsGeneratedContent(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"src/app.js":         "const value = 1;\n",
		"src/handwritten.js": strings.Repeat("const line = 2;\n", 8000), // ~136 KiB, short lines
		// One minified-length first line, padded past the sniff threshold.
		"public/app.js":  strings.Repeat("x", 2000) + "\n" + strings.Repeat("const a = 2;\n", 11000),
		"styles/all.css": "body{" + strings.Repeat("margin:0;", 16000) + "}\n",
	}
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	oversize := filepath.Join(root, "data", "dump.json")
	if err := os.MkdirAll(filepath.Dir(oversize), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oversize, bytes.Repeat([]byte("x"), maxCandidateBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Discover(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, file := range got.Files {
		found[file.RelativePath] = true
	}
	if !found["src/app.js"] || !found["src/handwritten.js"] {
		t.Fatalf("hand-written candidates were dropped: %+v", got.Files)
	}
	if found["public/app.js"] || found["styles/all.css"] || found["data/dump.json"] {
		t.Fatalf("generated content was scanned anyway: %+v", got.Files)
	}
	if got.Skipped != 3 {
		t.Fatalf("expected exactly 3 content-skip events, got %d", got.Skipped)
	}
}

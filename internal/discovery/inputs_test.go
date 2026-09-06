package discovery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLanguageTerraformAndLicenseBasenames pins the IMP-02 classifier
// additions: terraform files classify (so checkov/trivy can route on them),
// license artifacts classify as text when no known extension applies, and
// extension matching keeps precedence in both directions.
func TestLanguageTerraformAndLicenseBasenames(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"main.tf", "terraform"},
		{"infra/vars.tfvars", "terraform"},
		{"POLICY.HCL", "terraform"},
		{"LICENSE", "text"},
		{"LICENCE", "text"},
		{"COPYING", "text"},
		{"NOTICE", "text"},
		{"docs/LICENSE.md", "markdown"}, // known extension wins
		{"COPYING.LESSER", "text"},      // unknown extension falls to basename
		{"third_party/LICENSE-MIT", "text"},
		{"LICENSE_APACHE", "text"},
		{"licensee.ts", "typescript"}, // prefix must not swallow longer words
		{"licenses/report.docx", ""},  // unknown extension, not a license basename
		{"notices.txt", "text"},       // extension classifies before basename
		{"copying.go", "go"},          // source file named copying.go stays Go
	}
	for _, tc := range cases {
		if got := Language(tc.path); got != tc.want {
			t.Errorf("Language(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestDiscoverSkipCountsByReason proves the walk explains its coverage: each
// skip path lands in its own bucket instead of one opaque number, and Skipped
// stays the sum of the buckets.
func TestDiscoverSkipCountsByReason(t *testing.T) {
	root := t.TempDir()
	// A source-named file whose head is one enormous line trips the content
	// heuristic (generated_content); .min.js the NAME table catches counts as
	// a default exclusion instead.
	minified := "var x=" + strings.Repeat("1", 200<<10)
	writeTree(t, root, map[string]string{
		"main.py":               "x",
		"node_modules/pkg/a.js": "x",
		"vendor/lib.ts":         "x",
		"app.min.js":            "x",
		"bundle.js":             minified,
		"docs/guide.md":         "x",
		"keepme.txt":            "x",
	})
	got, err := Discover(context.Background(), root, []string{"docs/**"})
	if err != nil {
		t.Fatal(err)
	}
	if got.SkipCounts[SkipDefaultExcluded] == 0 {
		t.Fatalf("expected excluded_default skips, got %+v", got.SkipCounts)
	}
	if got.SkipCounts[SkipUserExcluded] != 1 {
		t.Fatalf("excluded_user = %d, want 1 (docs/guide.md): %+v", got.SkipCounts[SkipUserExcluded], got.SkipCounts)
	}
	if got.SkipCounts[SkipGenerated] != 1 {
		t.Fatalf("generated_content = %d, want 1 (bundle.js): %+v", got.SkipCounts[SkipGenerated], got.SkipCounts)
	}
	sum := 0
	for _, count := range got.SkipCounts {
		sum += count
	}
	if got.Skipped != sum {
		t.Fatalf("Skipped (%d) must equal the sum of SkipCounts (%d)", got.Skipped, sum)
	}
}

// TestDiscoverTracksDependencyInputs pins the routing fix for dependency
// analyzers: manifests and lockfiles are tracked as dependency inputs even
// when the lockfile is smart-skipped out of Files, so osv/trivy eligibility
// survives the artifact exclusion. package.json is both a selected file (it
// classifies as json) and a dependency input.
func TestDiscoverTracksDependencyInputs(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"main.py":           "x",
		"package.json":      `{"name":"x"}`,
		"package-lock.json": `{"lockfileVersion":3}`,
		"go.mod":            "module x\n",
		"requirements.txt":  "flask==1.0\n",
	})
	got, err := Discover(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"package.json": false, "package-lock.json": false, "go.mod": false, "requirements.txt": false}
	for _, input := range got.DependencyInputs {
		if _, ok := want[input]; ok {
			want[input] = true
		}
	}
	for input, seen := range want {
		if !seen {
			t.Errorf("dependency input %s missing from %+v", input, got.DependencyInputs)
		}
	}
	selected := map[string]bool{}
	for _, file := range got.Files {
		selected[file.RelativePath] = true
	}
	if !selected["package.json"] {
		t.Errorf("package.json should still be a selected file (json classifies)")
	}
	if selected["package-lock.json"] {
		t.Errorf("package-lock.json must stay excluded from Files (artifact) while tracked as a dependency input")
	}
	if got.SkipCounts[SkipDefaultExcluded] < 1 {
		t.Errorf("the excluded lockfile should count as a default exclusion: %+v", got.SkipCounts)
	}
}

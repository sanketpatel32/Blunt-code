package discovery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseIgnoreFile(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		wantPatterns []string
		wantNegated  int
	}{{
		name:         "empty file",
		input:        "",
		wantPatterns: nil,
	}, {
		name:         "comments and blank lines only",
		input:        "# comment\n\n   # indented comment\n\t\n",
		wantPatterns: nil,
	}, {
		name:         "plain patterns with LF endings",
		input:        "legacy.py\nvendor/**\n**/scratch.py\n",
		wantPatterns: []string{"legacy.py", "vendor/**", "**/scratch.py"},
	}, {
		name:         "CRLF endings and stray whitespace are trimmed",
		input:        "  legacy.py \r\n\tvendor/**\t\r\n**/scratch.py\r\n",
		wantPatterns: []string{"legacy.py", "vendor/**", "**/scratch.py"},
	}, {
		name:         "comment between patterns",
		input:        "legacy.py\n# why this is ignored\nvendor/**",
		wantPatterns: []string{"legacy.py", "vendor/**"},
	}, {
		name:         "negated lines are counted and skipped",
		input:        "legacy.py\n!keep.py\n!**/keep.py\nvendor/**",
		wantPatterns: []string{"legacy.py", "vendor/**"},
		wantNegated:  2,
	}, {
		name:         "windows-style backslashes normalize to slashes",
		input:        "Build\\Temp\r\nvendor\\**",
		wantPatterns: []string{"Build/Temp", "vendor/**"},
	}, {
		name:         "pattern count is capped",
		input:        strings.Repeat("p.py\n", ignoreFileMaxPatterns+500),
		wantPatterns: strings.Split(strings.TrimSuffix(strings.Repeat("p.py,", ignoreFileMaxPatterns), ","), ","),
	}, {
		name:         "invalid UTF-8 yields no patterns",
		input:        "legacy.py\n\xff\xfe/broken\nvendor/**",
		wantPatterns: nil,
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			patterns, negated := parseIgnoreFile([]byte(tc.input))
			if negated != tc.wantNegated {
				t.Fatalf("negated = %d, want %d", negated, tc.wantNegated)
			}
			if len(patterns) != len(tc.wantPatterns) {
				t.Fatalf("patterns = %#v, want %#v", patterns, tc.wantPatterns)
			}
			for i := range patterns {
				if patterns[i] != tc.wantPatterns[i] {
					t.Fatalf("patterns = %#v, want %#v", patterns, tc.wantPatterns)
				}
			}
		})
	}
}

func TestWorkspaceExcludesMerge(t *testing.T) {
	root := t.TempDir()
	writeIgnoreFile := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, IgnoreFileName), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	user := []string{"dbonly/**", "SHARED.py"}

	t.Run("missing file returns user excludes unchanged", func(t *testing.T) {
		got := WorkspaceExcludes(root, user)
		if len(got) != 2 || got[0] != "dbonly/**" || got[1] != "SHARED.py" {
			t.Fatalf("merged = %#v, want user excludes unchanged", got)
		}
	})

	t.Run("file patterns are appended and deduplicated", func(t *testing.T) {
		writeIgnoreFile("# ignores\nlegacy.py\nshared.py\n")
		got := WorkspaceExcludes(root, user)
		// "shared.py" is a case-insensitive duplicate of the user's
		// "SHARED.py", so only one of the two survives.
		want := []string{"dbonly/**", "SHARED.py", "legacy.py"}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("merged = %#v, want %#v", got, want)
		}
		// Case-insensitive dedupe makes re-merging an already-merged list a no-op.
		if again := WorkspaceExcludes(root, got); fmt.Sprint(again) != fmt.Sprint(want) {
			t.Fatalf("re-merge = %#v, want %#v (must be idempotent)", again, want)
		}
	})

	t.Run("broken file is tolerated", func(t *testing.T) {
		writeIgnoreFile("legacy.py\n\xff\xfe\n")
		got := WorkspaceExcludes(root, user)
		if len(got) != 2 || got[0] != "dbonly/**" || got[1] != "SHARED.py" {
			t.Fatalf("merged = %#v, want user excludes only (invalid UTF-8 ignored)", got)
		}
	})
}

// TestDiscoverHonorsIgnoreFile drives the full walk with a workspace that
// commits a .bluntcodeignore next to DB-provided user excludes: both sources
// must apply, using the same pattern forms and case-insensitive matching the
// user exclude rules already use.
func TestDiscoverHonorsIgnoreFile(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"main.py":                 "x=1",
		"legacy.py":               "x=1",
		"generated/deep/mod.py":   "x=1",
		"vendor/pkg/bundled.py":   "x=1",
		"src/nested/scratch.py":   "x=1",
		"dbonly/internal/tool.py": "x=1",
		"GENUP/stream.py":         "x=1",
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
	// Case-mixed on purpose: matching is case-insensitive for both sources.
	if err := os.WriteFile(filepath.Join(root, IgnoreFileName), []byte("# committed ignores\r\n  LEGACY.py \r\nGENERATED/**\r\nvendor/**\r\n**/scratch.py\r\n!keep.py\r\ngenup/**\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Discover(context.Background(), root, []string{"dbonly/**"})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, file := range result.Files {
		paths[file.RelativePath] = true
	}
	if !paths["main.py"] {
		t.Fatalf("main.py missing from discovery: %#v", result.Files)
	}
	for _, excluded := range []string{"legacy.py", "generated/deep/mod.py", "vendor/pkg/bundled.py", "src/nested/scratch.py", "dbonly/internal/tool.py", "GENUP/stream.py"} {
		if paths[excluded] {
			t.Fatalf("%s should be excluded (file or user pattern), got %#v", excluded, result.Files)
		}
	}
}

// TestTreeHonorsIgnoreFile pins the file-tree endpoint's behavior: the ignore
// file applies with the matcher's existing forms. A basename pattern hides a
// directory entry itself; a "**/name" pattern hides matching files inside an
// expanded folder. ("dir/**" hides only directory contents, matching how DB
// user excludes behave in the tree today.)
func TestTreeHonorsIgnoreFile(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"main.py", "generated/mod.py", "src/scratch.py", "src/keep.py"} {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x=1"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, IgnoreFileName), []byte("generated\n**/scratch.py\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := Tree(context.Background(), root, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.RelativePath == "generated" || item.RelativePath == IgnoreFileName {
			t.Fatalf("unexpected tree entry %q: %#v", item.RelativePath, items)
		}
	}
	nested, err := Tree(context.Background(), root, "src", nil)
	if err != nil || len(nested) != 1 || nested[0].RelativePath != "src/keep.py" {
		t.Fatalf("unexpected src tree: %#v (%v), want only src/keep.py", nested, err)
	}
}

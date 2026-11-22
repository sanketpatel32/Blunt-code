package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"bluntcode/internal/core"
)

// TestWorkspaceListSearchMatchesDiscoveredLanguages pins that the paged list's
// search covers never-scanned workspaces through their discovered languages,
// warming the cache itself, so the Overview needs no client-side language filter.
func TestWorkspaceListSearchMatchesDiscoveredLanguages(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.rs"), []byte("fn main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: root, Name: "Fresh"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	names := func(target string) []string {
		t.Helper()
		response := get(t, s, http.MethodGet, target)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", target, response.Code, response.Body.String())
		}
		var body struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, item := range body.Items {
			out = append(out, item.Name)
		}
		if body.Total != len(out) {
			t.Fatalf("total %d vs %v", body.Total, out)
		}
		return out
	}
	// The language cache starts cold; the search itself warms it.
	var got []string
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if got = names("/api/v1/workspaces?page=1&search=RUST"); len(got) > 0 {
			break
		}
	}
	if !reflect.DeepEqual(got, []string{"Fresh"}) {
		t.Fatalf("language search = %v, want [Fresh]", got)
	}
	if got := names("/api/v1/workspaces?page=1&search=other"); !reflect.DeepEqual(got, []string{"Other"}) {
		t.Fatalf("name search = %v, want [Other]", got)
	}
	if got := names("/api/v1/workspaces?page=1&search=python"); len(got) != 0 {
		t.Fatalf("no-match search = %v, want []", got)
	}
}

func TestLanguageMatchesKeepsStaleEntries(t *testing.T) {
	c := newWorkspaceCaches()
	c.storeLanguages("ws-a", "k", []string{"go"})
	c.storeLanguages("ws-b", "k", []string{"typescript"})
	c.languages["ws-a"] = languageEntry{key: "k", names: []string{"go"}, at: time.Now().Add(-time.Hour)}
	if got := c.languageMatches(" GO "); !reflect.DeepEqual(got, []string{"ws-a"}) {
		t.Fatalf("languageMatches = %v, want [ws-a]", got)
	}
	if got := c.languageMatches(""); got != nil {
		t.Fatalf("empty needle = %v, want nil", got)
	}
}

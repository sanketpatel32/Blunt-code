package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"bluntcode/internal/core"
)

// TestWorkspaceListSearchMatchesDiscoveredLanguages pins that the paged list's
// search covers never-scanned workspaces through their discovered languages,
// so the Overview needs no second client-side language filter.
func TestWorkspaceListSearchMatchesDiscoveredLanguages(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	fresh, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Fresh"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	s.cache.storeLanguages(fresh.ID, "k", []string{"rust", "toml"})
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
	if got := names("/api/v1/workspaces?page=1&search=RUST"); len(got) != 1 || got[0] != "Fresh" {
		t.Fatalf("language search = %v, want [Fresh]", got)
	}
	if got := names("/api/v1/workspaces?page=1&search=other"); len(got) != 1 || got[0] != "Other" {
		t.Fatalf("name search = %v, want [Other]", got)
	}
	if got := names("/api/v1/workspaces?page=1&search=python"); len(got) != 0 {
		t.Fatalf("no-match search = %v, want []", got)
	}
}

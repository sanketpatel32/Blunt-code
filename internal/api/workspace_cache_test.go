package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bluntcode/internal/core"
)

func listLanguages(t *testing.T, s *Server, id string) []string {
	t.Helper()
	response := get(t, s, http.MethodGet, "/api/v1/workspaces")
	var body struct {
		Items []struct {
			ID        string   `json:"id"`
			Languages []string `json:"languages"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, item := range body.Items {
		if item.ID == id {
			return item.Languages
		}
	}
	t.Fatalf("workspace %s missing from list", id)
	return nil
}

// TestWorkspaceListNeverWalksTheFolder: an unscanned workspace's languages
// come from a cache the list warms in the background, never a walk inside
// the request; the single-workspace GET still answers synchronously.
func TestWorkspaceListNeverWalksTheFolder(t *testing.T) {
	s := testServer(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.py"), []byte("print(1)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: root, Name: "Langs"})
	if err != nil {
		t.Fatal(err)
	}
	// Cold cache: the list answers without languages instead of walking.
	if langs := listLanguages(t, s, work.ID); len(langs) != 0 {
		t.Fatalf("cold list walked the folder: %v", langs)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if langs := listLanguages(t, s, work.ID); len(langs) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background warm never filled the language cache")
		}
		time.Sleep(20 * time.Millisecond)
	}
	response := get(t, s, http.MethodGet, "/api/v1/workspaces/"+work.ID)
	var single struct {
		Languages []string `json:"languages"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &single); err != nil || len(single.Languages) == 0 {
		t.Fatalf("single GET languages: %s", response.Body.String())
	}
}

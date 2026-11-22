package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"bluntcode/internal/core"
)

type rootView struct {
	ID          string `json:"id"`
	RootMissing bool   `json:"root_missing"`
	RootProblem string `json:"root_problem"`
}

func listRootViews(t *testing.T, s *Server, target string) map[string]rootView {
	t.Helper()
	response := get(t, s, http.MethodGet, target)
	if response.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", target, response.Code, response.Body.String())
	}
	var body struct {
		Items []rootView `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	views := map[string]rootView{}
	for _, item := range body.Items {
		views[item.ID] = item
	}
	return views
}

// TestWorkspaceListFlagsMissingRoots pins issue #62: a workspace whose folder
// was deleted is marked root_missing on both list shapes, while a healthy one
// is explicitly false.
func TestWorkspaceListFlagsMissingRoots(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	healthy, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Healthy"})
	if err != nil {
		t.Fatal(err)
	}
	goneRoot := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(goneRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	gone, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: goneRoot, Name: "Gone"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(goneRoot); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/api/v1/workspaces", "/api/v1/workspaces?page=1&page_size=25"} {
		views := listRootViews(t, s, target)
		if v := views[healthy.ID]; v.RootMissing || v.RootProblem != "" {
			t.Fatalf("%s healthy = %+v, want not missing", target, v)
		}
		if v := views[gone.ID]; !v.RootMissing || v.RootProblem != rootProblemMissing {
			t.Fatalf("%s gone = %+v, want root_missing/missing", target, v)
		}
	}
}

// TestWorkspaceGetFlagsFileRoot: a root replaced by a file is not a usable
// folder either, and the single-workspace GET says so.
func TestWorkspaceGetFlagsFileRoot(t *testing.T) {
	s := testServer(t)
	root := filepath.Join(t.TempDir(), "proj")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: root, Name: "Swapped"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	response := get(t, s, http.MethodGet, "/api/v1/workspaces/"+work.ID)
	var view rootView
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil || response.Code != http.StatusOK {
		t.Fatalf("get: %d %s", response.Code, response.Body.String())
	}
	if !view.RootMissing || view.RootProblem != rootProblemNotDirectory {
		t.Fatalf("view = %+v, want root_missing/not_directory", view)
	}
}

// TestMissingRootFileEndpointsFailHonestly: endpoints that read the folder
// answer 422 WORKSPACE_ROOT_MISSING naming the path, instead of a 500
// discovery failure; removing the stale workspace still works.
func TestMissingRootFileEndpointsFailHonestly(t *testing.T) {
	s := testServer(t)
	root := filepath.Join(t.TempDir(), "deleted")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: root, Name: "Ghost"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/workspaces/" + work.ID
	for _, call := range []struct{ method, target string }{
		{http.MethodPost, base + "/discover"},
		{http.MethodGet, base + "/tree"},
		{http.MethodGet, base + "/scan-plan"},
		{http.MethodPost, base + "/scans"},
	} {
		method, target := call.method, call.target
		response := get(t, s, method, target)
		body := response.Body.String()
		if response.Code != http.StatusUnprocessableEntity || !strings.Contains(body, `"code":"WORKSPACE_ROOT_MISSING"`) || !strings.Contains(body, `"root_problem":"missing"`) {
			t.Fatalf("%s %s: %d %s", method, target, response.Code, body)
		}
		var envelope struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || !strings.Contains(envelope.Error.Message, root) {
			t.Fatalf("%s message should name the path: %s", target, body)
		}
	}
	if response := get(t, s, http.MethodDelete, "/api/v1/workspaces/"+work.ID); response.Code >= 300 {
		t.Fatalf("delete missing workspace: %d %s", response.Code, response.Body.String())
	}
}

// TestWorkspaceListDoesNotWaitOnHangingRoot: a probe stuck on an unreachable
// share must not stall the list; the unresolved root reads as healthy.
func TestWorkspaceListDoesNotWaitOnHangingRoot(t *testing.T) {
	s := testServer(t)
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: t.TempDir(), Name: "Share"})
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	s.checkRoot = func(string) string { <-release; return rootProblemMissing }
	s.rootTimeout = 50 * time.Millisecond
	started := time.Now()
	views := listRootViews(t, s, "/api/v1/workspaces")
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("list waited %s on a hanging root probe", elapsed)
	}
	if v := views[work.ID]; v.RootMissing {
		t.Fatalf("unresolved probe reported missing: %+v", v)
	}
}

func TestCheckWorkspaceRootUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := checkWorkspaceRoot(root); got != "" {
		t.Fatalf("healthy root = %q", got)
	}
	if err := os.Chmod(root, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	if got := checkWorkspaceRoot(root); got != rootProblemUnreadable {
		t.Fatalf("locked root = %q, want unreadable", got)
	}
}

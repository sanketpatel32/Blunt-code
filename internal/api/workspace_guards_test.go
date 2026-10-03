package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bluntcode/internal/core"
)

// patchWorkspace issues a PATCH through the full handler chain with a JSON body.
func patchWorkspace(t *testing.T, s *Server, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPatch, "http://127.0.0.1/api/v1/workspaces/"+id, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	return response
}

// TestDeleteWorkspaceRejectsActiveScan pins the deletion guard: removing a
// workspace while one of its scans is mid-flight would cascade the scan row
// out from under the running engine (its writes then land against deleted
// rows and the scan page polls a vanished scan), so the delete is a 409
// until the scan is terminal — the same contract DELETE /scans/{id} has.
func TestDeleteWorkspaceRejectsActiveScan(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Busy"})
	if err != nil {
		t.Fatal(err)
	}
	// Every mid-lifecycle state blocks the delete, not just "running" — a
	// deep scan spends minutes in discovery before the first analyzer starts.
	scan, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "discovering"})
	if err != nil {
		t.Fatal(err)
	}
	response := get(t, s, http.MethodDelete, "/api/v1/workspaces/"+work.ID)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "SCAN_IN_PROGRESS") {
		t.Fatalf("active scan must block delete: %d %s", response.Code, response.Body.String())
	}
	if _, err := s.db.Workspace(ctx, work.ID); err != nil {
		t.Fatalf("workspace must survive a blocked delete: %v", err)
	}
	// Once the scan is terminal the same delete succeeds.
	if err := s.db.CompleteScan(ctx, scan.ID, "cancelled", ""); err != nil {
		t.Fatal(err)
	}
	response = get(t, s, http.MethodDelete, "/api/v1/workspaces/"+work.ID)
	if response.Code != http.StatusNoContent {
		t.Fatalf("terminal scan must allow delete: %d %s", response.Code, response.Body.String())
	}
}

// TestUpdateWorkspaceValidatesDefaultProfile pins the PATCH boundary: the
// stored default feeds the workspaces list's one-click Scan button and
// POST /workspaces/{id}/scans already rejects unknown profiles, so a bogus
// value must die here instead of persisting and failing every later default
// scan. An empty value resets to the same default CreateWorkspace applies.
func TestUpdateWorkspaceValidatesDefaultProfile(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Picky"})
	if err != nil {
		t.Fatal(err)
	}
	if response := patchWorkspace(t, s, work.ID, `{"default_profile":"thorough"}`); response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_PROFILE") {
		t.Fatalf("bogus profile must 400: %d %s", response.Code, response.Body.String())
	}
	if response := patchWorkspace(t, s, work.ID, `{"default_profile":"deep"}`); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"default_profile":"deep"`) {
		t.Fatalf("valid profile must persist: %d %s", response.Code, response.Body.String())
	}
	if response := patchWorkspace(t, s, work.ID, `{"name":"Renamed"}`); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"default_profile":"deep"`) {
		t.Fatalf("name-only patch must keep the stored profile: %d %s", response.Code, response.Body.String())
	}
	if response := patchWorkspace(t, s, work.ID, `{"default_profile":""}`); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"default_profile":"standard"`) {
		t.Fatalf("empty profile must reset to standard: %d %s", response.Code, response.Body.String())
	}
}

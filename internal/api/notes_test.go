package api

import (
	"bluntcode/internal/core"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFindingNotesAPIValidationAndPersistence(t *testing.T) {
	s := testServer(t)
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{Name: "Notes", RootPath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/workspaces/" + work.ID + "/notes/fingerprint"
	request := func(method, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.mux.ServeHTTP(w, r)
		return w
	}
	if w := request(http.MethodPost, `{"text":" "}`); w.Code != 400 {
		t.Fatalf("empty note = %d", w.Code)
	}
	if w := request(http.MethodPost, `{"text":"Review this"}`); w.Code != 201 {
		t.Fatalf("save = %d %s", w.Code, w.Body.String())
	}
	w := request(http.MethodGet, "")
	var body struct{ Items []struct{ Text string } }
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(body.Items) != 1 || body.Items[0].Text != "Review this" {
		t.Fatalf("get = %d %s", w.Code, w.Body.String())
	}
}
func TestScanPlanRejectsInvalidProfilesAndMissingService(t *testing.T) {
	s := testServer(t)
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{Name: "Plan", RootPath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		profile string
		status  int
	}{{"unknown", 400}, {"quick", 503}} {
		w := httptest.NewRecorder()
		s.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+work.ID+"/scan-plan?profile="+test.profile, nil))
		if w.Code != test.status {
			t.Fatalf("%s: %d %s", test.profile, w.Code, w.Body.String())
		}
	}
}

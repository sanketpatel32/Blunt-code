package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The embedded web bundle ships a real index.html; every assertion below
// depends on it, so fail loudly if the build step did not run.
func requireIndexAsset(t *testing.T) {
	t.Helper()
	content, err := fs.Sub(staticFiles, "static")
	if err != nil {
		t.Fatalf("static subtree: %v", err)
	}
	data, err := fs.ReadFile(content, "index.html")
	if err != nil || len(data) == 0 {
		t.Fatalf("embedded index.html missing or empty; run scripts/build.ps1 before testing")
	}
}

func TestStaticHandlerServesAppShellAndFallsBackForClientRoutes(t *testing.T) {
	requireIndexAsset(t)
	handler := staticHandler()

	// The root serves the app shell.
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `<div id="root">`) {
		t.Fatalf("root must serve the app shell: %d", response.Code)
	}

	// Client-side routes fall back to the same shell instead of a 404.
	for _, route := range []string{"/workspaces", "/findings", "/tools", "/settings"} {
		request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+route, nil)
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `<div id="root">`) {
			t.Fatalf("client route %s must fall back to the app shell: %d", route, response.Code)
		}
	}
}

func TestStaticHandlerServesRealAssetsDirectly(t *testing.T) {
	requireIndexAsset(t)
	content, err := fs.Sub(staticFiles, "static")
	if err != nil {
		t.Fatalf("static subtree: %v", err)
	}
	entries, err := fs.ReadDir(content, "assets")
	if err != nil || len(entries) == 0 {
		t.Skip("no hashed assets present; skipping direct asset probe")
	}
	name := entries[0].Name()
	handler := staticHandler()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/assets/"+name, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.Len() == 0 {
		t.Fatalf("hashed asset %s must serve directly: %d (%d bytes)", name, response.Code, response.Body.Len())
	}
}

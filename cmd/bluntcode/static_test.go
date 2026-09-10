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
	for _, route := range []string{"/workspaces", "/findings", "/tools", "/settings", "/scans/foo"} {
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

func TestStaticHandlerMissingAssetsReturn404(t *testing.T) {
	requireIndexAsset(t)
	handler := staticHandler()
	// Missing file-like paths must 404 instead of falling back to the shell:
	// index.html under a .js URL fails the module MIME check and gets cached.
	for _, path := range []string{"/assets/missing-bundle.js", "/assets/stale-abc123.css", "/missing-icon.svg", "/favicon.ico", "/robots.txt"} {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("missing asset %s must 404, got %d", path, response.Code)
		}
		if strings.Contains(response.Body.String(), `<div id="root">`) {
			t.Fatalf("missing asset %s must not serve the app shell", path)
		}
	}
}

func TestStaticHandlerCacheHeaders(t *testing.T) {
	requireIndexAsset(t)
	handler := staticHandler()
	content, err := fs.Sub(staticFiles, "static")
	if err != nil {
		t.Fatalf("static subtree: %v", err)
	}
	entries, err := fs.ReadDir(content, "assets")
	if err != nil || len(entries) == 0 {
		t.Skip("no hashed assets present; skipping cache header probe")
	}
	// Content-hashed bundles are immutable.
	name := entries[0].Name()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/assets/"+name, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if got := response.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("hashed asset %s must be immutable, got Cache-Control %q", name, got)
	}

	// The shell and everything else must always be revalidated.
	for _, path := range []string{"/", "/workspaces", "/scans/foo", "/llms.txt"} {
		request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+path, nil)
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s must serve: %d", path, response.Code)
		}
		if got := response.Header().Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("%s must be no-cache, got Cache-Control %q", path, got)
		}
	}

	// /index.html canonicalizes to / with a redirect; that redirect must not
	// be cached either.
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/index.html", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMovedPermanently {
		t.Fatalf("/index.html must redirect to /: %d", response.Code)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("/index.html redirect must be no-cache, got Cache-Control %q", got)
	}
}

func TestStaticHandlerSetsUICSP(t *testing.T) {
	requireIndexAsset(t)
	handler := staticHandler()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	csp := response.Header().Get("Content-Security-Policy")
	// The hash must pin the inline theme-bootstrap script byte-for-byte, and
	// the baseline guarantees (default-src, frame-ancestors) must survive.
	for _, want := range []string{
		"default-src 'self'",
		"script-src 'self' 'sha256-mOPhT864bFfna/DIOUP/k3m5V3MsQvJPSfsIB3Tt+UQ='",
		"style-src 'self' 'unsafe-inline'",
		"frame-ancestors 'none'",
	} {
		if !strings.Contains(csp, want) {
			t.Fatalf("UI Content-Security-Policy must contain %q, got %q", want, csp)
		}
	}
}

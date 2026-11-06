package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSeedDemoDisabledByDefault(t *testing.T) {
	// Isolate from a parent process that exported BLUNTCODE_DEMO_SEED / TOKEN.
	t.Setenv("BLUNTCODE_DEMO_SEED", "")
	t.Setenv("BLUNTCODE_DEMO_TOKEN", "")
	ResetRateLimiter()
	s := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/system/seed-demo", bytes.NewReader([]byte(`{"preset":"small"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] == nil {
		t.Fatalf("expected error envelope: %s", rec.Body.String())
	}
}

func TestSeedDemoInvalidJSON(t *testing.T) {
	t.Setenv("BLUNTCODE_DEMO_SEED", "1")
	ResetRateLimiter()
	s := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/system/seed-demo", bytes.NewReader([]byte(`not json`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestSeedDemoInvalidPreset(t *testing.T) {
	t.Setenv("BLUNTCODE_DEMO_SEED", "1")
	ResetRateLimiter()
	s := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/system/seed-demo", bytes.NewReader([]byte(`{"preset":"nope"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSeedDemoRequiresTokenWhenConfigured(t *testing.T) {
	t.Setenv("BLUNTCODE_DEMO_SEED", "1")
	t.Setenv("BLUNTCODE_DEMO_TOKEN", "secret-test")
	ResetRateLimiter()
	s := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/system/seed-demo", bytes.NewReader([]byte(`{"preset":"small"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 without token, got %d", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/system/seed-demo", bytes.NewReader([]byte(`{"preset":"small"}`)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Bluntcode-Demo-Token", "secret-test")
	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("want 200 with token, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestSeedDemoSmallSuccessAndRepeat(t *testing.T) {
	t.Setenv("BLUNTCODE_DEMO_SEED", "1")
	ResetRateLimiter()
	s := testServer(t)
	call := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/system/seed-demo", bytes.NewReader([]byte(`{"preset":"small"}`)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}
	r1 := call()
	if r1.Code != http.StatusOK {
		t.Fatalf("first seed: %d %s", r1.Code, r1.Body.String())
	}
	var a map[string]any
	if err := json.Unmarshal(r1.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if a["preset"] != "small" {
		t.Fatalf("preset: %v", a)
	}
	r2 := call()
	if r2.Code != http.StatusOK {
		t.Fatalf("second seed: %d %s", r2.Code, r2.Body.String())
	}
}

func TestMetaReportsDemoSeedFlags(t *testing.T) {
	t.Setenv("BLUNTCODE_DEMO_SEED", "1")
	t.Setenv("BLUNTCODE_DEMO_TOKEN", "x")
	ResetRateLimiter()
	s := testServer(t)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/meta", nil))
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["demo_seed_env_enabled"] != true || m["demo_seed_requires_token"] != true {
		t.Fatalf("meta: %v", m)
	}
}

func TestGlobalStatsIncludesAssessmentRollupFields(t *testing.T) {
	s := testServer(t)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/stats", nil))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	raw := rec.Body.String()
	for _, key := range []string{`"scanned_workspaces"`, `"partial_assessments"`, `"last_assessment_at"`} {
		if !strings.Contains(raw, key) {
			t.Fatalf("stats missing %q: %s", key, raw)
		}
	}
}

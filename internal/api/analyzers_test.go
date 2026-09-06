package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/config"
	"bluntcode/internal/database"
	"bluntcode/internal/events"
	"bluntcode/internal/scans"
)

// analyzersEndpointServer builds a Server whose scans service carries a real
// registry, so GET /api/v1/analyzers can project the capability inventory.
func analyzersEndpointServer(t *testing.T, adapters ...analyzers.Analyzer) *Server {
	t.Helper()
	ResetRateLimiter()
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	registry := analyzers.NewRegistry()
	for _, adapter := range adapters {
		if err := registry.Register(adapter); err != nil {
			t.Fatal(err)
		}
	}
	service := scans.New(db, registry, events.New(), filepath.Join(paths.DataDir, "reports"), paths.ToolsDir, nil)
	return New(db, events.New(), service, nil, paths, "test", nil)
}

type stubCapabilityAnalyzer struct{ id string }

func (a stubCapabilityAnalyzer) ID() string                    { return a.id }
func (a stubCapabilityAnalyzer) DisplayName() string           { return "Stub " + a.id }
func (a stubCapabilityAnalyzer) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguagePython}
}
func (a stubCapabilityAnalyzer) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	return analyzers.ToolStatus{Ready: true, Version: "9.9"}
}
func (a stubCapabilityAnalyzer) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error { return nil }
func (a stubCapabilityAnalyzer) Plan(context.Context, analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	return analyzers.AnalyzerPlan{AnalyzerID: a.id}, nil
}
func (a stubCapabilityAnalyzer) Run(context.Context, analyzers.AnalyzerPlan, analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	return analyzers.AnalyzerResult{}, nil
}
func (a stubCapabilityAnalyzer) Normalize(context.Context, analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	return nil, nil, nil
}

// TestListAnalyzersServesCapabilityInventory pins the endpoint contract: one
// row per inventory entry (even ones the registry did not register), with
// registered adapters carrying live languages, version, and readiness.
func TestListAnalyzersServesCapabilityInventory(t *testing.T) {
	s := analyzersEndpointServer(t, stubCapabilityAnalyzer{id: "ruff"})
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/analyzers", nil)
	response := httptest.NewRecorder()
	s.mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var payload struct {
		Items []struct {
			ID         string   `json:"id"`
			Profiles   []string `json:"profiles"`
			Network    string   `json:"network"`
			Languages  []string `json:"languages"`
			Version    string   `json:"version"`
			Ready      bool     `json:"ready"`
			Registered bool     `json:"registered"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Items) != 12 {
		t.Fatalf("inventory served %d analyzers, want the 12 shipped entries", len(payload.Items))
	}
	byID := map[string]int{}
	for i, item := range payload.Items {
		byID[item.ID] = i
	}
	ruff, ok := byID["ruff"]
	if !ok {
		t.Fatal("ruff missing from inventory")
	}
	if !payload.Items[ruff].Registered || !payload.Items[ruff].Ready || payload.Items[ruff].Version != "9.9" {
		t.Errorf("registered ruff should carry live readiness/version: %+v", payload.Items[ruff])
	}
	if len(payload.Items[ruff].Languages) != 1 || payload.Items[ruff].Languages[0] != "python" {
		t.Errorf("ruff languages = %v, want [python] from the adapter", payload.Items[ruff].Languages)
	}
	// The pentest analyzer is withheld in offline mode (not registered here);
	// the inventory must still list it so the UI cannot undercount.
	pentest, ok := byID["pentest"]
	if !ok {
		t.Fatal("pentest missing from inventory")
	}
	if payload.Items[pentest].Registered {
		t.Error("pentest was not registered in this server; Registered must be false")
	}
	if payload.Items[byID["osv-dependencies"]].Network != "outbound" {
		t.Error("osv-dependencies must document its outbound network use")
	}
}

// A missing scans service must fail closed (503), never serve an empty list a
// caller could mistake for "no analyzers".
func TestListAnalyzersWithoutScanService(t *testing.T) {
	s := testServer(t) // nil scans service
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/analyzers", nil)
	response := httptest.NewRecorder()
	s.mux.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
)

// get fetches target through the full handler chain and returns the recorder.
func get(t *testing.T, s *Server, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "http://127.0.0.1"+target, nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	return response
}

// TestScanHistoryServesPagedEnvelope pins the scan history endpoint's page
// contract: {items,total,page,page_size,has_next}, page 1-based, page_size
// capped like the findings list, and 400s with JSON errors for garbage.
func TestScanHistoryServesPagedEnvelope(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Paged"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "completed"}); err != nil {
			t.Fatal(err)
		}
	}

	var body struct {
		Items    []core.Scan `json:"items"`
		Total    int         `json:"total"`
		Page     int         `json:"page"`
		PageSize int         `json:"page_size"`
		HasNext  bool        `json:"has_next"`
	}
	if err := json.Unmarshal(get(t, s, http.MethodGet, "/api/v1/workspaces/"+work.ID+"/scans?page=1&page_size=2").Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 || body.Total != 3 || body.Page != 1 || body.PageSize != 2 || !body.HasNext {
		t.Fatalf("first page wrong: %#v", body)
	}
	body = struct {
		Items    []core.Scan `json:"items"`
		Total    int         `json:"total"`
		Page     int         `json:"page"`
		PageSize int         `json:"page_size"`
		HasNext  bool        `json:"has_next"`
	}{}
	if err := json.Unmarshal(get(t, s, http.MethodGet, "/api/v1/workspaces/"+work.ID+"/scans?page=2&page_size=2").Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Page != 2 || body.HasNext {
		t.Fatalf("last page wrong: %#v", body)
	}
	// Absent params echo the applied defaults.
	body = struct {
		Items    []core.Scan `json:"items"`
		Total    int         `json:"total"`
		Page     int         `json:"page"`
		PageSize int         `json:"page_size"`
		HasNext  bool        `json:"has_next"`
	}{}
	if err := json.Unmarshal(get(t, s, http.MethodGet, "/api/v1/workspaces/"+work.ID+"/scans").Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Page != 1 || body.PageSize != database.DefaultScanPageSize || body.Total != 3 {
		t.Fatalf("defaults wrong: %#v", body)
	}
	if response := get(t, s, http.MethodGet, "/api/v1/workspaces/"+work.ID+"/scans?page=zero"); response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_SCAN_QUERY") {
		t.Fatalf("garbage page: %d %s", response.Code, response.Body.String())
	}
	if response := get(t, s, http.MethodGet, "/api/v1/workspaces/"+work.ID+"/scans?page_size=101"); response.Code != http.StatusBadRequest {
		t.Fatalf("oversized page_size: %d %s", response.Code, response.Body.String())
	}
}

// TestScanDetailCarriesDurationAndComparisonCounts pins the single-scan
// additions: duration_ms computed from the stored timestamps, and the
// new/fixed pair computed like the report comparison (previous finding gone
// while its analyzer succeeded counts as fixed; the survivor is persistent).
func TestScanDetailCarriesDurationAndComparisonCounts(t *testing.T) {
	s := testServer(t)
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: t.TempDir(), Name: "Delta"})
	if err != nil {
		t.Fatal(err)
	}
	completedScan(t, s, work.ID, "completed", finding("ruff", "GONE", "src/gone.py", "old issue", analyzers.SeverityHigh, analyzers.CategorySecurity))
	current := completedScan(t, s, work.ID, "completed", finding("ruff", "FRESH", "src/fresh.py", "new issue", analyzers.SeverityLow, analyzers.CategorySecurity))

	var body map[string]any
	if err := json.Unmarshal(get(t, s, http.MethodGet, "/api/v1/scans/"+current.ID).Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["duration_ms"]; !ok {
		t.Fatalf("completed scan must carry duration_ms: %v", body)
	}
	if body["new_count"] != float64(1) || body["fixed_count"] != float64(1) {
		t.Fatalf("comparison counts wrong: new=%v fixed=%v", body["new_count"], body["fixed_count"])
	}
	// A scan without a finish time has no duration to report.
	running, err := s.db.CreateScan(context.Background(), core.Scan{WorkspaceID: work.ID, State: "running"})
	if err != nil {
		t.Fatal(err)
	}
	body = map[string]any{}
	if err := json.Unmarshal(get(t, s, http.MethodGet, "/api/v1/scans/"+running.ID).Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["duration_ms"]; ok {
		t.Fatalf("running scan must omit duration_ms: %v", body["duration_ms"])
	}
}

// TestWorkspaceViewAttachesRunsAndCounts pins the workspace page's "Latest
// analysis" card contract: latest_scan carries analyzer_runs and the
// new/fixed pair like GET /scans/{id}, and latest_scan_coverage is
// lowercase snake case so the partial-coverage UI can read it.
func TestWorkspaceViewAttachesRunsAndCounts(t *testing.T) {
	s := testServer(t)
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: t.TempDir(), Name: "Card"})
	if err != nil {
		t.Fatal(err)
	}
	completedScan(t, s, work.ID, "completed", finding("ruff", "OLD", "src/old.py", "old issue", analyzers.SeverityHigh, analyzers.CategorySecurity))
	completedScan(t, s, work.ID, "completed", finding("ruff", "NEW", "src/new.py", "new issue", analyzers.SeverityLow, analyzers.CategoryCorrectness))

	raw := get(t, s, http.MethodGet, "/api/v1/workspaces/"+work.ID).Body.String()
	if !strings.Contains(raw, `"analyzer_runs":[{`) || !strings.Contains(raw, `"new_count":1`) || !strings.Contains(raw, `"fixed_count":1`) {
		t.Fatalf("latest_scan must carry runs and counts: %s", raw)
	}
	var body struct {
		LatestScan struct {
			AnalyzerRuns []map[string]any `json:"analyzer_runs"`
			NewCount     int              `json:"new_count"`
			FixedCount   int              `json:"fixed_count"`
			DurationMS   *int64           `json:"duration_ms"`
		} `json:"latest_scan"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.LatestScan.AnalyzerRuns) != 1 {
		t.Fatalf("latest_scan must carry analyzer_runs: %#v", body.LatestScan)
	}
	if body.LatestScan.NewCount != 1 || body.LatestScan.FixedCount != 1 || body.LatestScan.DurationMS == nil {
		t.Fatalf("latest_scan comparison wrong: %#v", body.LatestScan)
	}
	// The workspace list pairs the same latest scan with its coverage; the
	// coverage JSON must be lowercase snake case for the partial-coverage UI.
	var list struct {
		Items []struct {
			LatestScanCov *database.ScanCoverage `json:"latest_scan_coverage"`
		} `json:"items"`
	}
	listRaw := get(t, s, http.MethodGet, "/api/v1/workspaces").Body.String()
	if !strings.Contains(listRaw, `"latest_scan_coverage":{"scan_id":"`) {
		t.Fatalf("coverage must be snake case: %s", listRaw)
	}
	if err := json.Unmarshal([]byte(listRaw), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].LatestScanCov == nil || list.Items[0].LatestScanCov.Succeeded != 1 || list.Items[0].LatestScanCov.Total != 1 {
		t.Fatalf("coverage wrong: %#v", list.Items)
	}
}

// TestSearchFindingsDefaultsFacetsAndControls pins the global search fixes:
// applied pagination defaults are echoed, the severity facet counts the same
// filtered population, hostile control bytes in q are rejected, and LIKE
// wildcards in q match literally.
func TestSearchFindingsDefaultsFacetsAndControls(t *testing.T) {
	s := testServer(t)
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: t.TempDir(), Name: "Search"})
	if err != nil {
		t.Fatal(err)
	}
	current := completedScan(t, s, work.ID, "completed",
		finding("ruff", "BIG", "src/one.py", "serious issue", analyzers.SeverityCritical, analyzers.CategorySecurity),
		finding("semgrep", "small", "src/two.py", "minor issue", analyzers.SeverityLow, analyzers.CategoryStyle))

	var body struct {
		Total          int            `json:"total"`
		Page           int            `json:"page"`
		PageSize       int            `json:"page_size"`
		SeverityCounts map[string]int `json:"severity_counts"`
	}
	if err := json.Unmarshal(get(t, s, http.MethodGet, "/api/v1/findings/search").Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 2 || body.Page != 1 || body.PageSize != database.DefaultSearchPageSize {
		t.Fatalf("defaults wrong: %#v", body)
	}
	if body.SeverityCounts["critical"] != 1 || body.SeverityCounts["low"] != 1 || len(body.SeverityCounts) != 2 {
		t.Fatalf("severity facets wrong: %#v", body.SeverityCounts)
	}
	if response := get(t, s, http.MethodGet, "/api/v1/findings/search?q=run%00.py"); response.Code != http.StatusBadRequest {
		t.Fatalf("control bytes in q must 400: %d %s", response.Code, response.Body.String())
	}
	// % is a literal character to match, never a wildcard.
	body.SeverityCounts = nil
	if err := json.Unmarshal(get(t, s, http.MethodGet, "/api/v1/findings/search?q=%25").Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 0 {
		t.Fatalf("q=%% must match literally: %#v", body)
	}
	// The per-scan path filter honors the same escaping rule.
	var findings struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(get(t, s, http.MethodGet, "/api/v1/scans/"+current.ID+"/findings?path=%25").Body.Bytes(), &findings); err != nil {
		t.Fatal(err)
	}
	if findings.Total != 0 {
		t.Fatalf("path=%% must match literally: %d", findings.Total)
	}
	if err := json.Unmarshal(get(t, s, http.MethodGet, "/api/v1/scans/"+current.ID+"/findings?path=src").Body.Bytes(), &findings); err != nil {
		t.Fatal(err)
	}
	if findings.Total != 2 {
		t.Fatalf("plain path substring must still match: %d", findings.Total)
	}
}

// TestWorkspaceTagsRoutesLifecycle pins the tag endpoints' registration and
// behavior: PUT replaces the sorted, de-duplicated set, GET lists it, invalid
// tags 400, and unknown workspaces 404 with the JSON error envelope.
func TestWorkspaceTagsRoutesLifecycle(t *testing.T) {
	s := testServer(t)
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: t.TempDir(), Name: "Tagged"})
	if err != nil {
		t.Fatal(err)
	}
	put := func(target, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPut, "http://127.0.0.1"+target, strings.NewReader(body))
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		return response
	}
	response := put("/api/v1/workspaces/"+work.ID+"/tags", `{"tags":["zeta","alpha","zeta"]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("put tags: %d %s", response.Code, response.Body.String())
	}
	var body struct {
		Items []string `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 || body.Items[0] != "alpha" || body.Items[1] != "zeta" {
		t.Fatalf("tags wrong: %#v", body.Items)
	}
	if err := json.Unmarshal(get(t, s, http.MethodGet, "/api/v1/workspaces/"+work.ID+"/tags").Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 || body.Items[0] != "alpha" {
		t.Fatalf("get tags wrong: %#v", body.Items)
	}
	if response = put("/api/v1/workspaces/"+work.ID+"/tags", `{"tags":["has spaces!"]}`); response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_TAG") {
		t.Fatalf("invalid tag: %d %s", response.Code, response.Body.String())
	}
	missing := get(t, s, http.MethodGet, "/api/v1/workspaces/00000000-0000-0000-0000-000000000000/tags")
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), "WORKSPACE_NOT_FOUND") {
		t.Fatalf("unknown workspace: %d %s", missing.Code, missing.Body.String())
	}
}

// TestUppercaseIDsResolve pins case-insensitive ID handling: UUID path values
// are validated and looked up case-insensitively, so the uppercase spelling
// of a stored lowercase ID resolves everywhere.
func TestUppercaseIDsResolve(t *testing.T) {
	s := testServer(t)
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: t.TempDir(), Name: "Loud"})
	if err != nil {
		t.Fatal(err)
	}
	scan := completedScan(t, s, work.ID, "completed", finding("ruff", "UP", "src/up.py", "issue", analyzers.SeverityLow, analyzers.CategoryStyle))

	response := get(t, s, http.MethodGet, "/api/v1/workspaces/"+strings.ToUpper(work.ID))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"Loud"`) {
		t.Fatalf("uppercase workspace id: %d %s", response.Code, response.Body.String())
	}
	response = get(t, s, http.MethodGet, "/api/v1/scans/"+strings.ToUpper(scan.ID))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"id":"`+scan.ID+`"`) {
		t.Fatalf("uppercase scan id: %d %s", response.Code, response.Body.String())
	}
}

// TestTreeRootMissingReturns422 pins the dedicated error for a workspace
// whose root directory disappeared: a 422 JSON error, not a 500 discovery
// failure.
func TestTreeRootMissingReturns422(t *testing.T) {
	s := testServer(t)
	root := t.TempDir()
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: root, Name: "Ghost"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	response := get(t, s, http.MethodGet, "/api/v1/workspaces/"+work.ID+"/tree")
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "WORKSPACE_ROOT_MISSING") {
		t.Fatalf("missing root: %d %s", response.Code, response.Body.String())
	}
}

// TestHeadDoesNotReorderWorkspaces pins the HEAD rule: Go 1.22 routes HEAD to
// the GET handler, but a HEAD probe must not count as opening the workspace,
// so the recency-ordered list stays untouched.
func TestHeadDoesNotReorderWorkspaces(t *testing.T) {
	s := testServer(t)
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: t.TempDir(), Name: "Quiet"})
	if err != nil {
		t.Fatal(err)
	}
	if response := get(t, s, http.MethodHead, "/api/v1/workspaces/"+work.ID); response.Code != http.StatusOK {
		t.Fatalf("head: %d %s", response.Code, response.Body.String())
	}
	raw := get(t, s, http.MethodGet, "/api/v1/workspaces").Body.String()
	if strings.Contains(raw, "last_opened_at") {
		t.Fatalf("HEAD must not touch the workspace: %s", raw)
	}
	if response := get(t, s, http.MethodGet, "/api/v1/workspaces/"+work.ID); response.Code != http.StatusOK {
		t.Fatalf("get: %d %s", response.Code, response.Body.String())
	}
	raw = get(t, s, http.MethodGet, "/api/v1/workspaces").Body.String()
	if !strings.Contains(raw, "last_opened_at") {
		t.Fatalf("GET must touch the workspace: %s", raw)
	}
}

// TestUnknownAPIRouteCarriesNoStore pins the middleware-level cache rule:
// mux-generated 404/405 text responses that bypass writeJSON still carry
// Cache-Control: no-store.
func TestUnknownAPIRouteCarriesNoStore(t *testing.T) {
	s := testServer(t)
	response := get(t, s, http.MethodGet, "/api/v1/does-not-exist")
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown route: %d", response.Code)
	}
	if cache := response.Header().Get("Cache-Control"); cache != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cache)
	}
}

package api

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/config"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/events"
	"bluntcode/internal/tools"
)

// topLevelKeys decodes the response body's top-level object into its key set so
// the test can pin the exact fixed shape of the payload.
func topLevelKeys(t *testing.T, body []byte) []string {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("response is not a JSON object: %v: %s", err, body)
	}
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func equalKeys(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestGlobalStatsServesOverview pins GET /api/v1/stats: the aggregate values,
// the fixed top-level key set (severity as a fixed struct, never a map), the
// RFC3339 generated_at stamp, and the omission of the tools section when no
// tools service is wired into the server.
func TestGlobalStatsServesOverview(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	alpha, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Beta"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Gamma"}); err != nil {
		t.Fatal(err)
	}
	// Alpha: three completed scans, only the newest may feed the rollup.
	trendScan(t, s, alpha.ID, "completed", base,
		finding("ruff", "OLD-C", "src/old-c.py", "old critical", analyzers.SeverityCritical, analyzers.CategorySecurity),
		finding("ruff", "OLD-L", "src/old-l.py", "old low", analyzers.SeverityLow, analyzers.CategoryCorrectness))
	trendScan(t, s, alpha.ID, "completed_with_warnings", base.Add(time.Hour),
		finding("ruff", "MID-H", "src/mid-h.py", "mid high", analyzers.SeverityHigh, analyzers.CategoryCorrectness))
	trendScan(t, s, alpha.ID, "completed", base.Add(2*time.Hour),
		finding("ruff", "NEW-C", "src/new-c.py", "new critical", analyzers.SeverityCritical, analyzers.CategorySecurity),
		finding("ruff", "NEW-C2", "src/new-c2.py", "new critical 2", analyzers.SeverityCritical, analyzers.CategorySecurity),
		finding("ruff", "NEW-M", "src/new-m.py", "new medium", analyzers.SeverityMedium, analyzers.CategoryCorrectness))
	// Beta: a failed scan and an in-flight scan count in the totals but never
	// in the severity rollup.
	trendScan(t, s, beta.ID, "failed", base.Add(3*time.Hour),
		finding("ruff", "FAIL-H", "src/fail-h.py", "failed high", analyzers.SeverityHigh, analyzers.CategoryCorrectness))
	if _, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: beta.ID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	// Distinct 64-character hex fingerprints (the shape production writes):
	// the (workspace_id, fingerprint) key upserts, so a repeat would collapse
	// into one row.
	for i, work := range []core.Workspace{alpha, beta, alpha} {
		if _, err := s.db.AddSuppression(ctx, work.ID, strings.Repeat("a", 63)+string(rune('0'+i)), "wontfix"); err != nil {
			t.Fatal(err)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/stats", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("stats: %d %s", response.Code, response.Body.String())
	}
	var body statsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid stats payload: %v: %s", err, response.Body.String())
	}
	if body.Workspaces != 3 || body.Suppressions != 3 {
		t.Fatalf("workspace and suppression counters: %#v", body)
	}
	if body.Scans != (database.GlobalScans{Total: 5, Completed: 3, Running: 1}) {
		t.Fatalf("scan counters: %#v", body.Scans)
	}
	if body.Findings.Severity != (database.SeverityCounts{Critical: 2, Medium: 1}) || body.Findings.Total != 3 {
		t.Fatalf("findings must come from Alpha's newest completed scan only: %#v", body.Findings)
	}
	// Alpha's newest completed scan (2 critical + 1 medium = 22 weighted
	// points) grades C; Beta's failed scan and Gamma's missing history never
	// grade, so every workspace lands in exactly one bucket.
	wantGrades := map[string]int{"A": 0, "B": 0, "C": 1, "D": 0}
	if !maps.Equal(body.RiskGrades, wantGrades) {
		t.Fatalf("risk grades = %#v, want %#v", body.RiskGrades, wantGrades)
	}
	if body.Tools != nil {
		t.Fatalf("tools section must be omitted when no tools service is wired: %#v", body.Tools)
	}
	if _, err := time.Parse(time.RFC3339, body.GeneratedAt); err != nil {
		t.Fatalf("generated_at must be an RFC3339 timestamp: %q (%v)", body.GeneratedAt, err)
	}

	// The shape is fixed: exactly these top-level keys, and the severity
	// block carries exactly the five fixed severity names.
	keys := topLevelKeys(t, response.Body.Bytes())
	if !equalKeys(keys, []string{"findings", "generated_at", "risk_grades", "scans", "suppressions", "workspaces"}) {
		t.Fatalf("top-level keys = %v", keys)
	}
	var riskBlock struct {
		RiskGrades map[string]int `json:"risk_grades"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &riskBlock); err != nil {
		t.Fatalf("invalid risk_grades block: %v", err)
	}
	if !maps.Equal(riskBlock.RiskGrades, map[string]int{"A": 0, "B": 0, "C": 1, "D": 0}) {
		t.Fatalf("serialized risk_grades = %#v", riskBlock.RiskGrades)
	}
	var findingsBlock struct {
		Findings struct {
			Severity map[string]json.RawMessage `json:"severity"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &findingsBlock); err != nil {
		t.Fatalf("invalid findings block: %v", err)
	}
	severity := findingsBlock.Findings.Severity
	severityKeys := make([]string, 0, len(severity))
	for key := range severity {
		severityKeys = append(severityKeys, key)
	}
	sort.Strings(severityKeys)
	if !equalKeys(severityKeys, []string{"critical", "high", "info", "low", "medium"}) {
		t.Fatalf("severity keys = %v", severityKeys)
	}
}

// TestGlobalStatsIncludesToolReadiness pins the tools section when a tools
// service is wired: a count pair over the status machinery, with the full
// per-tool list left to GET /api/v1/tools.
func TestGlobalStatsIncludesToolReadiness(t *testing.T) {
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// An empty manifest yields one status per known tool, none ready.
	s := New(db, events.New(), nil, tools.NewService(t.TempDir(), tools.Manifest{}, false), paths, "test", nil)

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/stats", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("stats: %d %s", response.Code, response.Body.String())
	}
	var body statsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid stats payload: %v: %s", err, response.Body.String())
	}
	if body.Tools == nil || body.Tools.Total != 8 || body.Tools.Ready != 0 {
		t.Fatalf("tools section must count every known tool and the ready ones: %#v", body.Tools)
	}
	if keys := topLevelKeys(t, response.Body.Bytes()); !equalKeys(keys, []string{"findings", "generated_at", "risk_grades", "scans", "suppressions", "tools", "workspaces"}) {
		t.Fatalf("top-level keys = %v", keys)
	}
}

// TestGlobalStatsRejectsNonGET pins the method contract: the route is
// registered for GET only, so other methods get the ServeMux's 405 exactly like
// every other single-method route (health is asserted alongside as the
// reference behavior).
func TestGlobalStatsRejectsNonGET(t *testing.T) {
	s := testServer(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		request := httptest.NewRequest(method, "http://127.0.0.1/api/v1/stats", nil)
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		reference := httptest.NewRequest(method, "http://127.0.0.1/api/v1/health", nil)
		referenceResponse := httptest.NewRecorder()
		s.Handler().ServeHTTP(referenceResponse, reference)
		if response.Code != http.StatusMethodNotAllowed || response.Code != referenceResponse.Code {
			t.Fatalf("%s /api/v1/stats = %d (health reference %d)", method, response.Code, referenceResponse.Code)
		}
	}
}

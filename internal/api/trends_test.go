package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
)

type trendSeverity struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
}

type trendPoint struct {
	ScanID     string        `json:"scan_id"`
	FinishedAt time.Time     `json:"finished_at"`
	Profile    string        `json:"profile"`
	State      string        `json:"state"`
	Severity   trendSeverity `json:"severity"`
	Total      int           `json:"total"`
}

type trendsResponse struct {
	Items []trendPoint `json:"items"`
}

func fetchTrends(t *testing.T, s *Server, target string) trendsResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+target, nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var body trendsResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
		t.Fatalf("trends %s: %d %s", target, response.Code, response.Body.String())
	}
	return body
}

// trendScan seeds one scan whose finished_at is forced to a fixed instant, so
// ordering assertions never depend on wall-clock resolution between inserts.
func trendScan(t *testing.T, s *Server, workspaceID, state string, finished time.Time, findings ...analyzers.Finding) core.Scan {
	t.Helper()
	scan := completedScan(t, s, workspaceID, state, findings...)
	when := finished.UTC().Format(time.RFC3339Nano)
	if _, err := s.db.SQL.ExecContext(context.Background(), `UPDATE scans SET started_at=?, finished_at=? WHERE id=?`, when, when, scan.ID); err != nil {
		t.Fatal(err)
	}
	return scan
}

func TestWorkspaceTrendsReturnsCompletedScansOldestFirst(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	// Active and failed scans carry counts but must never chart; only the
	// producing terminal states feed the series.
	if _, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "running"}); err != nil {
		t.Fatal(err)
	}
	completedScan(t, s, work.ID, "failed", finding("ruff", "FAIL-C", "src/fail.py", "failed critical", analyzers.SeverityCritical, analyzers.CategorySecurity))
	oldest := trendScan(t, s, work.ID, "completed", base,
		finding("ruff", "A-C1", "src/a1.py", "old critical", analyzers.SeverityCritical, analyzers.CategorySecurity),
		finding("ruff", "A-C2", "src/a2.py", "old critical 2", analyzers.SeverityCritical, analyzers.CategorySecurity),
		finding("ruff", "A-H", "src/a3.py", "old high", analyzers.SeverityHigh, analyzers.CategoryCorrectness),
		finding("ruff", "A-M1", "src/a4.py", "old medium", analyzers.SeverityMedium, analyzers.CategoryStyle),
		finding("ruff", "A-M2", "src/a5.py", "old medium 2", analyzers.SeverityMedium, analyzers.CategoryStyle),
		finding("ruff", "A-M3", "src/a6.py", "old medium 3", analyzers.SeverityMedium, analyzers.CategoryStyle),
		finding("ruff", "A-I", "src/a7.py", "old info", analyzers.SeverityInfo, analyzers.CategoryStyle))
	middle := trendScan(t, s, work.ID, "completed_with_warnings", base.Add(24*time.Hour),
		finding("ruff", "B-L", "src/b1.py", "middle low", analyzers.SeverityLow, analyzers.CategoryCorrectness))
	newest := trendScan(t, s, work.ID, "completed", base.Add(48*time.Hour))

	body := fetchTrends(t, s, "/api/v1/workspaces/"+work.ID+"/trends")
	if len(body.Items) != 3 {
		t.Fatalf("only the three completed scans chart: %#v", body.Items)
	}
	if body.Items[0].ScanID != oldest.ID || body.Items[1].ScanID != middle.ID || body.Items[2].ScanID != newest.ID {
		t.Fatalf("series must read oldest to newest: %#v", body.Items)
	}
	first := body.Items[0]
	if !first.FinishedAt.Equal(base) {
		t.Fatalf("oldest point timestamp = %v, want %v", first.FinishedAt, base)
	}
	if first.Profile != "standard" || first.State != "completed" {
		t.Fatalf("oldest point identity: %#v", first)
	}
	if first.Severity != (trendSeverity{Critical: 2, High: 1, Medium: 3, Low: 0, Info: 1}) || first.Total != 7 {
		t.Fatalf("oldest point counts must match the persisted severity split: %#v", first.Severity)
	}
	if body.Items[1].State != "completed_with_warnings" || body.Items[1].Severity != (trendSeverity{Low: 1}) || body.Items[1].Total != 1 {
		t.Fatalf("warning-completed scan must chart: %#v", body.Items[1])
	}
	if body.Items[2].Severity != (trendSeverity{}) || body.Items[2].Total != 0 {
		t.Fatalf("zero-finding scan charts as an empty point: %#v", body.Items[2])
	}

	// limit picks the newest window and still returns it oldest-first.
	limited := fetchTrends(t, s, "/api/v1/workspaces/"+work.ID+"/trends?limit=2")
	if len(limited.Items) != 2 || limited.Items[0].ScanID != middle.ID || limited.Items[1].ScanID != newest.ID {
		t.Fatalf("limit window: %#v", limited.Items)
	}
	single := fetchTrends(t, s, "/api/v1/workspaces/"+work.ID+"/trends?limit=1")
	if len(single.Items) != 1 || single.Items[0].ScanID != newest.ID {
		t.Fatalf("limit 1 keeps only the newest scan: %#v", single.Items)
	}

	// A workspace with no completed scans reports an empty array, never null.
	empty, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Empty"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces/"+empty.ID+"/trends", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items":[]`) {
		t.Fatalf("empty workspace: %d %s", response.Code, response.Body.String())
	}

	// Unknown and malformed workspace ids share the 404 pattern.
	for _, id := range []string{"11111111-1111-1111-1111-111111111111", "not-a-uuid"} {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces/"+id+"/trends", nil)
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "WORKSPACE_NOT_FOUND") {
			t.Fatalf("workspace %s: got %d %s", id, response.Code, response.Body.String())
		}
	}
}

func TestWorkspaceTrendsLimitDefaultsAndBounds(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	// 25 completed scans with forced, strictly increasing timestamps.
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	scans := make([]core.Scan, 25)
	for i := range scans {
		scans[i] = trendScan(t, s, work.ID, "completed", base.Add(time.Duration(i)*time.Hour),
			finding("ruff", "RULE-"+strings.Repeat("A", i+1), "src/file.py", "issue", analyzers.SeverityLow, analyzers.CategoryCorrectness))
	}

	// Default limit keeps the newest 20 scans, oldest first.
	body := fetchTrends(t, s, "/api/v1/workspaces/"+work.ID+"/trends")
	if len(body.Items) != 20 {
		t.Fatalf("default limit returned %d points, want 20", len(body.Items))
	}
	if body.Items[0].ScanID != scans[5].ID || body.Items[19].ScanID != scans[24].ID {
		t.Fatalf("default window must span scans 5..24 oldest to newest: %s..%s", body.Items[0].ScanID, body.Items[19].ScanID)
	}

	// The maximum accepts the whole history here (25 scans < 100 cap).
	full := fetchTrends(t, s, "/api/v1/workspaces/"+work.ID+"/trends?limit=100")
	if len(full.Items) != 25 || full.Items[24].ScanID != scans[24].ID {
		t.Fatalf("limit 100 returned %d points", len(full.Items))
	}

	// Beyond the cap, zero, and non-numeric limits all reject with the shared
	// 400 envelope.
	for _, query := range []string{"?limit=101", "?limit=0", "?limit=-3", "?limit=abc", "?limit=1.5"} {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces/"+work.ID+"/trends"+query, nil)
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_TREND_QUERY") {
			t.Fatalf("trends %s: got %d %s", query, response.Code, response.Body.String())
		}
	}
}

// TestWorkspaceTrendsCarryScoreAndGrade pins the grade-history fields: each
// point carries the /risk score and band for that scan's counts.
func TestWorkspaceTrendsCarryScoreAndGrade(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Graded"})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	trendScan(t, s, work.ID, "completed", base)
	trendScan(t, s, work.ID, "completed", base.Add(time.Hour),
		finding("ruff", "G-C1", "src/g1.py", "c1", analyzers.SeverityCritical, analyzers.CategorySecurity),
		finding("ruff", "G-C2", "src/g2.py", "c2", analyzers.SeverityCritical, analyzers.CategorySecurity),
		finding("ruff", "G-C3", "src/g3.py", "c3", analyzers.SeverityCritical, analyzers.CategorySecurity),
		finding("ruff", "G-C4", "src/g4.py", "c4", analyzers.SeverityCritical, analyzers.CategorySecurity),
		finding("ruff", "G-C5", "src/g5.py", "c5", analyzers.SeverityCritical, analyzers.CategorySecurity))
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces/"+work.ID+"/trends", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var body struct {
		Items []struct {
			Score float64 `json:"score"`
			Grade string  `json:"grade"`
			Total int     `json:"total"`
		} `json:"items"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || len(body.Items) != 2 {
		t.Fatalf("trends: %d %s", response.Code, response.Body.String())
	}
	if body.Items[0].Score != 0 || body.Items[0].Grade != "A" {
		t.Fatalf("clean scan grade: %#v", body.Items[0])
	}
	want := riskScoreOf(database.RiskCounts{Critical: 5})
	if body.Items[1].Score != want || body.Items[1].Grade != riskGrade(want) || body.Items[1].Grade == "A" {
		t.Fatalf("critical scan grade: %#v want score %v", body.Items[1], want)
	}
}

// TestListWorkspacesCarriesSparkline pins the card sparkline on GET
// /workspaces: completed totals only, oldest first, omitted when none exist.
func TestListWorkspacesCarriesSparkline(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Spark"})
	if err != nil {
		t.Fatal(err)
	}
	idle, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Idle"})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	trendScan(t, s, work.ID, "completed", base,
		finding("ruff", "S-1", "src/s1.py", "s1", analyzers.SeverityLow, analyzers.CategoryStyle),
		finding("ruff", "S-2", "src/s2.py", "s2", analyzers.SeverityLow, analyzers.CategoryStyle))
	trendScan(t, s, work.ID, "failed", base.Add(time.Hour))
	trendScan(t, s, work.ID, "completed", base.Add(2*time.Hour),
		finding("ruff", "S-3", "src/s3.py", "s3", analyzers.SeverityLow, analyzers.CategoryStyle))
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var body struct {
		Items []struct {
			ID    string `json:"id"`
			Trend []int  `json:"trend"`
		} `json:"items"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
		t.Fatalf("workspaces: %d %s", response.Code, response.Body.String())
	}
	found := 0
	for _, item := range body.Items {
		switch item.ID {
		case work.ID:
			found++
			if len(item.Trend) != 2 || item.Trend[0] != 2 || item.Trend[1] != 1 {
				t.Fatalf("sparkline = %v, want [2 1]", item.Trend)
			}
		case idle.ID:
			found++
			if item.Trend != nil {
				t.Fatalf("idle workspace has no sparkline: %v", item.Trend)
			}
		}
	}
	if found != 2 {
		t.Fatalf("both workspaces listed: %s", response.Body.String())
	}
}

package api

// IMP-13 acceptance: the letter grade is paired with the coverage and
// freshness of the scan behind it, and every surface that quotes scan totals
// (dashboard card, risk endpoint, scan detail, findings list, global stats)
// reconciles from the same stored scan.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
)

// partialScan stores a scan with a mixed analyzer outcome: one clean run,
// one failed run, and one run that succeeded with degraded output.
func partialScan(t *testing.T, s *Server, workspaceID, state string, findings ...analyzers.Finding) core.Scan {
	t.Helper()
	scan, err := s.db.CreateScan(context.Background(), core.Scan{WorkspaceID: workspaceID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.SaveAnalyzerResult(context.Background(), scan.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "t", State: "succeeded"}, findings, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.SaveAnalyzerResult(context.Background(), scan.ID, database.AnalyzerRunInput{AnalyzerID: "semgrep", Version: "t", State: "failed", Error: "boom"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.SaveAnalyzerResult(context.Background(), scan.ID, database.AnalyzerRunInput{AnalyzerID: "trivy", Version: "t", State: "succeeded", WarningCount: 2}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(context.Background(), scan.ID, state, ""); err != nil {
		t.Fatal(err)
	}
	return scan
}

// TestWorkspaceRiskPairsGradeWithCoverageAndFreshness verifies the IMP-13
// pairing: a grade computed from a partially-covered scan arrives together
// with the scan state, its finish time, and the analyzer outcome counts that
// say how much weight the grade can carry.
func TestWorkspaceRiskPairsGradeWithCoverageAndFreshness(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	partial, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Partial"})
	if err != nil {
		t.Fatal(err)
	}
	partialScan(t, s, partial.ID, "completed_with_warnings",
		finding("ruff", "R1", "src/a.py", "one high", analyzers.SeverityHigh, analyzers.CategoryStyle),
	)
	clean, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Clean"})
	if err != nil {
		t.Fatal(err)
	}
	completedScan(t, s, clean.ID, "completed",
		finding("ruff", "R1", "src/a.py", "one high", analyzers.SeverityHigh, analyzers.CategoryStyle),
	)

	fetch := func(id string) map[string]any {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces/"+id+"/risk", nil)
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		var body map[string]any
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
			t.Fatalf("risk(%s): %d %s", id, response.Code, response.Body.String())
		}
		return body
	}

	partialBody := fetch(partial.ID)
	if partialBody["scan_state"] != "completed_with_warnings" {
		t.Fatalf("partial risk hides its scan state: %v", partialBody)
	}
	if _, ok := partialBody["finished_at"]; !ok {
		t.Fatalf("partial risk lacks a finish time: %v", partialBody)
	}
	coverage, ok := partialBody["coverage"].(map[string]any)
	if !ok {
		t.Fatalf("partial risk lacks coverage pairing: %v", partialBody)
	}
	for key, want := range map[string]float64{"total": 3, "succeeded": 2, "failed": 1, "warned": 1} {
		if got := coverage[key].(float64); got != want {
			t.Fatalf("coverage[%s] = %v, want %v (full: %v)", key, got, want, coverage)
		}
	}
	if partialBody["complete"] != false {
		t.Fatal("a scan with a failed and a warned analyzer run must not claim complete coverage")
	}

	cleanBody := fetch(clean.ID)
	if cleanBody["complete"] != true {
		t.Fatalf("clean scan should claim complete coverage: %v", cleanBody)
	}
	cleanCoverage := cleanBody["coverage"].(map[string]any)
	if cleanCoverage["total"].(float64) != 1 || cleanCoverage["succeeded"].(float64) != 1 || cleanCoverage["failed"].(float64) != 0 {
		t.Fatalf("clean coverage wrong: %v", cleanCoverage)
	}
	// Same finding shape, same score — the difference the UI must show is the
	// coverage pairing, not a silently shifted grade.
	if partialBody["score"] != cleanBody["score"] {
		t.Fatalf("identical findings must score identically: %v vs %v", partialBody["score"], cleanBody["score"])
	}
}

// TestWorkspaceListPairsLatestScanWithCoverage verifies the dashboard payload
// carries the same pairing per workspace card.
func TestWorkspaceListPairsLatestScanWithCoverage(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Paired"})
	if err != nil {
		t.Fatal(err)
	}
	partialScan(t, s, work.ID, "completed_with_warnings")

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var body struct {
		Items []struct {
			LatestScan *core.Scan `json:"latest_scan"`
			Coverage   *struct {
				Total     int `json:"total"`
				Succeeded int `json:"succeeded"`
				Failed    int `json:"failed"`
				Warned    int `json:"warned"`
			} `json:"latest_scan_coverage"`
		} `json:"items"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
		t.Fatalf("workspaces: %d %s", response.Code, response.Body.String())
	}
	if len(body.Items) != 1 || body.Items[0].Coverage == nil {
		t.Fatalf("workspace card lacks coverage pairing: %#v", body.Items)
	}
	cov := body.Items[0].Coverage
	if cov.Total != 3 || cov.Succeeded != 2 || cov.Failed != 1 || cov.Warned != 1 {
		t.Fatalf("card coverage = %+v, want 3 total / 2 succeeded / 1 failed / 1 warned", cov)
	}
}

// TestScanTotalsReconcileAcrossViews is the IMP-13 reconciliation
// acceptance: dashboard card, risk endpoint, scan detail, paginated findings,
// and the global overview all quote the same numbers for one stored scan.
func TestScanTotalsReconcileAcrossViews(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Reconciled"})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	highs := []analyzers.Finding{
		finding("ruff", "H1", "src/a.py", "high one", analyzers.SeverityHigh, analyzers.CategoryStyle),
		finding("semgrep", "H2", "src/b.py", "high two", analyzers.SeverityHigh, analyzers.CategorySecurity),
	}
	if _, err := s.db.SaveAnalyzerResult(ctx, scan.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "t", State: "succeeded"}, highs[:1], nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.SaveAnalyzerResult(ctx, scan.ID, database.AnalyzerRunInput{AnalyzerID: "semgrep", Version: "t", State: "succeeded"}, highs[1:], nil); err != nil {
		t.Fatal(err)
	}
	low := finding("ruff", "L1", "src/c.py", "low one", analyzers.SeverityLow, analyzers.CategoryStyle)
	if _, err := s.db.SaveAnalyzerResult(ctx, scan.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "t", State: "succeeded"}, []analyzers.Finding{low}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(ctx, scan.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}

	get := func(path string, dest any) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+path, nil)
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), dest) != nil {
			t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
		}
	}

	var cards struct {
		Items []struct {
			LatestScan *core.Scan `json:"latest_scan"`
		} `json:"items"`
	}
	get("/api/v1/workspaces", &cards)
	card := cards.Items[0].LatestScan
	if card == nil || card.TotalFindings != 3 || card.HighCount != 2 || card.LowCount != 1 {
		t.Fatalf("dashboard card totals wrong: %#v", card)
	}

	var risk map[string]any
	get("/api/v1/workspaces/"+work.ID+"/risk", &risk)
	counts := risk["counts"].(map[string]any)
	if counts["high"].(float64) != 2 || counts["low"].(float64) != 1 || risk["score"].(float64) != 11 {
		t.Fatalf("risk totals wrong: %v", risk)
	}

	var detail struct {
		core.Scan
		AnalyzerRuns []map[string]any `json:"analyzer_runs"`
	}
	get("/api/v1/scans/"+scan.ID, &detail)
	if detail.TotalFindings != 3 || len(detail.AnalyzerRuns) != 3 {
		t.Fatalf("scan detail wrong: total=%d runs=%d", detail.TotalFindings, len(detail.AnalyzerRuns))
	}

	var list findingsResponse
	get("/api/v1/scans/"+scan.ID+"/findings?page=1&page_size=2", &list)
	if list.Total != 3 || len(list.Items) != 2 || list.HasMore != true {
		t.Fatalf("findings list wrong: total=%d items=%d more=%v", list.Total, len(list.Items), list.HasMore)
	}

	var stats struct {
		Findings struct {
			Severity map[string]int `json:"severity"`
			Total    int            `json:"total"`
		} `json:"findings"`
	}
	get("/api/v1/stats", &stats)
	if stats.Findings.Total != 3 || stats.Findings.Severity["high"] != 2 || stats.Findings.Severity["low"] != 1 {
		t.Fatalf("global stats wrong: %+v", stats.Findings)
	}
}

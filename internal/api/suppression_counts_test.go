package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
)

// TestSuppressionRecountsScanTotals: suppressing a finding drops it from the
// scan's persisted counts (header, grade, history) immediately, and
// restoring it puts it back — no new scan needed.
func TestSuppressionRecountsScanTotals(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Counts"})
	if err != nil {
		t.Fatal(err)
	}
	high := finding("ruff", "S1", "a.py", "high one", analyzers.SeverityHigh, analyzers.CategorySecurity)
	low := finding("ruff", "S2", "b.py", "low one", analyzers.SeverityLow, analyzers.CategoryCorrectness)
	scan, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.SaveAnalyzerResult(ctx, scan.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{high, low}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(ctx, scan.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	counts := func() (int, int, int) {
		t.Helper()
		got, err := s.db.Scan(ctx, scan.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.TotalFindings, got.HighCount, got.LowCount
	}
	if total, h, l := counts(); total != 2 || h != 1 || l != 1 {
		t.Fatalf("before: total=%d high=%d low=%d", total, h, l)
	}
	body, _ := json.Marshal(map[string]string{"fingerprint": high.Fingerprint, "reason": "False positive"})
	if response := send(t, s, http.MethodPost, "/api/v1/workspaces/"+work.ID+"/suppressions", string(body)); response.Code != http.StatusCreated {
		t.Fatalf("suppress: %d %s", response.Code, response.Body.String())
	}
	if total, h, l := counts(); total != 1 || h != 0 || l != 1 {
		t.Fatalf("after suppress: total=%d high=%d low=%d", total, h, l)
	}
	if response := send(t, s, http.MethodDelete, "/api/v1/workspaces/"+work.ID+"/suppressions/"+high.Fingerprint, ""); response.Code != http.StatusNoContent {
		t.Fatalf("restore: %d %s", response.Code, response.Body.String())
	}
	if total, h, l := counts(); total != 2 || h != 1 || l != 1 {
		t.Fatalf("after restore: total=%d high=%d low=%d", total, h, l)
	}
}

// TestFindingsFirstPageCarriesStatusCounts: page-mode first pages include
// status_counts that count suppressed findings too.
func TestFindingsFirstPageCarriesStatusCounts(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Status"})
	if err != nil {
		t.Fatal(err)
	}
	a := finding("ruff", "A", "a.py", "a", analyzers.SeverityHigh, analyzers.CategorySecurity)
	b := finding("ruff", "B", "b.py", "b", analyzers.SeverityLow, analyzers.CategorySecurity)
	scan, _ := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if _, err := s.db.SaveAnalyzerResult(ctx, scan.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{a, b}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(ctx, scan.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.AddSuppression(ctx, work.ID, a.Fingerprint, "Safe by design"); err != nil {
		t.Fatal(err)
	}
	response := get(t, s, http.MethodGet, "/api/v1/scans/"+scan.ID+"/findings?page=1&page_size=25&status=new")
	var body struct {
		Total        int            `json:"total"`
		StatusCounts map[string]int `json:"status_counts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 1 || body.StatusCounts["new"] != 1 || body.StatusCounts["suppressed"] != 1 || body.StatusCounts["persistent"] != 0 {
		t.Fatalf("body %s", response.Body.String())
	}
}

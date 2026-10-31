package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
)

// TestWorkspaceListStaysFastWithLargeScans guards GET /workspaces, which
// computes each card's new/fixed pair from the latest and previous scans.
// The pair only needs identity columns, so the list must not pay for whole
// finding rows (messages, metadata JSON) on every dashboard refresh.
func TestWorkspaceListStaysFastWithLargeScans(t *testing.T) {
	if testing.Short() {
		t.Skip("large seed skipped in -short mode")
	}
	s := testServer(t)
	ctx := context.Background()
	const workspaces, perScan = 20, 2000
	for w := 0; w < workspaces; w++ {
		work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: fmt.Sprintf("ws-%02d", w)})
		if err != nil {
			t.Fatal(err)
		}
		build := func(offset int) []analyzers.Finding {
			out := make([]analyzers.Finding, 0, perScan)
			for i := 0; i < perScan; i++ {
				f := finding("ruff", fmt.Sprintf("R%04d", i+offset), fmt.Sprintf("src/m%03d.py", (i+offset)%300), fmt.Sprintf("issue %d with a reasonably long explanatory message body", i+offset), analyzers.SeverityMedium, analyzers.CategoryCorrectness)
				f.Metadata = map[string]any{"context": "some metadata payload", "index": i}
				out = append(out, f)
			}
			return out
		}
		completedScan(t, s, work.ID, "completed", build(0)...)
		completedScan(t, s, work.ID, "completed", build(perScan/2)...)
	}
	handler := s.Handler()
	var worst time.Duration
	for round := 0; round < 3; round++ {
		started := time.Now()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces", nil))
		elapsed := time.Since(started)
		if response.Code != http.StatusOK {
			t.Fatalf("status %d: %s", response.Code, response.Body.String())
		}
		var body struct {
			Items []workspaceView `json:"items"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Items) != workspaces {
			t.Fatalf("items=%d err=%v", len(body.Items), err)
		}
		for _, item := range body.Items {
			if item.LatestScan == nil || item.LatestScan.NewCount != perScan/2 || item.LatestScan.FixedCount != perScan/2 {
				t.Fatalf("%s: comparison = %+v, want %d new / %d fixed", item.Name, item.LatestScan, perScan/2, perScan/2)
			}
		}
		worst = max(worst, elapsed)
	}
	t.Logf("GET /workspaces worst of 3: %s (%d workspaces x 2 scans x %d findings)", worst, workspaces, perScan)
	if worst > 2*time.Second {
		t.Fatalf("GET /workspaces took %s", worst)
	}
}

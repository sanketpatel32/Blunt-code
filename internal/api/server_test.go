package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/config"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/events"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	paths, err := config.NewPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(db, events.New(), nil, nil, paths, "test", nil)
}

func TestFindingsFiltersPaginationAndComparisonStatus(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	persistent := finding("ruff", "PERSIST", "src/persistent.py", "same issue", analyzers.SeverityHigh, analyzers.CategorySecurity)
	previous, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.SaveAnalyzerResult(ctx, previous.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{persistent}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(ctx, previous.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	current, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	items := []analyzers.Finding{persistent}
	for i := 1; i < 30; i++ {
		severity := analyzers.SeverityMedium
		if i%2 == 0 {
			severity = analyzers.SeverityHigh
		}
		items = append(items, finding("ruff", "RULE-"+strconv.Itoa(i), "src/file-"+strconv.Itoa(i)+".py", "Searchable issue "+strconv.Itoa(i), severity, analyzers.CategoryCorrectness))
	}
	if _, err := s.db.SaveAnalyzerResult(ctx, current.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, items, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(ctx, current.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+current.ID+"/findings?limit=25&sort=path", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var first findingsResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &first) != nil {
		t.Fatalf("first page: %d %s", response.Code, response.Body.String())
	}
	if first.Total != 30 || len(first.Items) != 25 || !first.HasMore || first.NextOffset == nil || *first.NextOffset != 25 {
		t.Fatalf("unexpected first page: %#v", first)
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+current.ID+"/findings?limit=25&offset=25", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var second findingsResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &second) != nil || len(second.Items) != 5 || second.HasMore {
		t.Fatalf("second page: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+current.ID+"/findings?severity=high&category=security&analyzer=ruff&path=persistent&q=PERSIST&status=persistent", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var filtered findingsResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &filtered) != nil || filtered.Total != 1 || len(filtered.Items) != 1 || filtered.Items[0].Status != "persistent" {
		t.Fatalf("filtered page: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+current.ID+"/findings?limit=17", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid pagination was accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestFindingPreviewReturnsContainedSourceExcerpt(t *testing.T) {
	s := testServer(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.ts"), []byte("first\nsecond issue\nthird\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: root, Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := s.db.CreateScan(context.Background(), core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	f := finding("biome", "lint/suspicious/example", "src/main.ts", "Example issue", analyzers.SeverityLow, analyzers.CategoryMaintainability)
	f.StartLine, f.StartColumn, f.EndLine, f.EndColumn = 2, 1, 2, 7
	if _, err := s.db.SaveAnalyzerResult(context.Background(), scan.ID, database.AnalyzerRunInput{AnalyzerID: "biome", Version: "test", State: "succeeded"}, []analyzers.Finding{f}, nil); err != nil {
		t.Fatal(err)
	}
	page, err := s.db.FindingsPage(context.Background(), scan, database.FindingFilter{Limit: 25})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID == "" {
		t.Fatalf("saved finding: %#v, err %v", page, err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scan.ID+"/findings/"+page.Items[0].ID+"/preview", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"number":2`) || !strings.Contains(response.Body.String(), "second issue") || !strings.Contains(response.Body.String(), `"highlight_start_line":2`) {
		t.Fatalf("preview: %d %s", response.Code, response.Body.String())
	}
}

func TestGetScanIncludesAnalyzerProgress(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.SaveAnalyzerResult(ctx, scan.ID, database.AnalyzerRunInput{AnalyzerID: "biome", Version: "test", State: "succeeded"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scan.ID, nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var body struct {
		AnalyzerRuns []struct {
			AnalyzerID string `json:"analyzer_id"`
			Status     string `json:"status"`
		} `json:"analyzer_runs"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
		t.Fatalf("scan: %d %s", response.Code, response.Body.String())
	}
	if len(body.AnalyzerRuns) != 1 || body.AnalyzerRuns[0].AnalyzerID != "biome" || body.AnalyzerRuns[0].Status != "succeeded" {
		t.Fatalf("unexpected analyzer progress: %#v", body.AnalyzerRuns)
	}
}

func TestWorkspaceListIncludesLanguagesAndLatestScan(t *testing.T) {
	s := testServer(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.py"), []byte("x = 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: root, Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := s.db.CreateScan(context.Background(), core.Scan{WorkspaceID: work.ID, State: "queued", Snapshot: &core.ScanSnapshot{Languages: map[string]int{"python": 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(context.Background(), scan.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var body struct {
		Items []struct {
			Languages  []string   `json:"languages"`
			LatestScan *core.Scan `json:"latest_scan"`
		} `json:"items"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
		t.Fatalf("workspaces: %d %s", response.Code, response.Body.String())
	}
	if len(body.Items) != 1 || len(body.Items[0].Languages) != 1 || body.Items[0].Languages[0] != "python" || body.Items[0].LatestScan == nil || body.Items[0].LatestScan.State != "completed" {
		t.Fatalf("unexpected workspace card data: %#v", body.Items)
	}
}

type findingsResponse struct {
	Items      []analyzers.Finding `json:"items"`
	Total      int                 `json:"total"`
	HasMore    bool                `json:"has_more"`
	NextOffset *int                `json:"next_offset"`
}

type recentScansResponse struct {
	Scans   []database.RecentScan `json:"scans"`
	Total   int                   `json:"total"`
	Summary database.ScanSummary  `json:"summary"`
}

func TestRecentScansListsAcrossWorkspacesNewestFirst(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	alpha, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Beta"})
	if err != nil {
		t.Fatal(err)
	}
	first := completedScan(t, s, alpha.ID, "completed", finding("ruff", "A1", "src/a1.py", "old issue", analyzers.SeverityHigh, analyzers.CategoryCorrectness))
	second := completedScan(t, s, beta.ID, "completed", finding("ruff", "B1", "src/b1.py", "beta issue", analyzers.SeverityLow, analyzers.CategoryCorrectness))
	third, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: alpha.ID, State: "running"})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var body recentScansResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
		t.Fatalf("recent scans: %d %s", response.Code, response.Body.String())
	}
	if body.Total != 3 || len(body.Scans) != 3 {
		t.Fatalf("unexpected feed size: total=%d items=%d", body.Total, len(body.Scans))
	}
	if body.Scans[0].ID != third.ID || body.Scans[0].WorkspaceID != alpha.ID || body.Scans[0].WorkspaceName != "Alpha" || body.Scans[0].State != "running" {
		t.Fatalf("newest scan is not first: %#v", body.Scans[0])
	}
	if body.Scans[1].ID != second.ID || body.Scans[1].WorkspaceName != "Beta" || body.Scans[2].ID != first.ID {
		t.Fatalf("unexpected ordering or workspace join: %#v", body.Scans)
	}
	if body.Summary.WorkspacesTotal != 2 || body.Summary.ActiveScans != 1 || body.Summary.ScansTotal != 3 {
		t.Fatalf("unexpected summary alongside feed: %#v", body.Summary)
	}

	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans?limit=2", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || len(body.Scans) != 2 || body.Total != 3 {
		t.Fatalf("limited feed: %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans?limit=500", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || len(body.Scans) != 3 {
		t.Fatalf("oversized limit must clamp to 50, not fail: %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans?state=completed", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.Total != 2 || len(body.Scans) != 2 {
		t.Fatalf("state filter: %d %s", response.Code, response.Body.String())
	}
	for _, scan := range body.Scans {
		if scan.State != "completed" {
			t.Fatalf("state filter leaked other states: %#v", body.Scans)
		}
	}

	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans?state=bogus", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown state was accepted: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans?limit=0", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("zero limit was accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestScansSummarySumsLatestCompletedScanPerWorkspace(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
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
	// Alpha's history: only the newest completed scan may feed the totals.
	completedScan(t, s, alpha.ID, "completed_with_warnings",
		finding("ruff", "OLD-C", "src/old-c.py", "old critical", analyzers.SeverityCritical, analyzers.CategorySecurity),
		finding("ruff", "OLD-L", "src/old-l.py", "old low", analyzers.SeverityLow, analyzers.CategoryCorrectness))
	completedScan(t, s, alpha.ID, "completed",
		finding("ruff", "MID-H", "src/mid-h.py", "mid high", analyzers.SeverityHigh, analyzers.CategoryCorrectness),
		finding("ruff", "MID-H2", "src/mid-h2.py", "mid high 2", analyzers.SeverityHigh, analyzers.CategoryCorrectness),
		finding("ruff", "MID-H3", "src/mid-h3.py", "mid high 3", analyzers.SeverityHigh, analyzers.CategoryCorrectness),
		finding("ruff", "MID-H4", "src/mid-h4.py", "mid high 4", analyzers.SeverityHigh, analyzers.CategoryCorrectness),
		finding("ruff", "MID-H5", "src/mid-h5.py", "mid high 5", analyzers.SeverityHigh, analyzers.CategoryCorrectness),
		finding("ruff", "MID-I", "src/mid-i.py", "mid info", analyzers.SeverityInfo, analyzers.CategoryCorrectness),
		finding("ruff", "MID-I2", "src/mid-i2.py", "mid info 2", analyzers.SeverityInfo, analyzers.CategoryCorrectness),
		finding("ruff", "MID-I3", "src/mid-i3.py", "mid info 3", analyzers.SeverityInfo, analyzers.CategoryCorrectness),
		finding("ruff", "MID-I4", "src/mid-i4.py", "mid info 4", analyzers.SeverityInfo, analyzers.CategoryCorrectness),
		finding("ruff", "MID-I5", "src/mid-i5.py", "mid info 5", analyzers.SeverityInfo, analyzers.CategoryCorrectness))
	completedScan(t, s, alpha.ID, "completed_with_warnings",
		finding("ruff", "NEW-C", "src/new-c.py", "new critical", analyzers.SeverityCritical, analyzers.CategorySecurity),
		finding("ruff", "NEW-C2", "src/new-c2.py", "new critical 2", analyzers.SeverityCritical, analyzers.CategorySecurity),
		finding("ruff", "NEW-M", "src/new-m.py", "new medium", analyzers.SeverityMedium, analyzers.CategoryCorrectness))
	// Beta never produced a completed result: a failed scan keeps its counts
	// but must stay out of the dashboard sums, and a running scan is active.
	completedScan(t, s, beta.ID, "failed",
		finding("ruff", "FAIL-H", "src/fail-h.py", "failed high", analyzers.SeverityHigh, analyzers.CategoryCorrectness),
		finding("ruff", "FAIL-H2", "src/fail-h2.py", "failed high 2", analyzers.SeverityHigh, analyzers.CategoryCorrectness),
		finding("ruff", "FAIL-H3", "src/fail-h3.py", "failed high 3", analyzers.SeverityHigh, analyzers.CategoryCorrectness),
		finding("ruff", "FAIL-H4", "src/fail-h4.py", "failed high 4", analyzers.SeverityHigh, analyzers.CategoryCorrectness))
	if _, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: beta.ID, State: "running"}); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var body recentScansResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
		t.Fatalf("recent scans: %d %s", response.Code, response.Body.String())
	}
	summary := body.Summary
	if summary.WorkspacesTotal != 3 || summary.WorkspacesScanned != 1 {
		t.Fatalf("workspace counts: %#v", summary)
	}
	if summary.CriticalCount != 2 || summary.HighCount != 0 || summary.MediumCount != 1 || summary.LowCount != 0 || summary.InfoCount != 0 || summary.TotalFindings != 3 {
		t.Fatalf("severity sums must come from each workspace's latest completed scan only: %#v", summary)
	}
	if summary.ScansTotal != 5 || summary.ScansLast7d != 5 || summary.ActiveScans != 1 {
		t.Fatalf("scan counters: %#v", summary)
	}
}

func completedScan(t *testing.T, s *Server, workspaceID, state string, findings ...analyzers.Finding) core.Scan {
	t.Helper()
	scan, err := s.db.CreateScan(context.Background(), core.Scan{WorkspaceID: workspaceID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.SaveAnalyzerResult(context.Background(), scan.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, findings, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(context.Background(), scan.ID, state, ""); err != nil {
		t.Fatal(err)
	}
	return scan
}

func finding(analyzer, rule, path, message string, severity analyzers.Severity, category analyzers.Category) analyzers.Finding {
	f := analyzers.Finding{AnalyzerID: analyzer, RuleID: rule, RelativePath: path, Message: message, Severity: severity, Category: category}
	f.SetFingerprint()
	return f
}
func TestRejectsNonLoopbackHost(t *testing.T) {
	s := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "http://example.com/api/v1/health", nil)
	request.Host = "example.com"
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != 421 {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
}
func TestRejectsCrossOriginPost(t *testing.T) {
	s := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/workspaces", strings.NewReader(`{"root_path":"."}`))
	request.Header.Set("Origin", "https://example.com")
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("got %d", response.Code)
	}
}
func TestRejectsTraversalInTree(t *testing.T) {
	s := testServer(t)
	root := t.TempDir()
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/workspaces", strings.NewReader(`{"root_path":`+quote(root)+`}`))
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != 201 {
		t.Fatal(response.Body.String())
	}
	body := response.Body.String()
	id := extractID(body)
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces/"+id+"/tree?path=..%2Foutside", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != 400 {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
}

func TestReportEndpointReturnsStructuredReport(t *testing.T) {
	s := testServer(t)
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := s.db.CreateScan(context.Background(), core.Scan{WorkspaceID: work.ID, State: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scan.ID+"/report", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"scan"`) || !strings.Contains(response.Body.String(), `"comparison"`) {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
}

func TestMarkdownReportIsRegeneratedFromScanData(t *testing.T) {
	s := testServer(t)
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := s.db.CreateScan(context.Background(), core.Scan{WorkspaceID: work.ID, State: "completed", Profile: "standard"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "legacy.md")
	if err := os.WriteFile(path, []byte("old export"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(context.Background(), scan.ID, "completed", path); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scan.ID+"/report.md", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "## Quality Metrics Summary") || strings.Contains(response.Body.String(), "old export") {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
}

func TestSARIFReportDownloadsAsAttachment(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "completed", Profile: "standard",
		Snapshot: &core.ScanSnapshot{BluntCodeVersion: "9.9.9", CapturedAt: time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	f := finding("ruff", "F401", "src/main.py", "unused import", analyzers.SeverityHigh, analyzers.CategoryCorrectness)
	f.StartLine, f.StartColumn, f.EndLine, f.EndColumn = 3, 1, 3, 20
	if _, err := s.db.SaveAnalyzerResult(ctx, scan.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{f}, nil); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scan.ID+"/report.sarif", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	disposition := response.Header().Get("Content-Disposition")
	if response.Code != http.StatusOK || disposition != `attachment; filename="bluntcode-scan-`+scan.ID[:8]+`.sarif"` {
		t.Fatalf("got %d disposition %q: %s", response.Code, disposition, response.Body.String())
	}
	var body struct {
		Schema  string `json:"$schema"`
		Version string `json:"version"`
		Runs    []struct {
			Tool struct {
				Driver struct {
					Name    string `json:"name"`
					Version string `json:"version"`
					Rules   []struct {
						ID string `json:"id"`
					} `json:"rules"`
				} `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID string `json:"ruleId"`
				Level  string `json:"level"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid SARIF: %v: %s", err, response.Body.String())
	}
	if body.Schema != "https://json.schemastore.org/sarif-2.1.0.json" || body.Version != "2.1.0" || len(body.Runs) != 1 {
		t.Fatalf("unexpected SARIF header: %#v", body)
	}
	run := body.Runs[0]
	if run.Tool.Driver.Name != "Blunt Code" || run.Tool.Driver.Version != "9.9.9" || len(run.Tool.Driver.Rules) != 1 || run.Tool.Driver.Rules[0].ID != "F401" {
		t.Fatalf("unexpected driver: %#v", run.Tool.Driver)
	}
	if len(run.Results) != 1 || run.Results[0].RuleID != "F401" || run.Results[0].Level != "error" {
		t.Fatalf("unexpected results: %#v", run.Results)
	}
}

func TestSARIFReportRejectsUnknownScan(t *testing.T) {
	s := testServer(t)
	unknown := "00000000-0000-4000-8000-000000000000"
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+unknown+"/report.sarif", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "SCAN_NOT_FOUND") {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/not-a-uuid/report.sarif", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "SCAN_NOT_FOUND") {
		t.Fatalf("malformed id got %d: %s", response.Code, response.Body.String())
	}
}

func TestHTMLReportDownloadsAsAttachment(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "completed", Profile: "standard",
		Snapshot: &core.ScanSnapshot{BluntCodeVersion: "9.9.9", CapturedAt: time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	f := finding("ruff", "F401", "src/main.py", "unused import", analyzers.SeverityHigh, analyzers.CategoryCorrectness)
	if _, err := s.db.SaveAnalyzerResult(ctx, scan.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{f}, nil); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scan.ID+"/report.html", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	disposition := response.Header().Get("Content-Disposition")
	if response.Code != http.StatusOK || disposition != `attachment; filename="bluntcode-scan-`+scan.ID[:8]+`.html"` {
		t.Fatalf("got %d disposition %q: %s", response.Code, disposition, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("content type %q: %s", contentType, response.Body.String())
	}
	body := response.Body.String()
	if !strings.HasPrefix(body, "<!DOCTYPE html>") || !strings.Contains(body, "</html>") {
		t.Fatalf("body must be a complete HTML document: %s", body)
	}
	if !strings.Contains(body, "unused import") || !strings.Contains(body, "Example") {
		t.Fatalf("body must carry the regenerated scan data: %s", body)
	}
}

func TestHTMLReportRejectsUnknownScan(t *testing.T) {
	s := testServer(t)
	unknown := "00000000-0000-4000-8000-000000000000"
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+unknown+"/report.html", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "SCAN_NOT_FOUND") {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/not-a-uuid/report.html", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "SCAN_NOT_FOUND") {
		t.Fatalf("malformed id got %d: %s", response.Code, response.Body.String())
	}
}

func TestSettingsPersistOfflineMode(t *testing.T) {
	s := testServer(t)
	request := httptest.NewRequest(http.MethodPatch, "http://127.0.0.1/api/v1/settings", strings.NewReader(`{"offline":true,"open_browser":false}`))
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"offline":true`) || !strings.Contains(response.Body.String(), `"open_browser":false`) {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/settings", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"offline":true`) || !strings.Contains(response.Body.String(), `"open_browser":false`) {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
}

func TestStopServerRequestsGracefulShutdown(t *testing.T) {
	s := testServer(t)
	stopped := make(chan struct{}, 1)
	s.SetShutdown(func() { stopped <- struct{}{} })
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/system/stop", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"stopping"`) {
		t.Fatalf("stop response: %d %s", response.Code, response.Body.String())
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("shutdown callback was not called")
	}
}

func TestRejectsUnknownScanProfile(t *testing.T) {
	s := testServer(t)
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/workspaces/"+work.ID+"/scans", strings.NewReader(`{"profile":"turbo"}`))
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_PROFILE") {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
}

func TestPathOverridesDriveTreeSelection(t *testing.T) {
	s := testServer(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"drop.py", "keep.py"} {
		if err := os.WriteFile(filepath.Join(root, "src", name), []byte("x=1"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: root, Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, "http://127.0.0.1/api/v1/workspaces/"+work.ID+"/path-overrides", strings.NewReader(`{"overrides":[{"relative_path":"src","mode":"exclude"},{"relative_path":"src/keep.py","mode":"include"}]}`))
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("save overrides: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces/"+work.ID+"/tree?path=src", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"path":"src/drop.py","name":"drop.py","type":"file","included":false`) || !strings.Contains(response.Body.String(), `"path":"src/keep.py","name":"keep.py","type":"file","included":true`) {
		t.Fatalf("tree selections: %d %s", response.Code, response.Body.String())
	}
}
func quote(s string) string { return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"` }
func extractID(body string) string {
	start := strings.Index(body, `"id":"`) + 6
	end := strings.Index(body[start:], `"`)
	return body[start : start+end]
}

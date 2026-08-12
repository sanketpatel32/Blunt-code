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

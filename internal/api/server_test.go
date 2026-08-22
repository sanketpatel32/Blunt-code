package api

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
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

// csvRows fetches findings.csv and decodes it with encoding/csv so hostile
// field content is asserted per CSV rules, never by string matching.
func csvRows(t *testing.T, s *Server, scanID, query string) [][]string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scanID+"/findings.csv"+query, nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("findings.csv%s: %d %s", query, response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/csv; charset=utf-8" {
		t.Fatalf("content type %q: %s", contentType, response.Body.String())
	}
	disposition := response.Header().Get("Content-Disposition")
	if disposition != `attachment; filename="bluntcode-scan-`+scanID[:8]+`-findings.csv"` {
		t.Fatalf("disposition %q", disposition)
	}
	body := response.Body.Bytes()
	if !bytes.HasPrefix(body, []byte(csvBOM)) {
		t.Fatalf("CSV must start with a UTF-8 BOM for Excel: %q", body)
	}
	records, err := csv.NewReader(bytes.NewReader(body[len(csvBOM):])).ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v: %q", err, body)
	}
	wantHeader := "severity,category,analyzer,rule_id,title,message,file,line,column,end_line,status,remediation,documentation_url"
	if len(records) == 0 || strings.Join(records[0], ",") != wantHeader {
		t.Fatalf("unexpected header: %#v", records)
	}
	for _, row := range records {
		if len(row) != 13 {
			t.Fatalf("every row must carry 13 fields: %#v", row)
		}
	}
	return records
}

func csvRowByRule(records [][]string, rule string) []string {
	for _, row := range records[1:] {
		if row[3] == rule {
			return row
		}
	}
	return nil
}

func TestFindingsCSVDownloadsAttachmentWithFiltersAndEscaping(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	hostileMessage := "comma, quote \" and\nnewline with ünicode ✓"
	persistent := finding("ruff", "PERSIST", "src/persistent.py", hostileMessage, analyzers.SeverityHigh, analyzers.CategorySecurity)
	persistent.StartLine, persistent.StartColumn, persistent.EndLine = 12, 3, 14
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
	items := []analyzers.Finding{persistent,
		finding("biome", "lint/a", "src/a.ts", "alpha issue", analyzers.SeverityHigh, analyzers.CategoryCorrectness),
		finding("biome", "lint/b", "src/b.ts", "beta issue", analyzers.SeverityMedium, analyzers.CategoryStyle),
		finding("biome", "lint/c", "src/c.ts", "gamma issue", analyzers.SeverityMedium, analyzers.CategoryStyle)}
	if _, err := s.db.SaveAnalyzerResult(ctx, current.ID, database.AnalyzerRunInput{AnalyzerID: "biome", Version: "test", State: "succeeded"}, items, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(ctx, current.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}

	records := csvRows(t, s, current.ID, "")
	if len(records) != 5 {
		t.Fatalf("header plus every matching finding: %#v", records)
	}
	if row := csvRowByRule(records, "lint/a"); row == nil || row[0] != "high" || row[2] != "biome" || row[6] != "src/a.ts" || row[7] != "" || row[8] != "" || row[9] != "" || row[10] != "new" {
		t.Fatalf("finding without a location must export empty position cells and status new: %#v", row)
	}
	hostile := csvRowByRule(records, "PERSIST")
	if hostile == nil || hostile[5] != hostileMessage || hostile[0] != "high" || hostile[1] != "security" || hostile[2] != "ruff" || hostile[6] != "src/persistent.py" || hostile[7] != "12" || hostile[8] != "3" || hostile[9] != "14" || hostile[10] != "persistent" || hostile[11] != "" || hostile[12] != "" {
		t.Fatalf("hostile content must round-trip per CSV rules: %#v", hostile)
	}

	filtered := csvRows(t, s, current.ID, "?severity=high")
	if len(filtered) != 3 || csvRowByRule(filtered, "lint/b") != nil || csvRowByRule(filtered, "lint/c") != nil {
		t.Fatalf("severity filter must keep only high rows: %#v", filtered)
	}
	for _, row := range filtered[1:] {
		if row[0] != "high" {
			t.Fatalf("severity filter leaked other severities: %#v", row)
		}
	}

	ignored := csvRows(t, s, current.ID, "?severity=high&limit=9999&offset=9999")
	if len(ignored) != len(filtered) {
		t.Fatalf("limit and offset must be ignored by the export: %d vs %d rows", len(ignored)-1, len(filtered)-1)
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+current.ID+"/findings.csv?severity=bogus", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_FINDING_QUERY") {
		t.Fatalf("shared filter validation must reject bad input: %d %s", response.Code, response.Body.String())
	}
}

func TestFindingsCSVRejectsUnknownScan(t *testing.T) {
	s := testServer(t)
	unknown := "00000000-0000-4000-8000-000000000000"
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+unknown+"/findings.csv", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "SCAN_NOT_FOUND") {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/not-a-uuid/findings.csv", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "SCAN_NOT_FOUND") {
		t.Fatalf("malformed id got %d: %s", response.Code, response.Body.String())
	}
}

// TestFindingsCSVNeutralizesFormulaInjection is the regression test for CSV
// formula injection (CWE-1236). Every analyzer-derived text column must be
// prefixed with a quote when it starts with a character Excel treats as a
// formula introducer, so a hostile scanned repository cannot achieve code
// execution through the export. Benign cells must round-trip untouched.
func TestFindingsCSVNeutralizesFormulaInjection(t *testing.T) {
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
	hostile := finding("ruff", "=RULE-EVIL", "=weird\\path.py", "=cmd|'/c calc'!A1", analyzers.SeverityHigh, analyzers.CategorySecurity)
	hostile.Title = "+HYPERLINK(\"http://evil.example\",\"click me\")"
	hostile.Remediation = "-2+cmd|'/C calc'!A0"
	hostile.DocumentationURL = "@SUM(1+1)*cmd|'/C calc'!A0"
	tab := finding("ruff", "TAB", "src/tab.py", "\t=cmd|'/c calc'!A1", analyzers.SeverityMedium, analyzers.CategoryStyle)
	carriage := finding("ruff", "CR", "src/cr.py", "\r=cmd|'/c calc'!A1", analyzers.SeverityLow, analyzers.CategoryStyle)
	benign := finding("ruff", "PLAIN", "src/plain.py", "plain, \"quoted\" text", analyzers.SeverityInfo, analyzers.CategoryCorrectness)
	if _, err := s.db.SaveAnalyzerResult(ctx, scan.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{hostile, tab, carriage, benign}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(ctx, scan.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}

	records := csvRows(t, s, scan.ID, "")
	if len(records) != 5 {
		t.Fatalf("header plus every finding: %#v", records)
	}
	row := csvRowByRule(records, "'=RULE-EVIL")
	if row == nil {
		t.Fatalf("hostile rule id must be neutralized and findable: %#v", records)
	}
	wantNeutralized := map[int]string{
		3:  "'=RULE-EVIL",
		4:  "'+HYPERLINK(\"http://evil.example\",\"click me\")",
		5:  "'=cmd|'/c calc'!A1",
		6:  "'=weird\\path.py",
		11: "'-2+cmd|'/C calc'!A0",
		12: "'@SUM(1+1)*cmd|'/C calc'!A0",
	}
	for index, want := range wantNeutralized {
		if row[index] != want {
			t.Fatalf("column %d: got %q, want %q (full row %#v)", index, row[index], want, row)
		}
	}
	if tabbed := csvRowByRule(records, "TAB"); tabbed == nil || tabbed[5] != "'\t=cmd|'/c calc'!A1" {
		t.Fatalf("tab-prefixed message must be neutralized: %#v", tabbed)
	}
	if returned := csvRowByRule(records, "CR"); returned == nil || returned[5] != "'\r=cmd|'/c calc'!A1" {
		t.Fatalf("carriage-return-prefixed message must be neutralized: %#v", returned)
	}
	if plain := csvRowByRule(records, "PLAIN"); plain == nil || plain[5] != "plain, \"quoted\" text" || plain[3] != "PLAIN" {
		t.Fatalf("benign cells must round-trip untouched: %#v", plain)
	}
	for _, record := range records[1:] {
		for _, cell := range record {
			if cell == "" {
				continue
			}
			switch cell[0] {
			case '=', '+', '@':
				t.Fatalf("unneutralized formula cell %q leaked into the export: %#v", cell, record)
			}
		}
	}
}

type fixedFindingsResponse struct {
	Fixed               []analyzers.Finding `json:"fixed"`
	TotalFixed          int                 `json:"total_fixed"`
	ComparisonAvailable bool                `json:"comparison_available"`
	PreviousScanID      *string             `json:"previous_scan_id"`
}

func TestFixedFindingsListsCoverageAwareFixedRows(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	// STAY persists, GONE-HIGH and GONE-LOW disappear while ruff still
	// succeeds, and SEM-UNKNOWN disappears while semgrep fails this scan, so
	// the coverage rule must keep it out of the fixed list.
	stay := finding("ruff", "STAY", "src/stay.py", "still here", analyzers.SeverityCritical, analyzers.CategoryCorrectness)
	goneHigh := finding("ruff", "GONE-HIGH", "src/gone-high.py", "fixed high", analyzers.SeverityHigh, analyzers.CategoryCorrectness)
	goneLow := finding("ruff", "GONE-LOW", "src/gone-low.py", "fixed low", analyzers.SeverityLow, analyzers.CategoryCorrectness)
	covered := finding("semgrep", "SEM-UNKNOWN", "src/covered.py", "analyzer failed this scan", analyzers.SeverityCritical, analyzers.CategorySecurity)
	previous, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.SaveAnalyzerResult(ctx, previous.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{stay, goneHigh, goneLow}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.SaveAnalyzerResult(ctx, previous.ID, database.AnalyzerRunInput{AnalyzerID: "semgrep", Version: "test", State: "succeeded"}, []analyzers.Finding{covered}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(ctx, previous.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	current, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.SaveAnalyzerResult(ctx, current.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{stay}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.SaveAnalyzerResult(ctx, current.ID, database.AnalyzerRunInput{AnalyzerID: "semgrep", Version: "test", State: "failed", Error: "tool crashed"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(ctx, current.ID, "completed_with_warnings", ""); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+current.ID+"/fixed", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var body fixedFindingsResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
		t.Fatalf("fixed: %d %s", response.Code, response.Body.String())
	}
	if !body.ComparisonAvailable || body.PreviousScanID == nil || *body.PreviousScanID != previous.ID {
		t.Fatalf("comparison context: %#v", body)
	}
	if body.TotalFixed != 2 || len(body.Fixed) != 2 {
		t.Fatalf("only the two ruff findings are fixed: %#v", body)
	}
	if body.Fixed[0].RuleID != "GONE-HIGH" || body.Fixed[0].Status != "fixed" || body.Fixed[1].RuleID != "GONE-LOW" || body.Fixed[1].Status != "fixed" {
		t.Fatalf("severity order and fixed status: %#v", body.Fixed)
	}

	// The capped list never changes the reported total.
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+current.ID+"/fixed?limit=1", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.TotalFixed != 2 || len(body.Fixed) != 1 {
		t.Fatalf("capped list: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+current.ID+"/fixed?limit=0", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_FINDING_QUERY") {
		t.Fatalf("invalid limit: %d %s", response.Code, response.Body.String())
	}

	// First scan of a fresh workspace: no baseline, so nothing can be fixed.
	solo, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Solo"})
	if err != nil {
		t.Fatal(err)
	}
	first := completedScan(t, s, solo.ID, "completed", finding("ruff", "ONLY", "src/only.py", "no baseline", analyzers.SeverityLow, analyzers.CategoryCorrectness))
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+first.ID+"/fixed", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.ComparisonAvailable || body.TotalFixed != 0 || len(body.Fixed) != 0 || body.PreviousScanID != nil {
		t.Fatalf("no baseline: %d %s", response.Code, response.Body.String())
	}

	unknown := "00000000-0000-4000-8000-000000000000"
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+unknown+"/fixed", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "SCAN_NOT_FOUND") {
		t.Fatalf("unknown scan got %d: %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/not-a-uuid/fixed", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "SCAN_NOT_FOUND") {
		t.Fatalf("malformed id got %d: %s", response.Code, response.Body.String())
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

// TestFindingPreviewHandlesDeletedAndMissingPaths locks the behavior for rows
// whose source no longer exists (the fixed-findings panel can surface paths
// from a previous scan): a missing or deleted file must return a 4xx error,
// never a 500 or a read outside the workspace.
func TestFindingPreviewHandlesDeletedAndMissingPaths(t *testing.T) {
	s := testServer(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "gone.py"), []byte("x = 1\n"), 0o600); err != nil {
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
	items := []analyzers.Finding{
		finding("ruff", "DELETED", "gone.py", "was here", analyzers.SeverityMedium, analyzers.CategoryCorrectness),
		finding("ruff", "NEVER", "never-existed.py", "never here", analyzers.SeverityMedium, analyzers.CategoryCorrectness),
	}
	if _, err := s.db.SaveAnalyzerResult(context.Background(), scan.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, items, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "gone.py")); err != nil {
		t.Fatal(err)
	}
	page, err := s.db.FindingsPage(context.Background(), scan, database.FindingFilter{Limit: 25})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("saved findings: %#v, err %v", page, err)
	}
	for _, item := range page.Items {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scan.ID+"/findings/"+item.ID+"/preview", nil)
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity && response.Code != http.StatusNotFound {
			t.Fatalf("missing file %s: got %d (want 4xx, never 500): %s", item.RelativePath, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "DATABASE_ERROR") {
			t.Fatalf("missing file %s must not surface a database error: %s", item.RelativePath, response.Body.String())
		}
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

func postOpenFolder(t *testing.T, s *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/system/open-folder", strings.NewReader(body))
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	return response
}

func TestOpenFolderMapsEachKindToConfiguredPath(t *testing.T) {
	s := testServer(t)
	// The reports folder only exists once a scan has written a report; the
	// scan service creates it on demand in production, so the test does too.
	if err := os.Mkdir(s.paths.ReportsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	var opened []string
	s.openFolder = func(dir string) error { opened = append(opened, dir); return nil }
	for _, kind := range []string{"data", "reports", "logs", "tools"} {
		response := postOpenFolder(t, s, `{"kind":"`+kind+`"}`)
		if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
			t.Fatalf("%s: got %d %q", kind, response.Code, response.Body.String())
		}
	}
	want := []string{s.paths.DataDir, s.paths.ReportsDir, s.paths.LogsDir, s.paths.ToolsDir}
	if len(opened) != len(want) {
		t.Fatalf("launcher ran %d times: %#v", len(opened), opened)
	}
	for i := range want {
		if opened[i] != want[i] {
			t.Fatalf("kind %d opened %q, want %q", i, opened[i], want[i])
		}
	}
}

func TestOpenFolderRejectsUnknownKind(t *testing.T) {
	s := testServer(t)
	launched := false
	s.openFolder = func(dir string) error { launched = true; return nil }
	// The raw-path attempt matters most: DisallowUnknownFields must reject any
	// smuggled path key so only the server-side enum can pick a target.
	for _, body := range []string{`{"kind":"secrets"}`, `{"kind":""}`, `{}`, ``, `{"kind":"data","path":"C:\\Windows"}`} {
		response := postOpenFolder(t, s, body)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_FOLDER_KIND") {
			t.Fatalf("body %q: got %d %s", body, response.Code, response.Body.String())
		}
	}
	if launched {
		t.Fatal("invalid kinds must never reach the launcher")
	}
}

func TestOpenFolderRequiresExistingDirectory(t *testing.T) {
	s := testServer(t)
	launched := false
	s.openFolder = func(dir string) error { launched = true; return nil }
	// The reports folder is the natural missing case on a fresh install: the
	// scan service creates it only after the first report is written.
	response := postOpenFolder(t, s, `{"kind":"reports"}`)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "FOLDER_NOT_FOUND") {
		t.Fatalf("got %d %s", response.Code, response.Body.String())
	}
	if launched {
		t.Fatal("missing folders must never reach the launcher")
	}
}

func TestOpenFolderReportsLauncherFailure(t *testing.T) {
	s := testServer(t)
	s.openFolder = func(dir string) error { return errors.New("explorer exploded") }
	response := postOpenFolder(t, s, `{"kind":"logs"}`)
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "FOLDER_OPEN_FAILED") || !strings.Contains(response.Body.String(), "Windows Explorer could not open this folder.") {
		t.Fatalf("got %d %s", response.Code, response.Body.String())
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

// assertJSONList fetches target and asserts the response is 200 and contains
// the exact substring want. Raw-body matching is required for null-vs-[]
// checks because encoding/json decodes both into an empty Go slice.
func assertJSONList(t *testing.T, s *Server, target, want string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+target, nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("%s: got %d %s", target, response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), want) {
		t.Fatalf("%s: body must contain %s: %s", target, want, response.Body.String())
	}
}

// TestEmptyListsSerializeAsArraysNotNull pins the API consistency rule that
// every list payload serializes as a JSON array, never null, when empty:
// typed clients decode these responses into slices and must not be forced to
// null-check each one. The regression covers the dashboard feed, per-resource
// lists, and the report's embedded findings, metrics, and warnings lists.
func TestEmptyListsSerializeAsArraysNotNull(t *testing.T) {
	s := testServer(t)
	// A completely empty database must still feed the dashboard arrays.
	assertJSONList(t, s, "/api/v1/scans", `"scans":[]`)
	assertJSONList(t, s, "/api/v1/workspaces", `"items":[]`)

	ctx := context.Background()
	empty, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Empty"})
	if err != nil {
		t.Fatal(err)
	}
	// A workspace that exists but has never been scanned.
	assertJSONList(t, s, "/api/v1/workspaces/"+empty.ID+"/scans", `"items":[]`)
	assertJSONList(t, s, "/api/v1/workspaces/"+empty.ID+"/rules", `"items":[]`)
	assertJSONList(t, s, "/api/v1/workspaces/"+empty.ID+"/path-overrides", `"items":[]`)
	assertJSONList(t, s, "/api/v1/workspaces/"+empty.ID+"/tree", `"items":[]`)

	// A completed scan with no findings: list, fixed panel, and report must
	// all serialize their empty collections as arrays.
	scanned, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Scanned"})
	if err != nil {
		t.Fatal(err)
	}
	scan := completedScan(t, s, scanned.ID, "completed")
	assertJSONList(t, s, "/api/v1/scans/"+scan.ID+"/findings", `"items":[]`)
	assertJSONList(t, s, "/api/v1/scans/"+scan.ID+"/fixed", `"fixed":[]`)
	assertJSONList(t, s, "/api/v1/scans/"+scan.ID+"/report", `"findings":[]`)
	assertJSONList(t, s, "/api/v1/scans/"+scan.ID+"/report", `"metrics":[]`)
	assertJSONList(t, s, "/api/v1/scans/"+scan.ID+"/report", `"warnings":[]`)
}

// TestFindingsAndCSVRejectIdenticalEnumInputs pins the agreement between the
// JSON findings list and the CSV export: both parse their filters through the
// shared findingQueryFilter, so every invalid enum value (severity, status,
// sort, order) must draw the same 400 INVALID_FINDING_QUERY envelope from
// both endpoints. limit and offset deliberately differ: the export ignores
// pagination because it writes every matching row.
func TestFindingsAndCSVRejectIdenticalEnumInputs(t *testing.T) {
	s := testServer(t)
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	scan := completedScan(t, s, work.ID, "completed", finding("ruff", "F401", "src/main.py", "unused import", analyzers.SeverityHigh, analyzers.CategoryCorrectness))
	for _, q := range []string{"?severity=bogus", "?status=bogus", "?sort=bogus", "?order=sideways"} {
		for _, suffix := range []string{"findings", "findings.csv"} {
			request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scan.ID+"/"+suffix+q, nil)
			response := httptest.NewRecorder()
			s.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_FINDING_QUERY") {
				t.Fatalf("%s%s: got %d %s", suffix, q, response.Code, response.Body.String())
			}
		}
	}
}

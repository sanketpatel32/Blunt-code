package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
)

// seedFindingsFilterAPI seeds the same nine-finding scan as the database
// fixture: HIGH persists from a completed previous scan, CRIT is suppressed
// for the workspace, and the paths cover prefix-matching pitfalls.
func seedFindingsFilterAPI(t *testing.T) (*Server, core.Scan) {
	t.Helper()
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	mk := func(analyzer, rule, path, message string, severity analyzers.Severity, line int) analyzers.Finding {
		f := finding(analyzer, rule, path, message, severity, analyzers.CategoryCorrectness)
		f.StartLine = line
		return f
	}
	high := mk("ruff", "HIGH", "src/main.py", "persisting issue", analyzers.SeverityHigh, 10)
	all := []analyzers.Finding{
		mk("ruff", "CRIT", "src/main.py", "critical issue", analyzers.SeverityCritical, 40),
		high,
		mk("ruff", "MED", "src/util/helper.py", "medium issue", analyzers.SeverityMedium, 5),
		mk("biome", "LOW", "src/util/helper.py", "low issue", analyzers.SeverityLow, 30),
		mk("semgrep", "PCT", "100%.py", "percent path issue", analyzers.SeverityLow, 2),
		mk("biome", "INFO", "tests/src/edge.ts", "info issue", analyzers.SeverityInfo, 1),
		mk("ruff", "", "Docs/Readme.MD", "no rule issue", analyzers.SeverityInfo, 0),
		mk("ruff", "UND1", "src/a_b.py", "underscore one", analyzers.SeverityInfo, 7),
		mk("ruff", "UND2", "src/axb.py", "underscore two", analyzers.SeverityInfo, 3),
	}
	previous, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.SaveAnalyzerResult(ctx, previous.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{high}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(ctx, previous.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	scan := completedScan(t, s, work.ID, "completed", all...)
	if _, err := s.db.AddSuppression(ctx, work.ID, all[0].Fingerprint, "wontfix"); err != nil {
		t.Fatal(err)
	}
	return s, scan
}

// findingsPayload decodes the JSON list response including the page-mode
// additions; page fields are pointers so legacy envelopes decode cleanly.
type findingsPayload struct {
	Items      []analyzers.Finding `json:"items"`
	Total      int                 `json:"total"`
	Limit      int                 `json:"limit"`
	Offset     int                 `json:"offset"`
	NextOffset *int                `json:"next_offset"`
	HasMore    bool                `json:"has_more"`
	Page       *int                `json:"page"`
	PageSize   *int                `json:"page_size"`
	HasNext    *bool               `json:"has_next"`
}

func fetchFindings(t *testing.T, s *Server, scanID, query string) findingsPayload {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scanID+"/findings"+query, nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("findings%s: %d %s", query, response.Code, response.Body.String())
	}
	var body findingsPayload
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("findings%s: %v", query, err)
	}
	return body
}

func fetchFindingsStatus(t *testing.T, s *Server, scanID, query string) int {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scanID+"/findings"+query, nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	return response.Code
}

func rulesOfItems(items []analyzers.Finding) []string {
	rules := make([]string, 0, len(items))
	for _, item := range items {
		rules = append(rules, item.RuleID)
	}
	return rules
}

func rulesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFindingsRuleAnalyzerAndPathPrefixParams(t *testing.T) {
	s, scan := seedFindingsFilterAPI(t)

	if body := fetchFindings(t, s, scan.ID, "?rule=high"); body.Total != 1 || !rulesEqual(rulesOfItems(body.Items), []string{"HIGH"}) {
		t.Fatalf("rule must match exactly and case-insensitively: %#v", rulesOfItems(body.Items))
	}
	if body := fetchFindings(t, s, scan.ID, "?rule=HIG"); body.Total != 0 || len(body.Items) != 0 {
		t.Fatalf("rule must not match partially: %#v", rulesOfItems(body.Items))
	}
	if body := fetchFindings(t, s, scan.ID, "?analyzer=biome"); body.Total != 2 || !rulesEqual(rulesOfItems(body.Items), []string{"LOW", "INFO"}) {
		t.Fatalf("analyzer filter: %#v", rulesOfItems(body.Items))
	}

	if body := fetchFindings(t, s, scan.ID, "?path_prefix=src"); body.Total != 6 || !rulesEqual(rulesOfItems(body.Items), []string{"UND1", "UND2", "HIGH", "CRIT", "MED", "LOW"}) {
		t.Fatalf("path_prefix must anchor at the start: %#v", rulesOfItems(body.Items))
	}
	if body := fetchFindings(t, s, scan.ID, "?path_prefix=src%5Cutil"); body.Total != 2 || !rulesEqual(rulesOfItems(body.Items), []string{"MED", "LOW"}) {
		t.Fatalf("backslash path_prefix must normalize: %#v", rulesOfItems(body.Items))
	}
	if body := fetchFindings(t, s, scan.ID, "?path_prefix=./SRC/"); body.Total != 6 {
		t.Fatalf("path_prefix must tolerate case and ./: %#v", rulesOfItems(body.Items))
	}
	if body := fetchFindings(t, s, scan.ID, "?path_prefix=edge"); body.Total != 0 || len(body.Items) != 0 {
		t.Fatalf("path_prefix must not behave as a substring: %#v", rulesOfItems(body.Items))
	}
	if body := fetchFindings(t, s, scan.ID, "?analyzer=ruff&path_prefix=src%2Futil&rule=MED"); body.Total != 1 || body.Items[0].RuleID != "MED" {
		t.Fatalf("combined filters: %#v", rulesOfItems(body.Items))
	}
}

func TestFindingsCommaListParams(t *testing.T) {
	s, scan := seedFindingsFilterAPI(t)

	if body := fetchFindings(t, s, scan.ID, "?severity=high,critical"); body.Total != 2 || !rulesEqual(rulesOfItems(body.Items), []string{"HIGH", "CRIT"}) {
		t.Fatalf("severity comma list: %#v", rulesOfItems(body.Items))
	}
	if body := fetchFindings(t, s, scan.ID, "?severity=high"); body.Total != 1 {
		t.Fatalf("single severity must keep working: %#v", body)
	}
	if body := fetchFindings(t, s, scan.ID, "?severity=CRITICAL"); body.Total != 1 {
		t.Fatalf("severity stays case-insensitive: %#v", body)
	}
	for _, query := range []string{"?severity=high,bogus", "?severity=high,", "?severity=,high", "?status=new,bogus"} {
		if code := fetchFindingsStatus(t, s, scan.ID, query); code != http.StatusBadRequest {
			t.Fatalf("%s must be rejected: %d", query, code)
		}
	}
	// Statuses: CRIT suppressed, HIGH persistent, the remaining seven new.
	if body := fetchFindings(t, s, scan.ID, "?status=new,persistent"); body.Total != 8 {
		t.Fatalf("status comma list: total=%d %#v", body.Total, rulesOfItems(body.Items))
	}
	if body := fetchFindings(t, s, scan.ID, "?status=persistent,suppressed"); body.Total != 2 {
		t.Fatalf("status list with suppressed: total=%d %#v", body.Total, rulesOfItems(body.Items))
	}
}

func TestFindingsSortParams(t *testing.T) {
	s, scan := seedFindingsFilterAPI(t)

	if body := fetchFindings(t, s, scan.ID, "?sort=line"); !rulesEqual(rulesOfItems(body.Items), []string{"", "INFO", "PCT", "UND2", "MED", "UND1", "HIGH", "LOW", "CRIT"}) {
		t.Fatalf("sort=line asc: %#v", rulesOfItems(body.Items))
	}
	if body := fetchFindings(t, s, scan.ID, "?sort=-line"); !rulesEqual(rulesOfItems(body.Items), []string{"CRIT", "LOW", "HIGH", "UND1", "MED", "UND2", "PCT", "INFO", ""}) {
		t.Fatalf("sort=-line (descending prefix): %#v", rulesOfItems(body.Items))
	}
	if body := fetchFindings(t, s, scan.ID, "?sort=rule&order=desc"); body.Items[0].RuleID != "UND2" || body.Items[len(body.Items)-1].RuleID != "" {
		t.Fatalf("sort=rule desc: %#v", rulesOfItems(body.Items))
	}
	// Severity ranks by domain order (critical first descending), never
	// alphabetically (critical/info would swap).
	if body := fetchFindings(t, s, scan.ID, "?sort=-severity"); body.Items[0].Severity != analyzers.SeverityCritical || body.Items[len(body.Items)-1].Severity != analyzers.SeverityInfo {
		t.Fatalf("sort=-severity must rank critical first and info last: %#v", body.Items)
	}
	if body := fetchFindings(t, s, scan.ID, "?sort=severity&order=asc"); body.Items[0].Severity != analyzers.SeverityInfo || body.Items[len(body.Items)-1].Severity != analyzers.SeverityCritical {
		t.Fatalf("sort=severity asc: %#v", body.Items)
	}
	// An explicit order parameter wins over the "-" prefix.
	if body := fetchFindings(t, s, scan.ID, "?sort=-severity&order=asc"); body.Items[0].Severity != analyzers.SeverityInfo {
		t.Fatalf("order=asc must override the descending prefix: %#v", body.Items)
	}
	for _, query := range []string{"?sort=bogus", "?sort=-bogus", "?sort=-", "?sort=line&order=sideways"} {
		if code := fetchFindingsStatus(t, s, scan.ID, query); code != http.StatusBadRequest {
			t.Fatalf("%s must be rejected: %d", query, code)
		}
	}
	// The CSV export shares the parser, so the new sort controls apply there too.
	if code := func() int {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scan.ID+"/findings.csv?sort=-line", nil)
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		return response.Code
	}(); code != http.StatusOK {
		t.Fatalf("findings.csv must accept the new sort controls: %d", code)
	}
}

func TestFindingsPageParams(t *testing.T) {
	s, scan := seedFindingsFilterAPI(t)

	first := fetchFindings(t, s, scan.ID, "?page=1&page_size=5")
	if first.Total != 9 || len(first.Items) != 5 || first.Limit != 5 || first.Offset != 0 ||
		first.Page == nil || *first.Page != 1 || first.PageSize == nil || *first.PageSize != 5 || first.HasNext == nil || !*first.HasNext || !first.HasMore {
		t.Fatalf("page 1 envelope: %#v", first)
	}
	second := fetchFindings(t, s, scan.ID, "?page=2&page_size=5")
	if second.Total != 9 || len(second.Items) != 4 || second.HasNext == nil || *second.HasNext || second.HasMore {
		t.Fatalf("page 2 holds the remainder: %#v", second)
	}
	beyond := fetchFindings(t, s, scan.ID, "?page=3&page_size=5")
	if beyond.Total != 9 || len(beyond.Items) != 0 || beyond.HasNext == nil || *beyond.HasNext {
		t.Fatalf("page beyond the end is empty without a next page: %#v", beyond)
	}
	// page alone defaults to a 50-row window; page_size alone starts at page 1.
	if body := fetchFindings(t, s, scan.ID, "?page=1"); len(body.Items) != 9 || body.PageSize == nil || *body.PageSize != database.DefaultFindingsPageSize {
		t.Fatalf("page default size: %#v", body)
	}
	if body := fetchFindings(t, s, scan.ID, "?page_size=200"); len(body.Items) != 9 || body.Page == nil || *body.Page != 1 || body.Limit != 200 {
		t.Fatalf("page_size alone: %#v", body)
	}
	// Filters compose with pagination and keep the total exact.
	if body := fetchFindings(t, s, scan.ID, "?page=2&page_size=2&severity=info"); body.Total != 4 || len(body.Items) != 2 {
		t.Fatalf("filtered pagination: %#v", body)
	}
	for _, query := range []string{
		"?page=0", "?page=-1", "?page=abc", "?page=999999999999999999999",
		"?page_size=0", "?page_size=201", "?page_size=-5", "?page_size=abc",
		"?page=1&limit=25", "?page=1&offset=0", "?page_size=5&limit=25", "?page_size=5&offset=10",
	} {
		if code := fetchFindingsStatus(t, s, scan.ID, query); code != http.StatusBadRequest {
			t.Fatalf("%s must be rejected: %d", query, code)
		}
	}
	// Legacy pagination keeps working beside the new mode.
	legacy := fetchFindings(t, s, scan.ID, "?limit=25&offset=5")
	if legacy.Page != nil || legacy.PageSize != nil || legacy.HasNext != nil || len(legacy.Items) != 4 || legacy.Limit != 25 || legacy.Offset != 5 {
		t.Fatalf("legacy pagination: %#v", legacy)
	}
}

// TestFindingsLegacyEnvelopeUnchanged pins the backward-compatibility
// contract: with no page/page_size parameters the response carries exactly the
// original six envelope fields (no page metadata), the default limit stays 50,
// and the default ordering stays relative_path then start_line ascending.
func TestFindingsLegacyEnvelopeUnchanged(t *testing.T) {
	s, scan := seedFindingsFilterAPI(t)

	get := func(query string) (map[string]json.RawMessage, []byte) {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scan.ID+"/findings"+query, nil)
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("findings%s: %d %s", query, response.Code, response.Body.String())
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(response.Body.Bytes(), &raw); err != nil {
			t.Fatalf("findings%s: %v", query, err)
		}
		return raw, response.Body.Bytes()
	}

	raw, _ := get("")
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	want := []string{"has_more", "items", "limit", "next_offset", "offset", "total"}
	if len(keys) != len(want) {
		t.Fatalf("legacy envelope must carry exactly the original keys, got %v", keys)
	}
	for i := range keys {
		if keys[i] != want[i] {
			t.Fatalf("legacy envelope keys = %v, want %v", keys, want)
		}
	}
	var body findingsPayload
	if err := json.Unmarshal(raw["limit"], &body.Limit); err != nil {
		t.Fatal(err)
	}
	if body.Limit != 50 {
		t.Fatalf("default limit must stay 50, got %d", body.Limit)
	}
	full := fetchFindings(t, s, scan.ID, "")
	if full.Total != 9 || len(full.Items) != 9 || full.HasMore {
		t.Fatalf("default page must hold every row: %#v", full)
	}
	// Default ordering is the stored order: relative_path, then start_line.
	wantOrder := []string{"PCT", "", "UND1", "UND2", "HIGH", "CRIT", "MED", "LOW", "INFO"}
	if !rulesEqual(rulesOfItems(full.Items), wantOrder) {
		t.Fatalf("default ordering changed: %#v", rulesOfItems(full.Items))
	}
	// The same request through the legacy limit/offset controls keeps the
	// original envelope too.
	rawLegacy, _ := get("?limit=25&offset=5&sort=path&order=asc")
	if len(rawLegacy) != len(want) {
		t.Fatalf("legacy params must not add page metadata, got %d keys", len(rawLegacy))
	}
}

package api

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bluntcode/internal/core"
)

// suppressionCSV fetches suppressions.csv and decodes it with encoding/csv,
// asserting the BOM, content type, disposition, and fixed header shape.
func fetchSuppressionsCSV(t *testing.T, s *Server, workspaceID string) [][]string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces/"+workspaceID+"/suppressions.csv", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("suppressions.csv: %d %s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/csv; charset=utf-8" {
		t.Fatalf("content type %q", contentType)
	}
	disposition := response.Header().Get("Content-Disposition")
	if disposition != `attachment; filename="bluntcode-workspace-`+workspaceID[:8]+`-suppressions.csv"` {
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
	if len(records) == 0 || strings.Join(records[0], ",") != "fingerprint,reason,created_at" {
		t.Fatalf("unexpected header: %#v", records)
	}
	return records
}

func postImportSuppressions(t *testing.T, s *Server, workspaceID, csvText string) (int, map[string]int) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"csv": csvText})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/workspaces/"+workspaceID+"/suppressions/import", bytes.NewReader(payload))
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("import: %d %s", response.Code, response.Body.String())
	}
	var body map[string]int
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid import payload: %v: %s", err, response.Body.String())
	}
	return response.Code, body
}

// TestSuppressionsCSVRoundTripsIntoAnotherWorkspace pins the export/import
// pair: the export writes fingerprint,reason,created_at with RFC3339 stamps
// and formula-neutralized reason cells, and importing that exact text into a
// second workspace reproduces every fingerprint.
func TestSuppressionsCSVRoundTripsIntoAnotherWorkspace(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	source, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Source"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Target"})
	if err != nil {
		t.Fatal(err)
	}
	hostileReason := "=cmd|'/c calc'!A1, comma \"quoted\""
	if _, err := s.db.AddSuppression(ctx, source.ID, strings.Repeat("ab", 32), hostileReason); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.AddSuppression(ctx, source.ID, strings.Repeat("cd", 32), ""); err != nil {
		t.Fatal(err)
	}

	records := fetchSuppressionsCSV(t, s, source.ID)
	if len(records) != 3 {
		t.Fatalf("header plus two rows expected: %#v", records)
	}
	byFingerprint := map[string][]string{}
	for _, row := range records[1:] {
		if len(row) != 3 {
			t.Fatalf("every row carries three columns: %#v", row)
		}
		byFingerprint[row[0]] = row
	}
	hostileRow := byFingerprint[strings.Repeat("ab", 32)]
	if hostileRow == nil {
		t.Fatalf("first fingerprint missing: %#v", records)
	}
	if hostileRow[1] != "'"+hostileReason {
		t.Fatalf("formula-prefixed reason must be neutralized for spreadsheets: %q", hostileRow[1])
	}
	if _, err := time.Parse(time.RFC3339, hostileRow[2]); err != nil {
		t.Fatalf("created_at must be RFC3339: %q (%v)", hostileRow[2], err)
	}

	var csvText string
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces/"+source.ID+"/suppressions.csv", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	csvText = string(raw[len(csvBOM):])

	code, body := postImportSuppressions(t, s, target.ID, csvText)
	if code != http.StatusOK || body["imported"] != 2 || body["skipped_invalid"] != 0 || body["duplicate"] != 0 {
		t.Fatalf("round-trip import: %d %#v", code, body)
	}
	imported, err := s.db.Suppressions(ctx, target.ID)
	if err != nil || len(imported) != 2 {
		t.Fatalf("target workspace must hold both fingerprints: %#v err %v", imported, err)
	}
	seen := map[string]bool{}
	for _, item := range imported {
		seen[item.Fingerprint] = true
	}
	if !seen[strings.Repeat("ab", 32)] || !seen[strings.Repeat("cd", 32)] {
		t.Fatalf("fingerprints did not survive the round trip: %#v", imported)
	}

	// Re-importing the same text reports duplicates instead of rewriting rows.
	_, body = postImportSuppressions(t, s, target.ID, csvText)
	if body["imported"] != 0 || body["duplicate"] != 2 || body["skipped_invalid"] != 0 {
		t.Fatalf("second import must dedupe against existing suppressions: %#v", body)
	}
}

// TestSuppressionsImportCountsInvalidAndDuplicateRows pins the per-row
// accounting: uppercase fingerprints normalize to lowercase, invalid ones are
// skipped without failing the request, and repeats within one upload count
// as duplicates.
func TestSuppressionsImportCountsInvalidAndDuplicateRows(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	validLower := strings.Repeat("12", 32)
	csvText := "fingerprint,reason,created_at\n" +
		strings.ToUpper(strings.Repeat("ef", 32)) + ",uppercase normalizes,\n" + // imported
		validLower + ",imported once,\n" +
		validLower + ",repeated in-file -> duplicate,\n" +
		"short-fp,bad length,\n" +
		strings.Repeat("g", 64) + ",not hex,\n" +
		",empty cell,\n"
	code, body := postImportSuppressions(t, s, work.ID, csvText)
	if code != http.StatusOK {
		t.Fatalf("import rejected: %d %v", code, body)
	}
	if body["imported"] != 2 || body["skipped_invalid"] != 3 || body["duplicate"] != 1 {
		t.Fatalf("row accounting = %#v", body)
	}
	items, err := s.db.Suppressions(ctx, work.ID)
	if err != nil || len(items) != 2 {
		t.Fatalf("only valid rows may persist: %#v err %v", items, err)
	}
	for _, item := range items {
		if item.Fingerprint != strings.ToLower(item.Fingerprint) {
			t.Fatalf("fingerprint must normalize to lowercase: %q", item.Fingerprint)
		}
	}
}

func TestSuppressionsCSVAndImportRejectBadInput(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	missing := "11111111-1111-1111-1111-111111111111"

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces/"+missing+"/suppressions.csv", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown workspace export: %d %s", response.Code, response.Body.String())
	}

	post := func(workspaceID, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/workspaces/"+workspaceID+"/suppressions/import", strings.NewReader(body))
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		return response
	}
	if response := post(missing, `{"csv":"fingerprint\n"}`); response.Code != http.StatusNotFound {
		t.Fatalf("unknown workspace import: %d %s", response.Code, response.Body.String())
	}
	for name, body := range map[string]string{
		"missing csv field": `{}`,
		"empty csv":         `{"csv":""}`,
		"malformed json":    `{"csv":`,
	} {
		if response := post(work.ID, body); response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_CSV") {
			t.Fatalf("%s must be rejected with INVALID_CSV: %d %s", name, response.Code, response.Body.String())
		}
	}
	if response := post(work.ID, `{"csv":"name,note\nabc,def"}`); response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_CSV") {
		t.Fatalf("missing fingerprint column must be rejected: %d %s", response.Code, response.Body.String())
	}
	// A bare quote inside an unquoted field is rejected by encoding/csv.
	if response := post(work.ID, `{"csv":"fingerprint\nab\"c"}`); response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "INVALID_CSV") {
		t.Fatalf("unparseable CSV must be rejected: %d %s", response.Code, response.Body.String())
	}
	// A header-only upload is valid and imports nothing.
	if response := post(work.ID, `{"csv":"fingerprint"}`); response.Code != http.StatusOK {
		t.Fatalf("header-only import: %d %s", response.Code, response.Body.String())
	}
}

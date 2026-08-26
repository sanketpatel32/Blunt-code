package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
)

// TestFindingsJSONLDownloadsAttachment pins the newline-delimited JSON
// export: one self-contained JSON object per finding, exactly as many lines
// as findings (the trailing LF terminates the last record rather than adding
// an empty one), served as a text/plain attachment like its csv/json
// siblings, regenerated from stored scan data through the report model.
func TestFindingsJSONLDownloadsAttachment(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	scan := completedScan(t, s, work.ID, "completed",
		finding("ruff", "F401", "src/main.py", "unused import", analyzers.SeverityHigh, analyzers.CategoryCorrectness),
		finding("biome", "lint/a", "src/a.ts", "alpha issue", analyzers.SeverityMedium, analyzers.CategoryStyle),
		finding("biome", "lint/b", "src/b.ts", "beta issue", analyzers.SeverityLow, analyzers.CategoryStyle))

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scan.ID+"/findings.jsonl", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("findings.jsonl: %d %s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/plain; charset=utf-8" {
		t.Fatalf("content type %q", contentType)
	}
	disposition := response.Header().Get("Content-Disposition")
	if disposition != `attachment; filename="bluntcode-scan-`+scan.ID[:8]+`-findings.jsonl"` {
		t.Fatalf("disposition %q", disposition)
	}

	body := response.Body.Bytes()
	if len(body) == 0 || body[len(body)-1] != '\n' {
		t.Fatalf("every JSONL record must be LF-terminated: %q", body)
	}
	var lines [][]byte
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		lines = append(lines, scanner.Bytes())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan jsonl: %v", err)
	}
	const wantLines = 3 // exactly one line per finding
	if len(lines) != wantLines {
		t.Fatalf("body must carry exactly %d lines, got %d: %q", wantLines, len(lines), body)
	}
	type jsonlRow struct {
		RuleID       string `json:"rule_id"`
		RelativePath string `json:"relative_path"`
	}
	pathsByRule := map[string]string{}
	for index, line := range lines {
		var row jsonlRow
		if err := json.Unmarshal(line, &row); err != nil {
			t.Fatalf("line %d is not valid JSON (%v): %q", index+1, err, line)
		}
		if row.RelativePath == "" || !strings.Contains(string(line), `"relative_path":"`) {
			t.Fatalf("line %d must carry the finding relative_path: %q", index+1, line)
		}
		pathsByRule[row.RuleID] = row.RelativePath
	}
	wantPaths := map[string]string{
		"F401":   "src/main.py",
		"lint/a": "src/a.ts",
		"lint/b": "src/b.ts",
	}
	for rule, want := range wantPaths {
		if pathsByRule[rule] != want {
			t.Fatalf("finding %q exported relative_path %q, want %q", rule, pathsByRule[rule], want)
		}
	}
}

func TestFindingsJSONLRejectsUnknownScan(t *testing.T) {
	s := testServer(t)
	unknown := "00000000-0000-4000-8000-000000000000"
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+unknown+"/findings.jsonl", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "SCAN_NOT_FOUND") {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/not-a-uuid/findings.jsonl", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "SCAN_NOT_FOUND") {
		t.Fatalf("malformed id got %d: %s", response.Code, response.Body.String())
	}
}

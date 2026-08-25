package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/config"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/events"
	"bluntcode/internal/scans"
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

// csvHostileCorpus is the CSV export's instance of the hostile fixture table
// (mirrored from internal/reports, whose test fixtures are not importable).
// Each finding plants one attack family in the column where it is most
// dangerous: formula introducers, LF/CRLF/CR in every text field, markup,
// quote and backslash soup, control bytes, URL-shaped paths, hostile rule
// ids, empties everywhere, and position shapes that are invalid per SARIF
// but must stay a faithful dump in CSV.
func csvHostileCorpus() []analyzers.Finding {
	return []analyzers.Finding{
		{ // BMP + astral planes, CJK, RTL with bidi marks, combining characters
			AnalyzerID: "ruff", RuleID: "unicode-bmp-astral", Severity: analyzers.SeverityHigh, Category: analyzers.CategorySecurity,
			Title:        "Ünïcode ✨ 中文タイトル مرحبا שלום",
			Message:      "BMP ✆ CJK 日本語 RTL مرحبا bidi \u202b\u202e emoji 🚨🚀 ZWJ 👨‍👩‍👧‍👦 combining e\u0301\u0301clat",
			RelativePath: "src/ünïcode/日本語/مرحبا/coffee☕.py", StartLine: 3, StartColumn: 4, EndLine: 3, EndColumn: 9,
			Remediation: "直す", DocumentationURL: "https://例え.jp/☕",
		},
		{ // LF, CRLF, and bare CR in every text field at once
			AnalyzerID: "ruff", RuleID: "newlines-lf-crlf-cr", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryBug,
			Title: "title\ntitle2\r\ntitle3\rtitle4", Message: "line1\nline2\r\nline3\rline4\n\rmixed",
			RelativePath: "dir/new\nline\r\nname.py", Remediation: "fix\r\nme\nplease",
			DocumentationURL: "https://x.example/a\nb\r\nc\rd",
		},
		{ // every formula introducer in a different column
			AnalyzerID: "ruff", RuleID: "=RULE", Severity: analyzers.SeverityCritical, Category: analyzers.CategorySecurity,
			Title:   `+HYPERLINK("http://evil.example","click me")`,
			Message: "=cmd|'/c calc'!A1", RelativePath: "-leading-dash.py",
			Remediation: "-2+cmd|'/C calc'!A0", DocumentationURL: "@SUM(1+1)*cmd|'/C calc'!A0",
		},
		{AnalyzerID: "ruff", RuleID: "TAB-MSG", Severity: analyzers.SeverityHigh, Category: analyzers.CategorySecurity,
			Message: "\t=cmd|'/c calc'!A1", RelativePath: "src/tab.py"},
		{AnalyzerID: "ruff", RuleID: "CR-MSG", Severity: analyzers.SeverityHigh, Category: analyzers.CategorySecurity,
			Message: "\r=cmd|'/c calc'!A1", RelativePath: "src/cr.py"},
		{ // markdown structure forgers: pipes, backticks, ATX headings
			AnalyzerID: "ruff", RuleID: "pipes-backticks-headings", Severity: analyzers.SeverityLow, Category: analyzers.CategoryStyle,
			Title: "| Title | Forge |", Message: "# H1\n## H2 | a | b || :--- | `code` ``` ticks", RelativePath: "dir|pipe.py",
		},
		{ // HTML and script markup stays inert text in a spreadsheet cell
			AnalyzerID: "semgrep", RuleID: "html-markup", Severity: analyzers.SeverityHigh, Category: analyzers.CategoryVulnerability,
			Title:        `</tr></td><script>alert("xss")</script>`,
			Message:      `<script>alert(1)</script> & <img src=x onerror=alert(1)>`,
			RelativePath: "src/<b>bold</b>.py",
		},
		{ // JSON-breaking quotes, backslashes, and brace soup
			AnalyzerID: "ruff", RuleID: "json-breaking", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryBug,
			Message: `quotes " backslash \ escaped \" and {"json":"like\"structure"}`, RelativePath: `weird"path\name.py`,
		},
		{ // leading/trailing whitespace and tabs
			AnalyzerID: "ruff", RuleID: "whitespace-edges", Severity: analyzers.SeverityLow, Category: analyzers.CategoryStyle,
			Title: "\ttabbed title\t", Message: "   leading and trailing   ", RelativePath: "  spaced path  ",
		},
		{ // very long single token
			AnalyzerID: "ruff", RuleID: "long-token", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryMaintainability,
			Message: strings.Repeat("A", 10_000) + "☕", RelativePath: "long.py",
		},
		{ // paths that look like URLs
			AnalyzerID: "ruff", RuleID: "url-like-path", Severity: analyzers.SeverityLow, Category: analyzers.CategoryStyle,
			Message: "path looks like a URL", RelativePath: "https://evil.example/payload.py",
		},
		{AnalyzerID: "ruff", RuleID: "file-url-path", Severity: analyzers.SeverityLow, Category: analyzers.CategoryStyle,
			Message: "path looks like a file URL", RelativePath: "file:///C:/Users/x/file.py"},
		{ // percent, hash, quotes, spaces, plus, unicode in one path
			AnalyzerID: "ruff", RuleID: "path-zoo", Severity: analyzers.SeverityInfo, Category: analyzers.CategoryStyle,
			Message: "zoo", RelativePath: `100% #1 'single' "double" +plus ünïcode.py`,
		},
		{ // Windows separators
			AnalyzerID: "biome", RuleID: "windows-path", Severity: analyzers.SeverityLow, Category: analyzers.CategoryStyle,
			Message: "windows", RelativePath: `dir\sub dir\file name.py`,
		},
		{ // rule ids with colons, dashes, spaces, unicode
			AnalyzerID: "biome", RuleID: "rule:with:colons", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryBug,
			Message: "colons", RelativePath: "a.py",
		},
		{AnalyzerID: "ruff", RuleID: "rule with spaces and ‼️", Severity: analyzers.SeverityInfo, Category: analyzers.CategoryStyle,
			Message: "spaced rule", RelativePath: "c.py"},
		{ // empty strings in every optional field
			AnalyzerID: "ruff", RuleID: "empty-fields", Severity: analyzers.SeverityInfo, Category: analyzers.CategoryOther,
			Title: "", Message: "", RelativePath: "", Remediation: "", DocumentationURL: "",
		},
		{ // NUL, BEL, ANSI escapes, DEL, vertical tab, form feed: stripped everywhere
			AnalyzerID: "ruff", RuleID: "nul-and-controls", Severity: analyzers.SeverityHigh, Category: analyzers.CategorySecurity,
			Title: "t\x00i\x07t", Message: "nul\x00bell\x07esc\x1b[31mred\x1b[0m del\x7f vertical\x0btab\x0c",
			RelativePath: "ctrl.py", Remediation: "r\x1br", DocumentationURL: "https://x.example/\x00",
		},
		{ // severity exports normalized; raw analyzer casing is display-only
			AnalyzerID: "sonarqube", RuleID: "raw-severity-casing", Severity: analyzers.SeverityHigh, RawSeverity: "HIGH!!!",
			Category: analyzers.CategoryCodeSmell, Message: "raw severity kept for display", RelativePath: "raw.py",
		},
		{ // start column without a start line: exported faithfully
			AnalyzerID: "ruff", RuleID: "column-without-line", Severity: analyzers.SeverityInfo, Category: analyzers.CategoryStyle,
			Message: "column only", RelativePath: "col.py", StartColumn: 7,
		},
		{ // end positions before start positions: CSV is a dump, SARIF clamps
			AnalyzerID: "ruff", RuleID: "inverted-region", Severity: analyzers.SeverityMedium, Category: analyzers.CategoryBug,
			Message: "inverted", RelativePath: "inv.py", StartLine: 10, StartColumn: 8, EndLine: 5, EndColumn: 3,
		},
	}
}

// csvWantCell is the test's independent model of csvCell: hostile control
// bytes become spaces (one for one) and a leading formula introducer gains a
// single quote. The corpus test proves these are the ONLY changes the export
// makes to analyzer text.
func csvWantCell(value string) string {
	var b strings.Builder
	for _, r := range value {
		if (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0x7f {
			r = ' '
		}
		b.WriteRune(r)
	}
	out := b.String()
	if out == "" {
		return out
	}
	switch out[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + out
	}
	return out
}

// csvParseNormal models encoding/csv's reader behavior: a CRLF inside a
// quoted field is returned as LF. Lone CR and LF round-trip, and the raw file
// still carries the original bytes (asserted separately), so this is the
// documented reader normalization, not export corruption.
func csvParseNormal(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// csvHasHostileControl matches the export's scrub predicate.
func csvHasHostileControl(r rune) bool {
	return (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0x7f
}

// TestFindingsCSVSURVIVESHostileCorpus round-trips the hostile fixture table
// through findings.csv and decodes it with encoding/csv. It pins: the BOM,
// byte-level UTF-8 validity, no hostile control byte anywhere in the file,
// non-ASCII kept raw, the header, one 13-column row per finding, field-for-
// field equality with the exact source strings (modulo the modeled csvCell
// transform and the reader's in-quote CRLF normalization), verbatim field
// bytes for embedded newlines, and LF record terminators with byte-exact CR/LF
// accounting.
func TestFindingsCSVSURVIVESHostileCorpus(t *testing.T) {
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
	corpus := csvHostileCorpus()
	if _, err := s.db.SaveAnalyzerResult(ctx, scan.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, corpus, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(ctx, scan.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scan.ID+"/findings.csv", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("findings.csv: %d %s", response.Code, response.Body.String())
	}
	body := response.Body.Bytes()

	// Byte level: UTF-8 BOM for Excel, valid UTF-8, no hostile control byte,
	// and non-ASCII payloads stay raw (CSV has no escape syntax to hide them).
	if !bytes.HasPrefix(body, []byte(csvBOM)) {
		t.Fatalf("CSV must start with the UTF-8 BOM: %q", body[:min(len(body), 40)])
	}
	if !utf8.Valid(body) {
		t.Fatal("CSV must be valid UTF-8")
	}
	for _, banned := range []string{"\x00", "\x07", "\x0b", "\x0c", "\x1b", "\x7f"} {
		if bytes.Contains(body, []byte(banned)) {
			t.Fatalf("raw control byte %q survived into the CSV bytes", banned)
		}
	}
	for _, want := range []string{"🚨🚀", "日本語", "\u202b\u202e", "e\u0301\u0301clat", strings.Repeat("A", 10_000) + "☕"} {
		if !bytes.Contains(body, []byte(want)) {
			t.Fatalf("non-ASCII payload %q must stay raw UTF-8 in the file", want)
		}
	}

	// Structure: header intact, one row per finding, 13 columns everywhere.
	records, err := csv.NewReader(bytes.NewReader(body[len(csvBOM):])).ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v: %q", err, body)
	}
	if len(records) != 1+len(corpus) {
		t.Fatalf("expected header plus %d rows, got %d records", len(corpus), len(records))
	}
	wantHeader := "severity,category,analyzer,rule_id,title,message,file,line,column,end_line,status,remediation,documentation_url"
	if strings.Join(records[0], ",") != wantHeader {
		t.Fatalf("unexpected header: %#v", records[0])
	}
	for _, row := range records {
		if len(row) != 13 {
			t.Fatalf("every row must carry 13 fields: %#v", row)
		}
	}

	// Field-for-field round-trip: each cell decodes to the exact source value
	// modulo the modeled csvCell transform and the reader's CRLF compression.
	byRule := map[string][]string{}
	for _, row := range records[1:] {
		byRule[row[3]] = row
	}
	csvNum := func(v int) string {
		if v == 0 {
			return ""
		}
		return strconv.Itoa(v)
	}
	textColumns := func(f analyzers.Finding) []string {
		return []string{string(f.Severity), string(f.Category), f.AnalyzerID, f.RuleID, f.Title, f.Message, f.RelativePath, f.Remediation, f.DocumentationURL}
	}
	for _, f := range corpus {
		row := byRule[csvParseNormal(csvWantCell(f.RuleID))]
		if row == nil {
			t.Fatalf("finding %q missing from the export (rows keyed by rule_id: %v)", f.RuleID, len(byRule))
		}
		want := []string{
			csvParseNormal(csvWantCell(string(f.Severity))), csvParseNormal(csvWantCell(string(f.Category))),
			csvParseNormal(csvWantCell(f.AnalyzerID)), csvParseNormal(csvWantCell(f.RuleID)),
			csvParseNormal(csvWantCell(f.Title)), csvParseNormal(csvWantCell(f.Message)),
			csvParseNormal(csvWantCell(f.RelativePath)),
			csvNum(f.StartLine), csvNum(f.StartColumn), csvNum(f.EndLine), "new",
			csvParseNormal(csvWantCell(f.Remediation)), csvParseNormal(csvWantCell(f.DocumentationURL)),
		}
		for i := range want {
			if row[i] != want[i] {
				t.Fatalf("%q column %d: got %q, want %q (full row %#v)", f.RuleID, i, row[i], want[i], row)
			}
		}
	}

	// The transform is precisely as modeled and is the ONLY change: benign
	// cells are identity, formula-first cells gain exactly one leading quote,
	// and control bytes trade one rune for one space.
	for _, f := range corpus {
		for _, v := range textColumns(f) {
			transformed := csvWantCell(v)
			if !strings.ContainsFunc(v, csvHasHostileControl) {
				switch {
				case v == "":
				case strings.ContainsRune("=+-@\t\r", rune(v[0])):
					if transformed != "'"+v {
						t.Fatalf("formula-first cell %q must gain exactly one leading quote, got %q", v, transformed)
					}
				default:
					if transformed != v {
						t.Fatalf("benign cell %q must round-trip untouched, got %q", v, transformed)
					}
				}
				continue
			}
			// One for one: every hostile rune becomes exactly one space, plus
			// the optional single quote prefix.
			quote := 0
			if strings.HasPrefix(transformed, "'") {
				quote = 1
			}
			if got, wantRunes := len([]rune(transformed)), len([]rune(v))+quote; got != wantRunes {
				t.Fatalf("scrub must be one rune for one rune: %q (%d) -> %q (%d)", v, len([]rune(v)), transformed, got)
			}
		}
	}

	// Verbatim field bytes: the writer must not rewrite any embedded newline
	// (UseCRLF mode deletes lone CR bytes, which this export refuses to do).
	// The mixed-newline message carries no quotes, so it is emitted verbatim
	// inside its quotes with LF, CRLF, and lone CR all intact.
	if !bytes.Contains(body, []byte("\"line1\nline2\r\nline3\rline4\n\rmixed\"")) {
		t.Fatalf("embedded newline bytes must survive verbatim inside the quoted field")
	}

	// LF record terminators, byte-accounted: every CR in the file belongs to
	// a field; every LF is either a field byte or one of the 1+len(corpus)
	// record terminators.
	wantLF, wantCR := len(records), 0
	for _, f := range corpus {
		for _, v := range textColumns(f) {
			cell := csvWantCell(v)
			wantLF += strings.Count(cell, "\n")
			wantCR += strings.Count(cell, "\r")
		}
	}
	if got := bytes.Count(body, []byte("\n")); got != wantLF {
		t.Fatalf("file carries %d LF bytes, want %d (%d record terminators plus embedded field bytes)", got, wantLF, len(records))
	}
	if got := bytes.Count(body, []byte("\r")); got != wantCR {
		t.Fatalf("file carries %d CR bytes, want %d (only embedded field bytes; terminators are bare LF)", got, wantCR)
	}

	// Severity exports normalized; the raw analyzer casing never reaches the file.
	if bytes.Contains(body, []byte("HIGH!!!")) {
		t.Fatalf("raw analyzer severity casing leaked into the CSV")
	}
	if row := byRule["raw-severity-casing"]; row == nil || row[0] != "high" {
		t.Fatalf("severity column must carry the normalized value: %#v", row)
	}
}

// TestFindingsJSONDownloadsAttachment pins the findings.json export: the same
// attachment conventions as findings.csv (content type, disposition, short
// scan id), served through the loopback/security middleware like every route,
// carrying the full versioned report document rather than a bare findings
// array.
func TestFindingsJSONDownloadsAttachment(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "completed", Profile: "standard",
		CandidateFileCount: 5, SelectedFileCount: 3,
		Snapshot: &core.ScanSnapshot{BluntCodeVersion: "9.9.9", CapturedAt: time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	// A previous completed scan carrying the same F401 finding makes its
	// comparison status persistent in the current scan; S101 is brand new.
	f := finding("ruff", "F401", "src/main.py", "unused import", analyzers.SeverityHigh, analyzers.CategoryCorrectness)
	f.StartLine, f.StartColumn, f.EndLine, f.EndColumn = 3, 1, 3, 20
	f.Remediation, f.DocumentationURL = "Remove the import", "https://docs.astral.sh/ruff/rules/F401"
	previous, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.SaveAnalyzerResult(ctx, previous.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{f}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(ctx, previous.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	fresh := finding("ruff", "S101", "src/other.py", "assert used", analyzers.SeverityMedium, analyzers.CategoryBug)
	if _, err := s.db.SaveAnalyzerResult(ctx, scan.ID, database.AnalyzerRunInput{AnalyzerID: "ruff", Version: "test", State: "succeeded"}, []analyzers.Finding{f, fresh}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.db.CompleteScan(ctx, scan.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+scan.ID+"/findings.json", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("findings.json: %d %s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf("content type %q", contentType)
	}
	disposition := response.Header().Get("Content-Disposition")
	if disposition != `attachment; filename="bluntcode-scan-`+scan.ID[:8]+`-findings.json"` {
		t.Fatalf("disposition %q", disposition)
	}
	body := response.Body.Bytes()
	if !bytes.HasSuffix(body, []byte("}\n")) {
		t.Fatalf("document must end with a single trailing LF: %q", body[min(len(body), 32):])
	}
	var doc struct {
		Schema        string `json:"schema"`
		SchemaVersion int    `json:"schemaVersion"`
		Workspace     struct {
			Name string `json:"name"`
		} `json:"workspace"`
		Scan struct {
			ID        string `json:"id"`
			State     string `json:"state"`
			Profile   string `json:"profile"`
			StartedAt string `json:"started_at"`
		} `json:"scan"`
		Files struct {
			Candidate int `json:"candidate"`
			Selected  int `json:"selected"`
			Skipped   int `json:"skipped"`
		} `json:"files"`
		Severity struct {
			High   int `json:"high"`
			Medium int `json:"medium"`
			Total  int `json:"total"`
		} `json:"severity"`
		Comparison struct {
			New        int `json:"new"`
			Fixed      int `json:"fixed"`
			Persistent int `json:"persistent"`
		} `json:"comparison"`
		Analyzers []struct {
			ID       string `json:"id"`
			State    string `json:"state"`
			Findings int    `json:"findings"`
		} `json:"analyzers"`
		Findings []struct {
			Severity         string `json:"severity"`
			RuleID           string `json:"rule_id"`
			File             string `json:"file"`
			StartLine        int    `json:"start_line"`
			StartColumn      int    `json:"start_column"`
			EndLine          int    `json:"end_line"`
			EndColumn        int    `json:"end_column"`
			Status           string `json:"status"`
			Remediation      string `json:"remediation"`
			DocumentationURL string `json:"documentation_url"`
			Fingerprint      string `json:"fingerprint"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("invalid JSON report: %v: %s", err, body)
	}
	if doc.Schema != "bluntcode/scan-report" || doc.SchemaVersion != 1 {
		t.Fatalf("schema marker = %q v%d", doc.Schema, doc.SchemaVersion)
	}
	if doc.Workspace.Name != "Example" || doc.Scan.ID != scan.ID || doc.Scan.State != "completed" || doc.Scan.Profile != "standard" || doc.Scan.StartedAt == "" {
		t.Fatalf("scan block = %#v", doc.Scan)
	}
	if doc.Files.Candidate != 5 || doc.Files.Selected != 3 || doc.Files.Skipped != 2 {
		t.Fatalf("file counts = %#v", doc.Files)
	}
	if doc.Severity.High != 1 || doc.Severity.Medium != 1 || doc.Severity.Total != 2 {
		t.Fatalf("severity counts = %#v", doc.Severity)
	}
	if doc.Comparison.New != 1 || doc.Comparison.Fixed != 0 || doc.Comparison.Persistent != 1 {
		t.Fatalf("comparison = %#v", doc.Comparison)
	}
	if len(doc.Analyzers) != 1 || doc.Analyzers[0].ID != "ruff" || doc.Analyzers[0].State != "succeeded" || doc.Analyzers[0].Findings != 2 {
		t.Fatalf("analyzers = %#v", doc.Analyzers)
	}
	if len(doc.Findings) != 2 {
		t.Fatalf("findings = %#v", doc.Findings)
	}
	got, added := doc.Findings[0], doc.Findings[1]
	if got.Severity != "high" || got.RuleID != "F401" || got.File != "src/main.py" || got.StartLine != 3 || got.StartColumn != 1 ||
		got.EndLine != 3 || got.EndColumn != 20 || got.Status != "persistent" || got.Remediation != "Remove the import" ||
		got.DocumentationURL != "https://docs.astral.sh/ruff/rules/F401" || got.Fingerprint != f.Fingerprint {
		t.Fatalf("persistent finding = %#v", got)
	}
	if added.RuleID != "S101" || added.File != "src/other.py" || added.Status != "new" || added.Severity != "medium" {
		t.Fatalf("new finding = %#v", added)
	}
}

func TestFindingsJSONRejectsUnknownScan(t *testing.T) {
	s := testServer(t)
	unknown := "00000000-0000-4000-8000-000000000000"
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+unknown+"/findings.json", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "SCAN_NOT_FOUND") {
		t.Fatalf("got %d: %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/not-a-uuid/findings.json", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "SCAN_NOT_FOUND") {
		t.Fatalf("malformed id got %d: %s", response.Code, response.Body.String())
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

type suppressionsResponse struct {
	Items []core.Suppression `json:"items"`
}

func TestSuppressionRoutesLifecycle(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := strings.Repeat("ab", 32)
	base := "http://127.0.0.1/api/v1/workspaces/" + work.ID + "/suppressions"

	request := httptest.NewRequest(http.MethodPost, base, strings.NewReader(`{"fingerprint":"`+fingerprint+`","reason":"wontfix"}`))
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var created core.Suppression
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &created) != nil ||
		created.WorkspaceID != work.ID || created.Fingerprint != fingerprint || created.Reason != "wontfix" || created.CreatedAt.IsZero() {
		t.Fatalf("create: %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, base, nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var list suppressionsResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &list) != nil || len(list.Items) != 1 || list.Items[0].Fingerprint != fingerprint {
		t.Fatalf("list: %d %s", response.Code, response.Body.String())
	}

	// Invalid bodies and fingerprints are rejected.
	for name, body := range map[string]string{
		"short fingerprint":   `{"fingerprint":"abc"}`,
		"missing fingerprint": `{"reason":"no fingerprint"}`,
		"malformed json":      `{"fingerprint":`,
		"long reason":         `{"fingerprint":"` + fingerprint + `","reason":"` + strings.Repeat("x", 501) + `"}`,
	} {
		request = httptest.NewRequest(http.MethodPost, base, strings.NewReader(body))
		response = httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s was accepted: %d %s", name, response.Code, response.Body.String())
		}
	}

	// Deleting a well-formed fingerprint that was never suppressed is a 404.
	request = httptest.NewRequest(http.MethodDelete, base+"/"+strings.Repeat("99", 32), nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown suppression delete: %d %s", response.Code, response.Body.String())
	}
	// A malformed fingerprint is a 400, not a 404.
	request = httptest.NewRequest(http.MethodDelete, base+"/nothex", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("malformed fingerprint delete: %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodDelete, base+"/"+fingerprint, nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, base, nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &list) != nil || len(list.Items) != 0 {
		t.Fatalf("list after delete: %d %s", response.Code, response.Body.String())
	}

	// Unknown workspaces 404 on every verb.
	missing := "http://127.0.0.1/api/v1/workspaces/11111111-1111-1111-1111-111111111111/suppressions"
	for _, target := range []struct {
		method, url string
		body        io.Reader
	}{
		{http.MethodGet, missing, nil},
		{http.MethodPost, missing, strings.NewReader(`{"fingerprint":"` + fingerprint + `"}`)},
		{http.MethodDelete, missing + "/" + fingerprint, nil},
	} {
		request = httptest.NewRequest(target.method, target.url, target.body)
		response = httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("unknown workspace %s %s: %d %s", target.method, target.url, response.Code, response.Body.String())
		}
	}
}

// TestReportAndExportsExcludeSuppressedFindings pins the model filter point:
// once a fingerprint is suppressed, the report model (and every renderer built
// from it) and the CSV export drop the finding, while the findings list keeps
// it visible with the suppressed status.
func TestReportAndExportsExcludeSuppressedFindings(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	dismissed := finding("ruff", "DISMISSED", "src/dismissed.py", "dismissed issue", analyzers.SeverityHigh, analyzers.CategoryCorrectness)
	kept := finding("ruff", "KEPT", "src/kept.py", "kept issue", analyzers.SeverityMedium, analyzers.CategoryCorrectness)
	completedScan(t, s, work.ID, "completed", dismissed)
	current := completedScan(t, s, work.ID, "completed", dismissed, kept)

	fetchReport := func(t *testing.T) (findings []analyzers.Finding, comparison map[string]int) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+current.ID+"/report", nil)
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		var body struct {
			Findings   []analyzers.Finding `json:"findings"`
			Comparison map[string]int      `json:"comparison"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
			t.Fatalf("report: %d %s", response.Code, response.Body.String())
		}
		return body.Findings, body.Comparison
	}

	beforeFindings, beforeComparison := fetchReport(t)
	if len(beforeFindings) != 2 || beforeComparison["persistent_count"] != 1 || beforeComparison["new_count"] != 1 {
		t.Fatalf("report before suppression: %#v", beforeComparison)
	}

	if _, err := s.db.AddSuppression(ctx, work.ID, dismissed.Fingerprint, "wontfix"); err != nil {
		t.Fatal(err)
	}
	afterFindings, afterComparison := fetchReport(t)
	if len(afterFindings) != 1 || afterFindings[0].RuleID != "KEPT" {
		t.Fatalf("suppressed finding must be absent from the report model: %#v", afterFindings)
	}
	if afterComparison["new_count"] != 1 || afterComparison["persistent_count"] != 0 || afterComparison["fixed_count"] != 0 {
		t.Fatalf("comparison must ignore the suppressed fingerprint: %#v", afterComparison)
	}

	records := csvRows(t, s, current.ID, "")
	if len(records) != 2 || csvRowByRule(records, "DISMISSED") != nil {
		t.Fatalf("CSV export must exclude the suppressed finding: %#v", records)
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+current.ID+"/findings?status=suppressed", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var page findingsResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &page) != nil ||
		page.Total != 1 || len(page.Items) != 1 || page.Items[0].RuleID != "DISMISSED" || page.Items[0].Status != "suppressed" {
		t.Fatalf("findings list must keep the suppressed finding visible: %d %s", response.Code, response.Body.String())
	}
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

// scanLifecycleAnalyzer is a controllable analyzer for API-level lifecycle
// tests: Run blocks until the test releases it, so a scan stays active while
// the test exercises the cancel endpoint.
type scanLifecycleAnalyzer struct {
	id         string
	runEntered chan struct{}
	runGate    chan struct{}
}

func (a *scanLifecycleAnalyzer) ID() string          { return a.id }
func (a *scanLifecycleAnalyzer) DisplayName() string { return a.id }
func (a *scanLifecycleAnalyzer) SupportedLanguages() []analyzers.Language {
	return []analyzers.Language{analyzers.LanguagePython}
}
func (a *scanLifecycleAnalyzer) Check(context.Context, analyzers.ToolEnvironment) analyzers.ToolStatus {
	return analyzers.ToolStatus{Ready: true, Version: "test"}
}
func (a *scanLifecycleAnalyzer) EnsureInstalled(context.Context, analyzers.ToolEnvironment) error {
	return nil
}
func (a *scanLifecycleAnalyzer) Plan(context.Context, analyzers.ScanRequest) (analyzers.AnalyzerPlan, error) {
	return analyzers.AnalyzerPlan{AnalyzerID: a.id, Version: "test"}, nil
}
func (a *scanLifecycleAnalyzer) Run(context.Context, analyzers.AnalyzerPlan, analyzers.EventEmitter) (analyzers.AnalyzerResult, error) {
	if a.runEntered != nil {
		close(a.runEntered)
	}
	if a.runGate != nil {
		<-a.runGate
	}
	return analyzers.AnalyzerResult{}, nil
}
func (a *scanLifecycleAnalyzer) Normalize(context.Context, analyzers.AnalyzerResult) ([]analyzers.Finding, []analyzers.Metric, error) {
	return nil, nil, nil
}

func scanLifecycleServer(t *testing.T, adapters ...analyzers.Analyzer) (*Server, *scanLifecycleAnalyzer) {
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
	registry := analyzers.NewRegistry()
	blocking := &scanLifecycleAnalyzer{id: "fake", runEntered: make(chan struct{}), runGate: make(chan struct{})}
	if len(adapters) > 0 {
		for _, adapter := range adapters {
			if err := registry.Register(adapter); err != nil {
				t.Fatal(err)
			}
		}
	} else if err := registry.Register(blocking); err != nil {
		t.Fatal(err)
	}
	bus := events.New()
	service := scans.New(db, registry, bus, filepath.Join(paths.DataDir, "reports"), paths.ToolsDir, nil)
	return New(db, bus, service, nil, paths, "test", nil), blocking
}

func scanLifecycleWorkspace(t *testing.T, s *Server) core.Workspace {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.py"), []byte("x=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	work, err := s.db.CreateWorkspace(context.Background(), core.Workspace{Name: "Lifecycle", RootPath: root})
	if err != nil {
		t.Fatal(err)
	}
	return work
}

// TestScanEventsReplaysHistoryThenStreamsLiveInOrder pins the SSE contract:
// a client connecting mid-scan receives the bus replay first, then every live
// event, with no loss or duplication across the handoff.
func TestScanEventsReplaysHistoryThenStreamsLiveInOrder(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "SSE"})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "running"})
	if err != nil {
		t.Fatal(err)
	}
	s.bus.Publish(events.Event{Type: "scan.started", ScanID: scan.ID})
	s.bus.Publish(events.Event{Type: "analyzer.started", ScanID: scan.ID})
	httpServer := httptest.NewServer(s.Handler())
	defer httpServer.Close()
	requestCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, httpServer.URL+"/api/v1/scans/"+scan.ID+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if contentType := response.Header.Get("Content-Type"); contentType != "text/event-stream" {
		t.Fatalf("content type %q", contentType)
	}
	streamed := make(chan string, 32)
	go func() {
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			if line := scanner.Text(); strings.HasPrefix(line, "event: ") {
				streamed <- strings.TrimPrefix(line, "event: ")
			}
		}
		close(streamed)
	}()
	expectStreamed := func(want string) {
		t.Helper()
		select {
		case got := <-streamed:
			if got != want {
				t.Fatalf("streamed event = %q, want %q", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %q", want)
		}
	}
	expectStreamed("connected")
	expectStreamed("scan.started")
	expectStreamed("analyzer.started")
	s.bus.Publish(events.Event{Type: "analyzer.completed", ScanID: scan.ID})
	expectStreamed("analyzer.completed")
	s.bus.Publish(events.Event{Type: "scan.completed", ScanID: scan.ID})
	expectStreamed("scan.completed")
	cancel()
}

// TestCancelScanEndpointRejectsDoubleAndFinishedCancels checks cancel-of-cancel
// semantics through the HTTP surface: cancelling an active scan twice is a safe
// no-op, a second workspace scan is rejected with 409 while it runs, and after
// the scan reaches a terminal state further cancels get 409.
func TestCancelScanEndpointRejectsDoubleAndFinishedCancels(t *testing.T) {
	s, blocking := scanLifecycleServer(t)
	work := scanLifecycleWorkspace(t, s)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/workspaces/"+work.ID+"/scans", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("start scan: %d %s", response.Code, response.Body.String())
	}
	var started struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	<-blocking.runEntered

	second := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/workspaces/"+work.ID+"/scans", nil)
	secondResponse := httptest.NewRecorder()
	s.Handler().ServeHTTP(secondResponse, second)
	if secondResponse.Code != http.StatusConflict {
		t.Fatalf("second scan on the same workspace: %d, want 409", secondResponse.Code)
	}

	cancelRequest := func() int {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/scans/"+started.ID+"/cancel", nil)
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		return response.Code
	}
	if code := cancelRequest(); code != http.StatusAccepted {
		t.Fatalf("first cancel: %d, want 202", code)
	}
	if code := cancelRequest(); code != http.StatusAccepted {
		t.Fatalf("double cancel must be a safe no-op: %d, want 202", code)
	}
	close(blocking.runGate)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		scan, err := s.db.Scan(context.Background(), started.ID)
		if err != nil {
			t.Fatal(err)
		}
		if scan.State == "cancelled" {
			if code := cancelRequest(); code != http.StatusConflict {
				t.Fatalf("cancel after finish: %d, want 409", code)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("scan did not cancel")
}

// TestStartScanAllowedAfterInterruptedRecovery covers the kill -9 path: a
// stale running scan row blocks new scans with 409 until startup recovery
// marks it interrupted, after which a new scan starts normally.
func TestStartScanAllowedAfterInterruptedRecovery(t *testing.T) {
	s, _ := scanLifecycleServer(t, &scanLifecycleAnalyzer{id: "fake"})
	work := scanLifecycleWorkspace(t, s)
	stale, err := s.db.CreateScan(context.Background(), core.Scan{WorkspaceID: work.ID, State: "running"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/workspaces/"+work.ID+"/scans", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("scan during stale running row: %d, want 409", response.Code)
	}
	if err := s.db.MarkInterruptedScans(context.Background()); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.db.Scan(context.Background(), stale.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State != "interrupted" || recovered.FinishedAt == nil {
		t.Fatalf("recovered stale scan: %#v", recovered)
	}
	request = httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/workspaces/"+work.ID+"/scans", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("scan after recovery: %d %s, want 202", response.Code, response.Body.String())
	}
}

// TestCreateWorkspaceReusesJunctionAndCaseVariants pins the normalization
// contract at the API entry point: a workspace registered through its real
// path is reused (200, not 201) when re-added via an NTFS junction or a
// case-variant spelling of the same directory, and only one row is stored.
func TestCreateWorkspaceReusesJunctionAndCaseVariants(t *testing.T) {
	s := testServer(t)
	httpServer := httptest.NewServer(s.Handler())
	defer httpServer.Close()

	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "marker.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	post := func(path string) (int, string) {
		body, err := json.Marshal(map[string]string{"root_path": path})
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.Post(httpServer.URL+"/api/v1/workspaces", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		return response.StatusCode, string(raw)
	}

	if code, body := post(target); code != http.StatusCreated {
		t.Fatalf("first create = %d, want 201: %s", code, body)
	}
	if code, body := post(strings.ToUpper(target)); code != http.StatusOK {
		t.Fatalf("case-variant create = %d, want 200: %s", code, body)
	}
	if runtime.GOOS == "windows" {
		link := filepath.Join(t.TempDir(), "junction-link")
		out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
		if err != nil {
			t.Skipf("cannot create junction: %v: %s", err, out)
		}
		if code, body := post(link); code != http.StatusOK {
			t.Fatalf("junction create = %d, want 200: %s", code, body)
		}
	}

	list, err := s.db.Workspaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("stored %d workspaces, want 1", len(list))
	}
}

func TestRecentScansWorkspaceFilterScopesFeed(t *testing.T) {
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
	completedScan(t, s, alpha.ID, "completed", finding("ruff", "A1", "src/a1.py", "alpha issue", analyzers.SeverityHigh, analyzers.CategoryCorrectness))
	completedScan(t, s, beta.ID, "completed", finding("ruff", "B1", "src/b1.py", "beta issue", analyzers.SeverityLow, analyzers.CategoryCorrectness))

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans?workspace_id="+alpha.ID, nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var body recentScansResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
		t.Fatalf("workspace filter: %d %s", response.Code, response.Body.String())
	}
	if body.Total != 1 || len(body.Scans) != 1 || body.Scans[0].WorkspaceID != alpha.ID {
		t.Fatalf("workspace filter must scope the feed: total=%d items=%#v", body.Total, body.Scans)
	}

	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans?workspace_id="+alpha.ID+"&state=running", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.Total != 0 || len(body.Scans) != 0 {
		t.Fatalf("combined workspace+state filter: %d %s", response.Code, response.Body.String())
	}
}

func TestDeleteScanRemovesTerminalScanAndCascades(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Deletable"})
	if err != nil {
		t.Fatal(err)
	}
	terminal := completedScan(t, s, work.ID, "completed", finding("ruff", "R1", "src/a.py", "old issue", analyzers.SeverityLow, analyzers.CategoryCorrectness))
	active, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "running"})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodDelete, "http://127.0.0.1/api/v1/scans/"+active.ID, nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("deleting an active scan must conflict: %d %s", response.Code, response.Body.String())
	}

	var findingsBefore int
	if err := s.db.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM findings WHERE scan_id=?`, terminal.ID).Scan(&findingsBefore); err != nil {
		t.Fatal(err)
	}
	if findingsBefore != 1 {
		t.Fatalf("fixture expected one finding, got %d", findingsBefore)
	}
	request = httptest.NewRequest(http.MethodDelete, "http://127.0.0.1/api/v1/scans/"+terminal.ID, nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("deleting a terminal scan: %d %s", response.Code, response.Body.String())
	}
	var findingsAfter int
	if err := s.db.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM findings WHERE scan_id=?`, terminal.ID).Scan(&findingsAfter); err != nil {
		t.Fatal(err)
	}
	if findingsAfter != 0 {
		t.Fatalf("findings must cascade-delete with their scan, got %d", findingsAfter)
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+terminal.ID, nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("deleted scan must 404 on read: %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodDelete, "http://127.0.0.1/api/v1/scans/"+terminal.ID, nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("repeat delete must 404: %d", response.Code)
	}
}

func TestSearchFindingsSpansWorkspacesWithFilters(t *testing.T) {
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
	suppressMe := finding("semgrep", "py-eval", "app/run.py", "eval of untrusted input", analyzers.SeverityCritical, analyzers.CategorySecurity)
	first := completedScan(t, s, alpha.ID, "completed",
		suppressMe,
		finding("ruff", "E501", "app/run.py", "line too long", analyzers.SeverityLow, analyzers.CategoryStyle))
	second := completedScan(t, s, beta.ID, "completed",
		finding("secrets", "aws-key", "cfg/env.ini", "AWS access key looks live", analyzers.SeverityHigh, analyzers.CategorySecurity))

	search := func(query string) searchResponse {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/findings/search"+query, nil)
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("search %q: %d %s", query, response.Code, response.Body.String())
		}
		var body searchResponse
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode search body: %v", err)
		}
		return body
	}

	body := search("")
	if body.Total != 3 || len(body.Items) != 3 || body.HasNext {
		t.Fatalf("unfiltered search totals: %#v", body)
	}
	if body.Items[0].Severity != analyzers.SeverityCritical {
		t.Fatalf("results must lead with the highest severity: %#v", body.Items[0])
	}
	if body.Items[0].ScanID != first.ID || body.Items[0].WorkspaceID != alpha.ID {
		t.Fatalf("results must carry scan/workspace identity: %#v", body.Items[0])
	}

	body = search("?q=run.py")
	if body.Total != 2 {
		t.Fatalf("path substring match: %#v", body)
	}
	body = search("?severity=high,critical")
	if body.Total != 2 {
		t.Fatalf("severity list match: %#v", body)
	}
	body = search("?analyzer=Secrets")
	if body.Total != 1 || body.Items[0].RuleID != "aws-key" {
		t.Fatalf("analyzer filter must be case-insensitive: %#v", body)
	}
	body = search("?workspace_id=" + beta.ID)
	if body.Total != 1 || body.Items[0].ScanID != second.ID {
		t.Fatalf("workspace scoping: %#v", body)
	}
	body = search("?page=2&page_size=2")
	if body.Total != 3 || len(body.Items) != 1 || body.HasNext {
		t.Fatalf("paging window: %#v", body)
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/findings/search?page_size=1000", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized page_size must fail: %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/findings/search?severity=bogus", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown severity must fail: %d", response.Code)
	}

	if _, err := s.db.AddSuppression(ctx, alpha.ID, suppressMe.Fingerprint, ""); err != nil {
		t.Fatal(err)
	}
	body = search("")
	if body.Total != 2 {
		t.Fatalf("suppressed finding must drop from default search: total=%d", body.Total)
	}
}

type searchResponse struct {
	Items []struct {
		analyzers.Finding
		ScanID      string `json:"scan_id"`
		WorkspaceID string `json:"workspace_id"`
	} `json:"items"`
	Total    int  `json:"total"`
	Page     int  `json:"page"`
	PageSize int  `json:"page_size"`
	HasNext  bool `json:"has_next"`
}

func TestScanEventsStreamSendsHeartbeats(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Pulse"})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := s.db.CreateScan(ctx, core.Scan{WorkspaceID: work.ID, State: "running"})
	if err != nil {
		t.Fatal(err)
	}
	prevInterval := sseHeartbeatInterval
	sseHeartbeatInterval = 25 * time.Millisecond
	defer func() { sseHeartbeatInterval = prevInterval }()

	server := httptest.NewServer(s.Handler())
	defer server.Close()
	streamCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(streamCtx, http.MethodGet, server.URL+"/api/v1/scans/"+scan.ID+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	sawConnected, sawPing := false, false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "event: connected" {
			sawConnected = true
		}
		if line == ": ping" {
			sawPing = true
			break
		}
	}
	if !sawConnected || !sawPing {
		t.Fatalf("stream must open then heartbeat: connected=%v ping=%v", sawConnected, sawPing)
	}
}

func TestWorkspaceRiskScoresLatestCompletedScan(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Risky"})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Pristine"})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces/"+empty.ID+"/risk", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var body map[string]any
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil || body["available"] != false {
		t.Fatalf("workspace without scans must report unavailable risk: %d %s", response.Code, response.Body.String())
	}

	older := completedScan(t, s, work.ID, "completed",
		finding("semgrep", "c1", "a.py", "crit", analyzers.SeverityCritical, analyzers.CategorySecurity),
		finding("semgrep", "c2", "b.py", "crit", analyzers.SeverityCritical, analyzers.CategorySecurity),
		finding("ruff", "h1", "c.py", "high", analyzers.SeverityHigh, analyzers.CategoryCorrectness))
	newer := completedScan(t, s, work.ID, "completed_with_warnings",
		finding("ruff", "h2", "d.py", "high", analyzers.SeverityHigh, analyzers.CategoryCorrectness))

	type riskResponse struct {
		Available     bool           `json:"available"`
		ScanID        string         `json:"scan_id"`
		Score         float64        `json:"score"`
		Grade         string         `json:"grade"`
		Trend         string         `json:"trend"`
		PreviousScore float64        `json:"previous_score"`
		PreviousID    string         `json:"previous_scan_id"`
		Counts        map[string]int `json:"counts"`
	}
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces/"+work.ID+"/risk", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
		t.Fatalf("risk endpoint: %d %s", response.Code, response.Body.String())
	}
	raw, _ := json.Marshal(body)
	var risk riskResponse
	if err := json.Unmarshal(raw, &risk); err != nil {
		t.Fatalf("decode risk body: %v", err)
	}
	if !risk.Available || risk.ScanID != newer.ID {
		t.Fatalf("risk must read the newest completed scan: %#v", risk)
	}
	if risk.Score != 5 || risk.PreviousScore != 25 {
		t.Fatalf("scores: latest=%v previous=%v, want 5 and 25", risk.Score, risk.PreviousScore)
	}
	if risk.Grade != "B" || risk.Trend != "down" || risk.PreviousID != older.ID {
		t.Fatalf("grade/trend/previous mismatch: %#v", risk)
	}
	if risk.Counts["critical"] != 0 || risk.Counts["high"] != 1 {
		t.Fatalf("counts must mirror the latest scan only: %#v", risk.Counts)
	}

	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/workspaces/not-a-real-id/risk", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown workspace must 404: %d", response.Code)
	}
}

func TestPruneWorkspaceScansEndpoint(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Retained"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		completedScan(t, s, work.ID, "completed", finding("ruff", "R", "src/x.py", "issue", analyzers.SeverityLow, analyzers.CategoryStyle))
	}

	request := httptest.NewRequest(http.MethodDelete, "http://127.0.0.1/api/v1/workspaces/"+work.ID+"/scans?keep=0", nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("keep=0 must be rejected: %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodDelete, "http://127.0.0.1/api/v1/workspaces/"+work.ID+"/scans?keep=2", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var body struct {
		Deleted int64 `json:"deleted"`
		Kept    int   `json:"kept"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
		t.Fatalf("prune: %d %s", response.Code, response.Body.String())
	}
	if body.Deleted != 2 || body.Kept != 2 {
		t.Fatalf("prune deleted=%d kept=%d, want 2 and 2", body.Deleted, body.Kept)
	}
	var remaining int
	if err := s.db.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM scans WHERE workspace_id=?`, work.ID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("workspace must retain exactly the newest 2 scans, got %d", remaining)
	}
}

func TestScanCompareEndpointDiffsAgainstPreviousOrExplicitScan(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	work, err := s.db.CreateWorkspace(ctx, core.Workspace{RootPath: t.TempDir(), Name: "Diffed"})
	if err != nil {
		t.Fatal(err)
	}
	persistent := finding("ruff", "P1", "src/p.py", "still here", analyzers.SeverityMedium, analyzers.CategoryCorrectness)
	fixed := finding("ruff", "F1", "src/f.py", "now gone", analyzers.SeverityHigh, analyzers.CategorySecurity)
	fresh := finding("ruff", "N1", ".env", "new leak", analyzers.SeverityCritical, analyzers.CategorySecurity)
	first := completedScan(t, s, work.ID, "completed", persistent, fixed)
	second := completedScan(t, s, work.ID, "completed", persistent, fresh)

	get := func(url string) map[string]any {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, url, nil)
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("compare %q: %d %s", url, response.Code, response.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return body
	}

	body := get("http://127.0.0.1/api/v1/scans/" + second.ID + "/compare")
	if body["available"] != true || body["previous_scan_id"] != first.ID {
		t.Fatalf("implicit previous resolution: %#v", body["available"])
	}
	summary := body["summary"].(map[string]any)
	if summary["new"].(float64) != 1 || summary["fixed"].(float64) != 1 || summary["persistent"].(float64) != 1 {
		t.Fatalf("diff summary: %#v", summary)
	}

	body = get("http://127.0.0.1/api/v1/scans/" + second.ID + "/compare?with=" + first.ID)
	if body["available"] != true || body["previous_scan_id"] != first.ID {
		t.Fatalf("explicit with= resolution: %#v", body)
	}

	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+first.ID+"/compare?with="+second.ID, nil)
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	var reversed map[string]any
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &reversed) != nil {
		t.Fatalf("reversed compare: %d %s", response.Code, response.Body.String())
	}
	rsum := reversed["summary"].(map[string]any)
	if rsum["fixed"].(float64) != 1 { // fresh from second now counts as fixed when diffing backwards
		t.Fatalf("reversed diff summary: %#v", rsum)
	}

	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/scans/"+second.ID+"/compare?with=nope", nil)
	response = httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid with id must 400: %d", response.Code)
	}
}

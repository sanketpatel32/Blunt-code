// finding handlers handlers.
package api

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"bluntcode/internal/database"
	"bluntcode/internal/reports"
	"bluntcode/internal/workspace"
)

// searchFindings serves the cross-workspace findings search. Every control is
// optional; absent values fall back to their defaults (page 1, page_size
// DefaultSearchPageSize), and the response echoes the applied values plus a
// severity_counts facet map computed over the same filtered population.
func (s *Server) searchFindings(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filter := database.GlobalFindingsFilter{
		Page:        1,
		PageSize:    database.DefaultSearchPageSize,
		Query:       strings.TrimSpace(query.Get("q")),
		Analyzer:    strings.TrimSpace(query.Get("analyzer")),
		WorkspaceID: strings.TrimSpace(query.Get("workspace_id")),
	}
	// A q carrying NUL or other hostile control bytes would silently disable
	// the text filter downstream (SQLite drops the bytes); reject it instead
	// so callers never mistake a broken filter for an empty result.
	if strings.ContainsFunc(filter.Query, isHostileControl) {
		fail(w, 400, "INVALID_SEARCH_QUERY", "q must not contain control characters.")
		return
	}
	if raw := strings.TrimSpace(query.Get("severity")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			if value := strings.ToLower(strings.TrimSpace(part)); value != "" {
				filter.Severities = append(filter.Severities, value)
			}
		}
	}
	filter.IncludeSuppressed = strings.EqualFold(strings.TrimSpace(query.Get("include_suppressed")), "true")
	// scope=latest (default) searches each workspace's newest completed scan;
	// scope=all keeps the full history, one hit per scan.
	scope := strings.ToLower(strings.TrimSpace(query.Get("scope")))
	switch scope {
	case "", "latest":
		scope = "latest"
	case "all":
		filter.AllScans = true
	default:
		fail(w, 400, "INVALID_SEARCH_QUERY", "scope must be latest or all.")
		return
	}
	if raw := strings.TrimSpace(query.Get("page")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			fail(w, 400, "INVALID_SEARCH_QUERY", "page must be a positive integer.")
			return
		}
		filter.Page = value
	}
	if raw := strings.TrimSpace(query.Get("page_size")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > database.MaxSearchPageSize {
			fail(w, 400, "INVALID_SEARCH_QUERY", fmt.Sprintf("page_size must be between 1 and %d.", database.MaxSearchPageSize))
			return
		}
		filter.PageSize = value
	}
	items, total, err := s.db.SearchFindings(r.Context(), filter)
	if err != nil {
		fail(w, 400, "INVALID_SEARCH_QUERY", err.Error())
		return
	}
	severityCounts, err := s.db.SearchFindingsSeverityCounts(r.Context(), filter)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not count findings by severity.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": filter.Page, "page_size": filter.PageSize, "has_next": (filter.Page-1)*filter.PageSize+len(items) < total, "severity_counts": severityCounts, "scope": scope})
}

func (s *Server) findings(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if !validID(id) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	scan, err := s.db.Scan(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load scan.")
		return
	}
	filter, err := findingFilter(r)
	if err != nil {
		fail(w, 400, "INVALID_FINDING_QUERY", err.Error())
		return
	}
	page, err := s.db.FindingsPage(r.Context(), scan, filter)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load findings.")
		return
	}
	// Requests without the page/page_size params keep the original envelope
	// byte-for-byte; page-mode responses add page, page_size, and has_next on
	// top of the legacy fields so both client generations can read them.
	if filter.PageSize > 0 {
		body := map[string]any{"items": page.Items, "total": page.Total, "limit": page.Limit, "offset": page.Offset, "next_offset": page.NextOffset, "has_more": page.HasMore, "page": filter.Page, "page_size": filter.PageSize, "has_next": page.HasMore}
		// The first page also carries per-status totals under the same
		// non-status filters, so the status chips count suppressed findings
		// (which the report payload leaves out) without extra requests.
		if filter.Page <= 1 {
			if counts, err := s.db.FindingStatusCounts(r.Context(), scan, filter); err == nil {
				body["status_counts"] = counts
			}
			// ...and per-category / per-analyzer totals for the report charts.
			if facets, err := s.db.FindingFacets(r.Context(), scan, filter); err == nil {
				body["facets"] = facets
			}
		}
		writeJSON(w, 200, body)
		return
	}
	writeJSON(w, 200, map[string]any{"items": page.Items, "total": page.Total, "limit": page.Limit, "offset": page.Offset, "next_offset": page.NextOffset, "has_more": page.HasMore})
}

// csvCell neutralizes CSV formula injection (CWE-1236) and strips hostile
// control bytes. Findings text is derived from UNTRUSTED scanned code in two
// senses:
//
//   - A hostile lint message such as =cmd|'/c calc'!A1 would execute as a
//     formula the moment the exported file is opened in Excel or LibreOffice.
//     Excel-family spreadsheets treat a cell whose first character is =, +, -,
//     @, tab, or carriage return as a formula, so those cells are prefixed
//     with a single quote, the standard spreadsheet escape that forces text
//     interpretation.
//   - Non-whitespace C0 controls (NUL, BEL, ANSI escape sequences, ...) and
//     DEL are replaced with spaces; encoding/csv passes them through raw and
//     they corrupt terminals and pagers the export is piped into. Tab, LF,
//     and CR are kept because the CSV writer quotes them and they round-trip
//     exactly.
//
// These are the ONLY transformations csvCell performs. Every analyzer-derived
// text column routes through csvCell; the numeric columns come from strconv
// and cannot begin with a formula character.
func csvCell(value string) string {
	value = csvScrubControls(value)
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	}
	return value
}

// isHostileControl reports whether a rune is a C0 control (other than the
// trio tab, LF, CR, which encoding/csv quotes and round-trips) or DEL.
func isHostileControl(r rune) bool {
	return (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0x7f
}

// csvScrubControls replaces non-whitespace C0 controls and DEL with spaces so
// no raw NUL or terminal escape byte reaches the export, and rewrites invalid
// UTF-8 bytes as U+FFFD so the file is always valid UTF-8. It is the identity
// for strings that contain none of these.
func csvScrubControls(value string) string {
	if !strings.ContainsFunc(value, isHostileControl) {
		return value
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if isHostileControl(r) {
			r = ' '
		}
		b.WriteRune(r)
	}
	return b.String()
}

// findingsCSV exports the scan's findings as a spreadsheet-friendly CSV
// attachment. It shares the JSON endpoint's filter parsing so an export always
// matches what the findings list shows; limit and offset are ignored because
// every matching row is written, up to the repository safety cap.
//
// Record terminators are bare LF, not RFC 4180's CRLF, on purpose:
// encoding/csv's UseCRLF mode rewrites every CR inside a quoted field as part
// of a CRLF pair, which deletes lone CR bytes that analyzer output can carry
// in messages and paths (verified against the standard library). Field bytes
// stay verbatim in the file with LF terminators, and every CSV consumer
// (encoding/csv, Excel, LibreOffice, Python) accepts LF-terminated records.
func (s *Server) findingsCSV(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if !validID(id) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	scan, err := s.db.Scan(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load scan.")
		return
	}
	filter, err := findingQueryFilter(r)
	if err != nil {
		fail(w, 400, "INVALID_FINDING_QUERY", err.Error())
		return
	}
	findings, err := s.db.FindingsCSV(r.Context(), scan, filter)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load findings.")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="bluntcode-scan-%s-findings.csv"`, shortID(scan.ID)))
	_, _ = w.Write([]byte(csvBOM))
	writer := csv.NewWriter(w)
	if err := writer.Write(findingsCSVHeader); err != nil {
		s.log.Warn("csv header write failed", "error", err)
		return
	}
	for _, f := range findings {
		if err := writer.Write([]string{
			csvCell(string(f.Severity)), csvCell(string(f.Category)), csvCell(f.AnalyzerID), csvCell(f.RuleID), csvCell(f.Title), csvCell(f.Message), csvCell(f.RelativePath),
			csvNumber(f.StartLine), csvNumber(f.StartColumn), csvNumber(f.EndLine), csvCell(f.Status), csvCell(f.Remediation), csvCell(f.DocumentationURL),
		}); err != nil {
			s.log.Warn("csv row write failed", "error", err)
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		s.log.Warn("csv flush failed", "error", err)
	}
}

// csvNumber renders optional finding positions; zero means "not stored" and
// exports as an empty cell.
func csvNumber(value int) string {
	if value == 0 {
		return ""
	}
	return strconv.Itoa(value)
}

// findingsJSON exports the scan as the full versioned JSON report document
// (the same document `bluntcode scan --format json` prints). It follows the
// findings.csv handler's shape â€” an attachment regenerated from stored scan
// data â€” but serves the complete report model: severity counts, analyzer
// runs, metrics, and every finding field the other exports surface. Query
// filters do not apply; the document is always the whole scan.
func (s *Server) findingsJSON(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if !validID(id) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	scan, err := s.db.Scan(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load scan.")
		return
	}
	work, err := s.db.Workspace(r.Context(), scan.WorkspaceID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load report.")
		return
	}
	model, err := s.reportModel(r.Context(), scan, work)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load report.")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	reportDisposition(w, r, fmt.Sprintf("bluntcode-scan-%s-findings.json", shortID(scan.ID)), "")
	_, _ = w.Write(reports.JSON(model))
}

// findingsJSONL exports the scan as newline-delimited JSON (reports.JSONL):
// one self-contained finding object per line for log pipelines and jq-style
// consumers. Like findings.json it is unfiltered — query filters do not
// apply, and suppressed fingerprints never enter the report model — so the
// download always covers the whole scan.
func (s *Server) findingsJSONL(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if !validID(id) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	scan, err := s.db.Scan(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load scan.")
		return
	}
	work, err := s.db.Workspace(r.Context(), scan.WorkspaceID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load report.")
		return
	}
	model, err := s.reportModel(r.Context(), scan, work)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load report.")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="bluntcode-scan-%s-findings.jsonl"`, shortID(scan.ID)))
	_, _ = w.Write(reports.JSONL(model))
}

// findingPreview returns a small, current-source excerpt. The stored path is
// validated again so a finding can never be used to read outside its workspace.
func (s *Server) findingPreview(w http.ResponseWriter, r *http.Request) {
	scanID, findingID := pathID(r), strings.ToLower(r.PathValue("findingID"))
	if !validID(scanID) || !validID(findingID) {
		fail(w, 404, "FINDING_NOT_FOUND", "Finding was not found.")
		return
	}
	scan, err := s.db.Scan(r.Context(), scanID)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load scan.")
		return
	}
	finding, err := s.db.Finding(r.Context(), scanID, findingID)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "FINDING_NOT_FOUND", "Finding was not found.")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load finding.")
		return
	}
	work, err := s.db.Workspace(r.Context(), scan.WorkspaceID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load workspace.")
		return
	}
	if s.failRootUnavailable(w, work) {
		return
	}
	relative, err := workspace.ValidateRelativePath(work.RootPath, filepath.FromSlash(finding.RelativePath))
	if err != nil {
		fail(w, 422, "SOURCE_PATH_UNAVAILABLE", "The finding path is no longer available inside this workspace.")
		return
	}
	path := filepath.Join(work.RootPath, relative)
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		fail(w, 404, "SOURCE_FILE_NOT_FOUND", "This source file no longer exists.")
		return
	}
	if err != nil {
		fail(w, 422, "SOURCE_PATH_UNAVAILABLE", "This source file cannot be opened.")
		return
	}
	if !info.Mode().IsRegular() {
		fail(w, 422, "SOURCE_NOT_A_FILE", "Only source files can be previewed.")
		return
	}
	if info.Size() > maxPreviewFileBytes {
		fail(w, 413, "SOURCE_FILE_TOO_LARGE", "This source file is too large to preview.")
		return
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		fail(w, 422, "SOURCE_PATH_UNAVAILABLE", "This source file cannot be opened.")
		return
	}
	lines := strings.Split(strings.ReplaceAll(string(contents), "\r\n", "\n"), "\n")
	start, end, note := finding.StartLine, finding.EndLine, ""
	if start < 1 || start > len(lines) {
		start, end = 0, 0
		note = "This finding has no saved location; showing the start of the current file."
	} else if end < start || end > len(lines) {
		end = start
	}
	first := 1
	if start > 0 {
		first = start - 6
		if first < 1 {
			first = 1
		}
	}
	last := first + 17
	if end > 0 && end+12 > last {
		last = end + 12
	}
	if last > len(lines) {
		last = len(lines)
	}
	preview := sourcePreview{Path: filepath.ToSlash(relative), HighlightStartLine: start, HighlightEndLine: end, Note: note, Lines: make([]sourcePreviewLine, 0, last-first+1)}
	for index := first - 1; index < last; index++ {
		preview.Lines = append(preview.Lines, sourcePreviewLine{Number: index + 1, Text: strings.TrimSuffix(lines[index], "\r")})
	}
	writeJSON(w, http.StatusOK, preview)
}

// findingQueryFilter parses the finding controls shared by the JSON list and
// the CSV export: severity (single value or comma list), category, analyzer,
// rule, path (substring), path_prefix (normalized prefix), status (single
// value or comma list), q, sort, and order. sort accepts an optional leading
// "-" for descending (sort=-severity), equivalent to order=desc; an explicit
// order parameter wins over the prefix. Both endpoints reject the same bad
// input because they parse through this one function.
func findingQueryFilter(r *http.Request) (database.FindingFilter, error) {
	query := r.URL.Query()
	read := func(name string) string { return strings.TrimSpace(query.Get(name)) }
	sortParam := strings.ToLower(read("sort"))
	orderParam := strings.ToLower(read("order"))
	descending := strings.HasPrefix(sortParam, "-")
	if descending {
		sortParam = sortParam[1:]
	}
	filter := database.FindingFilter{
		Severity:   strings.ToLower(read("severity")),
		Category:   strings.ToLower(read("category")),
		Analyzer:   strings.ToLower(read("analyzer")),
		Rule:       read("rule"),
		Path:       read("path"),
		PathPrefix: read("path_prefix"),
		Status:     strings.ToLower(read("status")),
		Query:      read("q"),
		Sort:       sortParam,
		Order:      orderParam,
	}
	if descending && filter.Order == "" {
		filter.Order = "desc"
	}
	if filter.Severity != "" {
		for _, value := range strings.Split(filter.Severity, ",") {
			value = strings.TrimSpace(value)
			if !oneOf(value, "critical", "high", "medium", "low", "info") {
				return filter, fmt.Errorf("severity must be critical, high, medium, low, or info")
			}
			filter.Severities = append(filter.Severities, value)
		}
	}
	if filter.Status != "" {
		for _, value := range strings.Split(filter.Status, ",") {
			value = strings.TrimSpace(value)
			if !oneOf(value, "new", "persistent", "suppressed") {
				return filter, fmt.Errorf("status must be new, persistent, or suppressed")
			}
			filter.Statuses = append(filter.Statuses, value)
		}
	}
	// A bare "-" prefix (or "-" alone) is not a field; the oneOf check covers
	// it because descending forces validation even of an empty remainder.
	if filter.Sort != "" || descending {
		if !oneOf(filter.Sort, "severity", "path", "line", "rule", "analyzer", "status") {
			return filter, fmt.Errorf("sort must be severity, path, line, rule, analyzer, or status")
		}
	}
	if filter.Order != "" && !oneOf(filter.Order, "asc", "desc") {
		return filter, fmt.Errorf("order must be asc or desc")
	}
	return filter, nil
}

// findingFilter extends the shared controls with the JSON list's pagination.
// Two modes exist and are mutually exclusive: the original limit (25/50/100,
// default 50) plus offset pair, and the page-based page (1-based) plus
// page_size (1..MaxFindingsPageSize, default 50) pair. Combining them is
// rejected because the window each implies would be ambiguous.
func findingFilter(r *http.Request) (database.FindingFilter, error) {
	filter, err := findingQueryFilter(r)
	if err != nil {
		return filter, err
	}
	query := r.URL.Query()
	pageParam := strings.TrimSpace(query.Get("page"))
	pageSizeParam := strings.TrimSpace(query.Get("page_size"))
	if pageParam == "" && pageSizeParam == "" {
		filter.Limit = 50
		if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil {
				return filter, fmt.Errorf("limit must be 25, 50, or 100")
			}
			filter.Limit = value
		}
		if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 0 {
				return filter, fmt.Errorf("offset must be a non-negative integer")
			}
			filter.Offset = value
		}
		if !oneOf(strconv.Itoa(filter.Limit), "25", "50", "100") {
			return filter, fmt.Errorf("limit must be 25, 50, or 100")
		}
		return filter, nil
	}
	if strings.TrimSpace(query.Get("limit")) != "" || strings.TrimSpace(query.Get("offset")) != "" {
		return filter, fmt.Errorf("limit and offset cannot be combined with page and page_size")
	}
	page := 1
	if pageParam != "" {
		value, err := strconv.Atoi(pageParam)
		if err != nil || value < 1 {
			return filter, fmt.Errorf("page must be a positive integer")
		}
		page = value
	}
	pageSize := database.DefaultFindingsPageSize
	if pageSizeParam != "" {
		value, err := strconv.Atoi(pageSizeParam)
		if err != nil || value < 1 || value > database.MaxFindingsPageSize {
			return filter, fmt.Errorf("page_size must be between 1 and %d", database.MaxFindingsPageSize)
		}
		pageSize = value
	}
	filter.Page, filter.PageSize = page, pageSize
	return filter, nil
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

// findings repo.
package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
)

// FindingIdentities returns one scan's findings with only the fields a
// new/fixed comparison reads (fingerprint, analyzer, path). Dashboard cards
// need just the counts, so this skips the message, metadata JSON decode, and
// ORDER BY that Findings pays for every row.
func (d *DB) FindingIdentities(ctx context.Context, scanID string) ([]analyzers.Finding, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT fingerprint,analyzer_id,COALESCE(relative_path,'') FROM findings WHERE scan_id=?`, scanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	output := make([]analyzers.Finding, 0)
	for rows.Next() {
		var f analyzers.Finding
		if err := rows.Scan(&f.Fingerprint, &f.AnalyzerID, &f.RelativePath); err != nil {
			return nil, err
		}
		output = append(output, f)
	}
	return output, rows.Err()
}

// Finding returns one finding only when it belongs to the supplied scan.
func (d *DB) Finding(ctx context.Context, scanID, id string) (analyzers.Finding, error) {
	var f analyzers.Finding
	var severity, category, metadata string
	err := d.SQL.QueryRowContext(ctx, `SELECT id,analyzer_id,COALESCE(rule_id,''),fingerprint,severity,category,COALESCE(title,''),message,COALESCE(relative_path,''),COALESCE(start_line,0),COALESCE(start_column,0),COALESCE(end_line,0),COALESCE(end_column,0),COALESCE(remediation,''),COALESCE(documentation_url,''),COALESCE(raw_severity,''),metadata_json FROM findings WHERE scan_id=? AND id=?`, scanID, id).Scan(&f.ID, &f.AnalyzerID, &f.RuleID, &f.Fingerprint, &severity, &category, &f.Title, &f.Message, &f.RelativePath, &f.StartLine, &f.StartColumn, &f.EndLine, &f.EndColumn, &f.Remediation, &f.DocumentationURL, &f.RawSeverity, &metadata)
	if err != nil {
		return analyzers.Finding{}, err
	}
	f.Severity = analyzers.Severity(severity)
	f.Category = analyzers.Category(category)
	if err := json.Unmarshal([]byte(metadata), &f.Metadata); err != nil {
		// Corrupt metadata is not fatal to the row, but it must not be silent.
		log.Printf("finding %s: corrupt metadata_json replaced with empty object: %v", f.ID, err)
	}
	return f, nil
}

// FindingFilter contains only server-side finding query controls. Text values
// are matched case-insensitively as literal substrings; they are never used as
// SQL syntax.
type FindingFilter struct {
	Severity string
	// Severities selects several severities at once (severity=high,critical).
	// When non-empty it takes precedence over the single-value Severity field;
	// a one-element list compiles to the same SQL as Severity.
	Severities []string
	Category   string
	Analyzer   string
	// Rule matches rule_id exactly (case-insensitively), like Analyzer.
	Rule string
	Path string
	// PathPrefix matches workspace-relative paths by normalized prefix (both
	// slash styles accepted, case-insensitive). Path keeps its substring
	// semantics for backward compatibility.
	PathPrefix string
	Status     string   // "new", "persistent", or "suppressed"
	Statuses   []string // multi-value Status; takes precedence when non-empty
	Query      string
	Sort       string // severity, path, line, rule, analyzer, or status
	Order      string // asc or desc
	Limit      int
	Offset     int
	// Page and PageSize enable page-based pagination: when PageSize > 0 the
	// 1-based page window replaces the legacy limit/offset pair and Limit and
	// Offset are derived from it.
	Page     int
	PageSize int
	// ExcludeSuppressed hides findings whose fingerprint is suppressed for the
	// workspace unless Status explicitly selects them. The UI list leaves it
	// off so dismissed findings stay visible with their status; exports turn
	// it on so no artifact carries a dismissed finding.
	ExcludeSuppressed bool
}

type FindingsPage struct {
	Items      []analyzers.Finding
	Total      int
	Limit      int
	Offset     int
	NextOffset *int
	HasMore    bool
}

const (
	// DefaultFindingsPageSize and MaxFindingsPageSize bound the page/page_size
	// pagination mode of the findings list. Unlike the fixed legacy limit set
	// (25/50/100), any page size from 1 to MaxFindingsPageSize is allowed so
	// API clients can pick their own window.
	DefaultFindingsPageSize = 50
	MaxFindingsPageSize     = 200
)

// MaxFindingsExportRows bounds the CSV export: every matching row is written,
// but a pathological scan can never drag more than this many findings into
// memory or the HTTP response at once.
const MaxFindingsExportRows = 50000

// findingColumns is the projection shared by every filtered findings query;
// nullable text is coalesced so row scans never observe SQL NULLs.
const findingColumns = `findings.id,findings.analyzer_id,COALESCE(findings.rule_id,''),findings.fingerprint,findings.severity,findings.category,COALESCE(findings.title,''),findings.message,COALESCE(findings.relative_path,''),COALESCE(findings.start_line,0),COALESCE(findings.start_column,0),COALESCE(findings.end_line,0),COALESCE(findings.end_column,0),COALESCE(findings.remediation,''),COALESCE(findings.documentation_url,''),COALESCE(findings.raw_severity,''),findings.metadata_json`

// severityRankExpr maps severity names onto a descending sort rank
// (critical=5 down to info=1). The findings list and the fixed-findings panel
// share it so both views rank issues identically.
const severityRankExpr = `CASE findings.severity WHEN 'critical' THEN 5 WHEN 'high' THEN 4 WHEN 'medium' THEN 3 WHEN 'low' THEN 2 ELSE 1 END`

// suppressedStatusExpr is the EXISTS probe the status CASE uses to decide
// whether a row is suppressed. Its single workspace placeholder always binds
// before any other statusExpr placeholder.
const suppressedStatusExpr = `EXISTS (SELECT 1 FROM suppressed_findings suppressed WHERE suppressed.workspace_id=? AND suppressed.fingerprint=findings.fingerprint)`

// suppressedNotInExpr is the WHERE-clause form of the suppression exclusion.
// The subquery materializes the workspace's set once, which benchmarks faster
// than a correlated per-row probe on large scans (findings.fingerprint and
// suppressed_findings.fingerprint are both NOT NULL, so NOT IN is safe).
const suppressedNotInExpr = `findings.fingerprint NOT IN (SELECT fingerprint FROM suppressed_findings WHERE workspace_id=?)`

// findingQuery carries the filter, ordering, and comparison-status machinery
// shared by FindingsPage and FindingsCSV so the paged JSON list and the CSV
// export always agree on which rows match.
type findingQuery struct {
	previousID  string
	workspaceID string
	statusExpr  string
	whereSQL    string
	args        []any
	order       string
	orderArgs   []any
}

// buildFindingQuery validates the controls shared by both findings endpoints,
// resolves the previous completed scan for comparison status, and assembles
// the WHERE and ORDER BY clauses. Limit and offset stay the paged caller's
// responsibility. The computed status is a three-way CASE: suppressed wins
// over persistent, so a dismissed fingerprint never reports new/persistent.
func (d *DB) buildFindingQuery(ctx context.Context, scan core.Scan, filter FindingFilter) (findingQuery, error) {
	// Multi-value controls resolve to the effective list here: a single value
	// keeps the original one-placeholder SQL so legacy requests compile to
	// exactly the same statement as before the list fields existed.
	statuses := filter.Statuses
	if len(statuses) == 0 && filter.Status != "" {
		statuses = []string{filter.Status}
	}
	for _, status := range statuses {
		if status != "new" && status != "persistent" && status != "suppressed" {
			return findingQuery{}, fmt.Errorf("status must be new, persistent, or suppressed")
		}
	}
	for _, severity := range filter.Severities {
		if severity != "critical" && severity != "high" && severity != "medium" && severity != "low" && severity != "info" {
			return findingQuery{}, fmt.Errorf("severity must be critical, high, medium, low, or info")
		}
	}
	if filter.Sort != "" && filter.Sort != "severity" && filter.Sort != "path" && filter.Sort != "line" && filter.Sort != "rule" && filter.Sort != "analyzer" && filter.Sort != "status" {
		return findingQuery{}, fmt.Errorf("sort must be severity, path, line, rule, analyzer, or status")
	}
	if filter.Order != "" && filter.Order != "asc" && filter.Order != "desc" {
		return findingQuery{}, fmt.Errorf("order must be asc or desc")
	}
	previousID, err := d.PreviousCompletedScanID(ctx, scan.WorkspaceID, scan.ID)
	if errors.Is(err, sql.ErrNoRows) {
		previousID = ""
	} else if err != nil {
		return findingQuery{}, err
	}

	// statusExpr is deliberately repeated in the SELECT/WHERE/ORDER clauses so
	// SQLite can filter before paging without transferring a full finding set.
	statusExpr := `CASE WHEN ` + suppressedStatusExpr + ` THEN 'suppressed' WHEN EXISTS (SELECT 1 FROM findings previous WHERE previous.scan_id=? AND previous.fingerprint=findings.fingerprint) THEN 'persistent' ELSE 'new' END`
	where := []string{"findings.scan_id=?"}
	args := []any{scan.ID}
	if filter.ExcludeSuppressed && filter.Status != "suppressed" && !hasStatus(statuses, "suppressed") {
		where = append(where, suppressedNotInExpr)
		args = append(args, scan.WorkspaceID)
	}
	addEquals := func(column, value string) {
		if value != "" {
			where = append(where, "LOWER("+column+")=LOWER(?)")
			args = append(args, value)
		}
	}
	if severities := resolveSeverities(filter); len(severities) == 1 {
		addEquals("findings.severity", severities[0])
	} else if len(severities) > 1 {
		placeholders := make([]string, len(severities))
		for i, severity := range severities {
			placeholders[i] = "LOWER(?)"
			args = append(args, severity)
		}
		where = append(where, "LOWER(findings.severity) IN ("+strings.Join(placeholders, ",")+")")
	}
	addEquals("findings.category", filter.Category)
	addEquals("findings.analyzer_id", filter.Analyzer)
	addEquals("COALESCE(findings.rule_id,'')", filter.Rule)
	if filter.Path != "" {
		// pathPrefixEscaper + the ESCAPE clause keep % and _ literal here, the
		// same contract the prefix filter below honors; a substring path
		// search is a text match, never a wildcard expression.
		where = append(where, `LOWER(COALESCE(findings.relative_path,'')) LIKE '%' || LOWER(?) || '%' ESCAPE '\'`)
		args = append(args, pathPrefixEscaper.Replace(filter.Path))
	}
	if prefix := normalizePathPrefix(filter.PathPrefix); prefix != "" {
		where = append(where, "LOWER(COALESCE(findings.relative_path,'')) LIKE ? ESCAPE '\\'")
		args = append(args, likePatternPrefix(prefix))
	}
	if filter.Query != "" {
		escaped := pathPrefixEscaper.Replace(filter.Query)
		where = append(where, `(LOWER(findings.message) LIKE '%' || LOWER(?) || '%' ESCAPE '\' OR LOWER(COALESCE(findings.rule_id,'')) LIKE '%' || LOWER(?) || '%' ESCAPE '\' OR LOWER(COALESCE(findings.relative_path,'')) LIKE '%' || LOWER(?) || '%' ESCAPE '\')`)
		args = append(args, escaped, escaped, escaped)
	}
	if len(statuses) == 1 {
		where = append(where, statusExpr+"=?")
		args = append(args, scan.WorkspaceID, previousID, statuses[0])
	} else if len(statuses) > 1 {
		placeholders := make([]string, len(statuses))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		where = append(where, statusExpr+" IN ("+strings.Join(placeholders, ",")+")")
		args = append(args, scan.WorkspaceID, previousID)
		for _, status := range statuses {
			args = append(args, status)
		}
	}
	order, orderArgs := "findings.relative_path ASC, findings.start_line ASC, findings.analyzer_id ASC, findings.rule_id ASC", []any(nil)
	direction := "ASC"
	if filter.Order == "desc" {
		direction = "DESC"
		if filter.Sort == "" {
			order = "findings.relative_path DESC, findings.start_line DESC, findings.analyzer_id ASC, findings.rule_id ASC"
		}
	}
	switch filter.Sort {
	case "severity":
		order = severityRankExpr + " " + direction + ", findings.relative_path ASC, findings.start_line ASC"
	case "path":
		order = "findings.relative_path " + direction + ", findings.start_line " + direction + ", findings.analyzer_id ASC"
	case "line":
		order = "COALESCE(findings.start_line,0) " + direction + ", findings.relative_path ASC, findings.analyzer_id ASC, findings.rule_id ASC"
	case "rule":
		order = "COALESCE(findings.rule_id,'') " + direction + ", findings.relative_path ASC, COALESCE(findings.start_line,0) ASC"
	case "analyzer":
		order = "findings.analyzer_id " + direction + ", findings.relative_path ASC, findings.start_line ASC"
	case "status":
		order = statusExpr + " " + direction + ", findings.relative_path ASC, findings.start_line ASC"
		orderArgs = []any{scan.WorkspaceID, previousID}
	}
	return findingQuery{previousID: previousID, workspaceID: scan.WorkspaceID, statusExpr: statusExpr, whereSQL: strings.Join(where, " AND "), args: args, order: order, orderArgs: orderArgs}, nil
}

// resolveSeverities returns the effective severity list: the multi-value field
// wins, a legacy single value becomes a one-element list, and an empty filter
// stays empty.
func resolveSeverities(filter FindingFilter) []string {
	if len(filter.Severities) > 0 {
		return filter.Severities
	}
	if filter.Severity != "" {
		return []string{filter.Severity}
	}
	return nil
}

// hasStatus reports whether the resolved status list selects suppressed rows.
func hasStatus(statuses []string, status string) bool {
	for _, candidate := range statuses {
		if candidate == status {
			return true
		}
	}
	return false
}

// pathPrefixEscaper neutralizes SQL LIKE wildcards inside a path prefix so a
// prefix containing % or _ matches those characters literally; the companion
// ESCAPE '\' clause in the WHERE expression activates it.
var pathPrefixEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// likePatternPrefix builds the bound LIKE pattern for a normalized path
// prefix: wildcards escaped, lowercased to match LOWER(relative_path), and
// anchored at the start of the stored path.
func likePatternPrefix(prefix string) string {
	return strings.ToLower(pathPrefixEscaper.Replace(prefix)) + "%"
}

// normalizePathPrefix canonicalizes a workspace-relative path prefix for
// prefix matching: Windows backslashes become forward slashes (both analyzer
// output and user input arrive in either style), leading "./" and "/" are
// dropped because the stored paths are workspace-relative, and surrounding
// whitespace is trimmed. Matching is case-insensitive on Windows-style paths,
// handled by the LOWER() comparison in the query. A trailing slash is a
// meaningful directory boundary and is preserved.
func normalizePathPrefix(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, `\`, "/"))
	for strings.HasPrefix(value, "./") {
		value = value[2:]
	}
	return strings.TrimPrefix(value, "/")
}

// selectArgs orders bind parameters the way the shared SELECT mentions them:
// the computed status column first (suppression workspace, then the previous
// scan), then the WHERE placeholders, then any ORDER BY placeholders. Callers
// append their own LIMIT/OFFSET values.
func (q findingQuery) selectArgs() []any {
	args := []any{q.workspaceID, q.previousID}
	args = append(args, q.args...)
	return append(args, q.orderArgs...)
}

// scanFindingRow decodes one row of the shared filtered-findings SELECT,
// including the computed comparison status.
func scanFindingRow(rows *sql.Rows) (analyzers.Finding, error) {
	var f analyzers.Finding
	var severity, category, metadata string
	if err := rows.Scan(&f.ID, &f.AnalyzerID, &f.RuleID, &f.Fingerprint, &severity, &category, &f.Title, &f.Message, &f.RelativePath, &f.StartLine, &f.StartColumn, &f.EndLine, &f.EndColumn, &f.Remediation, &f.DocumentationURL, &f.RawSeverity, &metadata, &f.Status); err != nil {
		return analyzers.Finding{}, err
	}
	f.Severity = analyzers.Severity(severity)
	f.Category = analyzers.Category(category)
	if err := json.Unmarshal([]byte(metadata), &f.Metadata); err != nil {
		// Corrupt metadata is not fatal to the row, but it must not be silent.
		log.Printf("finding %s: corrupt metadata_json replaced with empty object: %v", f.ID, err)
	}
	return f, nil
}

// FindingsPage returns a bounded page and applies comparison status relative
// to the previous completed scan in the same workspace. Pagination has two
// modes: the legacy limit (25/50/100) plus offset pair, and the page-based
// mode enabled by PageSize > 0 (1-based Page, any size up to
// MaxFindingsPageSize), which derives limit and offset from the page window.
// FindingStatusCounts counts a scan's findings per comparison status (new,
// persistent, suppressed) under the filter's non-status controls, so the
// report's status chips show real totals, suppressed included.
func (d *DB) FindingStatusCounts(ctx context.Context, scan core.Scan, filter FindingFilter) (map[string]int, error) {
	filter.Status, filter.Statuses, filter.ExcludeSuppressed = "", nil, false
	filter.Sort, filter.Order = "", ""
	query, err := d.buildFindingQuery(ctx, scan, filter)
	if err != nil {
		return nil, err
	}
	args := []any{query.workspaceID, query.previousID}
	args = append(args, query.args...)
	rows, err := d.SQL.QueryContext(ctx, `SELECT `+query.statusExpr+` AS status, COUNT(*) FROM findings WHERE `+query.whereSQL+` GROUP BY status`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{"new": 0, "persistent": 0, "suppressed": 0}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		counts[status] += n
	}
	return counts, rows.Err()
}

func (d *DB) FindingsPage(ctx context.Context, scan core.Scan, filter FindingFilter) (FindingsPage, error) {
	if filter.PageSize > 0 {
		if filter.PageSize > MaxFindingsPageSize {
			return FindingsPage{}, fmt.Errorf("page_size must be between 1 and %d", MaxFindingsPageSize)
		}
		page := filter.Page
		if page < 0 {
			return FindingsPage{}, fmt.Errorf("page must be a positive integer")
		}
		if page == 0 {
			page = 1
		}
		// (page-1)*PageSize must not overflow int; a page that large could
		// never match anyway, so it is a validation error rather than a wrap.
		if page > 1 && page-1 > (math.MaxInt-filter.PageSize)/filter.PageSize {
			return FindingsPage{}, fmt.Errorf("page is out of range")
		}
		filter.Limit = filter.PageSize
		filter.Offset = (page - 1) * filter.PageSize
	} else {
		if filter.Limit == 0 {
			filter.Limit = 50
		}
		if filter.Limit < 1 || filter.Limit > 100 || (filter.Limit != 25 && filter.Limit != 50 && filter.Limit != 100) {
			return FindingsPage{}, fmt.Errorf("limit must be 25, 50, or 100")
		}
		if filter.Offset < 0 {
			return FindingsPage{}, fmt.Errorf("offset must not be negative")
		}
	}
	query, err := d.buildFindingQuery(ctx, scan, filter)
	if err != nil {
		return FindingsPage{}, err
	}
	countArgs := append([]any(nil), query.args...)
	var total int
	if err := d.SQL.QueryRowContext(ctx, "SELECT COUNT(*) FROM findings WHERE "+query.whereSQL, countArgs...).Scan(&total); err != nil {
		return FindingsPage{}, err
	}
	queryArgs := append(query.selectArgs(), filter.Limit, filter.Offset)
	rows, err := d.SQL.QueryContext(ctx, `SELECT `+findingColumns+`,`+query.statusExpr+` FROM findings WHERE `+query.whereSQL+` ORDER BY `+query.order+` LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return FindingsPage{}, err
	}
	defer rows.Close()
	page := FindingsPage{Total: total, Limit: filter.Limit, Offset: filter.Offset, Items: make([]analyzers.Finding, 0)}
	for rows.Next() {
		f, err := scanFindingRow(rows)
		if err != nil {
			return FindingsPage{}, err
		}
		page.Items = append(page.Items, f)
	}
	if err := rows.Err(); err != nil {
		return FindingsPage{}, err
	}
	page.HasMore = page.Offset+len(page.Items) < page.Total
	if page.HasMore {
		next := page.Offset + len(page.Items)
		page.NextOffset = &next
	}
	return page, nil
}

// FindingsCSV returns every finding matching the filter with exactly the same
// matching and ordering rules as FindingsPage, except that suppressed findings
// are excluded unless the filter explicitly selects status=suppressed â€” a
// dismissed finding never ships in an export. Limit and offset are ignored;
// the row count is capped at MaxFindingsExportRows.
func (d *DB) FindingsCSV(ctx context.Context, scan core.Scan, filter FindingFilter) ([]analyzers.Finding, error) {
	filter.ExcludeSuppressed = true
	query, err := d.buildFindingQuery(ctx, scan, filter)
	if err != nil {
		return nil, err
	}
	queryArgs := append(query.selectArgs(), MaxFindingsExportRows)
	rows, err := d.SQL.QueryContext(ctx, `SELECT `+findingColumns+`,`+query.statusExpr+` FROM findings WHERE `+query.whereSQL+` ORDER BY `+query.order+` LIMIT ?`, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var output []analyzers.Finding
	for rows.Next() {
		f, err := scanFindingRow(rows)
		if err != nil {
			return nil, err
		}
		output = append(output, f)
	}
	return output, rows.Err()
}

const (
	// DefaultFixedFindingsLimit and MaxFixedFindingsLimit bound the
	// fixed-findings panel: it is a "what changed" summary, not a full export.
	DefaultFixedFindingsLimit = 100
	MaxFixedFindingsLimit     = 200
)

// FixedFindingsResult carries the fixed-findings panel payload: the capped
// rows, the exact fixed total, and the comparison context that produced it.
type FixedFindingsResult struct {
	Items               []analyzers.Finding
	Total               int
	ComparisonAvailable bool
	PreviousScanID      string
}

// FixedFindings lists the findings that existed in the previous completed scan
// of the same workspace and disappeared from this scan. The coverage rules from
// scans.Compare are applied: a disappeared finding only counts as fixed when
// its analyzer also succeeded in the current scan (SQL) AND — when the caller
// passes the current scan's selected files — its file was actually part of this
// scan's input set (Go filter), because its absence is otherwise unknown, not
// fixed. Rows reuse the shared finding projection and row scanner of
// FindingsPage with the status column pinned to "fixed"; limit defaults to
// DefaultFixedFindingsLimit and is capped at MaxFixedFindingsLimit while Total
// always reports the uncapped, coverage-filtered count.
func (d *DB) FixedFindings(ctx context.Context, scan core.Scan, limit int, selectedRelPaths []string) (FixedFindingsResult, error) {
	if limit <= 0 {
		limit = DefaultFixedFindingsLimit
	}
	if limit > MaxFixedFindingsLimit {
		limit = MaxFixedFindingsLimit
	}
	// Mirrors scans.NewComparisonCoverage: an empty/absent path list leaves the
	// set nil, which falls back to the analyzer-only contract instead of
	// declaring every file out of scope.
	var evaluatedFiles map[string]bool
	for _, path := range selectedRelPaths {
		if path == "" {
			continue
		}
		if evaluatedFiles == nil {
			evaluatedFiles = make(map[string]bool, len(selectedRelPaths))
		}
		evaluatedFiles[core.NormalizePath(path)] = true
	}
	previousID, err := d.PreviousCompletedScanID(ctx, scan.WorkspaceID, scan.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return FixedFindingsResult{Items: []analyzers.Finding{}}, nil
	}
	if err != nil {
		return FixedFindingsResult{}, err
	}
	result := FixedFindingsResult{ComparisonAvailable: true, PreviousScanID: previousID, Items: []analyzers.Finding{}}
	// The fingerprint predicate mirrors the comparison status in
	// buildFindingQuery pointed the other way: previous findings whose
	// fingerprint no longer exists in the current scan are gone. The analyzer
	// subquery is the SQL form of SuccessfulAnalyzerIDs for the coverage rule,
	// and a suppressed fingerprint never reports as fixed — it was dismissed,
	// not resolved. The NOT IN form materializes the workspace's suppression
	// set once instead of probing per row (measured: the correlated EXISTS
	// probe added ~60% to a 20k-row panel). The file-coverage half of the rule
	// cannot be expressed against the snapshot JSON without duplicating the
	// path normalizer in SQL, so every SQL-matched row is loaded (no LIMIT)
	// and filtered in Go; the panel is on-demand and the returned window stays
	// capped at limit.
	where := `findings.scan_id=? AND NOT EXISTS (SELECT 1 FROM findings AS current_scan WHERE current_scan.scan_id=? AND current_scan.fingerprint=findings.fingerprint) AND findings.analyzer_id IN (SELECT analyzer_id FROM analyzer_runs WHERE scan_id=? AND state='succeeded') AND findings.fingerprint NOT IN (SELECT fingerprint FROM suppressed_findings WHERE workspace_id=?)`
	args := []any{previousID, scan.ID, scan.ID, scan.WorkspaceID}
	if evaluatedFiles == nil {
		var total int
		if err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM findings WHERE `+where, args...).Scan(&total); err != nil {
			return FixedFindingsResult{}, err
		}
		result.Total = total
		rows, err := d.SQL.QueryContext(ctx, `SELECT `+findingColumns+`,'fixed' FROM findings WHERE `+where+` ORDER BY `+severityRankExpr+` DESC, findings.relative_path ASC, findings.start_line ASC, findings.analyzer_id ASC, findings.rule_id ASC LIMIT ?`, append(args, limit)...)
		if err != nil {
			return FixedFindingsResult{}, err
		}
		defer rows.Close()
		for rows.Next() {
			f, err := scanFindingRow(rows)
			if err != nil {
				return FixedFindingsResult{}, err
			}
			result.Items = append(result.Items, f)
		}
		if err := rows.Err(); err != nil {
			return FixedFindingsResult{}, err
		}
		return result, nil
	}
	rows, err := d.SQL.QueryContext(ctx, `SELECT `+findingColumns+`,'fixed' FROM findings WHERE `+where+` ORDER BY `+severityRankExpr+` DESC, findings.relative_path ASC, findings.start_line ASC, findings.analyzer_id ASC, findings.rule_id ASC`, args...)
	if err != nil {
		return FixedFindingsResult{}, err
	}
	defer rows.Close()
	var matched []analyzers.Finding
	for rows.Next() {
		f, err := scanFindingRow(rows)
		if err != nil {
			return FixedFindingsResult{}, err
		}
		// Project-level (path-less) findings follow their analyzer: a
		// successful run re-evaluated the whole project, so they can be fixed.
		if evaluatedFiles != nil && f.RelativePath != "" && !evaluatedFiles[core.NormalizePath(f.RelativePath)] {
			continue
		}
		matched = append(matched, f)
	}
	if err := rows.Err(); err != nil {
		return FixedFindingsResult{}, err
	}
	result.Total = len(matched)
	if len(matched) > limit {
		matched = matched[:limit]
	}
	result.Items = matched
	return result, nil
}

func (d *DB) Metrics(ctx context.Context, scanID string) ([]analyzers.Metric, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT analyzer_id,metric_key,label,COALESCE(value_number,0),COALESCE(unit,'') FROM metrics WHERE scan_id=?`, scanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	output := make([]analyzers.Metric, 0)
	for rows.Next() {
		var m analyzers.Metric
		if err := rows.Scan(&m.AnalyzerID, &m.Key, &m.Label, &m.Value, &m.Unit); err != nil {
			return nil, err
		}
		output = append(output, m)
	}
	return output, rows.Err()
}

// GlobalFindingsFilter controls the cross-workspace findings search. Text
// matches message, rule id, and path as case-insensitive substrings;
// severities and analyzer match exactly; suppressed findings stay hidden
// unless explicitly included.
type GlobalFindingsFilter struct {
	Query             string
	Severities        []string
	Analyzer          string
	WorkspaceID       string
	IncludeSuppressed bool
	// AllScans searches every stored scan. Off (the default), only each
	// workspace's latest completed scan is searched, so a finding that
	// persisted across ten scans is one hit, not ten.
	AllScans bool
	Page     int
	PageSize int
}

// SearchedFinding is a finding enriched with its scan and workspace identity
// so global results can link back to the report they came from.
type SearchedFinding struct {
	analyzers.Finding
	ScanID      string `json:"scan_id"`
	WorkspaceID string `json:"workspace_id"`
}

// searchFindingsWhere assembles the WHERE clause shared by SearchFindings and
// SearchFindingsSeverityCounts so facet totals always describe exactly the
// population the paged list draws from. All values are bound as parameters;
// the text filter escapes LIKE wildcards so q is a substring match, never a
// wildcard expression.
// latestCompletedScanIDsSQL restricts a findings⋈scans query to each
// workspace's newest completed scan.
const latestCompletedScanIDsSQL = `scans.id IN (SELECT id FROM (SELECT id,ROW_NUMBER() OVER (PARTITION BY workspace_id ORDER BY COALESCE(finished_at,started_at) DESC,id DESC) AS recency FROM scans WHERE state IN ('completed','completed_with_warnings')) WHERE recency=1)`

func searchFindingsWhere(filter GlobalFindingsFilter) (string, []any) {
	where := []string{}
	args := []any{}
	if filter.WorkspaceID != "" {
		where = append(where, "scans.workspace_id=?")
		args = append(args, filter.WorkspaceID)
	}
	if !filter.IncludeSuppressed {
		where = append(where, "NOT EXISTS (SELECT 1 FROM suppressed_findings suppressed WHERE suppressed.workspace_id=scans.workspace_id AND suppressed.fingerprint=findings.fingerprint)")
	}
	if !filter.AllScans {
		where = append(where, latestCompletedScanIDsSQL)
	}
	if len(filter.Severities) == 1 {
		where = append(where, "LOWER(findings.severity)=LOWER(?)")
		args = append(args, filter.Severities[0])
	} else if len(filter.Severities) > 1 {
		placeholders := make([]string, len(filter.Severities))
		for i, severity := range filter.Severities {
			placeholders[i] = "LOWER(?)"
			args = append(args, severity)
		}
		where = append(where, "findings.severity IN ("+strings.Join(placeholders, ",")+")")
	}
	if filter.Analyzer != "" {
		where = append(where, "LOWER(findings.analyzer_id)=LOWER(?)")
		args = append(args, filter.Analyzer)
	}
	if filter.Query != "" {
		escaped := pathPrefixEscaper.Replace(filter.Query)
		where = append(where, `(LOWER(findings.message) LIKE '%' || LOWER(?) || '%' ESCAPE '\' OR LOWER(COALESCE(findings.rule_id,'')) LIKE '%' || LOWER(?) || '%' ESCAPE '\' OR LOWER(COALESCE(findings.relative_path,'')) LIKE '%' || LOWER(?) || '%' ESCAPE '\')`)
		args = append(args, escaped, escaped, escaped)
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}
	return whereSQL, args
}

// SearchFindingsSeverityCounts returns severity→count over the same filtered
// population SearchFindings pages (same WHERE, no pagination, one GROUP BY),
// so the UI can render real facet totals next to the result list. Severities
// absent from the result set are absent from the map.
func (d *DB) SearchFindingsSeverityCounts(ctx context.Context, filter GlobalFindingsFilter) (map[string]int, error) {
	whereSQL, args := searchFindingsWhere(filter)
	rows, err := d.SQL.QueryContext(ctx, "SELECT LOWER(findings.severity),COUNT(*) FROM findings JOIN scans ON scans.id=findings.scan_id"+whereSQL+" GROUP BY LOWER(findings.severity)", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var severity string
		var count int
		if err := rows.Scan(&severity, &count); err != nil {
			return nil, err
		}
		counts[severity] = count
	}
	return counts, rows.Err()
}

// SearchFindings runs the cross-workspace findings search: one COUNT plus one
// paged SELECT joining findings to scans, ordered by severity rank then path
// and line so the most serious hits surface first regardless of which
// workspace or analyzer produced them.
func (d *DB) SearchFindings(ctx context.Context, filter GlobalFindingsFilter) ([]SearchedFinding, int, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = DefaultSearchPageSize
	}
	if filter.PageSize > MaxSearchPageSize {
		return nil, 0, fmt.Errorf("page_size must be between 1 and %d", MaxSearchPageSize)
	}
	if filter.Page > 1 && filter.Page-1 > (math.MaxInt-filter.PageSize)/filter.PageSize {
		return nil, 0, fmt.Errorf("page is out of range")
	}
	for _, severity := range filter.Severities {
		switch analyzers.Severity(severity) {
		case analyzers.SeverityCritical, analyzers.SeverityHigh, analyzers.SeverityMedium, analyzers.SeverityLow, analyzers.SeverityInfo:
		default:
			return nil, 0, fmt.Errorf("severity must be critical, high, medium, low, or info")
		}
	}
	whereSQL, args := searchFindingsWhere(filter)
	var total int
	if err := d.SQL.QueryRowContext(ctx, "SELECT COUNT(*) FROM findings JOIN scans ON scans.id=findings.scan_id"+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (filter.Page - 1) * filter.PageSize
	queryArgs := append(append([]any(nil), args...), filter.PageSize, offset)
	order := severityRankExpr + " DESC, findings.relative_path ASC, findings.start_line ASC, findings.analyzer_id ASC"
	rows, err := d.SQL.QueryContext(ctx, "SELECT "+findingColumns+", '', scans.id, scans.workspace_id FROM findings JOIN scans ON scans.id=findings.scan_id"+whereSQL+" ORDER BY "+order+" LIMIT ? OFFSET ?", queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]SearchedFinding, 0)
	for rows.Next() {
		var f analyzers.Finding
		var severity, category, metadata, status string
		var scanID, workspaceID string
		if err := rows.Scan(&f.ID, &f.AnalyzerID, &f.RuleID, &f.Fingerprint, &severity, &category, &f.Title, &f.Message, &f.RelativePath, &f.StartLine, &f.StartColumn, &f.EndLine, &f.EndColumn, &f.Remediation, &f.DocumentationURL, &f.RawSeverity, &metadata, &status, &scanID, &workspaceID); err != nil {
			return nil, 0, err
		}
		f.Severity = analyzers.Severity(severity)
		f.Category = analyzers.Category(category)
		if err := json.Unmarshal([]byte(metadata), &f.Metadata); err != nil {
			// Corrupt metadata is not fatal to the row, but it must not be silent.
			log.Printf("finding %s: corrupt metadata_json replaced with empty object: %v", f.ID, err)
		}
		items = append(items, SearchedFinding{Finding: f, ScanID: scanID, WorkspaceID: workspaceID})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

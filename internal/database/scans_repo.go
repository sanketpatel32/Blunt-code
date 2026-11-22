// scans repo.
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
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
	"bluntcode/internal/reports"
)

// ScansPageFilter optional bounds for GET /workspaces/{id}/scans. Dates use the
// local calendar (time.Local), matching HTML date inputs, and compare against
// COALESCE(finished_at, started_at) like the web's scanMatchesDateRange.
type ScansPageFilter struct {
	DateFrom string // YYYY-MM-DD, inclusive start of local day
	DateTo   string // YYYY-MM-DD, inclusive end of local day
}

// ValidateScansPageFilter rejects malformed date strings before ScansPage runs SQL.
func ValidateScansPageFilter(filter ScansPageFilter) error {
	_, _, err := scansDateWhere(filter)
	return err
}

func scansDateWhere(filter ScansPageFilter) (extraWhere string, extraArgs []any, err error) {
	from := strings.TrimSpace(filter.DateFrom)
	to := strings.TrimSpace(filter.DateTo)
	if from == "" && to == "" {
		return "", nil, nil
	}
	loc := time.Local
	var clauses []string
	if from != "" {
		t, perr := time.ParseInLocation("2006-01-02", from, loc)
		if perr != nil {
			return "", nil, fmt.Errorf("date_from must be YYYY-MM-DD")
		}
		start := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).UTC()
		clauses = append(clauses, `COALESCE(finished_at, started_at) >= ?`)
		extraArgs = append(extraArgs, dbTime(start))
	}
	if to != "" {
		t, perr := time.ParseInLocation("2006-01-02", to, loc)
		if perr != nil {
			return "", nil, fmt.Errorf("date_to must be YYYY-MM-DD")
		}
		end := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, loc).UTC()
		clauses = append(clauses, `COALESCE(finished_at, started_at) <= ?`)
		extraArgs = append(extraArgs, dbTime(end))
	}
	return ` AND ` + strings.Join(clauses, ` AND `), extraArgs, nil
}

// ScansPage returns one page of the workspace's scan history (newest first,
// the same order Scans uses) plus the total row count under the same filter
// so clients can compute has_next. Page and size are clamped here; an absurd page window
// that would overflow the offset is rejected instead of wrapped.
func (d *DB) ScansPage(ctx context.Context, workspaceID string, page, pageSize int, filter ScansPageFilter) ([]core.Scan, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = DefaultScanPageSize
	}
	if pageSize > MaxScanPageSize {
		pageSize = MaxScanPageSize
	}
	if page > 1 && page-1 > (math.MaxInt-pageSize)/pageSize {
		return nil, 0, fmt.Errorf("page is out of range")
	}
	dateWhere, dateArgs, err := scansDateWhere(filter)
	if err != nil {
		return nil, 0, err
	}
	baseWhere := `workspace_id=?` + dateWhere
	args := append([]any{workspaceID}, dateArgs...)
	countSQL := `SELECT COUNT(*) FROM scans WHERE ` + baseWhere
	var total int
	if err := d.SQL.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []core.Scan{}, 0, nil
	}
	maxPage := (total + pageSize - 1) / pageSize
	if page > maxPage {
		return nil, 0, fmt.Errorf("page is out of range")
	}
	selectArgs := append([]any(nil), args...)
	selectArgs = append(selectArgs, pageSize, (page-1)*pageSize)
	rows, err := d.SQL.QueryContext(ctx, `SELECT `+scanSelectColumns("")+` FROM scans WHERE `+baseWhere+` ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`, selectArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]core.Scan, 0, pageSize)
	for rows.Next() {
		var s core.Scan
		if err := hydrateScanRow(rows, &s); err != nil {
			return nil, 0, err
		}
		result = append(result, s)
	}
	return result, total, rows.Err()
}

// CountScansForWorkspace returns how many scan rows exist for the workspace (any date).
func (d *DB) CountScansForWorkspace(ctx context.Context, workspaceID string) (int, error) {
	var n int
	if err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM scans WHERE workspace_id=?`, workspaceID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// LatestScans resolves each workspace's single most recent scan (same recency
// rule as Scans: started_at DESC) in one aggregate query. It exists so the
// workspace list can avoid the per-workspace Scans() loop, which issues one
// query per workspace and hydrates every snapshot of the entire scan history
// just to read scans[0] (measured at 50 workspaces: 51 queries loading all
// rows and snapshots vs 2 queries here).
func (d *DB) LatestScans(ctx context.Context) (map[string]core.Scan, error) {
	return d.latestScans(ctx, false, "")
}

// LatestCompletedScans resolves each workspace's most recent scan that actually
// finished (completed or completed_with_warnings) in one aggregate query. The
// dashboards pair it with LatestScans so a cancelled or interrupted re-scan
// cannot erase a workspace's last good numbers from boards keyed on "latest".
func (d *DB) LatestCompletedScans(ctx context.Context) (map[string]core.Scan, error) {
	return d.latestScans(ctx, true, "")
}

// WorkspaceLatestScans is LatestScans plus LatestCompletedScans for a single
// workspace, so the workspace detail view no longer ranks every workspace's
// history to read one row. Either result is nil when no such scan exists;
// completed is only looked up when latest itself did not finish.
func (d *DB) WorkspaceLatestScans(ctx context.Context, workspaceID string) (latest, completed *core.Scan, err error) {
	byWorkspace, err := d.latestScans(ctx, false, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	scan, ok := byWorkspace[workspaceID]
	if !ok {
		return nil, nil, nil
	}
	latest = &scan
	if scan.State == "completed" || scan.State == "completed_with_warnings" {
		return latest, nil, nil
	}
	byWorkspace, err = d.latestScans(ctx, true, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	if done, ok := byWorkspace[workspaceID]; ok {
		completed = &done
	}
	return latest, completed, nil
}

// latestScans is the one ranking query behind the latest-scan lookups: the
// newest scan per workspace (started_at DESC, id DESC), optionally limited to
// finished states and/or to one workspace.
func (d *DB) latestScans(ctx context.Context, completedOnly bool, workspaceID string) (map[string]core.Scan, error) {
	var where []string
	var args []any
	if completedOnly {
		where = append(where, "state IN ('completed','completed_with_warnings')")
	}
	if workspaceID != "" {
		where = append(where, "workspace_id=?")
		args = append(args, workspaceID)
	}
	filter := ""
	if len(where) > 0 {
		filter = " WHERE " + strings.Join(where, " AND ")
	}
	rows, err := d.SQL.QueryContext(ctx, `SELECT `+scanSelectColumns("")+` FROM scans WHERE id IN (SELECT id FROM (SELECT id,ROW_NUMBER() OVER (PARTITION BY workspace_id ORDER BY started_at DESC,id DESC) AS recency FROM scans`+filter+`) ranked WHERE recency=1)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]core.Scan)
	for rows.Next() {
		var s core.Scan
		if err := hydrateScanRow(rows, &s); err != nil {
			return nil, err
		}
		result[s.WorkspaceID] = s
	}
	return result, rows.Err()
}

func (d *DB) Scan(ctx context.Context, id string) (core.Scan, error) {
	var s core.Scan
	if err := hydrateScanRow(d.SQL.QueryRowContext(ctx, `SELECT `+scanSelectColumns("")+` FROM scans WHERE id=?`, id), &s); err != nil {
		return s, err
	}
	return s, nil
}

func hydrateScanSnapshot(scan *core.Scan) {
	if scan.SnapshotJSON == "" || scan.SnapshotJSON == "{}" {
		return
	}
	var snapshot core.ScanSnapshot
	if json.Unmarshal([]byte(scan.SnapshotJSON), &snapshot) == nil {
		scan.Snapshot = &snapshot
	}
}

// ActiveScanID returns the id of a non-terminal scan for this workspace
// ("" when none) with a bounded probe instead of materializing the whole
// scan history — and each row's snapshot_json — for a boolean check.
func (d *DB) ActiveScanID(ctx context.Context, workspaceID string) (string, error) {
	var id string
	err := d.SQL.QueryRowContext(ctx, `SELECT id FROM scans WHERE workspace_id=? AND state NOT IN ('completed','completed_with_warnings','failed','cancelled','interrupted') ORDER BY started_at DESC LIMIT 1`, workspaceID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// FingerprintExists reports whether a finding with this fingerprint exists in
// the workspace, guarding note writes against orphaned phantom fingerprints.
func (d *DB) FingerprintExists(ctx context.Context, workspaceID, fingerprint string) (bool, error) {
	var one int
	err := d.SQL.QueryRowContext(ctx, `SELECT 1 FROM findings f JOIN scans s ON f.scan_id=s.id WHERE s.workspace_id=? AND f.fingerprint=? LIMIT 1`, workspaceID, fingerprint).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// maxNotesPerFinding caps stored notes per (workspace, fingerprint) so a
// POST loop cannot grow finding_notes without bound.
const maxNotesPerFinding = 100

// FindingNoteCount returns how many notes already exist for the fingerprint.
func (d *DB) FindingNoteCount(ctx context.Context, workspaceID, fingerprint string) (int, error) {
	var n int
	err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM finding_notes WHERE workspace_id=? AND fingerprint=?`, workspaceID, fingerprint).Scan(&n)
	return n, err
}

func isTerminalScanState(state string) bool {
	switch state {
	case "completed", "completed_with_warnings", "failed", "cancelled", "interrupted":
		return true
	}
	return false
}

func stampWorkspaceLastScan(ctx context.Context, tx *sql.Tx, scanID, finishedAt string) error {
	_, err := tx.ExecContext(ctx, `UPDATE workspaces SET last_scan_at=? WHERE id=(SELECT workspace_id FROM scans WHERE id=?)`, finishedAt, scanID)
	return err
}

func recomputeWorkspaceLastScan(ctx context.Context, tx *sql.Tx, workspaceID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE workspaces SET last_scan_at=(SELECT MAX(COALESCE(finished_at, started_at)) FROM scans WHERE workspace_id=? AND state IN `+terminalScanStates+`) WHERE id=?`, workspaceID, workspaceID)
	return err
}

func (d *DB) UpdateScanState(ctx context.Context, id, state, errorSummary string) error {
	now := time.Now()
	finishedAt := dbTime(now)
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `UPDATE scans SET state=?,finished_at=CASE WHEN ? IN ('completed','completed_with_warnings','failed','cancelled','interrupted') THEN ? ELSE finished_at END,error_summary=? WHERE id=?`, state, state, finishedAt, errorSummary, id); err != nil {
		return err
	}
	if isTerminalScanState(state) {
		if err = stampWorkspaceLastScan(ctx, tx, id, finishedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) MarkInterruptedScans(ctx context.Context) error {
	finishedAt := dbTime(time.Now())
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `UPDATE workspaces SET last_scan_at=? WHERE id IN (SELECT workspace_id FROM scans WHERE state IN ('queued','preparing','installing_tools','discovering','running','normalizing','generating_report'))`, finishedAt); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE scans SET state='interrupted',finished_at=?,error_summary='Previous application process exited before scan completion.' WHERE state IN ('queued','preparing','installing_tools','discovering','running','normalizing','generating_report')`, finishedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// RecentScan is a scan row joined with its workspace name for the global
// dashboard activity feed. The immutable snapshot manifest is deliberately not
// hydrated; GET /api/v1/scans/{id} serves the full scan detail.
type RecentScan struct {
	core.Scan
	WorkspaceName string `json:"workspace_name"`
}

// RecentScansFilter bounds the global recent-scans query. State must be a
// known lifecycle state; it is always bound as a SQL parameter, never
// interpolated.
type RecentScansFilter struct {
	Limit       int
	Offset      int
	State       string
	WorkspaceID string
}

const (
	// DefaultRecentScansLimit and MaxRecentScansLimit bound the global scan
	// feed so one request can never drag the whole scan history into memory.
	DefaultRecentScansLimit = 10
	MaxRecentScansLimit     = 50
)

// RecentScans returns the newest scans across every workspace joined with the
// workspace name, plus the total number of scans matching the filter. Limit is
// clamped to 1..MaxRecentScansLimit with DefaultRecentScansLimit as default.
func (d *DB) RecentScans(ctx context.Context, filter RecentScansFilter) ([]RecentScan, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = DefaultRecentScansLimit
	}
	if filter.Limit > MaxRecentScansLimit {
		filter.Limit = MaxRecentScansLimit
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	where, args := "", []any(nil)
	conditions := []string{}
	if filter.State != "" {
		conditions = append(conditions, "scans.state=?")
		args = append(args, filter.State)
	}
	if filter.WorkspaceID != "" {
		conditions = append(conditions, "scans.workspace_id=?")
		args = append(args, filter.WorkspaceID)
	}
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}
	var total int
	if err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM scans`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append(append([]any(nil), args...), filter.Limit)
	limitClause := ` LIMIT ?`
	if filter.Offset > 0 {
		limitClause = ` LIMIT ? OFFSET ?`
		queryArgs = append(queryArgs, filter.Offset)
	}
	rows, err := d.SQL.QueryContext(ctx, `SELECT `+scanSelectColumns("scans")+`,COALESCE(workspaces.name,'') FROM scans JOIN workspaces ON workspaces.id=scans.workspace_id`+where+` ORDER BY scans.started_at DESC,scans.id DESC`+limitClause, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]RecentScan, 0)
	for rows.Next() {
		var item RecentScan
		var started, finished sql.NullString
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.State, &item.Profile, &started, &finished, &item.CandidateFileCount, &item.SelectedFileCount, &item.TotalFindings, &item.CriticalCount, &item.HighCount, &item.MediumCount, &item.LowCount, &item.InfoCount, &item.ErrorSummary, &item.SnapshotJSON, &item.WorkspaceName); err != nil {
			return nil, 0, err
		}
		item.StartedAt, _ = parseNullableTime(started)
		item.FinishedAt, _ = parseNullableTime(finished)
		item.DurationMS = durationMS(item.StartedAt, item.FinishedAt)
		// The snapshot manifest stays deliberately unhydrated here (see
		// RecentScan); the scan detail endpoint serves it in full.
		result = append(result, item)
	}
	return result, total, rows.Err()
}

// SeverityCounts is the per-severity split persisted on every completed scan
// row; CompleteScan writes it once at completion so history never recomputes.
type SeverityCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
}

// SeverityTrendPoint is one completed scan on the workspace trend chart: the
// persisted severity split plus the identity fields the chart labels. The
// timestamp prefers finished_at and falls back to started_at, matching the
// recency rule the summary and previous-scan lookups use.
type SeverityTrendPoint struct {
	ScanID     string         `json:"scan_id"`
	FinishedAt time.Time      `json:"finished_at"`
	Profile    string         `json:"profile"`
	State      string         `json:"state"`
	Severity   SeverityCounts `json:"severity"`
	Total      int            `json:"total"`
}

const (
	// DefaultSeverityTrendLimit and MaxSeverityTrendLimit bound the workspace
	// trend series: the chart shows the most recent window of scans, never the
	// unbounded history.
	DefaultSeverityTrendLimit = 20
	MaxSeverityTrendLimit     = 100
)

// RiskCounts is the persisted severity split of one completed scan, with the
// scan identity the risk endpoint reports back.
type RiskCounts struct {
	ScanID   string
	Critical int
	High     int
	Medium   int
	Low      int
	Info     int
}

// RecentCompletedSeverityCounts returns up to limit completed scans for one
// workspace, newest first, carrying their persisted severity columns. Only
// producing terminal states count (the SeverityTrend rule); failed,
// cancelled, interrupted, and in-flight scans never feed risk.
func (d *DB) RecentCompletedSeverityCounts(ctx context.Context, workspaceID string, limit int) ([]RiskCounts, error) {
	if limit < 1 {
		limit = 2
	}
	rows, err := d.SQL.QueryContext(ctx, `SELECT id,COALESCE(critical_count,0),COALESCE(high_count,0),COALESCE(medium_count,0),COALESCE(low_count,0),COALESCE(info_count,0) FROM scans WHERE workspace_id=? AND state IN ('completed','completed_with_warnings') ORDER BY COALESCE(finished_at,started_at) DESC,id DESC LIMIT ?`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]RiskCounts, 0, limit)
	for rows.Next() {
		var c RiskCounts
		if err := rows.Scan(&c.ScanID, &c.Critical, &c.High, &c.Medium, &c.Low, &c.Info); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ScanCoverage summarizes how much of the planned analyzer set one scan
// actually completed: runs recorded, succeeded, failed, and succeeded with
// degraded output (warning_count > 0). Risk grades pair with it so a
// low-finding partial scan cannot read as the assurance of a complete scan.
type ScanCoverage struct {
	ScanID    string `json:"scan_id"`
	Total     int    `json:"total"`
	Succeeded int    `json:"succeeded"`
	Failed    int    `json:"failed"`
	Warned    int    `json:"warned"`
}

// Complete reports whether the scan ran its full analyzer set cleanly — the
// precondition under which the letter grade alone is meaningful.
func (c ScanCoverage) Complete() bool {
	return c.Failed == 0 && c.Warned == 0 && c.Total > 0
}

// scanCoverageRow is the shared aggregate shape of the coverage queries.
const scanCoverageSQL = `SELECT ?1,COUNT(*),COALESCE(SUM(state='succeeded'),0),COALESCE(SUM(state<>'succeeded'),0),COALESCE(SUM(state='succeeded' AND warning_count>0),0) FROM analyzer_runs WHERE scan_id=?`

// ScanAnalyzerCoverage is ScanCoverage for one scan.
func (d *DB) ScanAnalyzerCoverage(ctx context.Context, scanID string) (ScanCoverage, error) {
	var c ScanCoverage
	err := d.SQL.QueryRowContext(ctx, scanCoverageSQL, scanID, scanID).Scan(&c.ScanID, &c.Total, &c.Succeeded, &c.Failed, &c.Warned)
	if err != nil {
		return ScanCoverage{}, err
	}
	return c, nil
}

// LatestScanCoverage returns ScanCoverage for the same latest-per-workspace
// scans LatestScans serves (any state), keyed by workspace id, so dashboards
// can pair every grade with its coverage without per-workspace queries.
// Workspaces whose latest scan recorded no analyzer runs are absent.
func (d *DB) LatestScanCoverage(ctx context.Context) (map[string]ScanCoverage, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT s.workspace_id,s.id,col.total,col.succeeded,col.failed,col.warned
FROM (SELECT id,workspace_id FROM (SELECT id,workspace_id,ROW_NUMBER() OVER (PARTITION BY workspace_id ORDER BY started_at DESC,id DESC) AS recency FROM scans) ranked WHERE recency=1) s
JOIN (SELECT scan_id,COUNT(*) AS total,COALESCE(SUM(state='succeeded'),0) AS succeeded,COALESCE(SUM(state<>'succeeded'),0) AS failed,COALESCE(SUM(state='succeeded' AND warning_count>0),0) AS warned FROM analyzer_runs GROUP BY scan_id) col ON col.scan_id=s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]ScanCoverage{}
	for rows.Next() {
		var workspaceID string
		var c ScanCoverage
		if err := rows.Scan(&workspaceID, &c.ScanID, &c.Total, &c.Succeeded, &c.Failed, &c.Warned); err != nil {
			return nil, err
		}
		result[workspaceID] = c
	}
	return result, rows.Err()
}

// SeverityTrend returns the newest limit completed scans of one workspace,
// ordered oldest first so the series reads left-to-right over time. Only the
// producing terminal states count as completed (the same rule as ScanSummary);
// failed, cancelled, interrupted, and still-running scans never chart. Limit is
// clamped to 1..MaxSeverityTrendLimit with DefaultSeverityTrendLimit default.
func (d *DB) SeverityTrend(ctx context.Context, workspaceID string, limit int) ([]SeverityTrendPoint, error) {
	if limit <= 0 {
		limit = DefaultSeverityTrendLimit
	}
	if limit > MaxSeverityTrendLimit {
		limit = MaxSeverityTrendLimit
	}
	rows, err := d.SQL.QueryContext(ctx, `SELECT id,profile,state,COALESCE(finished_at,started_at),critical_count,high_count,medium_count,low_count,info_count,total_findings FROM scans WHERE workspace_id=? AND state IN ('completed','completed_with_warnings') ORDER BY COALESCE(finished_at,started_at) DESC,id DESC LIMIT ?`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SeverityTrendPoint, 0, limit)
	for rows.Next() {
		var point SeverityTrendPoint
		var when sql.NullString
		if err := rows.Scan(&point.ScanID, &point.Profile, &point.State, &when, &point.Severity.Critical, &point.Severity.High, &point.Severity.Medium, &point.Severity.Low, &point.Severity.Info, &point.Total); err != nil {
			return nil, err
		}
		if finished, parseErr := parseNullableTime(when); parseErr == nil && finished != nil {
			point.FinishedAt = *finished
		}
		result = append(result, point)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// The query walks newest-to-oldest; the chart reads oldest-to-newest.
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result, nil
}

// ScanSummary holds the cross-workspace aggregates rendered by the dashboard.
// Severity totals are summed over each workspace's latest completed scan only,
// so rescanning one project never double-counts its findings.
type ScanSummary struct {
	WorkspacesTotal   int `json:"workspaces_total"`
	WorkspacesScanned int `json:"workspaces_scanned"`
	CriticalCount     int `json:"critical_count"`
	HighCount         int `json:"high_count"`
	MediumCount       int `json:"medium_count"`
	LowCount          int `json:"low_count"`
	InfoCount         int `json:"info_count"`
	TotalFindings     int `json:"total_findings"`
	ScansTotal        int `json:"scans_total"`
	ScansLast7d       int `json:"scans_last_7d"`
	ActiveScans       int `json:"active_scans"`
}

// ScanSummary computes the dashboard aggregates in two queries: scalar
// counters first, then severity sums over the single latest completed scan of
// each workspace. Only the producing terminal states ('completed',
// 'completed_with_warnings') count as completed; failed, cancelled, and
// interrupted scans never feed the totals, and active scans are any scan in a
// non-terminal state.
func (d *DB) ScanSummary(ctx context.Context) (ScanSummary, error) {
	var summary ScanSummary
	if err := d.SQL.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM workspaces),
		(SELECT COUNT(*) FROM scans),
		(SELECT COUNT(*) FROM scans WHERE started_at>=?),
		(SELECT COUNT(*) FROM scans WHERE state NOT IN ('completed','completed_with_warnings','failed','cancelled','interrupted'))`,
		dbTime(time.Now().Add(-7*24*time.Hour))).Scan(&summary.WorkspacesTotal, &summary.ScansTotal, &summary.ScansLast7d, &summary.ActiveScans); err != nil {
		return ScanSummary{}, err
	}
	if err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(latest.critical_count),0),COALESCE(SUM(latest.high_count),0),COALESCE(SUM(latest.medium_count),0),COALESCE(SUM(latest.low_count),0),COALESCE(SUM(latest.info_count),0),COALESCE(SUM(latest.total_findings),0) FROM (
		SELECT critical_count,high_count,medium_count,low_count,info_count,total_findings,ROW_NUMBER() OVER (PARTITION BY workspace_id ORDER BY COALESCE(finished_at,started_at) DESC,id DESC) AS recency FROM scans WHERE state IN ('completed','completed_with_warnings')
	) latest WHERE recency=1`).Scan(&summary.WorkspacesScanned, &summary.CriticalCount, &summary.HighCount, &summary.MediumCount, &summary.LowCount, &summary.InfoCount, &summary.TotalFindings); err != nil {
		return ScanSummary{}, err
	}
	return summary, nil
}

// GlobalStats holds the cross-workspace overview served at GET /api/v1/stats.
// Findings mirror the dashboard summary's semantics: the persisted severity
// split of the single latest completed scan per workspace, so rescanning one
// project never double-counts its findings. RiskGrades buckets each
// workspace's latest completed scan onto its A-D letter (the same weighted
// scale the workspace risk endpoint uses), so the overview shows how the
// fleet distributes across grades.
type GlobalStats struct {
	Workspaces   int            `json:"workspaces"`
	Scans        GlobalScans    `json:"scans"`
	Findings     GlobalFindings `json:"findings"`
	RiskGrades   map[string]int `json:"risk_grades"`
	Suppressions int            `json:"suppressions"`
	// ScannedWorkspaces counts workspaces whose newest producing scan (completed
	// or completed_with_warnings) exists — the same population as the findings
	// rollup and risk_grades, so the overview never under-counts when the ledger
	// loads one page at a time.
	ScannedWorkspaces int `json:"scanned_workspaces"`
	// PartialAssessments counts workspaces where that newest producing scan has
	// incomplete analyzer coverage (a failed run or a succeeded run with warnings).
	PartialAssessments int `json:"partial_assessments"`
	// LastAssessmentAt is the newest COALESCE(finished_at, started_at) among each
	// workspace's latest producing scan (RFC3339 UTC), or empty when none.
	LastAssessmentAt string `json:"last_assessment_at"`
}

// GlobalScans splits the scan history into the counters the overview shows:
// every scan, the producing terminal states (completed plus
// completed_with_warnings), and the in-flight states (anything non-terminal,
// the same active set ScanSummary counts).
type GlobalScans struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Running   int `json:"running"`
}

// GlobalFindings is the severity rollup over each workspace's latest completed
// scan. Severity reuses the fixed SeverityCounts struct (never a map) so the
// JSON keys are stable and an empty database serializes zeros, not nulls.
type GlobalFindings struct {
	Severity SeverityCounts `json:"severity"`
	Total    int            `json:"total"`
}

// GlobalStats computes the overview in two aggregate queries, following the
// ScanSummary pattern: one row of scalar subselects for the counters, then the
// latest-completed-per-workspace window (ROW_NUMBER over workspace partitions
// of the producing terminal states, the same recency rule the dashboard
// summary uses) summed in SQL. The risk grades reuse that same window shape,
// pulling the severity columns of every workspace's latest completed scan and
// grading them in Go with the weights documented below. No per-workspace
// queries are issued, and an empty database returns the zero value rather
// than an error.
func (d *DB) GlobalStats(ctx context.Context) (GlobalStats, error) {
	var stats GlobalStats
	if err := d.SQL.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM workspaces),
		(SELECT COUNT(*) FROM scans),
		(SELECT COUNT(*) FROM scans WHERE state IN ('completed','completed_with_warnings')),
		(SELECT COUNT(*) FROM scans WHERE state NOT IN ('completed','completed_with_warnings','failed','cancelled','interrupted')),
		(SELECT COUNT(*) FROM suppressed_findings)`).Scan(&stats.Workspaces, &stats.Scans.Total, &stats.Scans.Completed, &stats.Scans.Running, &stats.Suppressions); err != nil {
		return GlobalStats{}, err
	}
	if err := d.SQL.QueryRowContext(ctx, `SELECT COALESCE(SUM(latest.critical_count),0),COALESCE(SUM(latest.high_count),0),COALESCE(SUM(latest.medium_count),0),COALESCE(SUM(latest.low_count),0),COALESCE(SUM(latest.info_count),0),COALESCE(SUM(latest.total_findings),0),
		COUNT(*),
		COALESCE(MAX(latest.finished_or_started), '')
	FROM (
		SELECT critical_count,high_count,medium_count,low_count,info_count,total_findings,
			COALESCE(finished_at, started_at) AS finished_or_started,
			ROW_NUMBER() OVER (PARTITION BY workspace_id ORDER BY COALESCE(finished_at,started_at) DESC,id DESC) AS recency FROM scans WHERE state IN ('completed','completed_with_warnings')
	) latest WHERE recency=1`).Scan(&stats.Findings.Severity.Critical, &stats.Findings.Severity.High, &stats.Findings.Severity.Medium, &stats.Findings.Severity.Low, &stats.Findings.Severity.Info, &stats.Findings.Total, &stats.ScannedWorkspaces, &stats.LastAssessmentAt); err != nil {
		return GlobalStats{}, err
	}
	// Partial coverage: newest producing scan per workspace has a failed analyzer run or a succeeded run with parser warnings.
	if err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM (
		SELECT s.id FROM (
			SELECT id, workspace_id, ROW_NUMBER() OVER (PARTITION BY workspace_id ORDER BY COALESCE(finished_at,started_at) DESC,id DESC) AS recency
			FROM scans WHERE state IN ('completed','completed_with_warnings')
		) r
		JOIN scans s ON s.id = r.id AND r.recency = 1
		JOIN (
			SELECT scan_id,
				COALESCE(SUM(CASE WHEN state <> 'succeeded' THEN 1 ELSE 0 END),0) AS failed,
				COALESCE(SUM(CASE WHEN state = 'succeeded' AND warning_count > 0 THEN 1 ELSE 0 END),0) AS warned
			FROM analyzer_runs GROUP BY scan_id
		) ar ON ar.scan_id = s.id
		WHERE ar.failed > 0 OR ar.warned > 0
	)`).Scan(&stats.PartialAssessments); err != nil {
		return GlobalStats{}, err
	}
	stats.RiskGrades = map[string]int{"A": 0, "B": 0, "C": 0, "D": 0}
	rows, err := d.SQL.QueryContext(ctx, `SELECT latest.critical_count,latest.high_count,latest.medium_count,latest.low_count FROM (
		SELECT critical_count,high_count,medium_count,low_count,ROW_NUMBER() OVER (PARTITION BY workspace_id ORDER BY COALESCE(finished_at,started_at) DESC,id DESC) AS recency FROM scans WHERE state IN ('completed','completed_with_warnings')
	) latest WHERE recency=1`)
	if err != nil {
		return GlobalStats{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var critical, high, medium, low int
		if err := rows.Scan(&critical, &high, &medium, &low); err != nil {
			return GlobalStats{}, err
		}
		stats.RiskGrades[statsRiskGrade(critical, high, medium, low)]++
	}
	if err := rows.Err(); err != nil {
		return GlobalStats{}, err
	}
	return stats, nil
}

// Risk grade weights: identical to the API risk endpoint's documented scale
// (a critical is worth two highs, a high two-and-a-half mediums) and to the
// grade boundaries A<5, B<20, C<50, D beyond. They live here — not imported
// from the api package — because the api package already depends on this one.
const (
	riskGradeWeightCritical = 10
	riskGradeWeightHigh     = 5
	riskGradeWeightMedium   = 2
	riskGradeWeightLow      = 1
)

// statsRiskGrade maps one scan's persisted severity split onto its A-D letter
// using the weighted score above.
func statsRiskGrade(critical, high, medium, low int) string {
	score := float64(critical*riskGradeWeightCritical + high*riskGradeWeightHigh + medium*riskGradeWeightMedium + low*riskGradeWeightLow)
	switch {
	case score < 5:
		return "A"
	case score < 20:
		return "B"
	case score < 50:
		return "C"
	default:
		return "D"
	}
}

type AnalyzerRunInput struct {
	AnalyzerID string
	Version    string
	State      string
	StartedAt  time.Time
	FinishedAt time.Time
	ExitCode   int
	Error      string
	// WarningCount records degradations that did not fail the run but left its
	// coverage incomplete (unparseable output batches); the schema column
	// existed from day one and is only now written.
	WarningCount int
}

func (d *DB) SaveAnalyzerResult(ctx context.Context, scanID string, run AnalyzerRunInput, findings []analyzers.Finding, metrics []analyzers.Metric) (string, error) {
	runID := NewID()
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	duration := run.FinishedAt.Sub(run.StartedAt).Milliseconds()
	if _, err = tx.ExecContext(ctx, `INSERT INTO analyzer_runs(id,scan_id,analyzer_id,version,state,started_at,finished_at,duration_ms,exit_code,finding_count,warning_count,error_summary) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, runID, scanID, run.AnalyzerID, run.Version, run.State, dbTime(run.StartedAt), dbTime(run.FinishedAt), duration, run.ExitCode, len(findings), run.WarningCount, nullIfEmpty(run.Error)); err != nil {
		return "", err
	}
	// Occurrence-distinct identities for fresh output only: reused findings
	// (AppendReusedFindings) keep their stored fingerprints verbatim so
	// incremental scans stay comparable with the scan they came from.
	analyzers.SetFingerprints(findings)
	for _, finding := range findings {
		if err = insertFinding(ctx, tx, scanID, runID, finding); err != nil {
			return "", err
		}
	}
	for _, metric := range metrics {
		if _, err = tx.ExecContext(ctx, `INSERT INTO metrics(id,scan_id,analyzer_id,scope,metric_key,label,value_number,unit,metadata_json) VALUES(?,?,?,?,?,?,?,?,?)`, NewID(), scanID, metric.AnalyzerID, "project", metric.Key, metric.Label, metric.Value, nullIfEmpty(metric.Unit), "{}"); err != nil {
			return "", err
		}
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return runID, nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// insertFinding writes one finding row inside a transaction, deriving the
// fingerprint when the producer did not. It is shared by SaveAnalyzerResult
// (fresh analyzer output) and AppendReusedFindings (findings copied forward
// from a previous scan), so both paths persist the identical row shape.
func insertFinding(ctx context.Context, tx *sql.Tx, scanID, runID string, finding analyzers.Finding) error {
	if finding.Fingerprint == "" {
		finding.SetFingerprint()
	}
	metadata := "{}"
	if finding.Metadata != nil {
		data, err := json.Marshal(finding.Metadata)
		if err != nil {
			return err
		}
		metadata = string(data)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO findings(id,scan_id,analyzer_run_id,analyzer_id,rule_id,fingerprint,severity,category,title,message,relative_path,start_line,start_column,end_line,end_column,remediation,documentation_url,raw_severity,metadata_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, NewID(), scanID, runID, finding.AnalyzerID, finding.RuleID, finding.Fingerprint, string(finding.Severity), string(finding.Category), nullIfEmpty(finding.Title), finding.Message, nullIfEmpty(finding.RelativePath), nullInt(finding.StartLine), nullInt(finding.StartColumn), nullInt(finding.EndLine), nullInt(finding.EndColumn), nullIfEmpty(finding.Remediation), nullIfEmpty(finding.DocumentationURL), nullIfEmpty(finding.RawSeverity), metadata)
	return err
}

func nullInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func (d *DB) CompleteScan(ctx context.Context, scanID, state, reportPath string) error {
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	counts := map[string]int{}
	// Suppressed findings stay stored, but they must not feed the scan record's
	// totals or severity counts. The workspace join resolves the scan's
	// workspace so the NOT EXISTS probe needs no extra bind parameter.
	rows, err := tx.QueryContext(ctx, `SELECT findings.severity, COUNT(*) FROM findings JOIN scans ON scans.id=findings.scan_id WHERE findings.scan_id=? AND NOT EXISTS (SELECT 1 FROM suppressed_findings suppressed WHERE suppressed.workspace_id=scans.workspace_id AND suppressed.fingerprint=findings.fingerprint) GROUP BY severity`, scanID)
	if err == nil {
		for rows.Next() {
			var severity string
			var count int
			if err = rows.Scan(&severity, &count); err != nil {
				break
			}
			counts[severity] = count
		}
		rows.Close()
	}
	finishedAt := dbTime(time.Now())
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE scans SET state=?,finished_at=?,total_findings=?,critical_count=?,high_count=?,medium_count=?,low_count=?,info_count=?,report_markdown_path=? WHERE id=?`, state, finishedAt, counts["critical"]+counts["high"]+counts["medium"]+counts["low"]+counts["info"], counts["critical"], counts["high"], counts["medium"], counts["low"], counts["info"], reportPath, scanID)
	}
	if err == nil && isTerminalScanState(state) {
		err = stampWorkspaceLastScan(ctx, tx, scanID, finishedAt)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// RecountWorkspaceScans recomputes the persisted severity counts and totals
// of every finished scan in a workspace with the same rule CompleteScan uses:
// suppressed fingerprints do not count. Suppressing or restoring a finding
// calls it so the scan header, grade, history and home totals change at once
// instead of only after the next scan.
func (d *DB) RecountWorkspaceScans(ctx context.Context, workspaceID string) error {
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	count := func(severity string) string {
		return `(SELECT COUNT(*) FROM findings f WHERE f.scan_id=scans.id AND f.severity='` + severity + `' AND NOT EXISTS (SELECT 1 FROM suppressed_findings s WHERE s.workspace_id=scans.workspace_id AND s.fingerprint=f.fingerprint))`
	}
	_, err = tx.ExecContext(ctx, `UPDATE scans SET critical_count=`+count("critical")+`,high_count=`+count("high")+`,medium_count=`+count("medium")+`,low_count=`+count("low")+`,info_count=`+count("info")+` WHERE workspace_id=? AND finished_at IS NOT NULL`, workspaceID)
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE scans SET total_findings=critical_count+high_count+medium_count+low_count+info_count WHERE workspace_id=? AND finished_at IS NOT NULL`, workspaceID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) Findings(ctx context.Context, scanID string) ([]analyzers.Finding, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT id,analyzer_id,COALESCE(rule_id,''),fingerprint,severity,category,COALESCE(title,''),message,COALESCE(relative_path,''),COALESCE(start_line,0),COALESCE(start_column,0),COALESCE(end_line,0),COALESCE(end_column,0),COALESCE(remediation,''),COALESCE(documentation_url,''),COALESCE(raw_severity,''),metadata_json FROM findings WHERE scan_id=? ORDER BY relative_path,start_line`, scanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	output := make([]analyzers.Finding, 0)
	for rows.Next() {
		var f analyzers.Finding
		var severity, category, metadata string
		if err := rows.Scan(&f.ID, &f.AnalyzerID, &f.RuleID, &f.Fingerprint, &severity, &category, &f.Title, &f.Message, &f.RelativePath, &f.StartLine, &f.StartColumn, &f.EndLine, &f.EndColumn, &f.Remediation, &f.DocumentationURL, &f.RawSeverity, &metadata); err != nil {
			return nil, err
		}
		f.Severity = analyzers.Severity(severity)
		f.Category = analyzers.Category(category)
		if err := json.Unmarshal([]byte(metadata), &f.Metadata); err != nil {
			// Corrupt metadata is not fatal to the row, but it must not be silent.
			log.Printf("finding %s: corrupt metadata_json replaced with empty object: %v", f.ID, err)
		}
		output = append(output, f)
	}
	return output, rows.Err()
}

func (d *DB) AnalyzerRuns(ctx context.Context, scanID string) ([]reports.Run, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT analyzer_id,COALESCE(version,''),state,COALESCE(error_summary,''),finding_count,COALESCE(warning_count,0),COALESCE(duration_ms,0) FROM analyzer_runs WHERE scan_id=? ORDER BY started_at`, scanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var output []reports.Run
	for rows.Next() {
		var run reports.Run
		var duration int64
		if err := rows.Scan(&run.AnalyzerID, &run.Version, &run.State, &run.ErrorSummary, &run.FindingCount, &run.WarningCount, &duration); err != nil {
			return nil, err
		}
		run.Duration = time.Duration(duration) * time.Millisecond
		output = append(output, run)
	}
	return output, rows.Err()
}

// AnalyzerRunsForScans loads the analyzer runs of several scans in one
// query, keyed by scan id, in the same per-scan order AnalyzerRuns returns.
func (d *DB) AnalyzerRunsForScans(ctx context.Context, scanIDs []string) (map[string][]reports.Run, error) {
	output := make(map[string][]reports.Run, len(scanIDs))
	if len(scanIDs) == 0 {
		return output, nil
	}
	placeholders := make([]string, len(scanIDs))
	args := make([]any, len(scanIDs))
	for i, id := range scanIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := d.SQL.QueryContext(ctx, `SELECT scan_id,analyzer_id,COALESCE(version,''),state,COALESCE(error_summary,''),finding_count,COALESCE(warning_count,0),COALESCE(duration_ms,0) FROM analyzer_runs WHERE scan_id IN (`+strings.Join(placeholders, ",")+`) ORDER BY scan_id, started_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var scanID string
		var run reports.Run
		var duration int64
		if err := rows.Scan(&scanID, &run.AnalyzerID, &run.Version, &run.State, &run.ErrorSummary, &run.FindingCount, &run.WarningCount, &duration); err != nil {
			return nil, err
		}
		run.Duration = time.Duration(duration) * time.Millisecond
		output[scanID] = append(output[scanID], run)
	}
	return output, rows.Err()
}

func (d *DB) ReportPath(ctx context.Context, scanID string) (string, error) {
	var path sql.NullString
	err := d.SQL.QueryRowContext(ctx, `SELECT report_markdown_path FROM scans WHERE id=?`, scanID).Scan(&path)
	if err != nil {
		return "", err
	}
	return path.String, nil
}

func (d *DB) PreviousCompletedScanID(ctx context.Context, workspaceID, currentScanID string) (string, error) {
	var id string
	err := d.SQL.QueryRowContext(ctx, `SELECT id FROM scans WHERE workspace_id=? AND id<>? AND state IN ('completed','completed_with_warnings') ORDER BY COALESCE(finished_at, started_at) DESC, id DESC LIMIT 1`, workspaceID, currentScanID).Scan(&id)
	return id, err
}

func (d *DB) SuccessfulAnalyzerIDs(ctx context.Context, scanID string) (map[string]bool, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT analyzer_id FROM analyzer_runs WHERE scan_id=? AND state='succeeded'`, scanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// SaveScanFileHashes records the content hashes of a scan's selected files
// together with the analyzer identity that produced them (a small JSON blob
// pinned by the scan pipeline; the scans package owns its shape). Both tables
// are written in one transaction at scan completion; replace semantics keep a
// retry from colliding on the primary keys.
func (d *DB) SaveScanFileHashes(ctx context.Context, scanID string, hashes map[string]string, analyzersJSON string) error {
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `DELETE FROM scan_file_hashes WHERE scan_id=?`, scanID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM scan_hash_meta WHERE scan_id=?`, scanID); err != nil {
		return err
	}
	for path, hash := range hashes {
		if _, err = tx.ExecContext(ctx, `INSERT INTO scan_file_hashes(scan_id,relative_path,content_hash) VALUES(?,?,?)`, scanID, path, hash); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO scan_hash_meta(scan_id,analyzers_json) VALUES(?,?)`, scanID, analyzersJSON); err != nil {
		return err
	}
	return tx.Commit()
}

// ScanFileHashes loads the content hashes a scan recorded at completion. A
// scan without hash rows yields an empty map, not an error; callers decide
// what an empty base means.
func (d *DB) ScanFileHashes(ctx context.Context, scanID string) (map[string]string, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT relative_path,content_hash FROM scan_file_hashes WHERE scan_id=?`, scanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var path, hash string
		if err := rows.Scan(&path, &hash); err != nil {
			return nil, err
		}
		out[path] = hash
	}
	return out, rows.Err()
}

// ScanHashAnalyzerSet returns the analyzer identity JSON pinned to a scan's
// file hashes. sql.ErrNoRows means the scan never recorded hashes (it predates
// the feature or did not complete), which callers treat as "nothing to reuse".
func (d *DB) ScanHashAnalyzerSet(ctx context.Context, scanID string) (string, error) {
	var raw string
	err := d.SQL.QueryRowContext(ctx, `SELECT analyzers_json FROM scan_hash_meta WHERE scan_id=?`, scanID).Scan(&raw)
	return raw, err
}

// AppendReusedFindings copies findings from a previous scan into this scan by
// attaching them to the analyzer's existing succeeded run, bumping that run's
// finding count so run summaries stay truthful. Reused findings keep their
// fingerprints (the caller copied them verbatim), so suppression and the
// new/fixed/persistent comparison behave exactly as on fresh findings. It is
// an error when no succeeded run exists - callers only append after a real
// execution succeeded.
func (d *DB) AppendReusedFindings(ctx context.Context, scanID, analyzerID string, findings []analyzers.Finding) error {
	if len(findings) == 0 {
		return nil
	}
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var runID string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM analyzer_runs WHERE scan_id=? AND analyzer_id=? AND state='succeeded' ORDER BY started_at DESC LIMIT 1`, scanID, analyzerID).Scan(&runID); err != nil {
		return err
	}
	for _, finding := range findings {
		if err = insertFinding(ctx, tx, scanID, runID, finding); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE analyzer_runs SET finding_count=finding_count+? WHERE id=?`, len(findings), runID); err != nil {
		return err
	}
	return tx.Commit()
}

// SetScanNote records a non-fatal note on a scan row through error_summary,
// the field completed scans otherwise leave empty and every summary surface
// (CLI output, JSON, API) already displays.
func (d *DB) SetScanNote(ctx context.Context, scanID, note string) error {
	_, err := d.SQL.ExecContext(ctx, `UPDATE scans SET error_summary=? WHERE id=?`, note, scanID)
	return err
}

// SaveScanSnapshot overwrites a scan's immutable-at-rest snapshot JSON. It
// exists for the provenance amendments the scan pipeline records while the
// scan is already running: the input digest once content hashes exist, and
// the drift flag at completion. Callers amend the same *core.ScanSnapshot the
// scan row was created with, so in-memory and persisted views stay equal.
func (d *DB) SaveScanSnapshot(ctx context.Context, scanID string, snapshot *core.ScanSnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is required")
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal scan snapshot: %w", err)
	}
	_, err = d.SQL.ExecContext(ctx, `UPDATE scans SET snapshot_json=? WHERE id=?`, string(data), scanID)
	return err
}

// ValidateTag enforces the workspace tag shape: 1-20 lowercase letters,
// digits, or hyphens. The same rule is encoded in the workspace_tags table
// CHECK constraint, so API validation and storage agree.
func ValidateTag(tag string) error {
	if len(tag) < 1 || len(tag) > 20 {
		return fmt.Errorf("tag must be between 1 and 20 characters")
	}
	for _, r := range tag {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return fmt.Errorf("tag may only contain lowercase letters, digits, and hyphens")
		}
	}
	return nil
}

// PruneOldScans keeps the N most recent terminal scans for a workspace and
// deletes older scans in one transaction. Non-terminal scans are never
// deleted. Deletion cascades via foreign keys to findings, metrics,
// scan_files, scan_file_hashes, scan_hash_meta, and analyzer_runs. The
// deleted ids are returned — never discarded — because a deleted scan can be
// referenced externally: a CI `--baseline <scan-id>` or a baseline SARIF
// exported from it. Callers report the ids so retention cannot silently
// invalidate a referenced baseline. The second result lists the Markdown
// report files that no surviving scan references any more; the database does
// not own those files, so the caller must delete them (see
// reports.RemoveFiles) or they leak forever. Suppression history is
// unaffected: suppressions live on the workspace, not on scan rows.
func (d *DB) PruneOldScans(ctx context.Context, workspaceID string, keep int) (ids []string, orphanedReports []string, err error) {
	if keep < 1 || keep > 100 {
		return nil, nil, fmt.Errorf("keep must be between 1 and 100")
	}
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT id,COALESCE(report_markdown_path,'') FROM scans WHERE workspace_id=? AND state IN `+terminalScanStates+` ORDER BY started_at DESC, id DESC LIMIT -1 OFFSET ?`, workspaceID, keep)
	if err != nil {
		return nil, nil, err
	}
	var reportPaths []string
	for rows.Next() {
		var id, reportPath string
		if err = rows.Scan(&id, &reportPath); err != nil {
			_ = rows.Close()
			return nil, nil, err
		}
		ids = append(ids, id)
		reportPaths = append(reportPaths, reportPath)
	}
	if err = rows.Close(); err != nil {
		return nil, nil, err
	}
	if err = rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(ids) == 0 {
		return nil, nil, tx.Commit()
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM scans WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...); err != nil {
		return nil, nil, err
	}
	if err = recomputeWorkspaceLastScan(ctx, tx, workspaceID); err != nil {
		return nil, nil, err
	}
	if orphanedReports, err = unreferencedReports(ctx, tx, reportPaths); err != nil {
		return nil, nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, nil, err
	}
	return ids, orphanedReports, nil
}

// DeleteScan removes one scan by ID. Only terminal scans may be deleted; a
// running scan must be cancelled first. Deletion cascades via foreign keys to
// findings, metrics, scan_files, scan_file_hashes, scan_hash_meta, and
// analyzer_runs. deleted is false when no terminal scan has this ID.
// orphanedReport names the scan's Markdown report when no surviving scan
// references it; the caller owns deleting that file.
func (d *DB) DeleteScan(ctx context.Context, id string) (deleted bool, orphanedReport string, err error) {
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return false, "", err
	}
	defer func() { _ = tx.Rollback() }()
	var workspaceID, reportPath string
	err = tx.QueryRowContext(ctx, `SELECT workspace_id,COALESCE(report_markdown_path,'') FROM scans WHERE id=? AND state IN `+terminalScanStates, id).Scan(&workspaceID, &reportPath)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM scans WHERE id=?`, id); err != nil {
		return false, "", err
	}
	if err = recomputeWorkspaceLastScan(ctx, tx, workspaceID); err != nil {
		return false, "", err
	}
	orphaned, err := unreferencedReports(ctx, tx, []string{reportPath})
	if err != nil {
		return false, "", err
	}
	if err = tx.Commit(); err != nil {
		return false, "", err
	}
	if len(orphaned) > 0 {
		orphanedReport = orphaned[0]
	}
	return true, orphanedReport, nil
}

// unreferencedReports filters report paths down to the distinct, non-empty
// ones no remaining scan row points at. Report filenames carry only a
// second-resolution timestamp, so two scans can share one file; deleting one
// of them must not take the other's report with it.
func unreferencedReports(ctx context.Context, tx *sql.Tx, paths []string) ([]string, error) {
	var orphaned []string
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		var references int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM scans WHERE report_markdown_path=?`, path).Scan(&references); err != nil {
			return nil, err
		}
		if references == 0 {
			orphaned = append(orphaned, path)
		}
	}
	return orphaned, nil
}

// DefaultSearchPageSize and MaxSearchPageSize bound the cross-workspace
// findings search served at GET /api/v1/findings/search.
const (
	DefaultSearchPageSize = 25
	MaxSearchPageSize     = 100
)

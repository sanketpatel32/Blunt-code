package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
	"bluntcode/internal/reports"
)

func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("random ID: %v", err))
	}
	return hex.EncodeToString(b[0:4]) + "-" + hex.EncodeToString(b[4:6]) + "-" + hex.EncodeToString(b[6:8]) + "-" + hex.EncodeToString(b[8:10]) + "-" + hex.EncodeToString(b[10:])
}

func dbTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func parseNullableTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (d *DB) CreateWorkspace(ctx context.Context, workspace core.Workspace) (core.Workspace, error) {
	if workspace.ID == "" {
		workspace.ID = NewID()
	}
	if workspace.DefaultProfile == "" {
		workspace.DefaultProfile = "standard"
	}
	now := time.Now().UTC()
	workspace.CreatedAt, workspace.UpdatedAt = now, now
	_, err := d.SQL.ExecContext(ctx, `INSERT INTO workspaces (id,name,root_path,created_at,updated_at,default_profile,settings_json) VALUES (?,?,?,?,?,?,?)`, workspace.ID, workspace.Name, workspace.RootPath, dbTime(now), dbTime(now), workspace.DefaultProfile, "{}")
	if err != nil {
		return core.Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	return workspace, nil
}

func scanWorkspace(row *sql.Row) (core.Workspace, error) {
	var w core.Workspace
	var created, updated string
	var opened, scanned sql.NullString
	err := row.Scan(&w.ID, &w.Name, &w.RootPath, &created, &updated, &opened, &scanned, &w.DefaultProfile, &w.SettingsJSON)
	if err != nil {
		return w, err
	}
	var parseErr error
	if w.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, created); parseErr != nil {
		return w, parseErr
	}
	if w.UpdatedAt, parseErr = time.Parse(time.RFC3339Nano, updated); parseErr != nil {
		return w, parseErr
	}
	if w.LastOpenedAt, parseErr = parseNullableTime(opened); parseErr != nil {
		return w, parseErr
	}
	w.LastScanAt, parseErr = parseNullableTime(scanned)
	return w, parseErr
}

const workspaceFields = `id,name,root_path,created_at,updated_at,last_opened_at,last_scan_at,default_profile,settings_json`

func (d *DB) Workspace(ctx context.Context, id string) (core.Workspace, error) {
	return scanWorkspace(d.SQL.QueryRowContext(ctx, `SELECT `+workspaceFields+` FROM workspaces WHERE id=?`, id))
}
func (d *DB) WorkspaceByRoot(ctx context.Context, root string) (core.Workspace, error) {
	return scanWorkspace(d.SQL.QueryRowContext(ctx, `SELECT `+workspaceFields+` FROM workspaces WHERE root_path=?`, root))
}
func (d *DB) Workspaces(ctx context.Context) ([]core.Workspace, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT `+workspaceFields+` FROM workspaces ORDER BY COALESCE(last_opened_at, created_at) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []core.Workspace
	for rows.Next() {
		var w core.Workspace
		var created, updated string
		var opened, scanned sql.NullString
		if err := rows.Scan(&w.ID, &w.Name, &w.RootPath, &created, &updated, &opened, &scanned, &w.DefaultProfile, &w.SettingsJSON); err != nil {
			return nil, err
		}
		w.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		w.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		w.LastOpenedAt, _ = parseNullableTime(opened)
		w.LastScanAt, _ = parseNullableTime(scanned)
		result = append(result, w)
	}
	return result, rows.Err()
}
func (d *DB) UpdateWorkspace(ctx context.Context, w core.Workspace) (core.Workspace, error) {
	w.UpdatedAt = time.Now().UTC()
	result, err := d.SQL.ExecContext(ctx, `UPDATE workspaces SET name=?, updated_at=?, default_profile=? WHERE id=?`, w.Name, dbTime(w.UpdatedAt), w.DefaultProfile, w.ID)
	if err != nil {
		return core.Workspace{}, err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return core.Workspace{}, sql.ErrNoRows
	}
	return d.Workspace(ctx, w.ID)
}
func (d *DB) DeleteWorkspace(ctx context.Context, id string) error {
	result, err := d.SQL.ExecContext(ctx, `DELETE FROM workspaces WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func (d *DB) TouchWorkspace(ctx context.Context, id string) error {
	_, err := d.SQL.ExecContext(ctx, `UPDATE workspaces SET last_opened_at=?,updated_at=? WHERE id=?`, dbTime(time.Now()), dbTime(time.Now()), id)
	return err
}

type AppSettings struct {
	Offline     bool `json:"offline"`
	OpenBrowser bool `json:"open_browser"`
}

func (d *DB) AppSettings(ctx context.Context) (AppSettings, error) {
	var raw string
	err := d.SQL.QueryRowContext(ctx, `SELECT value_json FROM settings WHERE key='app'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return AppSettings{OpenBrowser: true}, nil
	}
	if err != nil {
		return AppSettings{}, err
	}
	var value AppSettings
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return AppSettings{}, fmt.Errorf("parse app settings: %w", err)
	}
	// Existing installations predate open_browser. Preserve the historical
	// launch behavior until the user explicitly changes it.
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &fields) == nil && fields["open_browser"] == nil {
		value.OpenBrowser = true
	}
	return value, nil
}

func (d *DB) SaveAppSettings(ctx context.Context, value AppSettings) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = d.SQL.ExecContext(ctx, `INSERT INTO settings(key,value_json,updated_at) VALUES('app',?,?) ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`, string(raw), dbTime(time.Now()))
	return err
}

func (d *DB) Rules(ctx context.Context, workspaceID string) ([]core.WorkspaceRule, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT id,workspace_id,rule_type,pattern,source,enabled,created_at FROM workspace_rules WHERE workspace_id=? ORDER BY created_at`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// Lists are initialized (never nil) so JSON responses serialize empty
	// results as [] instead of null; typed API clients expect arrays.
	rr := make([]core.WorkspaceRule, 0)
	for rows.Next() {
		var r core.WorkspaceRule
		var enabled int
		var created string
		if err := rows.Scan(&r.ID, &r.WorkspaceID, &r.RuleType, &r.Pattern, &r.Source, &enabled, &created); err != nil {
			return nil, err
		}
		r.Enabled = enabled != 0
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		rr = append(rr, r)
	}
	return rr, rows.Err()
}
func (d *DB) ReplaceUserRules(ctx context.Context, workspaceID string, rules []core.WorkspaceRule) error {
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM workspace_rules WHERE workspace_id=? AND source='user'`, workspaceID); err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, r := range rules {
		if r.RuleType != "include" && r.RuleType != "exclude" {
			_ = tx.Rollback()
			return fmt.Errorf("invalid rule type")
		}
		if r.ID == "" {
			r.ID = NewID()
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO workspace_rules(id,workspace_id,rule_type,pattern,source,enabled,created_at) VALUES(?,?,?,?,?,?,?)`, r.ID, workspaceID, r.RuleType, r.Pattern, "user", boolInt(r.Enabled), dbTime(time.Now())); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) PathOverrides(ctx context.Context, workspaceID string) ([]core.PathOverride, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT workspace_id,relative_path,mode FROM workspace_path_overrides WHERE workspace_id=? ORDER BY relative_path`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]core.PathOverride, 0)
	for rows.Next() {
		var override core.PathOverride
		if err := rows.Scan(&override.WorkspaceID, &override.RelativePath, &override.Mode); err != nil {
			return nil, err
		}
		result = append(result, override)
	}
	return result, rows.Err()
}

// ReplacePathOverrides saves the explicit file or directory choices for a
// workspace.  A missing path means the normal discovery default applies.
func (d *DB) ReplacePathOverrides(ctx context.Context, workspaceID string, overrides []core.PathOverride) error {
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM workspace_path_overrides WHERE workspace_id=?`, workspaceID); err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, override := range overrides {
		if override.Mode != "include" && override.Mode != "exclude" || strings.TrimSpace(override.RelativePath) == "" {
			_ = tx.Rollback()
			return fmt.Errorf("invalid path override")
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO workspace_path_overrides(workspace_id,relative_path,mode,created_at,updated_at) VALUES(?,?,?,?,?)`, workspaceID, override.RelativePath, override.Mode, dbTime(time.Now()), dbTime(time.Now())); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (d *DB) CreateScan(ctx context.Context, scan core.Scan) (core.Scan, error) {
	return d.CreateScanWithFiles(ctx, scan, nil)
}

// CreateScanWithFiles persists the immutable scan snapshot and selected-file
// manifest in the same transaction as the scan row.
func (d *DB) CreateScanWithFiles(ctx context.Context, scan core.Scan, files []core.FileEntry) (core.Scan, error) {
	if scan.ID == "" {
		scan.ID = NewID()
	}
	if scan.Profile == "" {
		scan.Profile = "standard"
	}
	if scan.State == "" {
		scan.State = "queued"
	}
	now := time.Now().UTC()
	scan.StartedAt = &now
	if scan.Snapshot != nil {
		if scan.Snapshot.CapturedAt.IsZero() {
			scan.Snapshot.CapturedAt = now
		}
		data, err := json.Marshal(scan.Snapshot)
		if err != nil {
			return core.Scan{}, fmt.Errorf("marshal scan snapshot: %w", err)
		}
		scan.SnapshotJSON = string(data)
	}
	if scan.SnapshotJSON == "" {
		scan.SnapshotJSON = "{}"
	}
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return core.Scan{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO scans(id,workspace_id,state,profile,started_at,candidate_file_count,selected_file_count,snapshot_json) VALUES(?,?,?,?,?,?,?,?)`, scan.ID, scan.WorkspaceID, scan.State, scan.Profile, dbTime(now), scan.CandidateFileCount, scan.SelectedFileCount, scan.SnapshotJSON); err != nil {
		_ = tx.Rollback()
		return core.Scan{}, err
	}
	for _, file := range files {
		if _, err = tx.ExecContext(ctx, `INSERT INTO scan_files(scan_id,relative_path,language,selected,skip_reason,size_bytes) VALUES(?,?,?,?,?,?)`, scan.ID, file.RelativePath, nullIfEmpty(file.Language), boolInt(file.Selected), nullIfEmpty(file.SkipReason), file.SizeBytes); err != nil {
			_ = tx.Rollback()
			return core.Scan{}, fmt.Errorf("save scan file manifest: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return core.Scan{}, err
	}
	return scan, nil
}
func (d *DB) Scans(ctx context.Context, workspaceID string) ([]core.Scan, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT id,workspace_id,state,profile,started_at,finished_at,candidate_file_count,selected_file_count,total_findings,COALESCE(error_summary,''),snapshot_json FROM scans WHERE workspace_id=? ORDER BY started_at DESC, id DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]core.Scan, 0)
	for rows.Next() {
		var s core.Scan
		var st, fin sql.NullString
		if err := rows.Scan(&s.ID, &s.WorkspaceID, &s.State, &s.Profile, &st, &fin, &s.CandidateFileCount, &s.SelectedFileCount, &s.TotalFindings, &s.ErrorSummary, &s.SnapshotJSON); err != nil {
			return nil, err
		}
		s.StartedAt, _ = parseNullableTime(st)
		s.FinishedAt, _ = parseNullableTime(fin)
		hydrateScanSnapshot(&s)
		result = append(result, s)
	}
	return result, rows.Err()
}
// LatestScans resolves each workspace's single most recent scan (same recency
// rule as Scans: started_at DESC) in one aggregate query. It exists so the
// workspace list can avoid the per-workspace Scans() loop, which issues one
// query per workspace and hydrates every snapshot of the entire scan history
// just to read scans[0] (measured at 50 workspaces: 51 queries loading all
// rows and snapshots vs 2 queries here).
func (d *DB) LatestScans(ctx context.Context) (map[string]core.Scan, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT id,workspace_id,state,profile,started_at,finished_at,candidate_file_count,selected_file_count,total_findings,COALESCE(error_summary,''),snapshot_json FROM scans WHERE id IN (SELECT id FROM (SELECT id,ROW_NUMBER() OVER (PARTITION BY workspace_id ORDER BY started_at DESC,id DESC) AS recency FROM scans) ranked WHERE recency=1)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]core.Scan)
	for rows.Next() {
		var s core.Scan
		var st, fin sql.NullString
		if err := rows.Scan(&s.ID, &s.WorkspaceID, &s.State, &s.Profile, &st, &fin, &s.CandidateFileCount, &s.SelectedFileCount, &s.TotalFindings, &s.ErrorSummary, &s.SnapshotJSON); err != nil {
			return nil, err
		}
		s.StartedAt, _ = parseNullableTime(st)
		s.FinishedAt, _ = parseNullableTime(fin)
		hydrateScanSnapshot(&s)
		result[s.WorkspaceID] = s
	}
	return result, rows.Err()
}

func (d *DB) Scan(ctx context.Context, id string) (core.Scan, error) {
	var s core.Scan
	var st, fin sql.NullString
	err := d.SQL.QueryRowContext(ctx, `SELECT id,workspace_id,state,profile,started_at,finished_at,candidate_file_count,selected_file_count,total_findings,COALESCE(error_summary,''),snapshot_json FROM scans WHERE id=?`, id).Scan(&s.ID, &s.WorkspaceID, &s.State, &s.Profile, &st, &fin, &s.CandidateFileCount, &s.SelectedFileCount, &s.TotalFindings, &s.ErrorSummary, &s.SnapshotJSON)
	if err != nil {
		return s, err
	}
	s.StartedAt, _ = parseNullableTime(st)
	s.FinishedAt, _ = parseNullableTime(fin)
	hydrateScanSnapshot(&s)
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
func (d *DB) UpdateScanState(ctx context.Context, id, state, errorSummary string) error {
	now := time.Now()
	_, err := d.SQL.ExecContext(ctx, `UPDATE scans SET state=?,finished_at=CASE WHEN ? IN ('completed','completed_with_warnings','failed','cancelled','interrupted') THEN ? ELSE finished_at END,error_summary=? WHERE id=?`, state, state, dbTime(now), errorSummary, id)
	return err
}
func (d *DB) MarkInterruptedScans(ctx context.Context) error {
	_, err := d.SQL.ExecContext(ctx, `UPDATE scans SET state='interrupted',finished_at=?,error_summary='Previous application process exited before scan completion.' WHERE state IN ('queued','preparing','installing_tools','discovering','running','normalizing','generating_report')`, dbTime(time.Now()))
	return err
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
	Limit int
	State string
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
	where, args := "", []any(nil)
	if filter.State != "" {
		where = " WHERE scans.state=?"
		args = append(args, filter.State)
	}
	var total int
	if err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM scans`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	queryArgs := append(append([]any(nil), args...), filter.Limit)
	rows, err := d.SQL.QueryContext(ctx, `SELECT scans.id,scans.workspace_id,scans.state,scans.profile,scans.started_at,scans.finished_at,scans.candidate_file_count,scans.selected_file_count,scans.total_findings,COALESCE(scans.error_summary,''),scans.snapshot_json,COALESCE(workspaces.name,'') FROM scans JOIN workspaces ON workspaces.id=scans.workspace_id`+where+` ORDER BY scans.started_at DESC,scans.id DESC LIMIT ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]RecentScan, 0)
	for rows.Next() {
		var item RecentScan
		var started, finished sql.NullString
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.State, &item.Profile, &started, &finished, &item.CandidateFileCount, &item.SelectedFileCount, &item.TotalFindings, &item.ErrorSummary, &item.SnapshotJSON, &item.WorkspaceName); err != nil {
			return nil, 0, err
		}
		item.StartedAt, _ = parseNullableTime(started)
		item.FinishedAt, _ = parseNullableTime(finished)
		result = append(result, item)
	}
	return result, total, rows.Err()
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

type AnalyzerRunInput struct {
	AnalyzerID string
	Version    string
	State      string
	StartedAt  time.Time
	FinishedAt time.Time
	ExitCode   int
	Error      string
}

func (d *DB) SaveAnalyzerResult(ctx context.Context, scanID string, run AnalyzerRunInput, findings []analyzers.Finding, metrics []analyzers.Metric) (string, error) {
	runID := NewID()
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	duration := run.FinishedAt.Sub(run.StartedAt).Milliseconds()
	if _, err = tx.ExecContext(ctx, `INSERT INTO analyzer_runs(id,scan_id,analyzer_id,version,state,started_at,finished_at,duration_ms,exit_code,finding_count,error_summary) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, runID, scanID, run.AnalyzerID, run.Version, run.State, dbTime(run.StartedAt), dbTime(run.FinishedAt), duration, run.ExitCode, len(findings), nullIfEmpty(run.Error)); err != nil {
		_ = tx.Rollback()
		return "", err
	}
	for _, finding := range findings {
		if finding.Fingerprint == "" {
			finding.SetFingerprint()
		}
		metadata := "{}"
		if finding.Metadata != nil {
			data, marshalErr := json.Marshal(finding.Metadata)
			if marshalErr != nil {
				_ = tx.Rollback()
				return "", marshalErr
			}
			metadata = string(data)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO findings(id,scan_id,analyzer_run_id,analyzer_id,rule_id,fingerprint,severity,category,title,message,relative_path,start_line,start_column,end_line,end_column,remediation,documentation_url,raw_severity,metadata_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, NewID(), scanID, runID, finding.AnalyzerID, finding.RuleID, finding.Fingerprint, string(finding.Severity), string(finding.Category), nullIfEmpty(finding.Title), finding.Message, nullIfEmpty(finding.RelativePath), nullInt(finding.StartLine), nullInt(finding.StartColumn), nullInt(finding.EndLine), nullInt(finding.EndColumn), nullIfEmpty(finding.Remediation), nullIfEmpty(finding.DocumentationURL), nullIfEmpty(finding.RawSeverity), metadata); err != nil {
			_ = tx.Rollback()
			return "", err
		}
	}
	for _, metric := range metrics {
		if _, err = tx.ExecContext(ctx, `INSERT INTO metrics(id,scan_id,analyzer_id,scope,metric_key,label,value_number,unit,metadata_json) VALUES(?,?,?,?,?,?,?,?,?)`, NewID(), scanID, metric.AnalyzerID, "project", metric.Key, metric.Label, metric.Value, nullIfEmpty(metric.Unit), "{}"); err != nil {
			_ = tx.Rollback()
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
	counts := map[string]int{}
	rows, err := tx.QueryContext(ctx, `SELECT severity, COUNT(*) FROM findings WHERE scan_id=? GROUP BY severity`, scanID)
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
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE scans SET state=?,finished_at=?,total_findings=?,critical_count=?,high_count=?,medium_count=?,low_count=?,info_count=?,report_markdown_path=? WHERE id=?`, state, dbTime(time.Now()), counts["critical"]+counts["high"]+counts["medium"]+counts["low"]+counts["info"], counts["critical"], counts["high"], counts["medium"], counts["low"], counts["info"], reportPath, scanID)
	}
	if err != nil {
		_ = tx.Rollback()
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
		_ = json.Unmarshal([]byte(metadata), &f.Metadata)
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
	_ = json.Unmarshal([]byte(metadata), &f.Metadata)
	return f, nil
}

// FindingFilter contains only server-side finding query controls. Text values
// are matched case-insensitively as literal substrings; they are never used as
// SQL syntax.
type FindingFilter struct {
	Severity string
	Category string
	Analyzer string
	Path     string
	Status   string // "new" or "persistent"
	Query    string
	Sort     string // severity, path, analyzer, status
	Order    string // asc or desc
	Limit    int
	Offset   int
}

type FindingsPage struct {
	Items      []analyzers.Finding
	Total      int
	Limit      int
	Offset     int
	NextOffset *int
	HasMore    bool
}

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

// findingQuery carries the filter, ordering, and comparison-status machinery
// shared by FindingsPage and FindingsCSV so the paged JSON list and the CSV
// export always agree on which rows match.
type findingQuery struct {
	previousID string
	statusExpr string
	whereSQL   string
	args       []any
	order      string
	orderArgs  []any
}

// buildFindingQuery validates the controls shared by both findings endpoints,
// resolves the previous completed scan for comparison status, and assembles
// the WHERE and ORDER BY clauses. Limit and offset stay the paged caller's
// responsibility.
func (d *DB) buildFindingQuery(ctx context.Context, scan core.Scan, filter FindingFilter) (findingQuery, error) {
	if filter.Status != "" && filter.Status != "new" && filter.Status != "persistent" {
		return findingQuery{}, fmt.Errorf("status must be new or persistent")
	}
	if filter.Sort != "" && filter.Sort != "severity" && filter.Sort != "path" && filter.Sort != "analyzer" && filter.Sort != "status" {
		return findingQuery{}, fmt.Errorf("sort must be severity, path, analyzer, or status")
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
	statusExpr := `CASE WHEN EXISTS (SELECT 1 FROM findings previous WHERE previous.scan_id=? AND previous.fingerprint=findings.fingerprint) THEN 'persistent' ELSE 'new' END`
	where := []string{"findings.scan_id=?"}
	args := []any{scan.ID}
	addEquals := func(column, value string) {
		if value != "" {
			where = append(where, "LOWER("+column+")=LOWER(?)")
			args = append(args, value)
		}
	}
	addEquals("findings.severity", filter.Severity)
	addEquals("findings.category", filter.Category)
	addEquals("findings.analyzer_id", filter.Analyzer)
	if filter.Path != "" {
		where = append(where, "LOWER(COALESCE(findings.relative_path,'')) LIKE '%' || LOWER(?) || '%'")
		args = append(args, filter.Path)
	}
	if filter.Query != "" {
		where = append(where, "(LOWER(findings.message) LIKE '%' || LOWER(?) || '%' OR LOWER(COALESCE(findings.rule_id,'')) LIKE '%' || LOWER(?) || '%' OR LOWER(COALESCE(findings.relative_path,'')) LIKE '%' || LOWER(?) || '%')")
		args = append(args, filter.Query, filter.Query, filter.Query)
	}
	if filter.Status != "" {
		where = append(where, statusExpr+"=?")
		args = append(args, previousID, filter.Status)
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
	case "analyzer":
		order = "findings.analyzer_id " + direction + ", findings.relative_path ASC, findings.start_line ASC"
	case "status":
		order = statusExpr + " " + direction + ", findings.relative_path ASC, findings.start_line ASC"
		orderArgs = []any{previousID}
	}
	return findingQuery{previousID: previousID, statusExpr: statusExpr, whereSQL: strings.Join(where, " AND "), args: args, order: order, orderArgs: orderArgs}, nil
}

// selectArgs orders bind parameters the way the shared SELECT mentions them:
// the computed status column first, then the WHERE placeholders, then any
// ORDER BY placeholders. Callers append their own LIMIT/OFFSET values.
func (q findingQuery) selectArgs() []any {
	args := append([]any{q.previousID}, q.args...)
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
	_ = json.Unmarshal([]byte(metadata), &f.Metadata)
	return f, nil
}

// FindingsPage returns a bounded page and applies comparison status relative
// to the previous completed scan in the same workspace.
func (d *DB) FindingsPage(ctx context.Context, scan core.Scan, filter FindingFilter) (FindingsPage, error) {
	if filter.Limit == 0 {
		filter.Limit = 50
	}
	if filter.Limit < 1 || filter.Limit > 100 || (filter.Limit != 25 && filter.Limit != 50 && filter.Limit != 100) {
		return FindingsPage{}, fmt.Errorf("limit must be 25, 50, or 100")
	}
	if filter.Offset < 0 {
		return FindingsPage{}, fmt.Errorf("offset must not be negative")
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
// matching, ordering, and comparison-status rules as FindingsPage. Limit and
// offset are ignored; the row count is capped at MaxFindingsExportRows.
func (d *DB) FindingsCSV(ctx context.Context, scan core.Scan, filter FindingFilter) ([]analyzers.Finding, error) {
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
// of the same workspace and disappeared from this scan. The coverage rule from
// scans.Compare is applied in SQL: a disappeared finding only counts as fixed
// when its analyzer also succeeded in the current scan, because its absence is
// otherwise unknown, not fixed. Rows reuse the shared finding projection and
// row scanner of FindingsPage with the status column pinned to "fixed"; limit
// defaults to DefaultFixedFindingsLimit and is capped at MaxFixedFindingsLimit
// while Total always reports the uncapped count.
func (d *DB) FixedFindings(ctx context.Context, scan core.Scan, limit int) (FixedFindingsResult, error) {
	if limit <= 0 {
		limit = DefaultFixedFindingsLimit
	}
	if limit > MaxFixedFindingsLimit {
		limit = MaxFixedFindingsLimit
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
	// subquery is the SQL form of SuccessfulAnalyzerIDs for the coverage rule.
	where := `findings.scan_id=? AND NOT EXISTS (SELECT 1 FROM findings AS current_scan WHERE current_scan.scan_id=? AND current_scan.fingerprint=findings.fingerprint) AND findings.analyzer_id IN (SELECT analyzer_id FROM analyzer_runs WHERE scan_id=? AND state='succeeded')`
	args := []any{previousID, scan.ID, scan.ID}
	if err := d.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM findings WHERE `+where, args...).Scan(&result.Total); err != nil {
		return FixedFindingsResult{}, err
	}
	queryArgs := append(append([]any(nil), args...), limit)
	rows, err := d.SQL.QueryContext(ctx, `SELECT `+findingColumns+`,'fixed' FROM findings WHERE `+where+` ORDER BY `+severityRankExpr+` DESC, findings.relative_path ASC, findings.start_line ASC, findings.analyzer_id ASC, findings.rule_id ASC LIMIT ?`, queryArgs...)
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
	return result, rows.Err()
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
func (d *DB) AnalyzerRuns(ctx context.Context, scanID string) ([]reports.Run, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT analyzer_id,COALESCE(version,''),state,COALESCE(error_summary,''),finding_count,COALESCE(duration_ms,0) FROM analyzer_runs WHERE scan_id=? ORDER BY started_at`, scanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var output []reports.Run
	for rows.Next() {
		var run reports.Run
		var duration int64
		if err := rows.Scan(&run.AnalyzerID, &run.Version, &run.State, &run.ErrorSummary, &run.FindingCount, &duration); err != nil {
			return nil, err
		}
		run.Duration = time.Duration(duration) * time.Millisecond
		output = append(output, run)
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

package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
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

// AddSuppression records a dismissed finding fingerprint for a workspace.
// Suppressing the same fingerprint again is idempotent and refreshes the
// reason. Suppressions are keyed by (workspace_id, fingerprint) and apply to
// every future scan of that workspace.
func (d *DB) AddSuppression(ctx context.Context, workspaceID, fingerprint, reason string) (core.Suppression, error) {
	if strings.TrimSpace(fingerprint) == "" {
		return core.Suppression{}, fmt.Errorf("fingerprint is required")
	}
	now := time.Now().UTC()
	if _, err := d.SQL.ExecContext(ctx, `INSERT INTO suppressed_findings(workspace_id,fingerprint,reason,created_at) VALUES(?,?,?,?) ON CONFLICT(workspace_id,fingerprint) DO UPDATE SET reason=excluded.reason`, workspaceID, fingerprint, nullIfEmpty(reason), dbTime(now)); err != nil {
		return core.Suppression{}, err
	}
	return core.Suppression{WorkspaceID: workspaceID, Fingerprint: fingerprint, Reason: reason, CreatedAt: now}, nil
}

// RemoveSuppression deletes one dismissal; it returns sql.ErrNoRows when the
// fingerprint was not suppressed, so the next scan stores and counts the
// finding normally again.
func (d *DB) RemoveSuppression(ctx context.Context, workspaceID, fingerprint string) error {
	result, err := d.SQL.ExecContext(ctx, `DELETE FROM suppressed_findings WHERE workspace_id=? AND fingerprint=?`, workspaceID, fingerprint)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Suppressions lists a workspace's dismissals oldest first, never nil, so API
// responses serialize an empty list as [].
func (d *DB) Suppressions(ctx context.Context, workspaceID string) ([]core.Suppression, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT workspace_id,fingerprint,COALESCE(reason,''),created_at FROM suppressed_findings WHERE workspace_id=? ORDER BY created_at,fingerprint`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]core.Suppression, 0)
	for rows.Next() {
		var item core.Suppression
		var created string
		if err := rows.Scan(&item.WorkspaceID, &item.Fingerprint, &item.Reason, &created); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		result = append(result, item)
	}
	return result, rows.Err()
}

// SuppressedFingerprints loads a workspace's dismissed fingerprints as a set,
// the shape scan-time filtering and count exclusion consume.
func (d *DB) SuppressedFingerprints(ctx context.Context, workspaceID string) (map[string]bool, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT fingerprint FROM suppressed_findings WHERE workspace_id=?`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var fingerprint string
		if err := rows.Scan(&fingerprint); err != nil {
			return nil, err
		}
		out[fingerprint] = true
	}
	return out, rows.Err()
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
	Limit       int
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
// project never double-counts its findings.
type GlobalStats struct {
	Workspaces   int            `json:"workspaces"`
	Scans        GlobalScans    `json:"scans"`
	Findings     GlobalFindings `json:"findings"`
	Suppressions int            `json:"suppressions"`
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
// summary uses) summed in SQL. No per-workspace queries are issued, and an
// empty database returns the zero value rather than an error.
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
	if err := d.SQL.QueryRowContext(ctx, `SELECT COALESCE(SUM(latest.critical_count),0),COALESCE(SUM(latest.high_count),0),COALESCE(SUM(latest.medium_count),0),COALESCE(SUM(latest.low_count),0),COALESCE(SUM(latest.info_count),0),COALESCE(SUM(latest.total_findings),0) FROM (
		SELECT critical_count,high_count,medium_count,low_count,info_count,total_findings,ROW_NUMBER() OVER (PARTITION BY workspace_id ORDER BY COALESCE(finished_at,started_at) DESC,id DESC) AS recency FROM scans WHERE state IN ('completed','completed_with_warnings')
	) latest WHERE recency=1`).Scan(&stats.Findings.Severity.Critical, &stats.Findings.Severity.High, &stats.Findings.Severity.Medium, &stats.Findings.Severity.Low, &stats.Findings.Severity.Info, &stats.Findings.Total); err != nil {
		return GlobalStats{}, err
	}
	return stats, nil
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
		if err = insertFinding(ctx, tx, scanID, runID, finding); err != nil {
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
		where = append(where, "LOWER(COALESCE(findings.relative_path,'')) LIKE '%' || LOWER(?) || '%'")
		args = append(args, filter.Path)
	}
	if prefix := normalizePathPrefix(filter.PathPrefix); prefix != "" {
		where = append(where, "LOWER(COALESCE(findings.relative_path,'')) LIKE ? ESCAPE '\\'")
		args = append(args, likePatternPrefix(prefix))
	}
	if filter.Query != "" {
		where = append(where, "(LOWER(findings.message) LIKE '%' || LOWER(?) || '%' OR LOWER(COALESCE(findings.rule_id,'')) LIKE '%' || LOWER(?) || '%' OR LOWER(COALESCE(findings.relative_path,'')) LIKE '%' || LOWER(?) || '%')")
		args = append(args, filter.Query, filter.Query, filter.Query)
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
	_ = json.Unmarshal([]byte(metadata), &f.Metadata)
	return f, nil
}

// FindingsPage returns a bounded page and applies comparison status relative
// to the previous completed scan in the same workspace. Pagination has two
// modes: the legacy limit (25/50/100) plus offset pair, and the page-based
// mode enabled by PageSize > 0 (1-based Page, any size up to
// MaxFindingsPageSize), which derives limit and offset from the page window.
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
	// subquery is the SQL form of SuccessfulAnalyzerIDs for the coverage rule,
	// and a suppressed fingerprint never reports as fixed â€” it was dismissed,
	// not resolved.
	// The fingerprint predicate mirrors the comparison status in
	// buildFindingQuery pointed the other way: previous findings whose
	// fingerprint no longer exists in the current scan are gone. The analyzer
	// subquery is the SQL form of SuccessfulAnalyzerIDs for the coverage rule,
	// and a suppressed fingerprint never reports as fixed â€” it was dismissed,
	// not resolved. The NOT IN form materializes the workspace's suppression
	// set once instead of probing per row (measured: the correlated EXISTS
	// probe added ~60% to a 20k-row panel).
	where := `findings.scan_id=? AND NOT EXISTS (SELECT 1 FROM findings AS current_scan WHERE current_scan.scan_id=? AND current_scan.fingerprint=findings.fingerprint) AND findings.analyzer_id IN (SELECT analyzer_id FROM analyzer_runs WHERE scan_id=? AND state='succeeded') AND findings.fingerprint NOT IN (SELECT fingerprint FROM suppressed_findings WHERE workspace_id=?)`
	args := []any{previousID, scan.ID, scan.ID, scan.WorkspaceID}
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
	if _, err = tx.ExecContext(ctx, `DELETE FROM scan_file_hashes WHERE scan_id=?`, scanID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM scan_hash_meta WHERE scan_id=?`, scanID); err != nil {
		_ = tx.Rollback()
		return err
	}
	for path, hash := range hashes {
		if _, err = tx.ExecContext(ctx, `INSERT INTO scan_file_hashes(scan_id,relative_path,content_hash) VALUES(?,?,?)`, scanID, path, hash); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO scan_hash_meta(scan_id,analyzers_json) VALUES(?,?)`, scanID, analyzersJSON); err != nil {
		_ = tx.Rollback()
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
	var runID string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM analyzer_runs WHERE scan_id=? AND analyzer_id=? AND state='succeeded' ORDER BY started_at DESC LIMIT 1`, scanID, analyzerID).Scan(&runID); err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, finding := range findings {
		if err = insertFinding(ctx, tx, scanID, runID, finding); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE analyzer_runs SET finding_count=finding_count+? WHERE id=?`, len(findings), runID); err != nil {
		_ = tx.Rollback()
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

// GetWorkspaceTags returns the workspace's tags sorted alphabetically.
func (d *DB) GetWorkspaceTags(ctx context.Context, workspaceID string) ([]string, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT tag FROM workspace_tags WHERE workspace_id=? ORDER BY tag`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tags := make([]string, 0)
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

// WorkspaceTagsMap batches one query for many workspaces so list endpoints can
// attach tags without per-workspace round trips.
func (d *DB) WorkspaceTagsMap(ctx context.Context, ids []string) (map[string][]string, error) {
	out := make(map[string][]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := d.SQL.QueryContext(ctx, `SELECT workspace_id, tag FROM workspace_tags WHERE workspace_id IN (`+strings.Join(placeholders, ",")+`) ORDER BY workspace_id, tag`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var workspaceID, tag string
		if err := rows.Scan(&workspaceID, &tag); err != nil {
			return nil, err
		}
		out[workspaceID] = append(out[workspaceID], tag)
	}
	return out, rows.Err()
}

// SetWorkspaceTags atomically replaces the workspace's tag set. Tags are
// validated, de-duplicated, and stored sorted.
func (d *DB) SetWorkspaceTags(ctx context.Context, workspaceID string, tags []string) error {
	if tags == nil {
		tags = make([]string, 0)
	}
	seen := map[string]bool{}
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if err := ValidateTag(tag); err != nil {
			return err
		}
		if seen[tag] {
			continue
		}
		seen[tag] = true
		normalized = append(normalized, tag)
	}
	sort.Strings(normalized)
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM workspace_tags WHERE workspace_id=?`, workspaceID); err != nil {
		_ = tx.Rollback()
		return err
	}
	for _, tag := range normalized {
		if _, err = tx.ExecContext(ctx, `INSERT INTO workspace_tags(workspace_id, tag) VALUES(?,?)`, workspaceID, tag); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// PruneOldScans keeps the N most recent terminal scans for a workspace and
// deletes older scans. Non-terminal scans are never deleted. Deletion cascades
// via foreign keys to findings, metrics, scan_files, scan_file_hashes,
// scan_hash_meta, and analyzer_runs.
func (d *DB) PruneOldScans(ctx context.Context, workspaceID string, keep int) (int64, error) {
	if keep < 1 || keep > 100 {
		return 0, fmt.Errorf("keep must be between 1 and 100")
	}
	rows, err := d.SQL.QueryContext(ctx, `SELECT id FROM scans WHERE workspace_id=? AND state IN ('completed','completed_with_warnings','failed','cancelled','interrupted') ORDER BY started_at DESC, id DESC LIMIT -1 OFFSET ?`, workspaceID, keep)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	result, err := d.SQL.ExecContext(ctx, `DELETE FROM scans WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return n, nil
}

// DeleteScan removes one scan by ID. Only terminal scans may be deleted; a
// running scan must be cancelled first. Deletion cascades via foreign keys to
// findings, metrics, scan_files, scan_file_hashes, scan_hash_meta, and
// analyzer_runs. The second return is false when no terminal scan has this ID.
func (d *DB) DeleteScan(ctx context.Context, id string) (bool, error) {
	result, err := d.SQL.ExecContext(ctx, `DELETE FROM scans WHERE id=? AND state IN ('completed','completed_with_warnings','failed','cancelled','interrupted')`, id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// DefaultSearchPageSize and MaxSearchPageSize bound the cross-workspace
// findings search served at GET /api/v1/findings/search.
const (
	DefaultSearchPageSize = 25
	MaxSearchPageSize     = 100
)

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
	Page              int
	PageSize          int
}

// SearchedFinding is a finding enriched with its scan and workspace identity
// so global results can link back to the report they came from.
type SearchedFinding struct {
	analyzers.Finding
	ScanID      string `json:"scan_id"`
	WorkspaceID string `json:"workspace_id"`
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
	where := []string{}
	args := []any{}
	if filter.WorkspaceID != "" {
		where = append(where, "scans.workspace_id=?")
		args = append(args, filter.WorkspaceID)
	}
	if !filter.IncludeSuppressed {
		where = append(where, "NOT EXISTS (SELECT 1 FROM suppressed_findings suppressed WHERE suppressed.workspace_id=scans.workspace_id AND suppressed.fingerprint=findings.fingerprint)")
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
		where = append(where, "(LOWER(findings.message) LIKE '%' || LOWER(?) || '%' OR LOWER(COALESCE(findings.rule_id,'')) LIKE '%' || LOWER(?) || '%' OR LOWER(COALESCE(findings.relative_path,'')) LIKE '%' || LOWER(?) || '%')")
		args = append(args, filter.Query, filter.Query, filter.Query)
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}
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
		_ = json.Unmarshal([]byte(metadata), &f.Metadata)
		items = append(items, SearchedFinding{Finding: f, ScanID: scanID, WorkspaceID: workspaceID})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

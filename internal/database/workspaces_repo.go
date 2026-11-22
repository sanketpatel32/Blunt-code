// workspaces repo.
package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"bluntcode/internal/core"
)

func (d *DB) CreateWorkspace(ctx context.Context, workspace core.Workspace) (core.Workspace, error) {
	if workspace.ID == "" {
		workspace.ID = NewID()
	}
	if workspace.DefaultProfile == "" {
		workspace.DefaultProfile = "standard"
	}
	now := time.Now().UTC()
	workspace.CreatedAt, workspace.UpdatedAt = now, now
	gitRepository := 0
	if workspace.GitRepository {
		gitRepository = 1
	}
	_, err := d.SQL.ExecContext(ctx, `INSERT INTO workspaces (id,name,root_path,created_at,updated_at,default_profile,settings_json,git_repository) VALUES (?,?,?,?,?,?,?,?)`, workspace.ID, workspace.Name, workspace.RootPath, dbTime(now), dbTime(now), workspace.DefaultProfile, mergedWorkspaceSettings(core.Workspace{SettingsJSON: "{}", TrustRepoAnalyzerConfig: workspace.TrustRepoAnalyzerConfig}), gitRepository)
	if err != nil {
		return core.Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	return workspace, nil
}

func scanWorkspace(row *sql.Row) (core.Workspace, error) {
	var w core.Workspace
	var created, updated string
	var opened, scanned sql.NullString
	var gitRepository int
	err := row.Scan(&w.ID, &w.Name, &w.RootPath, &created, &updated, &opened, &scanned, &w.DefaultProfile, &w.SettingsJSON, &gitRepository)
	if err != nil {
		return w, err
	}
	w.GitRepository = gitRepository != 0
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
	applyWorkspaceSettings(&w)
	return w, parseErr
}

// workspaceSettingTrustRepoAnalyzerConfig is the settings_json key for
// core.Workspace.TrustRepoAnalyzerConfig.
const workspaceSettingTrustRepoAnalyzerConfig = "trust_repo_analyzer_config"

// applyWorkspaceSettings decodes the typed settings carried in settings_json.
// Malformed JSON leaves every setting at its safe default.
func applyWorkspaceSettings(w *core.Workspace) {
	var settings map[string]json.RawMessage
	if json.Unmarshal([]byte(w.SettingsJSON), &settings) != nil {
		return
	}
	var trust bool
	if raw, ok := settings[workspaceSettingTrustRepoAnalyzerConfig]; ok && json.Unmarshal(raw, &trust) == nil {
		w.TrustRepoAnalyzerConfig = trust
	}
}

// mergedWorkspaceSettings writes the typed settings back into settings_json,
// preserving any keys this build does not know about.
func mergedWorkspaceSettings(w core.Workspace) string {
	settings := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(w.SettingsJSON), &settings); err != nil {
		// Corrupt stored JSON: return the blob untouched rather than silently
		// dropping every unknown key the merge claims to preserve.
		log.Printf("workspace %s: corrupt settings_json preserved as-is: %v", w.ID, err)
		return w.SettingsJSON
	}
	if settings == nil {
		settings = map[string]json.RawMessage{}
	}
	if w.TrustRepoAnalyzerConfig {
		settings[workspaceSettingTrustRepoAnalyzerConfig] = json.RawMessage("true")
	} else {
		delete(settings, workspaceSettingTrustRepoAnalyzerConfig)
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return "{}"
	}
	return string(data)
}

const workspaceFields = `id,name,root_path,created_at,updated_at,last_opened_at,last_scan_at,default_profile,settings_json,git_repository`

func (d *DB) Workspace(ctx context.Context, id string) (core.Workspace, error) {
	return scanWorkspace(d.SQL.QueryRowContext(ctx, `SELECT `+workspaceFields+` FROM workspaces WHERE id=?`, id))
}

func (d *DB) WorkspaceByRoot(ctx context.Context, root string) (core.Workspace, error) {
	return scanWorkspace(d.SQL.QueryRowContext(ctx, `SELECT `+workspaceFields+` FROM workspaces WHERE root_path=?`, root))
}

func (d *DB) Workspaces(ctx context.Context) ([]core.Workspace, error) {
	return d.queryWorkspaces(ctx, `SELECT `+workspaceFields+` FROM workspaces ORDER BY COALESCE(last_opened_at, created_at) DESC`)
}

// UnscannedWorkspaces lists workspaces with no scan rows, whose languages are
// known only from filesystem discovery.
func (d *DB) UnscannedWorkspaces(ctx context.Context) ([]core.Workspace, error) {
	return d.queryWorkspaces(ctx, `SELECT `+workspaceFields+` FROM workspaces w WHERE NOT EXISTS (SELECT 1 FROM scans s WHERE s.workspace_id = w.id) ORDER BY w.id`)
}

func (d *DB) queryWorkspaces(ctx context.Context, query string) ([]core.Workspace, error) {
	rows, err := d.SQL.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []core.Workspace
	for rows.Next() {
		var w core.Workspace
		var created, updated string
		var opened, scanned sql.NullString
		var gitRepository int
		if err := rows.Scan(&w.ID, &w.Name, &w.RootPath, &created, &updated, &opened, &scanned, &w.DefaultProfile, &w.SettingsJSON, &gitRepository); err != nil {
			return nil, err
		}
		w.GitRepository = gitRepository != 0
		w.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		w.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		w.LastOpenedAt, _ = parseNullableTime(opened)
		w.LastScanAt, _ = parseNullableTime(scanned)
		applyWorkspaceSettings(&w)
		result = append(result, w)
	}
	return result, rows.Err()
}

const (
	// DefaultWorkspacePageSize and MaxWorkspacePageSize bound GET /workspaces when
	// the client requests server paging (?page=&page_size=).
	DefaultWorkspacePageSize = 25
	MaxWorkspacePageSize     = 100
)

// WorkspacePageFilter selects sort and optional name/path/language/tag
// substring match for WorkspacesPage. Empty Query matches all; Sort must be
// name, last_opened, last_scan, or findings.
type WorkspacePageFilter struct {
	Query string
	// QueryLanguageIDs lists workspaces whose discovered (not snapshot)
	// languages match Query; they match only while they have no snapshot
	// languages, the same fallback the workspace view applies.
	QueryLanguageIDs []string
	Tag              string
	Sort             string
	Order            string // asc or desc
}

// workspaceSnapshotLanguages yields the language counts of the scan snapshot
// the workspace view reads languages from: the newest finished scan, else the
// newest scan.
const workspaceSnapshotLanguages = `json_each(COALESCE((
	SELECT CASE WHEN json_valid(s.snapshot_json) THEN s.snapshot_json END FROM scans s
	WHERE s.workspace_id = w.id
	ORDER BY CASE WHEN s.state IN ('completed','completed_with_warnings') THEN 0 ELSE 1 END, s.started_at DESC, s.id DESC
	LIMIT 1), '{}'), '$.languages') lang`

// WorkspacesPage returns one slice of workspaces with total matching rows, using
// SQL paging and sort. Text Query matches LOWER(name), LOWER(root_path), or a
// detected language id as a substring; Tag matches workspace_tags.tag as a
// substring (case-insensitive). Findings sort uses each workspace's newest scan
// (by started_at) total_findings; workspaces with no scans sink to the end on
// desc. Last-scan sort uses last_scan_at, else the newest scan's finished or
// started time; never-scanned workspaces sink to the end in both directions.
func (d *DB) WorkspacesPage(ctx context.Context, page, pageSize int, filter WorkspacePageFilter) ([]core.Workspace, int, error) {
	if page < 1 {
		return nil, 0, fmt.Errorf("page is out of range")
	}
	if pageSize < 1 {
		pageSize = DefaultWorkspacePageSize
	}
	if pageSize > MaxWorkspacePageSize {
		pageSize = MaxWorkspacePageSize
	}
	sortKey := strings.ToLower(strings.TrimSpace(filter.Sort))
	if sortKey == "" {
		sortKey = "last_opened"
	}
	order := strings.ToLower(strings.TrimSpace(filter.Order))
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	q := strings.TrimSpace(filter.Query)
	tagNeedle := strings.TrimSpace(strings.ToLower(filter.Tag))
	whereParts := []string(nil)
	whereArgs := []any(nil)
	if q != "" {
		like := "%" + strings.ToLower(q) + "%"
		hasSnapshotLanguages := `EXISTS (SELECT 1 FROM ` + workspaceSnapshotLanguages + ` WHERE lang.value > 0)`
		snapshotLanguageMatch := `EXISTS (SELECT 1 FROM ` + workspaceSnapshotLanguages + ` WHERE lang.value > 0 AND LOWER(lang.key) LIKE ?)`
		textMatch := `LOWER(w.name) LIKE ? OR LOWER(w.root_path) LIKE ? OR ` + snapshotLanguageMatch
		whereArgs = append(whereArgs, like, like, like)
		if len(filter.QueryLanguageIDs) > 0 {
			textMatch += ` OR (w.id IN (?` + strings.Repeat(`,?`, len(filter.QueryLanguageIDs)-1) + `) AND NOT ` + hasSnapshotLanguages + `)`
			for _, id := range filter.QueryLanguageIDs {
				whereArgs = append(whereArgs, id)
			}
		}
		whereParts = append(whereParts, `(`+textMatch+`)`)
	}
	if tagNeedle != "" {
		whereParts = append(whereParts, `EXISTS (SELECT 1 FROM workspace_tags t WHERE t.workspace_id = w.id AND LOWER(t.tag) LIKE ?)`)
		whereArgs = append(whereArgs, "%"+tagNeedle+"%")
	}
	whereSQL := ""
	if len(whereParts) > 0 {
		whereSQL = ` WHERE ` + strings.Join(whereParts, ` AND `)
	}
	joinLatest := ``
	if sortKey == "findings" || sortKey == "last_scan" {
		joinLatest = ` LEFT JOIN (
			SELECT workspace_id, total_findings, finished_at, started_at,
				ROW_NUMBER() OVER (PARTITION BY workspace_id ORDER BY started_at DESC, id DESC) AS recency
			FROM scans
		) latest ON latest.workspace_id = w.id AND latest.recency = 1`
	}
	var orderExpr string
	switch sortKey {
	case "name":
		orderExpr = `w.name COLLATE NOCASE`
	case "last_scan":
		scanned := `COALESCE(w.last_scan_at, latest.finished_at, latest.started_at)`
		orderExpr = `(` + scanned + ` IS NULL) ASC, ` + scanned
	case "findings":
		orderExpr = `COALESCE(latest.total_findings, -1)`
	default: // last_opened
		sortKey = "last_opened"
		orderExpr = `COALESCE(w.last_opened_at, w.created_at)`
	}
	orderDir := "DESC"
	if order == "asc" {
		orderDir = "ASC"
	}
	tie := `, w.id ASC`
	if orderDir == "DESC" {
		tie = `, w.id DESC`
	}
	countSQL := `SELECT COUNT(*) FROM workspaces w` + whereSQL
	var total int
	if err := d.SQL.QueryRowContext(ctx, countSQL, whereArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []core.Workspace{}, 0, nil
	}
	maxPage := (total + pageSize - 1) / pageSize
	if page > maxPage {
		return nil, 0, fmt.Errorf("page is out of range")
	}
	selectCols := `SELECT ` + workspaceFields + ` FROM workspaces w`
	if joinLatest != "" {
		selectCols = `SELECT ` + workspaceFieldsW() + ` FROM workspaces w` + joinLatest
	}
	sqlQuery := selectCols + whereSQL + ` ORDER BY ` + orderExpr + ` ` + orderDir + tie + ` LIMIT ? OFFSET ?`
	offset := (page - 1) * pageSize
	args := append(append([]any(nil), whereArgs...), pageSize, offset)
	rows, err := d.SQL.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var result []core.Workspace
	for rows.Next() {
		var w core.Workspace
		var created, updated string
		var opened, scanned sql.NullString
		var gitRepository int
		if err := rows.Scan(&w.ID, &w.Name, &w.RootPath, &created, &updated, &opened, &scanned, &w.DefaultProfile, &w.SettingsJSON, &gitRepository); err != nil {
			return nil, 0, err
		}
		w.GitRepository = gitRepository != 0
		w.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		w.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		w.LastOpenedAt, _ = parseNullableTime(opened)
		w.LastScanAt, _ = parseNullableTime(scanned)
		applyWorkspaceSettings(&w)
		result = append(result, w)
	}
	return result, total, rows.Err()
}

func workspaceFieldsW() string {
	// workspaceFields lists bare column names; qualify with w. for joined queries.
	parts := strings.Split(workspaceFields, ",")
	for i, p := range parts {
		parts[i] = "w." + strings.TrimSpace(p)
	}
	return strings.Join(parts, ",")
}
func (d *DB) UpdateWorkspace(ctx context.Context, w core.Workspace) (core.Workspace, error) {
	w.UpdatedAt = time.Now().UTC()
	result, err := d.SQL.ExecContext(ctx, `UPDATE workspaces SET name=?, updated_at=?, default_profile=?, settings_json=? WHERE id=?`, w.Name, dbTime(w.UpdatedAt), w.DefaultProfile, mergedWorkspaceSettings(w), w.ID)
	if err != nil {
		return core.Workspace{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return core.Workspace{}, err
	}
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
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func (d *DB) TouchWorkspace(ctx context.Context, id string) error {
	_, err := d.SQL.ExecContext(ctx, `UPDATE workspaces SET last_opened_at=?,updated_at=? WHERE id=?`, dbTime(time.Now()), dbTime(time.Now()), id)
	return err
}

// SetWorkspaceGitRepository persists whether the workspace root is a git
// working tree. Detection runs at create and again on discover, so the flag
// follows a root that gains or loses its .git entry between scans.
func (d *DB) SetWorkspaceGitRepository(ctx context.Context, id string, isRepository bool) error {
	value := 0
	if isRepository {
		value = 1
	}
	_, err := d.SQL.ExecContext(ctx, `UPDATE workspaces SET git_repository=? WHERE id=?`, value, id)
	return err
}

type AppSettings struct {
	Offline     bool `json:"offline"`
	OpenBrowser bool `json:"open_browser"`
	// UpdateCheck opts in to the UI's automatic release check on load (a GET
	// to api.github.com). Off by default; manual checks stay explicit.
	UpdateCheck bool `json:"update_check"`
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
		if parsed, parseErr := time.Parse(time.RFC3339Nano, created); parseErr == nil {
			r.CreatedAt = parsed
		} else {
			log.Printf("workspace rule %s: unparsable created_at %q: %v", r.ID, created, parseErr)
		}
		rr = append(rr, r)
	}
	return rr, rows.Err()
}
func (d *DB) ReplaceUserRules(ctx context.Context, workspaceID string, rules []core.WorkspaceRule) error {
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `DELETE FROM workspace_rules WHERE workspace_id=? AND source='user'`, workspaceID); err != nil {
		return err
	}
	for _, r := range rules {
		if r.RuleType != "include" && r.RuleType != "exclude" {
			return fmt.Errorf("invalid rule type")
		}
		if r.ID == "" {
			r.ID = NewID()
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO workspace_rules(id,workspace_id,rule_type,pattern,source,enabled,created_at) VALUES(?,?,?,?,?,?,?)`, r.ID, workspaceID, r.RuleType, r.Pattern, "user", boolInt(r.Enabled), dbTime(time.Now())); err != nil {
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
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `DELETE FROM workspace_path_overrides WHERE workspace_id=?`, workspaceID); err != nil {
		return err
	}
	for _, override := range overrides {
		if override.Mode != "include" && override.Mode != "exclude" || strings.TrimSpace(override.RelativePath) == "" {
			return fmt.Errorf("invalid path override")
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO workspace_path_overrides(workspace_id,relative_path,mode,created_at,updated_at) VALUES(?,?,?,?,?)`, workspaceID, override.RelativePath, override.Mode, dbTime(time.Now()), dbTime(time.Now())); err != nil {
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

// SuppressionInput is one row of a bulk suppression import.
type SuppressionInput struct {
	Fingerprint string
	Reason      string
}

// AddSuppressions records a batch of dismissals in a single transaction with
// the same upsert semantics as AddSuppression: either every row lands or none
// does, so a failed import can be retried as a whole instead of leaving a
// partial import behind.
func (d *DB) AddSuppressions(ctx context.Context, workspaceID string, items []SuppressionInput) (err error) {
	if len(items) == 0 {
		return nil
	}
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	statement, err := tx.PrepareContext(ctx, `INSERT INTO suppressed_findings(workspace_id,fingerprint,reason,created_at) VALUES(?,?,?,?) ON CONFLICT(workspace_id,fingerprint) DO UPDATE SET reason=excluded.reason`)
	if err != nil {
		return err
	}
	defer statement.Close()
	now := dbTime(time.Now().UTC())
	for _, item := range items {
		if strings.TrimSpace(item.Fingerprint) == "" {
			return fmt.Errorf("fingerprint is required")
		}
		if _, err = statement.ExecContext(ctx, workspaceID, item.Fingerprint, nullIfEmpty(item.Reason), now); err != nil {
			return fmt.Errorf("suppress %s: %w", item.Fingerprint, err)
		}
	}
	return tx.Commit()
}

// RemoveSuppression deletes one dismissal; it returns sql.ErrNoRows when the
// fingerprint was not suppressed, so the next scan stores and counts the
// finding normally again.
func (d *DB) RemoveSuppression(ctx context.Context, workspaceID, fingerprint string) error {
	result, err := d.SQL.ExecContext(ctx, `DELETE FROM suppressed_findings WHERE workspace_id=? AND fingerprint=?`, workspaceID, fingerprint)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
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
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `INSERT INTO scans(id,workspace_id,state,profile,started_at,candidate_file_count,selected_file_count,snapshot_json) VALUES(?,?,?,?,?,?,?,?)`, scan.ID, scan.WorkspaceID, scan.State, scan.Profile, dbTime(now), scan.CandidateFileCount, scan.SelectedFileCount, scan.SnapshotJSON); err != nil {
		return core.Scan{}, err
	}
	for _, file := range files {
		if _, err = tx.ExecContext(ctx, `INSERT INTO scan_files(scan_id,relative_path,language,selected,skip_reason,size_bytes) VALUES(?,?,?,?,?,?)`, scan.ID, file.RelativePath, nullIfEmpty(file.Language), boolInt(file.Selected), nullIfEmpty(file.SkipReason), file.SizeBytes); err != nil {
			return core.Scan{}, fmt.Errorf("save scan file manifest: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return core.Scan{}, err
	}
	return scan, nil
}

// scanSelectColumns lists every scans column hydrated into core.Scan, in the
// exact order hydrateScanRow reads them. The optional alias qualifies the
// table on joined queries (e.g. "scans").
func scanSelectColumns(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	columns := []string{"id", "workspace_id", "state", "profile", "started_at", "finished_at", "candidate_file_count", "selected_file_count", "total_findings", "critical_count", "high_count", "medium_count", "low_count", "info_count"}
	for i, column := range columns {
		columns[i] = prefix + column
	}
	return strings.Join(columns, ",") + `,COALESCE(` + prefix + `error_summary,''),` + prefix + `snapshot_json`
}

// hydrateScanRow scans one row selected via scanSelectColumns into s — the
// persisted severity split, nullable timestamps, and immutable snapshot
// included. It accepts *sql.Row and *sql.Rows alike.
func hydrateScanRow(row interface{ Scan(dest ...any) error }, s *core.Scan) error {
	var started, finished sql.NullString
	if err := row.Scan(&s.ID, &s.WorkspaceID, &s.State, &s.Profile, &started, &finished, &s.CandidateFileCount, &s.SelectedFileCount, &s.TotalFindings, &s.CriticalCount, &s.HighCount, &s.MediumCount, &s.LowCount, &s.InfoCount, &s.ErrorSummary, &s.SnapshotJSON); err != nil {
		return err
	}
	s.StartedAt, _ = parseNullableTime(started)
	s.FinishedAt, _ = parseNullableTime(finished)
	s.DurationMS = durationMS(s.StartedAt, s.FinishedAt)
	hydrateScanSnapshot(s)
	return nil
}

// durationMS reports finished-started in whole milliseconds; nil until the
// scan has both a start and a finish (or when the finish precedes its start),
// which keeps the JSON field off unfinished scans. A real zero-millisecond
// scan still reports the field, as 0.
func durationMS(started, finished *time.Time) *int64 {
	if started == nil || finished == nil || finished.Before(*started) {
		return nil
	}
	ms := finished.Sub(*started).Milliseconds()
	return &ms
}

// Scans loads a workspace's ENTIRE scan history — unbounded by design, so it
// stays for tests and full dumps only. Callers that need a count, the latest
// scan, or a page must use ScansPage so growth can't hurt them.
func (d *DB) Scans(ctx context.Context, workspaceID string) ([]core.Scan, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT `+scanSelectColumns("")+` FROM scans WHERE workspace_id=? ORDER BY started_at DESC, id DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]core.Scan, 0)
	for rows.Next() {
		var s core.Scan
		if err := hydrateScanRow(rows, &s); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

// DefaultScanPageSize and MaxScanPageSize bound the workspace scan history
// endpoint (GET /workspaces/{id}/scans). They mirror the findings page bounds
// the web client already speaks: page is 1-based and page_size caps at 100.
const (
	DefaultScanPageSize = 25
	MaxScanPageSize     = 100
)

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

// WorkspaceIDsWithTag returns ids of workspaces carrying a tag — used by the
// demo-seed purge to remove synthetic rows wholesale.
func (d *DB) WorkspaceIDsWithTag(ctx context.Context, tag string) ([]string, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT DISTINCT workspace_id FROM workspace_tags WHERE tag=?`, tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `DELETE FROM workspace_tags WHERE workspace_id=?`, workspaceID); err != nil {
		return err
	}
	for _, tag := range normalized {
		if _, err = tx.ExecContext(ctx, `INSERT INTO workspace_tags(workspace_id, tag) VALUES(?,?)`, workspaceID, tag); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// terminalScanStates is the SQL list of states a scan never leaves; only
// these scans may be deleted by retention or by hand.
const terminalScanStates = `('completed','completed_with_warnings','failed','cancelled','interrupted')`

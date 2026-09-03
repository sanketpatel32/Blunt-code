package database

import (
	"context"
	"fmt"
	"regexp"

	"bluntcode/internal/analyzers"
)

// sonarCorruptPath matches the relative_path shape stored by releases before
// the SonarQube project-key fix: "<workspace-uuid>:<real path>". The adapter
// trimmed components at the first colon, which only removed the "bluntcode:"
// head of the "bluntcode:<workspace-id>" project key and left the workspace id
// glued to every path.
var sonarCorruptPath = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}:(.+)$`)

// RepairSonarqubeFindingPaths rewrites SonarQube findings whose relative_path
// still carries the workspace-uuid component prefix, so existing scans render
// real file locations without requiring a rescan. Each repaired row's
// fingerprint is recomputed (it hashes the path), and any suppression recorded
// under the old fingerprint moves with it so dismissals survive the repair.
// Idempotent: repaired paths no longer match the pattern.
func (d *DB) RepairSonarqubeFindingPaths(ctx context.Context) (int, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT f.id, f.fingerprint, COALESCE(f.rule_id,''), COALESCE(f.message,''), COALESCE(f.relative_path,''), s.workspace_id
		FROM findings f JOIN scans s ON s.id = f.scan_id
		WHERE f.analyzer_id = 'sonarqube'`)
	if err != nil {
		return 0, fmt.Errorf("query sonarqube findings: %w", err)
	}
	type repair struct{ id, rule, message, path, oldFingerprint, newFingerprint, workspaceID string }
	var pending []repair
	for rows.Next() {
		var r repair
		if err := rows.Scan(&r.id, &r.oldFingerprint, &r.rule, &r.message, &r.path, &r.workspaceID); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan sonarqube finding: %w", err)
		}
		match := sonarCorruptPath.FindStringSubmatch(r.path)
		if match == nil {
			continue
		}
		f := analyzers.Finding{AnalyzerID: "sonarqube", RuleID: r.rule, Message: r.message, RelativePath: match[1]}
		f.SetFingerprint()
		r.path = f.RelativePath
		r.newFingerprint = f.Fingerprint
		pending = append(pending, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate sonarqube findings: %w", err)
	}
	rows.Close()
	if len(pending) == 0 {
		return 0, nil
	}
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin repair transaction: %w", err)
	}
	defer tx.Rollback()
	for _, r := range pending {
		if _, err := tx.ExecContext(ctx, `UPDATE findings SET relative_path = ?, fingerprint = ? WHERE id = ?`, r.path, r.newFingerprint, r.id); err != nil {
			return 0, fmt.Errorf("repair finding %s: %w", r.id, err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE OR IGNORE suppressed_findings SET fingerprint = ? WHERE fingerprint = ? AND workspace_id = ?`, r.newFingerprint, r.oldFingerprint, r.workspaceID); err != nil {
			return 0, fmt.Errorf("carry suppression for finding %s: %w", r.id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit repair: %w", err)
	}
	return len(pending), nil
}

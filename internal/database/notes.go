package database

import (
	"context"
	"time"
)

type FindingNote struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt"`
}

func (d *DB) FindingNotes(ctx context.Context, workspaceID, fingerprint string) ([]FindingNote, error) {
	rows, err := d.SQL.QueryContext(ctx, "SELECT id,text,created_at FROM finding_notes WHERE workspace_id=? AND fingerprint=? ORDER BY created_at,id", workspaceID, fingerprint)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	notes := []FindingNote{}
	for rows.Next() {
		var note FindingNote
		if err := rows.Scan(&note.ID, &note.Text, &note.CreatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	return notes, rows.Err()
}
func (d *DB) AddFindingNote(ctx context.Context, workspaceID, fingerprint, text string, importKeys ...string) (FindingNote, error) {
	note := FindingNote{ID: NewID(), Text: text, CreatedAt: dbTime(time.Now())}
	var importKey any
	if len(importKeys) > 0 && importKeys[0] != "" {
		importKey = importKeys[0]
	}
	_, err := d.SQL.ExecContext(ctx, "INSERT INTO finding_notes(id,workspace_id,fingerprint,text,created_at,import_key) VALUES(?,?,?,?,?,?) ON CONFLICT(workspace_id,fingerprint,import_key) DO NOTHING", note.ID, workspaceID, fingerprint, note.Text, note.CreatedAt, importKey)
	if err == nil && importKey != nil {
		err = d.SQL.QueryRowContext(ctx, "SELECT id,text,created_at FROM finding_notes WHERE workspace_id=? AND fingerprint=? AND import_key=?", workspaceID, fingerprint, importKey).Scan(&note.ID, &note.Text, &note.CreatedAt)
	}
	return note, err
}
func (d *DB) DeleteFindingNote(ctx context.Context, workspaceID, fingerprint, id string) (bool, error) {
	result, err := d.SQL.ExecContext(ctx, "DELETE FROM finding_notes WHERE workspace_id=? AND fingerprint=? AND id=?", workspaceID, fingerprint, id)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

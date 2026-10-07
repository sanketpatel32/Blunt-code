CREATE TABLE finding_notes (
 id TEXT PRIMARY KEY,
 workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
 fingerprint TEXT NOT NULL,
 text TEXT NOT NULL,
 import_key TEXT,
 created_at TEXT NOT NULL,
 UNIQUE(workspace_id, fingerprint, import_key)
);
CREATE INDEX finding_notes_scope ON finding_notes(workspace_id, fingerprint, created_at);

-- Fingerprint-based finding suppression: one row per dismissed finding
-- fingerprint per workspace. Scans keep storing findings whose fingerprint is
-- suppressed here, but scan totals, severity counts, reports, exports, and the
-- CI gate exclude them.
CREATE TABLE IF NOT EXISTS suppressed_findings (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  fingerprint TEXT NOT NULL,
  reason TEXT,
  created_at TEXT NOT NULL,
  PRIMARY KEY(workspace_id, fingerprint)
);

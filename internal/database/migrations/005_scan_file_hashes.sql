-- Incremental rescans: per-scan content hashes of every selected file, plus
-- the analyzer identity that produced them. A later incremental scan of the
-- same workspace reuses the previous completed scan's findings for every file
-- whose (path, content hash) still matches; the companion scan_hash_meta row
-- pins the analyzer set (bluntcode version, profile, analyzer id -> version)
-- so any change to it invalidates reuse and forces a full re-run.
CREATE TABLE IF NOT EXISTS scan_file_hashes (
  scan_id TEXT NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
  relative_path TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  PRIMARY KEY(scan_id, relative_path)
);
CREATE INDEX IF NOT EXISTS idx_scan_file_hashes_scan ON scan_file_hashes(scan_id);
CREATE TABLE IF NOT EXISTS scan_hash_meta (
  scan_id TEXT PRIMARY KEY REFERENCES scans(id) ON DELETE CASCADE,
  analyzers_json TEXT NOT NULL
);

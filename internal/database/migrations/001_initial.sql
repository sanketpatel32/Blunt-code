CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS workspaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  root_path TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  last_opened_at TEXT,
  last_scan_at TEXT,
  default_profile TEXT NOT NULL,
  settings_json TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE IF NOT EXISTS workspace_rules (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  rule_type TEXT NOT NULL CHECK(rule_type IN ('include', 'exclude')),
  pattern TEXT NOT NULL,
  source TEXT NOT NULL CHECK(source IN ('default', 'user')),
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_workspace_rules_workspace ON workspace_rules(workspace_id);
CREATE TABLE IF NOT EXISTS workspace_path_overrides (
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  relative_path TEXT NOT NULL,
  mode TEXT NOT NULL CHECK(mode IN ('include', 'exclude')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(workspace_id, relative_path)
);
CREATE TABLE IF NOT EXISTS scans (
  id TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  state TEXT NOT NULL,
  profile TEXT NOT NULL,
  started_at TEXT,
  finished_at TEXT,
  bluntcode_version TEXT,
  git_branch TEXT,
  git_commit TEXT,
  git_dirty INTEGER,
  candidate_file_count INTEGER NOT NULL DEFAULT 0,
  selected_file_count INTEGER NOT NULL DEFAULT 0,
  total_findings INTEGER NOT NULL DEFAULT 0,
  critical_count INTEGER NOT NULL DEFAULT 0,
  high_count INTEGER NOT NULL DEFAULT 0,
  medium_count INTEGER NOT NULL DEFAULT 0,
  low_count INTEGER NOT NULL DEFAULT 0,
  info_count INTEGER NOT NULL DEFAULT 0,
  error_summary TEXT,
  snapshot_json TEXT NOT NULL DEFAULT '{}',
  report_markdown_path TEXT
);
CREATE INDEX IF NOT EXISTS idx_scans_workspace_started ON scans(workspace_id, started_at DESC);
CREATE TABLE IF NOT EXISTS analyzer_runs (
  id TEXT PRIMARY KEY,
  scan_id TEXT NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
  analyzer_id TEXT NOT NULL,
  version TEXT,
  state TEXT NOT NULL,
  started_at TEXT,
  finished_at TEXT,
  duration_ms INTEGER,
  exit_code INTEGER,
  finding_count INTEGER NOT NULL DEFAULT 0,
  warning_count INTEGER NOT NULL DEFAULT 0,
  error_summary TEXT,
  log_path TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_analyzer_runs_scan ON analyzer_runs(scan_id);
CREATE TABLE IF NOT EXISTS findings (
  id TEXT PRIMARY KEY,
  scan_id TEXT NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
  analyzer_run_id TEXT NOT NULL REFERENCES analyzer_runs(id) ON DELETE CASCADE,
  analyzer_id TEXT NOT NULL,
  rule_id TEXT,
  fingerprint TEXT NOT NULL,
  severity TEXT NOT NULL,
  category TEXT NOT NULL,
  title TEXT,
  message TEXT NOT NULL,
  relative_path TEXT,
  start_line INTEGER,
  start_column INTEGER,
  end_line INTEGER,
  end_column INTEGER,
  remediation TEXT,
  documentation_url TEXT,
  raw_severity TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_findings_scan ON findings(scan_id);
CREATE INDEX IF NOT EXISTS idx_findings_fingerprint ON findings(fingerprint);
CREATE TABLE IF NOT EXISTS metrics (
  id TEXT PRIMARY KEY,
  scan_id TEXT NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
  analyzer_id TEXT NOT NULL,
  scope TEXT NOT NULL,
  relative_path TEXT,
  metric_key TEXT NOT NULL,
  label TEXT NOT NULL,
  value_text TEXT,
  value_number REAL,
  unit TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE IF NOT EXISTS scan_files (
  scan_id TEXT NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
  relative_path TEXT NOT NULL,
  language TEXT,
  selected INTEGER NOT NULL,
  analyzed_by_json TEXT NOT NULL DEFAULT '[]',
  skip_reason TEXT,
  size_bytes INTEGER NOT NULL,
  content_hash_optional TEXT,
  PRIMARY KEY(scan_id, relative_path)
);
CREATE TABLE IF NOT EXISTS tool_installations (
  tool_id TEXT NOT NULL,
  version TEXT NOT NULL,
  path TEXT NOT NULL,
  status TEXT NOT NULL,
  checksum TEXT,
  installed_at TEXT,
  last_checked_at TEXT,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  PRIMARY KEY(tool_id, version)
);

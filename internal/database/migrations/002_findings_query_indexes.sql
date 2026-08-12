-- Keep server-side findings filtering and pagination responsive for large scans.
CREATE INDEX IF NOT EXISTS idx_findings_scan_severity ON findings(scan_id, severity);
CREATE INDEX IF NOT EXISTS idx_findings_scan_category ON findings(scan_id, category);
CREATE INDEX IF NOT EXISTS idx_findings_scan_analyzer ON findings(scan_id, analyzer_id);
CREATE INDEX IF NOT EXISTS idx_findings_scan_path ON findings(scan_id, relative_path, start_line);

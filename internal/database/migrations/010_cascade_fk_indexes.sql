-- Indexes for every ON DELETE CASCADE column that lacked one. Without them a
-- workspace delete re-scans the whole child table once per deleted parent row
-- (findings for each analyzer_run, metrics for each scan), which took 6-9s on
-- real databases and held the single write connection the whole time.
CREATE INDEX IF NOT EXISTS idx_findings_analyzer_run ON findings(analyzer_run_id);
CREATE INDEX IF NOT EXISTS idx_metrics_scan ON metrics(scan_id);

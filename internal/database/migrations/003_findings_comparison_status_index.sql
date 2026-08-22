-- Comparison-status EXISTS probes (previous.scan_id=? AND previous.fingerprint=?)
-- previously picked a scan_id-only index and linearly scanned every finding of
-- the previous scan per row (measured: ~27ms per probe at 10k baseline findings,
-- turning the CSV export of a 50k scan into minutes). The composite index turns
-- each probe into a covering equality seek (measured: 63x faster page loads).
CREATE INDEX IF NOT EXISTS idx_findings_scan_fingerprint ON findings(scan_id, fingerprint);

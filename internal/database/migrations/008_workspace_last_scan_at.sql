-- Backfill the denormalized workspace scan timestamp from terminal scans.
-- New scans stamp workspaces.last_scan_at when they finish; this heals rows
-- written before that behavior existed without touching nonterminal scans.
UPDATE workspaces
SET last_scan_at = (
    SELECT COALESCE(scans.finished_at, scans.started_at)
    FROM scans
    WHERE scans.workspace_id = workspaces.id
      AND scans.state IN ('completed', 'completed_with_warnings', 'failed', 'cancelled', 'interrupted')
    ORDER BY scans.started_at DESC, scans.id DESC
    LIMIT 1
)
WHERE EXISTS (
    SELECT 1
    FROM scans
    WHERE scans.workspace_id = workspaces.id
      AND scans.state IN ('completed', 'completed_with_warnings', 'failed', 'cancelled', 'interrupted')
);

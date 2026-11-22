import { useRef, useState } from 'react';
import { api } from '../api';
import { useLoad } from '../hooks/useLoad';
import { formatBytes } from '../lib/format';
import { Button } from './ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from './ui/dialog';

export function ScanReviewDialog({ workspaceId, workspaceName, profile, confirmLabel, busy, onCancel, onConfirm }: {
  workspaceId: string; workspaceName?: string; profile: string; confirmLabel: string; busy: boolean;
  onCancel: () => void; onConfirm: (opts: { incremental: boolean }) => void;
}) {
  const returnFocus = useRef(document.activeElement instanceof HTMLElement ? document.activeElement : null);
  const [incremental, setIncremental] = useState(false);
  const review = useLoad((signal) => api.scanPlan(workspaceId, profile, { signal }), [workspaceId, profile]);
  const plan = review.data;
  const validPlan = plan && Array.isArray(plan.analyzers) && typeof plan.selected_files === 'number';
  const planned = validPlan ? plan.analyzers.filter((row) => row.planned) : [];
  const setupRows = planned.filter((row) => !row.ready && Boolean(row.managed_tool));
  const setup = validPlan ? plan.setup : undefined;
  const setupBytes = setup?.download_bytes ?? 0;
  const setupSize = setupBytes > 0
    ? `${setup?.estimated ? 'about ' : ''}${formatBytes(setupBytes)} of downloads${setup?.estimated ? ' (estimate; some tools fetch extra dependencies)' : ''}`
    : 'an unknown amount of downloads';
  const setupNames = setupRows.map((row) => row.display_name).join(', ');
  return <Dialog open onOpenChange={(open) => { if (!open && !busy) onCancel(); }}>
    <DialogContent className="max-w-2xl" onCloseAutoFocus={(event) => { event.preventDefault(); returnFocus.current?.focus(); }}>
      <DialogHeader>
        {/* The workspace name lives in the description, not the question — a
            long name truncated mid-token left a dangling "…name-?" line that
            read like the dialog doubting itself. */}
        <DialogTitle>Run {profile} scan?</DialogTitle>
        <DialogDescription>{workspaceName ? <>on <strong className="break-all">{workspaceName}</strong> — </> : null}Review current inputs, analyzer readiness, and network use. Files are rediscovered when the scan starts; this review does not modify your project.</DialogDescription>
      </DialogHeader>
      {review.loading ? <p role="status">Reviewing workspace inputs…</p> : review.error || !validPlan ? <div role="alert" className="availability-note">
        <p>{review.error ?? 'The server did not return a scan plan.'}</p>
        <Button variant="outline" onClick={() => void review.reload()}>Retry review</Button>
      </div> : <>
        <p><strong>{plan.selected_files}</strong> selected source files, {plan.candidate_files} candidates, {plan.dependency_inputs?.length ?? 0} dependency inputs</p>
        <p className="text-xs text-[var(--color-ink-soft)]">{plan.exclusions?.length ?? 0} exclusion patterns, {Object.values(plan.skip_counts ?? {}).reduce((sum, count) => sum + count, 0)} skipped entries</p>
        <div className="scan-review-list" role="group" aria-label="Analyzer plan">
          {plan.analyzers.map((row) => <section key={row.id} className="scan-review-row">
            <header><strong>{row.display_name}</strong><span>{row.planned ? row.ready ? 'Ready' : 'Setup required' : 'Skipped'}</span></header>
            <p>{row.reason}{row.planned && row.detail ? ` · ${row.detail}` : ''}</p>
            {row.planned && <p>Network: {row.network === 'none' ? 'No outbound requests during analysis' : row.network === 'loopback-only' ? 'Local server on this computer' : 'May contact external services'}{row.network_note ? ` · ${row.network_note}` : ''}</p>}
          </section>)}
        </div>
        {setup && setup.tools > 0 && setupRows.length > 0 && <p className="availability-note"><strong>First run setup:</strong> {setupNames} {setupRows.length === 1 ? 'needs' : 'need'} managed tools; expect {setupSize}. This can take several minutes.</p>}
        <p className="availability-note">Managed tool setup can download software. Missing or failed analyzers reduce coverage. You can cancel a running scan from its scan page.</p>
        <label className="flex items-center gap-2 text-sm text-[var(--color-ink-soft)]">
          <input type="checkbox" checked={incremental} onChange={(event) => setIncremental(event.target.checked)} />
          Incremental — re-scan only files changed since the last completed scan (falls back to a full scan when there is none)
        </label>
        {planned.length === 0 && <p role="alert">No analyzers have applicable inputs for this profile. Choose another profile or update file selection.</p>}
      </>}
      <DialogFooter>
        <Button variant="outline" onClick={onCancel} disabled={busy}>Cancel</Button>
        <Button onClick={() => onConfirm({ incremental })} disabled={busy || review.loading || !!review.error || !validPlan || planned.length === 0}>{busy ? 'Starting…' : confirmLabel}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}

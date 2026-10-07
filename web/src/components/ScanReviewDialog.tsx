import { useRef } from 'react';
import { api } from '../api';
import { useLoad } from '../hooks/useLoad';
import { Button } from './ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from './ui/dialog';

export function ScanReviewDialog({ workspaceId, workspaceName, profile, confirmLabel, busy, onCancel, onConfirm }: {
  workspaceId: string; workspaceName?: string; profile: string; confirmLabel: string; busy: boolean;
  onCancel: () => void; onConfirm: () => void;
}) {
  const returnFocus = useRef(document.activeElement instanceof HTMLElement ? document.activeElement : null);
  const review = useLoad(() => api.scanPlan(workspaceId, profile), [workspaceId, profile]);
  const plan = review.data;
  const validPlan = plan && Array.isArray(plan.analyzers) && typeof plan.selected_files === 'number';
  const planned = validPlan ? plan.analyzers.filter((row) => row.planned) : [];
  return <Dialog open onOpenChange={(open) => { if (!open && !busy) onCancel(); }}>
    <DialogContent className="max-w-2xl" onCloseAutoFocus={(event) => { event.preventDefault(); returnFocus.current?.focus(); }}>
      <DialogHeader>
        <DialogTitle>Run {profile} scan{workspaceName ? ` on ${workspaceName}` : ''}?</DialogTitle>
        <DialogDescription>Review current inputs, analyzer readiness, and network use. Files are rediscovered when the scan starts; this review does not modify your project.</DialogDescription>
      </DialogHeader>
      {review.loading ? <p role="status">Reviewing workspace inputs…</p> : review.error || !validPlan ? <div role="alert" className="availability-note">
        <p>{review.error ?? 'The server did not return a scan plan.'}</p>
        <Button variant="outline" onClick={() => void review.reload()}>Retry review</Button>
      </div> : <>
        <p><strong>{plan.selected_files}</strong> selected source files · {plan.candidate_files} candidates · {plan.dependency_inputs?.length ?? 0} dependency inputs</p>
        <p className="text-xs text-[var(--color-ink-soft)]">{plan.exclusions?.length ?? 0} exclusion patterns · {Object.values(plan.skip_counts ?? {}).reduce((sum, count) => sum + count, 0)} skipped entries</p>
        <div className="scan-review-list" aria-label="Analyzer plan">
          {plan.analyzers.map((row) => <section key={row.id} className="scan-review-row">
            <header><strong>{row.display_name}</strong><span>{row.planned ? row.ready ? 'Ready' : 'Setup required' : 'Skipped'}</span></header>
            <p>{row.reason}{row.planned && row.detail ? ` · ${row.detail}` : ''}</p>
            {row.planned && <p>Network: {row.network === 'none' ? 'No outbound requests during analysis' : row.network === 'loopback-only' ? 'Local server on this computer' : 'May contact external services'}{row.network_note ? ` · ${row.network_note}` : ''}</p>}
          </section>)}
        </div>
        <p className="availability-note">Managed tool setup can download software. Missing or failed analyzers reduce coverage. You can cancel a running scan from its scan page.</p>
        {planned.length === 0 && <p role="alert">No analyzers have applicable inputs for this profile. Choose another profile or update file selection.</p>}
      </>}
      <DialogFooter>
        <Button variant="outline" onClick={onCancel} disabled={busy}>Cancel</Button>
        <Button onClick={onConfirm} disabled={busy || review.loading || !!review.error || !validPlan || planned.length === 0}>{busy ? 'Starting…' : confirmLabel}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}

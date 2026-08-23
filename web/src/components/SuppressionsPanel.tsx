import { useState } from 'react';
import { api } from '../api';
import type { Suppression } from '../types';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { date } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { SkeletonLines } from './skeletons';

/** Fingerprints are 64-character sha256 hex; that is unreadable in a list, so rows lead with a short prefix and carry the full value on hover. */
const FINGERPRINT_VISIBLE = 12;

function shortFingerprint(fingerprint: string) {
  return fingerprint.length > FINGERPRINT_VISIBLE ? `${fingerprint.slice(0, FINGERPRINT_VISIBLE)}…` : fingerprint;
}

/**
 * Workspace-level suppression management (sits under the severity trend):
 * every fingerprint dismissed from any scan, newest context first, each with a
 * Restore action. Supplementary data only — loading shows a skeleton, failure a
 * quiet inline retry, and neither ever blocks the rest of the page.
 */
export function SuppressionsSection({ workspaceId, notify }: { workspaceId: string; notify: (n: Notice) => void }) {
  const suppressions = useLoad(() => api.suppressions(workspaceId), [workspaceId]);
  const [restoring, setRestoring] = useState<string>();
  const restore = async (fingerprint: string) => {
    setRestoring(fingerprint);
    try {
      await api.removeSuppression(workspaceId, fingerprint);
      notify({ kind: 'success', text: 'Finding restored. It will be counted again on the next scan.' });
      await suppressions.reload();
    } catch (e) {
      notify({ kind: 'error', text: message(e) });
    } finally {
      setRestoring(undefined);
    }
  };
  const header = <div className="section-head"><div><h2 id="suppressions-title">Suppressions</h2><p>Findings hidden from future scans, reports, and the CI gate.</p></div></div>;
  if (suppressions.loading) {
    return <section className="suppressions-section" aria-labelledby="suppressions-title" aria-busy="true">{header}<SkeletonLines lines={3} /></section>;
  }
  if (suppressions.error) {
    return <section className="suppressions-section"><p className="muted">Suppressions are unavailable right now. <button type="button" className="text-button" onClick={suppressions.reload}>Try again</button></p></section>;
  }
  const items = suppressions.data ?? [];
  return <section className="suppressions-section" aria-labelledby="suppressions-title">
    {header}
    {items.length
      ? <ul className="suppressions-list">{items.map((suppression) => <SuppressionRow suppression={suppression} busy={restoring === suppression.fingerprint} key={suppression.fingerprint} onRestore={restore} />)}</ul>
      : <p className="muted">Nothing is suppressed. Use “Suppress…” on a finding in any scan report to hide it from future scans.</p>}
  </section>;
}

function SuppressionRow({ suppression, busy, onRestore }: { suppression: Suppression; busy: boolean; onRestore: (fingerprint: string) => void }) {
  const short = shortFingerprint(suppression.fingerprint);
  return <li className="suppression-row">
    <code className="suppression-fingerprint" title={suppression.fingerprint}>{short}</code>
    <span className="suppression-reason">{suppression.reason || 'No reason given'}</span>
    <time className="suppression-date" dateTime={suppression.created_at}>{date(suppression.created_at)}</time>
    <button type="button" className="text-button" aria-label={`Restore ${short}`} title={`Stop hiding ${suppression.fingerprint}`} disabled={busy} onClick={() => void onRestore(suppression.fingerprint)}>{busy ? 'Restoring…' : 'Restore'}</button>
  </li>;
}

import { useState } from 'react';
import { api } from '../api';
import type { Suppression } from '../types';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { date } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { useDebouncedValue } from '../hooks/useDebouncedValue';
import { SkeletonLines } from './skeletons';

/** Fingerprints are 64-character sha256 hex; that is unreadable in a list, so rows lead with a short prefix and carry the full value on hover. */
const FINGERPRINT_VISIBLE = 12;
// Short enough to feel instant, long enough that typing never renders intermediate lists.
const SEARCH_DEBOUNCE_MS = 120;

function shortFingerprint(fingerprint: string) {
  return fingerprint.length > FINGERPRINT_VISIBLE ? `${fingerprint.slice(0, FINGERPRINT_VISIBLE)}…` : fingerprint;
}

/** Client-side match over the searchable text a row can carry; case-insensitive and prefix-friendly so a hex fragment finds its fingerprint. */
function matchesSearch(suppression: Suppression, needle: string) {
  return `${suppression.reason ?? ''} ${suppression.fingerprint}`.toLowerCase().includes(needle);
}

/**
 * Workspace-level suppression management (sits under the severity trend):
 * every fingerprint dismissed from any scan, newest context first, each with a
 * Restore action and a debounced client-side search. Supplementary data only —
 * loading shows a skeleton, failure a quiet inline retry, and neither ever
 * blocks the rest of the page.
 */
export function SuppressionsSection({ workspaceId, notify }: { workspaceId: string; notify: (n: Notice) => void }) {
  const suppressions = useLoad(() => api.suppressions(workspaceId), [workspaceId]);
  const [query, setQuery] = useState('');
  // Clearing applies immediately (delay 0); typing settles before the list re-filters.
  const debouncedQuery = useDebouncedValue(query, query ? SEARCH_DEBOUNCE_MS : 0);
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
  const needle = debouncedQuery.trim().toLowerCase();
  const visible = needle ? items.filter((suppression) => matchesSearch(suppression, needle)) : items;
  return <section className="suppressions-section" aria-labelledby="suppressions-title">
    {header}
    {items.length > 0 && <div className="suppressions-toolbar">
      <label className="search"><span>Search suppressions</span><input type="search" value={query} placeholder="Reason or fingerprint" onChange={(event) => setQuery(event.target.value)} onKeyDown={(event) => { if (event.key === 'Escape') setQuery(''); }} /></label>
      {needle !== '' && <p className="suppressions-count" role="status">{visible.length} of {items.length} {items.length === 1 ? 'suppression' : 'suppressions'} shown</p>}
    </div>}
    {items.length === 0
      ? <p className="muted">Nothing is suppressed. Use “Suppress…” on a finding in any scan report to hide it from future scans.</p>
      : visible.length
        ? <ul className="suppressions-list">{visible.map((suppression) => <SuppressionRow suppression={suppression} busy={restoring === suppression.fingerprint} key={suppression.fingerprint} onRestore={restore} />)}</ul>
        : <p className="muted">No suppressions match “{needle}”. Clear the search to see all {items.length}.</p>}
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

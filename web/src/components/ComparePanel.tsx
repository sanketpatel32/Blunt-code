import { useState } from 'react';
import { CalendarDays } from 'lucide-react';
import { ApiError, api } from '../api';
import type { CompareResult, Finding, Scan } from '../types';
import { analyzerName, findingLocation, preciseDate } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel } from './ui';
import { SkeletonLines } from './skeletons';

/** Rows shown inline per section before the "+N more" details expansion. */
const SECTION_VISIBLE_LIMIT = 10;

interface CompareSide { current: Scan; previous: Scan; comparison: CompareResult }

/** Stamp a scan sorts by: when it produced results, falling back to when it started. */
function scanStamp(scan: Scan) {
  const time = new Date(scan.finished_at ?? scan.started_at ?? '').getTime();
  return Number.isNaN(time) ? 0 : time;
}

/** Comparison of two scans, rendered above the history table. Accepts the ids
 *  in EITHER order (deep links can carry ?compare=&with= either way): both
 *  scans are loaded, sorted newest-first, and the diff is requested as
 *  (newer, older) so "new" and "fixed" read correctly. */
export function ComparePanel({ aId, bId, onClose }: { aId: string; bId: string; onClose?: () => void }) {
  const loaded = useLoad<CompareSide>(async () => {
    const [a, b] = await Promise.all([api.scan(aId), api.scan(bId)]);
    if (!a || !b) throw new ApiError('SCAN_NOT_FOUND', 'One of these scans could not be found.');
    const [current, previous] = scanStamp(a) >= scanStamp(b) ? [a, b] : [b, a];
    return { current, previous, comparison: await api.compareScans(current.id, previous.id) };
  }, [aId, bId]);

  if (loaded.loading) return <section className="compare-panel" aria-busy="true" aria-label="Scan comparison"><SkeletonLines lines={3} /></section>;
  if (loaded.error) return <div className="compare-panel"><ErrorPanel error={loaded.error} retry={loaded.reload} /></div>;
  const { current, previous, comparison } = loaded.data!;
  if (!comparison.available) {
    return <div className="compare-panel"><Empty title="Comparison unavailable">These scans can't be compared (one has no previous completed scan).</Empty></div>;
  }
  const summary = comparison.summary ?? {
    new: comparison.new?.length ?? 0,
    fixed: comparison.fixed?.length ?? 0,
    persistent: comparison.persistent?.length ?? 0,
  };
  const notEvaluated = comparison.not_evaluated ?? [];
  return (
    <section className="compare-panel" aria-label="Scan comparison">
      <header className="compare-panel-header">
        <div className="compare-panel-title"><h2>Scan comparison</h2>{onClose && <button type="button" className="text-button" onClick={onClose}>Close comparison</button>}</div>
        <p className="compare-headline">
          <span className="sr-only">Since {preciseDate(previous.finished_at ?? previous.started_at)}: </span>
          <span className="compare-stat" data-tone={summary.new > 0 ? 'danger' : undefined}><strong className="compare-count tabular-nums">{summary.new}</strong> new</span>
          <span className="sr-only">, </span>
          <span className="compare-stat" data-tone={summary.fixed > 0 ? 'ok' : undefined}><strong className="compare-count tabular-nums">{summary.fixed}</strong> fixed</span>
          <span className="sr-only">, </span>
          <span className="compare-stat"><strong className="compare-count tabular-nums">{summary.persistent}</strong> still present</span>
        </p>
        <p className="compare-sub"><CalendarDays aria-hidden="true" size={13} /> {preciseDate(previous.finished_at ?? previous.started_at)}{previous.profile ? ` · ${previous.profile}` : ''} <span aria-hidden="true">→</span><span className="sr-only"> compared with </span> {preciseDate(current.finished_at ?? current.started_at)}{current.profile ? ` · ${current.profile}` : ''}</p>
        {notEvaluated.length > 0 && <p className="compare-note">Some analyzers did not finish in the newer scan, so their findings are not counted: {notEvaluated.join(', ')}.</p>}
      </header>
      <CompareSection label="New" items={comparison.new ?? []} moreNoun="new" />
      <CompareSection label="Fixed" items={comparison.fixed ?? []} moreNoun="fixed" />
      <CompareSection label="Still present" items={comparison.persistent ?? []} moreNoun="still present" />
    </section>
  );
}

/** One collapsible group of findings; hidden entirely when the diff has none (the headline already says 0). Section headings carry no count chips — the headline owns the numbers. */
function CompareSection({ label, items, moreNoun }: { label: string; items: Finding[]; moreNoun: string }) {
  const [open, setOpen] = useState(true);
  if (items.length === 0) return null;
  const visible = items.slice(0, SECTION_VISIBLE_LIMIT);
  const rest = items.slice(SECTION_VISIBLE_LIMIT);
  return (
    <section className="compare-section">
      <h3 className="compare-section-heading">
        <button type="button" className="compare-section-toggle" aria-expanded={open} onClick={() => setOpen((value) => !value)}>
          <span className="disclose-arrow" aria-hidden="true">▸</span>
          {label}
        </button>
      </h3>
      {open && <>
        <ul className="what-changed-list">{visible.map((finding) => <CompareRow finding={finding} key={finding.id || finding.fingerprint} />)}</ul>
        {rest.length > 0 && <details className="what-changed-more"><summary>+{rest.length} more {moreNoun}</summary><ul className="what-changed-list">{rest.map((finding) => <CompareRow finding={finding} key={finding.id || finding.fingerprint} />)}</ul></details>}
      </>}
    </section>
  );
}

/** Same row pattern as ScanPage's WhatChanged (severity pill · title · path:line · tool badge), reusing its global row styles. */
function CompareRow({ finding }: { finding: Finding }) {
  return <li className="what-changed-row"><span className={`severity ${finding.severity}`}>{finding.severity}</span><span className="what-changed-rule">{finding.title ?? finding.rule_id ?? 'Finding'}</span><code>{findingLocation(finding)}</code><span className="badge">{analyzerName(finding.analyzer_id)}</span></li>;
}

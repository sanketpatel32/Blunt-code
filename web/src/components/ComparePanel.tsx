import { useState } from 'react';
import { ApiError, api } from '../api';
import type { CompareResult, Finding, Scan } from '../types';
import { analyzerName, date, findingLocation } from '../lib/format';
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
export function ComparePanel({ aId, bId }: { aId: string; bId: string }) {
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
        <h2>Scan comparison</h2>
        <p className="compare-headline">
          Since <strong>{date(previous.finished_at ?? previous.started_at)}</strong>:{' '}
          <strong className="compare-count">{summary.new}</strong> new ·{' '}
          <strong className="compare-count">{summary.fixed}</strong> fixed ·{' '}
          <strong className="compare-count">{summary.persistent}</strong> still present
        </p>
        <p className="compare-sub">Comparing {date(current.finished_at ?? current.started_at)} with {date(previous.finished_at ?? previous.started_at)}</p>
        {notEvaluated.length > 0 && <p className="compare-note">Some analyzers did not finish in the newer scan, so their findings are not counted: {notEvaluated.join(', ')}.</p>}
      </header>
      <CompareSection label="New" count={summary.new} items={comparison.new ?? []} moreNoun="new" />
      <CompareSection label="Fixed" count={summary.fixed} items={comparison.fixed ?? []} moreNoun="fixed" />
      <CompareSection label="Still present" count={summary.persistent} items={comparison.persistent ?? []} moreNoun="still present" />
    </section>
  );
}

/** One collapsible group of findings; hidden entirely when the diff has none (the headline already says 0). */
function CompareSection({ label, count, items, moreNoun }: { label: string; count: number; items: Finding[]; moreNoun: string }) {
  const [open, setOpen] = useState(true);
  if (items.length === 0) return null;
  const visible = items.slice(0, SECTION_VISIBLE_LIMIT);
  const rest = items.slice(SECTION_VISIBLE_LIMIT);
  return (
    <section className="compare-section">
      <h3 className="compare-section-heading">
        <button type="button" className="compare-section-toggle" aria-expanded={open} onClick={() => setOpen((value) => !value)}>
          <span className="disclose-arrow" aria-hidden="true">▸</span>
          {label} <span className="compare-section-count tabular-nums">{count}</span>
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

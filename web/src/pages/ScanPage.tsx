import { useEffect, useRef, useState } from 'react';
import { api } from '../api';
import type { AnalyzerRun, Finding, Scan } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { analyzerName, date, elapsed, findingLocation, scanStateDisplay } from '../lib/format';
import { eventCopy, isTerminalScanState, liveHeadline, stageLabels, type ScanEvent } from '../lib/scanEvents';
import { bandFor, riskGrade, riskScore, severityCountsOf } from '../lib/risk';
import { useLoad } from '../hooks/useLoad';
import { useTicker } from '../hooks/useTicker';
import { ErrorPanel, Loading, SeverityCounts } from '../components/ui';
import { SkeletonLines } from '../components/skeletons';
import { ReportView } from './report/ReportView';
import { pushNotification } from '../lib/notifications';
import { analyzerMeta, categoryColor, CATEGORY_LABELS } from '../lib/analyzerCatalog';
import { useReducedMotion } from '../hooks/useReducedMotion';
import { Button } from '../components/ui/button';

export type { ScanEvent } from '../lib/scanEvents';

export function ScanPage({ id, go, notify }: { id: string; go?: (r: Route) => void; notify: (n: Notice) => void }) {
  const scan = useLoad(() => api.scan(id), [id]);
  const [events, setEvents] = useState<ScanEvent[]>([]);
  const eventSeq = useRef(0);
  const [streamState, setStreamState] = useState<'connecting' | 'live' | 'reconnecting'>('connecting');
  const [streamAttempts, setStreamAttempts] = useState(0);
  const scanState = scan.data?.state;
  const scanReload = scan.reload;
  useEffect(() => {
    if (scanState && isTerminalScanState(scanState)) {
      setStreamState('live');
      return;
    }
    let source: EventSource | undefined; let retry: number | undefined;
    const receive = (event: MessageEvent) => {
      try {
        const envelope = JSON.parse(event.data) as { type?: string; data?: Record<string, unknown> };
        const data = (envelope.data ?? envelope) as Omit<ScanEvent, 'type' | 'at'>;
        const next: ScanEvent = { ...data, type: envelope.type ?? event.type, at: Date.now(), seq: eventSeq.current++ };
        setEvents((old) => [...old.slice(-23), next]);
        setStreamState('live');
        if (next.type === 'scan.completed') { try { pushNotification({ title: 'Scan completed', message: `Scan ${id.slice(0,8)} finished`, kind: 'success' }); } catch {} void scanReload(); } else if (next.type === 'scan.cancelled') void scanReload();
      } catch { /* Invalid optional event payloads do not interrupt the scan. */ }
    };
    let attempt = 0;
    const connect = () => {
      setStreamState((state) => state === 'live' ? 'reconnecting' : state);
      source = new EventSource(`/api/v1/scans/${encodeURIComponent(id)}/events`);
      ['scan.started', 'scan.stage', 'analyzer.started', 'analyzer.completed', 'analyzer.failed', 'analyzer.skipped', 'scan.warning', 'scan.completed', 'scan.cancelled'].forEach((type) => { source?.addEventListener(type, receive); });
      source.addEventListener('connected', () => { attempt = 0; setStreamAttempts(0); setStreamState('live'); });
      source.onerror = () => {
        source?.close();
        setStreamState('reconnecting');
        attempt += 1;
        setStreamAttempts(attempt);
        const delay = Math.min(15_000, Math.round(1000 * Math.pow(1.5, attempt - 1)));
        retry = window.setTimeout(() => { void scanReload(); connect(); }, delay);
      };
    };
    connect();
    return () => { source?.close(); if (retry) window.clearTimeout(retry); };
  }, [id, scanState, scanReload]);
  const current = scan.data;
  const terminal = current && isTerminalScanState(current.state);
  useTicker(!terminal);
  async function cancel() { try { await api.cancelScan(id); await scan.reload(); } catch (e) { notify({ kind: 'error', text: message(e) }); } }
  if (scan.loading) return <div className="page"><Loading /></div>;
  if (scan.error) return <div className="page"><ErrorPanel error={scan.error} retry={scan.reload} /></div>;
  if (!current) return <div className="page"><Loading /></div>;
  const state = scanStateDisplay(current.state);
  const live = !terminal;
  const counts = severityCountsOf(current);
  const total = current.total_findings ?? 0;
  const critical = counts.critical ?? 0;
  const score = riskScore(counts);
  const graded = current.state === 'completed' || current.state === 'completed_with_warnings';
  const grade = graded && total > 0 ? riskGrade(score) : null;
  /** One sentence the whole page hangs on: the verdict, in the same grade language as the dashboard. */
  const headline = live
    ? liveHeadline(events, current.state)
    : current.state === 'failed' ? 'Analysis failed'
      : current.state === 'cancelled' ? 'Analysis cancelled'
        : current.state === 'interrupted' ? 'Scan interrupted — partial results below'
          : total === 0 ? 'All clear — no findings'
            : `${bandFor(riskGrade(score)).label} — ${total} ${total === 1 ? 'finding' : 'findings'}${critical > 0 ? `, ${critical} critical` : ''}`;
  const reportedFindings = events.reduce((acc, event) => acc + (event.type === 'analyzer.completed' ? event.findings ?? 0 : 0), 0);
  const findingsSoFar = Math.max(total, reportedFindings);
  const runs = current.analyzer_runs ?? [];
  const succeeded = runs.filter((run) => run.status === 'succeeded').length;
  /** Card accent follows the worst finding on the page: danger > warning > success > neutral. */
  const heroTone = live ? 'live' : total === 0 ? 'success' : critical > 0 || (counts.high ?? 0) > 0 ? 'danger' : (counts.medium ?? 0) > 0 ? 'warning' : 'neutral';
  const startedText = date(current.started_at);
  const durationText = elapsed(current.started_at, current.finished_at);
  return <div className="page scan-page">
    <header className="scan-hero" data-tone={heroTone}>
      <div className="scan-hero-main">
        <p className="eyebrow">Analysis · {current.profile} profile</p>
        <div className="scan-hero-title-row">
          {graded && grade
            ? <span className="scan-grade" data-grade={grade} aria-hidden="true">{grade}</span>
            : <span className={`scan-grade scan-grade-clear`} aria-hidden="true">✓</span>}
          <div className="scan-hero-copy">
            <h1>{headline}</h1>
            <p className="scan-hero-meta">
              <span>Started {startedText}</span>
              <span aria-hidden="true">·</span>
              <span>{live ? `elapsed ${durationText}` : durationText}</span>
              {runs.length > 0 && <>
                <span aria-hidden="true">·</span>
                <span>{succeeded} of {runs.length} {runs.length === 1 ? 'engine' : 'engines'} succeeded</span>
              </>}
            </p>
          </div>
        </div>
        {live && <div className="scan-hero-progress"><ScanProgressBar scan={current} events={events} /></div>}
      </div>
      <div className="scan-hero-side">
        <span className={`stream-state ${streamState}`} aria-live="polite">
          <i aria-hidden="true" />
          {terminal ? 'Saved report' : streamState === 'live' ? 'Live updates' : streamState === 'reconnecting' ? `Reconnecting${streamAttempts > 1 ? ` (try ${streamAttempts})` : ''}` : 'Connecting'}
        </span>
        <div className="scan-hero-actions">
          {live && (
            <Button variant="destructive" size="sm" onClick={cancel}>
              Cancel scan
            </Button>
          )}
          {go && current.workspace_id && (
            <Button variant="outline" size="sm" onClick={() => go({ page: 'workspace', id: current.workspace_id })}>
              Workspace
            </Button>
          )}
        </div>
      </div>
    </header>
    {live && <section className="progress-layout">
      <ScanStageList scan={current} events={events} />
      <aside className="scan-side" aria-busy={!terminal ? 'true' : 'false'}>
        <div className="scan-side-head">
          <p className="eyebrow">Results so far</p>
          <h2 className="scan-metric"><span className="scan-metric-value">{findingsSoFar}</span> <span className="scan-metric-label">reported so far</span></h2>
        </div>
        <SeverityCounts scan={current} />
        <LiveAnalyzerStrip runs={current.analyzer_runs} events={events} />
        {current.error_summary && <div className="inline-warning">{current.error_summary}</div>}
      </aside>
    </section>}
    {terminal && total > 0 && <section className="analysis-verdict verdict-card" data-tone={heroTone} aria-label="Verdict">
      <div className="verdict-distro">
        <p className="eyebrow">Findings by severity</p>
        <ul className="verdict-bars">
          {(['critical', 'high', 'medium', 'low', 'info'] as const).map((sev) => {
            const count = counts[sev] ?? 0;
            const pct = total > 0 ? Math.max(count > 0 ? 2 : 0, Math.round((count / total) * 100)) : 0;
            return <li key={sev} className={`verdict-bar-row ${count === 0 ? 'is-zero' : ''}`}>
              <span className="verdict-bar-label"><i className={`sev-dot sev-${sev}`} aria-hidden="true" />{sev}</span>
              <span className="verdict-bar-track" role="img" aria-label={`${count} ${sev} findings`}>
                <i className={`verdict-bar-fill sev-${sev}`} style={{ width: `${pct}%` }} />
              </span>
              <span className="verdict-bar-count">{count}</span>
            </li>;
          })}
        </ul>
      </div>
      <dl className="verdict-stats">
        <div className="verdict-stat"><dt>Risk score</dt><dd>{score}<span className="verdict-stat-note">{bandFor(riskGrade(score)).range}</span></dd></div>
        <div className="verdict-stat"><dt>New</dt><dd>{current.new_count ?? 0}</dd></div>
        <div className="verdict-stat"><dt>Fixed</dt><dd>{current.fixed_count ?? 0}</dd></div>
        <div className="verdict-stat"><dt>Engines</dt><dd>{runs.length ? `${succeeded}/${runs.length}` : '—'}</dd></div>
      </dl>
    </section>}
    {terminal && (current.fixed_count ?? 0) > 0 && <WhatChanged scanId={id} fixedCount={current.fixed_count ?? 0} />}
    {terminal && <ReportView scanId={id} notify={notify} runs={current.analyzer_runs ?? []} />}
  </div>;
}

/** Stripe/Linear bento progress: 4px height card, shimmer indeterminate 1.6s, spring determinate width, success/danger terminal. */
function ScanProgressBar({ scan, events }: { scan: Scan; events: ScanEvent[] }) {
  const terminal = isTerminalScanState(scan.state);
  const liveDone = new Set(events.filter((event) => ['analyzer.completed', 'analyzer.failed', 'analyzer.skipped'].includes(event.type)).map((event) => event.analyzer_id ?? event.name));
  const runs = scan.analyzer_runs ?? [];
  const total = runs.length || undefined;
  const doneFromRuns = runs.filter((run) => run.status !== 'running' && run.status !== 'pending').length;
  const done = terminal ? total ?? 1 : Math.max(doneFromRuns, liveDone.size) || undefined;
  let state = 'indeterminate';
  let percent: number | undefined;
  if (terminal) { state = scan.state === 'failed' ? 'failed' : 'done'; percent = 100; }
  else if (total && done !== undefined) { state = 'determinate'; percent = Math.min(100, Math.round((done / total) * 100)); }
  const label = percent !== undefined ? `Scan progress: ${percent}%` : 'Scan in progress';
  return <div className={`scan-progress-card ${state}`} aria-hidden={false}>
    <div className={`scan-progress ${state}`} role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent ?? undefined} aria-busy={!terminal}>
      <i className="progress-fill" style={{ width: percent !== undefined ? `${percent}%` : undefined, willChange: 'width, transform' } as never} />
    </div>
    {percent !== undefined && <span className="scan-progress-meta" style={{ fontVariantNumeric: 'tabular-nums' } as never}>{percent}%</span>}
  </div>;
}

/** Lightweight confetti — 12 dots burst stagger 35ms, will-change for compositor */
function ConfettiStub({ show }: { show: boolean }) {
  const reduced = useReducedMotion();
  if (!show || reduced) return null;
  return <div aria-hidden="true" className="confetti-stub" style={{ display: 'flex', gap: '4px', marginTop: '8px', flexWrap: 'wrap' }}>{Array.from({ length: 12 }, (_, i) => <i key={i} className="confetti-dot" style={{ width: '6px', height: '6px', borderRadius: '50%', background: ['var(--color-accent)','var(--color-success)','var(--color-warning)'][i % 3], display: 'block', animation: 'scan-confetti 520ms var(--ease-out-quart) both', animationDelay: `${i * 35}ms`, willChange: 'transform, opacity' } as never} />)}</div>;
}

/** Live per-analyzer pills grouped by category with colored left border per category, stagger fadeIn. */
export function LiveAnalyzerStrip({ runs, events }: { runs?: AnalyzerRun[]; events: ScanEvent[] }) {
  const started = new Map<string, boolean>();
  for (const event of events) {
    if (event.type === 'analyzer.started') started.set(event.analyzer_id ?? event.name ?? '', true);
  }
  const pills = (runs?.length ? runs.map((run) => ({ id: run.analyzer_id, status: run.status, message: run.message })) : [...started.keys()].map((id) => ({ id, status: 'running', message: undefined as string | undefined })));
  if (!pills.length) return null;
  const grouped = new Map<string, typeof pills>();
  for (const p of pills) {
    const cat = analyzerMeta(p.id)?.category ?? 'other';
    const arr = grouped.get(cat) ?? [];
    arr.push(p);
    grouped.set(cat, arr);
  }
  const reduced = useReducedMotion();
  return <div className="live-analyzers-grouped" aria-label="Analyzer progress">
    {[...grouped.entries()].map(([cat, items], gIdx) => (
      <div key={cat} className="live-category" style={{ borderLeftColor: categoryColor(cat as never) } as never}>
        <span className="live-category-label" style={{ borderLeftColor: categoryColor(cat as never) } as never}>{CATEGORY_LABELS[cat as never] ?? cat}</span>
        <ul className="live-analyzers" aria-label={`${cat} analyzers`}>
          {items.map((pill, pIdx) => {
            const skipped = pill.status === 'skipped';
            const failed = pill.status === 'failed';
            return <li key={pill.id} title={skipped || failed ? (pill.message || (skipped ? 'Skipped — no applicable files or not enabled for this profile' : 'Failed')) : undefined} className={reduced ? '' : 'live-pill-enter'} style={reduced ? undefined : { animationDelay: `${(gIdx * items.length + pIdx) * 40}ms`, willChange: 'transform, opacity' } as never}>
              <span className="badge"><i className="category-dot" style={{ background: categoryColor((analyzerMeta(pill.id)?.category ?? 'security') as never) } as never} aria-hidden="true" />{pill.id}</span>
              <span className={`state ${pill.status}`}>{pill.status}</span>
              {skipped && <span className="text-xs" style={{ color: 'var(--color-ink-faint)', marginLeft: '4px' }} role="note">skipped: {pill.message || 'no applicable files'}</span>}
              {failed && pill.message && <span className="text-xs" style={{ color: 'var(--color-danger)', marginLeft: '4px' }} role="note">{pill.message}</span>}
            </li>;
          })}
        </ul>
      </div>
    ))}
    <div className="category-dots" aria-hidden="true">{[...grouped.keys()].map((cat) => <i key={cat} className="category-dot" style={{ background: categoryColor(cat as never) } as never} title={cat} />)}</div>
  </div>;
}

const FIXED_VISIBLE_LIMIT = 10;
const mediumTime = new Intl.DateTimeFormat(undefined, { timeStyle: 'medium' });

function WhatChanged({ scanId, fixedCount }: { scanId: string; fixedCount: number }) {
  const fixed = useLoad(() => api.fixedFindings(scanId), [scanId]);
  const header = <header className="what-changed-header"><div><h2>What changed</h2><p>Fixed since the previous scan</p></div></header>;
  if (fixed.loading) return <section className="what-changed what-changed-loading" aria-busy="true">{header}<SkeletonLines lines={2} /></section>;
  if (fixed.error || !fixed.data?.comparison_available) return null;
  const list = fixed.data.fixed ?? [];
  const total = fixed.data.total_fixed ?? fixedCount;
  const visible = list.slice(0, FIXED_VISIBLE_LIMIT);
  const rest = list.slice(FIXED_VISIBLE_LIMIT);
  return <section className="what-changed"><header className="what-changed-header"><div><h2>What changed</h2><p>Fixed since the previous scan</p></div><p className="what-changed-headline"><strong>{total}</strong> {total === 1 ? 'finding' : 'findings'} fixed</p></header><ul className="what-changed-list">{visible.map((finding) => <FixedRow finding={finding} key={finding.id || finding.fingerprint} />)}</ul>{rest.length > 0 && <details className="what-changed-more"><summary>+{rest.length} more fixed</summary><ul className="what-changed-list">{rest.map((finding) => <FixedRow finding={finding} key={finding.id || finding.fingerprint} />)}</ul>{total > list.length && <p className="what-changed-note">Showing the top {list.length} of {total} fixed findings.</p>}</details>}</section>;
}

function FixedRow({ finding }: { finding: Finding }) {
  return <li className="what-changed-row"><span className={`severity ${finding.severity}`}>{finding.severity}</span><span className="what-changed-rule">{finding.title ?? finding.rule_id ?? 'Finding'}</span><code>{findingLocation(finding)}</code><span className="badge">{analyzerName(finding.analyzer_id)}</span></li>;
}

function ScanStageList({ scan, events }: { scan: Scan; events: ScanEvent[] }) {
  const journey = events.filter((event) => event.type !== 'scan.started');
  const fallback = scan.state === 'queued' ? 'Waiting for the scan to begin.' : scan.state === 'interrupted' ? 'Scan interrupted before every analyzer finished.' : stageLabels[scan.state] ?? 'Working';
  const entries = journey.length ? journey : [{ type: 'scan.stage', stage: fallback, at: Date.now() } as ScanEvent];
  const active = !isTerminalScanState(scan.state) ? entries.at(-1) : undefined;
  const status = active?.type === 'analyzer.started' ? `${active.name ?? active.analyzer_id ?? 'Analyzer'} is checking your code` : active ? eventCopy(active) : scan.state === 'interrupted' ? 'Scan interrupted — completed checks are still available.' : scan.state.replaceAll('_', ' ');
  const reduced = useReducedMotion();
  const terminal = isTerminalScanState(scan.state);
  const terminalDone = scan.state === 'completed';
  // On completed/failed scans the status line just repeats the page title's
  // outcome; interrupted keeps it — it carries the "partial report saved" note.
  const showStatus = !terminal || scan.state === 'interrupted';
  return <section className="stage-list"><header><div><p className="eyebrow">Analysis flow</p><h2>{terminal ? 'How the analysis ran' : 'What is happening now'}</h2></div><span>{entries.length} update{entries.length === 1 ? '' : 's'}</span></header>{showStatus && <p className={`flow-now ${active ? 'active' : ''}`} role="status" aria-live="polite"><i aria-hidden="true" />{status}</p>}{terminalDone && <ConfettiStub show={terminalDone} />}<ol className="scan-flow">{entries.map((event, index) => { const state = event.type.includes('failed') ? 'failed' : event.type.includes('completed') ? 'done' : event.type.includes('cancelled') ? 'interrupted' : index === entries.length - 1 && !isTerminalScanState(scan.state) ? 'current' : ''; return <li className={`${state} ${reduced ? '' : 'stage-enter'}`} key={event.seq ?? `${event.type}-${event.at}`} style={reduced ? undefined : { animationDelay: `${index * 40}ms`, willChange: 'transform, opacity' } as never}><i className={`flow-marker flow-marker-spring ${state === 'current' ? 'current' : ''}`} aria-hidden="true" style={reduced ? undefined : { willChange: 'transform' } as never} /><div><strong>{eventCopy(event)}</strong><small style={{ fontVariantNumeric: 'tabular-nums' } as never}>{mediumTime.format(event.at)}</small></div></li>; })}</ol></section>;
}

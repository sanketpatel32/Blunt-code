import { useEffect, useRef, useState } from 'react';
import { api } from '../api';
import type { AnalyzerRun, Finding, Scan } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { analyzerName, date, elapsed, findingLocation } from '../lib/format';
import { eventCopy, isTerminalScanState, liveHeadline, stageLabels, type ScanEvent } from '../lib/scanEvents';
import { useLoad } from '../hooks/useLoad';
import { useTicker } from '../hooks/useTicker';
import { ErrorPanel, Loading, SeverityCounts } from '../components/ui';
import { SkeletonLines } from '../components/skeletons';
import { AnalyzerStatuses } from './WorkspaceDetailPage';
import { ReportView } from './report/ReportView';

export type { ScanEvent } from '../lib/scanEvents';

export function ScanPage({ id, notify }: { id: string; go?: (r: Route) => void; notify: (n: Notice) => void }) {
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
        if (next.type === 'scan.completed' || next.type === 'scan.cancelled') void scanReload();
      } catch { /* Invalid optional event payloads do not interrupt the scan. */ }
    };
    // Exponential reconnect backoff: 1s, 1.5s, 2.25s … capped at 15s. A
    // successful open resets the sequence so a healthy stream retries fast
    // again if it later drops.
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
  // Ticking re-render so the elapsed clock advances every second on live scans.
  useTicker(!terminal);
  async function cancel() { try { await api.cancelScan(id); await scan.reload(); } catch (e) { notify({ kind: 'error', text: message(e) }); } }
  if (scan.loading) return <div className="page"><Loading /></div>;
  if (scan.error) return <div className="page"><ErrorPanel error={scan.error} retry={scan.reload} /></div>;
  if (!current) return <div className="page"><Loading /></div>;
  const headline = terminal ? current.state.replaceAll('_', ' ') : liveHeadline(events, current.state);
  const reportedFindings = events.reduce((total, event) => total + (event.type === 'analyzer.completed' ? event.findings ?? 0 : 0), 0);
  const findingsSoFar = Math.max(current.total_findings ?? 0, reportedFindings);
  return <div className="page scan-page"><header className="scan-header"><div><p className="eyebrow">Analysis story</p><h1>{headline}</h1><p>Started {date(current.started_at)} · elapsed {elapsed(current.started_at, current.finished_at)}</p><ScanProgressBar scan={current} events={events} /></div><div className="scan-header-actions"><span className={`stream-state ${streamState}`} aria-live="polite"><i />{terminal ? 'Saved report' : streamState === 'live' ? 'Live updates' : streamState === 'reconnecting' ? `Reconnecting${streamAttempts > 1 ? ` (try ${streamAttempts})` : ''}` : 'Connecting'}</span>{!terminal && <button type="button" className="button danger" onClick={cancel}>Cancel scan</button>}</div></header><section className="progress-layout"><ScanStageList scan={current} events={events} /><aside className="scan-side"><p className="eyebrow">Results so far</p><h2>{findingsSoFar} findings {terminal ? 'collected' : 'reported so far'}</h2><SeverityCounts scan={current} /><LiveAnalyzerStrip runs={current.analyzer_runs} events={events} />{current.error_summary && <div className="inline-warning">{current.error_summary}</div>}<AnalyzerStatuses runs={current.analyzer_runs} /></aside></section>{terminal && (current.fixed_count ?? 0) > 0 && <WhatChanged scanId={id} fixedCount={current.fixed_count ?? 0} />}{terminal && <ReportView scanId={id} notify={notify} />}</div>;
}

/** Thin progress bar under the scan headline. Determinate when analyzer runs
 *  report a known set (finished ÷ total, counting failures as finished); an
 *  animated indeterminate sweep before that. Terminal scans render the full
 *  bar in success tone — or danger when nothing completed cleanly. */
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
  return <div className={`scan-progress ${state}`} role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent ?? undefined} aria-busy={!terminal}><i style={percent !== undefined ? { width: `${percent}%` } : undefined} /></div>;
}

/** Live per-analyzer pills for the side panel: stream events overlay the
 *  persisted analyzer_runs so a pill flips to "done" the moment its completion
 *  event lands, without waiting for the next scan reload. */
function LiveAnalyzerStrip({ runs, events }: { runs?: AnalyzerRun[]; events: ScanEvent[] }) {
  const started = new Map<string, boolean>();
  for (const event of events) {
    if (event.type === 'analyzer.started') started.set(event.analyzer_id ?? event.name ?? '', true);
  }
  const pills = (runs?.length ? runs.map((run) => ({ id: run.analyzer_id, status: run.status })) : [...started.keys()].map((id) => ({ id, status: 'running' })));
  if (!pills.length) return null;
  return <ul className="live-analyzers" aria-label="Analyzer progress">{pills.map((pill) => <li key={pill.id}><span className="badge">{pill.id}</span><span className={`state ${pill.status}`}>{pill.status}</span></li>)}</ul>;
}

/** Compact rows shown before the "<details>" overflow; the endpoint itself caps the list at 100. */
const FIXED_VISIBLE_LIMIT = 10;

/** Hoisted so the event list formats timestamps without rebuilding the formatter per entry. */
const mediumTime = new Intl.DateTimeFormat(undefined, { timeStyle: 'medium' });

/**
 * "What changed" panel for terminal scans with fixes: a success-tone summary of the findings
 * fixed since the previous completed scan. Supplementary data only — comparison gaps and fetch
 * errors stay silent here so the full report below is never blocked.
 */
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
  const entries = journey.length ? journey : [{ type: 'scan.stage', stage: fallback, at: Date.now() }];
  const active = !isTerminalScanState(scan.state) ? entries.at(-1) : undefined;
  const status = active?.type === 'analyzer.started' ? `${active.name ?? active.analyzer_id ?? 'Analyzer'} is checking your code` : active ? eventCopy(active) : scan.state === 'interrupted' ? 'Scan interrupted — completed checks are still available.' : scan.state.replaceAll('_', ' ');
  return <section className="stage-list"><header><div><p className="eyebrow">Analysis flow</p><h2>What is happening now</h2></div><span>{entries.length} update{entries.length === 1 ? '' : 's'}</span></header><p className={`flow-now ${active ? 'active' : ''}`} role="status" aria-live="polite"><i aria-hidden="true" />{status}</p><ol className="scan-flow">{entries.map((event, index) => { const state = event.type.includes('failed') ? 'failed' : event.type.includes('completed') ? 'done' : event.type.includes('cancelled') ? 'interrupted' : index === entries.length - 1 && !isTerminalScanState(scan.state) ? 'current' : ''; return <li className={state} key={event.seq ?? `${event.type}-${event.at}`}><i className="flow-marker" aria-hidden="true" /><div><strong>{eventCopy(event)}</strong><small>{mediumTime.format(event.at)}</small></div></li>; })}</ol></section>;
}

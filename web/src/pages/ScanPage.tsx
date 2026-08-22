import { useEffect, useRef, useState } from 'react';
import { api } from '../api';
import type { Finding, Scan } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { analyzerName, date, elapsed, findingLocation } from '../lib/format';
import { eventCopy, isTerminalScanState, liveHeadline, stageLabels, type ScanEvent } from '../lib/scanEvents';
import { useLoad } from '../hooks/useLoad';
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
    const connect = () => {
      setStreamState((state) => state === 'live' ? 'reconnecting' : state);
      source = new EventSource(`/api/v1/scans/${encodeURIComponent(id)}/events`);
      ['scan.started', 'scan.stage', 'analyzer.started', 'analyzer.completed', 'analyzer.failed', 'analyzer.skipped', 'scan.warning', 'scan.completed', 'scan.cancelled'].forEach((type) => { source?.addEventListener(type, receive); });
      source.addEventListener('connected', () => setStreamState('live'));
      source.onerror = () => { source?.close(); setStreamState('reconnecting'); retry = window.setTimeout(() => { void scanReload(); connect(); }, 4000); };
    };
    connect();
    return () => { source?.close(); if (retry) window.clearTimeout(retry); };
  }, [id, scanState, scanReload]);
  const current = scan.data;
  const terminal = current && isTerminalScanState(current.state);
  async function cancel() { try { await api.cancelScan(id); await scan.reload(); } catch (e) { notify({ kind: 'error', text: message(e) }); } }
  if (scan.loading) return <div className="page"><Loading /></div>;
  if (scan.error) return <div className="page"><ErrorPanel error={scan.error} retry={scan.reload} /></div>;
  if (!current) return <div className="page"><Loading /></div>;
  const headline = terminal ? current.state.replaceAll('_', ' ') : liveHeadline(events, current.state);
  const reportedFindings = events.reduce((total, event) => total + (event.type === 'analyzer.completed' ? event.findings ?? 0 : 0), 0);
  const findingsSoFar = Math.max(current.total_findings ?? 0, reportedFindings);
  return <div className="page scan-page"><header className="scan-header"><div><p className="eyebrow">Analysis story</p><h1>{headline}</h1><p>Started {date(current.started_at)} · elapsed {elapsed(current.started_at, current.finished_at)}</p></div><div className="scan-header-actions"><span className={`stream-state ${streamState}`} aria-live="polite"><i />{terminal ? 'Saved report' : streamState === 'live' ? 'Live updates' : streamState === 'reconnecting' ? 'Reconnecting' : 'Connecting'}</span>{!terminal && <button type="button" className="button danger" onClick={cancel}>Cancel scan</button>}</div></header><section className="progress-layout"><ScanStageList scan={current} events={events} /><aside className="scan-side"><p className="eyebrow">Results so far</p><h2>{findingsSoFar} findings {terminal ? 'collected' : 'reported so far'}</h2><SeverityCounts scan={current} />{current.error_summary && <div className="inline-warning">{current.error_summary}</div>}<AnalyzerStatuses runs={current.analyzer_runs} /></aside></section>{terminal && (current.fixed_count ?? 0) > 0 && <WhatChanged scanId={id} fixedCount={current.fixed_count ?? 0} />}{terminal && <ReportView scanId={id} />}</div>;
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

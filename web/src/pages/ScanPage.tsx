import { useEffect, useState } from 'react';
import { api } from '../api';
import type { Scan } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { date, elapsed } from '../lib/format';
import { eventCopy, isTerminalScanState, liveHeadline, stageLabels, type ScanEvent } from '../lib/scanEvents';
import { useLoad } from '../hooks/useLoad';
import { ErrorPanel, Loading, SeverityCounts } from '../components/ui';
import { AnalyzerStatuses } from './WorkspaceDetailPage';
import { ReportView } from './report/ReportView';

export type { ScanEvent } from '../lib/scanEvents';

export function ScanPage({ id, go, notify }: { id: string; go: (r: Route) => void; notify: (n: Notice) => void }) {
  const scan = useLoad(() => api.scan(id), [id]);
  const [events, setEvents] = useState<ScanEvent[]>([]);
  const [streamState, setStreamState] = useState<'connecting' | 'live' | 'reconnecting'>('connecting');
  useEffect(() => {
    if (scan.data && isTerminalScanState(scan.data.state)) {
      setStreamState('live');
      return;
    }
    let source: EventSource | undefined; let retry: number | undefined;
    const receive = (event: MessageEvent) => {
      try {
        const envelope = JSON.parse(event.data) as { type?: string; data?: Record<string, unknown> };
        const data = (envelope.data ?? envelope) as Omit<ScanEvent, 'type' | 'at'>;
        const next: ScanEvent = { ...data, type: envelope.type ?? event.type, at: Date.now() };
        setEvents((old) => [...old.slice(-23), next]);
        setStreamState('live');
        if (next.type === 'scan.completed' || next.type === 'scan.cancelled') void scan.reload();
      } catch { /* Invalid optional event payloads do not interrupt the scan. */ }
    };
    const connect = () => {
      setStreamState((state) => state === 'live' ? 'reconnecting' : state);
      source = new EventSource(`/api/v1/scans/${encodeURIComponent(id)}/events`);
      ['scan.started', 'scan.stage', 'analyzer.started', 'analyzer.completed', 'analyzer.failed', 'analyzer.skipped', 'scan.warning', 'scan.completed', 'scan.cancelled'].forEach((type) => source?.addEventListener(type, receive));
      source.addEventListener('connected', () => setStreamState('live'));
      source.onerror = () => { source?.close(); setStreamState('reconnecting'); retry = window.setTimeout(() => { void scan.reload(); connect(); }, 4000); };
    };
    connect();
    return () => { source?.close(); if (retry) window.clearTimeout(retry); };
  }, [id, scan.data?.state, scan.reload]);
  const current = scan.data;
  const terminal = current && isTerminalScanState(current.state);
  async function cancel() { try { await api.cancelScan(id); await scan.reload(); } catch (e) { notify({ kind: 'error', text: message(e) }); } }
  if (scan.loading) return <div className="page"><Loading /></div>;
  if (scan.error) return <div className="page"><ErrorPanel error={scan.error} retry={scan.reload} /></div>;
  if (!current) return <div className="page"><Loading /></div>;
  const headline = terminal ? current.state.replaceAll('_', ' ') : liveHeadline(events, current.state);
  const reportedFindings = events.reduce((total, event) => total + (event.type === 'analyzer.completed' ? event.findings ?? 0 : 0), 0);
  const findingsSoFar = Math.max(current.total_findings ?? 0, reportedFindings);
  return <div className="page scan-page"><header className="scan-header"><div><p className="eyebrow">Analysis story</p><h1>{headline}</h1><p>Started {date(current.started_at)} · elapsed {elapsed(current.started_at, current.finished_at)}</p></div><div className="scan-header-actions"><span className={`stream-state ${streamState}`} aria-live="polite"><i />{terminal ? 'Saved report' : streamState === 'live' ? 'Live updates' : streamState === 'reconnecting' ? 'Reconnecting' : 'Connecting'}</span>{!terminal && <button className="button danger" onClick={cancel}>Cancel scan</button>}</div></header><section className="progress-layout"><ScanStageList scan={current} events={events} /><aside className="scan-side"><p className="eyebrow">Results so far</p><h2>{findingsSoFar} findings {terminal ? 'collected' : 'reported so far'}</h2><SeverityCounts scan={current} />{current.error_summary && <div className="inline-warning">{current.error_summary}</div>}<AnalyzerStatuses runs={current.analyzer_runs} /></aside></section>{terminal && <ReportView scanId={id} />}</div>;
}

function ScanStageList({ scan, events }: { scan: Scan; events: ScanEvent[] }) {
  const journey = events.filter((event) => event.type !== 'scan.started');
  const fallback = scan.state === 'queued' ? 'Waiting for the scan to begin.' : scan.state === 'interrupted' ? 'Scan interrupted before every analyzer finished.' : stageLabels[scan.state] ?? 'Working';
  const entries = journey.length ? journey : [{ type: 'scan.stage', stage: fallback, at: Date.now() }];
  const active = !isTerminalScanState(scan.state) ? entries.at(-1) : undefined;
  const status = active?.type === 'analyzer.started' ? `${active.name ?? active.analyzer_id ?? 'Analyzer'} is checking your code` : active ? eventCopy(active) : scan.state === 'interrupted' ? 'Scan interrupted — completed checks are still available.' : scan.state.replaceAll('_', ' ');
  return <section className="stage-list"><header><div><p className="eyebrow">Analysis flow</p><h2>What is happening now</h2></div><span>{entries.length} update{entries.length === 1 ? '' : 's'}</span></header><p className={`flow-now ${active ? 'active' : ''}`} role="status" aria-live="polite"><i aria-hidden="true" />{status}</p><ol className="scan-flow">{entries.map((event, index) => { const state = event.type.includes('failed') ? 'failed' : event.type.includes('completed') ? 'done' : event.type.includes('cancelled') ? 'interrupted' : index === entries.length - 1 && !isTerminalScanState(scan.state) ? 'current' : ''; return <li className={state} key={`${event.type}-${event.at}-${index}`}><i className="flow-marker" aria-hidden="true" /><div><strong>{eventCopy(event)}</strong><small>{new Intl.DateTimeFormat(undefined, { timeStyle: 'medium' }).format(event.at)}</small></div></li>; })}</ol></section>;
}

import { Fragment, useEffect, useState } from 'react';
import { api } from '../api';
import '../css/history.css';
import type { Scan } from '../types';
import type { Route } from '../lib/router';
import { isTerminalScanState } from '../lib/scanEvents';
import { analyzerName, compactDuration, date, relativeTime } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel } from '../components/ui';
import { ScanIcon } from '../components/icons';
import { SkeletonTable } from '../components/skeletons';
import { WorkspaceContextSidebar } from '../components/WorkspaceContext';
import { PageHeader } from '../components/PageHeader';
import { Badge } from '../components/ui/badge';

function useDateFilter() {
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  return { from, to, setFrom, setTo, hasFilter: !!from || !!to };
}

export function HistoryPage({ workspaceId, go }: { workspaceId: string; go: (r: Route) => void }) {
  const [page, setPage] = useState(1);
  const dateFilter = useDateFilter();
  const state = useLoad(() => api.scansPage(workspaceId, page, historyPageSize), [workspaceId, page]);
  useEffect(() => {
    if (state.data && state.data.items.length === 0 && state.data.total > 0 && page > 1) setPage(1);
  }, [state.data, page]);
  return (
    <div className="page workspace-page">
      <WorkspaceContextSidebar id={workspaceId} current={{ page: 'history', id: workspaceId }} onNavigate={go} />
      <div className="workspace-page-body">
        <PageHeader
          eyebrow="History"
          title="Previous analyses"
          badge={state.data ? <Badge variant="secondary" className="text-xs font-mono tabular-nums">{state.data.total} {state.data.total === 1 ? 'scan' : 'scans'}</Badge> : undefined}
          description="Reports and findings are stored only on this computer."
        />
        <div className="history-filter-bar"><label className="history-filter-field"><span className="history-filter-label">From</span><span className="history-filter-input-wrap"><svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7"><rect x="3" y="4" width="18" height="17" rx="2"/><path d="M16 2v4M8 2v4M3 9h18"/></svg><input type="date" value={dateFilter.from} onChange={(e)=>{dateFilter.setFrom(e.target.value); setPage(1);}} className="history-filter-input" aria-label="Filter from date" /></span></label><label className="history-filter-field"><span className="history-filter-label">To</span><span className="history-filter-input-wrap"><svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7"><rect x="3" y="4" width="18" height="17" rx="2"/><path d="M16 2v4M8 2v4M3 9h18"/></svg><input type="date" value={dateFilter.to} onChange={(e)=>{dateFilter.setTo(e.target.value); setPage(1);}} className="history-filter-input" aria-label="Filter to date" /></span></label>{dateFilter.hasFilter && <button type="button" className="history-filter-clear" onClick={()=>{dateFilter.setFrom(''); dateFilter.setTo(''); setPage(1);}} aria-label="Clear date filters">✕ Clear</button>}<span className="history-filter-count tabular-nums" aria-live="polite">{state.data ? `${state.data.total} scan${state.data.total === 1 ? '' : 's'}` : ''}</span></div>{state.loading ? <SkeletonTable rows={6} cols={7} /> : state.error ? <ErrorPanel error={state.error} retry={state.reload} /> : <HistoryTable scans={state.data?.items ?? []} go={go} paging={{ page, pageSize: historyPageSize, total: state.data?.total ?? 0, hasNext: state.data?.has_next ?? false, onPage: setPage }} dateFrom={dateFilter.from} dateTo={dateFilter.to} />}
      </div>
    </div>
  );
}

/** Server-side paging contract handed to HistoryTable by HistoryPage. When absent the table falls back to slicing the given array client-side. */
export interface HistoryPaging {
  page: number;
  pageSize: number;
  total: number;
  hasNext: boolean;
  onPage: (page: number) => void;
}

const historyPageSize = 6;
const barSeverities = ['critical', 'high', 'medium', 'low'] as const;

const DAY_MS = 86_400_000;
export type HistoryBand = 'Today' | 'Yesterday' | 'This week' | 'Earlier';
const bandOrder: HistoryBand[] = ['Today', 'Yesterday', 'This week', 'Earlier'];

function startOfDay(time: number) {
  const day = new Date(time);
  day.setHours(0, 0, 0, 0);
  return day.getTime();
}

function bandOf(scan: Scan, todayStart: number, now: number): HistoryBand {
  const time = new Date(scan.finished_at ?? scan.started_at ?? '').getTime();
  if (Number.isNaN(time)) return 'Earlier';
  const day = startOfDay(time);
  if (day >= todayStart) return 'Today';
  if (day === todayStart - DAY_MS) return 'Yesterday';
  if (now - time < 7 * DAY_MS) return 'This week';
  return 'Earlier';
}

/** Buckets scans by recency of finished_at ?? started_at; missing or unparseable stamps land in Earlier. Only non-empty bands are returned, in chronological order. */
export function historyDateBands(scans: Scan[], now = Date.now()): Array<{ band: HistoryBand; scans: Scan[] }> {
  const todayStart = startOfDay(now);
  const grouped: Record<HistoryBand, Scan[]> = { Today: [], Yesterday: [], 'This week': [], Earlier: [] };
  for (const scan of scans) grouped[bandOf(scan, todayStart, now)].push(scan);
  return bandOrder.filter((band) => grouped[band].length > 0).map((band) => ({ band, scans: grouped[band] }));
}

function findingsCounts(scan: Scan) {
  return { critical: scan.critical_count ?? 0, high: scan.high_count ?? 0, medium: scan.medium_count ?? 0, low: scan.low_count ?? 0 };
}

function segmentWidth(count: number, total: number) {
  return `${Math.round((count * 1000) / total) / 10}%`;
}

function hasMarkdownExport(scan: Scan) {
  return isTerminalScanState(scan.state) && (scan.total_findings ?? 0) > 0;
}

function FindingsCell({ scan }: { scan: Scan }) {
  const counts = findingsCounts(scan);
  const barTotal = counts.critical + counts.high + counts.medium + counts.low;
  const total = scan.total_findings ?? barTotal;
  if (!total) return <span className="findings-zero tabular-nums">0</span>;
  const breakdown = `${counts.critical} critical · ${counts.high} high · ${counts.medium} medium · ${counts.low} low`;
  return <div className="findings-cell"><span className="findings-total tabular-nums">{total}</span>{barTotal > 0 ? <div className="severity-bar stacked severity-bar--pill" role="img" aria-label={breakdown} title={breakdown}>{barSeverities.filter((severity) => counts[severity] > 0).map((severity) => <i key={severity} className={`bar-${severity}`} style={{ width: segmentWidth(counts[severity], barTotal) }} />)}</div> : <div className="severity-bar severity-bar--pill" role="img" aria-label={breakdown} title={breakdown} />}</div>;
}

export function HistoryTable({ scans, go, paging, dateFrom, dateTo }: { scans: Scan[]; go: (r: Route) => void; paging?: HistoryPaging; dateFrom?: string; dateTo?: string }) {
  const [clientPage, setClientPage] = useState(0);
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(new Set());
  const toggleExpanded = (id: string) => setExpanded((current) => { const next = new Set(current); if (next.has(id)) next.delete(id); else next.add(id); return next; });
  const serverMode = !!paging;
  const pageSize = serverMode ? paging!.pageSize : historyPageSize;
  const pageCount = serverMode ? Math.max(1, Math.ceil((paging!.total || scans.length) / paging!.pageSize)) : Math.max(1, Math.ceil(scans.length / historyPageSize));
  const currentPage = serverMode ? Math.min(paging!.page, pageCount) : Math.min(clientPage, pageCount - 1);
  const first = serverMode ? (paging!.page - 1) * paging!.pageSize : currentPage * historyPageSize;
  const dateFiltered = scans.filter((s) => {
    if (!dateFrom && !dateTo) return true;
    const t = new Date(s.finished_at ?? s.started_at ?? '').getTime();
    if (Number.isNaN(t)) return false;
    if (dateFrom && t < new Date(dateFrom).getTime()) return false;
    if (dateTo && t > new Date(dateTo).getTime() + DAY_MS - 1) return false;
    return true;
  });
  const visibleScansRaw = serverMode ? dateFiltered : dateFiltered.slice(first, first + historyPageSize);
  const visibleScans = visibleScansRaw;
  const windowTotal = serverMode ? paging!.total : scans.length;

  const newestScanId = scans[0]?.id;
  useEffect(() => { if (!serverMode && newestScanId !== undefined) setClientPage(0); }, [newestScanId, serverMode]);

  if (!scans.length && !windowTotal) return <Empty title="No scans yet" icon={<span style={{ display: 'grid', placeItems: 'center', width: 48, height: 48, color: 'var(--color-ink-faint)' }}><ScanIcon /></span>}>Analyze this workspace to create the first report.</Empty>;

  return <><div className="table-wrap history-table-wrap history-timeline overflow-x-auto overscroll-x-contain rounded-[var(--radius-card)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)] shadow-[var(--shadow-card)]"><table><caption className="sr-only">Scan history for this workspace</caption><thead className="sticky top-0 z-[1] bg-[var(--color-surface-muted)]"><tr><th scope="col">Date</th><th scope="col">Status</th><th scope="col">Findings</th><th scope="col" className="tabular-nums">New</th><th scope="col" className="tabular-nums">Fixed</th><th scope="col">Duration</th><th scope="col"><span className="sr-only">Actions</span></th></tr></thead><tbody>{historyDateBands(visibleScans).map(({ band, scans: banded }) => <Fragment key={band}><tr className="history-band-row"><th className="history-band" colSpan={7} scope="colgroup" data-count={String(banded.length)} data-date={band}>{band}</th></tr>{banded.map((scan) => {
    const tone = scan.id === scans[0].id ? scan.state === 'failed' ? 'row-danger' : scan.state === 'completed_with_warnings' ? 'row-warning' : '' : '';
    const isOpen = expanded.has(scan.id);
    const runs = scan.analyzer_runs ?? [];
    return <Fragment key={scan.id}><tr className={[tone, isOpen ? 'is-expanded' : ''].filter(Boolean).join(' ') || undefined}><td title={date(scan.finished_at ?? scan.started_at)}><span className="history-date"><button type="button" className="history-disclose" aria-expanded={isOpen} aria-controls={`history-detail-${scan.id}`} aria-label={`${isOpen ? 'Hide' : 'Show'} details for the scan from ${relativeTime(scan.finished_at ?? scan.started_at)}`} onClick={() => toggleExpanded(scan.id)}><span className="disclose-arrow" aria-hidden="true">▸</span></button>{relativeTime(scan.finished_at ?? scan.started_at)}</span></td><td><div className="history-status"><span className={`state ${scan.state}`}>{scan.state.replaceAll('_', ' ')}</span>{scan.profile && <span className="badge profile-badge">{scan.profile}</span>}</div></td><td><FindingsCell scan={scan} /></td><td>{scan.new_count ?? 0}</td><td>{scan.fixed_count ?? 0}</td><td>{compactDuration(scan.duration_ms)}</td><td><div className="table-actions"><button type="button" className="text-button" onClick={() => go({ page: 'scan', id: scan.id })}>Open report</button>{hasMarkdownExport(scan) && <><a className="text-button" href={api.markdownUrl(scan.id)}>Export .md</a><a className="text-button" href={api.exportUrl(scan.id, 'sarif')}>SARIF</a><a className="text-button" href={api.exportUrl(scan.id, 'html')}>HTML</a><a className="text-button" href={api.exportUrl(scan.id, 'json')}>JSON</a></>}</div></td></tr>{isOpen && <tr className="history-detail-row"><td colSpan={7}><div className="history-detail" id={`history-detail-${scan.id}`}><dl className="history-detail-meta"><div><dt>Started</dt><dd>{date(scan.started_at)}</dd></div><div><dt>Finished</dt><dd>{date(scan.finished_at)}</dd></div>{scan.profile && <div><dt>Profile</dt><dd>{scan.profile}</dd></div>}</dl>{scan.error_summary && <div className="inline-warning">Warning: {scan.error_summary}</div>}{runs.length > 0 && <ul className="history-analyzers">{runs.map((run) => <li key={run.analyzer_id}><span>{analyzerName(run.analyzer_id)}</span><span className={`state ${run.status}`}>{run.status.replaceAll('_', ' ')}</span></li>)}</ul>}</div></td></tr>}</Fragment>;
  })}</Fragment>)}</tbody></table></div><nav className="history-pagination" aria-label="Scan history pagination"><span className="history-pagination-count tabular-nums">Showing <strong>{windowTotal ? first + 1 : 0}–{Math.min(first + pageSize, windowTotal)}</strong> of <strong>{windowTotal}</strong> scans</span><div><button type="button" className="button secondary" onClick={() => serverMode ? paging!.onPage(paging!.page - 1) : setClientPage(currentPage - 1)} disabled={serverMode ? paging!.page <= 1 : currentPage === 0}><svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" width="14" height="14"><path d="M15 18 9 12l6-6" strokeLinecap="round" strokeLinejoin="round"/></svg>Previous</button><output aria-live="polite" className="tabular-nums">Page {serverMode ? paging!.page : currentPage + 1} of {pageCount}</output><button type="button" className="button secondary" onClick={() => serverMode ? paging!.onPage(paging!.page + 1) : setClientPage(currentPage + 1)} disabled={serverMode ? !paging!.hasNext : currentPage >= pageCount - 1}>Next<svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" width="14" height="14"><path d="M9 18l6-6-6-6" strokeLinecap="round" strokeLinejoin="round"/></svg></button></div></nav></>;
}

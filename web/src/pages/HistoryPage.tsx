import { Fragment, useEffect, useRef, useState } from 'react';
import { api } from '../api';
import '../css/history.css';
import type { Scan } from '../types';
import type { Route } from '../lib/router';
import { isTerminalScanState } from '../lib/scanEvents';
import { analyzerName, compactDuration, date, relativeTime, scanStateDisplay } from '../lib/format';
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

/** Upper bound on server pages walked for a date-filtered view (50 × page_size rows); guards the loop if a backend ever keeps serving full pages. */
const MAX_FILTER_PAGES = 50;

/** Reads the initial page from `?page=N` so a deep link or reload restores the user's place. */
function initialPageFromUrl(): number {
  const requested = Number(new URLSearchParams(window.location.search).get('page'));
  return Number.isInteger(requested) && requested > 1 ? requested : 1;
}

/** Mirrors the current page into `?page=N` (keeping any other params) without adding a history entry; page 1 removes the param. */
function syncPageParam(next: number) {
  const params = new URLSearchParams(window.location.search);
  if (next > 1) params.set('page', String(next));
  else params.delete('page');
  const query = params.toString();
  window.history.replaceState(null, '', `${window.location.pathname}${query ? `?${query}` : ''}`);
}

interface AllScansPages { items: Scan[]; total: number; }

export function HistoryPage({ workspaceId, go }: { workspaceId: string; go: (r: Route) => void }) {
  const [page, setPage] = useState(initialPageFromUrl);
  const dateFilter = useDateFilter();
  const workspace = useLoad(() => api.workspace(workspaceId), [workspaceId]);
  const state = useLoad(() => api.scansPage(workspaceId, page, historyPageSize), [workspaceId, page]);
  // A date filter must see the WHOLE history, not just the six rows of the
  // current server page — otherwise it silently misses matches living on pages
  // the user cannot see. While a filter is active, walk every server page
  // (capped) and filter the complete set client-side; when it clears, the
  // regular server-paged `state` view takes over again.
  const allPages = useLoad(async (): Promise<AllScansPages> => {
    if (!dateFilter.hasFilter) return { items: [], total: 0 };
    const first = await api.scansPage(workspaceId, 1, historyPageSize);
    const items = [...first.items];
    let lastCount = first.items.length;
    let pagesFetched = 1;
    while (lastCount === historyPageSize && pagesFetched < MAX_FILTER_PAGES) {
      pagesFetched += 1;
      const next = await api.scansPage(workspaceId, pagesFetched, historyPageSize);
      items.push(...next.items);
      lastCount = next.items.length;
    }
    return { items, total: first.total };
  }, [workspaceId, dateFilter.from, dateFilter.to]);
  const filtering = dateFilter.hasFilter;
  const view = filtering ? allPages : state;
  // With a filter active the badge and counter report the number of MATCHES
  // across the whole history; the count line under the table keeps the full
  // workspace total for context ("Showing 1–1 of 16 scans").
  const totalScans = filtering
    ? (allPages.data?.items ?? []).filter((s) => scanMatchesDateRange(s, dateFilter.from, dateFilter.to)).length
    : state.data?.total ?? state.data?.items?.length ?? 0;
  const gotoPage = (next: number) => { setPage(next); syncPageParam(next); };
  useEffect(() => {
    if (state.data && state.data.items.length === 0 && state.data.total > 0 && page > 1) { setPage(1); syncPageParam(1); }
  }, [state.data, page]);
  return (
    <div className="page workspace-page">
      <WorkspaceContextSidebar id={workspaceId} current={{ page: 'history', id: workspaceId }} onNavigate={go} />
      <div className="workspace-page-body">
        <PageHeader
          eyebrow="History"
          title={workspace.data?.name ? `${workspace.data.name} — History` : 'Previous analyses'}
          badge={view.data ? <Badge variant="secondary" className="text-xs font-mono tabular-nums">{totalScans} {totalScans === 1 ? 'scan' : 'scans'}</Badge> : undefined}
          description="Past analysis reports, severity trends, and findings stored locally on this computer."
        />
        <div className="history-filter-bar">
          <label className="history-filter-field"><span className="history-filter-label">From</span><span className="history-filter-input-wrap"><svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7"><rect x="3" y="4" width="18" height="17" rx="2"/><path d="M16 2v4M8 2v4M3 9h18"/></svg><input type="date" value={dateFilter.from} onChange={(e)=>{dateFilter.setFrom(e.target.value); setPage(1); syncPageParam(1);}} className="history-filter-input" aria-label="Filter from date" /></span></label>
          <label className="history-filter-field"><span className="history-filter-label">To</span><span className="history-filter-input-wrap"><svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7"><rect x="3" y="4" width="18" height="17" rx="2"/><path d="M16 2v4M8 2v4M3 9h18"/></svg><input type="date" value={dateFilter.to} onChange={(e)=>{dateFilter.setTo(e.target.value); setPage(1); syncPageParam(1);}} className="history-filter-input" aria-label="Filter to date" /></span></label>
          {dateFilter.hasFilter && <button type="button" className="history-filter-clear" onClick={()=>{dateFilter.setFrom(''); dateFilter.setTo(''); setPage(1); syncPageParam(1);}} aria-label="Clear date filters">✕ Clear</button>}
          <span className="history-filter-count tabular-nums" aria-live="polite">{view.data ? `${totalScans} scan${totalScans === 1 ? '' : 's'}` : ''}</span>
        </div>
        {view.loading ? <SkeletonTable rows={6} cols={6} /> : view.error ? <ErrorPanel error={view.error} retry={view.reload} /> : filtering
          ? <HistoryTable scans={allPages.data?.items ?? []} go={go} paging={{ page: 1, pageSize: historyPageSize, total: allPages.data?.total ?? 0, hasNext: false, onPage: () => {}, filtered: true }} dateFrom={dateFilter.from} dateTo={dateFilter.to} />
          : <HistoryTable scans={state.data?.items ?? []} go={go} paging={{ page, pageSize: historyPageSize, total: state.data?.total ?? 0, hasNext: state.data?.has_next ?? false, onPage: gotoPage }} dateFrom={dateFilter.from} dateTo={dateFilter.to} />}
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
  /** True when `scans` already spans every server page with the date filter applied across all of them: page controls hide and the count line reports the filtered range against the full total. */
  filtered?: boolean;
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

/** Local-day-bounds date match shared by HistoryPage's all-page filtering and
 *  HistoryTable. The date inputs are parsed as LOCAL day bounds
 *  (`new Date('2026-09-06')` alone reads as UTC midnight, so UTC+ users lost
 *  the 00:00-05:30 window). Scans with unparseable dates only match an unset
 *  filter. */
export function scanMatchesDateRange(scan: Scan, dateFrom?: string, dateTo?: string): boolean {
  if (!dateFrom && !dateTo) return true;
  const t = new Date(scan.finished_at ?? scan.started_at ?? '').getTime();
  if (Number.isNaN(t)) return false;
  if (dateFrom && t < new Date(`${dateFrom}T00:00`).getTime()) return false;
  if (dateTo && t > new Date(`${dateTo}T23:59:59.999`).getTime()) return false;
  return true;
}

function findingsCounts(scan: Scan) {
  return { critical: scan.critical_count ?? 0, high: scan.high_count ?? 0, medium: scan.medium_count ?? 0, low: scan.low_count ?? 0 };
}

function segmentWidth(count: number, total: number) {
  return `${Math.round((count * 1000) / total) / 10}%`;
}

/** Width style for a severity-bar segment; a nonzero segment also gets a 2px
 *  floor so small counts (4 critical in 11,228 findings) stay visible instead
 *  of rounding down to an invisible 0% sliver. */
function segmentStyle(count: number, total: number) {
  return { width: segmentWidth(count, total), minWidth: count > 0 ? 2 : undefined };
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
  return <div className="findings-cell"><span className="findings-total tabular-nums">{total}</span>{barTotal > 0 ? <div className="severity-bar stacked severity-bar--pill" role="img" aria-label={breakdown} title={breakdown}>{barSeverities.filter((severity) => counts[severity] > 0).map((severity) => <i key={severity} className={`bar-${severity}`} style={segmentStyle(counts[severity], barTotal)} />)}</div> : <div className="severity-bar severity-bar--pill" role="img" aria-label={breakdown} title={breakdown} />}</div>;
}

/** Human labels for discovery's skip reasons (core.SkipCounts) — why files the
 * walk saw did not become scan inputs. */
const SKIP_LABELS: Record<string, string> = {
  symlink: 'symlinks',
  excluded_default: 'by default rules',
  excluded_user: 'by your exclusions',
  outside_root: 'outside the workspace root',
  generated_content: 'as generated artifacts',
};

function skipCoverageSummary(skip?: Record<string, number>): string {
  if (!skip) return '';
  return Object.entries(skip)
    .filter(([, n]) => n > 0)
    .map(([reason, n]) => `${n} ${SKIP_LABELS[reason] ?? reason.replaceAll('_', ' ')}`)
    .join(', ');
}

export function HistoryTable({ scans, go, paging, dateFrom, dateTo }: { scans: Scan[]; go: (r: Route) => void; paging?: HistoryPaging; dateFrom?: string; dateTo?: string }) {
  const [clientPage, setClientPage] = useState(0);
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(new Set());
  const toggleExpanded = (id: string) => setExpanded((current) => { const next = new Set(current); if (next.has(id)) next.delete(id); else next.add(id); return next; });
  const tableWrapRef = useRef<HTMLDivElement>(null);
  const [scrollCue, setScrollCue] = useState(false);
  // Right-edge scroll cue: at narrow widths most columns hide behind the
  // horizontal scroll with no other hint. The fade drops away once the table
  // fits or the user has scrolled to the end.
  useEffect(() => {
    const el = tableWrapRef.current;
    if (!el) return;
    const update = () => setScrollCue(el.scrollWidth > el.clientWidth + 1 && el.scrollLeft < el.scrollWidth - el.clientWidth - 1);
    update();
    el.addEventListener('scroll', update, { passive: true });
    window.addEventListener('resize', update);
    return () => { el.removeEventListener('scroll', update); window.removeEventListener('resize', update); };
  }, [scans]);
  // Filtered mode: `scans` spans every server page and paging.filtered is set —
  // show every match with no page controls, and let the count line report the
  // filtered range against the full workspace total.
  const filteredMode = !!paging?.filtered;
  const serverMode = !!paging && !filteredMode;
  const pageSize = serverMode ? paging!.pageSize : historyPageSize;
  const pageCount = filteredMode ? 1 : serverMode ? Math.max(1, Math.ceil((paging!.total || scans.length) / paging!.pageSize)) : Math.max(1, Math.ceil(scans.length / historyPageSize));
  const currentPage = filteredMode ? 0 : serverMode ? Math.min(paging!.page, pageCount) : Math.min(clientPage, pageCount - 1);
  const first = filteredMode ? 0 : serverMode ? (paging!.page - 1) * paging!.pageSize : currentPage * historyPageSize;
  const dateFiltered = scans.filter((s) => scanMatchesDateRange(s, dateFrom, dateTo));
  const visibleScans = filteredMode || serverMode ? dateFiltered : dateFiltered.slice(first, first + historyPageSize);
  const windowTotal = paging ? paging.total : scans.length;
  const shownFrom = filteredMode ? (windowTotal ? 1 : 0) : windowTotal ? first + 1 : 0;
  const shownTo = filteredMode ? dateFiltered.length : Math.min(first + pageSize, windowTotal);

  const newestScanId = scans[0]?.id;
  useEffect(() => { if (!serverMode && newestScanId !== undefined) setClientPage(0); }, [newestScanId, serverMode]);

  if (!scans.length && !windowTotal) return <Empty title="No scans yet" icon={<span style={{ display: 'grid', placeItems: 'center', width: 48, height: 48, color: 'var(--color-ink-faint)' }}><ScanIcon /></span>}>Analyze this workspace to create the first report.</Empty>;
  // Active date filters that match nothing earn an empty state, not a bare
  // table with "Showing 0–0 of 0". (A transient over-range server page — empty
  // slice but total > 0 with no filters — keeps the pagination below; the page
  // resets itself to 1.)
  if (dateFiltered.length === 0 && (dateFrom || dateTo)) return <Empty title="No scans match these dates" icon={<span style={{ display: 'grid', placeItems: 'center', width: 48, height: 48, color: 'var(--color-ink-faint)' }}><ScanIcon /></span>}>Try widening the range or clearing the date filters.</Empty>;

  return <><div style={{ position: 'relative' }}><div ref={tableWrapRef} className="table-wrap history-table-wrap history-timeline overflow-x-auto overscroll-x-contain rounded-[var(--radius-card)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)] shadow-[var(--shadow-card)]"><table><caption className="sr-only">Scan history for this workspace</caption><thead className="sticky top-0 z-[1] bg-[var(--color-surface-muted)]"><tr><th scope="col">Date</th><th scope="col">Status</th><th scope="col">Findings</th><th scope="col">Duration</th><th scope="col"><span className="sr-only">Actions</span></th></tr></thead><tbody>{historyDateBands(visibleScans).map(({ band, scans: banded }) => <Fragment key={band}><tr className="history-band-row"><th className="history-band" colSpan={6} scope="colgroup" data-count={String(banded.length)} data-date={band}>{band}</th></tr>{banded.map((scan) => {
    const stateDisplay = scanStateDisplay(scan.state);
    const tone = scan.id === scans[0].id ? scan.state === 'failed' ? 'row-danger' : scan.state === 'completed_with_warnings' ? 'row-warning' : '' : '';
    const isOpen = expanded.has(scan.id);
    const runs = scan.analyzer_runs ?? [];
    return <Fragment key={scan.id}><tr className={[tone, isOpen ? 'is-expanded' : ''].filter(Boolean).join(' ') || undefined}><td title={date(scan.finished_at ?? scan.started_at)}><span className="history-date"><button type="button" className="history-disclose" aria-expanded={isOpen} aria-controls={`history-detail-${scan.id}`} aria-label={`${isOpen ? 'Hide' : 'Show'} details for the scan from ${relativeTime(scan.finished_at ?? scan.started_at)}`} onClick={() => toggleExpanded(scan.id)}><span className="disclose-arrow" aria-hidden="true">▸</span></button>{relativeTime(scan.finished_at ?? scan.started_at)}</span></td><td><div className="history-status"><Badge variant={stateDisplay.variant} className="whitespace-nowrap">{stateDisplay.label}</Badge>{scan.profile && <span className="badge profile-badge">{scan.profile}</span>}</div></td><td><FindingsCell scan={scan} /></td><td>{compactDuration(scan.duration_ms)}</td><td><div className="table-actions"><button type="button" className="text-button" onClick={() => go({ page: 'scan', id: scan.id })}>Open report</button>{hasMarkdownExport(scan) && <><a className="text-button" href={api.markdownUrl(scan.id)}>Export .md</a><a className="text-button" href={api.exportUrl(scan.id, 'sarif')}>SARIF</a><a className="text-button" href={api.exportUrl(scan.id, 'html')}>HTML</a><a className="text-button" href={api.exportUrl(scan.id, 'json')}>JSON</a></>}</div></td></tr>{isOpen && <tr className="history-detail-row"><td colSpan={6}><div className="history-detail" id={`history-detail-${scan.id}`}><dl className="history-detail-meta"><div><dt>Started</dt><dd>{date(scan.started_at)}</dd></div><div><dt>Finished</dt><dd>{date(scan.finished_at)}</dd></div>{scan.profile && <div><dt>Profile</dt><dd>{scan.profile}</dd></div>}</dl>{scan.snapshot && <p className="history-coverage">Selected {scan.snapshot.selected_file_count ?? 0} of {scan.snapshot.candidate_file_count ?? 0} candidate files{skipCoverageSummary(scan.snapshot.skip_counts) && <> — skipped {skipCoverageSummary(scan.snapshot.skip_counts)}</>}{(scan.snapshot.exclusions?.length ?? 0) > 0 && <> · {scan.snapshot.exclusions!.length} exclusion{scan.snapshot.exclusions!.length === 1 ? '' : 's'} in effect</>}</p>}{scan.error_summary && <div className="inline-warning">Warning: {scan.error_summary}</div>}{runs.length > 0 && <ul className="history-analyzers">{runs.map((run) => <li key={run.analyzer_id}><span>{analyzerName(run.analyzer_id)}</span><span className={`state ${run.status}`}>{run.status.replaceAll('_', ' ')}</span></li>)}</ul>}</div></td></tr>}</Fragment>;
  })}</Fragment>)}</tbody></table></div>{scrollCue && <div aria-hidden="true" style={{ position: 'absolute', top: 0, right: 0, bottom: 0, width: '2.25rem', pointerEvents: 'none', borderRadius: '0 var(--radius-card) var(--radius-card) 0', background: 'linear-gradient(to right, transparent, var(--color-surface))' }} />}</div><nav className="history-pagination" aria-label="Scan history pagination"><span className="history-pagination-count tabular-nums">Showing <strong>{shownFrom}–{shownTo}</strong> of <strong>{windowTotal}</strong> scans</span>{!filteredMode && <div><button type="button" className="button secondary" onClick={() => serverMode ? paging!.onPage(paging!.page - 1) : setClientPage(currentPage - 1)} disabled={serverMode ? paging!.page <= 1 : currentPage === 0}><svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" width="14" height="14"><path d="M15 18 9 12l6-6" strokeLinecap="round" strokeLinejoin="round"/></svg>Previous</button><output aria-live="polite" className="tabular-nums">Page {serverMode ? paging!.page : currentPage + 1} of {pageCount}</output><button type="button" className="button secondary" onClick={() => serverMode ? paging!.onPage(paging!.page + 1) : setClientPage(currentPage + 1)} disabled={serverMode ? !paging!.hasNext : currentPage >= pageCount - 1}>Next<svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" width="14" height="14"><path d="M9 18l6-6-6-6" strokeLinecap="round" strokeLinejoin="round"/></svg></button></div>}</nav></>;
}

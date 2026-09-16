import { Fragment, useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../api';
import '../css/history.css';
import type { Scan } from '../types';
import type { Route } from '../lib/router';
import { isTerminalScanState } from '../lib/scanEvents';
import { analyzerName, compactDuration, count, date, relativeTime, scanStateDisplay } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel } from '../components/ui';
import { ScanIcon } from '../components/icons';
import { SkeletonTable } from '../components/skeletons';
import { ComparePanel } from '../components/ComparePanel';
import { WorkspaceContextSidebar } from '../components/WorkspaceContext';
import { PageHeader } from '../components/PageHeader';
import { RowMenu, type RowMenuItem } from '../components/RowMenu';
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

/** Reads an in-progress or completed comparison from `?compare={a}&with={b}` so a deep link or reload restores it. Comparing a scan with itself is treated as a plain selection. */
function initialCompareFromUrl(): { a?: string; b?: string } {
  const params = new URLSearchParams(window.location.search);
  const a = params.get('compare') ?? undefined;
  const b = params.get('with') ?? undefined;
  if (!a) return {};
  return { a, b: b && b !== a ? b : undefined };
}

/** Mirrors the compare selection into `?compare=&with=` (keeping other params) without adding a history entry; a full cancel removes both. */
function syncCompareParams(a?: string, b?: string) {
  const params = new URLSearchParams(window.location.search);
  if (a) params.set('compare', a); else params.delete('compare');
  if (b) params.set('with', b); else params.delete('with');
  const query = params.toString();
  window.history.replaceState(null, '', `${window.location.pathname}${query ? `?${query}` : ''}`);
}

/** Scans that can take part in a comparison: finished cleanly or with warnings, plus cancelled scans — those are partial, so picking one asks for an explicit "compare anyway". */
export function isComparableScan(scan: Scan): boolean {
  return scan.state === 'completed' || scan.state === 'completed_with_warnings' || scan.state === 'cancelled';
}

interface AllScansPages { items: Scan[]; total: number; }

export function HistoryPage({ workspaceId, go }: { workspaceId: string; go: (r: Route) => void }) {
  const [page, setPage] = useState(initialPageFromUrl);
  const dateFilter = useDateFilter();
  // Two-phase compare selection: `a` is the scan the user started from, `b` the
  // "with this" pick. Both set → the panel above the table shows the diff. A
  // cancelled (partial) scan waits in `pendingPartial` for an explicit accept.
  const [pair, setPair] = useState(initialCompareFromUrl);
  const [pendingPartial, setPendingPartial] = useState<Scan>();
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
  // The toolbar count is the ONE place this screen reports "how many scans":
  // with a filter active it reports MATCHES across the whole history ("3 of 8
  // scans"); without one, the workspace total. No badge, no repeated chips.
  const fullTotal = state.data?.total ?? state.data?.items?.length ?? 0;
  const filteredTotal = filtering ? (allPages.data?.items ?? []).filter((s) => scanMatchesDateRange(s, dateFilter.from, dateFilter.to)).length : fullTotal;
  const countLine = !view.data ? '' : filtering && fullTotal !== filteredTotal ? `${count(filteredTotal)} of ${count(fullTotal)} scans` : `${count(filteredTotal)} ${filteredTotal === 1 ? 'scan' : 'scans'}`;
  const gotoPage = (next: number) => { setPage(next); syncPageParam(next); };
  useEffect(() => {
    if (state.data && state.data.items.length === 0 && state.data.total > 0 && page > 1) { setPage(1); syncPageParam(1); }
  }, [state.data, page]);

  // Compare selection flow. The diff itself is requested by ComparePanel as
  // (newer, older) — the page only tracks the picked ids in click order.
  const cancelCompare = useCallback(() => {
    setPair({});
    setPendingPartial(undefined);
    syncCompareParams();
  }, []);
  const startCompare = (scan: Scan) => {
    if (scan.state === 'cancelled') { setPendingPartial(scan); return; }
    setPendingPartial(undefined);
    setPair({ a: scan.id });
    syncCompareParams(scan.id);
  };
  const chooseSecond = (scan: Scan) => {
    if (!pair.a || scan.id === pair.a) return;
    if (scan.state === 'cancelled') { setPendingPartial(scan); return; }
    setPendingPartial(undefined);
    setPair({ a: pair.a, b: scan.id });
    syncCompareParams(pair.a, scan.id);
  };
  const acceptPartial = () => {
    const scan = pendingPartial;
    if (!scan) return;
    setPendingPartial(undefined);
    if (pair.a && scan.id !== pair.a) { setPair({ a: pair.a, b: scan.id }); syncCompareParams(pair.a, scan.id); }
    else { setPair({ a: scan.id }); syncCompareParams(scan.id); }
  };
  const onComparePick = (scan: Scan) => (pair.a ? chooseSecond(scan) : startCompare(scan));
  // Escape is the keyboard way out of any compare state — mid-selection or a shown panel.
  useEffect(() => {
    if (!pair.a && !pendingPartial) return;
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') cancelCompare(); };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [pair.a, pendingPartial, cancelCompare]);
  // Scans already in hand (either table source) feed the strip's copy; the strip
  // fetches on its own only when the base scan lives on another server page.
  const knownScans = new Map<string, Scan>();
  for (const scan of [...(state.data?.items ?? []), ...(allPages.data?.items ?? [])]) knownScans.set(scan.id, scan);
  const compareSelecting = !!pair.a && !pair.b;
  const stripBaseId = pendingPartial?.id ?? pair.a;
  const stripBaseScan = pendingPartial ?? (pair.a ? knownScans.get(pair.a) : undefined);
  // Rows always offer "Compare with…" in the row menu; once a base is picked
  // they swap to "Compare with this". With the pair complete the panel is up,
  // so the rows drop compare controls entirely.
  const tableCompareProps = pair.b ? {} : pair.a ? { compareBaseId: pair.a, onComparePick } : { onComparePick };
  return (
    <div className="page workspace-page">
      <WorkspaceContextSidebar id={workspaceId} current={{ page: 'history', id: workspaceId }} onNavigate={go} />
      <div className="workspace-page-body">
        <PageHeader title={workspace.data?.name ? `${workspace.data.name} — History` : 'Scan history'} />
        <div className="toolbar-row history-toolbar">
          <div className="toolbar-filters">
            <label className="history-filter-field"><span className="history-filter-label">From</span><span className="history-filter-input-wrap"><svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7"><rect x="3" y="4" width="18" height="17" rx="2"/><path d="M16 2v4M8 2v4M3 9h18"/></svg><input type="date" value={dateFilter.from} onChange={(e)=>{dateFilter.setFrom(e.target.value); setPage(1); syncPageParam(1);}} className="history-filter-input" aria-label="Filter from date" /></span></label>
            <label className="history-filter-field"><span className="history-filter-label">To</span><span className="history-filter-input-wrap"><svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7"><rect x="3" y="4" width="18" height="17" rx="2"/><path d="M16 2v4M8 2v4M3 9h18"/></svg><input type="date" value={dateFilter.to} onChange={(e)=>{dateFilter.setTo(e.target.value); setPage(1); syncPageParam(1);}} className="history-filter-input" aria-label="Filter to date" /></span></label>
            {dateFilter.hasFilter && <button type="button" className="history-filter-clear" onClick={()=>{dateFilter.setFrom(''); dateFilter.setTo(''); setPage(1); syncPageParam(1);}} aria-label="Clear date filters">✕ Clear</button>}
          </div>
          <span className="toolbar-meta history-count tabular-nums" aria-live="polite">{countLine}</span>
        </div>
        {(pendingPartial || compareSelecting) && stripBaseId && <CompareStrip baseId={stripBaseId} baseScan={stripBaseScan} pendingScan={pendingPartial} onCancel={cancelCompare} onAccept={acceptPartial} />}
        {pair.a && pair.b && <ComparePanel aId={pair.a} bId={pair.b} />}
        {view.loading ? <SkeletonTable rows={6} cols={5} /> : view.error ? <ErrorPanel error={view.error} retry={view.reload} /> : filtering
          ? <HistoryTable scans={allPages.data?.items ?? []} go={go} paging={{ page: 1, pageSize: historyPageSize, total: allPages.data?.total ?? 0, hasNext: false, onPage: () => {}, filtered: true }} dateFrom={dateFilter.from} dateTo={dateFilter.to} {...tableCompareProps} />
          : <HistoryTable scans={state.data?.items ?? []} go={go} paging={{ page, pageSize: historyPageSize, total: state.data?.total ?? 0, hasNext: state.data?.has_next ?? false, onPage: gotoPage }} dateFrom={dateFilter.from} dateTo={dateFilter.to} {...tableCompareProps} />}
      </div>
    </div>
  );
}

/** Sticky instruction strip for compare selection mode. Names the base scan and points at the row menus; when a cancelled (partial) scan was picked, it swaps to an explicit "compare anyway" acceptance. The base scan is usually already in the table rows; if the selection came from a deep link pointing at another server page, it is fetched here. */
function CompareStrip({ baseId, baseScan, pendingScan, onCancel, onAccept }: { baseId: string; baseScan?: Scan; pendingScan?: Scan; onCancel: () => void; onAccept: () => void }) {
  const loaded = useLoad(async (): Promise<Scan | undefined> => baseScan ?? ((await api.scan(baseId)) ?? undefined), [baseId, baseScan?.id]);
  const accepting = !!pendingScan;
  const subject = pendingScan ?? loaded.data;
  return (
    <div className={accepting ? 'compare-strip compare-strip--warning' : 'compare-strip'} role="region" aria-label="Scan comparison selection" aria-busy={!subject}>
      {accepting
        ? <p>The <strong>{date(pendingScan!.finished_at ?? pendingScan!.started_at)}</strong> scan is a partial scan (cancelled before every analyzer finished). Compare anyway?</p>
        : subject
          ? <p>Comparing the <strong>{date(subject.finished_at ?? subject.started_at)}</strong> scan (<strong className="tabular-nums">{subject.total_findings ?? 0}</strong> findings) with… — pick a second scan from a row's actions menu</p>
          : <p>Loading scan details…</p>}
      <div className="compare-strip-actions">
        {accepting && <button type="button" className="button secondary" onClick={onAccept}>Compare anyway</button>}
        <button type="button" className="text-button" onClick={onCancel}>{accepting ? 'Cancel' : 'Cancel compare'}</button>
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
const DAY_MS = 86_400_000;
export type HistoryBand = 'Today' | 'Yesterday' | 'This week' | 'Earlier';
const bandOrder: HistoryBand[] = ['Today', 'Yesterday', 'This week', 'Earlier'];

/** Short absolute stamp ("Sep 15, 14:05") shown beside a recent scan's relative time — chronology without stealing a second line. */
const shortStamp = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });

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

function hasMarkdownExport(scan: Scan) {
  return isTerminalScanState(scan.state) && (scan.total_findings ?? 0) > 0;
}

/** Downloads (or opens) an export by synthesizing the same anchor click the old inline links performed, so Content-Disposition handling stays identical. */
function triggerDownload(href: string) {
  const link = document.createElement('a');
  link.href = href;
  link.rel = 'noopener';
  document.body.append(link);
  link.click();
  link.remove();
}

const severityOrder = ['critical', 'high', 'medium', 'low'] as const;

/** The findings cell reads as severity-composed counts — colored numbers via the
 *  severity tokens ("2 critical · 14 high"), zero buckets omitted — replacing the
 *  old misaligned pill bar. When the API total exceeds the four categorized
 *  buckets, the full total leads ("500 findings · 2 critical · 480 low"). */
function FindingsCell({ scan }: { scan: Scan }) {
  const counts = findingsCounts(scan);
  const categorized = counts.critical + counts.high + counts.medium + counts.low;
  const total = scan.total_findings ?? categorized;
  // A scan that never finished has no meaningful zero — "0 findings" would read
  // as scanned-and-clean; only finished scans may claim a clean sheet.
  if (!total) return <span className="findings-zero tabular-nums">{scan.state === 'completed' || scan.state === 'completed_with_warnings' ? '0' : '—'}</span>;
  return (
    <span className="findings-inline tabular-nums" title={categorized > 0 ? `${counts.critical} critical · ${counts.high} high · ${counts.medium} medium · ${counts.low} low` : undefined}>
      {total > categorized && <span className="findings-total">{count(total)}</span>}
      {severityOrder.filter((severity) => counts[severity] > 0).map((severity) => (
        <span key={severity} className={`findings-sev sev-${severity}`}>{count(counts[severity])} <em>{severity}</em></span>
      ))}
    </span>
  );
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

/** One visible control per row ("Open report", the forward action); everything
 *  else — compare and the exports — lives in this overflow menu. */
function scanRowMenuItems(scan: Scan, compareBaseId?: string, onComparePick?: (scan: Scan) => void): RowMenuItem[] {
  const items: RowMenuItem[] = [];
  if (onComparePick) {
    if (!compareBaseId) {
      if (isComparableScan(scan)) items.push({ label: 'Compare with…', onSelect: () => onComparePick(scan) });
    } else if (scan.id !== compareBaseId && isComparableScan(scan)) {
      items.push({ label: 'Compare with this', onSelect: () => onComparePick(scan) });
    }
  }
  if (hasMarkdownExport(scan)) {
    items.push(
      { label: 'Export Markdown', onSelect: () => triggerDownload(api.markdownUrl(scan.id)) },
      { label: 'Export JSON', onSelect: () => triggerDownload(api.exportUrl(scan.id, 'json')) },
      { label: 'Export SARIF', onSelect: () => triggerDownload(api.exportUrl(scan.id, 'sarif')) },
      { label: 'Export HTML', onSelect: () => triggerDownload(api.exportUrl(scan.id, 'html')) },
    );
  }
  return items;
}

export function HistoryTable({ scans, go, paging, dateFrom, dateTo, compareBaseId, onComparePick }: { scans: Scan[]; go: (r: Route) => void; paging?: HistoryPaging; dateFrom?: string; dateTo?: string; compareBaseId?: string; onComparePick?: (scan: Scan) => void }) {
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
  // show every match with no page controls, and let the toolbar count report
  // the filtered range against the full workspace total.
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

  return <><div style={{ position: 'relative' }}><div ref={tableWrapRef} className="table-wrap history-table-wrap history-timeline overflow-x-auto overscroll-x-contain rounded-[var(--radius-card)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)] shadow-[var(--shadow-card)]"><table className="table-dense"><caption className="sr-only">Scan history for this workspace</caption><thead className="sticky top-0 z-[1] bg-[var(--color-surface-muted)]"><tr><th scope="col">Date</th><th scope="col">Status</th><th scope="col">Findings</th><th scope="col">Duration</th><th scope="col"><span className="sr-only">Actions</span></th></tr></thead><tbody>{historyDateBands(visibleScans).map(({ band, scans: banded }) => <Fragment key={band}><tr className="history-band-row"><th className="history-band" colSpan={5} scope="colgroup" data-count={String(banded.length)} data-date={band}>{band}</th></tr>{banded.map((scan) => {
    const stateDisplay = scanStateDisplay(scan.state);
    const tone = scan.id === scans[0].id ? scan.state === 'failed' ? 'row-danger' : scan.state === 'completed_with_warnings' ? 'row-warning' : '' : '';
    const isOpen = expanded.has(scan.id);
    const runs = scan.analyzer_runs ?? [];
    const stampValue = scan.finished_at ?? scan.started_at ?? '';
    const stampTime = new Date(stampValue).getTime();
    // Recent scans read "2 hours ago · Sep 15, 14:05"; older ones fall back to
    // the absolute short date that relativeTime already produces by itself.
    const showAbsDate = !Number.isNaN(stampTime) && Date.now() - stampTime >= 0 && Date.now() - stampTime < 7 * DAY_MS;
    const menuItems = scanRowMenuItems(scan, compareBaseId, onComparePick);
    return <Fragment key={scan.id}><tr className={[tone, isOpen ? 'is-expanded' : ''].filter(Boolean).join(' ') || undefined}><td title={date(stampValue)}><span className="history-date"><button type="button" className="history-disclose" aria-expanded={isOpen} aria-controls={`history-detail-${scan.id}`} aria-label={`${isOpen ? 'Hide' : 'Show'} details for the scan from ${relativeTime(stampValue)}`} onClick={() => toggleExpanded(scan.id)}><span className="disclose-arrow" aria-hidden="true">▸</span></button><span className="history-when"><span className="history-relative">{relativeTime(stampValue)}</span>{showAbsDate && <span className="history-abs">{shortStamp.format(stampTime)}</span>}</span></span></td><td><div className="history-status"><Badge variant={stateDisplay.variant} className="whitespace-nowrap">{stateDisplay.label}</Badge>{scan.profile && <span className="badge profile-badge">{scan.profile}</span>}</div></td><td><FindingsCell scan={scan} /></td><td>{compactDuration(scan.duration_ms)}</td><td><div className="table-actions history-actions"><button type="button" className="text-button" onClick={() => go({ page: 'scan', id: scan.id })}>Open report</button>{compareBaseId === scan.id && <span className="compare-base-tag">Base scan</span>}<RowMenu label={`Actions for scan ${scan.id}`} items={menuItems} /></div></td></tr>{isOpen && <tr className="history-detail-row"><td colSpan={5}><div className="history-detail" id={`history-detail-${scan.id}`}><dl className="history-detail-meta"><div><dt>Started</dt><dd>{date(scan.started_at)}</dd></div><div><dt>Finished</dt><dd>{date(scan.finished_at)}</dd></div>{scan.profile && <div><dt>Profile</dt><dd>{scan.profile}</dd></div>}</dl>{scan.snapshot && <p className="history-coverage">Selected {scan.snapshot.selected_file_count ?? 0} of {scan.snapshot.candidate_file_count ?? 0} candidate files{skipCoverageSummary(scan.snapshot.skip_counts) && <> — skipped {skipCoverageSummary(scan.snapshot.skip_counts)}</>}{(scan.snapshot.exclusions?.length ?? 0) > 0 && <> · {scan.snapshot.exclusions!.length} exclusion{scan.snapshot.exclusions!.length === 1 ? '' : 's'} in effect</>}</p>}{scan.error_summary && <div className="inline-warning">Warning: {scan.error_summary}</div>}{runs.length > 0 && <ul className="history-analyzers">{runs.map((run) => <li key={run.analyzer_id}><span>{analyzerName(run.analyzer_id)}</span><span className={`state ${run.status}`}>{run.status.replaceAll('_', ' ')}</span></li>)}</ul>}</div></td></tr>}</Fragment>;
  })}</Fragment>)}</tbody></table></div>{scrollCue && <div aria-hidden="true" style={{ position: 'absolute', top: 0, right: 0, bottom: 0, width: '2.25rem', pointerEvents: 'none', borderRadius: '0 var(--radius-card) var(--radius-card) 0', background: 'linear-gradient(to right, transparent, var(--color-surface))' }} />}</div>{!filteredMode && <nav className="history-pagination" aria-label="Scan history pagination">{!paging && <span className="history-pagination-count tabular-nums">Showing <strong>{shownFrom}–{shownTo}</strong> of <strong>{windowTotal}</strong> scans</span>}<div><button type="button" className="button secondary" onClick={() => serverMode ? paging!.onPage(paging!.page - 1) : setClientPage(currentPage - 1)} disabled={serverMode ? paging!.page <= 1 : currentPage === 0}><svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" width="14" height="14"><path d="M15 18 9 12l6-6" strokeLinecap="round" strokeLinejoin="round"/></svg>Previous</button><output aria-live="polite" className="tabular-nums">Page {serverMode ? paging!.page : currentPage + 1} of {pageCount}</output><button type="button" className="button secondary" onClick={() => serverMode ? paging!.onPage(paging!.page + 1) : setClientPage(currentPage + 1)} disabled={serverMode ? !paging!.hasNext : currentPage >= pageCount - 1}>Next<svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" width="14" height="14"><path d="M9 18l6-6-6-6" strokeLinecap="round" strokeLinejoin="round"/></svg></button></div></nav>}</>;
}

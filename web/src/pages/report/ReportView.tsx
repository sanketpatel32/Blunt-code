import * as React from 'react';
import { useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from 'react';
import { api } from '../../api';
import type { AnalyzerRun, Finding, FindingPage, Report, Scan, Severity } from '../../types';
import type { Notice } from '../../lib/notice';
import { message } from '../../lib/notice';
import { analyzerName, compactDuration, findingLocation } from '../../lib/format';
import { copyToClipboard } from '../../lib/clipboard';
import { useLoad } from '../../hooks/useLoad';
import { CATEGORY_LABELS } from '../../lib/analyzerCatalog';
import { useDebouncedValue } from '../../hooks/useDebouncedValue';
import { Empty, ErrorPanel } from '../../components/ui';
import { CheckShieldIcon, MagnifierIcon, SparkleIcon } from '../../components/icons';
import { SkeletonTable } from '../../components/skeletons';
import { SuppressFindingDialog } from '../../components/dialogs';
import { downloadJiraCsv } from '../../components/JiraExport';
import { SourcePane } from './SourcePane';

/**
 * One flat filter record for the findings list. `severity`/`status` are comma lists
 * (multi-select chips); `path`/`rule` survive in the type only so old shared URLs
 * keep rendering their chips — the toolbar deliberately has no inputs for them
 * (the search box already covers message, rule, and path server-side).
 */
export type FindingFilter = { severity: string; category: string; analyzer: string; rule: string; path: string; status: string; q: string };

const SEVERITY_ORDER: Severity[] = ['critical', 'high', 'medium', 'low', 'info'];
const STATUS_OPTIONS = ['new', 'persistent', 'suppressed'] as const;

/** Text filters wait for typing to pause before hitting the API; empty values flush instantly. */
const TEXT_FILTER_DELAY_MS = 300;

/** Server-side fetch window for load-more: each "Show more" pulls one window of this size (API cap 200). */
const PAGE_SIZE_OPTIONS = [25, 50, 100, 200] as const;
const DEFAULT_PAGE_SIZE = 100;

/** Sortable findings columns; keys match the backend sort param (severity|path|analyzer|status). */
export type SortKey = 'severity' | 'path' | 'analyzer' | 'status';
type SortDir = 'asc' | 'desc';
export type SortState = { key: SortKey; dir: SortDir };
/**
 * Direction applied when a column becomes the active sort. The backend ranks severity
 * critical=5 … info=1, so DESC is the reading default — critical findings load on top.
 */
const SORT_DEFAULT_DIR: Record<SortKey, SortDir> = { severity: 'desc', path: 'asc', analyzer: 'asc', status: 'asc' };
const DEFAULT_SORT: SortState = { key: 'severity', dir: 'desc' };

const VALID_SORT_KEYS: SortKey[] = ['severity', 'path', 'analyzer', 'status'];

/** Scan-level severity counts pulled from the scan record. */
function severityCounts(scan: Scan): Record<Severity, number> {
  return { critical: scan.critical_count ?? 0, high: scan.high_count ?? 0, medium: scan.medium_count ?? 0, low: scan.low_count ?? 0, info: scan.info_count ?? 0 };
}

/** Read the shareable report state (filters/sort/pages/page size) from the current URL. Unknown values fall back to defaults. */
function readUrlState(): { filters: FindingFilter; sort: SortState; page: number; pageSize: number } {
  if (typeof window === 'undefined') return { filters: { severity: '', category: '', analyzer: '', rule: '', path: '', status: '', q: '' }, sort: DEFAULT_SORT, page: 1, pageSize: DEFAULT_PAGE_SIZE };
  const sp = new URLSearchParams(window.location.search);
  const filters: FindingFilter = {
    severity: sp.get('severity') ?? '',
    category: sp.get('category') ?? '',
    analyzer: sp.get('tool') ?? sp.get('analyzer') ?? '',
    rule: sp.get('rule') ?? '',
    path: sp.get('path') ?? '',
    status: sp.get('status') ?? '',
    q: sp.get('q') ?? '',
  };
  const rawSort = sp.get('sort');
  const rawOrder = sp.get('order');
  let sort = DEFAULT_SORT;
  if (rawSort && (VALID_SORT_KEYS as string[]).includes(rawSort)) {
    const key = rawSort as SortKey;
    sort = { key, dir: rawOrder === 'asc' || rawOrder === 'desc' ? rawOrder : SORT_DEFAULT_DIR[key] };
  }
  let page = 1;
  const rawPage = Number(sp.get('page'));
  if (Number.isFinite(rawPage) && rawPage >= 1) page = Math.floor(rawPage);
  let pageSize = DEFAULT_PAGE_SIZE;
  const rawPageSize = Number(sp.get('page_size'));
  if ((PAGE_SIZE_OPTIONS as readonly number[]).includes(rawPageSize)) pageSize = rawPageSize;
  return { filters, sort, page, pageSize };
}

/** Serialize the shareable state to a query string; default values are omitted so clean URLs stay clean. */
function buildUrlSearch(filters: FindingFilter, sort: SortState, page: number, pageSize: number): string {
  const sp = new URLSearchParams();
  for (const key of ['severity', 'category', 'rule', 'status'] as const) {
    if (filters[key]) sp.set(key, filters[key]);
  }
  if (filters.analyzer) sp.set('analyzer', filters.analyzer);
  if (filters.path) sp.set('path', filters.path);
  if (filters.q) sp.set('q', filters.q);
  if (sort.key !== DEFAULT_SORT.key || sort.dir !== DEFAULT_SORT.dir) {
    sp.set('sort', sort.key);
    sp.set('order', sort.dir);
  }
  if (page !== 1) sp.set('page', String(page));
  if (pageSize !== DEFAULT_PAGE_SIZE) sp.set('page_size', String(pageSize));
  return sp.toString();
}

/** How long a row's copy button shows its check-mark confirm before reverting; short by design so long lists cannot toast-spam. */
const COPY_CONFIRM_MS = 800;

/** Stand-in notify for report views mounted without a toast channel (tests embed ReportView directly). */
const noopNotify = (_notice: Notice) => {};

/** Multi-line plain-text summary for the clipboard; missing fields are skipped rather than emitting empty lines. */
function findingSummaryText(finding: Finding) {
  const lines = [`[${finding.severity}]${finding.rule_id ? ` ${finding.rule_id}` : ''} — ${finding.message}`];
  if (finding.relative_path) lines.push(findingLocation(finding));
  lines.push(`analyzer: ${finding.analyzer_id}`);
  if (finding.remediation) lines.push(`remediation: ${finding.remediation}`);
  return lines.join('\n');
}

/** Row classes that tint a finding row's leading edge for the severities worth
 *  scanning for at a glance; low/info rows stay clean to avoid rainbow tables. */
function severityEdgeClass(severity: string) {
  return severity === 'critical' || severity === 'high' || severity === 'medium' ? `row-${severity}` : '';
}

/** Stable per-finding key for selection, dedupe, and pane lookup. */
function findingKey(finding: Finding, index = 0) {
  return finding.id || finding.fingerprint || `idx-${index}`;
}

/** Small decorative row-action icons; they inherit the button color and carry no semantics. */
function ClipboardIcon() {  return <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false"><rect x="9" y="9" width="11" height="11" rx="2" /><path d="M5 15h-.25A1.75 1.75 0 0 1 3 13.25V4.75C3 3.78 3.78 3 4.75 3h8.5c.97 0 1.75.78 1.75 1.75V5" /></svg>;
}

function CheckIcon() {
  return <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false"><path d="m5 13 4.2 4.2L19 6.5" /></svg>;
}

export function ReportView({ scanId, notify, runs: runsProp }: { scanId: string; notify?: (n: Notice) => void; runs?: AnalyzerRun[] }) {
  const report = useLoad(() => api.report(scanId), [scanId]);
  // Read the shareable URL state exactly once per mount so all three slices
  // stay mutually consistent even if the address changes mid-render.
  const [initialUrlState] = useState(readUrlState);
  // Findings row density: comfortable (default) vs compact, remembered per browser.
  const [dense, setDense] = useState(() => {
    try { return window.localStorage.getItem('bluntcode.findingsDensity') === 'compact'; } catch { return false; }
  });
  const toggleDensity = () => setDense((current) => {
    const next = !current;
    try { window.localStorage.setItem('bluntcode.findingsDensity', next ? 'compact' : 'comfortable'); } catch { /* private mode: session-only */ }
    return next;
  });
  const [filters, setFilters] = useState<FindingFilter>(initialUrlState.filters);
  const [pages, setPages] = useState(initialUrlState.page);
  const [pageSize, setPageSize] = useState(initialUrlState.pageSize);
  const [sort, setSort] = useState<SortState>(initialUrlState.sort);
  const [selectedKey, setSelectedKey] = useState<string | undefined>();
  const [suppressing, setSuppressing] = useState<Finding>();
  /** Keeps the pane-following row in view when walking findings with the keyboard. */
  useEffect(() => {
    if (!selectedKey) return;
    const row = document.querySelector('tr.active');
    if (row && typeof row.scrollIntoView === 'function') row.scrollIntoView({ block: 'nearest' });
  }, [selectedKey]);
  const debouncedQ = useDebouncedValue(filters.q, filters.q ? TEXT_FILTER_DELAY_MS : 0);
  // Keep the URL in sync for shareable report links (filters/sort/pages/page size).
  // The text filter uses its debounced value so typing does not spam history;
  // replaceState keeps back/forward linear. A no-op when nothing changed.
  useEffect(() => {
    const urlFilters: FindingFilter = { ...filters, q: debouncedQ };
    const qs = buildUrlSearch(urlFilters, sort, pages, pageSize);
    const current = window.location.search.replace(/^\?/, '');
    if (qs === current) return;
    const nextUrl = qs ? `${window.location.pathname}?${qs}${window.location.hash}` : `${window.location.pathname}${window.location.hash}`;
    window.history.replaceState(null, '', nextUrl);
  }, [filters, debouncedQ, sort, pages, pageSize]);
  // Restore filters/sort/pages when the user navigates back/forward.
  useEffect(() => {
    const onPopState = () => {
      const next = readUrlState();
      setFilters(next.filters);
      setSort(next.sort);
      setPages(next.page);
      setPageSize(next.pageSize);
    };
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);
  const params = useMemo(() => ({ ...filters, q: debouncedQ, page: String(pages), page_size: String(pageSize), sort: sort.key, order: sort.dir }), [filters, debouncedQ, pages, pageSize, sort]);
  /** CSV export query: the on-screen filters + sort minus paging, built exactly like the findings request so the file mirrors the list. */
  const csvParams = useMemo(() => Object.fromEntries(Object.entries(params).filter(([key]) => key !== 'page' && key !== 'page_size')), [params]);
  const findings = useLoad(() => api.findings(scanId, params), [scanId, ...Object.values(params)]);
  /** Rows accumulate across "Show more" windows instead of paginating away. Page 1 always
   *  replaces (every filter/sort change resets there); later windows merge by key so a
   *  refresh never duplicates a row and status flips land where the user can see them. */
  const [items, setItems] = useState<Finding[]>([]);
  const [envelope, setEnvelope] = useState<FindingPage>();
  useEffect(() => {
    const data = findings.data;
    if (!data) return;
    setEnvelope(data);
    setItems((prev) => {
      if (pages === 1) return data.items ?? [];
      const merged = new Map(prev.map((finding, index) => [findingKey(finding, index), finding]));
      for (const finding of data.items ?? []) merged.set(findingKey(finding), finding);
      return [...merged.values()];
    });
    // The scroll anchor should follow the pane selection, not the merge.
  }, [findings.data, pages]);
  /** Refetch every loaded window (used after suppress/restore so statuses flip in place, not just on the last page). */
  const reloadAllPages = async () => {
    const collected: Finding[] = [];
    let last: FindingPage | undefined;
    for (let page = 1; page <= pages; page += 1) {
      try {
        last = await api.findings(scanId, { ...params, page: String(page) });
        collected.push(...(last.items ?? []));
      } catch { /* a failed refresh keeps the rows already on screen */ }
    }
    if (last) setEnvelope(last);
    if (collected.length || pages === 1) setItems(collected);
  };
  const updateFilters = (next: FindingFilter) => { setFilters(next); setPages(1); };
  /** Toggles the active column's direction; switching columns applies that column's default direction. Either way the list restarts from the first window. */
  const changeSort = (key: SortKey) => { setSort((current) => (current.key === key ? { key, dir: current.dir === 'asc' ? 'desc' : 'asc' } : { key, dir: SORT_DEFAULT_DIR[key] })); setPages(1); };
  /** Switching the window size restarts on page one; the new window rarely contains the old rows. */
  const changePageSize = (next: number) => { setPageSize(next); setPages(1); };
  const removeFilter = (key: keyof FindingFilter) => updateFilters({ ...filters, [key]: '' });
  /** Severity chips are multi-select: the filter value is the comma list the server accepts. */
  const toggleSeverity = (severity: string) => {
    const active = filters.severity ? filters.severity.split(',').filter(Boolean) : [];
    const next = active.includes(severity) ? active.filter((sev) => sev !== severity) : [...active, severity];
    updateFilters({ ...filters, severity: next.join(',') });
  };
  const toggleAnalyzer = (analyzer: string) => updateFilters({ ...filters, analyzer: filters.analyzer === analyzer ? '' : analyzer });
  const toggleCategory = (category: string) => updateFilters({ ...filters, category: filters.category === category ? '' : category });
  const toggleStatus = (status: string) => updateFilters({ ...filters, status: filters.status === status ? '' : status });
  if (report.loading) return <section className="report"><SkeletonTable rows={6} cols={5} className="findings-table" /></section>;
  if (report.error) return <ErrorPanel error={report.error} retry={report.reload} />;
  const data = report.data ?? ({} as Report);
  /** A report payload without its scan still renders: zeroed controls beat a crashed report view. */
  const dataScan = data.scan ?? ({ id: scanId, workspace_id: '', state: '' } as Scan);
  const workspaceId = dataScan.workspace_id || '';
  /** Suppression is workspace-scoped, so row actions only appear when the scan names its workspace and the finding carries a fingerprint. */
  const canManageSuppressions = Boolean(workspaceId);
  /** Deleting a suppression restores the finding: the toast confirms what happened before the refresh. */
  async function restoreFinding(finding: Finding) {
    if (!finding.fingerprint || !workspaceId) return;
    try {
      await api.removeSuppression(workspaceId, finding.fingerprint);
      notify?.({ kind: 'success', text: 'Finding restored. It will be counted again on the next scan.' });
      await reloadAllPages();
    } catch (e) {
      notify?.({ kind: 'error', text: message(e) });
    }
  }
  const activeFilterCount = Object.values(filters).filter(Boolean).length;
  const clearFilters = () => updateFilters({ severity: '', category: '', analyzer: '', rule: '', path: '', status: '', q: '' });
  const counts = severityCounts(dataScan);
  const selectedSeverities = filters.severity ? filters.severity.split(',').filter(Boolean) : [];
  /** Tool chips carry each engine's finding count (from the report payload, which holds every finding). The report's scan object omits analyzer_runs, so the hosting ScanPage passes its /scans/{id} runs down; zero-finding engines stay visible with a 0 count. */
  const runs = runsProp ?? dataScan.analyzer_runs ?? [];
  const runById = new Map<string, AnalyzerRun | undefined>(runs.map((run) => [run.analyzer_id, run] as const));
  const countByAnalyzer = new Map<string, number>();
  for (const finding of data.findings ?? []) {
    countByAnalyzer.set(finding.analyzer_id, (countByAnalyzer.get(finding.analyzer_id) ?? 0) + 1);
    if (!runById.has(finding.analyzer_id)) runById.set(finding.analyzer_id, undefined);
  }
  const tools = [...runById.keys()];
  /** Type rail: the categories actually present in this scan, most findings first. */
  const categories = [...(data.findings ?? []).reduce((map, finding) => { if (finding.category) map.set(finding.category, (map.get(finding.category) ?? 0) + 1); return map; }, new Map<string, number>())].sort((a, b) => b[1] - a[1]);
  const noFindingsTitle = filters.analyzer ? `${analyzerName(filters.analyzer)} reported no findings` : 'No findings match these filters';
  const noFindingsCopy = filters.analyzer ? 'This analyzer completed without reportable issues for the selected files.' : 'Try clearing one or more filters.';
  /** A completed scan with zero findings and no active filters earns the celebratory panel; anything else keeps the filter-empty messaging. */
  const allClear = !activeFilterCount && dataScan.state === 'completed' && (dataScan.total_findings ?? 0) === 0;
  const total = envelope?.total ?? items.length;
  const hasNext = envelope?.has_next ?? envelope?.has_more ?? false;
  const loadingMore = findings.loading && items.length > 0;
  const selectedIndex = selectedKey ? items.findIndex((finding, index) => findingKey(finding, index) === selectedKey) : -1;
  const selected = selectedIndex >= 0 ? items[selectedIndex] : undefined;
  return <section className="report">
    {data.warnings?.length ? <div className="inline-warning"><strong>Incomplete analysis</strong>{data.warnings.map((warning) => <span key={warning}>{warning}</span>)}</div> : null}
    <div className="analysis-toolbar" role="search">
      <div className="toolbar-top">
        <label className="analysis-search"><MagnifierIcon aria-hidden="true" /><span className="sr-only">Search findings</span><input value={filters.q} onChange={(event) => setFilters((current) => ({ ...current, q: event.target.value }))} placeholder="Search message, rule, or file" /></label>
        <span className="toolbar-result-count" aria-live="polite"><strong>{total}</strong> {total === 1 ? 'finding' : 'findings'}</span>
        <button type="button" className="button secondary density-toggle" aria-pressed={dense} title={dense ? 'Switch to comfortable row spacing' : 'Switch to compact row spacing'} onClick={toggleDensity}>{dense ? 'Comfortable' : 'Compact'}</button>
        {activeFilterCount ? <button type="button" className="text-button toolbar-clear" onClick={clearFilters}>Clear</button> : null}
      </div>
      <div className="toolbar-groups">
        <div className="filter-group">
          <span className="filter-group-label" aria-hidden="true">Severity</span>
          <fieldset className="chip-group" aria-label="Severity">{SEVERITY_ORDER.map((severity) => { const count = counts[severity]; const active = selectedSeverities.includes(severity); return <button type="button" key={severity} className={`chip sev-chip ${severity}${active ? ' pressed' : ''}`} aria-pressed={active} disabled={count === 0 && !active} onClick={() => toggleSeverity(severity)}><i className={`sev-dot sev-${severity}`} aria-hidden="true" />{severity}<span className="count">{count}</span></button>; })}</fieldset>
        </div>
        <div className="filter-group">
          <span className="filter-group-label" aria-hidden="true">Status</span>
          <fieldset className="chip-group" aria-label="Status"><button type="button" className="chip" aria-pressed={!filters.status} onClick={() => updateFilters({ ...filters, status: '' })}>Any status</button>{STATUS_OPTIONS.map((status) => <button type="button" key={status} className={`chip${filters.status === status ? ' pressed' : ''}`} aria-pressed={filters.status === status} onClick={() => toggleStatus(status)}>{status}</button>)}</fieldset>
        </div>
        {tools.length > 0 && <div className="filter-group">
          <span className="filter-group-label" aria-hidden="true">Tool</span>
          <fieldset className="chip-group" aria-label="Tool">{tools.map((tool) => { const run = runById.get(tool); return <button type="button" key={tool} className={`chip${filters.analyzer === tool ? ' pressed' : ''}`} aria-pressed={filters.analyzer === tool} title={run ? `${run.status ?? 'unknown'}${run.duration_ms !== undefined ? ` · ${compactDuration(run.duration_ms)}` : ''}` : 'Reported findings; no run recorded on this scan'} onClick={() => toggleAnalyzer(tool)}>{analyzerName(tool)}<span className="count">{countByAnalyzer.get(tool) ?? 0}</span></button>; })}</fieldset>
        </div>}
        {categories.length > 0 && <div className="filter-group filter-group-wide">
          <span className="filter-group-label" aria-hidden="true">Type</span>
          <fieldset className="chip-rail chip-group" aria-label="Type">{categories.map(([category, count]) => <button type="button" key={category} className={`chip${filters.category === category ? ' pressed' : ''}`} aria-pressed={filters.category === category} onClick={() => toggleCategory(category)}>{(CATEGORY_LABELS as Record<string, string>)[category] ?? category}<span className="count">{count}</span></button>)}</fieldset>
        </div>}
      </div>
    </div>
    <FilterChips filters={filters} onRemove={removeFilter} />
    <div className="analysis-split" data-pane={selected ? 'open' : 'closed'}>
      <div className="findings-col" data-scoped={selected ? 'true' : undefined}>
        <div className={`finding-list analysis-list${dense ? ' finding-dense' : ''}`}>
          {findings.loading && !items.length ? <SkeletonTable rows={5} cols={5} className="findings-table" /> : findings.error && !items.length ? <ErrorPanel error={findings.error} retry={findings.reload} /> : items.length ? <FindingsTable findings={items} sort={sort} onSort={changeSort} activeKey={selectedKey} onSelect={(finding, index) => setSelectedKey(findingKey(finding, index))} canSuppress={canManageSuppressions} onSuppress={setSuppressing} onRestore={restoreFinding} /> : allClear ? <Empty title="All clear — no findings" tone="positive" icon={<><CheckShieldIcon /><span className="empty-sparkle one"><SparkleIcon /></span><span className="empty-sparkle two"><SparkleIcon /></span></>}>Every analyzer finished and found nothing to flag. Nice work.</Empty> : <Empty title={noFindingsTitle} icon={<MagnifierIcon />}>{noFindingsCopy}</Empty>}
        </div>
        {(items.length > 0 || loadingMore) && <div className="load-more">
          <span className="load-more-status">Showing {items.length} of {total}</span>
          <fieldset className="segmented page-size" aria-label="Rows per fetch">{PAGE_SIZE_OPTIONS.map((size) => <button key={size} type="button" aria-pressed={pageSize === size} onClick={() => { if (size !== pageSize) changePageSize(size); }}>{size}</button>)}</fieldset>
          {loadingMore ? <span className="loading-more" role="status"><i className="spinner" aria-hidden="true" />Loading more…</span> : hasNext ? <button type="button" className="button secondary" onClick={() => setPages((current) => current + 1)}>Show more</button> : <span className="load-more-status">End of list</span>}
        </div>}
      </div>
      {selected && <SourcePane scanId={scanId} finding={selected} workspaceId={workspaceId} onClose={() => setSelectedKey(undefined)} onPrev={() => { if (selectedIndex > 0) setSelectedKey(findingKey(items[selectedIndex - 1]!, selectedIndex - 1)); }} onNext={() => { if (selectedIndex >= 0 && selectedIndex < items.length - 1) setSelectedKey(findingKey(items[selectedIndex + 1]!, selectedIndex + 1)); }} hasPrev={selectedIndex > 0} hasNext={selectedIndex >= 0 && selectedIndex < items.length - 1} onSuppress={setSuppressing} onRestore={restoreFinding} />}
    </div>
    <footer className="report-foot">
      <span className="report-foot-count">{total} {total === 1 ? 'finding' : 'findings'} · {tools.length || 0} {tools.length === 1 ? 'engine' : 'engines'} ran</span>
      <ExportMenu scanId={scanId} csvParams={csvParams} findings={items} />
    </footer>
    {suppressing && <SuppressFindingDialog workspaceId={workspaceId} finding={suppressing} onClose={() => setSuppressing(undefined)} onSuppressed={(reason) => { setSuppressing(undefined); notify?.({ kind: 'success', text: `Finding suppressed${reason ? ` (${reason})` : ''}. It will not appear in future scans, reports, or the CI gate.` }); void reloadAllPages(); }} notify={notify ?? noopNotify} />}
  </section>;
}

/** Flat export row — plain `<a download>` GET links for the Markdown/HTML/SARIF attachments plus a CSV of the currently filtered findings and a client-side Jira CSV. */
function ExportMenu({ scanId, csvParams, findings }: { scanId: string; csvParams: Record<string, string>; findings?: Finding[] }) {
  const items = [
    { label: 'Markdown', ext: '.md', href: api.markdownUrl(scanId) },
    { label: 'HTML', ext: '.html', href: api.exportUrl(scanId, 'html') },
    { label: 'SARIF', ext: '.sarif', href: api.exportUrl(scanId, 'sarif') },
    { label: 'CSV (current filters)', ext: '.csv', href: api.exportUrl(scanId, 'csv', csvParams) },
  ];
  const handleJiraCsv = () => {
    const data = findings ?? [];
    if (!data.length) return;
    downloadJiraCsv(data, `jira-${scanId}.csv`);
  };
  return <fieldset className="export-menu export-inline" aria-label="Export report">{items.map((item) => <a key={item.label} className="export-item" href={item.href} download>{item.label}<small>{item.ext}</small></a>)}<button type="button" className="export-item" onClick={handleJiraCsv}>Jira CSV<small>.csv</small></button></fieldset>;
}

function FilterChips({ filters, onRemove }: { filters: FindingFilter; onRemove: (key: keyof FindingFilter) => void }) {
  const chips: Array<{ key: keyof FindingFilter; text: string; removeLabel: string }> = [];
  if (filters.severity) chips.push({ key: 'severity', text: `severity: ${filters.severity.split(',').join(', ')}`, removeLabel: 'Remove severity filter' });
  if (filters.category) chips.push({ key: 'category', text: `type: ${(CATEGORY_LABELS as Record<string, string>)[filters.category] ?? filters.category}`, removeLabel: 'Remove type filter' });
  if (filters.analyzer) chips.push({ key: 'analyzer', text: `tool: ${analyzerName(filters.analyzer)}`, removeLabel: 'Remove tool filter' });
  if (filters.rule) chips.push({ key: 'rule', text: `rule: ${filters.rule}`, removeLabel: 'Remove rule filter' });
  if (filters.path) chips.push({ key: 'path', text: `file: ${filters.path}`, removeLabel: 'Remove file filter' });
  if (filters.status) chips.push({ key: 'status', text: `status: ${filters.status}`, removeLabel: 'Remove status filter' });
  if (filters.q) chips.push({ key: 'q', text: `search: "${filters.q}"`, removeLabel: 'Remove search filter' });
  if (!chips.length) return null;
  return <ul className="filter-chips" aria-label="Active filters">{chips.map((chip) => <li className="filter-chip" key={chip.key}>{chip.text}<button type="button" className="filter-chip-remove" aria-label={chip.removeLabel} title={chip.removeLabel} onClick={() => onRemove(chip.key)}>×</button></li>)}</ul>;
}

const COL_WIDTHS_KEY = 'bluntcode.colWidths';
function useColWidthsReport() {
  const [widths, setWidths] = useState<Record<string, number>>(() => {
    try { const raw = window.localStorage.getItem(COL_WIDTHS_KEY); return raw ? JSON.parse(raw) as Record<string, number> : {}; } catch { return {}; }
  });
  const setWidth = (id: string, w: number) => {
    setWidths((prev) => {
      const next = { ...prev, [id]: Math.max(90, Math.min(600, w)) };
      try { window.localStorage.setItem(COL_WIDTHS_KEY, JSON.stringify(next)); } catch {}
      return next;
    });
  };
  return { widths, setWidth };
}

/**
 * Findings table for the split layout: clicking a row selects it — the docked source
 * pane shows its code, remediation, and actions while this list keeps scrolling.
 * Rows are keyboard-walkable (ArrowUp/ArrowDown move focus, Enter/Space select).
 * Bulk selection, per-row copy, suppress/restore, and resizable columns are preserved.
 */
function FindingsTable({ findings, sort, onSort, activeKey, onSelect, canSuppress, onSuppress, onRestore }: { findings: Finding[]; sort: SortState; onSort: (key: SortKey) => void; activeKey?: string; onSelect: (finding: Finding, index: number) => void; canSuppress: boolean; onSuppress: (finding: Finding) => void; onRestore: (finding: Finding) => void }) {
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const copyTimer = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(copyTimer.current), []);
  const copyFinding = async (finding: Finding, key: string) => {
    if (!(await copyToClipboard(findingSummaryText(finding)))) return;
    setCopiedKey(key);
    window.clearTimeout(copyTimer.current);
    copyTimer.current = window.setTimeout(() => setCopiedKey(null), COPY_CONFIRM_MS);
  };
  // bulk selection (checkbox per row, additive header)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const allIds = findings.map((finding, index) => findingKey(finding, index));
  const allSelected = allIds.length > 0 && allIds.every((id) => selectedIds.has(id));
  const someSelected = allIds.some((id) => selectedIds.has(id)) && !allSelected;
  const selectedCount = selectedIds.size;
  const selectAllRef = useRef<HTMLInputElement>(null);
  useEffect(() => { if (selectAllRef.current) selectAllRef.current.indeterminate = !!someSelected; }, [someSelected]);
  const toggleSelectAll = (checked: boolean) => {
    if (checked) setSelectedIds(new Set(allIds)); else setSelectedIds(new Set());
  };
  const toggleRowSelect = (id: string, checked: boolean) => {
    const next = new Set(selectedIds);
    if (checked) next.add(id); else next.delete(id);
    setSelectedIds(next);
  };
  /** ArrowUp/ArrowDown move focus between rows so the pane walk works from the keyboard alone. */
  const onRowKeyDown = (event: ReactKeyboardEvent<HTMLTableRowElement>, index: number) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      onSelect(findings[index]!, index);
      return;
    }
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return;
    event.preventDefault();
    const next = event.key === 'ArrowDown' ? Math.min(index + 1, findings.length - 1) : Math.max(index - 1, 0);
    (event.currentTarget.parentElement?.children[next] as HTMLTableRowElement | undefined)?.focus?.();
  };
  const handleBulkCopy = async () => {
    const sel = findings.filter((finding, index) => selectedIds.has(findingKey(finding, index)));
    const text = sel.map(findingSummaryText).join('\n\n');
    if (text) await copyToClipboard(text);
  };
  const handleBulkExport = () => {
    const sel = findings.filter((finding, index) => selectedIds.has(findingKey(finding, index)));
    if (!sel.length) return;
    const header = ['severity','rule_id','message','path','analyzer_id'].join(',');
    const rows = sel.map((f) => [f.severity, f.rule_id ?? '', `"${(f.message ?? '').replace(/"/g, '""')}"`, f.relative_path ?? '', f.analyzer_id].join(',')).join('\n');
    const blob = new Blob([header + '\n' + rows], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a'); a.href = url; a.download = 'findings-selected.csv'; a.click(); URL.revokeObjectURL(url);
  };
  const handleBulkSuppress = () => {
    const sel = findings.filter((finding, index) => selectedIds.has(findingKey(finding, index)) && finding.fingerprint && canSuppress);
    sel.forEach((finding) => { onSuppress(finding); });
  };
  // column resizing
  const { widths, setWidth } = useColWidthsReport();
  const resizingRef = useRef<{ id: string; startX: number; startW: number } | null>(null);
  const onResizeStart = (e: React.PointerEvent, id: string, cur: number) => {
    (e.target as HTMLElement).setPointerCapture?.(e.pointerId);
    resizingRef.current = { id, startX: e.clientX, startW: cur };
    const onMove = (ev: PointerEvent) => { if (!resizingRef.current) return; const d = ev.clientX - resizingRef.current.startX; setWidth(resizingRef.current.id, resizingRef.current.startW + d); };
    const onUp = () => { resizingRef.current = null; window.removeEventListener('pointermove', onMove); window.removeEventListener('pointerup', onUp); };
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
  };
  const headerStyle = (id: string) => widths[id] ? { width: widths[id], minWidth: widths[id] } : undefined;
  const resizeHandle = (id: string) => (
    <span role="separator" aria-orientation="vertical" aria-label={'Resize '+id+' column'} onPointerDown={(e)=>onResizeStart(e,id,widths[id]??160)} className="absolute right-0 top-0 h-full w-2 cursor-col-resize touch-none select-none hover:bg-[var(--color-accent-ghost)]" tabIndex={0} onKeyDown={(e)=>{ if(e.key==='ArrowLeft') setWidth(id,(widths[id]??160)-16); if(e.key==='ArrowRight') setWidth(id,(widths[id]??160)+16); }} />
  );
  return <div className="space-y-2">
    {selectedCount>0 && <div role="toolbar" aria-label="Bulk actions" className="flex flex-wrap items-center gap-2 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface-muted)] px-3 py-2 shadow-[var(--shadow-card)]"><span className="inline-flex items-center rounded-[var(--radius-button)] bg-[var(--color-accent)] px-2 py-0.5 text-xs font-bold text-[var(--color-accent-ink)]" aria-live="polite">{selectedCount} selected</span><button type="button" onClick={handleBulkSuppress} className="inline-flex h-7 items-center gap-1.5 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold shadow-[var(--shadow-card)]">Suppress selected</button><button type="button" onClick={handleBulkExport} className="inline-flex h-7 items-center gap-1.5 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold shadow-[var(--shadow-card)]">Export selected CSV</button><button type="button" onClick={handleBulkCopy} className="inline-flex h-7 items-center gap-1.5 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold shadow-[var(--shadow-card)]">Copy</button></div>}
    <div className="findings-table table-wrap"><table><caption className="sr-only">Findings matching the current filters</caption><thead><tr>
      <th scope="col" className="relative sticky left-0 z-[2] bg-[var(--color-surface-muted)] shadow-[var(--shadow-card)]" style={headerStyle('severity')} aria-sort={sort.key==='severity'?(sort.dir==='asc'?'ascending':'descending'):'none'}><div className="flex items-center gap-2"><input ref={selectAllRef} type="checkbox" role="checkbox" aria-label="Select all" checked={!!allSelected} onChange={(e)=>toggleSelectAll(e.target.checked)} /><button type="button" className={'th-sort'+(sort.key==='severity'?' active':'')} onClick={()=>onSort('severity')}>Severity<span className="sort-arrow" aria-hidden="true">{sort.key==='severity'?(sort.dir==='asc'?'▲':'▼'):'↕'}</span>{sort.key==='severity' && <span className="sr-only"> (sorted {sort.dir==='asc'?'ascending' : 'descending'})</span>}</button></div>{resizeHandle('severity')}</th>
      <th scope="col" className="relative" style={headerStyle('finding')}>Finding{resizeHandle('finding')}</th>
      <th scope="col" className="relative" style={headerStyle('path')} aria-sort={sort.key==='path'?(sort.dir==='asc'?'ascending':'descending'):'none'}><button type="button" className={'th-sort'+(sort.key==='path'?' active':'')} onClick={()=>onSort('path')}>File<span className="sort-arrow" aria-hidden="true">{sort.key==='path'?(sort.dir==='asc'?'▲':'▼'):'↕'}</span>{sort.key==='path' && <span className="sr-only"> (sorted {sort.dir==='asc'?'ascending':'descending'})</span>}</button>{resizeHandle('path')}</th>
      <th scope="col" className="relative" style={headerStyle('analyzer')} aria-sort={sort.key==='analyzer'?(sort.dir==='asc'?'ascending':'descending'):'none'}><button type="button" className={'th-sort'+(sort.key==='analyzer'?' active':'')} onClick={()=>onSort('analyzer')}>Tool<span className="sort-arrow" aria-hidden="true">{sort.key==='analyzer'?(sort.dir==='asc'?'▲':'▼'):'↕'}</span>{sort.key==='analyzer' && <span className="sr-only"> (sorted {sort.dir==='asc'?'ascending':'descending'})</span>}</button>{resizeHandle('analyzer')}</th>
      <th scope="col" className="relative" style={headerStyle('status')} aria-sort={sort.key==='status'?(sort.dir==='asc'?'ascending':'descending'):'none'}><button type="button" className={'th-sort'+(sort.key==='status'?' active':'')} onClick={()=>onSort('status')}>Status<span className="sort-arrow" aria-hidden="true">{sort.key==='status'?(sort.dir==='asc'?'▲':'▼'):'↕'}</span>{sort.key==='status' && <span className="sr-only"> (sorted {sort.dir==='asc'?'ascending':'descending'})</span>}</button>{resizeHandle('status')}</th>
    </tr></thead><tbody>{findings.map((finding, index) => { const rowKey = findingKey(finding, index); const copied = copiedKey === rowKey; const rowName = finding.title ?? finding.rule_id ?? 'finding'; const suppressed = finding.status === 'suppressed'; const isSelected = selectedIds.has(rowKey); const isActive = activeKey === rowKey; const rowAction = canSuppress && finding.fingerprint ? suppressed ? <button type="button" className="text-button restore-finding" aria-label={'Restore '+rowName} title="Stop hiding this finding" onClick={(e)=>{ e.stopPropagation(); onRestore(finding);}}>Restore</button> : <button type="button" className="text-button suppress-finding" aria-label={'Suppress '+rowName} title="Hide this finding from future scans" onClick={(e)=>{ e.stopPropagation(); onSuppress(finding);}}>Suppress…</button> : null; return <tr key={rowKey} data-index={index} className={`${severityEdgeClass(finding.severity)}${isSelected ? ' selected' : ''}${isActive ? ' active' : ''}`} aria-label={`${finding.severity} ${rowName}${finding.relative_path ? ` in ${finding.relative_path}` : ''} — open in the source pane`} tabIndex={0} onClick={()=>onSelect(finding, index)} onKeyDown={(e)=>onRowKeyDown(e, index)}><td className="sticky left-0 z-[1] bg-[var(--color-surface)] shadow-[var(--shadow-card)]" style={headerStyle('severity')} onClick={(e)=>e.stopPropagation()}><span className="flex items-center gap-2"><input type="checkbox" role="checkbox" aria-label={'Select row '+rowKey} checked={isSelected} onChange={(e)=>toggleRowSelect(rowKey,e.target.checked)} onClick={(e)=>e.stopPropagation()} /><span className={'severity '+finding.severity}><i className="sev-dot" aria-hidden="true" />{finding.severity}</span></span></td><td className="finding-summary" style={headerStyle('finding')} title={finding.message ?? rowName}><strong>{finding.title ?? finding.rule_id ?? 'Finding'}</strong><span className="finding-message">{finding.message}</span>{finding.remediation && <span className="finding-remediation">Fix: {finding.remediation}</span>}</td><td style={headerStyle('path')} onClick={(e)=>e.stopPropagation()}><code>{findingLocation(finding)}</code></td><td style={headerStyle('analyzer')}><span className="badge">{finding.analyzer_id}</span>{finding.rule_id && <code>{finding.rule_id}</code>}</td><td style={headerStyle('status')} onClick={(e)=>e.stopPropagation()}><div className="finding-actions-cell">{finding.status ? <span className={'status-text'+(suppressed ? ' suppressed' : '')}>{finding.status}</span> : '—'}<span className="finding-row-actions">{rowAction}<button type="button" className={'icon-button copy-finding'+(copied ? ' copied' : '')} aria-label="Copy finding details" title="Copy finding details" onClick={()=> void copyFinding(finding, rowKey)}>{copied ? <CheckIcon /> : <ClipboardIcon />}</button></span></div></td></tr>; })}</tbody></table></div></div>;
}

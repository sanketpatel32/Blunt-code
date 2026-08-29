import * as React from 'react';
import { useEffect, useMemo, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from 'react';
import { api } from '../../api';
import type { AnalyzerRun, Finding, Report, Scan, Severity } from '../../types';
import type { Notice } from '../../lib/notice';
import { message } from '../../lib/notice';
import { analyzerName, compactDuration, findingLocation } from '../../lib/format';
import { copyToClipboard } from '../../lib/clipboard';
import { useLoad } from '../../hooks/useLoad';
import { analyzerMeta, categoryColor, CATEGORY_LABELS } from '../../lib/analyzerCatalog';
import { useDebouncedValue } from '../../hooks/useDebouncedValue';
import { Empty, ErrorPanel, SummaryCard } from '../../components/ui';
import { CheckShieldIcon, MagnifierIcon, SparkleIcon } from '../../components/icons';
import { SkeletonCards, SkeletonLines, SkeletonTable } from '../../components/skeletons';
import { FindingPreviewDialog, SuppressFindingDialog } from '../../components/dialogs';
import { downloadJiraCsv, buildJiraIssueUrl } from '../../components/JiraExport';
import { AutoFixPanel } from '../../components/AutoFixPanel';
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription } from '../../components/ui/sheet';
import { Sparkles, MessageCircle } from 'lucide-react';
import { CommentsPanel } from '../../components/CommentsPanel';

export type FindingFilter = { severity: string; category: string; analyzer: string; rule: string; path: string; status: string; q: string };

const EMPTY_FILTER: FindingFilter = { severity: '', category: '', analyzer: '', rule: '', path: '', status: '', q: '' };
const SEVERITY_ORDER: Severity[] = ['critical', 'high', 'medium', 'low', 'info'];
const ZERO_SEVERITY_COUNTS: Record<Severity, number> = { critical: 0, high: 0, medium: 0, low: 0, info: 0 };

/** Scan-level severity counts pulled from the scan record. */
function severityCounts(scan: Scan): Record<Severity, number> {
  return { critical: scan.critical_count ?? 0, high: scan.high_count ?? 0, medium: scan.medium_count ?? 0, low: scan.low_count ?? 0, info: scan.info_count ?? 0 };
}

/** Segment width as a percentage with one decimal, matching the HistoryPage bars so both charts round identically. */
function segmentWidth(count: number, total: number) {
  return `${Math.round((count * 1000) / total) / 10}%`;
}

/** Human breakdown of non-zero severity counts, e.g. "2 high, 1 medium". */
function severityBreakdownLabel(counts: Record<Severity, number>) {
  return SEVERITY_ORDER.filter((severity) => counts[severity] > 0).map((severity) => `${counts[severity]} ${severity}`).join(', ');
}

/** The stacked-bar segments for a set of counts; zero-count severities contribute no segment. */
function SeveritySegments({ counts, total }: { counts: Record<Severity, number>; total: number }) {
  return <>{SEVERITY_ORDER.filter((severity) => counts[severity] > 0).map((severity) => <i key={severity} className={`seg-${severity}`} style={{ width: segmentWidth(counts[severity], total) }} />)}</>;
}
/** Text filters wait for typing to pause before hitting the API; empty values flush instantly. */
const TEXT_FILTER_DELAY_MS = 300;

/** Server-side page window for the findings list; modest by design so first paint stays fast and the table stays scrollable. */
const PAGE_SIZE = 50;

/** Sortable findings columns; keys match the backend sort param (severity|path|analyzer|status). */
export type SortKey = 'severity' | 'path' | 'analyzer' | 'status';
type SortDir = 'asc' | 'desc';
export type SortState = { key: SortKey; dir: SortDir };
/**
 * Direction applied when a column becomes the active sort. The backend ranks severity
 * critical=5 … info=1 and orders by direction, so DESC puts critical findings on top;
 * the other columns read naturally alphabetically ascending.
 */
const SORT_DEFAULT_DIR: Record<SortKey, SortDir> = { severity: 'desc', path: 'asc', analyzer: 'asc', status: 'asc' };

const VALID_SORT_KEYS: SortKey[] = ['severity', 'path', 'analyzer', 'status'];

/** Read the shareable report state (filters/sort/page) from the current URL. Unknown values fall back to defaults. */
function readUrlState(): { filters: FindingFilter; sort: SortState; page: number } {
  if (typeof window === 'undefined') return { filters: { ...EMPTY_FILTER }, sort: { key: 'severity', dir: 'asc' }, page: 1 };
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
  let sort: SortState = { key: 'severity', dir: 'asc' };
  if (rawSort && (VALID_SORT_KEYS as string[]).includes(rawSort)) {
    const key = rawSort as SortKey;
    sort = { key, dir: rawOrder === 'asc' || rawOrder === 'desc' ? rawOrder : SORT_DEFAULT_DIR[key] };
  }
  let page = 1;
  const rawPage = Number(sp.get('page'));
  if (Number.isFinite(rawPage) && rawPage >= 1) page = Math.floor(rawPage);
  return { filters, sort, page };
}

/** Serialize the shareable state to a query string; default values are omitted so clean URLs stay clean. */
function buildUrlSearch(filters: FindingFilter, sort: SortState, page: number): string {
  const sp = new URLSearchParams();
  for (const key of ['severity', 'category', 'rule', 'status'] as const) {
    if (filters[key]) sp.set(key, filters[key]);
  }
  if (filters.analyzer) sp.set('analyzer', filters.analyzer);
  if (filters.path) sp.set('path', filters.path);
  if (filters.q) sp.set('q', filters.q);
  if (sort.key !== 'severity' || sort.dir !== 'asc') {
    sp.set('sort', sort.key);
    sp.set('order', sort.dir);
  }
  if (page !== 1) sp.set('page', String(page));
  return sp.toString();
}

/** How long a row's copy button shows its check-mark confirm before reverting; short by design so 25 rows cannot toast-spam. */
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

/** Small decorative row-action icons; they inherit the button color and carry no semantics. */
function ClipboardIcon() {  return <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false"><rect x="9" y="9" width="11" height="11" rx="2" /><path d="M5 15h-.25A1.75 1.75 0 0 1 3 13.25V4.75C3 3.78 3.78 3 4.75 3h8.5c.97 0 1.75.78 1.75 1.75V5" /></svg>;
}

function CheckIcon() {
  return <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false"><path d="m5 13 4.2 4.2L19 6.5" /></svg>;
}

export function ReportView({ scanId, notify }: { scanId: string; notify?: (n: Notice) => void }) {
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
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [page, setPage] = useState(initialUrlState.page);
  const [sort, setSort] = useState<SortState>(initialUrlState.sort);
  const [previewFinding, setPreviewFinding] = useState<Finding>();
  const [suppressing, setSuppressing] = useState<Finding>();
  const [fixFinding, setFixFinding] = useState<Finding | undefined>();
  const [commentFinding, setCommentFinding] = useState<Finding | undefined>();
  const debouncedCategory = useDebouncedValue(filters.category, filters.category ? TEXT_FILTER_DELAY_MS : 0);
  const debouncedPath = useDebouncedValue(filters.path, filters.path ? TEXT_FILTER_DELAY_MS : 0);
  const debouncedQ = useDebouncedValue(filters.q, filters.q ? TEXT_FILTER_DELAY_MS : 0);
  // Keep the URL in sync for shareable report links (filters/sort/page).
  // Text filters use their debounced values so typing does not spam history;
  // replaceState keeps back/forward linear. A no-op when nothing changed.
  useEffect(() => {
    const urlFilters: FindingFilter = { ...filters, category: debouncedCategory, path: debouncedPath, q: debouncedQ };
    const qs = buildUrlSearch(urlFilters, sort, page);
    const current = window.location.search.replace(/^\?/, '');
    if (qs === current) return;
    const nextUrl = qs ? `${window.location.pathname}?${qs}${window.location.hash}` : `${window.location.pathname}${window.location.hash}`;
    window.history.replaceState(null, '', nextUrl);
  }, [filters, debouncedCategory, debouncedPath, debouncedQ, sort, page]);
  // Restore filters/sort/page when the user navigates back/forward.
  useEffect(() => {
    const onPopState = () => {
      const next = readUrlState();
      setFilters(next.filters);
      setSort(next.sort);
      setPage(next.page);
    };
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);
  const params = useMemo(() => ({ ...filters, category: debouncedCategory, path: debouncedPath, q: debouncedQ, page: String(page), page_size: String(PAGE_SIZE), sort: sort.key, order: sort.dir }), [filters, debouncedCategory, debouncedPath, debouncedQ, page, sort]);
  /** CSV export query: the on-screen filters + sort minus paging, built exactly like the findings request so the file mirrors the list. */
  const csvParams = useMemo(() => Object.fromEntries(Object.entries(params).filter(([key]) => key !== 'page' && key !== 'page_size')), [params]);
  const findings = useLoad(() => api.findings(scanId, params), [scanId, ...Object.values(params)]);
  const list = findings.data?.items ?? [];
  const updateFilters = (next: FindingFilter) => { setFilters(next); setPage(1); };
  /** Toggles the active column's direction; switching columns applies that column's default direction. Either way paging restarts on page one. */
  const changeSort = (key: SortKey) => { setSort((current) => (current.key === key ? { key, dir: current.dir === 'asc' ? 'desc' : 'asc' } : { key, dir: SORT_DEFAULT_DIR[key] })); setPage(1); };
  const removeFilter = (key: keyof FindingFilter) => updateFilters({ ...filters, [key]: '' });
  const toggleSeverity = (severity: string) => updateFilters({ ...filters, severity: filters.severity === severity ? '' : severity });
  if (report.loading) return <section className="report"><SkeletonCards count={4} /><SkeletonLines lines={3} /></section>;
  if (report.error) return <ErrorPanel error={report.error} retry={report.reload} />;
  const data = report.data ?? ({} as Report);
  /** A report payload without its scan still renders: zeroed cards beat a crashed report view. */
  const dataScan = data.scan ?? ({ id: scanId, workspace_id: '', state: '' } as Scan);
  const workspaceId = dataScan.workspace_id || '';
  /** Suppression is workspace-scoped, so row actions only appear when the scan names its workspace and the finding carries a fingerprint. */
  const canManageSuppressions = Boolean(workspaceId);
  /** Rule options for the Rule filter: distinct rule ids from the report payload (it carries every finding, so the list is complete without another request), scoped to the selected analyzer when one is active. */
  const ruleOptions = [...new Set((data.findings ?? []).filter((item) => !filters.analyzer || item.analyzer_id === filters.analyzer).map((item) => item.rule_id).filter((rule): rule is string => Boolean(rule)))].sort((a, b) => a.localeCompare(b));
  /** Switching tools invalidates a rule choice the newly selected tool never reports, so the rule filter resets alongside (the server matches rules case-insensitively, so must this check). */
  const changeAnalyzer = (analyzer: string) => {
    const keepRule = !filters.rule || !analyzer || (data.findings ?? []).some((item) => item.analyzer_id === analyzer && (item.rule_id ?? '').toLowerCase() === filters.rule.toLowerCase());
    updateFilters(keepRule ? { ...filters, analyzer } : { ...filters, analyzer, rule: '' });
  };
  /** Deleting a suppression restores the finding: the row leaves the suppressed-filtered view, so the toast confirms what happened before the refresh. */
  async function restoreFinding(finding: Finding) {
    if (!finding.fingerprint || !workspaceId) return;
    try {
      await api.removeSuppression(workspaceId, finding.fingerprint);
      notify?.({ kind: 'success', text: 'Finding restored. It will be counted again on the next scan.' });
      await findings.reload();
    } catch (e) {
      notify?.({ kind: 'error', text: message(e) });
    }
  }
  const activeFilterCount = Object.values(filters).filter(Boolean).length;
  const clearFilters = () => updateFilters(EMPTY_FILTER);
  const noFindingsTitle = filters.analyzer ? `${analyzerName(filters.analyzer)} reported no findings` : 'No findings match these filters';
  const noFindingsCopy = filters.analyzer ? 'This analyzer completed without reportable issues for the selected files.' : 'Try clearing one or more filters.';
  /** A completed scan with zero findings and no active filters earns the celebratory panel; anything else keeps the filter-empty messaging. */
  const allClear = !activeFilterCount && dataScan.state === 'completed' && (dataScan.total_findings ?? 0) === 0;
  return <section className="report"><header className="report-header"><div><p className="eyebrow">Report</p><h2>Analysis overview</h2><p>{data.warnings?.length ? 'Analysis completed with limitations.' : 'Analysis completed.'}</p></div><ExportMenu scanId={scanId} csvParams={csvParams} findings={list} /></header><AnalyzerDurationChips runs={dataScan.analyzer_runs} />{data.warnings?.length ? <div className="inline-warning"><strong>Incomplete analysis</strong>{data.warnings.map((warning) => <span key={warning}>{warning}</span>)}</div> : null}<section className="summary-grid"><SummaryCard label="Total findings" value={dataScan.total_findings ?? 0} /><SummaryCard label="New" value={data.comparison?.new_count ?? dataScan.new_count ?? 0} /><SummaryCard label="Fixed" value={data.comparison?.fixed_count ?? 0} /><SummaryCard label="Persistent" value={data.comparison?.persistent_count ?? 0} /></section><SeverityDistribution scan={dataScan} /><AnalyzerResults runs={dataScan.analyzer_runs} findings={data.findings} selectedAnalyzer={filters.analyzer} onSelect={(analyzer) => changeAnalyzer(filters.analyzer === analyzer ? '' : analyzer)} /><section className="findings-section"><div className="section-head"><div><h2>Findings</h2><p>Filter results without leaving the report.</p></div><div className="findings-toolbar"><button type="button" className="button secondary density-toggle" aria-pressed={dense} title={dense ? 'Switch to comfortable row spacing' : 'Switch to compact row spacing'} onClick={toggleDensity}>{dense ? 'Comfortable' : 'Compact'}</button><button type="button" className="button secondary" aria-expanded={filtersOpen} aria-controls="finding-filters" onClick={() => setFiltersOpen((open) => !open)}>Filters{activeFilterCount ? ` (${activeFilterCount})` : ''}</button>{activeFilterCount ? <button type="button" className="text-button" onClick={clearFilters}>Clear</button> : null}</div></div><FilterChips filters={filters} onRemove={removeFilter} />{filtersOpen && <FindingFilters filters={filters} setFilters={updateFilters} analyzers={dataScan.analyzer_runs} rules={ruleOptions} onAnalyzer={changeAnalyzer} />}<SeverityPills scan={dataScan} selected={filters.severity} onToggle={toggleSeverity} /><div className={`finding-list${dense ? ' finding-dense' : ''}`}>{findings.loading ? <SkeletonTable rows={5} cols={5} className="findings-table" /> : findings.error ? <ErrorPanel error={findings.error} retry={findings.reload} /> : list.length ? <FindingsTable findings={list} sort={sort} onSort={changeSort} onPreview={setPreviewFinding} canSuppress={canManageSuppressions} onSuppress={setSuppressing} onRestore={restoreFinding} onFix={setFixFinding} onComment={setCommentFinding} /> : allClear ? <Empty title="All clear — no findings" tone="positive" icon={<><CheckShieldIcon /><span className="empty-sparkle one"><SparkleIcon /></span><span className="empty-sparkle two"><SparkleIcon /></span></>}>Every analyzer finished and found nothing to flag. Nice work.</Empty> : <Empty title={noFindingsTitle} icon={<MagnifierIcon />}>{noFindingsCopy}</Empty>}</div>{findings.data && <FindingsPagination total={findings.data.total ?? 0} page={page} pageSize={PAGE_SIZE} hasNext={findings.data.has_next ?? findings.data.has_more} onPrevious={() => setPage((current) => Math.max(1, current - 1))} onNext={() => setPage((current) => current + 1)} />}</section>{previewFinding && <FindingPreviewDialog scanId={scanId} finding={previewFinding} onClose={() => setPreviewFinding(undefined)} />}{suppressing && <SuppressFindingDialog workspaceId={workspaceId} finding={suppressing} onClose={() => setSuppressing(undefined)} onSuppressed={(reason) => { setSuppressing(undefined); notify?.({ kind: 'success', text: `Finding suppressed${reason ? ` (${reason})` : ''}. It will not appear in future scans, reports, or the CI gate.` }); void findings.reload(); }} notify={notify ?? noopNotify} />}<Sheet open={!!commentFinding} onOpenChange={(o) => { if (!o) setCommentFinding(undefined); }}><SheetContent side="right" className="w-full max-w-md overflow-y-auto sm:max-w-md" aria-label="Comments"><SheetHeader><SheetTitle className="flex items-center gap-2"><MessageCircle className="h-4 w-4 text-[var(--color-accent)]" aria-hidden="true" /> Comments</SheetTitle><SheetDescription>{commentFinding ? (commentFinding.title ?? commentFinding.rule_id ?? 'Finding') : ''}</SheetDescription></SheetHeader>{commentFinding && <div className="mt-4"><CommentsPanel fingerprint={commentFinding.fingerprint ?? commentFinding.id ?? 'unknown'} title={findingLocation(commentFinding)} /></div>}</SheetContent></Sheet><Sheet open={!!fixFinding} onOpenChange={(o) => { if (!o) setFixFinding(undefined); }}><SheetContent side="right" className="w-full max-w-xl overflow-y-auto sm:max-w-xl" aria-label="AI Fix suggestion"><SheetHeader><SheetTitle className="flex items-center gap-2"><Sparkles className="h-4 w-4 text-[var(--color-accent)]" aria-hidden="true" /> AI Fix</SheetTitle><SheetDescription>AI-generated suggestion — review before applying</SheetDescription></SheetHeader>{fixFinding && <AutoFixPanel finding={fixFinding} onClose={() => setFixFinding(undefined)} />}</SheetContent></Sheet></section>;
}

/** Header export dropdown (Loops 16/17/18): plain `<a download>` GET links for the Markdown/HTML/SARIF attachments plus a CSV of the currently filtered findings. Lightweight menu a11y — role=menu/menuitem, Escape and outside mousedown close, focus moves to the first item on open and back to the toggle on Escape. */
function ExportMenu({ scanId, csvParams, findings }: { scanId: string; csvParams: Record<string, string>; findings?: Finding[] }) {
  const [open, setOpen] = useState(false);
  const wrapRef = useRef<HTMLDivElement>(null);
  const toggleRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onKeyDown = (event: KeyboardEvent) => { if (event.key === 'Escape') { setOpen(false); toggleRef.current?.focus(); } };
    const onMouseDown = (event: MouseEvent) => { if (!wrapRef.current?.contains(event.target as Node)) setOpen(false); };
    document.addEventListener('keydown', onKeyDown);
    document.addEventListener('mousedown', onMouseDown);
    menuRef.current?.querySelector<HTMLElement>('a[role="menuitem"]')?.focus();
    return () => { document.removeEventListener('keydown', onKeyDown); document.removeEventListener('mousedown', onMouseDown); };
  }, [open]);
  /** Arrow keys cycle the items; anything else falls through to normal tabbing. */
  const onMenuKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return;
    event.preventDefault();
    const items = [...(menuRef.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])];
    if (!items.length) return;
    const current = Math.max(0, items.indexOf(document.activeElement as HTMLElement));
    const next = event.key === 'ArrowDown' ? (current + 1) % items.length : (current - 1 + items.length) % items.length;
    items[next].focus();
  };
  const items = [
    { label: 'Markdown report', ext: '.md', href: api.markdownUrl(scanId) },
    { label: 'HTML report', ext: '.html', href: api.exportUrl(scanId, 'html') },
    { label: 'SARIF (code scanning)', ext: '.sarif', href: api.exportUrl(scanId, 'sarif') },
    { label: 'Findings CSV (current filters)', ext: '.csv', href: api.exportUrl(scanId, 'csv', csvParams) },
  ];
  const handleJiraCsv = () => {
    const data = findings ?? [];
    if (!data.length) return;
    downloadJiraCsv(data, `jira-${scanId}.csv`);
    setOpen(false);
  };
  return <div className={`export-menu${open ? ' open' : ''}`} ref={wrapRef}><button ref={toggleRef} type="button" className="button secondary export-toggle" aria-haspopup="menu" aria-expanded={open} aria-controls={open ? 'export-menu' : undefined} onClick={() => setOpen((value) => !value)}>Export <span className="export-caret" aria-hidden="true">⌄</span></button>{open && <div id="export-menu" ref={menuRef} className="export-popover" role="menu" aria-label="Export report" onKeyDown={onMenuKeyDown}>{items.map((item) => <a key={item.label} role="menuitem" className="export-item" href={item.href} download onClick={() => setOpen(false)}>{item.label}<small>{item.ext}</small></a>)}<button type="button" role="menuitem" className="export-item" onClick={handleJiraCsv}>Jira CSV<small>.csv</small></button></div>}</div>;
}

function FilterChips({ filters, onRemove }: { filters: FindingFilter; onRemove: (key: keyof FindingFilter) => void }) {
  const chips: Array<{ key: keyof FindingFilter; text: string; removeLabel: string }> = [];
  if (filters.severity) chips.push({ key: 'severity', text: `severity: ${filters.severity}`, removeLabel: 'Remove severity filter' });
  if (filters.category) chips.push({ key: 'category', text: `category: ${filters.category}`, removeLabel: 'Remove category filter' });
  if (filters.analyzer) chips.push({ key: 'analyzer', text: `tool: ${analyzerName(filters.analyzer)}`, removeLabel: 'Remove tool filter' });
  if (filters.rule) chips.push({ key: 'rule', text: `rule: ${filters.rule}`, removeLabel: 'Remove rule filter' });
  if (filters.path) chips.push({ key: 'path', text: `file: ${filters.path}`, removeLabel: 'Remove file filter' });
  if (filters.status) chips.push({ key: 'status', text: `status: ${filters.status}`, removeLabel: 'Remove status filter' });
  if (filters.q) chips.push({ key: 'q', text: `search: "${filters.q}"`, removeLabel: 'Remove search filter' });
  if (!chips.length) return null;
  return <ul className="filter-chips" aria-label="Active filters">{chips.map((chip) => <li className="filter-chip" key={chip.key}>{chip.text}<button type="button" className="filter-chip-remove" aria-label={chip.removeLabel} title={chip.removeLabel} onClick={() => onRemove(chip.key)}>×</button></li>)}</ul>;
}

function SeverityPills({ scan, selected, onToggle }: { scan: Scan; selected: string; onToggle: (severity: string) => void }) {
  const counts = severityCounts(scan);
  return <fieldset className="severity-pills" aria-label="Filter findings by severity">{SEVERITY_ORDER.map((severity) => { const count = counts[severity]; const active = selected === severity; return <button type="button" key={severity} className={`severity-pill ${severity}${active ? ' selected' : ''}`} aria-pressed={active} disabled={count === 0 && !active} onClick={() => onToggle(severity)}>{severity}<span className="count">{count}</span></button>; })}</fieldset>;
}

/** Scan-wide severity share as one stacked bar with a counted legend. Pure CSS flex widths — no JS chart — and the segment colors reuse the HistoryPage severity-bar token language so severity reads the same everywhere. Hidden for zero-finding scans; the all-clear panel covers those. */
function SeverityDistribution({ scan }: { scan: Scan }) {
  const counts = severityCounts(scan);
  const total = SEVERITY_ORDER.reduce((sum, severity) => sum + counts[severity], 0);
  if (!total) return null;
  const label = `Findings by severity: ${severityBreakdownLabel(counts)}`;
  return <section className="severity-distribution" aria-label="Severity distribution"><div className="severity-stack severity-bar" role="img" aria-label={label} title={label}><SeveritySegments counts={counts} total={total} /></div><ul className="severity-legend">{SEVERITY_ORDER.map((severity) => <li key={severity} className={counts[severity] ? severity : 'zero'}><i className={`seg-${severity}`} aria-hidden="true" />{severity}<span className="legend-count">{counts[severity]}</span></li>)}</ul></section>;
}

/** Loop W1 · One chip per analyzer run in the scan, right under the report header: display name plus compact duration, tinted by outcome (success/failed) so a failed run is visible before scrolling into the analyzer list. Hidden entirely when the scan carried no runs. */
function AnalyzerDurationChips({ runs }: { runs?: AnalyzerRun[] }) {
  if (!runs?.length) return null;
  return <ul className="analyzer-run-chips" aria-label="Analyzer run durations">{runs.map((run, index) => <li key={`${run.analyzer_id}-${index}`} className={`analyzer-run-chip state ${run.status === 'succeeded' ? 'success' : run.status === 'failed' ? 'failed' : run.status}`} title={run.message || `${run.status}${run.version ? ` (${run.version})` : ''}`}><span className="analyzer-run-name">{analyzerName(run.analyzer_id)}</span><span className="analyzer-run-duration">{compactDuration(run.duration_ms)}</span></li>)}</ul>;
}

function AnalyzerResults({ runs, findings, selectedAnalyzer, onSelect }: { runs?: AnalyzerRun[]; findings?: Finding[]; selectedAnalyzer: string; onSelect: (analyzer: string) => void }) {
  const [categoryFilter, setCategoryFilter] = useState('');
  if (!runs?.length) return null;
  const splits = new Map<string, Record<Severity, number>>();
  for (const finding of findings ?? []) {
    const split = splits.get(finding.analyzer_id) ?? { ...ZERO_SEVERITY_COUNTS };
    split[finding.severity] += 1;
    splits.set(finding.analyzer_id, split);
  }
  const countFor = (analyzerId: string) => { if (!findings) return undefined; const split = splits.get(analyzerId); return split ? SEVERITY_ORDER.reduce((sum, severity) => sum + split[severity], 0) : 0; };
  const ordered = [...runs].sort((a, b) => (countFor(b.analyzer_id) ?? -1) - (countFor(a.analyzer_id) ?? -1));
  const categories = [...new Set(ordered.map((r) => analyzerMeta(r.analyzer_id)?.category ?? 'other'))];
  const filtered = categoryFilter ? ordered.filter((r) => (analyzerMeta(r.analyzer_id)?.category ?? 'other') === categoryFilter) : ordered;
  return <section className="analyzer-results" aria-labelledby="analyzer-results-title"><div className="section-head"><div><h2 id="analyzer-results-title">Analyzer results</h2><p>Every completed check is shown. Select one to filter the combined report.</p></div><div className="flex gap-2 items-center flex-wrap"><label className="text-xs">Category <select aria-label="Filter by category" value={categoryFilter} onChange={(e) => setCategoryFilter(e.target.value)} className="text-xs border rounded px-2 py-1"><option value="">All</option>{categories.map((c) => <option key={c} value={c}>{(CATEGORY_LABELS as Record<string,string>)[c] ?? c}</option>)}</select></label></div></div><div className="analyzer-result-list">{filtered.map((run) => { const count = countFor(run.analyzer_id); const split = splits.get(run.analyzer_id); const selected = selectedAnalyzer === run.analyzer_id; const status = run.status === 'succeeded' ? 'success' : run.status; const barLabel = split ? `${analyzerName(run.analyzer_id)} findings by severity: ${severityBreakdownLabel(split)}` : ''; const meta = analyzerMeta(run.analyzer_id); return <div key={run.analyzer_id} className="analyzer-result-wrap border rounded-lg" style={{ borderLeftColor: meta ? categoryColor(meta.category) : undefined, borderLeftWidth: meta ? '3px' : undefined }}><button type="button" className={`analyzer-result ${selected ? 'selected' : ''}`} aria-pressed={selected} onClick={() => onSelect(run.analyzer_id)}><span><strong>{analyzerName(run.analyzer_id)}</strong>{meta && <span className="badge text-[10px] ml-2" style={{ background: `color-mix(in oklch, ${categoryColor(meta.category)} 16%, var(--color-surface))`, color: categoryColor(meta.category), borderColor: `color-mix(in oklch, ${categoryColor(meta.category)} 34%, var(--color-rule))` }}>{meta.category}</span>}<small>{run.status === 'succeeded' && count === 0 ? 'Completed with no findings' : run.message || run.status}</small>{meta && <span className="flex gap-1 mt-1 flex-wrap">{meta.languages.slice(0,3).map((l) => <span key={l} className="badge text-[10px]">{l}</span>)}{meta.docUrl && <a href={meta.docUrl} target="_blank" rel="noreferrer" className="text-xs underline" onClick={(e) => e.stopPropagation()}>docs</a>}</span>}</span><span className={`state ${status}`}>{count === undefined ? 'Results unavailable' : `${count} ${count === 1 ? 'finding' : 'findings'}`}</span>{split && count !== undefined && count > 0 ? <span className="analyzer-bar severity-bar" role="img" aria-label={barLabel} title={barLabel}><SeveritySegments counts={split} total={count} /></span> : null}</button></div>; })}</div></section>;
}

function FindingFilters({ filters, setFilters, analyzers, rules, onAnalyzer }: { filters: FindingFilter; setFilters: (next: FindingFilter) => void; analyzers?: AnalyzerRun[]; rules: string[]; onAnalyzer: (analyzer: string) => void }) { const set = (key: keyof FindingFilter, value: string) => setFilters({ ...filters, [key]: value }); const tools = [...new Set(analyzers?.map((run) => run.analyzer_id) ?? [])]; return <div className="filters" id="finding-filters"><label>Severity<select value={filters.severity} onChange={(e) => set('severity', e.target.value)}><option value="">All</option>{['critical', 'high', 'medium', 'low', 'info'].map((value) => <option value={value} key={value}>{value}</option>)}</select></label><label>Category<input value={filters.category} onChange={(e) => set('category', e.target.value)} placeholder="All categories" /></label><label>Tool<select value={filters.analyzer} onChange={(e) => onAnalyzer(e.target.value)}><option value="">All tools</option>{tools.map((tool) => <option value={tool} key={tool}>{analyzerName(tool)}</option>)}</select></label><label>Rule<select value={filters.rule} onChange={(e) => set('rule', e.target.value)}><option value="">All rules</option>{rules.map((rule) => <option value={rule} key={rule}>{rule}</option>)}</select></label><label>File<input value={filters.path} onChange={(e) => set('path', e.target.value)} placeholder="Any path" /></label><label>Status<select value={filters.status} onChange={(e) => set('status', e.target.value)}><option value="">All</option><option value="new">New</option><option value="persistent">Persistent</option><option value="suppressed">Suppressed</option></select></label><label className="filter-search">Search<input value={filters.q} onChange={(e) => set('q', e.target.value)} placeholder="Message or rule" /></label></div>; }

/** Page-mode pagination: Previous/Next walk the server's `page` windows while the status line narrates both the slice and the page count (announced politely via the same `<output aria-live>` pattern as the scan history pager). `hasNext` comes from the envelope's `has_next`, falling back to legacy `has_more` envelopes. */
function FindingsPagination({ total, page, pageSize, hasNext, onPrevious, onNext }: { total: number; page: number; pageSize: number; hasNext: boolean; onPrevious: () => void; onNext: () => void }) { const pageCount = Math.max(1, Math.ceil(total / pageSize)); const first = total ? (page - 1) * pageSize + 1 : 0; const last = Math.min(page * pageSize, total); return <nav className="findings-pagination" aria-label="Findings pagination"><span>Showing {first}–{last} of {total}</span><div><button type="button" className="button secondary" onClick={onPrevious} disabled={page <= 1}>Previous</button><output aria-live="polite">Page {page} of {pageCount}</output><button type="button" className="button secondary" onClick={onNext} disabled={!hasNext}>Next</button></div></nav>; }

function SortHeader({ label, column, sort, onSort }: { label: string; column: SortKey; sort: SortState; onSort: (key: SortKey) => void }) { const active = sort.key === column; return <th scope="col" aria-sort={active ? (sort.dir === 'asc' ? 'ascending' : 'descending') : 'none'}><button type="button" className={`th-sort${active ? ' active' : ''}`} onClick={() => onSort(column)}>{label}<span className="sort-arrow" aria-hidden="true">{active ? (sort.dir === 'asc' ? '▲' : '▼') : '↕'}</span>{active && <span className="sr-only"> (sorted {sort.dir === 'asc' ? 'ascending' : 'descending'})</span>}</button></th>; }

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

/** Loop 25 · Findings table: each row ends in a copy-details icon button (clipboard with a hidden-textarea fallback for non-secure loopback origins; success swaps in a check for 800ms). The copy buttons form a roving-tabindex group — only the current row's button is tabbable, ArrowUp/ArrowDown/Home/End move focus, and the handlers live on the buttons themselves so arrows are never hijacked elsewhere. Suppression actions are plain labelled buttons (always tabbable): "Suppress…" on fingerprinted findings, "Restore" once status flips to suppressed. */
function FindingsTable({ findings, sort, onSort, onPreview, canSuppress, onSuppress, onRestore, onFix, onComment }: { findings: Finding[]; sort: SortState; onSort: (key: SortKey) => void; onPreview: (finding: Finding) => void; canSuppress: boolean; onSuppress: (finding: Finding) => void; onRestore: (finding: Finding) => void; onFix: (finding: Finding) => void; onComment: (finding: Finding) => void }) {
  const [focusIndex, setFocusIndex] = useState(0);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const copyTimer = useRef<number | undefined>(undefined);
  const copyRefs = useRef<Array<HTMLButtonElement | null>>([]);
  useEffect(() => () => window.clearTimeout(copyTimer.current), []);
  const activeIndex = Math.min(focusIndex, Math.max(0, findings.length - 1));
  const onCopyKeyDown = (event: ReactKeyboardEvent<HTMLButtonElement>, index: number) => {
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
    event.preventDefault();
    const last = findings.length - 1;
    const next = event.key === 'ArrowDown' ? Math.min(index + 1, last) : event.key === 'ArrowUp' ? Math.max(index - 1, 0) : event.key === 'Home' ? 0 : last;
    setFocusIndex(next);
    copyRefs.current[next]?.focus();
  };
  const copyFinding = async (finding: Finding) => {
    if (!(await copyToClipboard(findingSummaryText(finding)))) return;
    setCopiedKey(finding.id || finding.fingerprint || null);
    window.clearTimeout(copyTimer.current);
    copyTimer.current = window.setTimeout(() => setCopiedKey(null), COPY_CONFIRM_MS);
  };
  // bulk selection (checkbox per row, additive header)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const allIds = findings.map((f, i) => f.id || f.fingerprint || String(i));
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
  // row expansion
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const toggleExpand = (id: string) => { const n = new Set(expanded); if (n.has(id)) n.delete(id); else n.add(id); setExpanded(n); };
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
  const rowHeight = 'var(--row-height, 52px)';
  const handleBulkCopy = async () => {
    const sel = findings.filter((f, i) => selectedIds.has(f.id || f.fingerprint || String(i)));
    const text = sel.map(findingSummaryText).join('\n\n');
    if (text) await copyToClipboard(text);
  };
  const handleBulkExport = () => {
    const sel = findings.filter((f, i) => selectedIds.has(f.id || f.fingerprint || String(i)));
    if (!sel.length) return;
    const header = ['severity','rule_id','message','path','analyzer_id'].join(',');
    const rows = sel.map((f) => [f.severity, f.rule_id ?? '', `"${(f.message ?? '').replace(/"/g, '""')}"`, f.relative_path ?? '', f.analyzer_id].join(',')).join('\n');
    const blob = new Blob([header + '\n' + rows], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a'); a.href = url; a.download = 'findings-selected.csv'; a.click(); URL.revokeObjectURL(url);
  };
  const handleBulkSuppress = () => {
    const sel = findings.filter((f, i) => selectedIds.has(f.id || f.fingerprint || String(i)) && f.fingerprint && canSuppress);
    sel.forEach((f) => onSuppress(f));
  };
  const colIds = ['severity','finding','path','analyzer','status','fix'];
  const headerStyle = (id: string) => widths[id] ? { width: widths[id], minWidth: widths[id] } : undefined;
  const resizeHandle = (id: string) => (
    <span role="separator" aria-orientation="vertical" aria-label={'Resize '+id+' column'} onPointerDown={(e)=>onResizeStart(e,id,widths[id]??160)} className="absolute right-0 top-0 h-full w-2 cursor-col-resize touch-none select-none hover:bg-[var(--color-accent-ghost)]" tabIndex={0} onKeyDown={(e)=>{ if(e.key==='ArrowLeft') setWidth(id,(widths[id]??160)-16); if(e.key==='ArrowRight') setWidth(id,(widths[id]??160)+16); }} />
  );
  return <div className="space-y-2" style={{ ['--row-height']: rowHeight } as React.CSSProperties}>
    {selectedCount>0 && <div role="toolbar" aria-label="Bulk actions" className="flex flex-wrap items-center gap-2 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface-muted)] px-3 py-2 shadow-[var(--shadow-card)]"><span className="inline-flex items-center rounded-[var(--radius-button)] bg-[var(--color-accent)] px-2 py-0.5 text-xs font-bold text-[var(--color-accent-ink)]" aria-live="polite">{selectedCount} selected</span><button type="button" onClick={handleBulkSuppress} className="inline-flex h-7 items-center gap-1.5 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold shadow-[var(--shadow-card)]">Suppress selected</button><button type="button" onClick={handleBulkExport} className="inline-flex h-7 items-center gap-1.5 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold shadow-[var(--shadow-card)]">Export selected CSV</button><button type="button" onClick={handleBulkCopy} className="inline-flex h-7 items-center gap-1.5 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold shadow-[var(--shadow-card)]">Copy</button></div>}
    <div className="findings-table table-wrap"><table><caption className="sr-only">Findings matching the current filters</caption><thead><tr>
      <th scope="col" className="relative sticky left-0 z-[2] bg-[var(--color-surface-muted)] shadow-[var(--shadow-card)]" style={headerStyle('severity')} aria-sort={sort.key==='severity'?(sort.dir==='asc'?'ascending':'descending'):'none'}><div className="flex items-center gap-2"><input ref={selectAllRef} type="checkbox" role="checkbox" aria-label="Select all" checked={!!allSelected} onChange={(e)=>toggleSelectAll(e.target.checked)} /><button type="button" className={'th-sort'+(sort.key==='severity'?' active':'')} onClick={()=>onSort('severity')}>Severity<span className="sort-arrow" aria-hidden="true">{sort.key==='severity'?(sort.dir==='asc'?'▲':'▼'):'↕'}</span>{sort.key==='severity' && <span className="sr-only"> (sorted {sort.dir==='asc'?'ascending':'descending'})</span>}</button></div>{resizeHandle('severity')}</th>
      <th scope="col" className="relative" style={headerStyle('finding')}>Finding{resizeHandle('finding')}</th>
      <th scope="col" className="relative" style={headerStyle('path')} aria-sort={sort.key==='path'?(sort.dir==='asc'?'ascending':'descending'):'none'}><button type="button" className={'th-sort'+(sort.key==='path'?' active':'')} onClick={()=>onSort('path')}>File<span className="sort-arrow" aria-hidden="true">{sort.key==='path'?(sort.dir==='asc'?'▲':'▼'):'↕'}</span></button>{resizeHandle('path')}</th>
      <th scope="col" className="relative" style={headerStyle('analyzer')} aria-sort={sort.key==='analyzer'?(sort.dir==='asc'?'ascending':'descending'):'none'}><button type="button" className={'th-sort'+(sort.key==='analyzer'?' active':'')} onClick={()=>onSort('analyzer')}>Tool<span className="sort-arrow" aria-hidden="true">{sort.key==='analyzer'?(sort.dir==='asc'?'▲':'▼'):'↕'}</span></button>{resizeHandle('analyzer')}</th>
      <th scope="col" className="relative" style={headerStyle('status')} aria-sort={sort.key==='status'?(sort.dir==='asc'?'ascending':'descending'):'none'}><button type="button" className={'th-sort'+(sort.key==='status'?' active':'')} onClick={()=>onSort('status')}>Status<span className="sort-arrow" aria-hidden="true">{sort.key==='status'?(sort.dir==='asc'?'▲':'▼'):'↕'}</span></button>{resizeHandle('status')}</th>
      <th scope="col" className="relative" style={headerStyle('fix')}>Fix{resizeHandle('fix')}</th>
    </tr></thead><tbody>{findings.map((finding, index) => { const rowKey = finding.id || finding.fingerprint || String(index); const copied = copiedKey === rowKey; const rowName = finding.title ?? finding.rule_id ?? 'finding'; const suppressed = finding.status === 'suppressed'; const isSelected = selectedIds.has(rowKey); const isExpanded = expanded.has(rowKey); const related = isExpanded ? findings.filter((f, i)=> i!==index && (f.rule_id===finding.rule_id || f.relative_path===finding.relative_path)).slice(0,3) : []; const rowAction = canSuppress && finding.fingerprint ? suppressed ? <button type="button" className="text-button restore-finding" aria-label={'Restore '+rowName} title="Stop hiding this finding" onClick={(e)=>{ e.stopPropagation(); onRestore(finding);}}>Restore</button> : <button type="button" className="text-button suppress-finding" aria-label={'Suppress '+rowName} title="Hide this finding from future scans" onClick={(e)=>{ e.stopPropagation(); onSuppress(finding);}}>Suppress…</button> : null; return <React.Fragment key={rowKey}><tr className={severityEdgeClass(finding.severity)+(isSelected?' selected':'')} data-state={isSelected?'selected':undefined} aria-label={`${finding.severity} ${rowName}${finding.relative_path ? ` in ${finding.relative_path}` : ''} — ${isExpanded ? 'collapse' : 'expand'} details`} style={{ height: rowHeight } as React.CSSProperties} onClick={()=>toggleExpand(rowKey)} tabIndex={0} onKeyDown={(e)=>{ if(e.key==='Enter'||e.key===' '){ e.preventDefault(); toggleExpand(rowKey); } }}><td className="sticky left-0 z-[1] bg-[var(--color-surface)] shadow-[var(--shadow-card)]" style={headerStyle('severity')} onClick={(e)=>e.stopPropagation()}><span className="flex items-center gap-2"><input type="checkbox" role="checkbox" aria-label={'Select row '+rowKey} checked={isSelected} onChange={(e)=>toggleRowSelect(rowKey,e.target.checked)} onClick={(e)=>e.stopPropagation()} /><span className={'severity '+finding.severity}>{finding.severity}</span></span></td><td className="finding-summary" style={headerStyle('finding')}><strong>{finding.title ?? finding.rule_id ?? 'Finding'}</strong>{isExpanded ? <span className="sr-only"> — expanded, showing remediation</span> : null}<span className="finding-message">{finding.message}</span>{finding.remediation && <span className="finding-remediation">Fix: {finding.remediation}</span>}{finding.documentation_url && <a href={finding.documentation_url} target="_blank" rel="noreferrer" onClick={(e)=>e.stopPropagation()}>Rule docs</a>}</td><td style={headerStyle('path')} onClick={(e)=>e.stopPropagation()}>{finding.relative_path && finding.id ? <button type="button" className="finding-file" onClick={() => onPreview(finding)} aria-label={'Preview '+findingLocation(finding)}><code>{findingLocation(finding)}</code></button> : <code>{findingLocation(finding)}</code>}</td><td style={headerStyle('analyzer')}><span className="badge">{finding.analyzer_id}</span>{finding.rule_id && <code>{finding.rule_id}</code>}</td><td style={headerStyle('status')}><div className="finding-actions-cell">{finding.status ? <span className={'status-text'+(suppressed ? ' suppressed' : '')}>{finding.status}</span> : '—'}<span className="finding-row-actions" onClick={(e)=>e.stopPropagation()}>{rowAction}<button type="button" className="icon-button" aria-label={'Comment on '+rowName} title="Comment" onClick={() => onComment(finding)}><MessageCircle className="h-4 w-4" aria-hidden="true" /></button><a href={buildJiraIssueUrl(finding)} target="_blank" rel="noreferrer noopener" className="text-button" aria-label={'Create Jira issue for '+rowName} title="Create Jira issue (external stub)">Jira ↗</a><button ref={(el)=>{ copyRefs.current[index]=el; }} type="button" className={'icon-button copy-finding'+(copied ? ' copied' : '')} tabIndex={index===activeIndex?0:-1} aria-label="Copy finding details" title="Copy finding details" onKeyDown={(e)=>onCopyKeyDown(e,index)} onClick={()=> void copyFinding(finding)}>{copied ? <CheckIcon /> : <ClipboardIcon />}</button></span></div></td><td style={headerStyle('fix')} onClick={(e)=>e.stopPropagation()}><button type="button" className="button secondary" style={{ borderRadius: 'var(--radius-button)', padding: '0.25rem 0.5rem', fontSize: '12px' }} aria-label={'AI Fix for '+rowName} onClick={() => onFix(finding)}><Sparkles width={14} height={14} aria-hidden="true" /> AI Fix</button></td></tr>{isExpanded && <tr className="bg-[var(--color-surface-muted)]"><td colSpan={6} className="p-0"><div className="px-4 py-3 text-sm grid gap-2 motion-reduce:transition-none animate-[page-enter_var(--dur-slow)_var(--ease-out)] motion-reduce:animate-none"><div><strong className="text-[var(--color-ink)]">Remediation</strong><p className="text-[var(--color-ink-soft)]">{finding.remediation || 'No remediation provided. See rule docs.'}</p></div>{related.length>0 && <div><strong className="text-[var(--color-ink)]">Related findings</strong><ul className="list-disc pl-4 text-[var(--color-ink-soft)]">{related.map((r)=> <li key={r.id||r.fingerprint}><code>{r.rule_id}</code> — {r.message} <span className="text-[var(--color-ink-faint)]">{r.relative_path}</span></li>)}</ul></div>}</div></td></tr>}</React.Fragment>; })}</tbody></table></div></div>;
}

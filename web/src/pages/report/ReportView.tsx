import { useMemo, useState } from 'react';
import { api } from '../../api';
import type { AnalyzerRun, Finding, Scan, Severity } from '../../types';
import { analyzerName, findingLocation } from '../../lib/format';
import { useLoad } from '../../hooks/useLoad';
import { useDebouncedValue } from '../../hooks/useDebouncedValue';
import { Empty, ErrorPanel, SummaryCard } from '../../components/ui';
import { SkeletonCards, SkeletonLines, SkeletonTable } from '../../components/skeletons';
import { FindingPreviewDialog } from '../../components/dialogs';

export type FindingFilter = { severity: string; category: string; analyzer: string; path: string; status: string; q: string };

const EMPTY_FILTER: FindingFilter = { severity: '', category: '', analyzer: '', path: '', status: '', q: '' };
const SEVERITY_ORDER: Severity[] = ['critical', 'high', 'medium', 'low', 'info'];
/** Text filters wait for typing to pause before hitting the API; empty values flush instantly. */
const TEXT_FILTER_DELAY_MS = 300;

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

export function ReportView({ scanId }: { scanId: string }) {
  const report = useLoad(() => api.report(scanId), [scanId]);
  const [filters, setFilters] = useState<FindingFilter>(EMPTY_FILTER);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [offset, setOffset] = useState(0);
  const [limit, setLimit] = useState(25);
  const [sort, setSort] = useState<SortState>({ key: 'severity', dir: 'asc' });
  const [previewFinding, setPreviewFinding] = useState<Finding>();
  const debouncedCategory = useDebouncedValue(filters.category, filters.category ? TEXT_FILTER_DELAY_MS : 0);
  const debouncedPath = useDebouncedValue(filters.path, filters.path ? TEXT_FILTER_DELAY_MS : 0);
  const debouncedQ = useDebouncedValue(filters.q, filters.q ? TEXT_FILTER_DELAY_MS : 0);
  const params = useMemo(() => ({ ...filters, category: debouncedCategory, path: debouncedPath, q: debouncedQ, limit: String(limit), offset: String(offset), sort: sort.key, order: sort.dir }), [filters, debouncedCategory, debouncedPath, debouncedQ, limit, offset, sort]);
  const findings = useLoad(() => api.findings(scanId, params), [scanId, ...Object.values(params)]);
  const list = findings.data?.items ?? [];
  const updateFilters = (next: FindingFilter) => { setFilters(next); setOffset(0); };
  /** Toggles the active column's direction; switching columns applies that column's default direction. Either way paging restarts on page one. */
  const changeSort = (key: SortKey) => { setSort((current) => (current.key === key ? { key, dir: current.dir === 'asc' ? 'desc' : 'asc' } : { key, dir: SORT_DEFAULT_DIR[key] })); setOffset(0); };
  const removeFilter = (key: keyof FindingFilter) => updateFilters({ ...filters, [key]: '' });
  const toggleSeverity = (severity: string) => updateFilters({ ...filters, severity: filters.severity === severity ? '' : severity });
  if (report.loading) return <section className="report"><SkeletonCards count={4} /><SkeletonLines lines={3} /></section>;
  if (report.error) return <ErrorPanel error={report.error} retry={report.reload} />;
  const data = report.data!;
  const activeFilterCount = Object.values(filters).filter(Boolean).length;
  const clearFilters = () => updateFilters(EMPTY_FILTER);
  const noFindingsTitle = filters.analyzer ? `${analyzerName(filters.analyzer)} reported no findings` : 'No findings match these filters';
  const noFindingsCopy = filters.analyzer ? 'This analyzer completed without reportable issues for the selected files.' : 'Try clearing one or more filters.';
  return <section className="report"><header className="report-header"><div><p className="eyebrow">Report</p><h2>Analysis overview</h2><p>{data.warnings?.length ? 'Analysis completed with limitations.' : 'Analysis completed.'}</p></div><a className="button secondary" href={api.markdownUrl(scanId)}>Export Markdown</a></header>{data.warnings?.length ? <div className="inline-warning"><strong>Incomplete analysis</strong>{data.warnings.map((warning) => <span key={warning}>{warning}</span>)}</div> : null}<section className="summary-grid"><SummaryCard label="Total findings" value={data.scan.total_findings ?? 0} /><SummaryCard label="New" value={data.comparison?.new_count ?? data.scan.new_count ?? 0} /><SummaryCard label="Fixed" value={data.comparison?.fixed_count ?? 0} /><SummaryCard label="Persistent" value={data.comparison?.persistent_count ?? 0} /></section><AnalyzerResults runs={data.scan.analyzer_runs} findings={data.findings} selectedAnalyzer={filters.analyzer} onSelect={(analyzer) => updateFilters({ ...filters, analyzer: filters.analyzer === analyzer ? '' : analyzer })} /><section className="findings-section"><div className="section-head"><div><h2>Findings</h2><p>Filter results without leaving the report.</p></div><div className="findings-toolbar"><button className="button secondary" aria-expanded={filtersOpen} aria-controls="finding-filters" onClick={() => setFiltersOpen((open) => !open)}>Filters{activeFilterCount ? ` (${activeFilterCount})` : ''}</button>{activeFilterCount ? <button className="text-button" onClick={clearFilters}>Clear</button> : null}</div></div><FilterChips filters={filters} onRemove={removeFilter} />{filtersOpen && <FindingFilters filters={filters} setFilters={updateFilters} analyzers={data.scan.analyzer_runs} />}<SeverityPills scan={data.scan} selected={filters.severity} onToggle={toggleSeverity} /><div className="finding-list">{findings.loading ? <SkeletonTable rows={5} cols={5} className="findings-table" /> : findings.error ? <ErrorPanel error={findings.error} retry={findings.reload} /> : list.length ? <FindingsTable findings={list} sort={sort} onSort={changeSort} onPreview={setPreviewFinding} /> : <Empty title={noFindingsTitle}>{noFindingsCopy}</Empty>}</div>{findings.data && <FindingsPagination total={findings.data.total} offset={findings.data.offset} limit={findings.data.limit} hasMore={findings.data.has_more} onLimit={(next) => { setLimit(next); setOffset(0); }} onPrevious={() => setOffset(Math.max(0, offset - limit))} onNext={() => findings.data?.next_offset !== undefined && setOffset(findings.data.next_offset)} />}</section>{previewFinding && <FindingPreviewDialog scanId={scanId} finding={previewFinding} onClose={() => setPreviewFinding(undefined)} />}</section>;
}

function FilterChips({ filters, onRemove }: { filters: FindingFilter; onRemove: (key: keyof FindingFilter) => void }) {
  const chips: Array<{ key: keyof FindingFilter; text: string; removeLabel: string }> = [];
  if (filters.severity) chips.push({ key: 'severity', text: `severity: ${filters.severity}`, removeLabel: 'Remove severity filter' });
  if (filters.category) chips.push({ key: 'category', text: `category: ${filters.category}`, removeLabel: 'Remove category filter' });
  if (filters.analyzer) chips.push({ key: 'analyzer', text: `tool: ${analyzerName(filters.analyzer)}`, removeLabel: 'Remove tool filter' });
  if (filters.path) chips.push({ key: 'path', text: `file: ${filters.path}`, removeLabel: 'Remove file filter' });
  if (filters.status) chips.push({ key: 'status', text: `status: ${filters.status}`, removeLabel: 'Remove status filter' });
  if (filters.q) chips.push({ key: 'q', text: `search: "${filters.q}"`, removeLabel: 'Remove search filter' });
  if (!chips.length) return null;
  return <div className="filter-chips" aria-label="Active filters">{chips.map((chip) => <span className="filter-chip" key={chip.key}>{chip.text}<button type="button" className="filter-chip-remove" aria-label={chip.removeLabel} title={chip.removeLabel} onClick={() => onRemove(chip.key)}>×</button></span>)}</div>;
}

function SeverityPills({ scan, selected, onToggle }: { scan: Scan; selected: string; onToggle: (severity: string) => void }) {
  const counts: Record<Severity, number> = { critical: scan.critical_count ?? 0, high: scan.high_count ?? 0, medium: scan.medium_count ?? 0, low: scan.low_count ?? 0, info: scan.info_count ?? 0 };
  return <div className="severity-pills" role="group" aria-label="Filter findings by severity">{SEVERITY_ORDER.map((severity) => { const count = counts[severity]; const active = selected === severity; return <button type="button" key={severity} className={`severity-pill ${severity}${active ? ' selected' : ''}`} aria-pressed={active} disabled={count === 0 && !active} onClick={() => onToggle(severity)}>{severity}<span className="count">{count}</span></button>; })}</div>;
}

function AnalyzerResults({ runs, findings, selectedAnalyzer, onSelect }: { runs?: AnalyzerRun[]; findings?: Finding[]; selectedAnalyzer: string; onSelect: (analyzer: string) => void }) {
  if (!runs?.length) return null;
  return <section className="analyzer-results" aria-labelledby="analyzer-results-title"><div className="section-head"><div><h2 id="analyzer-results-title">Analyzer results</h2><p>Every completed check is shown. Select one to filter the combined report.</p></div></div><div className="analyzer-result-list">{runs.map((run) => { const count = findings ? findings.filter((finding) => finding.analyzer_id === run.analyzer_id).length : undefined; const selected = selectedAnalyzer === run.analyzer_id; const status = run.status === 'succeeded' ? 'success' : run.status; return <button type="button" className={`analyzer-result ${selected ? 'selected' : ''}`} aria-pressed={selected} key={run.analyzer_id} onClick={() => onSelect(run.analyzer_id)}><span><strong>{analyzerName(run.analyzer_id)}</strong><small>{run.status === 'succeeded' && count === 0 ? 'Completed with no findings' : run.message || run.status}</small></span><span className={`state ${status}`}>{count === undefined ? 'Results unavailable' : `${count} ${count === 1 ? 'finding' : 'findings'}`}</span></button>; })}</div></section>;
}

function FindingFilters({ filters, setFilters, analyzers }: { filters: FindingFilter; setFilters: (next: FindingFilter) => void; analyzers?: AnalyzerRun[] }) { const set = (key: keyof FindingFilter, value: string) => setFilters({ ...filters, [key]: value }); const tools = [...new Set(analyzers?.map((run) => run.analyzer_id) ?? [])]; return <div className="filters" id="finding-filters"><label>Severity<select value={filters.severity} onChange={(e) => set('severity', e.target.value)}><option value="">All</option>{['critical', 'high', 'medium', 'low', 'info'].map((value) => <option value={value} key={value}>{value}</option>)}</select></label><label>Category<input value={filters.category} onChange={(e) => set('category', e.target.value)} placeholder="All categories" /></label><label>Tool<select value={filters.analyzer} onChange={(e) => set('analyzer', e.target.value)}><option value="">All tools</option>{tools.map((tool) => <option value={tool} key={tool}>{analyzerName(tool)}</option>)}</select></label><label>File<input value={filters.path} onChange={(e) => set('path', e.target.value)} placeholder="Any path" /></label><label>Status<select value={filters.status} onChange={(e) => set('status', e.target.value)}><option value="">All</option><option value="new">New</option><option value="persistent">Persistent</option></select></label><label className="filter-search">Search<input value={filters.q} onChange={(e) => set('q', e.target.value)} placeholder="Message or rule" /></label></div>; }

function FindingsPagination({ total, offset, limit, hasMore, onLimit, onPrevious, onNext }: { total: number; offset: number; limit: number; hasMore: boolean; onLimit: (value: number) => void; onPrevious: () => void; onNext: () => void }) { const first = total ? offset + 1 : 0; const last = Math.min(offset + limit, total); return <nav className="findings-pagination" aria-label="Findings pagination"><span>Showing {first}–{last} of {total}</span><label>Rows<select value={limit} onChange={(event) => onLimit(Number(event.target.value))}><option value={25}>25</option><option value={50}>50</option><option value={100}>100</option></select></label><div><button className="button secondary" onClick={onPrevious} disabled={offset === 0}>Previous</button><button className="button secondary" onClick={onNext} disabled={!hasMore}>Next</button></div></nav>; }

function SortHeader({ label, column, sort, onSort }: { label: string; column: SortKey; sort: SortState; onSort: (key: SortKey) => void }) { const active = sort.key === column; return <th scope="col" aria-sort={active ? (sort.dir === 'asc' ? 'ascending' : 'descending') : 'none'}><button type="button" className={`th-sort${active ? ' active' : ''}`} onClick={() => onSort(column)}>{label}<span className="sort-arrow" aria-hidden="true">{active ? (sort.dir === 'asc' ? '▲' : '▼') : '↕'}</span>{active && <span className="sr-only"> (sorted {sort.dir === 'asc' ? 'ascending' : 'descending'})</span>}</button></th>; }

function FindingsTable({ findings, sort, onSort, onPreview }: { findings: Finding[]; sort: SortState; onSort: (key: SortKey) => void; onPreview: (finding: Finding) => void }) { return <div className="findings-table table-wrap"><table><thead><tr><SortHeader label="Severity" column="severity" sort={sort} onSort={onSort} /><th scope="col">Finding</th><SortHeader label="File" column="path" sort={sort} onSort={onSort} /><SortHeader label="Tool" column="analyzer" sort={sort} onSort={onSort} /><SortHeader label="Status" column="status" sort={sort} onSort={onSort} /></tr></thead><tbody>{findings.map((finding) => <tr key={finding.id || finding.fingerprint}><td><span className={`severity ${finding.severity}`}>{finding.severity}</span></td><td className="finding-summary"><strong>{finding.title ?? finding.rule_id ?? 'Finding'}</strong><span className="finding-message">{finding.message}</span>{finding.remediation && <span className="finding-remediation">Fix: {finding.remediation}</span>}{finding.documentation_url && <a href={finding.documentation_url} target="_blank" rel="noreferrer">Rule docs</a>}</td><td>{finding.relative_path && finding.id ? <button type="button" className="finding-file" onClick={() => onPreview(finding)} aria-label={`Preview ${findingLocation(finding)}`}><code>{findingLocation(finding)}</code></button> : <code>{findingLocation(finding)}</code>}</td><td><span className="badge">{finding.analyzer_id}</span>{finding.rule_id && <code>{finding.rule_id}</code>}</td><td>{finding.status ? <span className="status-text">{finding.status}</span> : '—'}</td></tr>)}</tbody></table></div>; }

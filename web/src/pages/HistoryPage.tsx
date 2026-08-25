import { Fragment, useEffect, useState } from 'react';
import { api } from '../api';
import '../css/history.css';
import type { Scan } from '../types';
import type { Route } from '../lib/router';
import { isTerminalScanState } from '../lib/scanEvents';
import { compactDuration, date, relativeTime } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel } from '../components/ui';
import { ScanIcon } from '../components/icons';
import { SkeletonTable } from '../components/skeletons';

export function HistoryPage({ workspaceId, go }: { workspaceId: string; go: (r: Route) => void }) {
  const state = useLoad(() => api.scans(workspaceId), [workspaceId]);
  return <div className="page"><header className="page-heading"><div><p className="eyebrow">Scan history</p><h1>Previous analyses</h1><p>Reports and findings are stored only on this computer.</p></div></header>{state.loading ? <SkeletonTable rows={6} cols={7} /> : state.error ? <ErrorPanel error={state.error} retry={state.reload} /> : <HistoryTable scans={state.data ?? []} go={go} />}</div>;
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
  if (!total) return <span className="findings-zero">0</span>;
  const breakdown = `${counts.critical} critical · ${counts.high} high · ${counts.medium} medium · ${counts.low} low`;
  return <div className="findings-cell"><span className="findings-total">{total}</span>{barTotal > 0 ? <div className="severity-bar" role="img" aria-label={breakdown} title={breakdown}>{barSeverities.filter((severity) => counts[severity] > 0).map((severity) => <i key={severity} className={`bar-${severity}`} style={{ width: segmentWidth(counts[severity], barTotal) }} />)}</div> : <div className="severity-bar" role="img" aria-label={breakdown} title={breakdown} />}</div>;
}

export function HistoryTable({ scans, go }: { scans: Scan[]; go: (r: Route) => void }) {
  const [page, setPage] = useState(0);
  const pageCount = Math.max(1, Math.ceil(scans.length / historyPageSize));
  const currentPage = Math.min(page, pageCount - 1);
  const first = currentPage * historyPageSize;
  const visibleScans = scans.slice(first, first + historyPageSize);

  // Reset pagination whenever a different scan set loads (a new scan always
  // changes the newest id, so it captures exactly the interesting changes).
  const newestScanId = scans[0]?.id;
  useEffect(() => { if (newestScanId !== undefined) setPage(0); }, [newestScanId]);

  if (!scans.length) return <Empty title="No scans yet" icon={<ScanIcon />}>Analyze this workspace to create the first report.</Empty>;

  return <><div className="table-wrap"><table><thead><tr><th scope="col">Date</th><th scope="col">Status</th><th scope="col">Findings</th><th scope="col">New</th><th scope="col">Fixed</th><th scope="col">Duration</th><th scope="col"><span className="sr-only">Actions</span></th></tr></thead><tbody>{historyDateBands(visibleScans).map(({ band, scans: banded }) => <Fragment key={band}><tr className="history-band-row"><th className="history-band" colSpan={7} scope="colgroup">{band}</th></tr>{banded.map((scan) => {
    const tone = scan.id === scans[0].id ? scan.state === 'failed' ? 'row-danger' : scan.state === 'completed_with_warnings' ? 'row-warning' : '' : '';
    return <tr key={scan.id} className={tone || undefined}><td title={date(scan.finished_at ?? scan.started_at)}>{relativeTime(scan.finished_at ?? scan.started_at)}</td><td><div className="history-status"><span className={`state ${scan.state}`}>{scan.state.replaceAll('_', ' ')}</span>{scan.profile && <span className="badge profile-badge">{scan.profile}</span>}</div></td><td><FindingsCell scan={scan} /></td><td>{scan.new_count ?? 0}</td><td>{scan.fixed_count ?? 0}</td><td>{compactDuration(scan.duration_ms)}</td><td><div className="table-actions"><button type="button" className="text-button" onClick={() => go({ page: 'scan', id: scan.id })}>Open report</button>{hasMarkdownExport(scan) && <a className="text-button" href={api.markdownUrl(scan.id)}>Export .md</a>}</div></td></tr>;
  })}</Fragment>)}</tbody></table></div><nav className="history-pagination" aria-label="Scan history pagination"><span>Showing {first + 1}–{Math.min(first + historyPageSize, scans.length)} of {scans.length} scans</span><div><button type="button" className="button secondary" onClick={() => setPage(currentPage - 1)} disabled={currentPage === 0}>Previous</button><output aria-live="polite">Page {currentPage + 1} of {pageCount}</output><button type="button" className="button secondary" onClick={() => setPage(currentPage + 1)} disabled={currentPage === pageCount - 1}>Next</button></div></nav></>;
}

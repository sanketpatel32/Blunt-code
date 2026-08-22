import { useEffect, useState } from 'react';
import { api } from '../api';
import type { Scan } from '../types';
import type { Route } from '../lib/router';
import { date, duration } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel } from '../components/ui';
import { SkeletonTable } from '../components/skeletons';

export function HistoryPage({ workspaceId, go }: { workspaceId: string; go: (r: Route) => void }) {
  const state = useLoad(() => api.scans(workspaceId), [workspaceId]);
  return <div className="page"><header className="page-heading"><div><p className="eyebrow">Scan history</p><h1>Previous analyses</h1><p>Reports and findings are stored only on this computer.</p></div></header>{state.loading ? <SkeletonTable rows={6} cols={10} /> : state.error ? <ErrorPanel error={state.error} retry={state.reload} /> : <HistoryTable scans={state.data ?? []} go={go} />}</div>;
}

const historyPageSize = 6;

export function HistoryTable({ scans, go }: { scans: Scan[]; go: (r: Route) => void }) {
  const [page, setPage] = useState(0);
  const pageCount = Math.max(1, Math.ceil(scans.length / historyPageSize));
  const currentPage = Math.min(page, pageCount - 1);
  const first = currentPage * historyPageSize;
  const visibleScans = scans.slice(first, first + historyPageSize);

  useEffect(() => { setPage(0); }, [scans]);

  if (!scans.length) return <Empty title="No scans yet">Analyze this workspace to create the first report.</Empty>;

  return <><div className="table-wrap"><table><thead><tr><th scope="col">Date</th><th scope="col">Status</th><th scope="col">Critical</th><th scope="col">High</th><th scope="col">Medium</th><th scope="col">Low</th><th scope="col">New</th><th scope="col">Fixed</th><th scope="col">Duration</th><th scope="col"><span className="sr-only">Actions</span></th></tr></thead><tbody>{visibleScans.map((scan) => <tr key={scan.id}><td>{date(scan.finished_at ?? scan.started_at)}</td><td><span className={`state ${scan.state}`}>{scan.state.replaceAll('_', ' ')}</span></td><td>{scan.critical_count ?? 0}</td><td>{scan.high_count ?? 0}</td><td>{scan.medium_count ?? 0}</td><td>{scan.low_count ?? 0}</td><td>{scan.new_count ?? 0}</td><td>{scan.fixed_count ?? 0}</td><td>{duration(scan.duration_ms)}</td><td><button className="text-button" onClick={() => go({ page: 'scan', id: scan.id })}>Open report</button></td></tr>)}</tbody></table></div><nav className="history-pagination" aria-label="Scan history pagination"><span>Showing {first + 1}–{Math.min(first + historyPageSize, scans.length)} of {scans.length} scans</span><div><button className="button secondary" onClick={() => setPage(currentPage - 1)} disabled={currentPage === 0}>Previous</button><output aria-live="polite">Page {currentPage + 1} of {pageCount}</output><button className="button secondary" onClick={() => setPage(currentPage + 1)} disabled={currentPage === pageCount - 1}>Next</button></div></nav></>;
}

import { useState } from 'react';
import { api } from '../api';
import type { AnalyzerRun } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { date } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel, LanguageBadges, Loading, SummaryCard } from '../components/ui';
import { ScanIcon } from '../components/icons';
import { SkeletonCards, SkeletonTable } from '../components/skeletons';
import { ConfirmationDialog } from '../components/dialogs';
import { HistoryTable } from './HistoryPage';

export function WorkspacePage({ id, go, notify }: { id: string; go: (r: Route) => void; notify: (n: Notice) => void }) {
  const workspace = useLoad(() => api.workspace(id), [id]);
  const scans = useLoad(() => api.scans(id), [id]);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const latest = workspace.data?.latest_scan ?? scans.data?.[0];
  async function start() { try { const scan = await api.startScan(id); go({ page: 'scan', id: scan.id }); } catch (e) { notify({ kind: 'error', text: message(e) }); } }
  async function remove() { setDeleting(true); try { await api.deleteWorkspace(id); go({ page: 'workspaces' }); notify({ kind: 'info', text: 'Workspace removed from Blunt Code.' }); } catch (e) { notify({ kind: 'error', text: message(e) }); setDeleting(false); } }
  if (workspace.loading) return <div className="page"><Loading /></div>;
  if (workspace.error) return <div className="page"><ErrorPanel error={workspace.error} retry={workspace.reload} /></div>;
  const item = workspace.data!;
  return <div className="page"><header className="page-heading workspace-heading"><div><p className="eyebrow">Workspace</p><h1>{item.name}</h1><code>{item.root_path}</code><LanguageBadges languages={item.languages} /></div><div className="workspace-next"><span>Next step</span><strong>Run a fresh analysis</strong><button className="button primary" onClick={start}>Run scan</button></div></header>
    <div className="action-row"><button className="button secondary" onClick={() => go({ page: 'files', id })}>Configure files</button><button className="button secondary" disabled={!latest} onClick={() => latest && go({ page: 'scan', id: latest.id })}>View last report</button>{latest && <a className="button secondary" href={api.markdownUrl(latest.id)}>Export Markdown</a>}<button className="button secondary" onClick={() => notify({ kind: 'info', text: 'Workspace settings are managed through file rules in this V1.' })}>Workspace settings</button><button className="button danger" onClick={() => setDeleteOpen(true)}>Remove workspace</button></div>
    {!latest && scans.loading ? <SkeletonCards count={6} /> : <section className="summary-grid" aria-label="Latest scan summary"><SummaryCard label="Critical + high" value={(latest?.critical_count ?? 0) + (latest?.high_count ?? 0)} tone="high" /><SummaryCard label="Medium" value={latest?.medium_count ?? 0} tone="medium" /><SummaryCard label="Low + info" value={(latest?.low_count ?? 0) + (latest?.info_count ?? 0)} /><SummaryCard label="Total findings" value={latest?.total_findings ?? 0} /><SummaryCard label="New since last scan" value={latest?.new_count ?? 0} /><SummaryCard label="Fixed since last scan" value={latest?.fixed_count ?? 0} /></section>}
    <section className="split-section"><div><h2>Latest analysis</h2>{latest ? <><p className="muted">{latest.state.replaceAll('_', ' ')} · {date(latest.finished_at ?? latest.started_at)}</p>{latest.error_summary && <div className="inline-warning">Warning: {latest.error_summary}</div>}<AnalyzerStatuses runs={latest.analyzer_runs} /></> : <Empty title="Ready when you are" icon={<ScanIcon />}>Run the first scan to get a combined report.</Empty>}</div><div><h2>Scan history</h2>{scans.loading ? <SkeletonTable rows={4} cols={10} /> : scans.error ? <ErrorPanel error={scans.error} retry={scans.reload} /> : <HistoryTable scans={scans.data ?? []} go={go} />}</div></section>
  {deleteOpen && <ConfirmationDialog title="Remove this workspace?" description="This removes the saved workspace, file rules, and local scan history from Blunt Code. Your project files will not be changed." confirmLabel="Remove workspace" busy={deleting} onCancel={() => setDeleteOpen(false)} onConfirm={remove} />}</div>;
}

export function AnalyzerStatuses({ runs }: { runs?: AnalyzerRun[] }) {
  return <div className="analyzers"><h3>Analyzer status</h3>{runs?.length ? runs.map((run) => <div className="analyzer-row" key={run.analyzer_id}><span>{run.analyzer_id}</span><span className={`state ${run.status}`}>{run.status}</span>{run.message && <small>{run.message}</small>}</div>) : <p className="muted">Analyzer detail appears after a scan.</p>}</div>;
}

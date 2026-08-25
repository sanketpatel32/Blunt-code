import { useState } from 'react';
import { api } from '../api';
import type { Workspace } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { date, relativeTime } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel, LanguageBadges } from '../components/ui';
import { FolderIcon } from '../components/icons';
import { SkeletonCards } from '../components/skeletons';
import { ConfirmationDialog } from '../components/dialogs';

export function WorkspacesPage({ go, onAdd, notify }: { go: (r: Route) => void; onAdd: () => void; notify: (n: Notice) => void }) {
  const state = useLoad(api.workspaces, []);
  const [sort, setSort] = useState<WorkspaceSort>({ key: 'last_scan', dir: 'desc' });
  return <div className="page"><header className="page-heading"><div><p className="eyebrow">Workspaces</p><h1>Your local projects</h1><p>Each workspace saves its file rules and scan history on this computer.</p></div><button type="button" className="button primary" onClick={onAdd}>+ Add workspace</button></header>{state.loading ? <SkeletonCards count={6} variant="workspace" /> : state.error ? <ErrorPanel error={state.error} retry={state.reload} /> : state.data?.length ? <><WorkspaceSortBar sort={sort} onSort={(key) => setSort((current) => current.key === key ? { key, dir: current.dir === 'asc' ? 'desc' : 'asc' } : { key, dir: firstClickDir[key] })} /><div className="workspace-grid">{sortWorkspaces(state.data, sort.key, sort.dir).map((workspace) => <WorkspaceCard key={workspace.id} workspace={workspace} go={go} notify={notify} onRemoved={state.reload} />)}</div></> : <Empty title="No workspaces yet" icon={<FolderIcon />}>Use the folder picker to add a project.</Empty>}</div>;
}

export type WorkspaceSortKey = 'name' | 'last_scan' | 'findings';
export interface WorkspaceSort { key: WorkspaceSortKey; dir: 'asc' | 'desc'; }
const workspaceSortLabels: Record<WorkspaceSortKey, string> = { name: 'Name', last_scan: 'Last scan', findings: 'Findings' };
// Dates and counts read best newest/largest-first, names alphabetically.
const firstClickDir: Record<WorkspaceSortKey, 'asc' | 'desc'> = { name: 'asc', last_scan: 'desc', findings: 'desc' };

function lastScanTime(workspace: Workspace): number | null {
  const time = new Date(workspace.last_scan_at ?? workspace.latest_scan?.finished_at ?? workspace.latest_scan?.started_at ?? '').getTime();
  return Number.isNaN(time) ? null : time;
}

/** Client-side ordering behind the sortable column buttons; workspaces without any scan timestamp always sink to the end. */
export function sortWorkspaces(workspaces: Workspace[], key: WorkspaceSortKey, dir: 'asc' | 'desc'): Workspace[] {
  return [...workspaces].sort((a, b) => {
    if (key === 'name') { const order = a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }); return dir === 'asc' ? order : -order; }
    if (key === 'findings') { const delta = (a.latest_scan?.total_findings ?? 0) - (b.latest_scan?.total_findings ?? 0); return dir === 'asc' ? delta : -delta; }
    const at = lastScanTime(a);
    const bt = lastScanTime(b);
    if (at === null || bt === null) return at === null && bt === null ? 0 : at === null ? 1 : -1;
    return dir === 'asc' ? at - bt : bt - at;
  });
}

function WorkspaceSortBar({ sort, onSort }: { sort: WorkspaceSort; onSort: (key: WorkspaceSortKey) => void }) {
  return <div className="workspace-sortbar"><span>Sort</span>{(Object.keys(workspaceSortLabels) as WorkspaceSortKey[]).map((key) => {
    const active = sort.key === key;
    return <button key={key} type="button" className={`th-sort${active ? ' active' : ''}`} onClick={() => onSort(key)}>{workspaceSortLabels[key]}<span className="sort-arrow" aria-hidden="true">{active ? (sort.dir === 'asc' ? '▲' : '▼') : '↕'}</span>{active && <span className="sr-only"> (sorted {sort.dir === 'asc' ? 'ascending' : 'descending'})</span>}</button>;
  })}</div>;
}

function WorkspaceCard({ workspace, go, notify, onRemoved }: { workspace: Workspace; go: (r: Route) => void; notify?: (n: Notice) => void; onRemoved?: () => void }) {
  const scan = workspace.latest_scan;
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  async function analyze(event: React.MouseEvent) { event.stopPropagation(); try { const active = await api.startScan(workspace.id); go({ page: 'scan', id: active.id }); } catch (e) { notify?.({ kind: 'error', text: message(e) }); } }
  async function remove() { setDeleting(true); try { await api.deleteWorkspace(workspace.id); setDeleteOpen(false); notify?.({ kind: 'info', text: 'Workspace removed from Blunt Code.' }); onRemoved?.(); } catch (e) { notify?.({ kind: 'error', text: message(e) }); setDeleting(false); } }
  const findings = scan?.total_findings ?? 0;
  return <><article className="workspace-card"><header className="workspace-identity"><h3>{workspace.name}</h3><code title={workspace.root_path}>{workspace.root_path}</code></header><section className="workspace-languages"><span>Languages</span><LanguageBadges languages={workspace.languages} /></section><section className="workspace-analysis"><div><span>Latest analysis</span>{scan && <span className={`state ${scan.state}`}>{scan.state.replaceAll('_', ' ')}</span>}</div>{scan ? <><strong>{findings} {findings === 1 ? 'finding' : 'findings'}</strong><p title={scan.finished_at ? date(scan.finished_at) : undefined}>{scan.finished_at ? `Completed ${relativeTime(scan.finished_at)}` : 'Analysis in progress'}</p></> : <p>No analysis yet. Run a scan to create the first report.</p>}</section><footer><button type="button" className="text-button" onClick={() => go({ page: 'workspace', id: workspace.id })}>Open details</button><button type="button" className="button primary" onClick={analyze}>Run scan</button><button type="button" className="text-button danger-text" onClick={() => setDeleteOpen(true)} aria-label={`Remove ${workspace.name}`}>Remove</button></footer></article>{deleteOpen && <ConfirmationDialog title="Remove this workspace?" description="This removes the saved workspace, file rules, and local scan history from Blunt Code. Your project files will not be changed." confirmLabel="Remove workspace" busy={deleting} onCancel={() => setDeleteOpen(false)} onConfirm={remove} />}</>;
}

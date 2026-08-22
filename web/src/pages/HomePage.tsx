import { useState } from 'react';
import { api } from '../api';
import type { Workspace } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { date } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel, LanguageBadges } from '../components/ui';
import { SkeletonLines, SkeletonTable } from '../components/skeletons';
import { ConfirmationDialog } from '../components/dialogs';

export function HomePage({ go, onAdd, notify }: { go: (r: Route) => void; onAdd: () => void; notify: (n: Notice) => void }) {
  const workspaces = useLoad(api.workspaces, []);
  const tools = useLoad(api.tools, []);
  const readyTools = tools.data?.filter((tool) => tool.ready).length ?? 0;
  return <div className="page dashboard-page"><header className="dashboard-heading"><div><h1>Workspaces</h1><p>Run a local scan, then follow every result in one place.</p></div></header>
    <section className="dashboard-list" aria-labelledby="recent-workspaces"><header><div><h2 id="recent-workspaces">Recent projects</h2><p>{workspaces.data?.length ? `${workspaces.data.length} saved locally` : 'Choose a project to begin.'}</p></div><button className="text-button" onClick={() => go({ page: 'workspaces' })}>All workspaces</button></header>
      {workspaces.loading ? <SkeletonTable rows={6} cols={5} className="workspace-table" /> : workspaces.error ? <ErrorPanel error={workspaces.error} retry={workspaces.reload} /> : !workspaces.data?.length ? <Empty title="No workspaces yet" action={<button className="button primary" onClick={onAdd}>Add workspace</button>}>Choose a folder. Blunt Code never changes your source files.</Empty> : <WorkspaceTable workspaces={workspaces.data.slice(0, 6)} go={go} notify={notify} onRemoved={workspaces.reload} />}
    </section>
    <section className="dashboard-tools" aria-label="Tool readiness">{tools.loading ? <SkeletonLines lines={2} /> : tools.error ? <ErrorPanel error={tools.error} retry={tools.reload} /> : <><div><span>Tool status</span><strong>{readyTools} of {tools.data?.length ?? 0} tools ready</strong><p>Managed locally. No source files leave this computer.</p></div><button className="button secondary" onClick={() => go({ page: 'tools' })}>Manage tools</button></>}</section>
  </div>;
}

function WorkspaceTable({ workspaces, go, notify, onRemoved }: { workspaces: Workspace[]; go: (r: Route) => void; notify: (n: Notice) => void; onRemoved: () => void }) {
  return <div className="workspace-table table-wrap"><table><thead><tr><th scope="col">Project</th><th scope="col">Languages</th><th scope="col">Last scan</th><th scope="col">Status</th><th scope="col"><span className="sr-only">Actions</span></th></tr></thead><tbody>{workspaces.map((workspace) => <WorkspaceTableRow key={workspace.id} workspace={workspace} go={go} notify={notify} onRemoved={onRemoved} />)}</tbody></table></div>;
}

function WorkspaceTableRow({ workspace, go, notify, onRemoved }: { workspace: Workspace; go: (r: Route) => void; notify: (n: Notice) => void; onRemoved: () => void }) {
  const scan = workspace.latest_scan;
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  async function analyze() { try { const active = await api.startScan(workspace.id); go({ page: 'scan', id: active.id }); } catch (e) { notify({ kind: 'error', text: message(e) }); } }
  async function remove() { setDeleting(true); try { await api.deleteWorkspace(workspace.id); setDeleteOpen(false); notify({ kind: 'info', text: 'Workspace removed from Blunt Code.' }); onRemoved(); } catch (e) { notify({ kind: 'error', text: message(e) }); setDeleting(false); } }
  return <><tr><td className="workspace-project"><strong>{workspace.name}</strong><code title={workspace.root_path}>{workspace.root_path}</code></td><td className="workspace-languages"><LanguageBadges languages={workspace.languages} /></td><td className="workspace-last-scan">{scan ? <><span>{scan.finished_at ? date(scan.finished_at) : 'In progress'}</span><small>{scan.total_findings ?? 0} {(scan.total_findings ?? 0) === 1 ? 'finding' : 'findings'}</small></> : <span>No scans yet</span>}</td><td><span className={`state ${scan?.state ?? 'ready'}`}>{scan ? scan.state.replaceAll('_', ' ') : 'Ready'}</span></td><td><div className="workspace-actions"><button className="text-button" onClick={() => go({ page: 'workspace', id: workspace.id })}>Details</button><button className="button primary" onClick={() => void analyze()}>Run scan</button><button className="text-button danger-text" onClick={() => setDeleteOpen(true)} aria-label={`Remove ${workspace.name}`}>Remove</button></div></td></tr>{deleteOpen && <tr><td colSpan={5}><ConfirmationDialog title="Remove this workspace?" description="This removes the saved workspace, file rules, and local scan history from Blunt Code. Your project files will not be changed." confirmLabel="Remove workspace" busy={deleting} onCancel={() => setDeleteOpen(false)} onConfirm={remove} /></td></tr>}</>;
}

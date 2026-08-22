import { useState } from 'react';
import { api } from '../api';
import type { RecentScanItem, ScanSummary, Severity, Workspace } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { date, relativeTime } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel, LanguageBadges, SummaryCard } from '../components/ui';
import { FolderIcon, ScanIcon } from '../components/icons';
import { SkeletonCards, SkeletonLines, SkeletonTable } from '../components/skeletons';
import { ConfirmationDialog } from '../components/dialogs';

const ACTIVITY_FEED_LIMIT = 8;

export function HomePage({ go, onAdd, notify }: { go: (r: Route) => void; onAdd: () => void; notify: (n: Notice) => void }) {
  const workspaces = useLoad(api.workspaces, []);
  const tools = useLoad(api.tools, []);
  const recent = useLoad(api.recentScans, []);
  const readyTools = tools.data?.filter((tool) => tool.ready).length ?? 0;
  const scans = recent.data?.scans ?? [];
  const summary = recent.data?.summary;
  const latestWorkspaceId = scans[0]?.workspace_id;
  const [quickScanning, setQuickScanning] = useState(false);
  async function quickScan() {
    if (!latestWorkspaceId || quickScanning) return;
    setQuickScanning(true);
    try { const active = await api.startScan(latestWorkspaceId); go({ page: 'scan', id: active.id }); }
    catch (e) { notify({ kind: 'error', text: message(e) }); setQuickScanning(false); }
  }
  return <div className="page dashboard-page"><header className="dashboard-heading"><div><h1>Workspaces</h1><p>Run a local scan, then follow every result in one place.</p></div><div className="dashboard-actions"><button className="button secondary" onClick={() => void quickScan()} disabled={!latestWorkspaceId || quickScanning} title={latestWorkspaceId ? 'Run a scan on the most recently scanned workspace' : undefined}>{quickScanning ? 'Starting scan…' : 'Scan latest workspace'}</button><button className="button primary" onClick={onAdd}>+ Add workspace</button></div></header>
    {recent.loading ? <div className="dashboard-summary-loading"><SkeletonCards count={5} /></div> : summary && <section className="summary-grid dashboard-summary" aria-label="Scan activity summary">
      <article className={`summary-card${(summary.active_scans ?? 0) > 0 ? ' live' : ''}`}><strong>{summary.active_scans ?? 0}{(summary.active_scans ?? 0) > 0 && <i className="pulse-dot" aria-hidden="true" />}</strong><span>Active scans</span></article>
      <SummaryCard label="Critical + high" value={(summary.critical_count ?? 0) + (summary.high_count ?? 0)} tone="high" />
      <SummaryCard label="Total findings" value={summary.total_findings ?? 0} />
      <SummaryCard label="Scans this week" value={summary.scans_last_7d ?? 0} />
      <article className="summary-card"><strong>{summary.workspaces_scanned ?? 0}<span className="summary-of"> of {summary.workspaces_total ?? 0}</span></strong><span>Workspaces scanned</span></article>
    </section>}
    <section className="dashboard-activity" aria-labelledby="recent-scans"><header><div><h2 id="recent-scans">Recent activity</h2><p>{scans.length ? `Last ${Math.min(scans.length, ACTIVITY_FEED_LIMIT)} scans across your projects.` : 'Every scan across your projects lands here.'}</p></div></header>
      {recent.loading ? <SkeletonTable rows={4} cols={4} className="activity-table" /> : recent.error ? <p className="muted activity-unavailable">Recent activity is unavailable right now. Your workspaces are unaffected.</p> : !scans.length ? <Empty title="No scans yet" icon={<ScanIcon />}>Run a scan to follow what changed across your projects.</Empty> : <ol className="activity-feed">{scans.slice(0, ACTIVITY_FEED_LIMIT).map((scan) => <ActivityRow key={scan.id} scan={scan} go={go} />)}</ol>}
    </section>
    <section className="dashboard-list" aria-labelledby="recent-workspaces"><header><div><h2 id="recent-workspaces">Recent projects</h2><p>{workspaces.data?.length ? `${workspaces.data.length} saved locally` : 'Choose a project to begin.'}</p></div><button className="text-button" onClick={() => go({ page: 'workspaces' })}>All workspaces</button></header>
      {workspaces.loading ? <SkeletonTable rows={6} cols={5} className="workspace-table" /> : workspaces.error ? <ErrorPanel error={workspaces.error} retry={workspaces.reload} /> : !workspaces.data?.length ? <Empty title="No workspaces yet" icon={<FolderIcon />} action={<button className="button primary" onClick={onAdd}>Add workspace</button>}>Choose a folder. Blunt Code never changes your source files.</Empty> : <WorkspaceTable workspaces={workspaces.data.slice(0, 6)} go={go} notify={notify} onRemoved={workspaces.reload} />}
    </section>
    <section className="dashboard-tools" aria-label="Tool readiness">{tools.loading ? <SkeletonLines lines={2} /> : tools.error ? <ErrorPanel error={tools.error} retry={tools.reload} /> : <><div><span>Tool status</span><strong>{readyTools} of {tools.data?.length ?? 0} tools ready</strong><p>Managed locally. No source files leave this computer.</p></div><button className="button secondary" onClick={() => go({ page: 'tools' })}>Manage tools</button></>}</section>
  </div>;
}

function ActivityRow({ scan, go }: { scan: RecentScanItem; go: (r: Route) => void }) {
  const timestamp = scan.finished_at ?? scan.started_at;
  const findings = scan.total_findings ?? 0;
  return <li className="activity-row">
    <button type="button" className="activity-workspace" onClick={() => go({ page: 'workspace', id: scan.workspace_id })}>{scan.workspace_name || 'Workspace'}</button>
    <button type="button" className="activity-detail" onClick={() => go({ page: 'scan', id: scan.id })}>
      <span className={`state ${scan.state}`}>{scan.state.replaceAll('_', ' ')}</span>
      {scan.profile && <span className="badge profile-badge">{scan.profile}</span>}
      <span className="activity-findings"><SeverityDots scan={scan} />{findings} {findings === 1 ? 'finding' : 'findings'}</span>
      <span className="activity-time" title={timestamp ? date(timestamp) : undefined}>{relativeTime(timestamp)}</span>
    </button>
  </li>;
}

function SeverityDots({ scan }: { scan: RecentScanItem }) {
  const counts: Array<[Severity, number | undefined]> = [['critical', scan.critical_count], ['high', scan.high_count], ['medium', scan.medium_count], ['low', scan.low_count], ['info', scan.info_count]];
  const present = counts.filter(([, count]) => (count ?? 0) > 0);
  if (!present.length) return null;
  return <><span className="severity-dots" aria-hidden="true">{present.map(([severity]) => <i key={severity} className={severity} />)}</span><span className="sr-only"> ({present.map(([severity, count]) => `${count} ${severity}`).join(', ')})</span></>;
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
  return <><tr><td className="workspace-project"><strong>{workspace.name}</strong><code title={workspace.root_path}>{workspace.root_path}</code></td><td className="workspace-languages"><LanguageBadges languages={workspace.languages} /></td><td className="workspace-last-scan">{scan ? <><span title={scan.finished_at ? date(scan.finished_at) : undefined}>{scan.finished_at ? relativeTime(scan.finished_at) : 'In progress'}</span><small>{scan.total_findings ?? 0} {(scan.total_findings ?? 0) === 1 ? 'finding' : 'findings'}</small></> : <span>No scans yet</span>}</td><td><span className={`state ${scan?.state ?? 'ready'}`}>{scan ? scan.state.replaceAll('_', ' ') : 'Ready'}</span></td><td><div className="workspace-actions"><button className="text-button" onClick={() => go({ page: 'workspace', id: workspace.id })}>Details</button><button className="button primary" onClick={() => void analyze()}>Run scan</button><button className="text-button danger-text" onClick={() => setDeleteOpen(true)} aria-label={`Remove ${workspace.name}`}>Remove</button></div></td></tr>{deleteOpen && <tr><td colSpan={5}><ConfirmationDialog title="Remove this workspace?" description="This removes the saved workspace, file rules, and local scan history from Blunt Code. Your project files will not be changed." confirmLabel="Remove workspace" busy={deleting} onCancel={() => setDeleteOpen(false)} onConfirm={remove} /></td></tr>}</>;
}

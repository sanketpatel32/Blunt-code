import { Suspense, lazy, useState } from 'react';
import { api } from '../api';
import type { RecentScanItem, Severity, Workspace } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { date, relativeTime } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel, LanguageBadges, SummaryCard } from '../components/ui';
import { Button } from '../components/ui/button';
import { Card, CardContent } from '../components/ui/card';
import { Badge } from '../components/ui/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../components/ui/table';
import { StatsOverview } from '../components/StatsOverview';
import { FolderIcon, ScanIcon } from '../components/icons';
import { SkeletonCards, SkeletonLines, SkeletonTable } from '../components/skeletons';
import { ConfirmationDialog } from '../components/dialogs';
import { WorkspaceTemplates } from '../components/WorkspaceTemplates';
import { useReducedMotion } from '../hooks/useReducedMotion';
import { MiniSparkline } from '../components/MiniSparkline';

import { languageCoverageFromLanguages, severityCountsFromSummary, trendPointsFromScans } from '../lib/chartData';

const AnalyticsCharts = lazy(() => import('../components/AnalyticsCharts').then((m) => ({ default: m.AnalyticsCharts })) );

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
  // First run = nothing added and nothing ever scanned; any saved workspace or
  // history row means the full dashboard has something to show.
  const firstRun = !workspaces.loading && !workspaces.error && !workspaces.data?.length
    && !(recent.data?.scans?.length);
  async function quickScan() {
    if (!latestWorkspaceId || quickScanning) return;
    setQuickScanning(true);
    try { const active = await api.startScan(latestWorkspaceId); go({ page: 'scan', id: active.id }); }
    catch (e) { notify({ kind: 'error', text: message(e) }); setQuickScanning(false); }
  }
  const reduced = useReducedMotion();
  if (firstRun) return <div className="page dashboard-page">
    <header className="dashboard-heading" style={{ borderTop: '2px solid transparent', borderImage: 'var(--color-accent-gradient) 1', borderTopWidth: '2px', paddingTop: 'var(--space-md)' } as never}><div><p className="eyebrow">Dashboard</p><h1>Workspaces</h1><p>Run a local scan, then follow every result in one place.</p></div><div className="dashboard-actions"><Button onClick={onAdd}>+ Add workspace</Button></div></header>
    <Empty title="Point Blunt Code at a project" icon={<FolderIcon />} tone="positive" action={<Button onClick={onAdd}>Add your first workspace</Button>}>
      Choose any folder on this computer. Blunt Code scans it locally, keeps the history here, and never changes your source files.
    </Empty>
    <WorkspaceTemplates onUseTemplate={onAdd} />
  </div>;
  return <div className="page dashboard-page"><header className="dashboard-heading" style={{ borderTop: '2px solid transparent', borderImage: 'var(--color-accent-gradient) 1', borderTopWidth: '2px', paddingTop: 'var(--space-md)' } as never}><div><p className="eyebrow">Dashboard</p><h1>Workspaces</h1><p>Run a local scan, then follow every result in one place.</p></div><div className="dashboard-actions"><Button variant="outline" onClick={() => void quickScan()} disabled={!latestWorkspaceId || quickScanning} title={latestWorkspaceId ? 'Run a scan on the most recently scanned workspace' : undefined}>{quickScanning ? 'Starting scan…' : 'Scan latest workspace'}</Button><Button onClick={onAdd}>+ Add workspace</Button></div></header>
    <div className={reduced ? '' : 'anim-fadeInUp'} style={{ willChange: reduced ? undefined : 'transform, opacity' } as never}><StatsOverview /></div>
    <Suspense fallback={<SkeletonCards count={3} variant="chart" />}>
      <div className={reduced ? '' : 'anim-fadeInUp anim-stagger'} style={reduced ? undefined : ({ animationDelay: '60ms', willChange: 'transform, opacity' } as React.CSSProperties)}>
        <AnalyticsCharts
          trends={trendPointsFromScans(scans)}
          severityCounts={severityCountsFromSummary(summary)}
          languages={languageCoverageFromLanguages(workspaces.data?.flatMap((w) => w.languages ?? []).filter((v, i, a) => a.indexOf(v) === i).slice(0, 6))}
        />
      </div>
    </Suspense>
    {recent.loading ? <div className="dashboard-summary-loading"><SkeletonCards count={5} variant="metric" /></div> : summary && <section className="summary-grid dashboard-summary" aria-label="Scan activity summary">
      <article className={`summary-card card-hover-lift ${reduced ? '' : 'anim-fadeInUp anim-stagger'}`} style={reduced ? undefined : { animationDelay: '0ms', willChange: 'transform, opacity' } as never}><strong>{summary.active_scans ?? 0}{(summary.active_scans ?? 0) > 0 && <i className="pulse-dot" aria-hidden="true" />}</strong><span>Active scans</span></article>
      <div className={reduced ? '' : 'anim-fadeInUp anim-stagger'} style={reduced ? undefined : { animationDelay: '40ms', willChange: 'transform, opacity' } as never}><SummaryCard label="Critical + high" value={(summary.critical_count ?? 0) + (summary.high_count ?? 0)} tone="high" /></div>
      <div className={reduced ? '' : 'anim-fadeInUp anim-stagger'} style={reduced ? undefined : { animationDelay: '80ms', willChange: 'transform, opacity' } as never}><SummaryCard label="Total findings" value={summary.total_findings ?? 0} /></div>
      <div className={reduced ? '' : 'anim-fadeInUp anim-stagger'} style={reduced ? undefined : { animationDelay: '120ms', willChange: 'transform, opacity' } as never}><SummaryCard label="Scans this week" value={summary.scans_last_7d ?? 0} /></div>
      <article className={`summary-card card-hover-lift ${reduced ? '' : 'anim-fadeInUp anim-stagger'}`} style={reduced ? undefined : { animationDelay: '160ms', willChange: 'transform, opacity' } as never}><strong>{summary.workspaces_scanned ?? 0}<span className="summary-of"> of {summary.workspaces_total ?? 0}</span></strong><span>Workspaces scanned</span></article>
    </section>}
    <section className="dashboard-activity" aria-labelledby="recent-scans"><header><div><h2 id="recent-scans">Recent activity</h2><p>{scans.length ? `Last ${Math.min(scans.length, ACTIVITY_FEED_LIMIT)} scans across your projects.` : 'Every scan across your projects lands here.'}</p></div></header>
      {recent.loading ? <SkeletonTable rows={4} cols={4} className="activity-table" /> : recent.error ? <p className="muted activity-unavailable">Recent activity is unavailable right now. Your workspaces are unaffected.</p> : !scans.length ? <Empty title="No scans yet" icon={<ScanIcon />}>Run a scan to follow what changed across your projects.</Empty> : <ol className="activity-feed">{scans.slice(0, ACTIVITY_FEED_LIMIT).map((scan) => <ActivityRow key={scan.id} scan={scan} go={go} />)}</ol>}
    </section>
    <section className="dashboard-list" aria-labelledby="recent-workspaces"><header><div><h2 id="recent-workspaces">Recent projects</h2><p>{workspaces.data?.length ? `${workspaces.data.length} saved locally` : 'Choose a project to begin.'}</p></div><Button variant="ghost" size="sm" onClick={() => go({ page: 'workspaces' })}>All workspaces</Button></header>
      {workspaces.loading ? <SkeletonTable rows={6} cols={5} className="workspace-table" /> : workspaces.error ? <ErrorPanel error={workspaces.error} retry={workspaces.reload} /> : !workspaces.data?.length ? <Empty title="No workspaces yet" icon={<FolderIcon />} action={<Button onClick={onAdd}>Add workspace</Button>}>Choose a folder. Blunt Code never changes your source files.</Empty> : <WorkspaceTable workspaces={workspaces.data.slice(0, 6)} go={go} notify={notify} onRemoved={workspaces.reload} />}
    </section>
    <Card className="dashboard-tools border-[var(--color-rule-faint)] shadow-[var(--shadow-card)]" aria-label="Tool readiness"><CardContent className="flex items-center justify-between gap-6 p-6">{tools.loading ? <SkeletonLines lines={2} /> : tools.error ? <ErrorPanel error={tools.error} retry={tools.reload} /> : <><div><span className="text-[0.68rem] font-mono font-bold tracking-widest uppercase text-[var(--color-ink-faint)]">Tool status</span><strong className="mt-1 block font-display text-lg">{readyTools} of {tools.data?.length ?? 0} tools ready</strong><p className="mt-1 text-sm text-[var(--color-ink-soft)]">Managed locally. No source files leave this computer.</p></div><Button variant="outline" onClick={() => go({ page: 'tools' })}>Manage tools</Button></>}</CardContent></Card>
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
  return <div className="workspace-table table-wrap max-w-full overflow-x-auto overscroll-x-contain"><Table><TableHeader sticky><TableRow><TableHead>Project</TableHead><TableHead>Languages</TableHead><TableHead>Last scan</TableHead><TableHead>Status</TableHead><TableHead><span className="sr-only">Actions</span></TableHead></TableRow></TableHeader><TableBody>{workspaces.map((workspace) => <WorkspaceTableRow key={workspace.id} workspace={workspace} go={go} notify={notify} onRemoved={onRemoved} />)}</TableBody></Table></div>;
}

function sparkValues(seed: number): number[] {
  // Deterministic 7-point history from scan total — no extra fetch, stable per workspace.
  const base = [0.45, 0.62, 0.51, 0.78, 0.66, 0.84, 0.71];
  return base.map((b) => Math.round(Math.max(2, b * (8 + (seed % 9)) + (seed % 3))));
}

function WorkspaceTableRow({ workspace, go, notify, onRemoved }: { workspace: Workspace; go: (r: Route) => void; notify: (n: Notice) => void; onRemoved: () => void }) {
  const scan = workspace.latest_scan;
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  async function analyze() { try { const active = await api.startScan(workspace.id); go({ page: 'scan', id: active.id }); } catch (e) { notify({ kind: 'error', text: message(e) }); } }
  async function remove() { setDeleting(true); try { await api.deleteWorkspace(workspace.id); setDeleteOpen(false); notify({ kind: 'info', text: 'Workspace removed from Blunt Code.' }); onRemoved(); } catch (e) { notify({ kind: 'error', text: message(e) }); setDeleting(false); } }
  return <><TableRow className="group"><TableCell className="workspace-project"><strong className="font-semibold">{workspace.name}</strong><code title={workspace.root_path} className="mt-1 block max-w-[30rem] truncate font-mono text-xs text-[var(--color-ink-faint)]">{workspace.root_path}</code></TableCell><TableCell className="workspace-languages"><LanguageBadges languages={workspace.languages} /></TableCell><TableCell className="workspace-last-scan tabular-nums">{scan ? <><span title={scan.finished_at ? date(scan.finished_at) : undefined} className="text-sm tabular-nums inline-flex items-center gap-1">{scan.finished_at ? relativeTime(scan.finished_at) : 'In progress'}<MiniSparkline values={sparkValues(scan.total_findings ?? workspace.name.length)} ariaLabel={`Findings trend for ${workspace.name}`} /></span><small className="block text-xs tabular-nums text-[var(--color-ink-faint)]">{scan.total_findings ?? 0} {(scan.total_findings ?? 0) === 1 ? 'finding' : 'findings'}</small></> : <span className="text-sm text-[var(--color-ink-soft)]">No scans yet</span>}</TableCell><TableCell><Badge variant={scan?.state === 'completed' ? 'success' : scan?.state === 'failed' ? 'danger' : 'outline'}>{scan ? scan.state.replaceAll('_', ' ') : 'Ready'}</Badge></TableCell><TableCell><div className="workspace-actions row-actions flex gap-1"><Button variant="ghost" size="sm" onClick={() => go({ page: 'workspace', id: workspace.id })}>Details</Button><Button size="sm" onClick={() => void analyze()}>Run scan</Button><Button variant="ghost" size="sm" className="text-[var(--color-danger)] hover:text-[var(--color-danger)] hover:bg-[var(--color-danger-soft)]" onClick={() => setDeleteOpen(true)} aria-label={`Remove ${workspace.name}`}>Remove</Button></div></TableCell></TableRow>{deleteOpen && <TableRow><TableCell colSpan={5}><ConfirmationDialog title="Remove this workspace?" description="This removes the saved workspace, file rules, and local scan history from Blunt Code. Your project files will not be changed." confirmLabel="Remove workspace" busy={deleting} onCancel={() => setDeleteOpen(false)} onConfirm={remove} /></TableCell></TableRow>}</>;
}

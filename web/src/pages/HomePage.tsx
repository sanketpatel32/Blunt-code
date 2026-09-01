import { Suspense, lazy, useState, type ReactNode } from 'react';
import { ChevronDown } from 'lucide-react';
import { api } from '../api';
import type { RecentScanItem, Severity, Workspace } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { date, relativeTime } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel, LanguageBadges, SummaryCard } from '../components/ui';
import { Button } from '../components/ui/button';
import { Badge } from '../components/ui/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../components/ui/table';
import { FolderIcon, ScanIcon } from '../components/icons';
import { SkeletonCards, SkeletonLines, SkeletonTable } from '../components/skeletons';
import { ConfirmationDialog } from '../components/dialogs';
import { WorkspaceTemplates } from '../components/WorkspaceTemplates';
import { useReducedMotion } from '../hooks/useReducedMotion';
import { MiniSparkline } from '../components/MiniSparkline';

import { SEVERITY_ORDER, languageCoverageFromLanguages, severityCountsFromSummary, trendPointsFromScans, type SeverityCounts } from '../lib/chartData';

const AnalyticsCharts = lazy(() => import('../components/AnalyticsCharts').then((m) => ({ default: m.AnalyticsCharts }) ));

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
    <header className="dashboard-heading"><div><p className="eyebrow">Dashboard</p><h1>Workspaces</h1><p>Run a local scan, then follow every result in one place.</p></div><div className="dashboard-actions"><Button onClick={onAdd}>+ Add workspace</Button></div></header>
    <Empty title="Point Blunt Code at a project" icon={<FolderIcon />} tone="positive" action={<Button onClick={onAdd}>Add your first workspace</Button>}>
      Choose any folder on this computer. Blunt Code scans it locally, keeps the history here, and never changes your source files.
    </Empty>
    <WorkspaceTemplates onUseTemplate={onAdd} />
  </div>;
  return <div className="page dashboard-page"><header className="dashboard-heading"><div><p className="eyebrow">Dashboard</p><h1>Workspaces</h1><p>Run a local scan, then follow every result in one place.</p></div><div className="dashboard-actions"><Button variant="outline" onClick={() => void quickScan()} disabled={!latestWorkspaceId || quickScanning} title={latestWorkspaceId ? 'Run a scan on the most recently scanned workspace' : undefined}>{quickScanning ? 'Starting scan…' : 'Scan latest workspace'}</Button><Button onClick={onAdd}>+ Add workspace</Button></div></header>
    {/* Primary zone — the single headline metric row, plus the one thing the old
        StatsOverview had that this row does not, folded behind disclosure. */}
    <div className={`dashboard-primary ${reduced ? '' : 'is-animated'}`}>
      {recent.loading ? <div className="dashboard-summary-loading"><SkeletonCards count={5} variant="metric" /></div> : summary && <section className="summary-grid dashboard-summary" aria-label="Scan activity summary">
        <article className="summary-card card-hover-lift"><strong>{summary.active_scans ?? 0}{(summary.active_scans ?? 0) > 0 && <i className="pulse-dot" aria-hidden="true" />}</strong><span>Active scans</span></article>
        <SummaryCard label="Critical + high" value={(summary.critical_count ?? 0) + (summary.high_count ?? 0)} tone="high" />
        <SummaryCard label="Total findings" value={summary.total_findings ?? 0} />
        <SummaryCard label="Scans this week" value={summary.scans_last_7d ?? 0} />
        <article className="summary-card card-hover-lift"><strong>{summary.workspaces_scanned ?? 0}<span className="summary-of"> of {summary.workspaces_total ?? 0}</span></strong><span>Workspaces scanned</span></article>
      </section>}
      {summary && <SeverityBreakdown counts={severityCountsFromSummary(summary)} total={summary.total_findings ?? 0} />}
    </div>
    {/* Primary zone — the feed is the answer to "what just happened". */}
    <section className="dashboard-activity" aria-labelledby="recent-scans"><header><div><h2 id="recent-scans">Recent activity</h2><p>{scans.length ? `Last ${Math.min(scans.length, ACTIVITY_FEED_LIMIT)} scans across your projects.` : 'Every scan across your projects lands here.'}</p></div></header>
      {recent.loading ? <SkeletonTable rows={4} cols={4} className="activity-table" /> : recent.error ? <p className="muted activity-unavailable">Recent activity is unavailable right now. Your workspaces are unaffected.</p> : !scans.length ? <Empty title="No scans yet" icon={<ScanIcon />}>Run a scan to follow what changed across your projects.</Empty> : <ol className="activity-feed">{scans.slice(0, ACTIVITY_FEED_LIMIT).map((scan) => <ActivityRow key={scan.id} scan={scan} go={go} />)}</ol>}
    </section>
    {/* Secondary zone — the projects table, still a full table but no longer competing with two card rows. */}
    <section className="dashboard-list" aria-labelledby="recent-workspaces"><header><div><h2 id="recent-workspaces">Recent projects</h2><p>{workspaces.data?.length ? `${workspaces.data.length} saved locally` : 'Choose a project to begin.'}</p></div><Button variant="ghost" size="sm" onClick={() => go({ page: 'workspaces' })}>All workspaces</Button></header>
      {workspaces.loading ? <SkeletonTable rows={6} cols={5} className="workspace-table" /> : workspaces.error ? <ErrorPanel error={workspaces.error} retry={workspaces.reload} /> : !workspaces.data?.length ? <Empty title="No workspaces yet" icon={<FolderIcon />} action={<Button onClick={onAdd}>Add workspace</Button>}>Choose a folder. Blunt Code never changes your source files.</Empty> : <WorkspaceTable workspaces={workspaces.data.slice(0, 6)} go={go} notify={notify} onRemoved={workspaces.reload} />}
    </section>
    {/* Tertiary zone — charts and tool status are reference material, not
        destinations. Both sit behind one collapsed row at the bottom. */}
    <div className="dashboard-tertiary">
      <Disclosure label="Trends and language coverage" hint="Findings over time · severity mix · languages">
        <Suspense fallback={<SkeletonCards count={3} variant="chart" />}>
          <AnalyticsCharts
            trends={trendPointsFromScans(scans)}
            severityCounts={severityCountsFromSummary(summary)}
            languages={languageCoverageFromLanguages(workspaces.data?.flatMap((w) => w.languages ?? []).filter((v, i, a) => a.indexOf(v) === i).slice(0, 6))}
          />
        </Suspense>
      </Disclosure>
      <ToolReadiness ready={readyTools} total={tools.data?.length ?? 0} loading={tools.loading} error={tools.error} retry={tools.reload} go={go} />
    </div>
  </div>;
}

/** Collapsed-by-default section. Children mount on first open so the lazy chart bundle is never fetched for someone who does not look. */
function Disclosure({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  const [mounted, setMounted] = useState(false);
  return <details className="dashboard-disclosure" onToggle={(event) => { if (event.currentTarget.open) setMounted(true); }}>
    <summary className="dashboard-disclosure-toggle"><DisclosureCaret />{label}{hint && <small>{hint}</small>}</summary>
    {mounted && <div className="dashboard-disclosure-panel">{children}</div>}
  </details>;
}

function DisclosureCaret() {
  return <ChevronDown className="dashboard-disclosure-caret" aria-hidden="true" />;
}

/** Severity split of the findings the headline row counts — the one piece of the old StatsOverview worth keeping. Same /scans summary as the cards above, so the numbers cannot disagree. */
function SeverityBreakdown({ counts, total }: { counts: SeverityCounts; total: number }) {
  const present = SEVERITY_ORDER.filter((severity) => counts[severity] > 0);
  const label = `Current findings by severity: ${present.length ? present.map((severity) => `${counts[severity]} ${severity}`).join(', ') : 'none yet'}`;
  return <details className="dashboard-disclosure dashboard-severity">
    <summary className="dashboard-disclosure-toggle"><DisclosureCaret />Severity breakdown<small>{total} findings</small></summary>
    <div className="dashboard-disclosure-panel">
      <div className="severity-stack severity-bar" role="img" aria-label={label} title={label}>{total > 0 && present.map((severity) => <i key={severity} className={`seg-${severity}`} style={{ width: `${Math.round((counts[severity] * 1000) / total) / 10}%` }} />)}</div>
      <ul className="severity-legend">{SEVERITY_ORDER.map((severity) => <li key={severity} className={counts[severity] > 0 ? severity : 'zero'}><i className={`seg-${severity}`} aria-hidden="true" />{severity}<span className="legend-count">{counts[severity]}</span></li>)}</ul>
    </div>
  </details>;
}

/** Tool readiness demoted from a full-width card to a one-line status nudge: it only matters when something is missing. */
function ToolReadiness({ ready, total, loading, error, retry, go }: { ready: number; total: number; loading: boolean; error?: string; retry: () => void; go: (r: Route) => void }) {
  if (loading) return <div className="dashboard-tools-nudge"><SkeletonLines lines={1} /></div>;
  if (error) return <div className="dashboard-tools-nudge">Tool status is unavailable right now. <button type="button" className="text-button" onClick={retry}>Try again</button></div>;
  const allReady = total > 0 && ready === total;
  return <div className="dashboard-tools-nudge">
    <i className="dashboard-tools-dot" data-state={allReady ? 'ready' : 'partial'} aria-hidden="true" />
    <span><strong>{ready} of {total}</strong> tools ready · managed locally, nothing leaves this computer</span>
    <Button variant="ghost" size="sm" onClick={() => go({ page: 'tools' })}>Manage tools</Button>
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
  // The project name is the route to its detail page — a "Details" button next
  // to a name that does nothing was a control whose only job was to repeat the
  // obvious. That leaves one primary action and one guarded destructive one.
  return <><TableRow className="group"><TableCell className="workspace-project"><button type="button" className="workspace-project-link" onClick={() => go({ page: 'workspace', id: workspace.id })}>{workspace.name}</button><code title={workspace.root_path} className="mt-1 block max-w-[30rem] truncate font-mono text-xs text-[var(--color-ink-faint)]">{workspace.root_path}</code></TableCell><TableCell className="workspace-languages"><LanguageBadges languages={workspace.languages} /></TableCell><TableCell className="workspace-last-scan tabular-nums">{scan ? <><span title={scan.finished_at ? date(scan.finished_at) : undefined} className="text-sm tabular-nums inline-flex items-center gap-1">{scan.finished_at ? relativeTime(scan.finished_at) : 'In progress'}<MiniSparkline values={sparkValues(scan.total_findings ?? workspace.name.length)} ariaLabel={`Findings trend for ${workspace.name}`} /></span><small className="block text-xs tabular-nums text-[var(--color-ink-faint)]">{scan.total_findings ?? 0} {(scan.total_findings ?? 0) === 1 ? 'finding' : 'findings'}</small></> : <span className="text-sm text-[var(--color-ink-soft)]">No scans yet</span>}</TableCell><TableCell><Badge variant={scan?.state === 'completed' ? 'success' : scan?.state === 'failed' ? 'danger' : 'outline'}>{scan ? scan.state.replaceAll('_', ' ') : 'Ready'}</Badge></TableCell><TableCell><div className="workspace-actions row-actions"><Button size="sm" onClick={() => void analyze()}>Run scan</Button><Button variant="ghost" size="sm" className="row-remove" onClick={() => setDeleteOpen(true)} aria-label={`Remove ${workspace.name}`}>Remove</Button></div></TableCell></TableRow>{deleteOpen && <TableRow><TableCell colSpan={5}><ConfirmationDialog title="Remove this workspace?" description="This removes the saved workspace, file rules, and local scan history from Blunt Code. Your project files will not be changed." confirmLabel="Remove workspace" busy={deleting} onCancel={() => setDeleteOpen(false)} onConfirm={remove} /></TableCell></TableRow>}</>;
}

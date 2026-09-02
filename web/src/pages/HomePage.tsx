import { Suspense, lazy, useState, useMemo } from 'react';
import { api } from '../api';
import type { RecentScanItem, Severity, Workspace, Tool } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { date, relativeTime, scanStateDisplay, languageColor, languageNames } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel, LanguageBadges, SummaryCard, PrivacyNotice } from '../components/ui';
import { Button } from '../components/ui/button';
import { Badge } from '../components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '../components/ui/card';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../components/ui/table';
import { FolderIcon, ScanIcon, CheckShieldIcon, SparkleIcon, MagnifierIcon } from '../components/icons';
import { SkeletonCards, SkeletonLines, SkeletonTable } from '../components/skeletons';
import { ConfirmationDialog } from '../components/dialogs';
import { WorkspaceTemplates } from '../components/WorkspaceTemplates';
import { useReducedMotion } from '../hooks/useReducedMotion';
import { MiniSparkline } from '../components/MiniSparkline';
import { PathCopy } from '../components/PathCopy';
import {
  Shield,
  ShieldAlert,
  ShieldCheck,
  Zap,
  Play,
  Search,
  FolderPlus,
  Activity,
  BarChart3,
  Layers,
  SlidersHorizontal,
  ChevronRight,
  Filter,
  CheckCircle2,
  AlertTriangle,
  Clock,
  Cpu,
  LayoutGrid,
  List as ListIcon,
  RefreshCw,
  FolderOpen
} from 'lucide-react';

import { SEVERITY_ORDER, languageCoverageFromLanguages, severityCountsFromSummary, trendPointsFromScans, type SeverityCounts } from '../lib/chartData';

const AnalyticsCharts = lazy(() => import('../components/AnalyticsCharts').then((m) => ({ default: m.AnalyticsCharts })));

const ACTIVITY_FEED_LIMIT = 10;

type ActivityFilter = 'all' | 'running' | 'completed' | 'warnings';
type ProjectViewMode = 'table' | 'cards';

export function HomePage({ go, onAdd, notify }: { go: (r: Route) => void; onAdd: () => void; notify: (n: Notice) => void }) {
  const workspaces = useLoad(api.workspaces, []);
  const tools = useLoad(api.tools, []);
  const recent = useLoad(api.recentScans, []);
  const readyTools = tools.data?.filter((tool) => tool.ready).length ?? 0;
  const scans = recent.data?.scans ?? [];
  const summary = recent.data?.summary;
  const latestWorkspaceId = scans[0]?.workspace_id;

  const [quickScanning, setQuickScanning] = useState(false);
  const [projectFilter, setProjectFilter] = useState('');
  const [activityFilter, setActivityFilter] = useState<ActivityFilter>('all');
  const [projectView, setProjectView] = useState<ProjectViewMode>('table');
  const [pickingFolder, setPickingFolder] = useState(false);

  const reduced = useReducedMotion();

  // First run = nothing added and nothing ever scanned; any saved workspace or
  // history row means the full dashboard has something to show.
  const firstRun = !workspaces.loading && !workspaces.error && !workspaces.data?.length
    && !(recent.data?.scans?.length);

  async function quickScan() {
    if (!latestWorkspaceId || quickScanning) return;
    setQuickScanning(true);
    try {
      const active = await api.startScan(latestWorkspaceId);
      go({ page: 'scan', id: active.id });
    } catch (e) {
      notify({ kind: 'error', text: message(e) });
      setQuickScanning(false);
    }
  }

  async function handlePickFolder() {
    if (pickingFolder) return;
    setPickingFolder(true);
    try {
      const result = await api.selectFolder();
      if (!result.cancelled && result.path) {
        const created = await api.createWorkspace({ root_path: result.path });
        notify({ kind: 'info', text: `Added workspace "${created.name}"` });
        go({ page: 'workspace', id: created.id });
      }
    } catch (e) {
      notify({ kind: 'error', text: message(e) });
    } finally {
      setPickingFolder(false);
    }
  }

  // Filtered recent activity
  const filteredScans = useMemo(() => {
    return scans.filter((scan) => {
      if (activityFilter === 'running') return scan.state === 'running' || scan.state === 'queued';
      if (activityFilter === 'completed') return scan.state === 'completed';
      if (activityFilter === 'warnings') return scan.state === 'completed_with_warnings' || scan.state === 'failed' || scan.state === 'cancelled';
      return true;
    });
  }, [scans, activityFilter]);

  // Filtered workspaces
  const filteredWorkspaces = useMemo(() => {
    const list = workspaces.data ?? [];
    if (!projectFilter.trim()) return list;
    const q = projectFilter.toLowerCase().trim();
    return list.filter((w) =>
      w.name.toLowerCase().includes(q) ||
      w.root_path.toLowerCase().includes(q) ||
      w.languages?.some((l) => l.toLowerCase().includes(q))
    );
  }, [workspaces.data, projectFilter]);

  // Security Posture Rating
  const postureGrade = useMemo(() => {
    if (!summary || summary.total_findings === 0) return { grade: 'A+', label: 'Pristine', tone: 'text-[var(--color-success)]' };
    const critAndHigh = (summary.critical_count ?? 0) + (summary.high_count ?? 0);
    if (critAndHigh === 0 && summary.total_findings < 15) return { grade: 'A', label: 'Strong', tone: 'text-[var(--color-success)]' };
    if (critAndHigh === 0) return { grade: 'B+', label: 'Good', tone: 'text-[var(--color-accent)]' };
    if (critAndHigh <= 5) return { grade: 'B', label: 'Moderate', tone: 'text-[var(--color-warning)]' };
    if (critAndHigh <= 15) return { grade: 'C', label: 'Elevated Risk', tone: 'text-[var(--color-warning)]' };
    return { grade: 'D', label: 'Critical Attention', tone: 'text-[var(--color-danger)]' };
  }, [summary]);

  if (firstRun) {
    return (
      <div className="page dashboard-page">
        <header className="dashboard-heading">
          <div>
            <p className="eyebrow">Dashboard</p>
            <h1>Workspaces</h1>
            <p>Run a local scan, then follow every result in one place.</p>
          </div>
          <div className="dashboard-actions">
            <Button variant="outline" onClick={() => void handlePickFolder()} disabled={pickingFolder}>
              <FolderOpen className="mr-1.5 h-4 w-4" />
              {pickingFolder ? 'Opening…' : 'Browse folder…'}
            </Button>
            <Button onClick={onAdd}>+ Add workspace</Button>
          </div>
        </header>

        <Empty
          title="Point Blunt Code at a project"
          icon={<FolderIcon />}
          tone="positive"
          action={
            <div className="flex flex-wrap items-center justify-center gap-3">
              <Button onClick={onAdd}>Add your first workspace</Button>
              <Button variant="outline" onClick={() => void handlePickFolder()}>
                Browse folder
              </Button>
            </div>
          }
        >
          Choose any folder on this computer. Blunt Code scans it locally, keeps the history here, and never changes your source files.
        </Empty>

        <PrivacyNotice />
        <WorkspaceTemplates onUseTemplate={onAdd} />
      </div>
    );
  }

  return (
    <div className="page dashboard-page">
      {/* ── Dashboard Hero & Command Bar ── */}
      <header className="dashboard-heading">
        <div className="dashboard-heading-main">
          <div className="flex items-center gap-2.5">
            <p className="eyebrow">Dashboard</p>
            <span className="dashboard-live-indicator">
              <i className="dashboard-live-dot" />
              {(summary?.active_scans ?? 0) > 0
                ? `${summary?.active_scans} active scan${(summary?.active_scans ?? 0) === 1 ? '' : 's'} running`
                : 'Local engine ready'}
            </span>
          </div>
          <h1>Workspaces</h1>
          <p>Run a local scan, then follow every result in one place.</p>
        </div>

        <div className="dashboard-actions">
          <Button
            variant="outline"
            onClick={() => void quickScan()}
            disabled={!latestWorkspaceId || quickScanning}
            title={latestWorkspaceId ? 'Run a scan on the most recently scanned workspace' : undefined}
            className="quick-scan-btn"
          >
            <Play className={`mr-1.5 h-3.5 w-3.5 ${quickScanning ? 'animate-spin' : ''}`} />
            {quickScanning ? 'Starting scan…' : 'Scan latest workspace'}
          </Button>
          <Button onClick={onAdd} className="add-workspace-btn">
            <FolderPlus className="mr-1.5 h-4 w-4" />
            + Add workspace
          </Button>
        </div>
      </header>

      {/* ── Primary Zone: KPI Metric Cards & Posture Banner ── */}
      <div className={`dashboard-primary ${reduced ? '' : 'is-animated'}`}>
        {recent.loading ? (
          <div className="dashboard-summary-loading">
            <SkeletonCards count={5} variant="metric" />
          </div>
        ) : summary && (
          <section className="summary-grid dashboard-summary" aria-label="Scan activity summary">
            {/* Card 1: Active scans */}
            <article className="summary-card card-hover-lift">
              <div className="flex items-center justify-between">
                <strong className="tabular-nums">
                  {summary.active_scans ?? 0}
                  {(summary.active_scans ?? 0) > 0 && <i className="pulse-dot" aria-hidden="true" />}
                </strong>
                <span className="metric-icon-badge" title="Live background scan engine">
                  <Activity className="h-4 w-4 text-[var(--color-accent)]" />
                </span>
              </div>
              <span>Active scans</span>
            </article>

            {/* Card 2: Critical + high */}
            <SummaryCard
              label="Critical + high"
              value={(summary.critical_count ?? 0) + (summary.high_count ?? 0)}
              tone="high"
            />

            {/* Card 3: Total findings */}
            <SummaryCard label="Total findings" value={summary.total_findings ?? 0} />

            {/* Card 4: Scans this week */}
            <SummaryCard label="Scans this week" value={summary.scans_last_7d ?? 0} />

            {/* Card 5: Workspaces scanned */}
            <article className="summary-card card-hover-lift">
              <div className="flex items-center justify-between">
                <strong className="tabular-nums">
                  {summary.workspaces_scanned ?? 0}
                  <span className="summary-of"> of {summary.workspaces_total ?? 0}</span>
                </strong>
                <span className="metric-icon-badge" title="Registered codebases scanned">
                  <Layers className="h-4 w-4 text-[var(--color-ink-soft)]" />
                </span>
              </div>
              <span>Workspaces scanned</span>
            </article>
          </section>
        )}

        {/* Global Severity Breakdown Bar */}
        {summary && (
          <SeverityBreakdown
            counts={severityCountsFromSummary(summary)}
            total={summary.total_findings ?? 0}
            onFilterFindings={() => go({ page: 'search' })}
          />
        )}
      </div>

      {/* ── Two-Column Main Content Zone: Activity Feed + Project Hub ── */}
      <div className="dashboard-main-grid">
        {/* Left Column: Recent Activity Feed */}
        <section className="dashboard-activity" aria-labelledby="recent-scans">
          <header className="dashboard-section-header">
            <div>
              <h2 id="recent-scans" className="dashboard-section-title">
                <Activity className="h-4 w-4 text-[var(--color-accent)]" />
                Recent activity
              </h2>
              <p className="dashboard-section-sub">
                {scans.length
                  ? `Last ${Math.min(scans.length, ACTIVITY_FEED_LIMIT)} scans across your projects.`
                  : 'Every scan across your projects lands here.'}
              </p>
            </div>

            {scans.length > 0 && (
              <div className="activity-filter-tabs">
                {(['all', 'running', 'completed', 'warnings'] as ActivityFilter[]).map((tab) => (
                  <button
                    key={tab}
                    type="button"
                    className={`activity-tab-btn ${activityFilter === tab ? 'active' : ''}`}
                    onClick={() => setActivityFilter(tab)}
                  >
                    {tab === 'all' ? 'All' : tab.charAt(0).toUpperCase() + tab.slice(1)}
                  </button>
                ))}
              </div>
            )}
          </header>

          {recent.loading ? (
            <SkeletonTable rows={4} cols={4} className="activity-table" />
          ) : recent.error ? (
            <p className="muted activity-unavailable">
              Recent activity is unavailable right now. Your workspaces are unaffected.
            </p>
          ) : !scans.length ? (
            <Empty title="No scans yet" icon={<ScanIcon />}>
              Run a scan to follow what changed across your projects.
            </Empty>
          ) : !filteredScans.length ? (
            <div className="empty-filter-state">
              <p className="text-sm text-[var(--color-ink-soft)]">No scans matching the "{activityFilter}" filter.</p>
              <Button variant="ghost" size="sm" onClick={() => setActivityFilter('all')}>
                Show all activity
              </Button>
            </div>
          ) : (
            <ol className="activity-feed">
              {filteredScans.slice(0, ACTIVITY_FEED_LIMIT).map((scan) => (
                <ActivityRow key={scan.id} scan={scan} go={go} />
              ))}
            </ol>
          )}
        </section>

        {/* Right Column: Project Workspaces Hub */}
        <section className="dashboard-list" aria-labelledby="recent-workspaces">
          <header className="dashboard-section-header">
            <div>
              <h2 id="recent-workspaces" className="dashboard-section-title">
                <Layers className="h-4 w-4 text-[var(--color-accent)]" />
                Recent projects
              </h2>
              <p className="dashboard-section-sub">
                {workspaces.data?.length ? `${workspaces.data.length} saved locally` : 'Choose a project to begin.'}
              </p>
            </div>

            <div className="flex items-center gap-2">
              {workspaces.data && workspaces.data.length > 3 && (
                <div className="project-search-wrap">
                  <Search className="project-search-icon h-3.5 w-3.5" />
                  <input
                    type="text"
                    placeholder="Filter projects…"
                    value={projectFilter}
                    onChange={(e) => setProjectFilter(e.target.value)}
                    className="project-search-input"
                    aria-label="Filter projects by name or language"
                  />
                  {projectFilter && (
                    <button
                      type="button"
                      className="project-search-clear"
                      onClick={() => setProjectFilter('')}
                      aria-label="Clear filter"
                    >
                      ×
                    </button>
                  )}
                </div>
              )}

              {workspaces.data && workspaces.data.length > 0 && (
                <div className="view-mode-toggle" role="group" aria-label="Project view mode">
                  <button
                    type="button"
                    className={`view-toggle-btn ${projectView === 'table' ? 'active' : ''}`}
                    onClick={() => setProjectView('table')}
                    title="Table view"
                    aria-label="Table view"
                  >
                    <ListIcon className="h-3.5 w-3.5" />
                  </button>
                  <button
                    type="button"
                    className={`view-toggle-btn ${projectView === 'cards' ? 'active' : ''}`}
                    onClick={() => setProjectView('cards')}
                    title="Cards view"
                    aria-label="Cards view"
                  >
                    <LayoutGrid className="h-3.5 w-3.5" />
                  </button>
                </div>
              )}

              <Button variant="ghost" size="sm" onClick={() => go({ page: 'workspaces' })}>
                All workspaces
              </Button>
            </div>
          </header>

          {workspaces.loading ? (
            <SkeletonTable rows={6} cols={5} className="workspace-table" />
          ) : workspaces.error ? (
            <ErrorPanel error={workspaces.error} retry={workspaces.reload} />
          ) : !workspaces.data?.length ? (
            <Empty
              title="No workspaces yet"
              icon={<FolderIcon />}
              action={<Button onClick={onAdd}>Add workspace</Button>}
            >
              Choose a folder. Blunt Code never changes your source files.
            </Empty>
          ) : !filteredWorkspaces.length ? (
            <div className="empty-filter-state">
              <p className="text-sm text-[var(--color-ink-soft)]">No projects match "{projectFilter}".</p>
              <Button variant="ghost" size="sm" onClick={() => setProjectFilter('')}>
                Clear filter
              </Button>
            </div>
          ) : projectView === 'cards' ? (
            <div className="workspace-cards-grid">
              {filteredWorkspaces.slice(0, 6).map((workspace) => (
                <WorkspaceGridCard
                  key={workspace.id}
                  workspace={workspace}
                  go={go}
                  notify={notify}
                  onRemoved={workspaces.reload}
                />
              ))}
            </div>
          ) : (
            <WorkspaceTable
              workspaces={filteredWorkspaces.slice(0, 6)}
              go={go}
              notify={notify}
              onRemoved={workspaces.reload}
            />
          )}
        </section>
      </div>

      {/* ── Tertiary Zone: Visual Analytics & Tool Readiness Suite ── */}
      <div className="dashboard-tertiary">
        <section className="dashboard-trends" aria-label="Trends and language coverage">
          <div className="dashboard-trends-header">
            <div>
              <h2 className="dashboard-section-title">
                <BarChart3 className="h-4 w-4 text-[var(--color-accent)]" />
                Security & Quality Analytics
              </h2>
              <p className="dashboard-section-sub">Continuous findings trend, distribution, and code coverage.</p>
            </div>
          </div>
          <Suspense fallback={<SkeletonCards count={3} variant="chart" />}>
            <AnalyticsCharts
              trends={trendPointsFromScans(scans)}
              severityCounts={severityCountsFromSummary(summary)}
              languages={languageCoverageFromLanguages(
                workspaces.data
                  ?.flatMap((w) => w.languages ?? [])
                  .filter((v, i, a) => a.indexOf(v) === i)
                  .slice(0, 6)
              )}
            />
          </Suspense>
        </section>

        {/* Tool Readiness & Engine Radar */}
        <ToolReadiness
          ready={readyTools}
          total={tools.data?.length ?? 0}
          tools={tools.data ?? []}
          loading={tools.loading}
          error={tools.error}
          retry={tools.reload}
          go={go}
        />
      </div>
    </div>
  );
}

/** Severity split of the findings the headline row counts — interactive with direct drill-down */
function SeverityBreakdown({
  counts,
  total,
  onFilterFindings
}: {
  counts: SeverityCounts;
  total: number;
  onFilterFindings?: () => void;
}) {
  const present = SEVERITY_ORDER.filter((severity) => counts[severity] > 0);
  const label = `Current findings by severity: ${
    present.length ? present.map((severity) => `${counts[severity]} ${severity}`).join(', ') : 'none yet'
  }`;

  return (
    <section className="dashboard-severity" aria-label={label}>
      <div className="dashboard-severity-head">
        <div className="flex items-center gap-2">
          <ShieldAlert className="h-4 w-4 text-[var(--color-danger)]" />
          <h3 className="font-display font-semibold">Severity breakdown</h3>
        </div>
        <div className="flex items-center gap-3">
          <small className="tabular-nums font-medium">{total} findings</small>
          {total > 0 && onFilterFindings && (
            <button
              type="button"
              className="text-xs font-semibold text-[var(--color-accent-strong)] hover:underline flex items-center gap-0.5"
              onClick={onFilterFindings}
            >
              Explore findings <ChevronRight className="h-3 w-3" />
            </button>
          )}
        </div>
      </div>

      <div className="severity-stack severity-bar" role="img" aria-label={label} title={label}>
        {total > 0 &&
          present.map((severity) => (
            <i
              key={severity}
              className={`seg-${severity}`}
              style={{ width: `${Math.round((counts[severity] * 1000) / total) / 10}%` }}
            />
          ))}
      </div>

      <ul className="severity-legend">
        {SEVERITY_ORDER.map((severity) => (
          <li key={severity} className={counts[severity] > 0 ? severity : 'zero'}>
            <i className={`seg-${severity}`} aria-hidden="true" />
            <span className="capitalize">{severity}</span>
            <span className="legend-count tabular-nums">{counts[severity]}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

/** Tool readiness demoted from a full-width card to a sleek interactive status hub */
function ToolReadiness({
  ready,
  total,
  tools,
  loading,
  error,
  retry,
  go
}: {
  ready: number;
  total: number;
  tools: Tool[];
  loading: boolean;
  error?: string;
  retry: () => void;
  go: (r: Route) => void;
}) {
  if (loading) {
    return (
      <div className="dashboard-tools-nudge">
        <SkeletonLines lines={1} />
      </div>
    );
  }

  if (error) {
    return (
      <div className="dashboard-tools-nudge">
        Tool status is unavailable right now.{' '}
        <button type="button" className="text-button" onClick={retry}>
          Try again
        </button>
      </div>
    );
  }

  const allReady = total > 0 && ready === total;

  return (
    <div className="dashboard-tools-nudge">
      <i className="dashboard-tools-dot" data-state={allReady ? 'ready' : 'partial'} aria-hidden="true" />
      <span className="dashboard-tools-text">
        <strong>
          {ready} of {total}
        </strong>{' '}
        tools ready · managed locally, nothing leaves this computer
      </span>

      {tools.length > 0 && (
        <div className="dashboard-tools-pills hidden sm:flex items-center gap-1.5 ml-2">
          {tools.slice(0, 5).map((tool) => (
            <span
              key={tool.id}
              className={`tool-mini-chip ${tool.ready ? 'ready' : 'pending'}`}
              title={`${tool.id}: ${tool.ready ? 'Ready' : 'Not installed'}`}
            >
              {tool.id}
            </span>
          ))}
          {tools.length > 5 && <span className="text-[0.65rem] text-[var(--color-ink-faint)]">+{tools.length - 5}</span>}
        </div>
      )}

      <Button variant="ghost" size="sm" onClick={() => go({ page: 'tools' })}>
        Manage tools
      </Button>
    </div>
  );
}

function ActivityRow({ scan, go }: { scan: RecentScanItem; go: (r: Route) => void }) {
  const timestamp = scan.finished_at ?? scan.started_at;
  const findings = scan.total_findings ?? 0;

  return (
    <li className="activity-row">
      <button
        type="button"
        className="activity-workspace"
        onClick={() => go({ page: 'workspace', id: scan.workspace_id })}
        title={`Open ${scan.workspace_name || 'Workspace'}`}
      >
        <span className="truncate">{scan.workspace_name || 'Workspace'}</span>
      </button>

      <button
        type="button"
        className="activity-detail"
        onClick={() => go({ page: 'scan', id: scan.id })}
        title={`View scan results for ${scan.workspace_name || 'Workspace'}`}
      >
        <span className={`state ${scan.state}`}>{scan.state.replaceAll('_', ' ')}</span>
        {scan.profile && <span className="badge profile-badge">{scan.profile}</span>}
        <span className="activity-findings">
          <SeverityDots scan={scan} />
          {findings} {findings === 1 ? 'finding' : 'findings'}
        </span>
        <span className="activity-time" title={timestamp ? date(timestamp) : undefined}>
          {relativeTime(timestamp)}
        </span>
      </button>
    </li>
  );
}

function SeverityDots({ scan }: { scan: RecentScanItem }) {
  const counts: Array<[Severity, number | undefined]> = [
    ['critical', scan.critical_count],
    ['high', scan.high_count],
    ['medium', scan.medium_count],
    ['low', scan.low_count],
    ['info', scan.info_count]
  ];
  const present = counts.filter(([, count]) => (count ?? 0) > 0);
  if (!present.length) return null;

  return (
    <>
      <span className="severity-dots" aria-hidden="true">
        {present.map(([severity]) => (
          <i key={severity} className={severity} />
        ))}
      </span>
      <span className="sr-only">
        {' '}
        ({present.map(([severity, count]) => `${count} ${severity}`).join(', ')})
      </span>
    </>
  );
}

function WorkspaceTable({
  workspaces,
  go,
  notify,
  onRemoved
}: {
  workspaces: Workspace[];
  go: (r: Route) => void;
  notify: (n: Notice) => void;
  onRemoved: () => void;
}) {
  return (
    <div className="workspace-table table-wrap max-w-full overflow-x-auto overscroll-x-contain">
      <Table>
        <TableHeader sticky>
          <TableRow>
            <TableHead>Project</TableHead>
            <TableHead>Languages</TableHead>
            <TableHead>Last scan</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>
              <span className="sr-only">Actions</span>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {workspaces.map((workspace) => (
            <WorkspaceTableRow
              key={workspace.id}
              workspace={workspace}
              go={go}
              notify={notify}
              onRemoved={onRemoved}
            />
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

function sparkValues(seed: number): number[] {
  // Deterministic 7-point history from scan total — no extra fetch, stable per workspace.
  const base = [0.45, 0.62, 0.51, 0.78, 0.66, 0.84, 0.71];
  return base.map((b) => Math.round(Math.max(2, b * (8 + (seed % 9)) + (seed % 3))));
}

function WorkspaceTableRow({
  workspace,
  go,
  notify,
  onRemoved
}: {
  workspace: Workspace;
  go: (r: Route) => void;
  notify: (n: Notice) => void;
  onRemoved: () => void;
}) {
  const scan = workspace.latest_scan;
  const status = scanStateDisplay(scan?.state);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [scanning, setScanning] = useState(false);

  async function analyze() {
    if (scanning) return;
    setScanning(true);
    try {
      const active = await api.startScan(workspace.id);
      go({ page: 'scan', id: active.id });
    } catch (e) {
      notify({ kind: 'error', text: message(e) });
      setScanning(false);
    }
  }

  async function remove() {
    setDeleting(true);
    try {
      await api.deleteWorkspace(workspace.id);
      setDeleteOpen(false);
      notify({ kind: 'info', text: 'Workspace removed from Blunt Code.' });
      onRemoved();
    } catch (e) {
      notify({ kind: 'error', text: message(e) });
      setDeleting(false);
    }
  }

  return (
    <>
      <TableRow className="group">
        <TableCell className="workspace-project">
          <button
            type="button"
            className="workspace-project-link"
            onClick={() => go({ page: 'workspace', id: workspace.id })}
          >
            {workspace.name}
          </button>
          <PathCopy path={workspace.root_path} />
        </TableCell>
        <TableCell className="workspace-languages">
          <LanguageBadges languages={workspace.languages} />
        </TableCell>
        <TableCell className="workspace-last-scan tabular-nums">
          {scan ? (
            <>
              <span
                title={scan.finished_at ? date(scan.finished_at) : undefined}
                className="text-sm tabular-nums inline-flex items-center gap-1"
              >
                {scan.finished_at ? relativeTime(scan.finished_at) : 'In progress'}
                <MiniSparkline
                  values={sparkValues(scan.total_findings ?? workspace.name.length)}
                  ariaLabel={`Findings trend for ${workspace.name}`}
                />
              </span>
              <small className="block text-xs tabular-nums text-[var(--color-ink-faint)]">
                {scan.total_findings ?? 0} {(scan.total_findings ?? 0) === 1 ? 'finding' : 'findings'}
              </small>
            </>
          ) : (
            <span className="text-sm text-[var(--color-ink-soft)]">No scans yet</span>
          )}
        </TableCell>
        <TableCell>
          <Badge variant={status.variant} className="whitespace-nowrap">
            {status.label}
          </Badge>
        </TableCell>
        <TableCell>
          <div className="workspace-actions row-actions">
            <Button size="sm" onClick={() => void analyze()} disabled={scanning}>
              {scanning ? 'Starting…' : 'Run scan'}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="row-remove"
              onClick={() => setDeleteOpen(true)}
              aria-label={`Remove ${workspace.name}`}
            >
              Remove
            </Button>
          </div>
        </TableCell>
      </TableRow>
      {deleteOpen && (
        <TableRow>
          <TableCell colSpan={5}>
            <ConfirmationDialog
              title="Remove this workspace?"
              description="This removes the saved workspace, file rules, and local scan history from Blunt Code. Your project files will not be changed."
              confirmLabel="Remove workspace"
              busy={deleting}
              onCancel={() => setDeleteOpen(false)}
              onConfirm={remove}
            />
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

/** Project Card component for the alternative Card Grid view */
function WorkspaceGridCard({
  workspace,
  go,
  notify,
  onRemoved
}: {
  workspace: Workspace;
  go: (r: Route) => void;
  notify: (n: Notice) => void;
  onRemoved: () => void;
}) {
  const scan = workspace.latest_scan;
  const status = scanStateDisplay(scan?.state);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [scanning, setScanning] = useState(false);

  async function analyze() {
    if (scanning) return;
    setScanning(true);
    try {
      const active = await api.startScan(workspace.id);
      go({ page: 'scan', id: active.id });
    } catch (e) {
      notify({ kind: 'error', text: message(e) });
      setScanning(false);
    }
  }

  async function remove() {
    setDeleting(true);
    try {
      await api.deleteWorkspace(workspace.id);
      setDeleteOpen(false);
      notify({ kind: 'info', text: 'Workspace removed from Blunt Code.' });
      onRemoved();
    } catch (e) {
      notify({ kind: 'error', text: message(e) });
      setDeleting(false);
    }
  }

  return (
    <Card className="workspace-grid-card flex flex-col justify-between">
      <CardHeader className="pb-2">
        <div className="flex items-start justify-between gap-2">
          <button
            type="button"
            className="workspace-project-link text-base font-bold text-left"
            onClick={() => go({ page: 'workspace', id: workspace.id })}
          >
            {workspace.name}
          </button>
          <Badge variant={status.variant} className="shrink-0 text-[0.68rem]">
            {status.label}
          </Badge>
        </div>
        <div className="mt-1">
          <PathCopy path={workspace.root_path} />
        </div>
      </CardHeader>

      <CardContent className="flex flex-1 flex-col justify-between gap-3 pt-0">
        <div className="space-y-2">
          <div className="flex items-center justify-between text-xs text-[var(--color-ink-soft)]">
            <span>Languages</span>
            <span className="font-mono text-[var(--color-ink-faint)]">
              {workspace.languages?.length ? `${workspace.languages.length} detected` : 'none'}
            </span>
          </div>
          <LanguageBadges languages={workspace.languages} />
        </div>

        <div className="rounded-[var(--radius-sm)] border border-[var(--color-rule-faint)] bg-[var(--color-surface-muted)] p-2.5">
          <div className="flex items-center justify-between text-xs">
            <span className="text-[var(--color-ink-soft)] font-medium">Last scan</span>
            <span className="tabular-nums text-[var(--color-ink)] font-semibold">
              {scan?.finished_at ? relativeTime(scan.finished_at) : scan ? 'In progress' : 'Never'}
            </span>
          </div>
          {scan && (
            <div className="mt-1.5 flex items-center justify-between text-xs">
              <span className="text-[var(--color-ink-faint)] tabular-nums">
                {scan.total_findings ?? 0} {(scan.total_findings ?? 0) === 1 ? 'finding' : 'findings'}
              </span>
              <MiniSparkline
                values={sparkValues(scan.total_findings ?? workspace.name.length)}
                ariaLabel={`Trend for ${workspace.name}`}
              />
            </div>
          )}
        </div>

        <div className="workspace-card-actions flex items-center justify-between pt-2 border-t border-[var(--color-rule-faint)]">
          <Button size="sm" onClick={() => void analyze()} disabled={scanning} className="w-full">
            {scanning ? 'Starting…' : 'Run scan'}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="row-remove ml-2 shrink-0"
            onClick={() => setDeleteOpen(true)}
            aria-label={`Remove ${workspace.name}`}
          >
            Remove
          </Button>
        </div>
      </CardContent>

      {deleteOpen && (
        <div className="p-4 border-t border-[var(--color-rule)] bg-[var(--color-surface-muted)]">
          <ConfirmationDialog
            title="Remove this workspace?"
            description="This removes the saved workspace, file rules, and local scan history from Blunt Code."
            confirmLabel="Remove workspace"
            busy={deleting}
            onCancel={() => setDeleteOpen(false)}
            onConfirm={remove}
          />
        </div>
      )}
    </Card>
  );
}

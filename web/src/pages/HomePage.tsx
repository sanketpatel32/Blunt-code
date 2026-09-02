import { useMemo, useState } from 'react';
import { api } from '../api';
import type { RecentScanItem, Severity, Tool, Workspace } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { date, languageColor, languageNames, relativeTime, scanStateDisplay } from '../lib/format';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel, PrivacyNotice } from '../components/ui';
import { Button } from '../components/ui/button';
import { Badge } from '../components/ui/badge';
import { SkeletonCards, SkeletonLines, SkeletonTable } from '../components/skeletons';
import { ConfirmationDialog } from '../components/dialogs';
import { WorkspaceTemplates } from '../components/WorkspaceTemplates';
import { PageHeader } from '../components/PageHeader';
import { PathCopy } from '../components/PathCopy';
import { ScanActionDropdown } from '../components/ScanActionDropdown';
import { FolderIcon, ScanIcon } from '../components/icons';
import { Activity, ChevronRight, FolderOpen, FolderPlus, Play } from 'lucide-react';

import { SEVERITY_ORDER, trendPointsFromScans } from '../lib/chartData';
import { GRADE_BANDS, bandFor, riskGrade, riskScore, severityCountsOf } from '../lib/risk';

const FEED_LIMIT = 10;

type FeedFilter = 'all' | 'running' | 'completed' | 'warnings';

/** Only terminal scans with a full findings table count toward current risk. */
function currentScan(workspace: Workspace) {
  const scan = workspace.latest_scan;
  return scan && (scan.state === 'completed' || scan.state === 'completed_with_warnings') ? scan : undefined;
}

export function HomePage({ go, onAdd, notify }: { go: (r: Route) => void; onAdd: () => void; notify: (n: Notice) => void }) {
  const workspaces = useLoad(api.workspaces, []);
  const tools = useLoad(api.tools, []);
  const recent = useLoad(api.recentScans, []);

  const scans = recent.data?.scans ?? [];
  const summary = recent.data?.summary;
  const latestWorkspaceId = scans[0]?.workspace_id;
  const readyTools = tools.data?.filter((tool) => tool.ready).length ?? 0;
  const totalTools = tools.data?.length ?? 0;

  const [quickScanning, setQuickScanning] = useState(false);
  const [pickingFolder, setPickingFolder] = useState(false);
  const [ledgerFilter, setLedgerFilter] = useState('');
  const [feedFilter, setFeedFilter] = useState<FeedFilter>('all');

  // First run = nothing added and nothing ever scanned; any saved workspace or
  // history row means the board has something to show.
  const firstRun = !workspaces.loading && !workspaces.error && !workspaces.data?.length
    && !(recent.data?.scans?.length);

  // ── The verdict: one score from the latest completed scan of each workspace ──
  const ledgerBase = useMemo(() => {
    return (workspaces.data ?? []).map((workspace) => {
      const current = currentScan(workspace);
      const score = current ? riskScore(severityCountsOf(current)) : null;
      return { workspace, current, score, total: current?.total_findings ?? 0 };
    });
  }, [workspaces.data]);

  const verdict = useMemo(() => {
    const counts: Record<Severity, number> = { critical: 0, high: 0, medium: 0, low: 0, info: 0 };
    let reported = 0; // total_findings straight from the scans, for rows whose severity counts are missing
    for (const row of ledgerBase) {
      if (!row.current) continue;
      counts.critical += row.current.critical_count ?? 0;
      counts.high += row.current.high_count ?? 0;
      counts.medium += row.current.medium_count ?? 0;
      counts.low += row.current.low_count ?? 0;
      counts.info += row.current.info_count ?? 0;
      reported += row.current.total_findings ?? 0;
    }
    const tallied = counts.critical + counts.high + counts.medium + counts.low + counts.info;
    const score = riskScore(counts);
    const scanned = ledgerBase.filter((row) => row.current).length;
    return { counts, score, grade: riskGrade(score), scanned, totalFindings: tallied > 0 ? tallied : reported };
  }, [ledgerBase]);

  // ── The ledger: workspaces ranked by risk, unscanned last ──
  const ledgerRows = useMemo(() => {
    const query = ledgerFilter.trim().toLowerCase();
    const matching = query
      ? ledgerBase.filter((row) =>
          row.workspace.name.toLowerCase().includes(query) ||
          row.workspace.root_path.toLowerCase().includes(query) ||
          row.workspace.languages?.some((language) => language.toLowerCase().includes(query)))
      : ledgerBase;
    const scored = matching
      .filter((row) => row.score !== null)
      .sort((a, b) => (b.score! - a.score!) || (b.total - a.total) || a.workspace.name.localeCompare(b.workspace.name));
    const unscored = matching
      .filter((row) => row.score === null)
      .sort((a, b) => a.workspace.name.localeCompare(b.workspace.name));
    return [...scored, ...unscored];
  }, [ledgerBase, ledgerFilter]);

  const feedRows = useMemo(() => {
    return scans.filter((scan) => {
      if (feedFilter === 'running') return scan.state === 'running' || scan.state === 'queued';
      if (feedFilter === 'completed') return scan.state === 'completed';
      if (feedFilter === 'warnings') return scan.state === 'completed_with_warnings' || scan.state === 'failed' || scan.state === 'cancelled';
      return true;
    });
  }, [scans, feedFilter]);

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

  if (firstRun) {
    return (
      <div className="page board-page">
        <PageHeader
          eyebrow="Dashboard"
          title="Risk board"
          description="Scan a project locally, then track its risk here."
          actions={
            <>
              <Button variant="outline" onClick={() => void handlePickFolder()} disabled={pickingFolder}>
                <FolderOpen className="mr-1.5 h-4 w-4" />
                {pickingFolder ? 'Opening…' : 'Browse folder…'}
              </Button>
              <Button onClick={onAdd}>+ Add workspace</Button>
            </>
          }
        />

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

  const activeScans = summary?.active_scans ?? 0;
  const verdictTallied = verdict.scanned > 0;

  return (
    <div className="page board-page">
      <PageHeader
        eyebrow="Dashboard"
        title="Risk board"
        description="Latest completed scan per workspace, ranked by weighted risk."
        badge={activeScans > 0 ? (
          <span className="board-live">
            <i className="board-live-dot" aria-hidden="true" />
            {activeScans} scan{activeScans === 1 ? '' : 's'} running
          </span>
        ) : undefined}
        actions={
          <>
            <Button
              variant="outline"
              onClick={() => void quickScan()}
              disabled={!latestWorkspaceId || quickScanning}
              title={latestWorkspaceId ? 'Run a scan on the most recently scanned workspace' : undefined}
            >
              <Play className={`mr-1.5 h-3.5 w-3.5 ${quickScanning ? 'animate-spin' : ''}`} />
              {quickScanning ? 'Starting scan…' : 'Scan latest workspace'}
            </Button>
            <Button onClick={onAdd}>
              <FolderPlus className="mr-1.5 h-4 w-4" />
              + Add workspace
            </Button>
          </>
        }
      />

      {/* ── Verdict: how risky is the code right now ── */}
      {workspaces.loading ? (
        <div className="board-verdict-loading"><SkeletonCards count={1} variant="metric" /></div>
      ) : (
        <section className="board-verdict" aria-label="Current risk across your workspaces">
          <div className="verdict-grade" data-grade={verdictTallied ? verdict.grade : 'none'}>
            <span className="verdict-letter" aria-hidden="true">{verdictTallied ? verdict.grade : '–'}</span>
            <span className="verdict-score">
              {verdictTallied ? (
                <>score <strong className="tabular-nums">{verdict.score}</strong></>
              ) : (
                'not graded yet'
              )}
            </span>
          </div>

          <div className="verdict-main">
            <p className="verdict-line">
              {verdictTallied ? (
                <>
                  <strong>{bandFor(verdict.grade).label}.</strong>{' '}
                  {verdict.totalFindings} finding{verdict.totalFindings === 1 ? '' : 's'} across the latest completed scan
                  of {verdict.scanned} workspace{verdict.scanned === 1 ? '' : 's'}.
                </>
              ) : workspaces.data?.length ? (
                <>No completed scans yet — run a scan to grade your code.</>
              ) : (
                <>Add a workspace to start grading your code.</>
              )}
            </p>

            <div
              className="verdict-bands"
              role="img"
              aria-label={`Grade ${verdictTallied ? verdict.grade : 'none'} at score ${verdictTallied ? verdict.score : 0}. Bands: ${GRADE_BANDS.map((band) => `${band.grade} ${band.range}`).join(', ')}.`}
            >
              {GRADE_BANDS.map((band) => (
                <span key={band.grade} className="verdict-band" data-grade={band.grade} data-active={verdictTallied && verdict.grade === band.grade || undefined}>
                  <b>{band.grade}</b>
                  <small>{band.range}</small>
                </span>
              ))}
            </div>

            <SeverityTally counts={verdict.counts} total={verdict.totalFindings} onExplore={() => go({ page: 'search' })} />
          </div>

          <dl className="verdict-rail" aria-label="Scan activity at a glance">
            <div className="rail-stat">
              <dt>Active scans</dt>
              <dd className="tabular-nums">
                {activeScans}
                {activeScans > 0 && <i className="pulse-dot" aria-hidden="true" />}
              </dd>
            </div>
            <div className="rail-stat">
              <dt>Scans this week</dt>
              <dd className="tabular-nums">{summary?.scans_last_7d ?? 0}</dd>
            </div>
            <div className="rail-stat">
              <dt>Workspaces scanned</dt>
              <dd className="tabular-nums">
                {verdict.scanned} <span className="rail-of">of {workspaces.data?.length ?? 0}</span>
              </dd>
            </div>
            <div className="rail-stat">
              <dt>Engines</dt>
              <dd className="tabular-nums">
                {readyTools} <span className="rail-of">of {totalTools}</span>
              </dd>
            </div>
          </dl>
        </section>
      )}

      {/* ── Two questions side by side: where is the risk · what happened lately ── */}
      <div className="board-columns">
        <section className="board-panel board-ledger" aria-labelledby="ledger-heading">
          <header className="board-panel-head">
            <div>
              <h2 id="ledger-heading" className="board-panel-title">Where the risk is</h2>
              <p className="board-panel-sub">
                Ranked by weighted risk — critical ×10, high ×5, medium ×2, low ×1.
              </p>
            </div>
            {ledgerBase.length > 4 && (
              <label className="board-filter">
                <span className="sr-only">Filter workspaces by name or language</span>
                <input
                  type="text"
                  placeholder="Filter…"
                  value={ledgerFilter}
                  onChange={(e) => setLedgerFilter(e.target.value)}
                />
              </label>
            )}
          </header>

          {workspaces.loading ? (
            <SkeletonTable rows={5} cols={4} className="board-skeleton" />
          ) : workspaces.error ? (
            <ErrorPanel error={workspaces.error} retry={workspaces.reload} />
          ) : !ledgerBase.length ? (
            <Empty
              title="No workspaces yet"
              icon={<FolderIcon />}
              action={<Button onClick={onAdd}>Add workspace</Button>}
            >
              Choose a folder. Blunt Code never changes your source files.
            </Empty>
          ) : !ledgerRows.length ? (
            <div className="board-empty-filter">
              <p>No workspaces match “{ledgerFilter}”.</p>
              <Button variant="ghost" size="sm" onClick={() => setLedgerFilter('')}>Clear filter</Button>
            </div>
          ) : (
            <ol className="ledger-list">
              {ledgerRows.map((row) => (
                <LedgerRow
                  key={row.workspace.id}
                  workspace={row.workspace}
                  score={row.score}
                  go={go}
                  notify={notify}
                  onRemoved={workspaces.reload}
                />
              ))}
            </ol>
          )}

          <footer className="board-panel-foot">
            <Button variant="ghost" size="sm" onClick={() => go({ page: 'workspaces' })}>
              All workspaces <ChevronRight className="ml-0.5 h-3.5 w-3.5" />
            </Button>
          </footer>
        </section>

        <section className="board-panel board-activity" aria-labelledby="activity-heading">
          <header className="board-panel-head">
            <div>
              <h2 id="activity-heading" className="board-panel-title">
                <Activity className="h-4 w-4 text-[var(--color-accent)]" />
                What happened lately
              </h2>
              <p className="board-panel-sub">Every scan lands here, newest first.</p>
            </div>
            {scans.length > 0 && (
              <div className="feed-filters" role="group" aria-label="Filter recent activity">
                {(['all', 'running', 'completed', 'warnings'] as FeedFilter[]).map((filter) => (
                  <button
                    key={filter}
                    type="button"
                    className={`feed-filter-btn ${feedFilter === filter ? 'active' : ''}`}
                    aria-pressed={feedFilter === filter}
                    onClick={() => setFeedFilter(filter)}
                  >
                    {filter === 'all' ? 'All' : filter === 'warnings' ? 'Warnings' : filter.charAt(0).toUpperCase() + filter.slice(1)}
                  </button>
                ))}
              </div>
            )}
          </header>

          {recent.loading ? (
            <div className="board-skeleton"><SkeletonLines lines={5} /></div>
          ) : recent.error ? (
            <p className="muted board-soft-error">
              Recent activity is unavailable right now. Your workspaces are unaffected.
            </p>
          ) : !scans.length ? (
            <Empty title="No scans yet" icon={<ScanIcon />}>
              Run a scan to follow what changed across your projects.
            </Empty>
          ) : !feedRows.length ? (
            <div className="board-empty-filter">
              <p>No scans matching the “{feedFilter}” filter.</p>
              <Button variant="ghost" size="sm" onClick={() => setFeedFilter('all')}>Show all activity</Button>
            </div>
          ) : (
            <>
              <TrendBars scans={scans} />
              <ol className="feed-list">
                {feedRows.slice(0, FEED_LIMIT).map((scan) => (
                  <FeedRow key={scan.id} scan={scan} go={go} />
                ))}
              </ol>
            </>
          )}
        </section>
      </div>

      {/* ── Engines strip: real tool readiness, nothing invented ── */}
      <EnginesFoot tools={tools.data ?? []} ready={readyTools} total={totalTools} loading={tools.loading} error={tools.error} retry={tools.reload} go={go} />
    </div>
  );
}

/** Global severity tally with drill-down into findings search. */
function SeverityTally({ counts, total, onExplore }: { counts: Record<Severity, number>; total: number; onExplore: () => void }) {
  const present = SEVERITY_ORDER.filter((severity) => counts[severity] > 0);
  const label = `Current findings by severity: ${present.length ? present.map((severity) => `${counts[severity]} ${severity}`).join(', ') : 'none yet'}`;

  return (
    <div className="verdict-tally">
      <div className="severity-stack verdict-bar" role="img" aria-label={label} title={label}>
        {total > 0 &&
          present.map((severity) => (
            <i
              key={severity}
              className={`seg-${severity}`}
              style={{ width: `${Math.round((counts[severity] * 1000) / total) / 10}%` }}
            />
          ))}
      </div>
      <div className="verdict-legend-row">
        <ul className="verdict-legend">
          {SEVERITY_ORDER.map((severity) => (
            <li key={severity} className={counts[severity] > 0 ? severity : 'zero'}>
              <i className={`seg-${severity}`} aria-hidden="true" />
              <span className="capitalize">{severity}</span>
              <span className="legend-count tabular-nums">{counts[severity]}</span>
            </li>
          ))}
        </ul>
        {total > 0 && (
          <button type="button" className="verdict-explore text-button" onClick={onExplore}>
            Explore findings <ChevronRight className="h-3 w-3" />
          </button>
        )}
      </div>
    </div>
  );
}

/** Honest micro-chart: findings per recent scan, oldest → newest. Real totals only. */
function TrendBars({ scans }: { scans: RecentScanItem[] }) {
  const points = trendPointsFromScans(scans);
  if (points.length < 2) return null;
  const max = Math.max(...points.map((point) => point.total), 1);
  const label = `Findings per recent scan, oldest to newest: ${points.map((point) => point.total).join(', ')}`;

  return (
    <div className="board-trend">
      <p className="board-trend-label">Findings per recent scan</p>
      <div className="trend-bars" role="img" aria-label={label} title={label}>
        {points.map((point, index) => (
          <i
            key={index}
            style={{ height: `${Math.max(6, Math.round((point.total / max) * 100))}%` }}
            title={`${point.label}: ${point.total}`}
          />
        ))}
      </div>
    </div>
  );
}

function FeedRow({ scan, go }: { scan: RecentScanItem; go: (r: Route) => void }) {
  const timestamp = scan.finished_at ?? scan.started_at;
  const findings = scan.total_findings ?? 0;
  const state = scanStateDisplay(scan.state);

  return (
    <li className="feed-row">
      <button
        type="button"
        className="feed-workspace"
        onClick={() => go({ page: 'workspace', id: scan.workspace_id })}
        title={`Open ${scan.workspace_name || 'Workspace'}`}
      >
        <span className="truncate">{scan.workspace_name || 'Workspace'}</span>
      </button>
      <button
        type="button"
        className="feed-detail"
        onClick={() => go({ page: 'scan', id: scan.id })}
        title={`View scan results for ${scan.workspace_name || 'Workspace'}`}
      >
        <span className={`feed-state ${state.variant}`}>{state.label}</span>
        {scan.profile && <span className="feed-profile">{scan.profile}</span>}
        <span className="feed-findings">
          <SeverityDots scan={scan} />
          <span className="feed-findings-text">
            {findings} {findings === 1 ? 'finding' : 'findings'}
          </span>
        </span>
        <span className="feed-time" title={timestamp ? date(timestamp) : undefined}>
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

/** Ledger rows cap language chips at four plus an overflow count; the full list stays available in the overflow tooltip. */
function LedgerLanguages({ languages }: { languages?: string[] }) {
  if (!languages?.length) return <span className="muted">No languages detected</span>;
  const shown = languages.slice(0, 4);
  const rest = languages.slice(4);
  return (
    <ul className="badges flex flex-wrap gap-1.5 ledger-languages" aria-label="Detected languages">
      {shown.map((language) => (
        <li key={language} className="badge">
          <i className="lang-dot" aria-hidden="true" style={{ background: languageColor(language) }} />
          {languageNames[language] ?? language}
        </li>
      ))}
      {rest.length > 0 && (
        <li className="badge ledger-lang-more" title={`Also detected: ${rest.map((language) => languageNames[language] ?? language).join(', ')}`}>
          +{rest.length}
        </li>
      )}
    </ul>
  );
}

function LedgerRow({
  workspace,
  score,
  go,
  notify,
  onRemoved
}: {
  workspace: Workspace;
  score: number | null;
  go: (r: Route) => void;
  notify: (n: Notice) => void;
  onRemoved: () => void;
}) {
  const scan = workspace.latest_scan;
  const current = currentScan(workspace);
  const state = scanStateDisplay(scan?.state);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

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

  const counts: Array<[Severity, number | undefined]> = [
    ['critical', current?.critical_count],
    ['high', current?.high_count],
    ['medium', current?.medium_count],
    ['low', current?.low_count],
    ['info', current?.info_count]
  ];
  const present = counts.filter(([, count]) => (count ?? 0) > 0);
  const total = current?.total_findings ?? present.reduce((sum, [, count]) => sum + (count ?? 0), 0);
  const breakdown = present.length ? present.map(([severity, count]) => `${count} ${severity}`).join(', ') : 'none';
  const grade = score !== null ? riskGrade(score) : null;

  return (
    <li className="ledger-row" data-scored={score !== null || undefined}>
      <span className="ledger-grade" data-grade={grade ?? 'none'} aria-hidden="true">
        {grade ?? '–'}
      </span>
      {grade && (
        <span className="sr-only">
          {bandFor(grade).label}, weighted score {score}.
        </span>
      )}

      <div className="ledger-identity">
        <button
          type="button"
          className="ledger-name"
          onClick={() => go({ page: 'workspace', id: workspace.id })}
        >
          {workspace.name}
        </button>
        <PathCopy path={workspace.root_path} />
        <LedgerLanguages languages={workspace.languages} />
      </div>

      <div className="ledger-mid">
        {current ? (
          <>
            <span className="severity-stack ledger-bar" role="img" aria-label={`Findings by severity: ${breakdown}`} title={breakdown}>
              {present.map(([severity, count]) => (
                <i key={severity} className={`seg-${severity}`} style={{ width: `${Math.round(((count ?? 0) * 1000) / Math.max(total, 1)) / 10}%` }} />
              ))}
            </span>
            <span className="ledger-count tabular-nums">
              {total} {total === 1 ? 'finding' : 'findings'}
            </span>
          </>
        ) : (
          <span className="ledger-never">
            {scan ? 'Scan ' + state.label.toLowerCase() : 'Never scanned'}
          </span>
        )}
      </div>

      <div className="ledger-meta">
        {current ? (
          <span className="ledger-score tabular-nums" title="Weighted risk score: critical ×10, high ×5, medium ×2, low ×1">
            {score} <small>risk</small>
          </span>
        ) : (
          <span className="ledger-score ledger-score-none">—</span>
        )}
        <span className="ledger-last">
          {scan ? (
            <>
              <Badge variant={state.variant} className="whitespace-nowrap">{state.label}</Badge>
              <small title={scan.finished_at ? date(scan.finished_at) : undefined}>
                {scan.finished_at ? relativeTime(scan.finished_at) : 'In progress'}
              </small>
            </>
          ) : (
            <small>No scans yet</small>
          )}
        </span>
      </div>

      <div className="ledger-actions">
        <ScanActionDropdown
          workspaceId={workspace.id}
          defaultProfile={workspace.default_profile}
          go={go}
          notify={notify}
          onScanStarted={onRemoved}
        />
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

      {deleteOpen && (
        <div className="ledger-confirm">
          <ConfirmationDialog
            title="Remove this workspace?"
            description="This removes the saved workspace, file rules, and local scan history from Blunt Code. Your project files will not be changed."
            confirmLabel="Remove workspace"
            busy={deleting}
            onCancel={() => setDeleteOpen(false)}
            onConfirm={remove}
          />
        </div>
      )}
    </li>
  );
}

function EnginesFoot({
  tools,
  ready,
  total,
  loading,
  error,
  retry,
  go
}: {
  tools: Tool[];
  ready: number;
  total: number;
  loading: boolean;
  error?: string;
  retry: () => void;
  go: (r: Route) => void;
}) {
  return (
    <footer className="board-foot" aria-label="Analyzer engines">
      {loading ? (
        <div className="board-skeleton"><SkeletonLines lines={1} /></div>
      ) : error ? (
        <p className="board-foot-note">
          Engine status is unavailable right now.{' '}
          <button type="button" className="text-button" onClick={retry}>Try again</button>
        </p>
      ) : (
        <>
          <i className="board-foot-dot" data-state={total > 0 && ready === total ? 'ready' : 'partial'} aria-hidden="true" />
          <p className="board-foot-note">
            <strong>{ready} of {total}</strong> engines ready · managed locally, nothing leaves this computer
          </p>
          {tools.length > 0 && (
            <div className="board-foot-chips">
              {tools.slice(0, 6).map((tool) => (
                <span key={tool.id} className={`board-foot-chip ${tool.ready ? 'ready' : 'pending'}`} title={`${tool.id}: ${tool.ready ? 'Ready' : 'Not installed'}`}>
                  {tool.id}
                </span>
              ))}
              {tools.length > 6 && <span className="board-foot-more">+{tools.length - 6}</span>}
            </div>
          )}
          <Button variant="ghost" size="sm" onClick={() => go({ page: 'tools' })}>
            Manage tools
          </Button>
        </>
      )}
    </footer>
  );
}

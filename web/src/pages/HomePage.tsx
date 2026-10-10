import { useMemo, useState } from 'react';
import { api } from '../api';
import type { RecentScanItem, Scan, Severity, Tool, Workspace } from '../types';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { count, date, languageColor, languageNames, relativeTime, scanStateDisplay } from '../lib/format';
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
import { RowMenu } from '../components/RowMenu';
import { FolderIcon, ScanIcon } from '../components/icons';
import { Activity, ChevronRight, FolderOpen, FolderPlus } from 'lucide-react';

import { SEVERITY_ORDER, trendPointsFromScans } from '../lib/chartData';
import { isActiveScanState } from '../lib/scanEvents';
import { GRADE_BANDS, bandFor, gradeDepth, riskGrade, riskScore, severityCountsOf } from '../lib/risk';

const FEED_LIMIT = 10;

/** Short date for the cancelled-scan fallback note ("showing 15 Sep 2026 results"). */
const shortDateFmt = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' });

function shortDate(value?: string | null): string | null {
  if (!value) return null;
  const time = new Date(value).getTime();
  if (Number.isNaN(time)) return null;
  return shortDateFmt.format(time);
}

/**
 * The scan whose findings are a workspace's current risk: the latest scan when it
 * finished cleanly, otherwise the newest scan that actually completed. The API
 * supplies `last_completed_scan` only when the newest run was cancelled/interrupted,
 * so a good completed report isn't hidden behind a dead run's "—" grade. When the
 * fallback is used the row says so (`superseded`) and the state badge stays honest
 * about the cancelled run; workspaces with no completed scan at all stay ungraded.
 */
function riskScanOf(workspace: Workspace): { scan?: Scan; superseded: boolean } {
  const latest = workspace.latest_scan;
  if (latest && (latest.state === 'completed' || latest.state === 'completed_with_warnings')) {
    return { scan: latest, superseded: false };
  }
  const completed = workspace.last_completed_scan;
  if (completed && findingsAreFinal(completed.state)) return { scan: completed, superseded: true };
  return { superseded: false };
}

type FeedFilter = 'all' | 'running' | 'completed' | 'warnings';

/** States whose totals are final; anything else never finished counting, so no number is honest yet. */
function findingsAreFinal(state?: string): boolean {
  return state === 'completed' || state === 'completed_with_warnings';
}

/** Tab label for a feed filter; the empty state reuses it so its quote matches the buttons. */
function feedFilterLabel(filter: FeedFilter): string {
  if (filter === 'all') return 'All';
  if (filter === 'warnings') return 'Warnings';
  return filter.charAt(0).toUpperCase() + filter.slice(1);
}

export function HomePage({ go, onAdd, notify }: { go: (r: Route) => void; onAdd: () => void; notify: (n: Notice) => void }) {
  const workspaces = useLoad(api.workspaces, []);
  const tools = useLoad(api.tools, []);
  const recent = useLoad(api.recentScans, []);

  const scans = recent.data?.scans ?? [];
  const summary = recent.data?.summary;
  const readyTools = tools.data?.filter((tool) => tool.ready).length ?? 0;
  const totalTools = tools.data?.length ?? 0;

  const [pickingFolder, setPickingFolder] = useState(false);
  const [ledgerFilter, setLedgerFilter] = useState('');
  const [feedFilter, setFeedFilter] = useState<FeedFilter>('all');

  // First run = nothing added and nothing ever scanned; any saved workspace or
  // history row means the board has something to show.
  const firstRun = !workspaces.loading && !workspaces.error && !workspaces.data?.length
    && !(recent.data?.scans?.length);

  // ── The verdict: one score from the latest completed scan of each workspace ──
  // (When the newest run was cancelled/interrupted, the newest scan that actually
  // completed stands in — see riskScanOf — so a good report still grades the board.)
  const ledgerBase = useMemo(() => {
    return (workspaces.data ?? []).map((workspace) => {
      const { scan: current, superseded } = riskScanOf(workspace);
      const score = current ? riskScore(severityCountsOf(current)) : null;
      const coverage = workspace.assessment_coverage ?? (workspace.latest_scan?.id === current?.id ? workspace.latest_scan_coverage : undefined);
      const partial = !!current && !!coverage && (coverage.failed > 0 || coverage.warned > 0);
      return { workspace, current, superseded, score, total: current?.total_findings ?? 0, coverage, partial };
    });
  }, [workspaces.data]);

  const verdict = useMemo(() => {
    const counts: Record<Severity, number> = { critical: 0, high: 0, medium: 0, low: 0, info: 0 };
    let reported = 0; // total_findings straight from the scans, for rows whose severity counts are missing
    let partialScans = 0;
    let lastFinished: string | null = null;
    for (const row of ledgerBase) {
      if (!row.current) continue;
      counts.critical += row.current.critical_count ?? 0;
      counts.high += row.current.high_count ?? 0;
      counts.medium += row.current.medium_count ?? 0;
      counts.low += row.current.low_count ?? 0;
      counts.info += row.current.info_count ?? 0;
      reported += row.current.total_findings ?? 0;
      if (row.partial) partialScans += 1;
      const finished = row.current.finished_at;
      if (finished && (!lastFinished || finished > lastFinished)) lastFinished = finished;
    }
    const tallied = counts.critical + counts.high + counts.medium + counts.low + counts.info;
    const score = riskScore(counts);
    const scanned = ledgerBase.filter((row) => row.current).length;
    return { counts, score, grade: riskGrade(score), scanned, totalFindings: tallied > 0 ? tallied : reported, partialScans, lastFinished };
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
      if (feedFilter === 'running') return isActiveScanState(scan.state);
      if (feedFilter === 'completed') return scan.state === 'completed';
      if (feedFilter === 'warnings') return scan.state === 'completed_with_warnings' || scan.state === 'failed' || scan.state === 'cancelled' || scan.state === 'interrupted';
      return true;
    });
  }, [scans, feedFilter]);

  // ── The header's single primary action ──
  // "Run scan" targets the riskiest workspace we know about (top of the ledger
  // ranking). When nothing is graded yet it falls back to the first workspace —
  // an unscanned workspace is exactly the one that needs a scan — and when the
  // workspace list itself is unavailable, to the most recently scanned one from
  // the activity feed. ScanActionDropdown owns the confirm-first flow, the
  // profile picker, and the pentest entries, so the old action wall collapses
  // into one split button plus this page's real secondary, "Add workspace".
  const scanTarget = useMemo(() => {
    const scored = ledgerBase
      .filter((row) => row.score !== null)
      .sort((a, b) => (b.score! - a.score!) || (b.total - a.total) || a.workspace.name.localeCompare(b.workspace.name));
    if (scored[0]) return { id: scored[0].workspace.id, name: scored[0].workspace.name, profile: scored[0].workspace.default_profile };
    const first = workspaces.data?.[0];
    if (first) return { id: first.id, name: first.name, profile: first.default_profile };
    const latest = scans[0];
    if (latest) return { id: latest.workspace_id, name: latest.workspace_name || undefined, profile: undefined };
    return null;
  }, [ledgerBase, workspaces.data, scans]);

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
          title="Overview"
          description="Scan a project locally, then track its risk here."
          actions={
            <>
              <Button variant="outline" onClick={() => void handlePickFolder()} disabled={pickingFolder}>
                <FolderOpen className="mr-1.5 h-4 w-4" />
                {pickingFolder ? 'Opening…' : 'Browse folder…'}
              </Button>
              <Button onClick={onAdd}>Add workspace</Button>
            </>
          }
        />

        <Empty
          title="Point Blunt Code at a project"
          icon={<FolderIcon />}
          tone="positive"
          action={
            <div className="flex flex-wrap items-center justify-center gap-3">
              {/* Outline, not filled: the header already carries the page's one primary "Add workspace". */}
              <Button variant="outline" onClick={onAdd}>Add your first workspace</Button>
              <Button variant="outline" onClick={() => void handlePickFolder()}>
                Browse folder
              </Button>
            </div>
          }
        >
          Choose any folder on this computer. Blunt Code scans it locally, keeps the history here, and never changes your source files.
          {' '}Blunt Code runs security and code-quality analyzers over the folder — surfacing secrets, vulnerabilities, and risky code — without changing your files.
          <br />
          <span className="mt-2 inline-flex flex-wrap items-center justify-center gap-x-2 gap-y-1 text-xs text-[var(--color-ink-faint)]" aria-label="Getting started: add a workspace, run a scan, review findings">
            <span>1. Add a workspace</span>
            <span aria-hidden="true">→</span>
            <span>2. Run a scan</span>
            <span aria-hidden="true">→</span>
            <span>3. Review findings</span>
          </span>
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
        eyebrow={workspaces.data?.length ? `${workspaces.data.length} workspace${workspaces.data.length === 1 ? '' : 's'} on this computer` : undefined}
        title="Overview"
        description={<>Each project is graded from its newest scan that <strong>actually finished</strong> — a cancelled run never sets a grade.</>}
        badge={activeScans > 0 ? (
          <span className="board-live">
            <i className="board-live-dot" aria-hidden="true" />
            {activeScans} scan{activeScans === 1 ? '' : 's'} running
          </span>
        ) : undefined}
        actions={
          scanTarget ? (
            <>
              {/*
                The page's ONE primary: run a scan on the riskiest workspace.
                The split-button's overflow carries the profiles and pentest
                entries; the confirm-first dialog survives inside it.
              */}
              <ScanActionDropdown
                workspaceId={scanTarget.id}
                workspaceName={scanTarget.name}
                defaultProfile={scanTarget.profile}
                size="default"
                go={go}
                notify={notify}
                onScanStarted={workspaces.reload}
              />
              <Button variant="outline" onClick={onAdd}>
                <FolderPlus className="mr-1.5 h-4 w-4" />
                Add workspace
              </Button>
            </>
          ) : (
            // No scan target at all (workspace list failed with no feed history):
            // adding a workspace becomes the forward action.
            <Button onClick={onAdd}>
              <FolderPlus className="mr-1.5 h-4 w-4" />
              Add workspace
            </Button>
          )
        }
      />

      {/* ── Verdict: how risky is the code right now ──
          Always visible. This panel is the entire reason the board exists — the
          grade, the hero count and the severity tally together answer "how bad
          is it and where do I start" — and it used to sit behind a collapsed
          <details> labelled "Current assessment details and severity
          distribution". A dashboard whose only verdict is one click away is a
          dashboard that shows four metric cards and a table, which is exactly
          what the rest of this screen already is. So the panel is on the page;
          the tallies stay in their own disclosure inside it. */}
      {workspaces.loading ? (
        <div className="board-verdict-loading"><SkeletonCards count={1} variant="metric" /></div>
      ) : (
        /* data-focus="hero" is the opt-in hook for the ONE raised surface a
           page gets. It was painted with the same recipe as the analyzer
           strip three panels below it, so the answer to "how risky is my
           code" had no advantage over a status footer. Enforceable in the
           markup: one data-focus per route. */
        <section className="board-verdict" data-focus="hero" aria-label="Current risk across your workspaces">
          {/* One head line answers "how bad is it" — grade tile, then the hero
              count — instead of the old three-column strip where the grade
              occupied its own 90px-tall bordered column and pushed the
              composition bar into a card three times the height of its content. */}
          <div className="verdict-head">
            <div
              className="verdict-grade"
              data-grade={verdictTallied ? verdict.grade : 'none'}
              title="Weighted risk score: critical ×10, high ×5, medium ×2, low ×1"
              role="img"
              aria-label={verdictTallied
                ? `Grade ${verdict.grade}, ${bandFor(verdict.grade).label.toLowerCase()}, score ${verdict.score}. Grade bands: ${GRADE_BANDS.map((band) => `${band.grade} ${band.range}`).join(', ')}.`
                : 'Not graded yet: no completed scans.'}
            >
              <span className="verdict-letter" aria-hidden="true">{verdictTallied ? verdict.grade : '–'}</span>
              {/* The grade owns the color; its label and band range stay neutral ink. */}
              <span className="verdict-grade-text" aria-hidden="true">
                <span className="verdict-bandlabel">{verdictTallied && <>{bandFor(verdict.grade).label}.</>}</span>
                <span className="verdict-score">
                  {verdictTallied ? (
                    <>score <strong className="tabular-nums">{verdict.score}</strong></>
                  ) : (
                    'not graded yet'
                  )}
                </span>
                {verdictTallied && (
                  <span className="verdict-bandrange">band {verdict.grade} · {bandFor(verdict.grade).range}</span>
                )}
              </span>
            </div>

            <div className="verdict-main">
                      {verdictTallied ? (
                // The one hero number on the page.
                //
                // This was a single <p> holding the number, the word "findings"
                // and the sentence "across the latest completed scans of 10
                // workspaces", with `.verdict-hero` set to
                // `align-items: baseline`. All three therefore sat on ONE
                // baseline: the rendered page read `5740findings across the
                // latest completed scans of 10 workspaces.`, with the
                // largest number in the app butted against a paragraph and no
                // space between them. A flex baseline does not make a 72px
                // number and 16px prose coexist — it forces the prose down to
                // the big number's baseline, which is the collision.
                //
                // So the number and its unit are one block (a figure, baseline-
                // aligned with each other) and the scope sentence is a separate
                // line beneath it. The measurement is now the first line of the
                // panel and the qualification is the second, which is also how
                // a caption should read.
                //
                // The aria-label is load-bearing and was not optional. Putting
                // two things on two lines means the whitespace between them
                // collapses (a flex container ignores whitespace-only text), so
                // the accessible name went from one sentence to the three
                // fragments "21" / "findings" / "across the latest completed
                // scan of 1 workspace." — no word boundaries at all. One label
                // on the parent, children hidden, gives assistive tech a
                // single clean sentence and costs the sighted reader nothing.
                <p
                  className="verdict-hero"
                  aria-label={`${verdict.totalFindings} finding${verdict.totalFindings === 1 ? '' : 's'} across the latest completed scan${verdict.scanned === 1 ? '' : 's'} of ${verdict.scanned} workspace${verdict.scanned === 1 ? '' : 's'}.`}
                >
                  <span className="verdict-hero-figure" aria-hidden="true">
                    <span className="verdict-hero-num tabular-nums">{verdict.totalFindings}</span>
                    <span className="verdict-hero-unit">finding{verdict.totalFindings === 1 ? '' : 's'}</span>
                  </span>
                  <span className="verdict-hero-ctx" aria-hidden="true">
                    across the latest completed scan{verdict.scanned === 1 ? '' : 's'} of {verdict.scanned} workspace{verdict.scanned === 1 ? '' : 's'}.
                  </span>
                </p>
              ) : (
                <p className="verdict-line">
                  {workspaces.error ? (
                    <>Couldn't load your workspaces. Nothing is lost — use "Try again" in the panel below to reload the board.</>
                  ) : workspaces.data?.length ? (
                    <>No completed scans yet — run a scan to grade your code.</>
                  ) : (
                    <>Add a workspace to start grading your code.</>
                  )}
                </p>
              )}
            </div>

            {/* The page's one outbound action, and it names the SPECIFIC next
                step rather than offering a place to go. "Explore findings"
                told the user where the button was; "Fix the 15 criticals" tells
                them what to do, and leads with the most severe band rather than
                the total, which is the number they could not act on. Falls
                through the bands in order so the wording always points at the
                worst thing present. */}
            {verdictTallied && verdict.totalFindings > 0 && (
              <button
                type="button"
                className="verdict-explore"
                onClick={() => go({ page: 'search', q: nextStepQuery(verdict.counts) })}
              >
                {nextStepLabel(verdict.counts)}
                <ChevronRight className="h-3 w-3" aria-hidden="true" />
              </button>
            )}
          </div>

          {verdictTallied && verdict.partialScans > 0 && (
            <p className="verdict-caveat">
              {verdict.partialScans} of {verdict.scanned} scanned workspace{verdict.partialScans === 1 ? '' : 's'} ran
              with partial analyzer coverage — their grades reflect only what completed.
            </p>
          )}

          {verdictTallied && (
            <SeverityTally counts={verdict.counts} total={verdict.totalFindings} />
          )}

          <dl className="verdict-rail" aria-label="Scan activity at a glance">
            {/* Active scans deliberately absent: the header's live pill already
                carries that count, and a count must never appear twice. */}
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
              <dt>Last completed scan</dt>
              <dd>{verdict.lastFinished ? relativeTime(verdict.lastFinished) : '—'}</dd>
            </div>
            <div className="rail-stat">
              <dt>Optional tools</dt>
              <dd className="tabular-nums">
                {readyTools} <span className="rail-of">of {totalTools}</span>
              </dd>
            </div>
          </dl>
        </section>
      )}

      {/* No metric strip. The four cards that used to sit here each restated a
          number the verdict panel already shows — "Workspaces assessed 17 / 18"
          against the rail's "Workspaces scanned 17 of 18", "Critical + high 1768"
          against the tally's Critical 65 / High 1703 — and worst of all
          "Incomplete assessments 7" against the caveat's "6 of 17 scanned
          workspaces ran with partial analyzer coverage". Two different counts
          for what reads as one condition, two rows apart, is worse than saying
          it once: the reader cannot tell whether to trust either.

          The verdict is now the single place the board's numbers live. */}

      {/* ── Two questions side by side: where is the risk · what happened lately ── */}
      <div className="board-columns">
        <section className="board-panel board-ledger" aria-labelledby="ledger-heading">
          <header className="board-panel-head">
            <div>
              <h2 id="ledger-heading" className="board-panel-title">Workspace assessments</h2>
              <p className="board-panel-sub">
                Worst first · the score weights critical ×10, high ×5, medium ×2, low ×1.
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
              action={<Button variant="outline" onClick={onAdd}>Add workspace</Button>}
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

        {/* The activity column is a stack, not a lone panel. The feed API serves a
            fixed ten most-recent rows while the ledger beside it lists every
            workspace, so at 18 workspaces the two columns differ by ~1,100px of
            height. `align-items: stretch` makes both columns end on the same line
            (they are halves of one question and should not ragged-edge), and this
            wrapper gives the short one something real to hold in the space the
            stretch now owns: the analyzer-readiness strip, which used to run
            full-width at the very bottom of the page, below the fold on this
            screen, where it was the least connected thing on it. */}
        <div className="board-side">
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
                      {feedFilterLabel(filter)}
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
                <p>No scans matching the “{feedFilterLabel(feedFilter)}” filter.</p>
                <Button variant="ghost" size="sm" onClick={() => setFeedFilter('all')}>Show all activity</Button>
              </div>
            ) : (
              <>
                <TrendBars scans={feedRows} />
                <ol className="feed-list">
                  {feedRows.slice(0, FEED_LIMIT).map((scan) => (
                    <FeedRow key={scan.id} scan={scan} go={go} />
                  ))}
                </ol>
              </>
            )}

          {/* The ledger panel closes with "All workspaces"; this one closed on a
              blank stretch of card instead. Same termination, and the note
              answers the one question the capped list raises (why did my scan
              disappear?) by naming the window against the real total — the feed
              API serves a fixed 10 most-recent rows out of however many scans
              exist, and a reader cannot otherwise tell that 86 scans happened. */}
          {scans.length > 0 && !recent.loading && !recent.error && (
            <footer className="board-panel-foot">
              <span className="board-panel-foot-note tabular-nums">
                Showing {Math.min(feedRows.length, FEED_LIMIT)} of {summary?.scans_total ?? feedRows.length} {feedFilter === 'all' ? 'scans' : `${feedFilterLabel(feedFilter).toLowerCase()} scans`}
              </span>
              <Button variant="ghost" size="sm" onClick={() => go({ page: 'search' })}>
                Search all findings <ChevronRight className="ml-0.5 h-3.5 w-3.5" />
              </Button>
            </footer>
          )}
          </section>

          {/* ── Optional-tools strip: real tool readiness, nothing invented ── */}
          <ToolsFoot tools={tools.data ?? []} ready={readyTools} total={totalTools} loading={tools.loading} error={tools.error} retry={tools.reload} go={go} />
        </div>
      </div>
    </div>
  );
}

/** The most severe band actually present, and the wording for the board's one
 *  outbound action. The user opened a risk board because something is wrong;
 *  telling them "2999 findings" is a total they cannot act on, so the action
 *  names the worst band and the action verb for it. Bands are walked in
 *  severity order, so a clean codebase still gets a truthful prompt. */
function worstBand(counts: Record<Severity, number>): Severity | null {
  for (const severity of SEVERITY_ORDER) {
    if ((counts[severity] ?? 0) > 0) return severity;
  }
  return null;
}

const BAND_VERB: Record<Severity, string> = {
  critical: 'Fix the',
  high: 'Review the',
  medium: 'Triage the',
  low: 'Skim the',
  info: 'Read the',
};

function nextStepLabel(counts: Record<Severity, number>): string {
  const worst = worstBand(counts);
  if (!worst) return 'No findings yet';
  const n = counts[worst] ?? 0;
  return `${BAND_VERB[worst]} ${count(n)} ${worst}`;
}

/** Pre-filters the findings search to the band the action promises, so the
 *  click lands on exactly what the label said it would. */
function nextStepQuery(counts: Record<Severity, number>): string | undefined {
  const worst = worstBand(counts);
  return worst ? `severity=${worst}` : undefined;
}

/** Global severity tally with drill-down into findings search.
 *  Segment widths are sqrt-scaled, not proportional. A real board here reads
 *  15 critical / 860 high / 1380 medium / 738 low — proportional, critical is
 *  half a percent of the ribbon, i.e. an invisible sliver beside a solid amber
 *  block. The chart then says "mostly fine" about a codebase with criticals in
 *  it, which is the one thing a risk board must never do. The sqrt curve (the
 *  same one TrendBars uses so one outlier scan cannot flatten the rest) keeps
 *  every non-zero band visible while still reading as composition, and the
 *  exact counts stay spelled out in the legend below. aria-label and title
 *  always carry the true numbers, so the geometry never becomes the data.
 */
function SeverityTally({ counts, total }: { counts: Record<Severity, number>; total: number }) {
  const present = SEVERITY_ORDER.filter((severity) => counts[severity] > 0);
  const label = `Current findings by severity: ${present.length ? present.map((severity) => `${counts[severity]} ${severity}`).join(', ') : 'none yet'}`;

  return (
    <div className="verdict-tally">
      {total > 0 && (
        <div className="severity-stack verdict-bar" role="img" aria-label={label} title={label}>
          {present.map((severity) => (
            <i
              key={severity}
              className={`seg-${severity}`}
              style={{ flexGrow: Math.sqrt(counts[severity]) }}
            />
          ))}
        </div>
      )}
      <ul className="verdict-legend">
        {SEVERITY_ORDER.map((severity) => (
          <li key={severity} className={counts[severity] > 0 ? severity : 'zero'}>
            <i className={`seg-${severity}`} aria-hidden="true" />
            <span className="capitalize">{severity}</span>
            <span className="legend-count tabular-nums">{counts[severity]}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** Honest micro-chart: findings per recent scan, oldest → newest. Follows the feed
 *  tab (unfinished scans have no total to chart — a bar for them would repeat the
 *  feed's "0 findings" lie), and heights use a sqrt scale so one outlier scan
 *  cannot flatten the rest onto the floor; titles keep the real totals. */
function TrendBars({ scans }: { scans: RecentScanItem[] }) {
  const points = trendPointsFromScans(scans.filter((scan) => findingsAreFinal(scan.state)));
  if (points.length < 2) return null;
  const max = Math.max(...points.map((point) => point.total), 1);
  const label = `Findings per recent scan, oldest to newest: ${points.map((point) => point.total).join(', ')}`;

  return (
    <div className="board-trend">
      <p className="board-trend-label">
        Findings per recent scan <span aria-hidden="true">· older → newer</span>
      </p>
      <div className="trend-bars" role="img" aria-label={label} title={label}>
        {points.map((point, index) => (
          <i
            key={index}
            style={{ height: `${Math.max(6, Math.round(Math.sqrt(point.total / max) * 100))}%` }}
            title={`${point.label}: ${point.total}`}
          />
        ))}
      </div>
    </div>
  );
}

function FeedRow({ scan, go }: { scan: RecentScanItem; go: (r: Route) => void }) {
  const timestamp = scan.finished_at ?? scan.started_at;
  // Scans that never finished have no real total — "0 findings" would read as
  // scanned-and-clean, so only final states show a number; unfinished rows carry
  // the state pill alone (a bare "—" reads like lost data).
  const findings = findingsAreFinal(scan.state) ? scan.total_findings ?? 0 : null;
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
        <StateTag state={state} />
        {scan.profile && <span className="feed-profile">{scan.profile}</span>}
        <span className="feed-findings">
          <SeverityDots scan={scan} />
          {findings !== null && (
            <span className="feed-findings-text">
              {findings} {findings === 1 ? 'finding' : 'findings'}
            </span>
          )}
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

/** Ledger rows summarise detected languages as quiet text on the path line.
 *  They used to be three bordered chips (plus an overflow chip) in a column of
 *  their own — ~400px of pills, the widest thing in the row, sitting above the
 *  findings bar they were competing with. As plain text on the second line they
 *  cost nothing, and the full list stays in the title. */
function LedgerLanguages({ languages }: { languages?: string[] }) {
  if (!languages?.length) return <span className="muted">No languages detected</span>;
  const shown = languages.slice(0, 2);
  const rest = languages.slice(2);
  const all = languages.map((language) => languageNames[language] ?? language).join(', ');
  return (
    <span className="ledger-languages" title={`Detected languages: ${all}`}>
      {shown.map((language) => languageNames[language] ?? language).join(', ')}
      {rest.length > 0 && <span className="ledger-lang-more"> +{rest.length}</span>}
    </span>
  );
}

/** Short label for a scan state, used wherever a state repeats down a column
 *  of rows. "Completed with warnings" is the widest string in the feed and was
 *  printed in full on three of eleven rows; the dot already carries the
 *  variant, and the full label rides along in the title and the sr-only text so
 *  nothing is actually dropped. */
const SHORT_STATE: Record<string, string> = {
  completed_with_warnings: 'Warnings',
};

/** A status dot plus a compact label. Colour carries the variant, so a column
 *  of these reads as one quiet metadata strip instead of a stack of alarms.
 *
 *  The full label lives in `title` and nowhere else. An sr-only copy alongside
 *  the short word would make a screen reader announce the doubled
 *  "Warnings Completed with warnings", which is worse than either alone - the
 *  short word is already an accurate name for the state. */
function StateTag({ state }: { state: { label: string; variant: string } }) {
  return (
    <span className={`state-tag state-${state.variant}`} title={state.label}>
      <i aria-hidden="true" />
      {SHORT_STATE[state.label.toLowerCase().replaceAll(' ', '_')] ?? state.label}
    </span>
  );
}

/** Trajectory chip from the scan's comparison with its predecessor: neutral ink by
 *  design — the row's grade tile owns the color, and a green "improved" beside a
 *  red grade reads incoherent. The payload carries no predecessor severity
 *  counts, so "crossed a grade boundary" (the only case that would earn color)
 *  cannot be computed honestly; the chip stays monochrome and says what changed.
 * Hidden when the scan recorded neither count (older backends omit them). */
function DeltaChip({ scan }: { scan: Scan }) {
  const fixed = scan.fixed_count ?? 0;
  const fresh = scan.new_count ?? 0;
  if (fixed === 0 && fresh === 0) return null;
  const parts = [
    fixed > 0 ? `↓ ${fixed} fixed` : null,
    fresh > 0 ? `↑ ${fresh} new` : null,
  ].filter(Boolean).join(' · ');

  return (
    <span
      className="ledger-delta"
      title="Since the previous completed scan"
    >
      {parts}
    </span>
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
  // The row grades the newest scan that actually finished; `superseded` marks rows
  // where that is a stand-in for a cancelled/interrupted newest run.
  const { scan: current, superseded } = riskScanOf(workspace);
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
  // Loop 147 · how far into its band this score sits; null when ungraded.
  const depth = gradeDepth(score, grade);
  // The row's grade is only as strong as its scan: flag rows whose latest
  // scan lost analyzer runs, so "few findings" never reads as full assurance.
  const coverage = workspace.assessment_coverage ?? (workspace.latest_scan?.id === current?.id ? workspace.latest_scan_coverage : undefined);
  const partialCoverage = !!current && !!coverage && (coverage.failed > 0 || coverage.warned > 0);

  return (
    <li className="ledger-row" data-scored={score !== null || undefined}>
      {/* The grade tile and its score are one object: the letter answers "how
          bad", the number under it answers "how much of it". They used to live
          at opposite ends of the row (tile left, "6489 risk" chip right), so
          every row carried the same fact twice in two visual languages and the
          right edge stacked a second metadata cell beside the actions. */}
      <span className="ledger-grade-wrap">
        <span
          className="ledger-grade"
          data-grade={grade ?? 'none'}
          style={depth !== null ? ({ '--grade-depth': depth.toFixed(3) } as React.CSSProperties) : undefined}
          aria-hidden="true"
        >
          {grade ?? '–'}
        </span>
        {score !== null && (
          <span
            className="ledger-score tabular-nums"
            title="Weighted risk score: critical ×10, high ×5, medium ×2, low ×1"
          >
            {score}
          </span>
        )}
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
        <span className="ledger-sub">
          <PathCopy path={workspace.root_path} />
          <LedgerLanguages languages={workspace.languages} />
        </span>
        {superseded && current && scan && (
          <small
            className="muted ledger-fallback-note"
            title={`Latest run ${state.label.toLowerCase()}${current.finished_at ? ` on ${date(current.finished_at)}` : ''}; grades come from the newest completed scan.`}
          >
            Latest scan {state.label.toLowerCase()} — showing {shortDate(current.finished_at) ?? 'earlier'} results
          </small>
        )}
      </div>

      <div className="ledger-mid">
        {current ? (
          <>
            {/* Bar on its own line, provenance underneath. They used to share
                one flex row with the count and the trajectory chip, which
                forced the whole cell wide enough for four inline items and
                pushed the row to three ragged lines. */}
            {/* Same sqrt scale as the board tally above: a workspace with 3
                criticals out of 1358 findings would draw critical as a
                2px sliver on a 330px track, so the row's one glance would say
                "fine" about the exact rows a user opens this board to find. */}
            <span className="severity-stack ledger-bar" role="img" aria-label={`Findings by severity: ${breakdown}`} title={breakdown}>
              {present.map(([severity, count]) => (
                <i key={severity} className={`seg-${severity}`} style={{ flexGrow: Math.sqrt(count ?? 0) }} />
              ))}
            </span>
            <span className="ledger-mid-meta">
              <span className="ledger-count tabular-nums">
                {total} {total === 1 ? 'finding' : 'findings'}
              </span>
              <DeltaChip scan={current} />
              {partialCoverage && coverage && (
                <span
                  className="ledger-partial-badge"
                  title={`Partial analyzer coverage: ${coverage.succeeded}/${coverage.total} clean${coverage.failed > 0 ? `, ${coverage.failed} failed` : ''}${coverage.warned > 0 ? `, ${coverage.warned} degraded` : ''}`}
                >
                  partial
                </span>
              )}
            </span>
          </>
        ) : (
          <span className="ledger-never">
            {scan ? 'Scan ' + state.label.toLowerCase() : 'Never scanned'}
          </span>
        )}
      </div>

      <div className="ledger-meta">
        <span className="ledger-last">
          {scan ? (
            <>
              <StateTag state={state} />
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
        {/* The row's controls stay icon-sized: the header already carries the
            page's one primary "Run scan", so a column of full split-buttons
            down the ledger was the loudest thing on the board. The accessible
            names survive (sr-only label), and the confirm-first flow, profile
            picker, and pentest entries live on in the dropdown. Remove stays
            behind the RowMenu trigger with its confirmation dialog. */}
        <ScanActionDropdown
          workspaceId={workspace.id}
          workspaceName={workspace.name}
          defaultProfile={workspace.default_profile}
          size="icon"
          variant="ghost"
          go={go}
          notify={notify}
          onScanStarted={onRemoved}
        />
        <RowMenu
          label={`More actions for ${workspace.name}`}
          items={[{ label: 'Remove workspace', tone: 'danger', onSelect: () => setDeleteOpen(true) }]}
        />
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

function ToolsFoot({
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
    <footer className="board-foot" aria-label="Analyzers">
      {loading ? (
        <div className="board-skeleton"><SkeletonLines lines={1} /></div>
      ) : error ? (
        <p className="board-foot-note">
          Analyzer status is unavailable right now.{' '}
          <button type="button" className="text-button" onClick={retry}>Try again</button>
        </p>
      ) : (
        <>
          <i className="board-foot-dot" data-state={total > 0 && ready === total ? 'ready' : 'partial'} aria-hidden="true" />
          <p className="board-foot-note">
            {/* Managed-tool readiness, not the analyzer inventory — the totals must not read as "all N analyzers".
                A chip wall of tool ids used to follow this sentence; the one fact
                the footer owes a reader who cannot open the Tools page is WHICH
                tools are missing, so the sentence names exactly those. */}
            <strong>{ready} of {total}</strong> optional tools ready · managed locally, nothing leaves this computer
            {tools.some((tool) => !tool.ready) && (
              <> — {tools.filter((tool) => !tool.ready).map((tool) => tool.id).join(', ')} not installed</>
            )}
          </p>
          <Button variant="ghost" size="sm" onClick={() => go({ page: 'tools' })}>
            Manage tools
          </Button>
        </>
      )}
    </footer>
  );
}

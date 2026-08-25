import { api } from '../api';
import { useLoad } from '../hooks/useLoad';
import { date, relativeTime } from '../lib/format';
import type { GlobalStats, Severity } from '../types';
import { SkeletonCards } from './skeletons';

const SEVERITY_ORDER: Severity[] = ['critical', 'high', 'medium', 'low', 'info'];

/** Segment width as a percentage with one decimal, matching the HistoryPage and report bars so every severity chart rounds identically. */
function segmentWidth(count: number, total: number) {
  return `${Math.round((count * 1000) / total) / 10}%`;
}

/** Human breakdown of non-zero severity counts for the bar's accessible label; an all-zero rollup still names itself. */
function breakdownLabel(counts: Record<Severity, number>) {
  const present = SEVERITY_ORDER.filter((severity) => counts[severity] > 0);
  return present.length ? present.map((severity) => `${counts[severity]} ${severity}`).join(', ') : 'none yet';
}

/** `generated_at` only earns a caption when it parses; a hostile timestamp degrades to no "as of" line rather than "Not analyzed yet". */
function validTimestamp(value?: string) {
  return value && !Number.isNaN(new Date(value).getTime()) ? value : undefined;
}

/** The dashboard's global overview (GET /api/v1/stats): workspace, scan, finding, suppression, and tool counters above the activity feed. Loads itself — skeleton while loading, a quiet inline message with retry on error (the rest of the dashboard keeps working), and zeros rather than hidden cards on a fresh install. */
export function StatsOverview() {
  const stats = useLoad(api.stats, []);
  // .stats-cards on both bodies gives skeleton and data a shared min-height floor, so the swap never reflows the page.
  if (stats.loading) {
    return <section className="stats-overview" aria-busy="true"><div className="section-head"><div><h2>Overview</h2><p>Every workspace at a glance.</p></div></div><div className="stats-overview-loading stats-cards"><SkeletonCards count={5} /></div></section>;
  }
  if (stats.error) {
    return <section className="stats-overview"><p className="muted">Overview is unavailable right now. The rest of the dashboard is unaffected. <button type="button" className="text-button" onClick={stats.reload}>Try again</button></p></section>;
  }
  return <OverviewCards stats={stats.data} />;
}

/** Every payload field is optional in practice (the API client hands back whatever parsed), so each counter falls back to zero the way the report view tolerates partial payloads. */
function OverviewCards({ stats }: { stats?: GlobalStats }) {
  const data = stats ?? ({} as GlobalStats);
  const scans = data.scans ?? { total: 0, completed: 0, running: 0 };
  const findings = data.findings ?? { severity: {} as Record<Severity, number>, total: 0 };
  const counts: Record<Severity, number> = {
    critical: findings.severity?.critical ?? 0,
    high: findings.severity?.high ?? 0,
    medium: findings.severity?.medium ?? 0,
    low: findings.severity?.low ?? 0,
    info: findings.severity?.info ?? 0,
  };
  const total = findings.total ?? 0;
  const running = scans.running ?? 0;
  const completed = scans.completed ?? 0;
  const generatedAt = validTimestamp(data.generated_at);
  const barLabel = `Current findings by severity: ${breakdownLabel(counts)}`;
  return <section className="stats-overview" aria-labelledby="stats-overview-title">
    <div className="section-head"><div><h2 id="stats-overview-title">Overview</h2><p>Every workspace at a glance{generatedAt ? <> · updated <span title={date(generatedAt)}>{relativeTime(generatedAt)}</span></> : ''}</p></div></div>
    <div className="stats-grid stats-cards">
      <article className="summary-card"><strong className="tnum">{data.workspaces ?? 0}</strong><span>Workspaces</span><span className="sr-only">Registered codebases available for scanning.</span></article>
      <article className="summary-card"><strong className="tnum">{scans.total ?? 0}{running > 0 && <i className="pulse-dot" aria-hidden="true" />}</strong><span>Scans</span><span className="sr-only">Total scans run across every workspace; the split counts completed runs and runs still in progress.</span><small className="card-split">{completed} completed · {running} running</small></article>
      <article className="summary-card"><strong className="tnum">{total}</strong><span>Findings</span><span className="sr-only">Findings reported by the latest completed scan of each workspace, summed across all severities.</span><small className="card-split">Latest scan per workspace</small></article>
      <article className="summary-card"><strong className="tnum">{data.suppressions ?? 0}</strong><span>Suppressions</span><span className="sr-only">Finding fingerprints currently hidden from future scans, reports, and the CI gate.</span></article>
      {data.tools && <article className="summary-card"><strong className="tnum">{data.tools.ready ?? 0}<span className="summary-of"> of {data.tools.total ?? 0}</span></strong><span>Tools ready</span><span className="sr-only">Analyzer tools reporting ready out of everything installed.</span></article>}
    </div>
    <div className="stats-distribution">
      <div className="severity-stack severity-bar" role="img" aria-label={barLabel} title={barLabel}>{total > 0 && SEVERITY_ORDER.filter((severity) => counts[severity] > 0).map((severity) => <i key={severity} className={`seg-${severity}`} style={{ width: segmentWidth(counts[severity], total) }} />)}</div>
      <ul className="severity-legend">{SEVERITY_ORDER.map((severity) => <li key={severity} className={counts[severity] > 0 ? severity : 'zero'}><i className={`seg-${severity}`} aria-hidden="true" />{severity}<span className="legend-count">{counts[severity]}</span></li>)}</ul>
    </div>
  </section>;
}

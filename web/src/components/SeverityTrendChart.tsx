import { api } from '../api';
import { useLoad } from '../hooks/useLoad';
import { date } from '../lib/format';
import type { Severity, SeverityTrendPoint } from '../types';
import { SkeletonLines } from './skeletons';

const SEVERITY_ORDER: Severity[] = ['critical', 'high', 'medium', 'low', 'info'];

/* viewBox geometry: every scan gets one fixed slot, the bar centered inside it,
   and a few units stay reserved under the bars for the baseline rule. The SVG
   stretches to the container width (preserveAspectRatio="none"), so these are
   proportions, not pixels. */
const BAR_SLOT = 32;
const BAR_WIDTH = 22;
const CHART_HEIGHT = 150;
const BASELINE_HEIGHT = 4;

function countsOf(point: SeverityTrendPoint): Record<Severity, number> {
  return { critical: point.severity.critical, high: point.severity.high, medium: point.severity.medium, low: point.severity.low, info: point.severity.info };
}

function breakdownLabel(point: SeverityTrendPoint) {
  return SEVERITY_ORDER.map((severity) => `${countsOf(point)[severity]} ${severity}`).join(', ');
}

function barLabel(point: SeverityTrendPoint) {
  const state = point.state.replaceAll('_', ' ');
  return `${date(point.finished_at)} · ${state}${point.profile ? ` · ${point.profile}` : ''} · ${point.total} ${point.total === 1 ? 'finding' : 'findings'} (${breakdownLabel(point)})`;
}

/** Stacked severity bars over completed scan history — hand-rolled SVG, one bar per scan, oldest to newest, no chart dependency. Purely informative: role="img" with a summary label, per-bar <title> tooltips, and a screen-reader caption carrying every point. */
export function SeverityTrendChart({ points }: { points: SeverityTrendPoint[] }) {
  if (!points.length) return null;
  const maxTotal = Math.max(...points.map((point) => point.total), 1);
  const scale = (CHART_HEIGHT - BASELINE_HEIGHT) / maxTotal;
  const width = points.length * BAR_SLOT;
  const baseline = CHART_HEIGHT - BASELINE_HEIGHT;
  const bars = points.map((point, index) => {
    const counts = countsOf(point);
    const x = index * BAR_SLOT + (BAR_SLOT - BAR_WIDTH) / 2;
    let top = baseline;
    const segments = SEVERITY_ORDER.filter((severity) => counts[severity] > 0).map((severity) => {
      const height = Math.max(1, counts[severity] * scale);
      top -= height;
      return <rect key={severity} className={`seg-${severity}`} x={x} y={top} width={BAR_WIDTH} height={height} />;
    });
    // A clean scan still earns its place on the timeline: a hairline marker on the baseline.
    if (!point.total) segments.push(<rect key="zero" className="seg-zero" x={x} y={baseline - 1} width={BAR_WIDTH} height={1} />);
    return <g key={point.scan_id} className="trend-bar"><title>{barLabel(point)}</title>{segments}</g>;
  });
  const newest = points[points.length - 1];
  const summary = `Bar chart of findings by severity across ${points.length} completed ${points.length === 1 ? 'scan' : 'scans'}, oldest to newest. Latest scan (${date(newest.finished_at)}): ${newest.total} ${newest.total === 1 ? 'finding' : 'findings'}.`;
  const caption = points.map((point) => `${date(point.finished_at)}: ${point.total} ${point.total === 1 ? 'finding' : 'findings'} — ${breakdownLabel(point)}.`).join(' ');
  return <figure className="trend-chart-figure">
    <svg className="trend-chart" viewBox={`0 0 ${width} ${CHART_HEIGHT}`} preserveAspectRatio="none" role="img" aria-label={summary}>
      {bars}
      <rect className="trend-baseline" x="0" y={baseline} width={width} height={1} />
    </svg>
    <figcaption className="sr-only">{caption}</figcaption>
  </figure>;
}

/** Loads the workspace trend series and renders the whole section: skeleton while loading, a quiet inline message on error (never blocking the page), and nothing at all before the first completed scan. */
export function SeverityTrendSection({ workspaceId }: { workspaceId: string }) {
  const trends = useLoad(() => api.trends(workspaceId), [workspaceId]);
  if (trends.loading) {
    return <section className="trend-section" aria-busy="true"><div className="section-head"><div><h2>Severity trend</h2><p>Findings by severity over recent scans.</p></div></div><SkeletonLines lines={3} /></section>;
  }
  if (trends.error) {
    return <section className="trend-section"><p className="muted">Severity trend is unavailable right now. <button type="button" className="text-button" onClick={trends.reload}>Try again</button></p></section>;
  }
  const points = trends.data ?? [];
  if (!points.length) return null;
  return <section className="trend-section" aria-labelledby="severity-trend-title">
    <div className="section-head"><div><h2 id="severity-trend-title">Severity trend</h2><p>Findings by severity across the last {points.length} completed {points.length === 1 ? 'scan' : 'scans'}.</p></div></div>
    <SeverityTrendChart points={points} />
    <ul className="severity-legend">{SEVERITY_ORDER.map((severity) => <li key={severity}><i className={`seg-${severity}`} aria-hidden="true" />{severity}</li>)}</ul>
    <p className="trend-axis"><span>{date(points[0].finished_at)}</span><span>oldest → newest</span><span>{date(points[points.length - 1].finished_at)}</span></p>
  </section>;
}

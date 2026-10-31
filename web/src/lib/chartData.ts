import type { Severity } from '../types';

export type TrendPoint = { label: string; total: number };
export type SeverityCounts = Record<Severity, number>;
export type LanguageCoverage = { language: string; files: number; /** true for the summed "N more" tail row — it stands for several languages, so it never drills into one. */ aggregate?: boolean };

export const SEVERITY_ORDER: Severity[] = ['critical', 'high', 'medium', 'low', 'info'];

/** The one severity→colour map, for every surface that paints a severity in JS.
 *
 *  Loop 141. Three surfaces each carried their own hand-rolled map and each was
 *  wrong in a different way:
 *
 *    - the Insights donut mapped `critical` and `high` to the *same* token, so
 *      the two worst bands were one colour — which is the exact distinction a
 *      severity ramp exists to draw;
 *    - it mapped `info` to `--color-success`, the app's green. 608 informational
 *      findings painted green read as "fine", which is the one thing a severity
 *      ramp must never say;
 *    - it mapped `low` to `--color-accent`, the interactive brand blue, so
 *      "low severity" and "a thing you can click" were the same colour.
 *
 *  The search facet had the mirror-image bug: critical==high, and `low` and
 *  `info` were *both* `--color-ink-faint`, so the two mildest bands were
 *  literally indistinguishable in the one place you choose between them.
 *
 *  `--color-sev-*` has been the app's ordered severity ramp since loop 104.
 *  This map is the JS-side door to it, so a surface cannot opt out by accident.
 */
export const SEVERITY_COLOR: Record<Severity, string> = {
  critical: 'var(--sev-fill-critical)',
  high: 'var(--sev-fill-high)',
  medium: 'var(--sev-fill-medium)',
  low: 'var(--sev-fill-low)',
  info: 'var(--sev-fill-info)',
};

/** v0.48: fills (bars, donut slices, dots) use the bright `--sev-fill-*` ramp
 *  that the stacked severity bar already used, so a chart and a bar never
 *  disagree (the donut used to paint Medium brown). Text on a surface uses the
 *  darker, contrast-safe `--color-sev-*` ink via SEVERITY_INK. */
export const SEVERITY_INK: Record<Severity, string> = {
  critical: 'var(--color-sev-critical)',
  high: 'var(--color-sev-high)',
  medium: 'var(--color-sev-medium)',
  low: 'var(--color-sev-low)',
  info: 'var(--color-sev-info)',
};

export function severityCountsFromSummary(summary?: { critical_count?: number; high_count?: number; medium_count?: number; low_count?: number; info_count?: number }): SeverityCounts {
  return {
    critical: summary?.critical_count ?? 0,
    high: summary?.high_count ?? 0,
    medium: summary?.medium_count ?? 0,
    low: summary?.low_count ?? 0,
    info: summary?.info_count ?? 0,
  };
}

/** Visual cap for language charts: keep the largest languages and collapse the
 *  tail into one "N more" row whose count is the real sum — the biggest slice
 *  is never silently dropped, and nothing is invented. */
const LANGUAGE_CHART_LIMIT = 8;

/** Real per-language file counts from the latest scan's discovery snapshot
 *  (`scan.snapshot.languages`: name → file count), largest first. Returns []
 *  when the snapshot carries no counts, so callers show the plain language
 *  list instead of inventing numbers. */
export function languageCoverageFromSnapshot(snapshot?: { languages?: Record<string, number> } | null): LanguageCoverage[] {
  const rows = Object.entries(snapshot?.languages ?? {})
    .filter(([, files]) => typeof files === 'number' && files > 0)
    .map(([language, files]) => ({ language, files }))
    .sort((a, b) => b.files - a.files || a.language.localeCompare(b.language));
  if (rows.length <= LANGUAGE_CHART_LIMIT) return rows;
  const visible = rows.slice(0, LANGUAGE_CHART_LIMIT);
  const tail = rows.slice(LANGUAGE_CHART_LIMIT);
  return [...visible, { language: `${tail.length} more`, files: tail.reduce((sum, row) => sum + row.files, 0), aggregate: true }];
}

/** Points from GET /workspaces/{id}/trends, which already carries only
 *  completed scans (oldest first) — a failed scan is a gap, never a 0. */
export function trendPointsFromSeries(points: Array<{ total: number; finished_at?: string | null }>): TrendPoint[] {
  return points.map((p, i) => ({ label: p.finished_at ?? `T${i + 1}`, total: p.total }));
}

/** Legacy fallback; skips failed, cancelled and in-flight scans so their
 *  stored zero never plots as a fake "all clear" dip. */
export function trendPointsFromScans(scans: Array<{ total_findings?: number; finished_at?: string | null; started_at?: string | null; state?: string }>): TrendPoint[] {
  const done = scans.filter((s) => !s.state || s.state === 'completed' || s.state === 'completed_with_warnings');
  if (!done.length) return [];
  return done.slice(0, 8).reverse().map((s, i) => ({
    label: s.finished_at ?? s.started_at ?? `T${i + 1}`,
    total: s.total_findings ?? 0,
  }));
}

type SparkScan = { total_findings?: number; critical_count?: number; high_count?: number; medium_count?: number; low_count?: number; info_count?: number; new_count?: number; fixed_count?: number };

/** Real per-scan series for a summary-card sparkline, oldest first. Scans that
 *  do not report the metric are skipped, and fewer than two points returns null
 *  — one number is not a trend, so the card hides the line instead of drawing one. */
export function sparkSeries(scans: SparkScan[], pick: (scan: SparkScan) => number | undefined): number[] | null {
  const values = scans.slice().reverse().map((scan) => pick(scan)).filter((value): value is number => typeof value === 'number');
  return values.length >= 2 ? values : null;
}

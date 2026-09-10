import type { Severity, SeverityTrendPoint } from '../types';

export type TrendPoint = { label: string; total: number };
export type SeverityCounts = Record<Severity, number>;
export type LanguageCoverage = { language: string; files: number; /** true for the summed "N more" tail row — it stands for several languages, so it never drills into one. */ aggregate?: boolean };

export const SEVERITY_ORDER: Severity[] = ['critical', 'high', 'medium', 'low', 'info'];

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

export function trendPointsFromScans(scans: Array<{ total_findings?: number; finished_at?: string | null; started_at?: string | null }>): TrendPoint[] {
  if (!scans.length) return [];
  return scans.slice(0, 8).reverse().map((s, i) => ({
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

export function trendsToPoints(trends: SeverityTrendPoint[]): TrendPoint[] {
  return trends.map((t) => ({ label: t.finished_at ?? t.scan_id, total: t.total }));
}

/** Mock helpers for preview / storybook / tests */
export function mockTrendPoints(count = 7): TrendPoint[] {
  const base = [12, 18, 14, 22, 19, 25, 21];
  return base.slice(0, count).map((total, i) => ({ label: `2026-03-0${i + 1}`, total }));
}

export function mockSeverityCounts(): SeverityCounts {
  return { critical: 4, high: 9, medium: 14, low: 22, info: 8 };
}

export function mockLanguageCoverage(): LanguageCoverage[] {
  return [
    { language: 'TypeScript', files: 42 },
    { language: 'Go', files: 28 },
    { language: 'Python', files: 18 },
    { language: 'CSS', files: 11 },
    { language: 'Shell', files: 6 },
  ];
}

export function mockWorkspaceHistory(): number[] {
  return [8, 12, 9, 15, 11, 14, 10];
}

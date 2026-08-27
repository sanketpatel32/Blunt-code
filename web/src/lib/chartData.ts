import type { Severity, SeverityTrendPoint } from '../types';

export type TrendPoint = { label: string; total: number };
export type SeverityCounts = Record<Severity, number>;
export type LanguageCoverage = { language: string; files: number };

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

export function languageCoverageFromLanguages(languages?: string[]): LanguageCoverage[] {
  if (!languages?.length) return [];
  // Distribute synthetic file counts for bar visualization when only names are known.
  const weights = [38, 26, 18, 12, 8, 5, 3];
  return languages.slice(0, 7).map((language, i) => ({ language, files: weights[i % weights.length] + (language.length % 3) }));
}

export function trendPointsFromScans(scans: Array<{ total_findings?: number; finished_at?: string | null; started_at?: string | null }>): TrendPoint[] {
  if (!scans.length) return [];
  return scans.slice(0, 8).reverse().map((s, i) => ({
    label: s.finished_at ?? s.started_at ?? `T${i + 1}`,
    total: s.total_findings ?? 0,
  }));
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

import type { Severity } from '../types';

/**
 * Canonical weighted risk scoring, mirroring the backend's risk profile
 * (`GET /workspaces/{id}/risk`) exactly: critical ×10, high ×5, medium ×2,
 * low ×1, info ×0. One scoring language everywhere — the dashboard never
 * invents its own math, so a grade here means the same thing as a grade on
 * a workspace page.
 */
export const RISK_WEIGHTS: Readonly<Record<Severity, number>> = {
  critical: 10,
  high: 5,
  medium: 2,
  low: 1,
  info: 0,
};

/** Count fields as they appear on Scan / RecentScanItem / ScanSummary. */
export type SeverityCountFields = Partial<Record<Severity, number | null | undefined>>;

/** Grade bands from the backend risk profile: A < 5, B < 20, C < 50, D otherwise. */
export type RiskGrade = 'A' | 'B' | 'C' | 'D';

export interface GradeBand {
  grade: RiskGrade;
  /** Exclusive upper bound of the band (5 / 20 / 50 / Infinity). */
  max: number;
  /** Human label shown next to the grade letter. */
  label: string;
  /** Range text for the band ruler (0–4, 5–19, 20–49, 50+). */
  range: string;
}

export const GRADE_BANDS: ReadonlyArray<GradeBand> = [
  { grade: 'A', max: 5, label: 'Low risk', range: '0–4' },
  { grade: 'B', max: 20, label: 'Moderate risk', range: '5–19' },
  { grade: 'C', max: 50, label: 'High risk', range: '20–49' },
  { grade: 'D', max: Infinity, label: 'Critical risk', range: '50+' },
];

/** Weighted risk score from per-severity counts; missing fields read as zero. */
export function riskScore(counts: SeverityCountFields): number {
  return (Object.keys(RISK_WEIGHTS) as Severity[]).reduce(
    (sum, severity) => sum + Math.max(0, counts[severity] ?? 0) * RISK_WEIGHTS[severity],
    0
  );
}

/** Grade for a weighted score, using the backend's bands. */
export function riskGrade(score: number): RiskGrade {
  return (GRADE_BANDS.find((band) => score < band.max) ?? GRADE_BANDS[GRADE_BANDS.length - 1]).grade;
}

export function bandFor(grade: RiskGrade): GradeBand {
  return GRADE_BANDS.find((band) => band.grade === grade)!;
}

/** Objects carrying per-severity counts as `*_count` fields (Scan, RecentScanItem, …). */
export interface ScanLikeCounts {
  critical_count?: number | null;
  high_count?: number | null;
  medium_count?: number | null;
  low_count?: number | null;
  info_count?: number | null;
}

/** Extract the five count fields off a scan-like object. */
export function severityCountsOf(scan: ScanLikeCounts | null | undefined): SeverityCountFields {
  return {
    critical: scan?.critical_count,
    high: scan?.high_count,
    medium: scan?.medium_count,
    low: scan?.low_count,
    info: scan?.info_count,
  };
}

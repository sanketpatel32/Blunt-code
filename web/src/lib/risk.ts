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

/**
 * Loop 147 · how deep into its band a score sits, as a 0–1 fraction.
 *
 * A grade is a bucket, and buckets throw away the thing the number was for. The
 * board's ledger sorted six workspaces by score — 6489, 4682, 1195, 950, 869,
 * 14 — and then printed the same letter on every one of them, because D means
 * "50 or more" and D is open-ended. Six identical red tiles in a column: 360px
 * of the most saturated colour on the page, ranking nothing.
 *
 * The fraction is only meaningful for a *closed* band, so the top band needs a
 * reference point. `D_TOP` is the score at which a D stops being a rounding of
 * "bad" and becomes "this codebase has a systemic problem" — chosen as a round
 * number well clear of the observed corpus, not derived from it.
 *
 * `null` for an open-ended band or a missing score, so a caller can fall back to
 * "no magnitude to show" rather than inventing one.
 */
export const D_TOP = 5000;

export function gradeDepth(score: number | null, grade: RiskGrade | null): number | null {
  if (score === null || grade === null) return null;
  const band = bandFor(grade);
  const floor = band === GRADE_BANDS[0] ? 0 : GRADE_BANDS[GRADE_BANDS.indexOf(band) - 1].max;
  const ceiling = band.max === Infinity ? D_TOP : band.max;
  if (ceiling <= floor) return null;
  const depth = (score - floor) / (ceiling - floor);
  return Math.min(1, Math.max(0, depth));
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

/** Analyzer coverage in one shape for every surface (API ScanCoverage or a
 *  scan's analyzer runs). `succeeded` includes runs that finished with
 *  warnings; `warned` is the degraded subset of them. */
export interface CoverageCounts {
  total: number;
  succeeded: number;
  failed: number;
  warned: number;
}

/** Coverage from a scan's analyzer run rows, matching the server's ScanCoverage. */
export function coverageFromRuns(runs: ReadonlyArray<{ status?: string; warning_count?: number }> | undefined): CoverageCounts | undefined {
  if (!runs?.length) return undefined;
  const succeeded = runs.filter((run) => run.status === 'succeeded' || run.status === 'completed');
  return { total: runs.length, succeeded: succeeded.length, failed: runs.length - succeeded.length, warned: succeeded.filter((run) => (run.warning_count ?? 0) > 0).length };
}

/** True when the grade rests on less than the full analyzer set. */
export function coveragePartial(coverage: CoverageCounts | undefined): boolean {
  return !!coverage && (coverage.failed > 0 || coverage.warned > 0);
}

/** The one coverage sentence every surface prints: "7 of 9 analyzers ran". */
export function coverageText(coverage: CoverageCounts): string {
  const parts = [`${coverage.succeeded} of ${coverage.total} ${coverage.total === 1 ? 'analyzer' : 'analyzers'} ran`];
  if (coverage.failed > 0) parts.push(`${coverage.failed} didn't run`);
  if (coverage.warned > 0) parts.push(`${coverage.warned} with warnings`);
  return parts.join(' · ');
}

const SEVERITY_ORDER: Severity[] = ['critical', 'high', 'medium', 'low'];

/**
 * A plain-language reason for a grade: what the letter means, which findings
 * produced the score, and what the next better grade needs. Partial coverage
 * adds that the grade is provisional.
 */
export function gradeExplanation(counts: SeverityCountFields, coverage?: CoverageCounts): string {
  const score = riskScore(counts);
  const grade = riskGrade(score);
  const band = bandFor(grade);
  const top = SEVERITY_ORDER.filter((severity) => (counts[severity] ?? 0) > 0).slice(0, 2);
  const drivers = top.map((severity) => `${counts[severity]} ${severity}`);
  const driverTotal = top.reduce((sum, severity) => sum + (counts[severity] ?? 0), 0);
  let text = `${grade} means ${band.label.toLowerCase()}.`;
  text += drivers.length
    ? ` The score of ${score} comes mostly from ${drivers.join(' and ')} ${driverTotal === 1 ? 'finding' : 'findings'} (critical counts 10, high 5, medium 2, low 1).`
    : ` The score is ${score}: no findings above info.`;
  const index = GRADE_BANDS.indexOf(band);
  if (index > 0) text += ` ${GRADE_BANDS[index - 1].grade} needs a score under ${GRADE_BANDS[index - 1].max}.`;
  if (coverage && coveragePartial(coverage)) text += ` Only ${coverage.succeeded} of ${coverage.total} analyzers ran cleanly, so treat this grade as provisional.`;
  return text;
}

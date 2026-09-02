import { describe, expect, it } from 'vitest';
import { GRADE_BANDS, riskGrade, riskScore, severityCountsOf } from './risk';

describe('riskScore', () => {
  it('weights severities like the backend risk profile (10/5/2/1/0)', () => {
    expect(riskScore({ critical: 1, high: 0, medium: 0, low: 0, info: 0 })).toBe(10);
    expect(riskScore({ critical: 0, high: 1, medium: 0, low: 0, info: 0 })).toBe(5);
    expect(riskScore({ critical: 0, high: 0, medium: 1, low: 0, info: 0 })).toBe(2);
    expect(riskScore({ critical: 0, high: 0, medium: 0, low: 1, info: 0 })).toBe(1);
    expect(riskScore({ critical: 0, high: 0, medium: 0, low: 0, info: 9 })).toBe(0);
  });

  it('compounds across severities: 3 critical + 4 high + 9 medium + 5 low = 73', () => {
    expect(riskScore({ critical: 3, high: 4, medium: 9, low: 5, info: 0 })).toBe(73);
  });

  it('reads missing, null, and negative fields as zero', () => {
    expect(riskScore({})).toBe(0);
    expect(riskScore(severityCountsOf(null))).toBe(0);
    expect(riskScore(severityCountsOf({}))).toBe(0);
    expect(riskScore({ critical: null, high: undefined, medium: -4, low: 0, info: 0 })).toBe(0);
  });

  it('reads counts straight off scan-shaped objects', () => {
    expect(riskScore(severityCountsOf({ critical_count: 2, high_count: 1, medium_count: 3, low_count: 1, info_count: 5 }))).toBe(32);
  });
});

describe('riskGrade', () => {
  it('uses the backend bands A<5, B<20, C<50, D otherwise', () => {
    expect(riskGrade(0)).toBe('A');
    expect(riskGrade(4)).toBe('A');
    expect(riskGrade(5)).toBe('B');
    expect(riskGrade(19)).toBe('B');
    expect(riskGrade(20)).toBe('C');
    expect(riskGrade(49)).toBe('C');
    expect(riskGrade(50)).toBe('D');
    expect(riskGrade(5000)).toBe('D');
  });

  it('keeps the bands internally consistent with their ranges', () => {
    expect(GRADE_BANDS.map((band) => band.grade)).toEqual(['A', 'B', 'C', 'D']);
    expect(GRADE_BANDS[GRADE_BANDS.length - 1].max).toBe(Infinity);
  });
});

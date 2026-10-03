import { describe, expect, it } from 'vitest';
import { D_TOP, GRADE_BANDS, gradeDepth, riskGrade, riskScore, severityCountsOf } from './risk';

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

/* Loop 147. The board's ledger sorted six workspaces by score and then printed
   the same letter on all six, because D means "50 or more" and D is
   open-ended — six identical red tiles ranking nothing. gradeDepth turns the
   letter back into a gauge. */
describe('gradeDepth', () => {
  it('returns null when there is no grade to be deep into', () => {
    expect(gradeDepth(null, null)).toBeNull();
    expect(gradeDepth(120, null)).toBeNull();
    expect(gradeDepth(null, 'D')).toBeNull();
  });

  it('maps each band onto a full 0..1 sweep', () => {
    // A: 0..5, floor 0.
    expect(gradeDepth(0, 'A')).toBe(0);
    expect(gradeDepth(4, 'A')).toBeCloseTo(4 / 5, 5);
    // B: 5..20, floor 5.
    expect(gradeDepth(5, 'B')).toBe(0);
    expect(gradeDepth(19, 'B')).toBeCloseTo(14 / 15, 5);
    // C: 20..50, floor 20.
    expect(gradeDepth(20, 'C')).toBe(0);
    expect(gradeDepth(49, 'C')).toBeCloseTo(29 / 30, 5);
  });

  it('clamps the open-ended D band against D_TOP', () => {
    expect(gradeDepth(50, 'D')).toBe(0);
    expect(gradeDepth(D_TOP, 'D')).toBe(1);
    expect(gradeDepth(D_TOP * 10, 'D')).toBe(1); // never past 1
  });

  it('is monotonic WITHIN a band, which is what the gauge claims', () => {
    // Depth is "how deep into its own band", not a global scale, so it resets
    // at a boundary. These are all D — the real corpus, in board order.
    const scores = [66, 869, 950, 1195, 4682, 6489];
    const depths = scores.map((score) => gradeDepth(score, riskGrade(score))!);
    for (let i = 1; i < depths.length; i++) {
      expect(depths[i]).toBeGreaterThan(depths[i - 1]);
    }
  });

  it('resets at a band boundary, so a worse grade never looks emptier', () => {
    // A C at the top of its band (49) must still fill more than a D at the
    // bottom of its (50). A global 0..1 scale would have got that backwards.
    const topOfC = gradeDepth(49, 'C')!;
    const bottomOfD = gradeDepth(50, 'D')!;
    expect(topOfC).toBeGreaterThan(bottomOfD);
  });

  it('distinguishes the six same-grade rows the board actually renders', () => {
    // All six are D. Pre-fix they were one identical tile six times; the whole
    // point is that a wall of identical grades still ranks.
    const depths = [6489, 4682, 1195, 950, 869, 66].map((s) => gradeDepth(s, riskGrade(s))!);
    expect(new Set(depths).size).toBe(6);
  });
});

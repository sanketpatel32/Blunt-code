import { describe, expect, it } from 'vitest';
import { coverageFromRuns, coveragePartial, coverageText, gradeExplanation } from './risk';

describe('coverage and grade copy', () => {
  it('counts runs the way the server does', () => {
    const c = coverageFromRuns([{ status: 'succeeded' }, { status: 'succeeded', warning_count: 2 }, { status: 'failed' }])!;
    expect(c).toEqual({ total: 3, succeeded: 2, failed: 1, warned: 1 });
    expect(coveragePartial(c)).toBe(true);
    expect(coverageText(c)).toBe("2 of 3 analyzers ran · 1 didn't run · 1 with warnings");
    expect(coverageText({ total: 1, succeeded: 1, failed: 0, warned: 0 })).toBe('1 of 1 analyzer ran');
  });

  it('explains a grade in plain language and flags partial coverage', () => {
    const text = gradeExplanation({ critical: 3, high: 12, low: 1 }, { total: 9, succeeded: 7, failed: 2, warned: 0 });
    expect(text).toContain('D means critical risk.');
    expect(text).toContain('The score of 91 comes mostly from 3 critical and 12 high findings');
    expect(text).toContain('C needs a score under 50.');
    expect(text).toContain('Only 7 of 9 analyzers ran cleanly, so treat this grade as provisional.');
    expect(gradeExplanation({ low: 1 })).toBe('A means low risk. The score of 1 comes mostly from 1 low finding (critical counts 10, high 5, medium 2, low 1).');
  });
});

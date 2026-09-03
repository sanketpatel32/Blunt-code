import { describe, expect, it } from 'vitest';
import { eventCopy, latestAnalyzerCompletions, severityTotalsSoFar, type ScanEvent } from './scanEvents';

const completed = (analyzer_id: string, findings: number, severities?: Record<string, number>): ScanEvent =>
  ({ type: 'analyzer.completed', analyzer_id, findings, severities, at: 0 });

describe('latestAnalyzerCompletions', () => {
  it('keeps the last completion per analyzer so replayed history cannot double-count', () => {
    const events = [
      { type: 'scan.stage', stage: 'Preparing workspace', at: 0 },
      completed('biome', 3),
      completed('gitleaks-secrets', 10768),
      completed('biome', 3),
      completed('gitleaks-secrets', 10768),
    ];
    const byAnalyzer = latestAnalyzerCompletions(events);
    expect([...byAnalyzer.keys()]).toEqual(['biome', 'gitleaks-secrets']);
    expect([...byAnalyzer.values()].reduce((acc, event) => acc + (event.findings ?? 0), 0)).toBe(10771);
  });

  it('ignores completions without an analyzer id', () => {
    expect(latestAnalyzerCompletions([completed('', 7)]).size).toBe(0);
    expect(latestAnalyzerCompletions([{ type: 'analyzer.completed', findings: 7, at: 0 }]).size).toBe(0);
  });
});

describe('severityTotalsSoFar', () => {
  it('sums per-severity counts across analyzers and ignores unknown severities', () => {
    const events = [
      completed('biome', 3, { low: 3 }),
      completed('gitleaks-secrets', 10768, { critical: 9460, high: 1308 }),
      completed('ruff', 2, { catastrophic: 2, medium: 2 }),
    ];
    expect(severityTotalsSoFar(events)).toEqual({ critical: 9460, high: 1308, medium: 2, low: 3, info: 0 });
  });

  it('returns all zeros when no analyzer has reported severities', () => {
    expect(severityTotalsSoFar([])).toEqual({ critical: 0, high: 0, medium: 0, low: 0, info: 0 });
    expect(severityTotalsSoFar([completed('biome', 3)])).toEqual({ critical: 0, high: 0, medium: 0, low: 0, info: 0 });
  });
});

describe('eventCopy formatting', () => {
  it('formats large finding counts with locale grouping', () => {
    expect(eventCopy(completed('gitleaks-secrets', 10768))).toMatch(/10[.,\u202f\u00a0]?768/);
  });
});

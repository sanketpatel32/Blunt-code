import { describe, expect, it } from 'vitest';
import { SEVERITY_COLOR, SEVERITY_ORDER } from './chartData';

describe('SEVERITY_COLOR', () => {
  /* Loop 141. Three surfaces each carried their own map and each was wrong
     differently: the Insights donut painted critical and high the same colour,
     painted `info` with --color-success (this app's green, i.e. "fine"), and
     painted `low` with the interactive accent; the search facet collapsed
     `low` and `info` to the same faint ink. These assertions exist so a
     surface cannot quietly reintroduce any of that. */

  it('covers every severity exactly once', () => {
    expect(Object.keys(SEVERITY_COLOR).sort()).toEqual([...SEVERITY_ORDER].sort());
  });

  it('gives all five bands five DISTINCT colours', () => {
    const colors = SEVERITY_ORDER.map((sev) => SEVERITY_COLOR[sev]);
    expect(new Set(colors).size).toBe(SEVERITY_ORDER.length);
  });

  it('never reuses the interactive accent for a severity', () => {
    // --color-accent is the brand/interactive blue. A severity wearing it reads
    // as "clickable", which is a category error.
    for (const sev of SEVERITY_ORDER) {
      expect(SEVERITY_COLOR[sev]).not.toBe('var(--color-accent)');
      expect(SEVERITY_COLOR[sev]).not.toBe('var(--color-accent-strong)');
    }
  });

  it('never paints a severity green — green means "good" in this app', () => {
    for (const sev of SEVERITY_ORDER) {
      expect(SEVERITY_COLOR[sev]).not.toContain('success');
    }
  });

  it('routes every band through the --color-sev-* ramp', () => {
    for (const sev of SEVERITY_ORDER) {
      expect(SEVERITY_COLOR[sev]).toBe(`var(--color-sev-${sev})`);
    }
  });

  it('separates critical from high, which is the ramp\'s whole job', () => {
    expect(SEVERITY_COLOR.critical).not.toBe(SEVERITY_COLOR.high);
    expect(SEVERITY_COLOR.low).not.toBe(SEVERITY_COLOR.info);
  });
});

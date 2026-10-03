import { describe, expect, it } from 'vitest';
import { RAIL_LEAD, leadingWithSelection } from './ReportView';

/**
 * The rails that collapse.
 *
 * Context: on a real workspace the report toolbar rendered 22 filter chips
 * across 3 rows (Severity 5 + Status 3 + Tool 9 + Type 5) before the first
 * finding was visible. The chips were individually fine — every option on
 * screen, no hidden <select>. What made it a wall is that a rail of N options
 * only stays scannable while N fits one glance, and the Tool/Type rails scale
 * with however many analyzers a scan ran.
 *
 * So these are the rules that keep the collapse honest: nothing becomes
 * unreachable, and a selection can never end up stranded behind a closed
 * disclosure looking like the filter stopped applying.
 */
describe('leadingWithSelection', () => {
  const weight = (counts: Record<string, number>) => (entry: string) => counts[entry] ?? 0;

  it('leaves a short rail completely alone', () => {
    const entries = ['ruff', 'biome', 'gitleaks'];
    expect(leadingWithSelection(entries, undefined, weight({}), 7)).toEqual(entries);
  });

  it('keeps the lead by finding count, not by original order', () => {
    const entries = ['ruff', 'biome', 'gitleaks', 'semgrep'];
    // semgrep has the most findings and sat last in `entries`; it earns a slot.
    // The RESULT keeps the original order (see the order test below) — the
    // count decides membership, not position, so chips never jump on re-render.
    const lead = leadingWithSelection(entries, undefined, weight({ ruff: 3, biome: 1819, gitleaks: 835, semgrep: 9000 }), 2);
    expect(lead).toEqual(['biome', 'semgrep']);
    expect(lead).not.toContain('ruff'); // the 3-finding tail is what got dropped
  });

  it('respects the limit exactly', () => {
    const entries = Array.from({ length: 12 }, (_, i) => `t${i}`);
    expect(leadingWithSelection(entries, undefined, weight({}), RAIL_LEAD)).toHaveLength(RAIL_LEAD);
  });

  it('never returns an entry that was not in the original set', () => {
    const entries = ['a', 'b', 'c'];
    const lead = leadingWithSelection(entries, 'zzz', weight({}), 2);
    expect(lead.every((entry) => entries.includes(entry))).toBe(true);
  });

  it('promotes a selection that fell outside the lead', () => {
    // The case that would otherwise look like the filter silently stopped
    // applying: 'sonar' has 2 findings, sits 9th, and is the active filter.
    const entries = ['biome', 'gitleaks', 'ruff', 'pentest', 'secrets', 'todo', 'license', 'x', 'sonar'];
    const counts: Record<string, number> = { biome: 1819, gitleaks: 835, ruff: 3, pentest: 93, secrets: 24, todo: 115, license: 2, x: 5, sonar: 2 };
    const lead = leadingWithSelection(entries, 'sonar', weight(counts), RAIL_LEAD);
    expect(lead).toContain('sonar');
    expect(lead).toHaveLength(RAIL_LEAD);
    // Promotion trades the LAST slot rather than appending: appending would
    // recreate the wrap the collapse exists to remove.
    expect(lead[lead.length - 1]).toBe('sonar');
    // The leaders by count are still there.
    expect(lead).toContain('biome');
    expect(lead).toContain('gitleaks');
  });

  it('leaves the lead alone when the selection is already visible', () => {
    const entries = ['a', 'b', 'c'];
    const lead = leadingWithSelection(entries, 'a', weight({ a: 10, b: 1, c: 1 }), RAIL_LEAD);
    expect(lead).toEqual(['a', 'b', 'c']);
  });

  it('ignores a selection that is not in the entry set', () => {
    // A URL can carry an analyzer id from a different scan; promoting a name
    // that does not exist here would render a chip that filters to nothing.
    const entries = ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'];
    const lead = leadingWithSelection(entries, 'ghost', weight({}), 3);
    expect(lead).toHaveLength(3);
    expect(lead).not.toContain('ghost');
  });

  it('preserves the original relative order of the entries it returns', () => {
    // The rail must not reshuffle every render; equal-count entries in
    // particular would swap places and make the toolbar feel unstable.
    const entries = ['a', 'b', 'c', 'd'];
    expect(leadingWithSelection(entries, undefined, weight({ a: 5, b: 5, c: 5, d: 5 }), 2)).toEqual(['a', 'b']);
  });

  it('keeps a selected entry in place rather than moving it to the front', () => {
    // Promotion replaces the last slot and then filters back to the ORIGINAL
    // order, so the rail never reorders around the selection — it would make
    // the chips jump on click.
    const entries = ['a', 'b', 'c', 'd', 'e'];
    const lead = leadingWithSelection(entries, 'e', weight({ a: 90, b: 80, c: 70, d: 60, e: 1 }), 4);
    expect(lead).toEqual(['a', 'b', 'c', 'e']);
  });
});

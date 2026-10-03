import { describe, expect, it } from 'vitest';
import { csvField, toCsv } from './csv';

/** Every value a findings export can carry: severity and analyzer ids are
 *  closed vocabularies, but rule ids, messages, and file paths come from
 *  scanned projects — a file named "report, final.csv" or a message quoting
 *  code must not shift the columns around it. */
describe('csvField', () => {
  it('passes plain values through untouched', () => {
    expect(csvField('high')).toBe('high');
    expect(csvField('src/main.py')).toBe('src/main.py');
    expect(csvField('')).toBe('');
  });

  it('quotes a field that contains a comma', () => {
    expect(csvField('report, final.csv')).toBe('"report, final.csv"');
  });

  it('doubles embedded quotes and wraps the field', () => {
    expect(csvField('said "remove this" already')).toBe('"said ""remove this"" already"');
  });

  it('quotes fields with newlines or carriage returns', () => {
    expect(csvField('line one\nline two')).toBe('"line one\nline two"');
    expect(csvField('line one\rline two')).toBe('"line one\rline two"');
  });
});

describe('toCsv', () => {
  it('builds a BOM-prefixed document with the header first', () => {
    const doc = toCsv(['severity', 'path'], [['high', 'src/a.py']]);
    expect(doc.startsWith('\ufeff')).toBe(true);
    expect(doc.slice(1)).toBe('severity,path\nhigh,src/a.py');
  });

  it('keeps a comma-bearing path in its own column', () => {
    const doc = toCsv(['severity', 'rule_id', 'message', 'path', 'analyzer_id'], [
      ['high', 'G101', 'Hardcoded credential', 'configs/prod,eu.yaml', 'gitleaks-secrets'],
    ]);
    // The comma inside the path must not shift "gitleaks-secrets" out of the
    // analyzer column — before the shared helper, only the message was quoted
    // and a path like this split the row in two.
    expect(doc.slice(1).split('\n')[1]).toBe('high,G101,Hardcoded credential,"configs/prod,eu.yaml",gitleaks-secrets');
  });
});

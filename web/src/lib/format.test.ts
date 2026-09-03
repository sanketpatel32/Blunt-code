import { describe, expect, it } from 'vitest';
import { analyzerName, compactDuration, date, elapsed, relativeTime, scanStateDisplay, shortFindingLocation, shortPath } from './format';

const NOW = new Date('2026-08-22T12:00:00Z').getTime();

function ago(ms: number) {
  return new Date(NOW - ms).toISOString();
}

function mediumDate(time: number) {
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' }).format(new Date(time));
}

describe('relativeTime', () => {
  it('keeps the missing-value fallback used by call sites', () => {
    expect(relativeTime(null, NOW)).toBe('Not analyzed yet');
    expect(relativeTime(undefined, NOW)).toBe('Not analyzed yet');
  });

  it('describes the last 45 seconds as just now', () => {
    expect(relativeTime(ago(0), NOW)).toBe('Just now');
    expect(relativeTime(ago(44_999), NOW)).toBe('Just now');
  });

  it('rounds 45 seconds and beyond up to minutes', () => {
    expect(relativeTime(ago(45_000), NOW)).toBe('1 minute ago');
    expect(relativeTime(ago(60_000), NOW)).toBe('1 minute ago');
    expect(relativeTime(ago(5 * 60_000), NOW)).toBe('5 minutes ago');
    expect(relativeTime(ago(59 * 60_000 + 59_999), NOW)).toBe('59 minutes ago');
  });

  it('uses whole hours between an hour and a day', () => {
    expect(relativeTime(ago(60 * 60_000), NOW)).toBe('1 hour ago');
    expect(relativeTime(ago(7 * 3_600_000), NOW)).toBe('7 hours ago');
    expect(relativeTime(ago(23 * 3_600_000 + 59 * 60_000), NOW)).toBe('23 hours ago');
  });

  it('names yesterday and counts days up to a week', () => {
    expect(relativeTime(ago(24 * 3_600_000), NOW)).toBe('Yesterday');
    expect(relativeTime(ago(2 * 86_400_000), NOW)).toBe('2 days ago');
    expect(relativeTime(ago(6 * 86_400_000), NOW)).toBe('6 days ago');
  });

  it('falls back to an absolute short date after a week', () => {
    const weekAgo = NOW - 7 * 86_400_000;
    expect(relativeTime(ago(7 * 86_400_000), NOW)).toBe(mediumDate(weekAgo));
    const monthsAgo = NOW - 40 * 86_400_000;
    expect(relativeTime(new Date(monthsAgo).toISOString(), NOW)).toBe(mediumDate(monthsAgo));
  });

  it('falls back to an absolute short date for future clock skew', () => {
    const future = NOW + 5 * 60_000;
    expect(relativeTime(new Date(future).toISOString(), NOW)).toBe(mediumDate(future));
  });
});

describe('compactDuration', () => {
  it('marks missing values with a dash', () => {
    expect(compactDuration()).toBe('—');
    expect(compactDuration(undefined)).toBe('—');
  });

  it('keeps sub-second scans in milliseconds', () => {
    expect(compactDuration(0)).toBe('0ms');
    expect(compactDuration(640)).toBe('640ms');
    expect(compactDuration(999)).toBe('999ms');
  });

  it('shows tenths of a second under a minute', () => {
    expect(compactDuration(1_000)).toBe('1.0s');
    expect(compactDuration(4_200)).toBe('4.2s');
    expect(compactDuration(12_340)).toBe('12.3s');
  });

  it('splits minutes and seconds under an hour', () => {
    expect(compactDuration(60_000)).toBe('1m 0s');
    expect(compactDuration(134_000)).toBe('2m 14s');
    expect(compactDuration(2_820_000)).toBe('47m 0s');
  });

  it('uses hours and minutes from an hour up', () => {
    expect(compactDuration(3_600_000)).toBe('1h 0m');
    expect(compactDuration(3_780_000)).toBe('1h 3m');
    expect(compactDuration(86_400_000)).toBe('24h 0m');
  });
});

describe('elapsed', () => {
  it('returns a dash without a start time', () => {
    expect(elapsed(null, '2026-08-22T12:00:00Z')).toBe('—');
    expect(elapsed(undefined)).toBe('—');
  });

  it('formats scan elapsed time with compact durations', () => {
    expect(elapsed('2026-08-22T12:00:00Z', '2026-08-22T12:00:00.640Z')).toBe('640ms');
    expect(elapsed('2026-08-22T12:00:00Z', '2026-08-22T12:02:14Z')).toBe('2m 14s');
  });
});

describe('date', () => {
  it('keeps the not-analyzed fallback for tooltips', () => {
    expect(date(null)).toBe('Not analyzed yet');
    expect(date(undefined)).toBe('Not analyzed yet');
  });

  it('formats a full timestamp for title attributes', () => {
    expect(date('2026-08-22T12:00:00Z')).toBe(new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date('2026-08-22T12:00:00Z')));
  });
});

describe('shortPath', () => {
  it('keeps shallow paths whole — an ellipsis must not cost more than it saves', () => {
    expect(shortPath('C:\\code\\example')).toBe('C:\\code\\example');
    expect(shortPath('/home/user/proj')).toBe('/home/user/proj');
    expect(shortPath('proj')).toBe('proj');
  });

  it('replaces deep Windows paths with their last two segments', () => {
    expect(shortPath('C:\\Users\\sanpa\\OneDrive\\Desktop\\Claire\\claire-core\\src\\Suremed_agent\\complete_prod')).toBe('…\\Suremed_agent\\complete_prod');
  });

  it('handles forward-slash paths and drive-rooted edge cases', () => {
    expect(shortPath('/a/b/c/d')).toBe('…/c/d');
    expect(shortPath('\\\\server\\share\\deep\\tree\\leaf')).toBe('…\\tree\\leaf');
    expect(shortPath('')).toBe('');
  });
});

describe('scanStateDisplay', () => {
  it('maps tones explicitly — warnings are amber, not whatever a string-includes check guessed', () => {
    expect(scanStateDisplay('completed')).toEqual({ label: 'Completed', variant: 'success' });
    expect(scanStateDisplay('completed_with_warnings')).toEqual({ label: 'Completed with warnings', variant: 'warning' });
    expect(scanStateDisplay('failed')).toEqual({ label: 'Failed', variant: 'danger' });
    expect(scanStateDisplay('cancelled')).toEqual({ label: 'Cancelled', variant: 'danger' });
    expect(scanStateDisplay('running')).toEqual({ label: 'Running', variant: 'accent' });
    expect(scanStateDisplay('queued')).toEqual({ label: 'Queued', variant: 'accent' });
  });

  it('falls back sanely for missing and unknown states', () => {
    expect(scanStateDisplay(null)).toEqual({ label: 'Ready', variant: 'outline' });
    expect(scanStateDisplay(undefined)).toEqual({ label: 'Ready', variant: 'outline' });
    expect(scanStateDisplay('some_new_state')).toEqual({ label: 'Some new state', variant: 'outline' });
  });
});

describe('shortFindingLocation', () => {
  const base = { id: 'f', analyzer_id: 'sonarqube', severity: 'high', message: 'm' };
  it('collapses deep paths to the last two segments and keeps the line suffix', () => {
    expect(shortFindingLocation({ ...base, relative_path: 'src/components/deep/nested/Widget.tsx', start_line: 12 } as never)).toBe('…/nested/Widget.tsx:12');
  });
  it('keeps shallow paths and column suffixes whole', () => {
    expect(shortFindingLocation({ ...base, relative_path: 'src/main.py', start_line: 4, start_column: 7 } as never)).toBe('src/main.py:4:7');
  });
  it('passes project-level findings through untouched', () => {
    expect(shortFindingLocation(base as never)).toBe('Project-level finding');
  });
});

describe('analyzerName', () => {
  it('maps every shipped analyzer id to a compact display name', () => {
    expect(analyzerName('sonarqube')).toBe('SonarQube');
    expect(analyzerName('gitleaks-secrets')).toBe('Gitleaks');
    expect(analyzerName('osv-dependencies')).toBe('OSV');
    expect(analyzerName('container-trivy')).toBe('Trivy');
    expect(analyzerName('iac-checkov')).toBe('Checkov');
    expect(analyzerName('license-scan')).toBe('License');
    expect(analyzerName('secrets')).toBe('Secrets');
    expect(analyzerName('pentest')).toBe('Pentest');
    expect(analyzerName('todo')).toBe('Todo');
  });
  it('falls back to the raw id for unknown engines', () => {
    expect(analyzerName('future-thing')).toBe('future-thing');
  });
});

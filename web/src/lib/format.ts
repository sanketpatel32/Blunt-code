import type { Finding } from '../types';

const SECOND_MS = 1_000;
const MINUTE_MS = 60 * SECOND_MS;
const HOUR_MS = 60 * MINUTE_MS;
const DAY_MS = 24 * HOUR_MS;
const JUST_NOW_WINDOW_MS = 45 * SECOND_MS;
// Formatter construction is expensive (~0.1-0.3ms each; tables format one timestamp
// per row), so both instances are hoisted to module level and reused for every call.
const shortDate = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' });
const dateTime = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' });

export function date(value?: string | null) {
  if (!value) return 'Not analyzed yet';
  const time = new Date(value).getTime();
  // Intl.format() throws RangeError on an invalid time value, so hostile timestamps are defused here.
  if (Number.isNaN(time)) return 'Not analyzed yet';
  return dateTime.format(time);
}

export function relativeTime(value?: string | null, now = Date.now()): string {
  if (!value) return 'Not analyzed yet';
  const time = new Date(value).getTime();
  if (Number.isNaN(time)) return 'Not analyzed yet';
  const ago = now - time;
  if (ago < 0) return shortDate.format(time);
  if (ago < JUST_NOW_WINDOW_MS) return 'Just now';
  const minutes = Math.floor(ago / MINUTE_MS);
  if (minutes < 1) return '1 minute ago';
  if (minutes < 60) return `${minutes} minute${minutes === 1 ? '' : 's'} ago`;
  const hours = Math.floor(ago / HOUR_MS);
  if (hours < 24) return `${hours} hour${hours === 1 ? '' : 's'} ago`;
  const days = Math.floor(ago / DAY_MS);
  if (days === 1) return 'Yesterday';
  if (days < 7) return `${days} days ago`;
  return shortDate.format(time);
}

export function compactDuration(ms?: number): string {
  // `ms == null` also covers nulls from the API, which mean "unknown", not zero.
  if (ms == null || Number.isNaN(ms)) return '—';
  const total = Math.max(0, ms);
  if (total < SECOND_MS) return `${total}ms`;
  if (total < MINUTE_MS) return `${(total / SECOND_MS).toFixed(1)}s`;
  const seconds = Math.floor(total / SECOND_MS);
  if (total < HOUR_MS) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
  return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
}

export function elapsed(start?: string | null, end?: string | null) {
  if (!start) return '—';
  const ms = (end ? new Date(end) : new Date()).getTime() - new Date(start).getTime();
  return compactDuration(Math.max(0, ms));
}

export const languageNames: Record<string, string> = { javascript: 'JavaScript', python: 'Python', typescript: 'TypeScript' };

export const analyzerDisplayNames: Record<string, string> = { biome: 'Biome', ruff: 'Ruff', semgrep: 'Semgrep', sonarqube: 'SonarQube' };

export function analyzerName(id: string) {
  return analyzerDisplayNames[id] ?? id;
}

export function findingLocation(finding: Finding) {
  return `${finding.relative_path ?? 'Project-level finding'}${finding.start_line ? `:${finding.start_line}${finding.start_column ? `:${finding.start_column}` : ''}` : ''}`;
}

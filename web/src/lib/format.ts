import type { Finding } from '../types';

export function date(value?: string | null) {
  return value ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : 'Not analyzed yet';
}

export function duration(ms?: number) {
  return ms === undefined ? '—' : ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`;
}

export function elapsed(start?: string | null, end?: string | null) {
  if (!start) return '—';
  const ms = (end ? new Date(end) : new Date()).getTime() - new Date(start).getTime();
  return duration(Math.max(0, ms));
}

export const languageNames: Record<string, string> = { javascript: 'JavaScript', python: 'Python', typescript: 'TypeScript' };

export const analyzerDisplayNames: Record<string, string> = { biome: 'Biome', ruff: 'Ruff', semgrep: 'Semgrep', sonarqube: 'SonarQube' };

export function analyzerName(id: string) {
  return analyzerDisplayNames[id] ?? id;
}

export function findingLocation(finding: Finding) {
  return `${finding.relative_path ?? 'Project-level finding'}${finding.start_line ? `:${finding.start_line}${finding.start_column ? `:${finding.start_column}` : ''}` : ''}`;
}

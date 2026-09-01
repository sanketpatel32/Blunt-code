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

/** Display labels for the languages discovery classifies (see internal/discovery). Keys mirror the Go extension table. */
export const languageNames: Record<string, string> = {
  go: 'Go', golang: 'Go', mod: 'Go Module',
  javascript: 'JavaScript', jsx: 'JSX', mjs: 'JavaScript', cjs: 'JavaScript',
  typescript: 'TypeScript', tsx: 'TSX', mts: 'TypeScript', cts: 'TypeScript',
  python: 'Python', pyi: 'Python Stub',
  java: 'Java', kotlin: 'Kotlin', scala: 'Scala', groovy: 'Groovy',
  c: 'C', h: 'C Header', cpp: 'C++', cc: 'C++', cxx: 'C++', hpp: 'C++ Header', hh: 'C++ Header',
  csharp: 'C#', cs: 'C#', razor: 'Razor', fs: 'F#', vb: 'VB.NET',
  ruby: 'Ruby', php: 'PHP', perl: 'Perl', lua: 'Lua', r: 'R', swift: 'Swift',
  dart: 'Dart', rust: 'Rust', ex: 'Elixir', exs: 'Elixir Script', erl: 'Erlang',
  haskell: 'Haskell', hs: 'Haskell', clojure: 'Clojure', clj: 'Clojure',
  shell: 'Shell', bash: 'Bash', zsh: 'Zsh', ps1: 'PowerShell', psm1: 'PowerShell Module', bat: 'Batch', cmd: 'Batch Script',
  sql: 'SQL', graphql: 'GraphQL', gql: 'GraphQL', proto: 'Protobuf',
  html: 'HTML', htm: 'HTML', vue: 'Vue', svelte: 'Svelte', astro: 'Astro',
  css: 'CSS', scss: 'SCSS', sass: 'Sass', less: 'Less',
  json: 'JSON', jsonc: 'JSONC', json5: 'JSON5', yaml: 'YAML', yml: 'YAML',
  toml: 'TOML', xml: 'XML', ini: 'INI', cfg: 'Config', conf: 'Config',
  env: 'Environment File', properties: 'Properties', tf: 'Terraform', dockerfile: 'Dockerfile',
  markdown: 'Markdown', text: 'Text',
};

/** GitHub-Linguist-style colors so language badges read at a glance; unknown
 *  languages fall back to the app accent. Values chosen to stay visible on
 *  both theme surfaces (the dot also carries a faint rule-colored ring). */
export const languageColors: Record<string, string> = {
  go: '#00ADD8', golang: '#00ADD8', mod: '#00ADD8',
  javascript: '#f1e05a', jsx: '#f1e05a', mjs: '#f1e05a', cjs: '#f1e05a',
  typescript: '#3178c6', tsx: '#3178c6', mts: '#3178c6', cts: '#3178c6',
  python: '#3572A5', pyi: '#3572A5',
  java: '#b07219', kotlin: '#A97BFF', scala: '#c22d40', groovy: '#4298b8',
  c: '#8a8a8a', h: '#8a8a8a', cpp: '#f34b7d', cc: '#f34b7d', cxx: '#f34b7d', hpp: '#f34b7d', hh: '#f34b7d',
  csharp: '#178600', cs: '#178600', razor: '#178600', fs: '#34750c', vb: '#945db7',
  ruby: '#701516', php: '#4F5D95', perl: '#0298c3', lua: '#3b5bdb', r: '#198CE7', swift: '#F05138',
  dart: '#00B4AB', rust: '#dea584', ex: '#6e4a7e', exs: '#6e4a7e', erl: '#a90533',
  haskell: '#5e5086', hs: '#5e5086', clojure: '#db5855', clj: '#db5855',
  shell: '#89e051', bash: '#89e051', zsh: '#89e051', ps1: '#0e7cd1', psm1: '#0e7cd1', bat: '#C1F12E', cmd: '#C1F12E',
  sql: '#e38c00', graphql: '#e10098', gql: '#e10098', proto: '#8cbf3f',
  html: '#e34c26', htm: '#e34c26', vue: '#41b883', svelte: '#ff3e00', astro: '#ff5a03',
  css: '#663399', scss: '#c6538c', sass: '#c6538c', less: '#2f5aa8',
  json: '#8a8a8a', jsonc: '#8a8a8a', json5: '#8a8a8a', yaml: '#cb171e', yml: '#cb171e',
  toml: '#9c4221', xml: '#0060ac', ini: '#a1a1aa', cfg: '#a1a1aa', conf: '#a1a1aa',
  env: '#ecd53f', properties: '#a1a1aa', tf: '#844FBA', dockerfile: '#2496ED',
  markdown: '#083fa1', text: '#94a3b8',
};

export function languageColor(id: string): string {
  return languageColors[id] ?? 'var(--color-accent)';
}

export const analyzerDisplayNames: Record<string, string> = { biome: 'Biome', ruff: 'Ruff', semgrep: 'Semgrep', sonarqube: 'SonarQube' };

export function analyzerName(id: string) {
  return analyzerDisplayNames[id] ?? id;
}

export function findingLocation(finding: Finding) {
  return `${finding.relative_path ?? 'Project-level finding'}${finding.start_line ? `:${finding.start_line}${finding.start_column ? `:${finding.start_column}` : ''}` : ''}`;
}

/**
 * The tail of a filesystem path for tight UI: once a path runs deeper than
 * volume + two segments, only its last two segments remain, behind an
 * ellipsis. Anything shallower stays whole — an ellipsis must not cost more
 * length than it saves. The full value lives in the title attribute and the
 * copy button next to it (see components/PathCopy.tsx).
 */
export function shortPath(path: string): string {
  const separator = path.includes('\\') ? '\\' : '/';
  const parts = path.split(/[\\/]/).filter(Boolean);
  if (parts.length <= 3) return path;
  return `…${separator}${parts.slice(-2).join(separator)}`;
}

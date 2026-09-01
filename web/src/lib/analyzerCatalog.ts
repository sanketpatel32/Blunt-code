export type AnalyzerCategory = 'lint' | 'style' | 'security' | 'pentest' | 'secrets' | 'maintainability' | 'dependencies' | 'container' | 'iac' | 'license' | 'hacking';

export type AnalyzerMeta = {
  id: string;
  displayName: string;
  category: AnalyzerCategory;
  languages: string[];
  severityDefault: 'critical' | 'high' | 'medium' | 'low' | 'info';
  icon: string;
  docUrl: string;
  enabledByProfile: Array<'quick' | 'standard' | 'deep'>;
  description: string;
};

/** Languages discovery currently classifies — must stay in sync with internal/discovery/discovery.go and internal/analyzers/analyzer.go. */
export const ALL_LANGUAGES = ['python', 'javascript', 'typescript', 'go', 'java', 'kotlin', 'csharp', 'c', 'cpp', 'ruby', 'php', 'rust', 'swift', 'scala', 'dart', 'elixir', 'haskell', 'clojure', 'erlang', 'fsharp', 'lua', 'zig', 'ocaml', 'perl', 'objective-c', 'vue', 'svelte', 'html', 'css', 'scss', 'json', 'yaml', 'toml', 'xml', 'sql', 'graphql', 'shell', 'powershell', 'batch', 'markdown', 'dockerfile', 'env'] as const;

export type SupportedLanguage = typeof ALL_LANGUAGES[number];

export const LANGUAGE_FAMILIES: Record<string, readonly string[]> = {
  Systems: ['c', 'cpp', 'rust', 'go', 'java', 'csharp', 'scala', 'kotlin', 'swift', 'objective-c', 'zig'] as const,
  Web: ['javascript', 'typescript', 'html', 'css', 'scss', 'vue', 'svelte', 'php', 'ruby', 'graphql'] as const,
  Mobile: ['swift', 'kotlin', 'dart', 'objective-c', 'java'] as const,
  Script: ['python', 'ruby', 'php', 'shell', 'elixir', 'powershell', 'batch', 'lua', 'perl'] as const,
  Data: ['json', 'yaml', 'toml', 'xml', 'sql', 'markdown', 'dockerfile', 'env'] as const,
  Functional: ['haskell', 'clojure', 'erlang', 'fsharp', 'elixir', 'ocaml', 'scala'] as const,
};

export const ANALYZER_CATALOG: AnalyzerMeta[] = [
  { id: 'ruff', displayName: 'Ruff', category: 'lint', languages: ['python'], severityDefault: 'medium', icon: 'Lint', docUrl: 'https://docs.astral.sh/ruff/', enabledByProfile: ['quick', 'standard', 'deep'], description: 'Fast Python linter and formatter.' },
  { id: 'biome', displayName: 'Biome', category: 'style', languages: ['javascript', 'typescript', 'jsx', 'tsx'], severityDefault: 'low', icon: 'Palette', docUrl: 'https://biomejs.dev/', enabledByProfile: ['quick', 'standard', 'deep'], description: 'JS/TS formatter and linter.' },
  { id: 'semgrep', displayName: 'Semgrep', category: 'security', languages: ['python', 'javascript', 'typescript', 'go', 'java', 'csharp', 'c', 'cpp', 'php', 'ruby', 'rust', 'swift', 'kotlin', 'scala', 'dart', 'elixir', 'objective-c', 'haskell', 'clojure', 'erlang', 'fsharp', 'lua', 'zig', 'ocaml', 'perl', 'shell', 'powershell', 'batch'], severityDefault: 'high', icon: 'Shield', docUrl: 'https://semgrep.dev/docs/', enabledByProfile: ['standard', 'deep'], description: 'Lightweight static analysis for security and correctness.' },
  { id: 'sonarqube', displayName: 'SonarQube', category: 'maintainability', languages: ['java', 'javascript', 'typescript', 'python', 'go', 'csharp', 'c', 'cpp', 'php', 'kotlin', 'swift', 'scala', 'dart', 'objective-c'], severityDefault: 'medium', icon: 'Gauge', docUrl: 'https://docs.sonarsource.com/sonarqube/', enabledByProfile: ['standard', 'deep'], description: 'Code quality and maintainability analysis.' },
  { id: 'secrets', displayName: 'Secrets', category: 'secrets', languages: [...ALL_LANGUAGES] as unknown as string[], severityDefault: 'critical', icon: 'KeyRound', docUrl: 'https://docs.bluntcode.local/analyzers/secrets', enabledByProfile: ['standard', 'deep'], description: 'Built-in credential and secret detection.' },
  { id: 'todo', displayName: 'Todo Scanner', category: 'maintainability', languages: [...ALL_LANGUAGES] as unknown as string[], severityDefault: 'info', icon: 'ListTodo', docUrl: 'https://docs.bluntcode.local/analyzers/todo', enabledByProfile: ['standard', 'deep'], description: 'Tracks TODO/FIXME and tech-debt markers.' },
];

const catalogMap = new Map(ANALYZER_CATALOG.map((m) => [m.id, m]));

export function analyzerMeta(id: string): AnalyzerMeta | undefined {
  return catalogMap.get(id);
}

export function categoryColor(category: AnalyzerCategory): string {
  // Categorical tokens only — no ad-hoc hex. Hues are spaced so adjacent
  // categories stay distinguishable at 8px dot size and after dark inversion.
  const map: Record<AnalyzerCategory, string> = {
    lint: 'var(--color-cat-1)',
    style: 'var(--color-cat-5)',
    security: 'var(--color-cat-4)',
    pentest: 'var(--color-cat-11)',
    secrets: 'var(--color-cat-9)',
    maintainability: 'var(--color-cat-2)',
    dependencies: 'var(--color-cat-10)',
    container: 'var(--color-cat-12)',
    iac: 'var(--color-cat-13)',
    license: 'var(--color-cat-8)',
    hacking: 'var(--color-cat-3)',
  };
  return map[category] ?? 'var(--color-ink-faint)';
}

export function allCategories(): AnalyzerCategory[] {
  return [...new Set(ANALYZER_CATALOG.map((a) => a.category))] as AnalyzerCategory[];
}

export const CATEGORY_LABELS: Record<AnalyzerCategory, string> = {
  lint: 'Lint',
  style: 'Style',
  security: 'Security',
  pentest: 'Pentest',
  secrets: 'Secrets',
  maintainability: 'Maintainability',
  dependencies: 'Dependencies',
  container: 'Container',
  iac: 'IaC',
  license: 'License',
  hacking: 'Hacking',
};

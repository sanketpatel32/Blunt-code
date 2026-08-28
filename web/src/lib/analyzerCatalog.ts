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
  { id: 'sonarqube', displayName: 'SonarQube', category: 'maintainability', languages: ['java', 'javascript', 'typescript', 'python', 'go', 'csharp', 'c', 'cpp', 'php', 'kotlin', 'swift', 'scala', 'dart', 'objective-c'], severityDefault: 'medium', icon: 'Gauge', docUrl: 'https://docs.sonarsource.com/sonarqube/', enabledByProfile: ['deep'], description: 'Code quality and maintainability analysis.' },
  { id: 'secrets', displayName: 'Secrets', category: 'secrets', languages: [...ALL_LANGUAGES] as unknown as string[], severityDefault: 'critical', icon: 'KeyRound', docUrl: 'https://docs.bluntcode.local/analyzers/secrets', enabledByProfile: ['quick', 'standard', 'deep'], description: 'Built-in credential and secret detection.' },
  { id: 'todo', displayName: 'Todo Scanner', category: 'maintainability', languages: [...ALL_LANGUAGES] as unknown as string[], severityDefault: 'info', icon: 'ListTodo', docUrl: 'https://docs.bluntcode.local/analyzers/todo', enabledByProfile: ['standard', 'deep'], description: 'Tracks TODO/FIXME and tech-debt markers.' },
  { id: 'snyk-oss', displayName: 'Snyk Open Source', category: 'dependencies', languages: ['javascript', 'typescript', 'python', 'go', 'java', 'csharp', 'c', 'cpp', 'php', 'ruby', 'rust', 'swift', 'kotlin', 'scala', 'dart', 'elixir'], severityDefault: 'high', icon: 'Package', docUrl: 'https://docs.snyk.io/', enabledByProfile: ['deep'], description: 'Open-source dependency vulnerability scanning.' },
  { id: 'dependabot-deps', displayName: 'Dependabot', category: 'dependencies', languages: ['javascript', 'typescript', 'python', 'go', 'java', 'csharp', 'c', 'cpp', 'php', 'rust', 'dart', 'elixir', 'ruby', 'swift', 'kotlin', 'scala'], severityDefault: 'medium', icon: 'GitBranch', docUrl: 'https://docs.github.com/en/code-security/dependabot', enabledByProfile: ['standard', 'deep'], description: 'Dependency freshness and known CVEs.' },
  { id: 'zap-pentest', displayName: 'OWASP ZAP', category: 'pentest', languages: ['javascript', 'typescript', 'python', 'java', 'php', 'go', 'ruby', 'c', 'cpp'], severityDefault: 'high', icon: 'Zap', docUrl: 'https://www.zaproxy.org/docs/', enabledByProfile: ['deep'], description: 'Dynamic application security testing (DAST).' },
  { id: 'burp-pentest', displayName: 'Burp Suite (stub)', category: 'pentest', languages: ['javascript', 'typescript', 'java', 'python', 'php', 'c', 'cpp', 'ruby'], severityDefault: 'high', icon: 'Crosshair', docUrl: 'https://portswigger.net/burp/documentation', enabledByProfile: ['deep'], description: 'Burp-guided pentest checks (stub — deep profile).' },
  { id: 'nuclei-pentest', displayName: 'Nuclei', category: 'pentest', languages: ['javascript', 'typescript', 'python', 'go', 'java', 'php', 'yaml', 'c', 'cpp', 'ruby'], severityDefault: 'high', icon: 'Radar', docUrl: 'https://docs.projectdiscovery.io/tools/nuclei/', enabledByProfile: ['deep'], description: 'Template-based vulnerability scanner.' },
  { id: 'gitleaks-secrets', displayName: 'Gitleaks', category: 'secrets', languages: ['python', 'javascript', 'typescript', 'go', 'java', 'csharp', 'c', 'cpp', 'php', 'rust', 'swift', 'kotlin', 'scala', 'dart', 'elixir', 'shell', 'yaml', 'json', 'env'], severityDefault: 'critical', icon: 'ScanSearch', docUrl: 'https://github.com/gitleaks/gitleaks', enabledByProfile: ['standard', 'deep'], description: 'Enhanced git-history secrets detection.' },
  { id: 'osv-dependencies', displayName: 'OSV Scanner', category: 'dependencies', languages: ['javascript', 'typescript', 'python', 'go', 'java', 'rust', 'php', 'c', 'cpp', 'ruby', 'dart', 'elixir', 'kotlin', 'scala'], severityDefault: 'high', icon: 'Database', docUrl: 'https://google.github.io/osv-scanner/', enabledByProfile: ['deep'], description: 'OSV database vulnerability scan.' },
  { id: 'container-trivy', displayName: 'Trivy (container)', category: 'container', languages: ['dockerfile', 'yaml', 'json'], severityDefault: 'high', icon: 'Container', docUrl: 'https://aquasecurity.github.io/trivy/', enabledByProfile: ['deep'], description: 'Container and IaC misconfiguration scan.' },
  { id: 'iac-checkov', displayName: 'Checkov (IaC)', category: 'iac', languages: ['terraform', 'yaml', 'json', 'dockerfile'], severityDefault: 'medium', icon: 'FileCog', docUrl: 'https://www.checkov.io/', enabledByProfile: ['deep'], description: 'Infrastructure-as-code policy checks.' },
  { id: 'license-scan', displayName: 'License Scan', category: 'license', languages: ['javascript', 'typescript', 'python', 'go', 'java', 'csharp', 'c', 'cpp', 'php', 'rust', 'ruby', 'swift', 'kotlin', 'scala', 'dart', 'elixir'], severityDefault: 'info', icon: 'Scale', docUrl: 'https://docs.bluntcode.local/analyzers/license', enabledByProfile: ['deep'], description: 'Open-source license compliance.' },
];

const catalogMap = new Map(ANALYZER_CATALOG.map((m) => [m.id, m]));

export function analyzerMeta(id: string): AnalyzerMeta | undefined {
  return catalogMap.get(id);
}

export function categoryColor(category: AnalyzerCategory): string {
  const map: Record<AnalyzerCategory, string> = {
    lint: 'var(--color-accent)',
    style: '#7c3aed',
    security: '#dc2626',
    pentest: '#ea580c',
    secrets: '#b45309',
    maintainability: '#059669',
    dependencies: '#2563eb',
    container: '#0e7490',
    iac: '#6d28d9',
    license: '#64748b',
    hacking: '#be123c',
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

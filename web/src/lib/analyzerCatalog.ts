export type AnalyzerCategory = 'lint' | 'style' | 'security' | 'pentest' | 'secrets' | 'maintainability' | 'dependencies' | 'container' | 'iac' | 'license';

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

export const ANALYZER_CATALOG: AnalyzerMeta[] = [
  { id: 'ruff', displayName: 'Ruff', category: 'lint', languages: ['python'], severityDefault: 'medium', icon: 'Lint', docUrl: 'https://docs.astral.sh/ruff/', enabledByProfile: ['quick', 'standard', 'deep'], description: 'Fast Python linter and formatter.' },
  { id: 'biome', displayName: 'Biome', category: 'style', languages: ['javascript', 'typescript', 'jsx', 'tsx'], severityDefault: 'low', icon: 'Palette', docUrl: 'https://biomejs.dev/', enabledByProfile: ['quick', 'standard', 'deep'], description: 'JS/TS formatter and linter.' },
  { id: 'semgrep', displayName: 'Semgrep', category: 'security', languages: ['python', 'javascript', 'typescript', 'go', 'java', 'csharp', 'php', 'ruby', 'rust', 'swift', 'kotlin'], severityDefault: 'high', icon: 'Shield', docUrl: 'https://semgrep.dev/docs/', enabledByProfile: ['standard', 'deep'], description: 'Lightweight static analysis for security and correctness.' },
  { id: 'sonarqube', displayName: 'SonarQube', category: 'maintainability', languages: ['java', 'javascript', 'typescript', 'python', 'go', 'csharp', 'php', 'kotlin', 'swift'], severityDefault: 'medium', icon: 'Gauge', docUrl: 'https://docs.sonarsource.com/sonarqube/', enabledByProfile: ['deep'], description: 'Code quality and maintainability analysis.' },
  { id: 'secrets', displayName: 'Secrets', category: 'secrets', languages: ['python', 'javascript', 'typescript', 'go', 'java', 'csharp', 'php', 'rust', 'swift', 'kotlin', 'shell', 'yaml', 'json', 'env'], severityDefault: 'critical', icon: 'KeyRound', docUrl: 'https://docs.bluntcode.local/analyzers/secrets', enabledByProfile: ['quick', 'standard', 'deep'], description: 'Built-in credential and secret detection.' },
  { id: 'todo', displayName: 'Todo Scanner', category: 'maintainability', languages: ['python', 'javascript', 'typescript', 'go', 'java', 'csharp', 'php', 'rust', 'swift', 'kotlin'], severityDefault: 'info', icon: 'ListTodo', docUrl: 'https://docs.bluntcode.local/analyzers/todo', enabledByProfile: ['standard', 'deep'], description: 'Tracks TODO/FIXME and tech-debt markers.' },
  // New analyzers — managed install placeholders (no backend logic yet)
  { id: 'snyk-oss', displayName: 'Snyk Open Source', category: 'dependencies', languages: ['javascript', 'typescript', 'python', 'go', 'java', 'csharp', 'php', 'ruby'], severityDefault: 'high', icon: 'Package', docUrl: 'https://docs.snyk.io/', enabledByProfile: ['deep'], description: 'Open-source dependency vulnerability scanning.' },
  { id: 'dependabot-deps', displayName: 'Dependabot', category: 'dependencies', languages: ['javascript', 'typescript', 'python', 'go', 'java', 'csharp', 'php', 'rust'], severityDefault: 'medium', icon: 'GitBranch', docUrl: 'https://docs.github.com/en/code-security/dependabot', enabledByProfile: ['standard', 'deep'], description: 'Dependency freshness and known CVEs.' },
  { id: 'zap-pentest', displayName: 'OWASP ZAP', category: 'pentest', languages: ['javascript', 'typescript', 'python', 'java', 'php', 'go'], severityDefault: 'high', icon: 'Zap', docUrl: 'https://www.zaproxy.org/docs/', enabledByProfile: ['deep'], description: 'Dynamic application security testing (DAST).' },
  { id: 'burp-pentest', displayName: 'Burp Suite (stub)', category: 'pentest', languages: ['javascript', 'typescript', 'java', 'python', 'php'], severityDefault: 'high', icon: 'Crosshair', docUrl: 'https://portswigger.net/burp/documentation', enabledByProfile: ['deep'], description: 'Burp-guided pentest checks (stub — deep profile).' },
  { id: 'nuclei-pentest', displayName: 'Nuclei', category: 'pentest', languages: ['javascript', 'typescript', 'python', 'go', 'java', 'php', 'yaml'], severityDefault: 'high', icon: 'Radar', docUrl: 'https://docs.projectdiscovery.io/tools/nuclei/', enabledByProfile: ['deep'], description: 'Template-based vulnerability scanner.' },
  { id: 'gitleaks-secrets', displayName: 'Gitleaks', category: 'secrets', languages: ['python', 'javascript', 'typescript', 'go', 'java', 'csharp', 'php', 'rust', 'swift', 'kotlin', 'shell', 'yaml'], severityDefault: 'critical', icon: 'ScanSearch', docUrl: 'https://github.com/gitleaks/gitleaks', enabledByProfile: ['standard', 'deep'], description: 'Enhanced git-history secrets detection.' },
  { id: 'osv-dependencies', displayName: 'OSV Scanner', category: 'dependencies', languages: ['javascript', 'typescript', 'python', 'go', 'java', 'rust', 'php'], severityDefault: 'high', icon: 'Database', docUrl: 'https://google.github.io/osv-scanner/', enabledByProfile: ['deep'], description: 'OSV database vulnerability scan.' },
  { id: 'container-trivy', displayName: 'Trivy (container)', category: 'container', languages: ['dockerfile', 'yaml', 'json'], severityDefault: 'high', icon: 'Container', docUrl: 'https://aquasecurity.github.io/trivy/', enabledByProfile: ['deep'], description: 'Container and IaC misconfiguration scan.' },
  { id: 'iac-checkov', displayName: 'Checkov (IaC)', category: 'iac', languages: ['terraform', 'yaml', 'json', 'dockerfile'], severityDefault: 'medium', icon: 'FileCog', docUrl: 'https://www.checkov.io/', enabledByProfile: ['deep'], description: 'Infrastructure-as-code policy checks.' },
  { id: 'license-scan', displayName: 'License Scan', category: 'license', languages: ['javascript', 'typescript', 'python', 'go', 'java', 'csharp', 'php', 'rust'], severityDefault: 'info', icon: 'Scale', docUrl: 'https://docs.bluntcode.local/analyzers/license', enabledByProfile: ['deep'], description: 'Open-source license compliance.' },
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
};

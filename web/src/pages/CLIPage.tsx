import { useMemo, useState } from 'react';
import { api } from '../api';
import { useLoad } from '../hooks/useLoad';
import { copyToClipboard } from '../lib/clipboard';
import { PageHeader } from '../components/PageHeader';
import { Button } from '../components/ui/button';
import {
  Terminal,
  Copy,
  Check,
  Search,
  Shield,
  Bot,
  Sliders,
  GitBranch,
} from 'lucide-react';

interface CLICommand {
  id: string;
  name: string;
  category: string;
  synopsis: string;
  description: string;
  badge?: string;
  flags: { flag: string; type: string; desc: string; def?: string }[];
  examples: { title: string; cmd: string }[];
  subcommands?: { name: string; desc: string; syntax: string }[];
}

const COMMANDS: CLICommand[] = [
  {
    id: 'scan',
    name: 'bluntcode scan',
    category: 'Scans & CI Gates',
    synopsis: 'bluntcode scan <path> [options]',
    description:
      'Runs an automated, headless quality and security scan on any folder. Tripping CI exit codes (0 = clean, 1 = gate tripped, 2 = flag error).',
    badge: 'Core / CI Gate',
    flags: [
      { flag: '--profile', type: 'string', desc: 'Scan depth: quick, standard, or deep', def: 'standard' },
      { flag: '--fail-on', type: 'severity+', desc: 'Exit 1 if findings remain at or above severity (e.g. high+, critical, medium+)' },
      { flag: '--max-findings', type: 'int', desc: 'Exit 1 if total findings exceed N' },
      { flag: '--baseline', type: 'id|sarif', desc: 'Baseline scan ID or SARIF file; only trip gate on NEW findings' },
      { flag: '--format', type: 'string', desc: 'Output format: text, json, github, sarif, csv, jsonl, markdown', def: 'text' },
      { flag: '--output', type: 'file', desc: 'Write document format to file instead of stdout' },
      { flag: '--incremental', type: 'bool', desc: 'Only scan files changed since previous completed scan' },
      { flag: '--jobs', type: 'int', desc: 'Run up to N analyzers concurrently', def: '4' },
      { flag: '--watch', type: 'bool', desc: 'Watch folder and automatically rescan when files change' },
      { flag: '--quiet', type: 'bool', desc: 'Suppress analyzer progress lines on stderr' },
      { flag: '--json', type: 'bool', desc: 'Print compact JSON summary block' },
    ],
    examples: [
      { title: 'Standard local scan', cmd: 'bluntcode scan .' },
      { title: 'Quick scan with JSON output', cmd: 'bluntcode scan . --profile quick --json' },
      { title: 'CI gate failing on high/critical issues', cmd: 'bluntcode scan . --fail-on high+ --format github' },
      { title: 'Export OASIS SARIF for GitHub Code Scanning', cmd: 'bluntcode scan . --format sarif --output audit.sarif' },
      { title: 'Compare against baseline in pull request', cmd: 'bluntcode scan . --baseline baseline.sarif --fail-on high+' },
    ],
  },
  {
    id: 'workspace',
    name: 'bluntcode workspace',
    category: 'Workspaces',
    synopsis: 'bluntcode workspace <list|add|show|tree|tags|delete> [args]',
    description: 'Manage registered workspaces, project metadata, directory trees, and tags.',
    badge: 'Management',
    subcommands: [
      { name: 'list', desc: 'List all registered workspaces with scan metrics', syntax: 'bluntcode workspace list [--json]' },
      { name: 'add', desc: 'Register a new workspace directory', syntax: 'bluntcode workspace add <path> [--name <name>] [--profile quick|standard|deep] [--json]' },
      { name: 'show', desc: 'Display workspace metadata, last scan, and tags', syntax: 'bluntcode workspace show <id|path> [--json]' },
      { name: 'tree', desc: 'View file structure, sizes, and excluded paths', syntax: 'bluntcode workspace tree <id|path> [--path <subpath>] [--json]' },
      { name: 'tags', desc: 'View or set workspace tags', syntax: 'bluntcode workspace tags <id|path> [--set "tag1,tag2"] [--json]' },
      { name: 'delete', desc: 'Remove a workspace registration', syntax: 'bluntcode workspace delete <id|path> [--json]' },
    ],
    flags: [
      { flag: '--json', type: 'bool', desc: 'Output structured JSON payload' },
      { flag: '--name', type: 'string', desc: 'Custom display name for workspace' },
      { flag: '--profile', type: 'string', desc: 'Default scan profile (quick, standard, deep)' },
      { flag: '--set', type: 'string', desc: 'Comma-separated tags to assign' },
      { flag: '--path', type: 'string', desc: 'Subpath within workspace for tree inspection' },
    ],
    examples: [
      { title: 'Register current directory', cmd: 'bluntcode workspace add . --name my-api' },
      { title: 'List all workspaces as JSON', cmd: 'bluntcode workspace list --json' },
      { title: 'Inspect workspace details', cmd: 'bluntcode workspace show my-api' },
      { title: 'Assign tags', cmd: 'bluntcode workspace tags my-api --set "backend,security,go"' },
      { title: 'Inspect source tree', cmd: 'bluntcode workspace tree my-api --path src' },
    ],
  },
  {
    id: 'findings',
    name: 'bluntcode findings',
    category: 'Findings & Code',
    synopsis: 'bluntcode findings <search|list|preview> [args]',
    description: 'Search, filter, and inspect code findings across all scans with line-level source excerpts.',
    badge: 'Code Inspection',
    subcommands: [
      { name: 'search', desc: 'Global text search across finding messages and rules', syntax: 'bluntcode findings search <query> [--workspace <id>] [--severity <sev>] [--limit N] [--json]' },
      { name: 'list', desc: 'List findings for a scan or workspace', syntax: 'bluntcode findings list <scan-id|path> [--severity <sev>] [--format text|json|jsonl|csv] [--output <file>]' },
      { name: 'preview', desc: 'Show source code excerpt with line numbers and caret', syntax: 'bluntcode findings preview <scan-id> <finding-id> [--lines N] [--json]' },
    ],
    flags: [
      { flag: '--severity', type: 'string', desc: 'Filter severities (e.g. critical, high+, medium)' },
      { flag: '--analyzer', type: 'string', desc: 'Filter by analyzer ID (e.g. semgrep, ruff, secrets)' },
      { flag: '--format', type: 'string', desc: 'Output format: text, json, jsonl, csv', def: 'text' },
      { flag: '--output', type: 'file', desc: 'Write output to file path' },
      { flag: '--lines', type: 'int', desc: 'Context lines around snippet for preview', def: '5' },
    ],
    examples: [
      { title: 'Search for secrets across all projects', cmd: 'bluntcode findings search "AWS" --severity critical' },
      { title: 'Export findings as CSV spreadsheet', cmd: 'bluntcode findings list . --format csv --output issues.csv' },
      { title: 'Output newline-delimited JSON for jq pipelines', cmd: 'bluntcode findings list . --format jsonl' },
      { title: 'Inspect finding snippet with line numbers', cmd: 'bluntcode findings preview <scan-id> <finding-id> --lines 8' },
    ],
  },
  {
    id: 'report',
    name: 'bluntcode report',
    category: 'Reports & Exports',
    synopsis: 'bluntcode report <scan-id|workspace-path> [--format md|sarif|html|json|csv|jsonl] [--output <file>]',
    description:
      'Exports a full audit report for any scan in standard formats, including SonarQube-style Markdown, self-contained HTML, and OASIS SARIF.',
    badge: 'Exporters',
    flags: [
      { flag: '--format', type: 'string', desc: 'Format: md (markdown), sarif, html, json, csv, jsonl', def: 'md' },
      { flag: '--output', type: 'file', desc: 'Write report to destination file instead of stdout' },
      { flag: '--json', type: 'bool', desc: 'Alias for --format json' },
    ],
    examples: [
      { title: 'Generate Markdown audit report to stdout', cmd: 'bluntcode report .' },
      { title: 'Export interactive HTML report', cmd: 'bluntcode report . --format html --output audit-report.html' },
      { title: 'Generate SARIF log for CI artifact', cmd: 'bluntcode report . --format sarif --output results.sarif' },
      { title: 'Export finding records as CSV', cmd: 'bluntcode report <scan-id> --format csv --output scan-findings.csv' },
    ],
  },
  {
    id: 'history',
    name: 'bluntcode history',
    category: 'History & Compare',
    synopsis: 'bluntcode history [path] | bluntcode history compare <id1> <id2> | bluntcode history delete <id>',
    description: 'Trace historical scan runs, compare two scans for regressions, and clean up past records.',
    badge: 'Audit Trail',
    subcommands: [
      { name: 'list', desc: 'List historical scans with durations and finding counts', syntax: 'bluntcode history [workspace-id|path] [--limit N] [--json]' },
      { name: 'compare', desc: 'Diff two scans to show new, fixed, and persistent findings', syntax: 'bluntcode history compare <scan1> <scan2> [--json]' },
      { name: 'delete', desc: 'Permanently remove a scan record', syntax: 'bluntcode history delete <scan-id> [--json]' },
    ],
    flags: [
      { flag: '--limit', type: 'int', desc: 'Maximum number of scans to display (1-100)', def: '20' },
      { flag: '--json', type: 'bool', desc: 'Output structured JSON payload' },
    ],
    examples: [
      { title: 'View recent scans for current workspace', cmd: 'bluntcode history . --limit 10' },
      { title: 'Compare baseline and PR scans', cmd: 'bluntcode history compare <scan-1-id> <scan-2-id>' },
      { title: 'Prune historical scans older than retention', cmd: 'bluntcode prune . --keep 10' },
    ],
  },
  {
    id: 'suppress',
    name: 'bluntcode suppress',
    category: 'Rules & Suppressions',
    synopsis: 'bluntcode suppress <list|add|remove|import> <workspace> [args]',
    description:
      'Manage dismissed findings and false positives. Suppressed findings are excluded from scan totals, reports, and CI gates.',
    badge: 'Triaging',
    subcommands: [
      { name: 'list', desc: 'List active suppressions for a workspace', syntax: 'bluntcode suppress list <workspace> [--json|--csv]' },
      { name: 'add', desc: 'Dismiss a finding fingerprint forever', syntax: 'bluntcode suppress add <workspace> --fingerprint <hash> [--reason <text>]' },
      { name: 'remove', desc: 'Unsuppress a previously dismissed fingerprint', syntax: 'bluntcode suppress remove <workspace> --fingerprint <hash>' },
      { name: 'import', desc: 'Batch import suppressions from a CSV file', syntax: 'bluntcode suppress import <workspace> <csv-file>' },
    ],
    flags: [
      { flag: '--fingerprint', type: 'string', desc: 'Unique SHA-256 finding fingerprint hash' },
      { flag: '--reason', type: 'string', desc: 'Justification for suppression (e.g. "Dev-only fixture")' },
      { flag: '--json', type: 'bool', desc: 'Output JSON payload' },
      { flag: '--csv', type: 'bool', desc: 'Output CSV spreadsheet format' },
    ],
    examples: [
      { title: 'List suppressions for workspace', cmd: 'bluntcode suppress list .' },
      { title: 'Suppress false positive', cmd: 'bluntcode suppress add . --fingerprint a7f3b89... --reason "Mock secret in tests"' },
      { title: 'Re-enable finding', cmd: 'bluntcode suppress remove . --fingerprint a7f3b89...' },
      { title: 'Import suppressions from CSV', cmd: 'bluntcode suppress import . team-suppressions.csv' },
    ],
  },
  {
    id: 'rules',
    name: 'bluntcode rules',
    category: 'Rules & Suppressions',
    synopsis: 'bluntcode rules <list|disable|enable|overrides> <workspace> [args]',
    description: 'Configure workspace file exclude overrides and enable or disable specific analyzer rules.',
    badge: 'Configuration',
    subcommands: [
      { name: 'list', desc: 'List configured rules and path exclusion overrides', syntax: 'bluntcode rules list <workspace> [--json]' },
      { name: 'disable', desc: 'Disable a specific rule pattern', syntax: 'bluntcode rules disable <workspace> <pattern>' },
      { name: 'enable', desc: 'Enable a specific rule pattern', syntax: 'bluntcode rules enable <workspace> <pattern>' },
      { name: 'overrides', desc: 'View or set path exclusion overrides', syntax: 'bluntcode rules overrides <workspace> [--set "path1,path2"] [--json]' },
    ],
    flags: [
      { flag: '--set', type: 'string', desc: 'Comma-separated glob paths to exclude from analysis' },
      { flag: '--json', type: 'bool', desc: 'Output JSON payload' },
    ],
    examples: [
      { title: 'View workspace rules and overrides', cmd: 'bluntcode rules list .' },
      { title: 'Exclude build directories from scans', cmd: 'bluntcode rules overrides . --set "dist/**,build/**,vendor/**"' },
      { title: 'Disable rule in test files', cmd: 'bluntcode rules disable . "tests/**"' },
    ],
  },
  {
    id: 'tools',
    name: 'bluntcode tools',
    category: 'Managed Analyzers',
    synopsis: 'bluntcode tools <list|install|repair|update> [tool-id]',
    description:
      'Inspect, verify, download, and repair local hermetic analyzer binaries (Ruff, Biome, Semgrep, Gitleaks, Checkov, Trivy, SonarQube).',
    badge: 'Toolchain',
    subcommands: [
      { name: 'list', desc: 'List all managed tools, versions, and readiness', syntax: 'bluntcode tools list [--json]' },
      { name: 'install', desc: 'Download and verify a hermetic analyzer binary', syntax: 'bluntcode tools install <tool-id>' },
      { name: 'repair', desc: 'Reinstall and verify analyzer binary', syntax: 'bluntcode tools repair <tool-id>' },
      { name: 'update', desc: 'Update analyzer to latest manifest version', syntax: 'bluntcode tools update <tool-id>' },
    ],
    flags: [
      { flag: '--json', type: 'bool', desc: 'Output JSON status array' },
    ],
    examples: [
      { title: 'Inspect toolchain readiness', cmd: 'bluntcode tools list' },
      { title: 'Download Semgrep analyzer', cmd: 'bluntcode tools install semgrep' },
      { title: 'Repair Gitleaks binary', cmd: 'bluntcode tools repair gitleaks' },
      { title: 'Output tools status as JSON for script', cmd: 'bluntcode tools list --json' },
    ],
  },
  {
    id: 'pentest',
    name: 'bluntcode pentest',
    category: 'Dynamic Pentest & DAST',
    synopsis: 'bluntcode pentest probe <url> [options]',
    description:
      'Performs dynamic DAST probing against any HTTP endpoint. Audits security headers, CORS misconfigurations, TLS configuration, info disclosure, and security grade.',
    badge: 'DAST Security',
    flags: [
      { flag: '--scope', type: 'string', desc: 'Probe depth: standard, full, or spider', def: 'standard' },
      { flag: '--auth-mode', type: 'string', desc: 'Auth mode: bearer, basic, or cookie' },
      { flag: '--auth-token', type: 'string', desc: 'Auth token credentials or cookie string' },
      { flag: '--json', type: 'bool', desc: 'Output complete structured JSON audit results' },
    ],
    examples: [
      { title: 'Probe local development server', cmd: 'bluntcode pentest probe http://localhost:3000' },
      { title: 'Full security probe on staging API', cmd: 'bluntcode pentest probe https://api.staging.internal --scope full' },
      { title: 'Probe authenticated endpoint with Bearer token', cmd: 'bluntcode pentest probe http://localhost:8080 --auth-mode bearer --auth-token "eyJh..."' },
      { title: 'Output JSON for automated security gating', cmd: 'bluntcode pentest probe http://localhost:3000 --json' },
    ],
  },
  {
    id: 'stats',
    name: 'bluntcode stats, trends, risk',
    category: 'Stats & Risk Metrics',
    synopsis: 'bluntcode stats [workspace] | bluntcode trends <workspace> | bluntcode risk <workspace>',
    description: 'Calculates global and per-workspace risk scores (0-100), risk grades (A-D), and historical severity trends.',
    badge: 'Metrics & Health',
    flags: [
      { flag: '--limit', type: 'int', desc: 'Number of scans to trace for trends (1-100)', def: '10' },
      { flag: '--json', type: 'bool', desc: 'Output machine-readable JSON metrics' },
    ],
    examples: [
      { title: 'Global workspace & scan statistics', cmd: 'bluntcode stats' },
      { title: 'Workspace-specific scan summary', cmd: 'bluntcode stats .' },
      { title: 'Risk score and grade assessment', cmd: 'bluntcode risk .' },
      { title: 'Severity trendline across recent scans', cmd: 'bluntcode trends . --limit 15' },
    ],
  },
  {
    id: 'doctor',
    name: 'bluntcode doctor, config, update',
    category: 'Diagnostics & System',
    synopsis: 'bluntcode doctor [--fix] [--json] | bluntcode config | bluntcode update check',
    description: 'Diagnoses system health, checks tools integrity, repairs missing folders, and checks for updates.',
    badge: 'Diagnostics',
    flags: [
      { flag: '--fix', type: 'bool', desc: 'Automatically repair missing folders and recreate corrupted databases' },
      { flag: '--json', type: 'bool', desc: 'Output structured diagnostic JSON' },
    ],
    examples: [
      { title: 'Run environment health check', cmd: 'bluntcode doctor' },
      { title: 'Automatically fix configuration issues', cmd: 'bluntcode doctor --fix' },
      { title: 'Print resolved data directories and ports', cmd: 'bluntcode config' },
      { title: 'Check if a newer version of Blunt Code is available', cmd: 'bluntcode update check' },
    ],
  },
  {
    id: 'agent',
    name: 'bluntcode agent, llm',
    category: 'AI Agents & Automation',
    synopsis: 'bluntcode agent docs | bluntcode agent scan <path> | bluntcode llm',
    description:
      'Optimized entrypoints for AI coding agents (Claude, Gemini, Cursor, Antigravity). Delivers clean JSON outputs and developer documentation.',
    badge: 'AI Agents',
    flags: [
      { flag: '--json', type: 'bool', desc: 'Machine-readable output (default for agent scan)' },
      { flag: '--quiet', type: 'bool', desc: 'Suppress interactive output (default for agent scan)' },
    ],
    examples: [
      { title: 'Print agent instructions (llm.txt)', cmd: 'bluntcode agent docs' },
      { title: 'Agent scan with machine-readable defaults', cmd: 'bluntcode agent scan .' },
      { title: 'Print llms.txt standard index', cmd: 'bluntcode llm' },
    ],
  },
];

const CI_WORKFLOW_GITHUB = `name: Blunt Code Security & Quality Gate

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  bluntcode:
    name: Local Static Analysis
    runs-on: windows-latest

    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Install Blunt Code
        run: |
          Invoke-WebRequest -Uri "https://github.com/sanketpatel32/Blunt-code/releases/latest/download/install-latest.ps1" -OutFile install.ps1
          .\install.ps1 -SkipBrowser
          echo "$env:LOCALAPPDATA\Programs\BluntCode" >> $env:GITHUB_PATH

      - name: Run Blunt Code Quality & Security Scan
        run: |
          bluntcode scan . \
            --profile standard \
            --fail-on high+ \
            --format sarif \
            --output bluntcode-results.sarif

      - name: Upload SARIF to GitHub Code Scanning
        uses: github/codeql-action/upload-sarif@v3
        if: always()
        with:
          sarif_file: bluntcode-results.sarif
`;

const CI_WORKFLOW_GITLAB = `stages:
  - test

bluntcode_audit:
  stage: test
  tags:
    - windows
  script:
    - bluntcode scan . --profile standard --fail-on high+ --format json --output report.json
  artifacts:
    when: always
    paths:
      - report.json
    reports:
      sast: report.json
`;

const PRE_COMMIT_HOOK = `#!/bin/sh
# .git/hooks/pre-commit
# Prevent committing code with Critical or High severity security vulnerabilities

echo "Running Blunt Code pre-commit quality check..."
bluntcode scan . --profile quick --fail-on high+ --quiet

if [ $? -ne 0 ]; then
  echo ""
  echo "❌ Blunt Code gate failed: Critical or High findings detected."
  echo "Run 'bluntcode scan .' or 'bluntcode' GUI to review and fix issues."
  exit 1
fi

echo "✅ Blunt Code passed."
exit 0
`;

export function CLIPage() {
  const meta = useLoad(api.meta, []);
  const [tab, setTab] = useState<'reference' | 'ci' | 'builder'>('reference');
  const [search, setSearch] = useState('');
  const [selectedCategory, setSelectedCategory] = useState<string>('All');
  const [copiedText, setCopiedText] = useState<string | null>(null);

  // Command Builder State
  const [builderTarget, setBuilderTarget] = useState('.');
  const [builderProfile, setBuilderProfile] = useState('standard');
  const [builderFailOn, setBuilderFailOn] = useState('high+');
  const [builderFormat, setBuilderFormat] = useState('sarif');
  const [builderOutput, setBuilderOutput] = useState('audit.sarif');
  const [builderQuiet, setBuilderQuiet] = useState(false);
  const [builderIncremental, setBuilderIncremental] = useState(false);

  const categories = useMemo(() => {
    const set = new Set<string>();
    COMMANDS.forEach((c) => set.add(c.category));
    return ['All', ...Array.from(set)];
  }, []);

  const filteredCommands = useMemo(() => {
    return COMMANDS.filter((cmd) => {
      const matchCat = selectedCategory === 'All' || cmd.category === selectedCategory;
      if (!matchCat) return false;
      if (!search.trim()) return true;
      const q = search.toLowerCase();
      return (
        cmd.name.toLowerCase().includes(q) ||
        cmd.description.toLowerCase().includes(q) ||
        cmd.synopsis.toLowerCase().includes(q) ||
        cmd.flags.some((f) => f.flag.toLowerCase().includes(q) || f.desc.toLowerCase().includes(q)) ||
        cmd.examples.some((e) => e.cmd.toLowerCase().includes(q) || e.title.toLowerCase().includes(q))
      );
    });
  }, [selectedCategory, search]);

  const copy = async (text: string) => {
    if (await copyToClipboard(text)) {
      setCopiedText(text);
      setTimeout(() => setCopiedText(null), 2000);
    }
  };

  const generatedBuilderCommand = useMemo(() => {
    const parts = ['bluntcode scan', builderTarget || '.'];
    if (builderProfile !== 'standard') parts.push(`--profile ${builderProfile}`);
    if (builderFailOn !== 'none') parts.push(`--fail-on ${builderFailOn}`);
    if (builderFormat !== 'text') parts.push(`--format ${builderFormat}`);
    if (builderOutput.trim()) parts.push(`--output ${builderOutput.trim()}`);
    if (builderIncremental) parts.push('--incremental');
    if (builderQuiet) parts.push('--quiet');
    return parts.join(' ');
  }, [builderTarget, builderProfile, builderFailOn, builderFormat, builderOutput, builderIncremental, builderQuiet]);

  return (
    <div className="page wide cli-page">
      <PageHeader
        eyebrow="Developer Reference"
        title="Command-Line Interface (CLI)"
        description="Every feature in Blunt Code is fully accessible from your terminal, CI/CD pipeline, pre-commit hook, or AI assistant."
      />

      {/* Hero Overview Card */}
      <section className="about-card" style={{ marginBottom: 'var(--space-md)' }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 'var(--space-sm)' }}>
          <div>
            <h2 style={{ margin: 0, display: 'flex', alignItems: 'center', gap: '8px' }}>
              <Terminal size={20} />
              <code>bluntcode</code> CLI {meta.data?.version && <span className="badge">v{meta.data.version}</span>}
            </h2>
            <p style={{ margin: '6px 0 0', color: 'var(--text-secondary)' }}>
              Headless code security analysis, vulnerability probing, and project metrics for Windows.
            </p>
          </div>
          <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
            <span className="badge" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
              <Shield size={13} /> 100% Local & Hermetic
            </span>
            <span className="badge" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
              <GitBranch size={13} /> CI Exit Codes
            </span>
            <span className="badge" style={{ display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
              <Bot size={13} /> Agent Ready
            </span>
          </div>
        </div>

        {/* Global CLI quick facts */}
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: '12px', marginTop: '16px' }}>
          <div style={{ padding: '12px', background: 'var(--bg-subtle, rgba(0,0,0,0.03))', borderRadius: '8px', border: '1px solid var(--border-subtle, rgba(0,0,0,0.08))' }}>
            <div style={{ fontWeight: 600, fontSize: '13px', color: 'var(--text-primary)' }}>Concurrent CLI & GUI</div>
            <div style={{ fontSize: '12px', color: 'var(--text-secondary)', marginTop: '4px' }}>
              Queries run safely without blocking or needing the single-instance mutex. Run CLI commands even while the GUI is open.
            </div>
          </div>
          <div style={{ padding: '12px', background: 'var(--bg-subtle, rgba(0,0,0,0.03))', borderRadius: '8px', border: '1px solid var(--border-subtle, rgba(0,0,0,0.08))' }}>
            <div style={{ fontWeight: 600, fontSize: '13px', color: 'var(--text-primary)' }}>Standard Exit Codes</div>
            <div style={{ fontSize: '12px', color: 'var(--text-secondary)', marginTop: '4px' }}>
              <code>0</code> = Clean / success, <code>1</code> = Gate tripped or issues found, <code>2</code> = Flag syntax or usage error.
            </div>
          </div>
          <div style={{ padding: '12px', background: 'var(--bg-subtle, rgba(0,0,0,0.03))', borderRadius: '8px', border: '1px solid var(--border-subtle, rgba(0,0,0,0.08))' }}>
            <div style={{ fontWeight: 600, fontSize: '13px', color: 'var(--text-primary)' }}>Built-in Manual</div>
            <div style={{ fontSize: '12px', color: 'var(--text-secondary)', marginTop: '4px' }}>
              Run <code>bluntcode cli &lt;command&gt;</code> in terminal for comprehensive in-terminal command documentation and examples.
            </div>
          </div>
        </div>
      </section>

      {/* Navigation Tabs */}
      <div style={{ display: 'flex', gap: '8px', marginBottom: '20px', borderBottom: '1px solid var(--border-subtle, rgba(0,0,0,0.1))', paddingBottom: '8px' }}>
        <button
          className={tab === 'reference' ? 'tab active' : 'tab'}
          onClick={() => setTab('reference')}
          style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', padding: '8px 16px', borderRadius: '6px', border: 'none', background: tab === 'reference' ? 'var(--accent-primary, #0969da)' : 'transparent', color: tab === 'reference' ? '#fff' : 'inherit', cursor: 'pointer', fontWeight: 600 }}
        >
          <Terminal size={16} /> Reference Manual
        </button>
        <button
          className={tab === 'ci' ? 'tab active' : 'tab'}
          onClick={() => setTab('ci')}
          style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', padding: '8px 16px', borderRadius: '6px', border: 'none', background: tab === 'ci' ? 'var(--accent-primary, #0969da)' : 'transparent', color: tab === 'ci' ? '#fff' : 'inherit', cursor: 'pointer', fontWeight: 600 }}
        >
          <GitBranch size={16} /> CI/CD & Automation
        </button>
        <button
          className={tab === 'builder' ? 'tab active' : 'tab'}
          onClick={() => setTab('builder')}
          style={{ display: 'inline-flex', alignItems: 'center', gap: '6px', padding: '8px 16px', borderRadius: '6px', border: 'none', background: tab === 'builder' ? 'var(--accent-primary, #0969da)' : 'transparent', color: tab === 'builder' ? '#fff' : 'inherit', cursor: 'pointer', fontWeight: 600 }}
        >
          <Sliders size={16} /> Command Builder
        </button>
      </div>

      {/* TAB 1: Reference Manual */}
      {tab === 'reference' && (
        <div>
          {/* Filter Bar */}
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: '12px', alignItems: 'center', marginBottom: '20px' }}>
            <div style={{ position: 'relative', flex: '1 1 240px' }}>
              <Search size={16} style={{ position: 'absolute', left: '12px', top: '50%', transform: 'translateY(-50%)', opacity: 0.5 }} />
              <input
                type="text"
                placeholder="Search commands, flags, or recipes..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                style={{ width: '100%', padding: '8px 12px 8px 36px', borderRadius: '6px', border: '1px solid var(--border-subtle, #ccc)', background: 'var(--bg-input, #fff)', color: 'inherit' }}
              />
            </div>
            <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap' }}>
              {categories.map((cat) => (
                <button
                  key={cat}
                  onClick={() => setSelectedCategory(cat)}
                  style={{
                    padding: '6px 12px',
                    borderRadius: '16px',
                    fontSize: '12px',
                    fontWeight: 500,
                    border: '1px solid var(--border-subtle, rgba(0,0,0,0.15))',
                    background: selectedCategory === cat ? 'var(--accent-primary, #0969da)' : 'var(--bg-surface, transparent)',
                    color: selectedCategory === cat ? '#fff' : 'inherit',
                    cursor: 'pointer',
                  }}
                >
                  {cat}
                </button>
              ))}
            </div>
          </div>

          {/* Commands List */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
            {filteredCommands.length === 0 ? (
              <div style={{ textAlign: 'center', padding: '40px', color: 'var(--text-secondary)' }}>
                No commands match your query "{search}".
              </div>
            ) : (
              filteredCommands.map((cmd) => (
                <div
                  key={cmd.id}
                  style={{
                    padding: '20px',
                    borderRadius: '8px',
                    background: 'var(--bg-surface, #fff)',
                    border: '1px solid var(--border-subtle, rgba(0,0,0,0.1))',
                    boxShadow: '0 1px 3px rgba(0,0,0,0.05)',
                  }}
                >
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', flexWrap: 'wrap', gap: '8px' }}>
                    <div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                        <h3 style={{ margin: 0, fontFamily: 'var(--font-mono, monospace)', fontSize: '17px' }}>{cmd.name}</h3>
                        {cmd.badge && <span className="badge">{cmd.badge}</span>}
                        <span style={{ fontSize: '12px', color: 'var(--text-secondary)' }}>{cmd.category}</span>
                      </div>
                      <p style={{ margin: '8px 0 12px', color: 'var(--text-secondary)' }}>{cmd.description}</p>
                    </div>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => copy(cmd.synopsis)}
                      title="Copy synopsis"
                      style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}
                    >
                      {copiedText === cmd.synopsis ? <Check size={14} color="#10b981" /> : <Copy size={14} />}
                      {copiedText === cmd.synopsis ? 'Copied' : 'Copy'}
                    </Button>
                  </div>

                  {/* Synopsis Box */}
                  <div
                    style={{
                      background: 'var(--bg-code, #1e1e1e)',
                      color: 'var(--text-code, #f3f4f6)',
                      padding: '10px 14px',
                      borderRadius: '6px',
                      fontFamily: 'monospace',
                      fontSize: '13px',
                      overflowX: 'auto',
                      marginBottom: '16px',
                    }}
                  >
                    <code>{cmd.synopsis}</code>
                  </div>

                  {/* Subcommands if any */}
                  {cmd.subcommands && cmd.subcommands.length > 0 && (
                    <div style={{ marginBottom: '16px' }}>
                      <div style={{ fontWeight: 600, fontSize: '13px', marginBottom: '8px', color: 'var(--text-primary)' }}>Subcommands:</div>
                      <div style={{ display: 'grid', gap: '6px' }}>
                        {cmd.subcommands.map((sub) => (
                          <div
                            key={sub.name}
                            style={{
                              padding: '8px 12px',
                              borderRadius: '6px',
                              background: 'var(--bg-subtle, rgba(0,0,0,0.02))',
                              border: '1px solid var(--border-subtle, rgba(0,0,0,0.06))',
                              display: 'flex',
                              justifyContent: 'space-between',
                              alignItems: 'center',
                              flexWrap: 'wrap',
                              gap: '8px',
                            }}
                          >
                            <div>
                              <strong style={{ fontFamily: 'monospace' }}>{sub.name}</strong> &mdash;{' '}
                              <span style={{ fontSize: '13px', color: 'var(--text-secondary)' }}>{sub.desc}</span>
                              <div style={{ fontSize: '12px', fontFamily: 'monospace', color: 'var(--text-secondary)', marginTop: '2px' }}>
                                {sub.syntax}
                              </div>
                            </div>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => copy(sub.syntax)}
                              title="Copy command"
                            >
                              {copiedText === sub.syntax ? <Check size={12} color="#10b981" /> : <Copy size={12} />}
                            </Button>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Flag Options Table */}
                  {cmd.flags && cmd.flags.length > 0 && (
                    <div style={{ marginBottom: '16px' }}>
                      <div style={{ fontWeight: 600, fontSize: '13px', marginBottom: '8px', color: 'var(--text-primary)' }}>Options & Flags:</div>
                      <div style={{ overflowX: 'auto' }}>
                        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: '12px', textAlign: 'left' }}>
                          <thead>
                            <tr style={{ borderBottom: '1px solid var(--border-subtle, rgba(0,0,0,0.1))', color: 'var(--text-secondary)' }}>
                              <th style={{ padding: '6px 8px' }}>Flag</th>
                              <th style={{ padding: '6px 8px' }}>Type</th>
                              <th style={{ padding: '6px 8px' }}>Description</th>
                              <th style={{ padding: '6px 8px' }}>Default</th>
                            </tr>
                          </thead>
                          <tbody>
                            {cmd.flags.map((f) => (
                              <tr key={f.flag} style={{ borderBottom: '1px solid var(--border-subtle, rgba(0,0,0,0.05))' }}>
                                <td style={{ padding: '6px 8px', fontFamily: 'monospace', fontWeight: 600 }}>{f.flag}</td>
                                <td style={{ padding: '6px 8px', color: 'var(--text-secondary)' }}>{f.type}</td>
                                <td style={{ padding: '6px 8px' }}>{f.desc}</td>
                                <td style={{ padding: '6px 8px', color: 'var(--text-secondary)', fontFamily: 'monospace' }}>{f.def || '-'}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  )}

                  {/* Examples */}
                  <div>
                    <div style={{ fontWeight: 600, fontSize: '13px', marginBottom: '8px', color: 'var(--text-primary)' }}>Practical Examples:</div>
                    <div style={{ display: 'grid', gap: '6px' }}>
                      {cmd.examples.map((ex, i) => (
                        <div
                          key={i}
                          style={{
                            display: 'flex',
                            justifyContent: 'space-between',
                            alignItems: 'center',
                            background: 'var(--bg-subtle, rgba(0,0,0,0.03))',
                            padding: '8px 12px',
                            borderRadius: '6px',
                            gap: '8px',
                          }}
                        >
                          <div>
                            <div style={{ fontSize: '12px', fontWeight: 500, color: 'var(--text-secondary)' }}>{ex.title}</div>
                            <div style={{ fontFamily: 'monospace', fontSize: '12px', color: 'var(--text-primary)' }}>{ex.cmd}</div>
                          </div>
                          <Button variant="ghost" size="sm" onClick={() => copy(ex.cmd)} title="Copy example">
                            {copiedText === ex.cmd ? <Check size={14} color="#10b981" /> : <Copy size={14} />}
                          </Button>
                        </div>
                      ))}
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {/* TAB 2: CI/CD & Automation Recipes */}
      {tab === 'ci' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
          {/* GitHub Actions Card */}
          <div style={{ padding: '20px', borderRadius: '8px', background: 'var(--bg-surface, #fff)', border: '1px solid var(--border-subtle, rgba(0,0,0,0.1))' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
              <div>
                <h3 style={{ margin: 0 }}>GitHub Actions with SARIF Code Scanning</h3>
                <p style={{ margin: '4px 0 0', color: 'var(--text-secondary)', fontSize: '13px' }}>
                  Runs on pull requests and pushes, trips the build on high/critical findings, and uploads OASIS SARIF alerts to GitHub Security tab.
                </p>
              </div>
              <Button variant="outline" size="sm" onClick={() => copy(CI_WORKFLOW_GITHUB)}>
                {copiedText === CI_WORKFLOW_GITHUB ? <Check size={14} color="#10b981" /> : <Copy size={14} />}
                {copiedText === CI_WORKFLOW_GITHUB ? 'Copied' : 'Copy YAML'}
              </Button>
            </div>
            <pre
              style={{
                background: 'var(--bg-code, #1e1e1e)',
                color: 'var(--text-code, #f3f4f6)',
                padding: '14px',
                borderRadius: '6px',
                fontSize: '12px',
                overflowX: 'auto',
                lineHeight: 1.5,
              }}
            >
              <code>{CI_WORKFLOW_GITHUB}</code>
            </pre>
          </div>

          {/* Pre-commit Hook Card */}
          <div style={{ padding: '20px', borderRadius: '8px', background: 'var(--bg-surface, #fff)', border: '1px solid var(--border-subtle, rgba(0,0,0,0.1))' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
              <div>
                <h3 style={{ margin: 0 }}>Git Pre-Commit Hook</h3>
                <p style={{ margin: '4px 0 0', color: 'var(--text-secondary)', fontSize: '13px' }}>
                  Save to <code>.git/hooks/pre-commit</code> to prevent committing vulnerable code or hardcoded secrets locally.
                </p>
              </div>
              <Button variant="outline" size="sm" onClick={() => copy(PRE_COMMIT_HOOK)}>
                {copiedText === PRE_COMMIT_HOOK ? <Check size={14} color="#10b981" /> : <Copy size={14} />}
                {copiedText === PRE_COMMIT_HOOK ? 'Copied' : 'Copy Script'}
              </Button>
            </div>
            <pre
              style={{
                background: 'var(--bg-code, #1e1e1e)',
                color: 'var(--text-code, #f3f4f6)',
                padding: '14px',
                borderRadius: '6px',
                fontSize: '12px',
                overflowX: 'auto',
                lineHeight: 1.5,
              }}
            >
              <code>{PRE_COMMIT_HOOK}</code>
            </pre>
          </div>

          {/* GitLab CI Card */}
          <div style={{ padding: '20px', borderRadius: '8px', background: 'var(--bg-surface, #fff)', border: '1px solid var(--border-subtle, rgba(0,0,0,0.1))' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
              <div>
                <h3 style={{ margin: 0 }}>GitLab CI Pipeline (.gitlab-ci.yml)</h3>
                <p style={{ margin: '4px 0 0', color: 'var(--text-secondary)', fontSize: '13px' }}>
                  Executes automated analysis on Windows runners and saves reports as build artifacts.
                </p>
              </div>
              <Button variant="outline" size="sm" onClick={() => copy(CI_WORKFLOW_GITLAB)}>
                {copiedText === CI_WORKFLOW_GITLAB ? <Check size={14} color="#10b981" /> : <Copy size={14} />}
                {copiedText === CI_WORKFLOW_GITLAB ? 'Copied' : 'Copy YAML'}
              </Button>
            </div>
            <pre
              style={{
                background: 'var(--bg-code, #1e1e1e)',
                color: 'var(--text-code, #f3f4f6)',
                padding: '14px',
                borderRadius: '6px',
                fontSize: '12px',
                overflowX: 'auto',
                lineHeight: 1.5,
              }}
            >
              <code>{CI_WORKFLOW_GITLAB}</code>
            </pre>
          </div>
        </div>
      )}

      {/* TAB 3: Interactive Command Builder */}
      {tab === 'builder' && (
        <div style={{ padding: '24px', borderRadius: '8px', background: 'var(--bg-surface, #fff)', border: '1px solid var(--border-subtle, rgba(0,0,0,0.1))' }}>
          <h3 style={{ margin: '0 0 8px' }}>Scan Command Generator</h3>
          <p style={{ margin: '0 0 20px', color: 'var(--text-secondary)', fontSize: '13px' }}>
            Customize your scan flags visually, preview the generated terminal command, and copy it straight into your terminal or pipeline.
          </p>

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: '16px', marginBottom: '24px' }}>
            <div>
              <label style={{ display: 'block', fontSize: '12px', fontWeight: 600, marginBottom: '6px' }}>Target Path</label>
              <input
                type="text"
                value={builderTarget}
                onChange={(e) => setBuilderTarget(e.target.value)}
                style={{ width: '100%', padding: '8px', borderRadius: '6px', border: '1px solid var(--border-subtle, #ccc)' }}
              />
            </div>

            <div>
              <label style={{ display: 'block', fontSize: '12px', fontWeight: 600, marginBottom: '6px' }}>Profile</label>
              <select
                value={builderProfile}
                onChange={(e) => setBuilderProfile(e.target.value)}
                style={{ width: '100%', padding: '8px', borderRadius: '6px', border: '1px solid var(--border-subtle, #ccc)', background: 'var(--bg-surface, #fff)' }}
              >
                <option value="quick">Quick (fastest, lightweight lints)</option>
                <option value="standard">Standard (recommended, full analyzers)</option>
                <option value="deep">Deep (exhaustive static rules)</option>
              </select>
            </div>

            <div>
              <label style={{ display: 'block', fontSize: '12px', fontWeight: 600, marginBottom: '6px' }}>CI Gate (--fail-on)</label>
              <select
                value={builderFailOn}
                onChange={(e) => setBuilderFailOn(e.target.value)}
                style={{ width: '100%', padding: '8px', borderRadius: '6px', border: '1px solid var(--border-subtle, #ccc)', background: 'var(--bg-surface, #fff)' }}
              >
                <option value="none">None (don't fail on findings)</option>
                <option value="critical">critical (fail only on critical)</option>
                <option value="high+">high+ (fail on critical or high)</option>
                <option value="medium+">medium+ (fail on medium, high, critical)</option>
                <option value="low+">low+ (fail on any issue)</option>
              </select>
            </div>

            <div>
              <label style={{ display: 'block', fontSize: '12px', fontWeight: 600, marginBottom: '6px' }}>Format (--format)</label>
              <select
                value={builderFormat}
                onChange={(e) => setBuilderFormat(e.target.value)}
                style={{ width: '100%', padding: '8px', borderRadius: '6px', border: '1px solid var(--border-subtle, #ccc)', background: 'var(--bg-surface, #fff)' }}
              >
                <option value="text">text (terminal table)</option>
                <option value="sarif">sarif (OASIS SARIF 2.1.0 log)</option>
                <option value="github">github (GitHub Actions ::warning/::error)</option>
                <option value="json">json (complete report model)</option>
                <option value="csv">csv (spreadsheet)</option>
                <option value="markdown">markdown (SonarQube-style document)</option>
              </select>
            </div>

            <div>
              <label style={{ display: 'block', fontSize: '12px', fontWeight: 600, marginBottom: '6px' }}>Output File (--output)</label>
              <input
                type="text"
                placeholder="leave empty for stdout"
                value={builderOutput}
                onChange={(e) => setBuilderOutput(e.target.value)}
                style={{ width: '100%', padding: '8px', borderRadius: '6px', border: '1px solid var(--border-subtle, #ccc)' }}
              />
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', justifyContent: 'center', gap: '8px' }}>
              <label style={{ display: 'flex', alignItems: 'center', gap: '8px', fontSize: '13px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={builderIncremental}
                  onChange={(e) => setBuilderIncremental(e.target.checked)}
                />
                Incremental Scan (--incremental)
              </label>
              <label style={{ display: 'flex', alignItems: 'center', gap: '8px', fontSize: '13px', cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={builderQuiet}
                  onChange={(e) => setBuilderQuiet(e.target.checked)}
                />
                Quiet stderr progress (--quiet)
              </label>
            </div>
          </div>

          {/* Generated Command Box */}
          <div style={{ marginTop: '16px' }}>
            <div style={{ fontSize: '12px', fontWeight: 600, marginBottom: '6px', color: 'var(--text-secondary)' }}>Generated Command:</div>
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                background: 'var(--bg-code, #1e1e1e)',
                color: 'var(--text-code, #f3f4f6)',
                padding: '14px',
                borderRadius: '6px',
                fontFamily: 'monospace',
                fontSize: '13px',
                gap: '12px',
              }}
            >
              <code style={{ wordBreak: 'break-all' }}>{generatedBuilderCommand}</code>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => copy(generatedBuilderCommand)}
                style={{ flexShrink: 0 }}
              >
                {copiedText === generatedBuilderCommand ? <Check size={14} color="#10b981" /> : <Copy size={14} />}
                {copiedText === generatedBuilderCommand ? 'Copied' : 'Copy'}
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

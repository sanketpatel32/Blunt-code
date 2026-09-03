import { useMemo, useState } from 'react';
import { api } from '../api';
import { useLoad } from '../hooks/useLoad';
import { copyToClipboard } from '../lib/clipboard';
import { PageHeader } from '../components/PageHeader';
import { Button } from '../components/ui/button';
import { Badge } from '../components/ui/badge';
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
import '../css/cli.css';

interface CLICommand {
  id: string;
  name: string;
  category: string;
  synopsis: string;
  description: string;
  badge?: string;
  flags?: { flag: string; type: string; desc: string; def?: string }[];
  examples?: { title: string; cmd: string }[];
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
      { flag: '--workspace', type: 'string', desc: 'Scope search to specific workspace name or ID' },
      { flag: '--severity', type: 'string', desc: 'Filter by comma-separated severities (critical, high, medium, low, info)' },
      { flag: '--analyzer', type: 'string', desc: 'Filter by analyzer ID (e.g. semgrep, ruff, secrets)' },
      { flag: '--limit', type: 'int', desc: 'Maximum number of findings to output (default: 50)' },
      { flag: '--format', type: 'string', desc: 'Output format: text, json, jsonl, or csv (default: text)' },
      { flag: '--output', type: 'file', desc: 'Destination file path for export' },
      { flag: '--lines', type: 'int', desc: 'Context lines around preview excerpt (default: 5)' },
    ],
    examples: [
      { title: 'Search for hardcoded credentials', cmd: 'bluntcode findings search "AWS" --severity critical' },
      { title: 'List issues in current workspace as CSV', cmd: 'bluntcode findings list . --format csv --output issues.csv' },
      { title: 'Inspect source snippet for finding', cmd: 'bluntcode findings preview 99479be6... df7e5ede... --lines 8' },
    ],
  },
  {
    id: 'report',
    name: 'bluntcode report',
    category: 'Reports & Exports',
    synopsis: 'bluntcode report <scan-id|workspace-path> [options]',
    description: 'Generates and exports complete multi-tool audit reports in standard document formats.',
    badge: 'Exporters',
    flags: [
      { flag: '--format', type: 'string', desc: 'Report format: md (Markdown), sarif, html, json, csv, jsonl', def: 'md' },
      { flag: '--output', type: 'file', desc: 'Destination file path (default: stdout)' },
      { flag: '--json', type: 'bool', desc: 'Alias for --format json' },
    ],
    examples: [
      { title: 'Generate SonarQube-style Markdown report', cmd: 'bluntcode report . --format md' },
      { title: 'Generate standalone HTML audit report', cmd: 'bluntcode report . --format html --output audit.html' },
      { title: 'Export OASIS SARIF log', cmd: 'bluntcode report . --format sarif --output audit.sarif' },
      { title: 'Pipe JSON report into jq', cmd: 'bluntcode report . --format json | jq ".findings[] | select(.severity==\"critical\")"' },
    ],
  },
  {
    id: 'history',
    name: 'bluntcode history',
    category: 'Audit & History',
    synopsis: 'bluntcode history [workspace-id|path] [options]',
    description: 'Inspect scan history, track fix velocity, and compare two scans for regressions.',
    badge: 'Audit Trail',
    subcommands: [
      { name: 'list', desc: 'List historical scans for workspace or globally', syntax: 'bluntcode history [workspace] [--limit N] [--json]' },
      { name: 'compare', desc: 'Diff two scans to show new, fixed, and persistent findings', syntax: 'bluntcode history compare <scan-1-id> <scan-2-id> [--json]' },
      { name: 'delete', desc: 'Delete a historical scan record', syntax: 'bluntcode history delete <scan-id> [--json]' },
    ],
    flags: [
      { flag: '--limit', type: 'int', desc: 'Maximum number of scans to display (default: 20)' },
      { flag: '--json', type: 'bool', desc: 'Output structured JSON payload' },
    ],
    examples: [
      { title: 'List recent scans in workspace', cmd: 'bluntcode history . --limit 10' },
      { title: 'Compare baseline and current scan', cmd: 'bluntcode history compare b71d4a... 99479b...' },
      { title: 'Delete an obsolete scan', cmd: 'bluntcode history delete 0c5cc632...' },
    ],
  },
  {
    id: 'suppress',
    name: 'bluntcode suppress',
    category: 'Suppressions & Rules',
    synopsis: 'bluntcode suppress <list|add|remove|import> <workspace> [options]',
    description: 'Manage suppressed false positives permanently across scans by SHA-256 fingerprint hash.',
    badge: 'Triage',
    subcommands: [
      { name: 'list', desc: 'List active suppressions for a workspace', syntax: 'bluntcode suppress list <workspace> [--json|--csv]' },
      { name: 'add', desc: 'Suppress a finding by fingerprint hash', syntax: 'bluntcode suppress add <workspace> --fingerprint <hash> [--reason <text>]' },
      { name: 'remove', desc: 'Remove a suppression to re-enable finding', syntax: 'bluntcode suppress remove <workspace> --fingerprint <hash>' },
      { name: 'import', desc: 'Batch import suppressions from a CSV file', syntax: 'bluntcode suppress import <workspace> <csv-file>' },
    ],
    flags: [
      { flag: '--fingerprint', type: 'hash', desc: 'SHA-256 fingerprint hash of finding to suppress' },
      { flag: '--reason', type: 'string', desc: 'Audit justification reason for suppression' },
      { flag: '--json', type: 'bool', desc: 'Output structured JSON array' },
      { flag: '--csv', type: 'bool', desc: 'Output suppressions as CSV' },
    ],
    examples: [
      { title: 'List suppressions for workspace', cmd: 'bluntcode suppress list .' },
      { title: 'Suppress a false positive finding', cmd: 'bluntcode suppress add . --fingerprint b1242b... --reason "Mock secret in unit test"' },
      { title: 'Batch import suppressions from audit CSV', cmd: 'bluntcode suppress import . suppressions.csv' },
    ],
  },
  {
    id: 'rules',
    name: 'bluntcode rules',
    category: 'Suppressions & Rules',
    synopsis: 'bluntcode rules <list|disable|enable|overrides> <workspace> [options]',
    description: 'Configure workspace file exclusions, path overrides, and toggle individual rules.',
    badge: 'Configuration',
    subcommands: [
      { name: 'list', desc: 'List all rule toggles and path exclusion overrides', syntax: 'bluntcode rules list <workspace> [--json]' },
      { name: 'disable', desc: 'Disable a rule pattern from scanning', syntax: 'bluntcode rules disable <workspace> <rule-or-path>' },
      { name: 'enable', desc: 'Re-enable a rule pattern', syntax: 'bluntcode rules enable <workspace> <rule-or-path>' },
      { name: 'overrides', desc: 'View or set comma-separated path exclusions', syntax: 'bluntcode rules overrides <workspace> [--set "path1,path2"]' },
    ],
    flags: [
      { flag: '--set', type: 'string', desc: 'Comma-separated list of path exclusion globs (e.g. "dist/**,vendor/**")' },
      { flag: '--json', type: 'bool', desc: 'Output structured JSON payload' },
    ],
    examples: [
      { title: 'Inspect active workspace rules', cmd: 'bluntcode rules list .' },
      { title: 'Exclude test fixtures directory', cmd: 'bluntcode rules overrides . --set "dist/**,build/**,tests/fixtures/**"' },
      { title: 'Disable a specific analyzer rule', cmd: 'bluntcode rules disable . "todo.fixme"' },
    ],
  },
  {
    id: 'tools',
    name: 'bluntcode tools',
    category: 'Analyzer Toolchain',
    synopsis: 'bluntcode tools <list|install|repair|update> [args]',
    description: 'Inspect status, install, repair, and update hermetic analyzer binaries (Ruff, Biome, Semgrep, SonarQube, etc.).',
    badge: 'Toolchain',
    subcommands: [
      { name: 'list', desc: 'List all managed tools, versions, and installation states', syntax: 'bluntcode tools list [--json]' },
      { name: 'install', desc: 'Download and verify a pinned analyzer binary', syntax: 'bluntcode tools install <tool-id>' },
      { name: 'repair', desc: 'Force re-download and repair of damaged tool', syntax: 'bluntcode tools repair <tool-id>' },
      { name: 'update', desc: 'Refresh tool configuration to latest pinned version', syntax: 'bluntcode tools update <tool-id>' },
    ],
    flags: [
      { flag: '--json', type: 'bool', desc: 'Output tool states as JSON array' },
    ],
    examples: [
      { title: 'Inspect installed analyzers', cmd: 'bluntcode tools list' },
      { title: 'Install Semgrep engine', cmd: 'bluntcode tools install semgrep' },
      { title: 'Repair Gitleaks binary installation', cmd: 'bluntcode tools repair gitleaks-secrets' },
    ],
  },
  {
    id: 'pentest',
    name: 'bluntcode pentest',
    category: 'Dynamic Pentest',
    synopsis: 'bluntcode pentest probe <url> [options]',
    description: 'Perform active DAST security header probing, TLS analysis, and dynamic vulnerability assessment against HTTP endpoints.',
    badge: 'DAST Probing',
    subcommands: [
      { name: 'probe', desc: 'Probe URL endpoint for security misconfigurations', syntax: 'bluntcode pentest probe <url> [--scope standard|full|spider] [--auth-mode bearer|basic|cookie] [--auth-token <token>] [--json]' },
    ],
    flags: [
      { flag: '--scope', type: 'string', desc: 'Probing scope: standard, full, or spider (default: standard)' },
      { flag: '--auth-mode', type: 'string', desc: 'Authentication scheme: bearer, basic, or cookie' },
      { flag: '--auth-token', type: 'string', desc: 'Credentials or token to authenticate probe requests' },
      { flag: '--json', type: 'bool', desc: 'Output structured JSON audit results' },
    ],
    examples: [
      { title: 'Probe local development server', cmd: 'bluntcode pentest probe http://localhost:3000' },
      { title: 'Deep authenticated API probe', cmd: 'bluntcode pentest probe https://api.staging.internal --scope full --auth-mode bearer --auth-token "eyJh..."' },
      { title: 'Export probe results as JSON', cmd: 'bluntcode pentest probe https://myapp.local --json > pentest-audit.json' },
    ],
  },
  {
    id: 'stats',
    name: 'bluntcode stats / trends / risk',
    category: 'Metrics & Trends',
    synopsis: 'bluntcode <stats|trends|risk> [workspace] [options]',
    description: 'Calculate global or workspace metrics, historical severity trends, and weighted risk scores (0-100) with letter grades (A-D).',
    badge: 'Analytics',
    subcommands: [
      { name: 'stats', desc: 'Print scan count and finding totals', syntax: 'bluntcode stats [workspace] [--json]' },
      { name: 'trends', desc: 'Historical severity counts over time', syntax: 'bluntcode trends <workspace> [--limit N] [--json]' },
      { name: 'risk', desc: 'Calculate weighted risk score (0-100) and grade (A-D)', syntax: 'bluntcode risk <workspace> [--json]' },
    ],
    flags: [
      { flag: '--limit', type: 'int', desc: 'Number of past scans to include in trendline (default: 10)' },
      { flag: '--json', type: 'bool', desc: 'Output structured JSON payload' },
    ],
    examples: [
      { title: 'View global statistics', cmd: 'bluntcode stats' },
      { title: 'View workspace risk score and grade', cmd: 'bluntcode risk .' },
      { title: 'Inspect severity trendline table', cmd: 'bluntcode trends . --limit 15' },
    ],
  },
  {
    id: 'diagnostics',
    name: 'bluntcode doctor / config / update',
    category: 'Diagnostics & System',
    synopsis: 'bluntcode <doctor|config|update> [options]',
    description: 'Run environment health checks, view effective directory configurations, and check for new releases.',
    badge: 'System',
    subcommands: [
      { name: 'doctor', desc: 'Diagnose runtime environment, PATH, and tool installations', syntax: 'bluntcode doctor [--fix] [--json]' },
      { name: 'config', desc: 'Print resolved directory paths and configurations', syntax: 'bluntcode config [--json]' },
      { name: 'update', desc: 'Check for newer Blunt Code releases on GitHub', syntax: 'bluntcode update check [--json]' },
    ],
    flags: [
      { flag: '--fix', type: 'bool', desc: 'Automatically repair detected issues' },
      { flag: '--json', type: 'bool', desc: 'Output structured JSON diagnostic payload' },
    ],
    examples: [
      { title: 'Check system health', cmd: 'bluntcode doctor' },
      { title: 'Self-heal environment issues', cmd: 'bluntcode doctor --fix' },
      { title: 'Check for software updates', cmd: 'bluntcode update check' },
    ],
  },
  {
    id: 'agent',
    name: 'bluntcode agent / llm',
    category: 'AI Agents & Automation',
    synopsis: 'bluntcode <agent|llm> [options]',
    description: 'Commands optimized for AI coding assistants (Claude, Gemini, Cursor, Antigravity) with zero-browser JSON outputs.',
    badge: 'Agent Ready',
    subcommands: [
      { name: 'agent docs', desc: 'Print llm.txt developer guidance to stdout', syntax: 'bluntcode agent docs' },
      { name: 'agent scan', desc: 'Run scan with automatic --json and --quiet defaults', syntax: 'bluntcode agent scan <path> [scan flags]' },
      { name: 'llm', desc: 'Print llms.txt standard index', syntax: 'bluntcode llm' },
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
        cmd.flags?.some((f) => f.flag.toLowerCase().includes(q) || f.desc.toLowerCase().includes(q)) ||
        cmd.examples?.some((e) => e.cmd.toLowerCase().includes(q) || e.title.toLowerCase().includes(q)) ||
        cmd.subcommands?.some((s) => s.name.toLowerCase().includes(q) || s.desc.toLowerCase().includes(q))
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
        badge={meta.data?.version ? <Badge variant="secondary" className="text-xs font-mono">v{meta.data.version}</Badge> : undefined}
        description="Every feature in Blunt Code is fully accessible from your terminal, CI/CD pipeline, pre-commit hook, or AI assistant."
      />

      {/* Hero Overview Card */}
      <section className="cli-hero">
        <div className="cli-hero-top">
          <div>
            <h2 className="cli-hero-title">
              <Terminal size={20} />
              <code>bluntcode</code> CLI {meta.data?.version && <span className="badge">v{meta.data.version}</span>}
            </h2>
            <p className="cli-hero-desc">
              Headless code security analysis, vulnerability probing, and project metrics for Windows.
            </p>
          </div>
          <div className="cli-badges">
            <span className="badge">
              <Shield size={13} /> 100% Local & Hermetic
            </span>
            <span className="badge">
              <GitBranch size={13} /> CI Exit Codes
            </span>
            <span className="badge">
              <Bot size={13} /> Agent Ready
            </span>
          </div>
        </div>

        {/* Global CLI quick facts */}
        <div className="cli-facts">
          <div className="cli-fact-tile">
            <div className="cli-fact-title">Concurrent CLI & GUI</div>
            <div className="cli-fact-desc">
              Queries run safely without blocking or needing the single-instance mutex. Run CLI commands even while the GUI is open.
            </div>
          </div>
          <div className="cli-fact-tile">
            <div className="cli-fact-title">Standard Exit Codes</div>
            <div className="cli-fact-desc">
              <code>0</code> = Clean / success, <code>1</code> = Gate tripped or issues found, <code>2</code> = Flag syntax or usage error.
            </div>
          </div>
          <div className="cli-fact-tile">
            <div className="cli-fact-title">Built-in Manual</div>
            <div className="cli-fact-desc">
              Run <code>bluntcode cli &lt;command&gt;</code> in terminal for comprehensive in-terminal command documentation and examples.
            </div>
          </div>
        </div>
      </section>

      {/* Navigation Tabs */}
      <nav className="cli-tabs" aria-label="CLI Documentation Tabs">
        <button
          type="button"
          className={`tab cli-tab${tab === 'reference' ? ' active' : ''}`}
          onClick={() => setTab('reference')}
        >
          <Terminal size={16} /> Reference Manual
        </button>
        <button
          type="button"
          className={`tab cli-tab${tab === 'ci' ? ' active' : ''}`}
          onClick={() => setTab('ci')}
        >
          <GitBranch size={16} /> CI/CD & Automation
        </button>
        <button
          type="button"
          className={`tab cli-tab${tab === 'builder' ? ' active' : ''}`}
          onClick={() => setTab('builder')}
        >
          <Sliders size={16} /> Command Builder
        </button>
      </nav>

      {/* TAB 1: Reference Manual */}
      {tab === 'reference' && (
        <div>
          {/* Filter Bar */}
          <div className="cli-filter-bar">
            <div className="cli-search-wrap">
              <Search size={16} className="cli-search-icon" aria-hidden="true" />
              <input
                type="text"
                placeholder="Search commands, flags, or recipes..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="cli-search-input"
              />
            </div>
            <div className="cli-cat-pills">
              {categories.map((cat) => (
                <button
                  key={cat}
                  type="button"
                  onClick={() => setSelectedCategory(cat)}
                  className={`cli-cat-pill${selectedCategory === cat ? ' active' : ''}`}
                >
                  {cat}
                </button>
              ))}
            </div>
          </div>

          {/* Commands List */}
          <div className="cli-commands-list">
            {filteredCommands.length === 0 ? (
              <div className="cli-empty">
                No commands match your query "{search}".
              </div>
            ) : (
              filteredCommands.map((cmd) => (
                <article key={cmd.id} className="cli-command-card">
                  <div className="cli-cmd-head">
                    <div>
                      <div className="cli-cmd-title-row">
                        <h3 className="cli-cmd-title">{cmd.name}</h3>
                        {cmd.badge && <span className="badge">{cmd.badge}</span>}
                        <span className="cli-cmd-category">{cmd.category}</span>
                      </div>
                      <p className="cli-cmd-desc">{cmd.description}</p>
                    </div>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => copy(cmd.synopsis)}
                      title="Copy synopsis"
                    >
                      {copiedText === cmd.synopsis ? <Check size={14} className="text-emerald-500" /> : <Copy size={14} />}
                      {copiedText === cmd.synopsis ? 'Copied' : 'Copy'}
                    </Button>
                  </div>

                  {/* Synopsis Box */}
                  <div className="cli-terminal-box">
                    <code className="cli-terminal-code">
                      <span className="cli-prompt-symbol" aria-hidden="true">&gt;</span>
                      {cmd.synopsis}
                    </code>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => copy(cmd.synopsis)}
                      title="Copy synopsis"
                    >
                      {copiedText === cmd.synopsis ? <Check size={14} className="text-emerald-500" /> : <Copy size={14} />}
                    </Button>
                  </div>

                  {/* Subcommands if any */}
                  {cmd.subcommands && cmd.subcommands.length > 0 && (
                    <div className="cli-subcommands">
                      <div className="cli-section-label">Subcommands</div>
                      <div className="cli-subcmd-grid">
                        {cmd.subcommands.map((sub) => (
                          <div key={sub.name} className="cli-subcmd-item">
                            <div>
                              <strong>{sub.name}</strong>
                              <span className="cli-subcmd-desc">{sub.desc}</span>
                              <div className="cli-subcmd-syntax">
                                <code>{sub.syntax}</code>
                              </div>
                            </div>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => copy(sub.syntax)}
                              title="Copy command"
                            >
                              {copiedText === sub.syntax ? <Check size={14} className="text-emerald-500" /> : <Copy size={14} />}
                            </Button>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Flag Options Table */}
                  {cmd.flags && cmd.flags.length > 0 && (
                    <div className="cli-flags-wrap">
                      <div className="cli-section-label">Options &amp; Flags</div>
                      <div className="cli-table-container">
                        <table className="cli-table">
                          <thead>
                            <tr>
                              <th>Flag</th>
                              <th>Type</th>
                              <th>Description</th>
                              <th>Default</th>
                            </tr>
                          </thead>
                          <tbody>
                            {cmd.flags.map((f) => (
                              <tr key={f.flag}>
                                <td className="cli-flag-name"><code>{f.flag}</code></td>
                                <td className="cli-flag-type">{f.type}</td>
                                <td className="cli-flag-desc">{f.desc}</td>
                                <td className="cli-flag-def"><code>{f.def || '—'}</code></td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  )}

                  {/* Examples */}
                  {cmd.examples && cmd.examples.length > 0 && (
                    <div className="cli-examples-wrap">
                      <div className="cli-section-label">Practical Examples</div>
                      <div className="cli-examples-grid">
                        {cmd.examples.map((ex, i) => (
                          <div key={i} className="cli-example-item">
                            <div>
                              <div className="cli-example-title">{ex.title}</div>
                              <code className="cli-example-cmd">{ex.cmd}</code>
                            </div>
                            <Button variant="ghost" size="sm" onClick={() => copy(ex.cmd)} title="Copy example">
                              {copiedText === ex.cmd ? <Check size={14} className="text-emerald-500" /> : <Copy size={14} />}
                            </Button>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </article>
              ))
            )}
          </div>
        </div>
      )}

      {/* TAB 2: CI/CD & Automation Recipes */}
      {tab === 'ci' && (
        <div className="cli-ci-list">
          {/* GitHub Actions Card */}
          <section className="cli-ci-card">
            <div className="cli-ci-head">
              <div>
                <h3 className="cli-ci-title">GitHub Actions with SARIF Code Scanning</h3>
                <p className="cli-ci-desc">
                  Runs on pull requests and pushes, trips the build on high/critical findings, and uploads OASIS SARIF alerts to GitHub Security tab.
                </p>
              </div>
              <Button variant="outline" size="sm" onClick={() => copy(CI_WORKFLOW_GITHUB)}>
                {copiedText === CI_WORKFLOW_GITHUB ? <Check size={14} className="text-emerald-500" /> : <Copy size={14} />}
                {copiedText === CI_WORKFLOW_GITHUB ? 'Copied' : 'Copy YAML'}
              </Button>
            </div>
            <pre className="cli-code-pre">
              <code>{CI_WORKFLOW_GITHUB}</code>
            </pre>
          </section>

          {/* Pre-commit Hook Card */}
          <section className="cli-ci-card">
            <div className="cli-ci-head">
              <div>
                <h3 className="cli-ci-title">Git Pre-Commit Hook</h3>
                <p className="cli-ci-desc">
                  Save to <code>.git/hooks/pre-commit</code> to prevent committing vulnerable code or hardcoded secrets locally.
                </p>
              </div>
              <Button variant="outline" size="sm" onClick={() => copy(PRE_COMMIT_HOOK)}>
                {copiedText === PRE_COMMIT_HOOK ? <Check size={14} className="text-emerald-500" /> : <Copy size={14} />}
                {copiedText === PRE_COMMIT_HOOK ? 'Copied' : 'Copy Script'}
              </Button>
            </div>
            <pre className="cli-code-pre">
              <code>{PRE_COMMIT_HOOK}</code>
            </pre>
          </section>

          {/* GitLab CI Card */}
          <section className="cli-ci-card">
            <div className="cli-ci-head">
              <div>
                <h3 className="cli-ci-title">GitLab CI Pipeline (.gitlab-ci.yml)</h3>
                <p className="cli-ci-desc">
                  Executes automated analysis on Windows runners and saves reports as build artifacts.
                </p>
              </div>
              <Button variant="outline" size="sm" onClick={() => copy(CI_WORKFLOW_GITLAB)}>
                {copiedText === CI_WORKFLOW_GITLAB ? <Check size={14} className="text-emerald-500" /> : <Copy size={14} />}
                {copiedText === CI_WORKFLOW_GITLAB ? 'Copied' : 'Copy YAML'}
              </Button>
            </div>
            <pre className="cli-code-pre">
              <code>{CI_WORKFLOW_GITLAB}</code>
            </pre>
          </section>
        </div>
      )}

      {/* TAB 3: Interactive Command Builder */}
      {tab === 'builder' && (
        <section className="cli-builder-card">
          <div className="cli-builder-head">
            <h3>Scan Command Generator</h3>
            <p>
              Customize your scan flags visually, preview the generated terminal command, and copy it straight into your terminal or pipeline.
            </p>
          </div>

          <div className="cli-builder-grid">
            <div className="cli-builder-field">
              <label className="cli-builder-label">Target Path</label>
              <input
                type="text"
                value={builderTarget}
                onChange={(e) => setBuilderTarget(e.target.value)}
                className="cli-builder-input"
              />
            </div>

            <div className="cli-builder-field">
              <label className="cli-builder-label">Profile</label>
              <select
                value={builderProfile}
                onChange={(e) => setBuilderProfile(e.target.value)}
                className="cli-builder-select"
              >
                <option value="quick">Quick (fastest, lightweight lints)</option>
                <option value="standard">Standard (recommended, full analyzers)</option>
                <option value="deep">Deep (exhaustive static rules)</option>
              </select>
            </div>

            <div className="cli-builder-field">
              <label className="cli-builder-label">CI Gate (--fail-on)</label>
              <select
                value={builderFailOn}
                onChange={(e) => setBuilderFailOn(e.target.value)}
                className="cli-builder-select"
              >
                <option value="none">None (don't fail on findings)</option>
                <option value="critical">critical (fail only on critical)</option>
                <option value="high+">high+ (fail on critical or high)</option>
                <option value="medium+">medium+ (fail on medium, high, critical)</option>
                <option value="low+">low+ (fail on any issue)</option>
              </select>
            </div>

            <div className="cli-builder-field">
              <label className="cli-builder-label">Format (--format)</label>
              <select
                value={builderFormat}
                onChange={(e) => setBuilderFormat(e.target.value)}
                className="cli-builder-select"
              >
                <option value="text">text (terminal table)</option>
                <option value="sarif">sarif (OASIS SARIF 2.1.0 log)</option>
                <option value="github">github (GitHub Actions ::warning/::error)</option>
                <option value="json">json (complete report model)</option>
                <option value="csv">csv (spreadsheet)</option>
                <option value="markdown">markdown (SonarQube-style document)</option>
              </select>
            </div>

            <div className="cli-builder-field">
              <label className="cli-builder-label">Output File (--output)</label>
              <input
                type="text"
                placeholder="leave empty for stdout"
                value={builderOutput}
                onChange={(e) => setBuilderOutput(e.target.value)}
                className="cli-builder-input"
              />
            </div>

            <div className="cli-builder-checkboxes">
              <label className="cli-builder-check-label">
                <input
                  type="checkbox"
                  checked={builderIncremental}
                  onChange={(e) => setBuilderIncremental(e.target.checked)}
                />
                Incremental Scan (--incremental)
              </label>
              <label className="cli-builder-check-label">
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
          <div className="cli-generated-wrap">
            <div className="cli-section-label">Generated Command:</div>
            <div className="cli-generated-box">
              <code className="cli-generated-code">{generatedBuilderCommand}</code>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => copy(generatedBuilderCommand)}
                className="shrink-0"
              >
                {copiedText === generatedBuilderCommand ? <Check size={14} className="text-emerald-500" /> : <Copy size={14} />}
                {copiedText === generatedBuilderCommand ? 'Copied' : 'Copy'}
              </Button>
            </div>
          </div>
        </section>
      )}
    </div>
  );
}

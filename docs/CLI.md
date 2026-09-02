# Blunt Code CLI Reference Manual

Blunt Code features complete command-line interface support for every capability in the system. Every action that can be performed in the web UI can also be executed headlessly via the `bluntcode` binary without starting a browser.

CLI queries open the SQLite database directly with shared concurrency, meaning CLI commands can run safely in parallel with the web UI server.

---

## Command Quick Reference

| Command | Category | Description |
| :--- | :--- | :--- |
| `bluntcode scan <path>` | CI & Scans | Run automated code security & quality scans with CI exit codes |
| `bluntcode prune <path>` | CI & Scans | Retain the last N completed scans for a workspace |
| `bluntcode workspace <cmd>` | Management | Register, inspect, list, tag, or delete workspaces |
| `bluntcode findings <cmd>` | Inspection | Search across findings, list scan issues, preview source code |
| `bluntcode report <target>` | Exporters | Export audit reports in Markdown, SARIF, HTML, JSON, CSV, JSONL |
| `bluntcode history <cmd>` | Audit Trail | Trace past scans and compare two scans for regressions |
| `bluntcode suppress <cmd>` | Triage | Suppress false positives forever by fingerprint hash |
| `bluntcode rules <cmd>` | Configuration | View/set path exclusion globs and toggle analyzer rules |
| `bluntcode tools <cmd>` | Toolchain | List, install, repair, and update hermetic analyzer binaries |
| `bluntcode pentest probe` | DAST | Dynamic HTTP security header, TLS, and vulnerability probe |
| `bluntcode stats / risk` | Metrics | Calculate global/workspace statistics, risk score (0-100), grade |
| `bluntcode doctor / update` | System | Run environment health checks, auto-repair, check updates |
| `bluntcode agent / llm` | AI Agents | Machine-readable defaults and developer documentation for agents |
| `bluntcode cli [command]` | Built-in Docs | Built-in CLI reference manual and syntax helper |

---

## Exit Codes

All Blunt Code commands return deterministic exit codes suitable for CI/CD automation:

- `0` — **Clean / Success**: Scan completed with no gate violations, or command executed successfully.
- `1` — **Gate Tripped / Findings Found / Execution Error**: Scan findings tripped `--fail-on` or `--max-findings`, or operation failed.
- `2` — **Usage / Flag Error**: Missing required arguments, invalid flag value, or syntax error.
- `130` — **Interrupted**: Execution aborted by `Ctrl+C` (SIGINT).

---

## 1. Automated Scans (`bluntcode scan`)

Run static code analysis on a project directory.

```bash
bluntcode scan <path> [options]
```

### Flags
- `--profile quick|standard|deep`: Analyzer depth (default: `standard`).
  - `quick`: Fast linters (Ruff, Biome, Todo, Secrets).
  - `standard`: Complete multi-engine suite (adds Semgrep, Gitleaks, Checkov, Trivy, OSV).
  - `deep`: Exhaustive rule sets and cross-file checks.
- `--fail-on <severity+>`: Trips exit code 1 if findings remain at or above severity (e.g. `critical`, `high+`, `medium+`, `low+`).
- `--max-findings N`: Trips exit code 1 if total non-suppressed findings exceed N.
- `--baseline <id-or-sarif>`: Compares findings against a baseline scan ID or SARIF file; only trips gate on **new** findings.
- `--format text|json|github|sarif|csv|jsonl|markdown`: Output document format (default: `text`).
- `--output <file>`: Write document to destination file instead of stdout.
- `--incremental`: Reuses previous scan findings for files whose content hashes have not changed.
- `--jobs N`: Run up to N analyzers concurrently (default: `4`).
- `--watch`: Watch directory and automatically trigger incremental rescan on file changes.
- `--quiet`: Suppress analyzer progress lines on stderr.
- `--json`: Print compact JSON summary block.

### Examples
```bash
# Standard local scan
bluntcode scan .

# CI gate for GitHub Actions failing on high/critical findings
bluntcode scan . --fail-on high+ --format github

# Export SARIF for GitHub Code Scanning
bluntcode scan . --format sarif --output audit.sarif

# Watch mode during local development
bluntcode scan . --watch --profile quick
```

---

## 2. Workspace Management (`bluntcode workspace`)

Manage registered projects, metadata, directories, and tags.

```bash
bluntcode workspace list [--json]
bluntcode workspace add <path> [--name <name>] [--profile quick|standard|deep] [--json]
bluntcode workspace show <id|path> [--json]
bluntcode workspace tree <id|path> [--path <subpath>] [--json]
bluntcode workspace tags <id|path> [--set "tag1,tag2"] [--json]
bluntcode workspace delete <id|path> [--json]
```

### Examples
```bash
# Register a workspace
bluntcode workspace add . --name my-service

# Assign tags
bluntcode workspace tags my-service --set "backend,security,go"

# Inspect file tree and exclusions
bluntcode workspace tree my-service --path src
```

---

## 3. Findings & Code Inspection (`bluntcode findings`)

Search, list, and preview source snippets for any finding.

```bash
# Search across all workspaces or a specific workspace
bluntcode findings search <query> [--workspace <id|path>] [--severity <sev>] [--limit N] [--json]

# List findings for a scan or workspace
bluntcode findings list <scan-id|path> [--severity <sev>] [--format text|json|jsonl|csv] [--output <file>]

# Preview source snippet around a finding
bluntcode findings preview <scan-id> <finding-id> [--lines N] [--json]
```

### Examples
```bash
# Search for hardcoded secrets
bluntcode findings search "AWS" --severity critical

# Export findings to CSV
bluntcode findings list . --format csv --output findings.csv

# Preview code snippet with line numbers
bluntcode findings preview 99479be6... df7e5ede... --lines 5
```

---

## 4. Report Exporter (`bluntcode report`)

Exports a full audit report for any scan in standard formats.

```bash
bluntcode report <scan-id|workspace-path> [--format md|sarif|html|json|csv|jsonl] [--output <file>]
```

### Supported Formats
- `md` / `markdown`: SonarQube-style human-readable Markdown report (default).
- `sarif`: OASIS SARIF 2.1.0 log for GitHub Code Scanning and IDEs.
- `html`: Self-contained interactive HTML executive audit report.
- `json`: Complete structured report payload.
- `csv`: Comma-separated spreadsheet with UTF-8 BOM.
- `jsonl`: Newline-delimited JSON stream for log pipelines and `jq`.

---

## 5. Scan History & Diff (`bluntcode history`)

Trace historical runs and compare scans for regressions.

```bash
# List historical scans
bluntcode history [workspace-id|path] [--limit N] [--json]

# Compare two scans (new, fixed, persistent)
bluntcode history compare <scan-1-id> <scan-2-id> [--json]

# Delete a scan record
bluntcode history delete <scan-id> [--json]
```

---

## 6. Suppressions (`bluntcode suppress`)

Suppress false positives forever by SHA-256 fingerprint hash.

```bash
# List suppressions
bluntcode suppress list <workspace> [--json|--csv]

# Suppress a finding
bluntcode suppress add <workspace> --fingerprint <hash> [--reason <text>]

# Remove a suppression
bluntcode suppress remove <workspace> --fingerprint <hash>

# Batch import suppressions from CSV
bluntcode suppress import <workspace> <csv-file>
```

---

## 7. Rules & Overrides (`bluntcode rules`)

Configure path exclusion overrides and toggle individual analyzer rules.

```bash
# List rules and overrides
bluntcode rules list <workspace> [--json]

# Disable / enable rule pattern
bluntcode rules disable <workspace> "test/**"
bluntcode rules enable <workspace> "test/**"

# Set path exclusion overrides
bluntcode rules overrides <workspace> --set "dist/**,build/**,node_modules/**"
```

---

## 8. Managed Analyzer Toolchain (`bluntcode tools`)

Inspect and manage hermetic analyzer binaries.

```bash
bluntcode tools list [--json]
bluntcode tools install <tool-id>
bluntcode tools repair <tool-id>
bluntcode tools update <tool-id>
```

---

## 9. Dynamic Pentest Probing (`bluntcode pentest`)

Perform dynamic DAST probing against HTTP endpoints.

```bash
bluntcode pentest probe <url> [options]
```

### Flags
- `--scope standard|full|spider`: Probe depth (default: `standard`).
- `--auth-mode bearer|basic|cookie`: Authentication mode.
- `--auth-token <credentials>`: Token, credentials, or cookie string.
- `--json`: Output complete structured JSON audit results.

---

## 10. Metrics, Trends & Risk (`bluntcode stats`)

Calculate aggregate figures, risk scores (0-100), and historical trendlines.

```bash
# Global statistics across all workspaces
bluntcode stats [--json]

# Workspace-specific summary
bluntcode stats <workspace> [--json]

# Severity trendline over time
bluntcode trends <workspace> [--limit N] [--json]

# Weighted risk assessment and risk grade (A-D)
bluntcode risk <workspace> [--json]
```

---

## 11. Diagnostics & Updates

```bash
# Diagnose system environment and tool installations
bluntcode doctor [--fix] [--json]

# Print resolved directory paths and configurations
bluntcode config [--json]

# Check for latest Blunt Code release
bluntcode update check [--json]
```

---

## 12. AI Agent Integration (`bluntcode agent`)

Optimized commands for AI coding assistants (Claude, Gemini, Cursor, Antigravity).

```bash
# Print llm.txt developer guidance
bluntcode agent docs
bluntcode llm

# Headless scan with automated --json --quiet defaults
bluntcode agent scan <path> [scan flags]
```

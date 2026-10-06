package main

import (
	"fmt"
	"io"
	"strings"
)

func runCLIDocs(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printAllCLIDocs(stdout)
		return 0
	}

	cmd := strings.ToLower(strings.TrimSpace(args[0]))
	switch cmd {
	case "scan":
		printScanDoc(stdout)
	case "workspace", "workspaces":
		printWorkspaceDoc(stdout)
	case "findings", "finding":
		printFindingsDoc(stdout)
	case "history", "scans":
		printHistoryDoc(stdout)
	case "report", "reports":
		printReportDoc(stdout)
	case "suppress", "suppressions":
		printSuppressDoc(stdout)
	case "rules", "rule":
		printRulesDoc(stdout)
	case "tools", "tool":
		printToolsDoc(stdout)
	case "pentest":
		printPentestDoc(stdout)
	case "stats", "trends", "risk":
		printStatsDoc(stdout)
	case "doctor":
		printDoctorDoc(stdout)
	case "config":
		printConfigDoc(stdout)
	case "clean":
		printCleanDoc(stdout)
	case "agent", "llm":
		printAgentDoc(stdout)
	default:
		fmt.Fprintf(stderr, "bluntcode cli: no documentation for %q\n\n", cmd)
		printAllCLIDocs(stderr)
		return 2
	}
	return 0
}

func printAllCLIDocs(w io.Writer) {
	fmt.Fprintf(w, "=========================================================================\n")
	fmt.Fprintf(w, "  BLUNT CODE CLI REFERENCE MANUAL (v%s)\n", version)
	fmt.Fprintf(w, "  Local code quality, security analysis, and dynamic testing for Windows\n")
	fmt.Fprintf(w, "=========================================================================\n\n")

	fmt.Fprintln(w, "Blunt Code scans a folder with local analyzers and reports code-quality and")
	fmt.Fprintln(w, "security findings. Nothing leaves this computer.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "START HERE - three commands cover most of what you need:")
	fmt.Fprintln(w, "  bluntcode scan .                     # scan the current folder, human summary")
	fmt.Fprintln(w, "  bluntcode findings list .            # what did it find, file by file")
	fmt.Fprintln(w, "  bluntcode report .                   # the full report of the last scan")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "THE 30-SECOND VERSION:")
	fmt.Fprintln(w, "  A scan copies nothing and changes nothing - it only reads your files and")
	fmt.Fprintln(w, "  writes its own database. Pick a profile to choose how deep it goes:")
	fmt.Fprintln(w, "    quick     linters only (Ruff, Biome) - usually under a minute")
	fmt.Fprintln(w, "    standard  + secrets, security patterns, SonarQube - minutes (default)")
	fmt.Fprintln(w, "    deep      + dependencies, containers, infrastructure - 10+ minutes")
	fmt.Fprintln(w, "    pentest   standard plus OWASP-focused checks")
	fmt.Fprintln(w, "  The exit code tells scripts what happened:")
	fmt.Fprintln(w, "    0 clean run   1 findings tripped your gate (--fail-on/--max-findings)")
	fmt.Fprintln(w, "    2 bad command   3 scan problem (analyzer failed or coverage incomplete)")
	fmt.Fprintln(w, "    4 you cancelled   130 double Ctrl+C")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "COMMAND CATEGORIES:")
	fmt.Fprintln(w, "  1. Scans & CI Gates           `scan`, `prune`")
	fmt.Fprintln(w, "  2. Workspace Management       `workspace <list|add|show|tree|tags|delete>`")
	fmt.Fprintln(w, "  3. Findings & Inspection      `findings <search|list|preview>`")
	fmt.Fprintln(w, "  4. Reports & Exports          `report <scan|path> [--format md|sarif|html|json]`")
	fmt.Fprintln(w, "  5. History & Compare          `history [path]`, `history compare <id1> <id2>`")
	fmt.Fprintln(w, "  6. Analyzer Toolchain         `tools <list|install|uninstall|repair|update>`")
	fmt.Fprintln(w, "  7. Rules & Suppressions       `suppress <list|add|remove|import>`, `rules`")
	fmt.Fprintln(w, "  8. Dynamic Pentest & DAST     `pentest probe <url>`")
	fmt.Fprintln(w, "  9. Stats, Trends & Risk       `stats`, `trends`, `risk`")
	fmt.Fprintln(w, "  10. Diagnostics, Clean & Update `doctor`, `config`, `clean`, `update`")
	fmt.Fprintln(w, "  11. AI Agents & Scripts       `agent docs`, `agent scan`, `llm`")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run `bluntcode cli <command>` for detailed command manual and recipes.")
	fmt.Fprintln(w, "For the web documentation, navigate to http://127.0.0.1:<port>/cli")
}

func printScanDoc(w io.Writer) {
	fmt.Fprintln(w, "NAME:")
	fmt.Fprintln(w, "  bluntcode scan - Run a headless, automated code quality and security scan")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "WHAT IT DOES:")
	fmt.Fprintln(w, "  Walks the folder, runs every analyzer that applies to its file types, and")
	fmt.Fprintln(w, "  prints a summary to stdout (progress goes to stderr). Your files are only")
	fmt.Fprintln(w, "  read, never modified. The same scan and findings are saved locally so")
	fmt.Fprintln(w, "  `findings`, `report`, and `history` can show them afterwards.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SYNOPSIS:")
	fmt.Fprintln(w, "  bluntcode scan <path> [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "PROFILES (--profile):")
	fmt.Fprintln(w, "  quick     Ruff + Biome only - Python/JS/TS linting, usually <1 minute.")
	fmt.Fprintln(w, "            Best as a fast pre-commit sanity check.")
	fmt.Fprintln(w, "  standard  Adds secrets, security patterns, pentest heuristics, TODO and")
	fmt.Fprintln(w, "            license checks, and SonarQube. Minutes. The sensible default.")
	fmt.Fprintln(w, "  deep      Adds dependency vulnerabilities (OSV, Trivy), infrastructure")
	fmt.Fprintln(w, "            checks (Checkov), and wider Ruff rules. 10+ minutes and may")
	fmt.Fprintln(w, "            download tool databases on the first run.")
	fmt.Fprintln(w, "  pentest   The standard set plus OWASP Top 10 focused checks.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "EXIT CODES (what your script should check):")
	fmt.Fprintln(w, "  0    scan completed; any gate you set passed")
	fmt.Fprintln(w, "  1    scan completed but findings tripped --fail-on / --max-findings")
	fmt.Fprintln(w, "  2    usage error (bad flag, bad path) - the command never ran")
	fmt.Fprintln(w, "  3    operational problem: scan failed, timed out, or finished with")
	fmt.Fprintln(w, "       incomplete coverage (exit 3 means the findings list may be partial)")
	fmt.Fprintln(w, "  4    cancelled by a single Ctrl+C")
	fmt.Fprintln(w, "  130  second Ctrl+C pressed - immediate exit")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "OPTIONS:")
	fmt.Fprintln(w, "  --profile quick|standard|deep|pentest  Select analyzer depth (default: standard)")
	fmt.Fprintln(w, "  --fail-on <severity+>          Exit 1 if findings remain at or above severity (e.g. high+, critical)")
	fmt.Fprintln(w, "  --max-findings N               Exit 1 if total findings exceed N")
	fmt.Fprintln(w, "  --baseline <id-or-sarif>       Compare against baseline and trip gate only on NEW findings")
	fmt.Fprintln(w, "  --gate-analyzer <ids>          Scope the CI gate to these analyzers only (comma-separated, e.g. semgrep,secrets)")
	fmt.Fprintln(w, "  --gate-category <categories>   Scope the CI gate to these categories only (e.g. security,correctness)")
	fmt.Fprintln(w, "  --format text|json|sarif|...   Output document format (text, json, github, sarif, csv, jsonl, markdown)")
	fmt.Fprintln(w, "  --json                         Print a machine-readable JSON summary instead of human text")
	fmt.Fprintln(w, "  --output <file>                Write report to file instead of stdout")
	fmt.Fprintln(w, "  --save-baseline <file>         Write the scan's SARIF document after a completed scan (same bytes as --format sarif)")
	fmt.Fprintln(w, "  --incremental                  Only scan files modified since previous completed scan")
	fmt.Fprintln(w, "  --jobs N                       Run up to N analyzers concurrently")
	fmt.Fprintln(w, "  --github-cap N                 Max GitHub annotations per severity before truncation (1-50, default 10)")
	fmt.Fprintln(w, "  --timeout <duration>           Abort the scan when it exceeds this duration (default 30m)")
	fmt.Fprintln(w, "  --watch                        Watch directory and automatically rescan on change")
	fmt.Fprintln(w, "  --watch-poll <duration>        Watch mode filesystem poll interval (default 2s)")
	fmt.Fprintln(w, "  --watch-quiet <duration>       Watch mode quiet window before a rescan starts (default 1.5s)")
	fmt.Fprintln(w, "  --quiet                        Suppress analyzer progress lines on stderr")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "EXAMPLES:")
	fmt.Fprintln(w, "  bluntcode scan . --profile quick")
	fmt.Fprintln(w, "  bluntcode scan . --fail-on high+ --format github")
	fmt.Fprintln(w, "  bluntcode scan C:\\projects\\api --format sarif --output audit.sarif")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "RECIPES:")
	fmt.Fprintln(w, "  First time on a project:")
	fmt.Fprintln(w, "    bluntcode scan . --fail-on critical            # how bad is it, worst first")
	fmt.Fprintln(w, "    bluntcode findings list . --severity critical  # then see each one")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Break the build only on NEW issues (PR review):")
	fmt.Fprintln(w, "    bluntcode scan . --save-baseline main.sarif    # once, on the main branch")
	fmt.Fprintln(w, "    bluntcode scan . --baseline main.sarif --fail-on high+")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  GitHub Actions (annotations appear on the diff):")
	fmt.Fprintln(w, "    bluntcode scan . --fail-on high+ --format github")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  While coding (rescans ~1.5s after you stop editing):")
	fmt.Fprintln(w, "    bluntcode scan . --profile quick --watch --incremental")
}

func printWorkspaceDoc(w io.Writer) {
	fmt.Fprintln(w, "NAME:")
	fmt.Fprintln(w, "  bluntcode workspace - Manage registered codebases and project metadata")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SUBCOMMANDS:")
	fmt.Fprintln(w, "  list                     List all registered workspaces")
	fmt.Fprintln(w, "  add <path>               Register a new workspace directory")
	fmt.Fprintln(w, "  show <id|path>           Display full workspace metadata, scan count, and tags")
	fmt.Fprintln(w, "  tree <id|path>           View file structure, sizes, and excluded paths")
	fmt.Fprintln(w, "  tags <id|path>           View or assign tags (--set \"tag1,tag2\")")
	fmt.Fprintln(w, "  delete <id|path>         Remove workspace registration")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "EXAMPLES:")
	fmt.Fprintln(w, "  bluntcode workspace add . --name my-service")
	fmt.Fprintln(w, "  bluntcode workspace show my-service")
	fmt.Fprintln(w, "  bluntcode workspace tree my-service --path src")
}

func printFindingsDoc(w io.Writer) {
	fmt.Fprintln(w, "NAME:")
	fmt.Fprintln(w, "  bluntcode findings - Search, filter, and inspect code findings across scans")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SUBCOMMANDS:")
	fmt.Fprintln(w, "  search <query>           Search finding messages and rules across all workspaces")
	fmt.Fprintln(w, "  list <scan|path>         List all findings for a scan (supports --severity, --format)")
	fmt.Fprintln(w, "  preview <scan> <id>      Show exact source code snippet around the finding")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "EXAMPLES:")
	fmt.Fprintln(w, "  bluntcode findings search \"hardcoded secret\" --severity high+")
	fmt.Fprintln(w, "  bluntcode findings list . --format csv --output issues.csv")
	fmt.Fprintln(w, "  bluntcode findings preview <scan-id> <finding-id> --lines 8")
}

func printHistoryDoc(w io.Writer) {
	fmt.Fprintln(w, "NAME:")
	fmt.Fprintln(w, "  bluntcode history - Trace historical scan runs and compare results")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SUBCOMMANDS:")
	fmt.Fprintln(w, "  [workspace]              List historical scans (limit, duration, finding counts)")
	fmt.Fprintln(w, "  delete <scan-id>         Delete a scan record")
	fmt.Fprintln(w, "  compare <id1> <id2>      Diff two scans showing new, fixed, and persistent issues")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "EXAMPLES:")
	fmt.Fprintln(w, "  bluntcode history . --limit 5")
	fmt.Fprintln(w, "  bluntcode history compare <baseline-scan-id> <current-scan-id>")
}

func printReportDoc(w io.Writer) {
	fmt.Fprintln(w, "NAME:")
	fmt.Fprintln(w, "  bluntcode report - Export full audit reports in standard formats")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "FORMATS:")
	fmt.Fprintln(w, "  md, sarif, html, json, csv, jsonl")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "EXAMPLES:")
	fmt.Fprintln(w, "  bluntcode report . --format md")
	fmt.Fprintln(w, "  bluntcode report . --format sarif --output code-scanning.sarif")
	fmt.Fprintln(w, "  bluntcode report <scan-id> --format html --output audit-report.html")
}

func printSuppressDoc(w io.Writer) {
	fmt.Fprintln(w, "NAME:")
	fmt.Fprintln(w, "  bluntcode suppress - Suppress false positives and accepted risks")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SUBCOMMANDS:")
	fmt.Fprintln(w, "  list <workspace>         List all active suppressions")
	fmt.Fprintln(w, "  add <workspace>          Suppress a fingerprint (--fingerprint <hash> --reason <text>)")
	fmt.Fprintln(w, "  remove <workspace>       Unsuppress a fingerprint (--fingerprint <hash>)")
	fmt.Fprintln(w, "  import <workspace>       Batch import suppressions from CSV")
	fmt.Fprintln(w)
}

func printRulesDoc(w io.Writer) {
	fmt.Fprintln(w, "NAME:")
	fmt.Fprintln(w, "  bluntcode rules - Workspace analyzer rule toggles and path exclusion overrides")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SUBCOMMANDS:")
	fmt.Fprintln(w, "  list <workspace>         List configured rules and path exclusion overrides")
	fmt.Fprintln(w, "  disable <workspace> <id> Disable a specific rule")
	fmt.Fprintln(w, "  enable <workspace> <id>  Enable a specific rule")
	fmt.Fprintln(w, "  overrides <workspace>    Set path exclusion glob patterns (--set \"dist/**,test/**\")")
	fmt.Fprintln(w)
}

func printToolsDoc(w io.Writer) {
	fmt.Fprintln(w, "NAME:")
	fmt.Fprintln(w, "  bluntcode tools - Inspect and manage hermetic analyzer binaries")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SUBCOMMANDS:")
	fmt.Fprintln(w, "  list                     List all managed tools and their install status")
	fmt.Fprintln(w, "  install <id>             Download and verify managed analyzer binary")
	fmt.Fprintln(w, "  uninstall <id>           Remove tool and reclaim disk space")
	fmt.Fprintln(w, "  repair <id>              Reinstall and verify analyzer")
	fmt.Fprintln(w, "  update <id>              Update analyzer to latest manifest version")
	fmt.Fprintln(w)
}

func printPentestDoc(w io.Writer) {
	fmt.Fprintln(w, "NAME:")
	fmt.Fprintln(w, "  bluntcode pentest - Dynamic DAST security probing and vulnerability audit")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SUBCOMMANDS:")
	fmt.Fprintln(w, "  probe <url>              Audit HTTP endpoint for security headers, CORS, TLS, and leaks")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "OPTIONS:")
	fmt.Fprintln(w, "  --auth-mode bearer|basic|cookie")
	fmt.Fprintln(w, "  --auth-token <credentials>")
	fmt.Fprintln(w, "  --scope standard|full|spider")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "EXAMPLES:")
	fmt.Fprintln(w, "  bluntcode pentest probe http://localhost:8080")
	fmt.Fprintln(w, "  bluntcode pentest probe https://myapp.example.com --scope full --json")
}

func printStatsDoc(w io.Writer) {
	fmt.Fprintln(w, "NAME:")
	fmt.Fprintln(w, "  bluntcode stats, trends, risk - Metrics, risk scores, and trendlines")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "COMMANDS:")
	fmt.Fprintln(w, "  bluntcode stats [workspace]    Overall aggregate figures and severity counts")
	fmt.Fprintln(w, "  bluntcode trends <workspace>   Historical severity counts over time")
	fmt.Fprintln(w, "  bluntcode risk <workspace>     Risk grade (A-D) and weighted score")
	fmt.Fprintln(w)
}

func printDoctorDoc(w io.Writer) {
	fmt.Fprintln(w, "NAME:")
	fmt.Fprintln(w, "  bluntcode doctor - Environment diagnostics and health repair")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "OPTIONS:")
	fmt.Fprintln(w, "  --fix    Automatically repair missing folders and corrupted installations")
	fmt.Fprintln(w, "  --json   Output machine-readable diagnostics")
	fmt.Fprintln(w)
}

func printConfigDoc(w io.Writer) {
	fmt.Fprintln(w, "NAME:")
	fmt.Fprintln(w, "  bluntcode config - Print resolved system paths and settings")
	fmt.Fprintln(w)
}

func printAgentDoc(w io.Writer) {
	fmt.Fprintln(w, "NAME:")
	fmt.Fprintln(w, "  bluntcode agent - AI Agent and LLM integration helper")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "COMMANDS:")
	fmt.Fprintln(w, "  bluntcode agent docs           Print llm.txt developer/agent guide")
	fmt.Fprintln(w, "  bluntcode agent scan <path>    Run scan with automated --json --quiet defaults")
	fmt.Fprintln(w, "  bluntcode llm                  Print llm.txt to stdout")
	fmt.Fprintln(w)
}

func printCleanDoc(w io.Writer) {
	fmt.Fprintln(w, "NAME:")
	fmt.Fprintln(w, "  bluntcode clean - Reclaim disk space by purging old logs, vulnerability caches, and compacting the database")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SYNOPSIS:")
	fmt.Fprintln(w, "  bluntcode clean [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "OPTIONS:")
	fmt.Fprintln(w, "  --logs     Prune scan logs older than 7 days")
	fmt.Fprintln(w, "  --cache    Clear Trivy vulnerability database cache (~1.3 GB)")
	fmt.Fprintln(w, "  --vacuum   Compact SQLite database (VACUUM)")
	fmt.Fprintln(w, "  --all      Perform all cleanup operations (default if no flags given)")
	fmt.Fprintln(w, "  --json     Output cleanup summary in JSON format")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "EXAMPLES:")
	fmt.Fprintln(w, "  bluntcode clean")
	fmt.Fprintln(w, "  bluntcode clean --logs")
	fmt.Fprintln(w, "  bluntcode clean --cache")
}

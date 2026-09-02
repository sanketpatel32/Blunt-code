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

	fmt.Fprintln(w, "Blunt Code provides comprehensive CLI support for every core capability.")
	fmt.Fprintln(w, "All commands support `--json` for machine automation and CI/CD pipelines.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "COMMAND CATEGORIES:")
	fmt.Fprintln(w, "  1. Scans & CI Gates           `scan`, `prune`")
	fmt.Fprintln(w, "  2. Workspace Management       `workspace <list|add|show|tree|tags|delete>`")
	fmt.Fprintln(w, "  3. Findings & Inspection      `findings <search|list|preview>`")
	fmt.Fprintln(w, "  4. Reports & Exports          `report <scan|path> [--format md|sarif|html|json]`")
	fmt.Fprintln(w, "  5. History & Compare          `history [path]`, `history compare <id1> <id2>`")
	fmt.Fprintln(w, "  6. Analyzer Toolchain         `tools <list|install|repair|update>`")
	fmt.Fprintln(w, "  7. Rules & Suppressions       `suppress <list|add|remove|import>`, `rules`")
	fmt.Fprintln(w, "  8. Dynamic Pentest & DAST     `pentest probe <url>`")
	fmt.Fprintln(w, "  9. Stats, Trends & Risk       `stats`, `trends`, `risk`")
	fmt.Fprintln(w, "  10. Diagnostics & Updates     `doctor`, `config`, `update`")
	fmt.Fprintln(w, "  11. AI Agents & Scripts       `agent docs`, `agent scan`, `llm`")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run `bluntcode cli <command>` for detailed command manual and recipes.")
	fmt.Fprintln(w, "For the web documentation, navigate to http://127.0.0.1:<port>/cli")
}

func printScanDoc(w io.Writer) {
	fmt.Fprintln(w, "NAME:")
	fmt.Fprintln(w, "  bluntcode scan - Run a headless, automated code quality and security scan")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SYNOPSIS:")
	fmt.Fprintln(w, "  bluntcode scan <path> [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "OPTIONS:")
	fmt.Fprintln(w, "  --profile quick|standard|deep  Select analyzer depth (default: standard)")
	fmt.Fprintln(w, "  --fail-on <severity+>          Exit 1 if findings remain at or above severity (e.g. high+, critical)")
	fmt.Fprintln(w, "  --max-findings N               Exit 1 if total findings exceed N")
	fmt.Fprintln(w, "  --baseline <id-or-sarif>       Compare against baseline and trip gate only on NEW findings")
	fmt.Fprintln(w, "  --format text|json|sarif|...   Output document format (text, json, github, sarif, csv, markdown)")
	fmt.Fprintln(w, "  --output <file>                Write report to file instead of stdout")
	fmt.Fprintln(w, "  --incremental                  Only scan files modified since previous completed scan")
	fmt.Fprintln(w, "  --jobs N                       Run up to N analyzers concurrently")
	fmt.Fprintln(w, "  --watch                        Watch directory and automatically rescan on change")
	fmt.Fprintln(w, "  --quiet                        Suppress analyzer progress lines on stderr")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "EXAMPLES:")
	fmt.Fprintln(w, "  bluntcode scan . --profile quick")
	fmt.Fprintln(w, "  bluntcode scan . --fail-on high+ --format github")
	fmt.Fprintln(w, "  bluntcode scan C:\\projects\\api --format sarif --output audit.sarif")
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

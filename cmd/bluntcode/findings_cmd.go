package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/database"
	"bluntcode/internal/scans"
	"bluntcode/internal/workspace"
)

func printFindingsHelp(w io.Writer) {
	fmt.Fprintln(w, "Blunt Code findings search and inspection")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  bluntcode findings search <query> [--workspace <id|path>] [--severity <sev>] [--analyzer <id>] [--limit N] [--json]")
	fmt.Fprintln(w, "  bluntcode findings list <scan-id|workspace-path> [--severity <sev>] [--analyzer <id>] [--format text|json|jsonl|csv] [--output <file>]")
	fmt.Fprintln(w, "  bluntcode findings preview <scan-id> <finding-id> [--lines N] [--json]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  bluntcode findings search \"eval\"")
	fmt.Fprintln(w, "  bluntcode findings search \"SQL injection\" --severity critical,high")
	fmt.Fprintln(w, "  bluntcode findings list . --severity high+")
	fmt.Fprintln(w, "  bluntcode findings list <scan-id> --format csv --output findings.csv")
	fmt.Fprintln(w, "  bluntcode findings preview <scan-id> <finding-id>")
}

func runFindings(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printFindingsHelp(stdout)
		return 0
	}

	app, release, err := openDBOnly()
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode findings: %v\n", err)
		return 1
	}
	defer release()
	ctx := context.Background()

	switch args[0] {
	case "search":
		return runFindingsSearch(ctx, app, args[1:], stdout, stderr)
	case "list", "ls":
		return runFindingsList(ctx, app, args[1:], stdout, stderr)
	case "preview", "show":
		return runFindingsPreview(ctx, app, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "bluntcode findings: unknown subcommand %q\n", args[0])
		printFindingsHelp(stderr)
		return 2
	}
}

func runFindingsSearch(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("findings search", flag.ContinueOnError)
	flags.SetOutput(stderr)
	workspaceFlag := flags.String("workspace", "", "scope search to a specific workspace ID or path")
	severityFlag := flags.String("severity", "", "comma-separated severities: critical, high, medium, low, info")
	analyzerFlag := flags.String("analyzer", "", "filter by analyzer ID (e.g. semgrep, ruff, secrets)")
	limitFlag := flags.Int("limit", 50, "maximum number of results to return (1-500)")
	jsonOut := flags.Bool("json", false, "output JSON array")
	positional, err := parseFlagsInterspersed(flags, args)
	if err != nil {
		return 2
	}

	if len(positional) == 0 {
		fmt.Fprintln(stderr, "bluntcode findings search: missing search query")
		fmt.Fprintln(stderr, "usage: bluntcode findings search <query> [flags]")
		return 2
	}
	query := strings.Join(positional, " ")

	workspaceID := ""
	if *workspaceFlag != "" {
		work, err := resolveWorkspace(ctx, app.db, *workspaceFlag)
		if err != nil {
			fmt.Fprintf(stderr, "bluntcode findings search: %v\n", err)
			return 1
		}
		workspaceID = work.ID
	}

	limit := *limitFlag
	if limit < 1 {
		limit = 1
	}
	if limit > 500 {
		limit = 500
	}

	var severities []string
	if *severityFlag != "" {
		for _, s := range strings.Split(*severityFlag, ",") {
			if t := strings.TrimSpace(s); t != "" {
				severities = append(severities, t)
			}
		}
	}

	filter := database.GlobalFindingsFilter{
		Query:       query,
		WorkspaceID: workspaceID,
		Severities:  severities,
		Analyzer:    *analyzerFlag,
		Page:        1,
		PageSize:    limit,
	}

	results, total, err := app.db.SearchFindings(ctx, filter)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode findings search: %v\n", err)
		return 1
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, map[string]any{
			"query":    query,
			"total":    total,
			"returned": len(results),
			"findings": results,
		})
		return 0
	}

	if len(results) == 0 {
		fmt.Fprintf(stdout, "No findings matching query %q.\n", query)
		return 0
	}

	fmt.Fprintf(stdout, "Found %d finding(s) matching %q (showing %d):\n\n", total, query, len(results))
	headers := []string{"SEV", "ANALYZER", "LOCATION", "RULE", "MESSAGE"}
	rows := make([][]string, len(results))
	for i, r := range results {
		loc := fmt.Sprintf("%s:%d", r.RelativePath, r.StartLine)
		msg := r.Message
		if len(msg) > 60 {
			msg = msg[:57] + "..."
		}
		rows[i] = []string{string(r.Severity), r.AnalyzerID, loc, r.RuleID, msg}
	}
	writeTable(stdout, headers, rows)
	return 0
}

func runFindingsList(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("findings list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	severityFlag := flags.String("severity", "", "filter by severity (e.g. critical, high+, medium)")
	analyzerFlag := flags.String("analyzer", "", "filter by analyzer ID")
	formatFlag := flags.String("format", "text", "output format: text, json, jsonl, csv")
	outputFlag := flags.String("output", "", "file path to write results into")
	jsonOut := flags.Bool("json", false, "alias for --format json")
	positional, err := parseFlagsInterspersed(flags, args)
	if err != nil {
		return 2
	}

	if *jsonOut {
		*formatFlag = "json"
	}

	target := ""
	if len(positional) > 0 {
		target = positional[0]
	}

	scan, err := resolveScan(ctx, app.db, target)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode findings list: %v\n", err)
		return 1
	}

	rawFindings, err := app.db.Findings(ctx, scan.ID)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode findings list: %v\n", err)
		return 1
	}

	// Filter suppressed
	suppressed, _ := app.db.SuppressedFingerprints(ctx, scan.WorkspaceID)
	findings := scans.FilterSuppressed(rawFindings, suppressed)

	// Filter by severity if requested
	if *severityFlag != "" {
		spec, parseErr := scans.ParseSeveritySpec(*severityFlag)
		if parseErr != nil {
			fmt.Fprintf(stderr, "bluntcode findings list: invalid severity: %v\n", parseErr)
			return 2
		}
		var filtered []analyzers.Finding
		for _, f := range findings {
			if spec.Matches(f.Severity) {
				filtered = append(filtered, f)
			}
		}
		findings = filtered
	}

	// Filter by analyzer if requested
	if *analyzerFlag != "" {
		reqAnalyzer := strings.ToLower(strings.TrimSpace(*analyzerFlag))
		var filtered []analyzers.Finding
		for _, f := range findings {
			if strings.ToLower(f.AnalyzerID) == reqAnalyzer {
				filtered = append(filtered, f)
			}
		}
		findings = filtered
	}

	// Select destination
	dest := stdout
	if *outputFlag != "" {
		f, createErr := os.Create(*outputFlag)
		if createErr != nil {
			fmt.Fprintf(stderr, "bluntcode findings list: could not create output file: %v\n", createErr)
			return 1
		}
		defer f.Close()
		dest = f
	}

	switch strings.ToLower(*formatFlag) {
	case "json":
		return outputJSONFindings(dest, findings)
	case "jsonl":
		enc := json.NewEncoder(dest)
		for _, f := range findings {
			_ = enc.Encode(f)
		}
		return 0
	case "csv":
		cw := csv.NewWriter(dest)
		_ = cw.Write([]string{"severity", "category", "analyzer", "rule_id", "message", "file", "line", "end_line"})
		for _, f := range findings {
			_ = cw.Write([]string{
				string(f.Severity),
				string(f.Category),
				f.AnalyzerID,
				f.RuleID,
				f.Message,
				f.RelativePath,
				strconv.Itoa(f.StartLine),
				strconv.Itoa(f.EndLine),
			})
		}
		cw.Flush()
		return 0
	case "text":
		if len(findings) == 0 {
			fmt.Fprintf(stdout, "No findings for scan %s.\n", scan.ID)
			return 0
		}
		fmt.Fprintf(dest, "Findings for scan %s (%d findings):\n\n", scan.ID, len(findings))
		headers := []string{"SEV", "ANALYZER", "LOCATION", "RULE", "MESSAGE"}
		rows := make([][]string, len(findings))
		for i, f := range findings {
			loc := fmt.Sprintf("%s:%d", f.RelativePath, f.StartLine)
			msg := f.Message
			if len(msg) > 60 {
				msg = msg[:57] + "..."
			}
			rows[i] = []string{string(f.Severity), f.AnalyzerID, loc, f.RuleID, msg}
		}
		writeTable(dest, headers, rows)
		return 0
	default:
		fmt.Fprintf(stderr, "bluntcode findings list: unknown format %q (allowed: text, json, jsonl, csv)\n", *formatFlag)
		return 2
	}
}

func outputJSONFindings(w io.Writer, findings []analyzers.Finding) int {
	if findings == nil {
		findings = []analyzers.Finding{}
	}
	_ = writeJSONOutput(w, findings)
	return 0
}

func runFindingsPreview(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("findings preview", flag.ContinueOnError)
	flags.SetOutput(stderr)
	linesAround := flags.Int("lines", 5, "number of context lines before and after finding")
	jsonOut := flags.Bool("json", false, "output JSON excerpt")
	positional, err := parseFlagsInterspersed(flags, args)
	if err != nil {
		return 2
	}

	if len(positional) < 2 {
		fmt.Fprintln(stderr, "usage: bluntcode findings preview <scan-id> <finding-id> [--lines N] [--json]")
		return 2
	}
	scanID := positional[0]
	findingID := positional[1]

	scan, err := app.db.Scan(ctx, scanID)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode findings preview: scan not found: %v\n", err)
		return 1
	}

	finding, err := app.db.Finding(ctx, scanID, findingID)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode findings preview: finding not found: %v\n", err)
		return 1
	}

	work, err := app.db.Workspace(ctx, scan.WorkspaceID)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode findings preview: workspace not found: %v\n", err)
		return 1
	}

	rel, err := workspace.ValidateRelativePath(work.RootPath, filepath.FromSlash(finding.RelativePath))
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode findings preview: invalid relative path: %v\n", err)
		return 1
	}

	absPath := filepath.Join(work.RootPath, rel)
	data, err := os.ReadFile(absPath)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode findings preview: read file: %v\n", err)
		return 1
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	start := finding.StartLine
	end := finding.EndLine
	if start < 1 {
		start = 1
	}
	if end < start {
		end = start
	}

	contextLines := *linesAround
	firstLine := start - contextLines
	if firstLine < 1 {
		firstLine = 1
	}
	lastLine := end + contextLines
	if lastLine > len(lines) {
		lastLine = len(lines)
	}

	if *jsonOut {
		type lineSnippet struct {
			Number  int    `json:"number"`
			Content string `json:"content"`
			Match   bool   `json:"match"`
		}
		var snippet []lineSnippet
		for i := firstLine; i <= lastLine; i++ {
			snippet = append(snippet, lineSnippet{
				Number:  i,
				Content: lines[i-1],
				Match:   i >= start && i <= end,
			})
		}
		_ = writeJSONOutput(stdout, map[string]any{
			"finding":    finding,
			"file_path":  finding.RelativePath,
			"start_line": start,
			"end_line":   end,
			"snippet":    snippet,
		})
		return 0
	}

	fmt.Fprintf(stdout, "[%s] %s (%s)\n", finding.Severity, finding.Message, finding.RuleID)
	fmt.Fprintf(stdout, "File: %s:%d-%d (Analyzer: %s)\n\n", finding.RelativePath, start, end, finding.AnalyzerID)

	for i := firstLine; i <= lastLine; i++ {
		prefix := "  "
		if i >= start && i <= end {
			prefix = "> "
		}
		fmt.Fprintf(stdout, "%s%4d | %s\n", prefix, i, lines[i-1])
	}
	return 0
}

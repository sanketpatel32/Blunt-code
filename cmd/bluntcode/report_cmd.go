package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/reports"
	"bluntcode/internal/scans"
)

const reportUsage = "usage: bluntcode report <scan-id|workspace-path> [--format md|sarif|html|json|csv|jsonl] [--output <file>]"

func printReportHelp(w io.Writer) {
	fmt.Fprintln(w, "Blunt Code report exporter")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Exports a comprehensive report for any scan in multiple standard formats.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  bluntcode report <scan-id|workspace-path> [--format md|sarif|html|json|csv|jsonl] [--output <file>]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Formats:")
	fmt.Fprintln(w, "  md, markdown   Human-readable Markdown audit report (default)")
	fmt.Fprintln(w, "  sarif          OASIS SARIF 2.1.0 log (for GitHub Code Scanning & CI)")
	fmt.Fprintln(w, "  html           Self-contained interactive HTML executive report")
	fmt.Fprintln(w, "  json           Structured JSON report model")
	fmt.Fprintln(w, "  csv            Comma-separated spreadsheet of findings")
	fmt.Fprintln(w, "  jsonl          Newline-delimited findings stream")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  bluntcode report .")
	fmt.Fprintln(w, "  bluntcode report . --format sarif --output results.sarif")
	fmt.Fprintln(w, "  bluntcode report <scan-id> --format html --output report.html")
}

func runReport(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printReportHelp(stdout)
		return 0
	}

	flags := flag.NewFlagSet("report", flag.ContinueOnError)
	flags.SetOutput(stderr)
	formatFlag := flags.String("format", "md", "output format: md, sarif, html, json, csv, jsonl")
	outputFlag := flags.String("output", "", "file path to write report into (default: stdout)")
	jsonOut := flags.Bool("json", false, "alias for --format json")
	positional, err := parseFlagsInterspersed(flags, args)
	if err != nil {
		return 2
	}

	if *jsonOut {
		*formatFlag = "json"
	}

	if len(positional) == 0 {
		fmt.Fprintln(stderr, "bluntcode report: missing scan ID or workspace path")
		fmt.Fprintln(stderr, reportUsage)
		return 2
	}
	target := positional[0]

	app, release, err := openDBOnly()
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode report: %v\n", err)
		return 1
	}
	defer release()
	ctx := context.Background()

	scan, err := resolveScan(ctx, app.db, target)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode report: %v\n", err)
		return 1
	}

	work, err := app.db.Workspace(ctx, scan.WorkspaceID)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode report: load workspace: %v\n", err)
		return 1
	}

	rawFindings, err := app.db.Findings(ctx, scan.ID)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode report: load findings: %v\n", err)
		return 1
	}

	suppressed, _ := app.db.SuppressedFingerprints(ctx, work.ID)
	findings := scans.FilterSuppressed(rawFindings, suppressed)

	metrics, _ := app.db.Metrics(ctx, scan.ID)
	runs, _ := app.db.AnalyzerRuns(ctx, scan.ID)

	var comparison reports.Comparison
	if prevID, prevErr := app.db.PreviousCompletedScanID(ctx, work.ID, scan.ID); prevErr == nil && prevID != "" {
		if prevRaw, prevFindingsErr := app.db.Findings(ctx, prevID); prevFindingsErr == nil {
			prevFiltered := scans.FilterSuppressed(prevRaw, suppressed)
			prevMap := make(map[string]analyzers.Finding, len(prevFiltered))
			for _, pf := range prevFiltered {
				prevMap[pf.Fingerprint] = pf
			}
			currMap := make(map[string]analyzers.Finding, len(findings))
			for _, cf := range findings {
				currMap[cf.Fingerprint] = cf
				if _, ok := prevMap[cf.Fingerprint]; ok {
					comparison.Persistent = append(comparison.Persistent, cf)
				} else {
					comparison.New = append(comparison.New, cf)
				}
			}
			for fp, pf := range prevMap {
				if _, ok := currMap[fp]; !ok {
					comparison.Fixed = append(comparison.Fixed, pf)
				}
			}
		}
	}

	startedAt := time.Now().UTC()
	if scan.StartedAt != nil {
		startedAt = *scan.StartedAt
	}
	finishedAt := startedAt
	if scan.FinishedAt != nil {
		finishedAt = *scan.FinishedAt
	}

	input := reports.Input{
		WorkspaceName:    work.Name,
		WorkspacePath:    work.RootPath,
		ScanID:           scan.ID,
		Profile:          scan.Profile,
		BluntCodeVersion: version,
		State:            scan.State,
		StartedAt:        startedAt,
		FinishedAt:       finishedAt,
		Findings:         findings,
		Metrics:          metrics,
		Runs:             runs,
		Comparison:       comparison,
	}
	model := reports.Build(input)

	dest := stdout
	if *outputFlag != "" {
		f, createErr := os.Create(*outputFlag)
		if createErr != nil {
			fmt.Fprintf(stderr, "bluntcode report: create output file: %v\n", createErr)
			return 1
		}
		defer f.Close()
		dest = f
	}

	format := strings.ToLower(strings.TrimSpace(*formatFlag))
	var document []byte
	switch format {
	case "md", "markdown":
		document = []byte(reports.Markdown(model))
	case "sarif":
		document = reports.SARIFBytes(model)
	case "html":
		document = reports.HTML(model)
	case "json":
		document = reports.JSON(model)
	case "csv":
		document = reports.CSV(model)
	case "jsonl":
		document = reports.JSONL(model)
	default:
		fmt.Fprintf(stderr, "bluntcode report: unsupported format %q (allowed: md, sarif, html, json, csv, jsonl)\n", format)
		return 2
	}

	if _, err := dest.Write(document); err != nil {
		fmt.Fprintf(stderr, "bluntcode report: write error: %v\n", err)
		return 1
	}

	if *outputFlag != "" {
		fmt.Fprintf(stdout, "Report exported to %s (format: %s).\n", *outputFlag, format)
	}
	return 0
}

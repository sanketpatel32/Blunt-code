package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/database"
	"bluntcode/internal/scans"
)

func printHistoryHelp(w io.Writer) {
	fmt.Fprintln(w, "Blunt Code scan history and comparison")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  bluntcode history [workspace-id|path] [--limit N] [--json]")
	fmt.Fprintln(w, "  bluntcode history delete <scan-id> [--json]")
	fmt.Fprintln(w, "  bluntcode history compare <scan-id-1> <scan-id-2> [--json]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  bluntcode history")
	fmt.Fprintln(w, "  bluntcode history . --limit 10")
	fmt.Fprintln(w, "  bluntcode history delete <scan-id>")
	fmt.Fprintln(w, "  bluntcode history compare <scan-id-1> <scan-id-2>")
}

func runHistory(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		printHistoryHelp(stdout)
		return 0
	}

	app, release, err := openDBOnly()
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode history: %v\n", err)
		return 1
	}
	defer release()
	ctx := context.Background()

	if len(args) > 0 {
		switch args[0] {
		case "delete", "rm":
			return runHistoryDelete(ctx, app, args[1:], stdout, stderr)
		case "compare", "diff":
			return runHistoryCompare(ctx, app, args[1:], stdout, stderr)
		}
	}

	return runHistoryList(ctx, app, args, stdout, stderr)
}

func runHistoryList(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("history", flag.ContinueOnError)
	flags.SetOutput(stderr)
	limitFlag := flags.Int("limit", 20, "maximum number of scans to list (1-100)")
	jsonOut := flags.Bool("json", false, "output JSON array")
	positional, err := parseFlagsInterspersed(flags, args)
	if err != nil {
		return 2
	}

	limit := *limitFlag
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}

	// If workspace argument passed
	if len(positional) > 0 {
		work, err := resolveWorkspace(ctx, app.db, positional[0])
		if err != nil {
			fmt.Fprintf(stderr, "bluntcode history: %v\n", err)
			return 1
		}
		scansList, err := app.db.Scans(ctx, work.ID)
		if err != nil {
			fmt.Fprintf(stderr, "bluntcode history: %v\n", err)
			return 1
		}
		if len(scansList) > limit {
			scansList = scansList[:limit]
		}

		if *jsonOut {
			_ = writeJSONOutput(stdout, scansList)
			return 0
		}

		if len(scansList) == 0 {
			fmt.Fprintf(stdout, "No scans recorded for workspace %s.\n", work.Name)
			return 0
		}

		fmt.Fprintf(stdout, "Scan history for %s (%d scans):\n\n", work.Name, len(scansList))
		headers := []string{"SCAN ID", "PROFILE", "STATE", "STARTED", "DURATION"}
		rows := make([][]string, len(scansList))
		for i, s := range scansList {
			dur := "-"
			startTime := "-"
			if s.StartedAt != nil {
				startTime = formatTime(*s.StartedAt)
				if s.FinishedAt != nil {
					dur = formatDuration(s.FinishedAt.Sub(*s.StartedAt))
				} else if s.State == "running" {
					dur = formatDuration(time.Since(*s.StartedAt))
				}
			}
			rows[i] = []string{s.ID, s.Profile, s.State, startTime, dur}
		}
		writeTable(stdout, headers, rows)
		return 0
	}

	// Global recent scans across workspaces
	recent, _, err := app.db.RecentScans(ctx, database.RecentScansFilter{Limit: limit})
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode history: %v\n", err)
		return 1
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, recent)
		return 0
	}

	if len(recent) == 0 {
		fmt.Fprintln(stdout, "No recent scans found. Run `bluntcode scan <path>` to execute a scan.")
		return 0
	}

	fmt.Fprintf(stdout, "Recent scans (%d):\n\n", len(recent))
	headers := []string{"SCAN ID", "WORKSPACE", "PROFILE", "STATE", "FINDINGS (C/H/M/L)", "STARTED", "DURATION"}
	rows := make([][]string, len(recent))
	for i, r := range recent {
		dur := "-"
		startTime := "-"
		if r.StartedAt != nil {
			startTime = formatTime(*r.StartedAt)
			if r.FinishedAt != nil {
				dur = formatDuration(r.FinishedAt.Sub(*r.StartedAt))
			} else if r.State == "running" {
				dur = formatDuration(time.Since(*r.StartedAt))
			}
		}
		counts := fmt.Sprintf("%d / %d / %d / %d", r.CriticalCount, r.HighCount, r.MediumCount, r.LowCount)
		rows[i] = []string{r.ID, r.WorkspaceName, r.Profile, r.State, counts, startTime, dur}
	}
	writeTable(stdout, headers, rows)
	return 0
}

func runHistoryDelete(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("history delete", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOut := flags.Bool("json", false, "output JSON")
	positional, err := parseFlagsInterspersed(flags, args)
	if err != nil {
		return 2
	}

	if len(positional) == 0 {
		fmt.Fprintln(stderr, "bluntcode history delete: missing scan ID")
		return 2
	}
	scanID := positional[0]

	deleted, err := app.db.DeleteScan(ctx, scanID)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode history delete: %v\n", err)
		return 1
	}
	if !deleted {
		fmt.Fprintf(stderr, "bluntcode history delete: scan %s not found\n", scanID)
		return 1
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, map[string]any{"deleted": true, "scan_id": scanID})
		return 0
	}

	fmt.Fprintf(stdout, "Deleted scan %s.\n", scanID)
	return 0
}

func runHistoryCompare(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("history compare", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOut := flags.Bool("json", false, "output JSON comparison")
	positional, err := parseFlagsInterspersed(flags, args)
	if err != nil {
		return 2
	}

	if len(positional) < 2 {
		fmt.Fprintln(stderr, "usage: bluntcode history compare <scan-id-1> <scan-id-2> [--json]")
		return 2
	}
	id1 := positional[0]
	id2 := positional[1]

	scan1, err := app.db.Scan(ctx, id1)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode history compare: scan %s not found: %v\n", id1, err)
		return 1
	}
	scan2, err := app.db.Scan(ctx, id2)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode history compare: scan %s not found: %v\n", id2, err)
		return 1
	}

	f1, err := app.db.Findings(ctx, scan1.ID)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode history compare: load findings for %s: %v\n", id1, err)
		return 1
	}
	f2, err := app.db.Findings(ctx, scan2.ID)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode history compare: load findings for %s: %v\n", id2, err)
		return 1
	}

	// Filter suppressions for scan 2's workspace
	suppressed, _ := app.db.SuppressedFingerprints(ctx, scan2.WorkspaceID)
	f1 = scans.FilterSuppressed(f1, suppressed)
	f2 = scans.FilterSuppressed(f2, suppressed)

	map1 := make(map[string]analyzers.Finding)
	for _, f := range f1 {
		map1[f.Fingerprint] = f
	}
	map2 := make(map[string]analyzers.Finding)
	for _, f := range f2 {
		map2[f.Fingerprint] = f
	}

	var newFindings, fixedFindings, persistentFindings []analyzers.Finding
	for fp, f := range map2 {
		if _, ok := map1[fp]; ok {
			persistentFindings = append(persistentFindings, f)
		} else {
			newFindings = append(newFindings, f)
		}
	}
	for fp, f := range map1 {
		if _, ok := map2[fp]; !ok {
			fixedFindings = append(fixedFindings, f)
		}
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, map[string]any{
			"scan1": map[string]any{"id": scan1.ID, "started_at": scan1.StartedAt, "total": len(f1)},
			"scan2": map[string]any{"id": scan2.ID, "started_at": scan2.StartedAt, "total": len(f2)},
			"counts": map[string]int{
				"new":        len(newFindings),
				"fixed":      len(fixedFindings),
				"persistent": len(persistentFindings),
			},
			"new":        newFindings,
			"fixed":      fixedFindings,
			"persistent": persistentFindings,
		})
		return 0
	}

	t1, t2 := "-", "-"
	if scan1.StartedAt != nil {
		t1 = formatTime(*scan1.StartedAt)
	}
	if scan2.StartedAt != nil {
		t2 = formatTime(*scan2.StartedAt)
	}

	fmt.Fprintf(stdout, "Scan Comparison: %s vs %s\n\n", scan1.ID, scan2.ID)
	fmt.Fprintf(stdout, "  Baseline (%s): %d findings\n", t1, len(f1))
	fmt.Fprintf(stdout, "  Current  (%s): %d findings\n\n", t2, len(f2))
	fmt.Fprintf(stdout, "Summary:\n")
	fmt.Fprintf(stdout, "  + New findings:        %d\n", len(newFindings))
	fmt.Fprintf(stdout, "  - Fixed findings:      %d\n", len(fixedFindings))
	fmt.Fprintf(stdout, "  = Persistent findings: %d\n", len(persistentFindings))

	if len(newFindings) > 0 {
		fmt.Fprintf(stdout, "\nNew findings in %s:\n", scan2.ID)
		for _, f := range newFindings {
			fmt.Fprintf(stdout, "  + [%s] %s (%s:%d)\n", f.Severity, f.Message, f.RelativePath, f.StartLine)
		}
	}
	if len(fixedFindings) > 0 {
		fmt.Fprintf(stdout, "\nFixed findings (no longer present):\n")
		for _, f := range fixedFindings {
			fmt.Fprintf(stdout, "  - [%s] %s (%s:%d)\n", f.Severity, f.Message, f.RelativePath, f.StartLine)
		}
	}
	return 0
}

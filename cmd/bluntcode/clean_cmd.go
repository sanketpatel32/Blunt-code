package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"bluntcode/internal/cleaner"
)

func printCleanHelp(w io.Writer) {
	fmt.Fprintln(w, "Blunt Code storage cleaner")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  bluntcode clean [--logs] [--cache] [--vacuum] [--all] [--json]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  --logs     Prune scan logs older than 7 days")
	fmt.Fprintln(w, "  --cache    Clear Trivy vulnerability database cache (~1.3 GB)")
	fmt.Fprintln(w, "  --vacuum   Compact SQLite database (VACUUM)")
	fmt.Fprintln(w, "  --all      Perform all cleanup operations (default if no flags given)")
	fmt.Fprintln(w, "  --json     Output cleanup summary in JSON format")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  bluntcode clean")
	fmt.Fprintln(w, "  bluntcode clean --logs")
	fmt.Fprintln(w, "  bluntcode clean --cache")
}

func runClean(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help" || args[0] == "-help") {
		printCleanHelp(stdout)
		return 0
	}

	flags := flag.NewFlagSet("clean", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { printCleanHelp(stderr) }
	logsFlag := flags.Bool("logs", false, "prune scan logs older than 7 days")
	cacheFlag := flags.Bool("cache", false, "clear vulnerability DB cache")
	vacuumFlag := flags.Bool("vacuum", false, "compact sqlite database")
	allFlag := flags.Bool("all", false, "clean all (logs, cache, vacuum)")
	jsonFlag := flags.Bool("json", false, "output JSON format")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printCleanHelp(stdout)
			return 0
		}
		return 2
	}

	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "bluntcode clean: unexpected argument %q\n", flags.Arg(0))
		printCleanHelp(stderr)
		return 2
	}

	app, release, err := openDBOnly()
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode clean: %v\n", err)
		return 1
	}
	defer release()

	ctx := context.Background()
	if summary, err := app.db.ScanSummary(ctx); err == nil && summary.ActiveScans > 0 {
		fmt.Fprintln(stderr, "bluntcode clean: cannot clean storage while scans are running. Please wait for active scans to finish.")
		return 1
	}
	opts := cleaner.Options{
		Logs:   *logsFlag,
		Cache:  *cacheFlag,
		Vacuum: *vacuumFlag,
		All:    *allFlag,
	}

	res, err := cleaner.Clean(ctx, app.paths, app.db, opts)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode clean: %v\n", err)
		return 1
	}

	if *jsonFlag {
		_ = writeJSONOutput(stdout, res)
		return 0
	}

	fmt.Fprintln(stdout, "Blunt Code Storage Cleaner")
	fmt.Fprintln(stdout)
	for _, detail := range res.Details {
		fmt.Fprintf(stdout, "  • %s\n", detail)
	}
	for _, errStr := range res.Errors {
		fmt.Fprintf(stderr, "  ! Warning: %s\n", errStr)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Total disk space reclaimed: %s\n", cleaner.FormatBytes(res.ReclaimedBytes))
	return 0
}

package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func printSuppressHelp(w io.Writer) {
	fmt.Fprintln(w, "Blunt Code finding suppressions management")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  bluntcode suppress list <workspace-id|path> [--json|--csv]")
	fmt.Fprintln(w, "  bluntcode suppress add <workspace-id|path> --fingerprint <hash> [--reason <text>]")
	fmt.Fprintln(w, "  bluntcode suppress remove <workspace-id|path> --fingerprint <hash>")
	fmt.Fprintln(w, "  bluntcode suppress import <workspace-id|path> <csv-file>")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  bluntcode suppress list .")
	fmt.Fprintln(w, "  bluntcode suppress add . --fingerprint 7f8a... --reason \"Accepted risk in dev\"")
	fmt.Fprintln(w, "  bluntcode suppress remove . --fingerprint 7f8a...")
	fmt.Fprintln(w, "  bluntcode suppress import . suppressions.csv")
}

func runSuppress(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printSuppressHelp(stdout)
		return 0
	}

	app, release, err := openDBOnly()
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode suppress: %v\n", err)
		return 1
	}
	defer release()
	ctx := context.Background()

	switch args[0] {
	case "list", "ls":
		return runSuppressList(ctx, app, args[1:], stdout, stderr)
	case "add":
		return runSuppressAdd(ctx, app, args[1:], stdout, stderr)
	case "remove", "rm", "delete":
		return runSuppressRemove(ctx, app, args[1:], stdout, stderr)
	case "import":
		return runSuppressImport(ctx, app, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "bluntcode suppress: unknown subcommand %q\n", args[0])
		printSuppressHelp(stderr)
		return 2
	}
}

func runSuppressList(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("suppress list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOut := flags.Bool("json", false, "output JSON array")
	csvOut := flags.Bool("csv", false, "output CSV format")
	positional, err := parseFlagsInterspersed(flags, args)
	if err != nil {
		return 2
	}

	target := ""
	if len(positional) > 0 {
		target = positional[0]
	}

	work, err := resolveWorkspace(ctx, app.db, target)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode suppress list: %v\n", err)
		return 1
	}

	items, err := app.db.Suppressions(ctx, work.ID)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode suppress list: %v\n", err)
		return 1
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, items)
		return 0
	}

	if *csvOut {
		w := csv.NewWriter(stdout)
		_ = w.Write([]string{"fingerprint", "reason", "created_at"})
		for _, s := range items {
			_ = w.Write([]string{s.Fingerprint, s.Reason, formatTime(s.CreatedAt)})
		}
		w.Flush()
		return 0
	}

	if len(items) == 0 {
		fmt.Fprintf(stdout, "No active suppressions for workspace %s.\n", work.Name)
		return 0
	}

	fmt.Fprintf(stdout, "Suppressions for workspace %s (%d items):\n\n", work.Name, len(items))
	headers := []string{"FINGERPRINT", "REASON", "CREATED AT"}
	rows := make([][]string, len(items))
	for i, s := range items {
		rows[i] = []string{s.Fingerprint, s.Reason, formatTime(s.CreatedAt)}
	}
	writeTable(stdout, headers, rows)
	return 0
}

func runSuppressAdd(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("suppress add", flag.ContinueOnError)
	flags.SetOutput(stderr)
	fpFlag := flags.String("fingerprint", "", "finding fingerprint (SHA-256 hash)")
	reasonFlag := flags.String("reason", "", "justification for suppressing the finding")
	jsonOut := flags.Bool("json", false, "output JSON")
	positional, err := parseFlagsInterspersed(flags, args)
	if err != nil {
		return 2
	}

	target := ""
	if len(positional) > 0 {
		target = positional[0]
	}

	if *fpFlag == "" {
		fmt.Fprintln(stderr, "bluntcode suppress add: --fingerprint is required")
		fmt.Fprintln(stderr, "usage: bluntcode suppress add <workspace> --fingerprint <hash> [--reason <reason>]")
		return 2
	}

	work, err := resolveWorkspace(ctx, app.db, target)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode suppress add: %v\n", err)
		return 1
	}

	created, err := app.db.AddSuppression(ctx, work.ID, *fpFlag, *reasonFlag)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode suppress add: %v\n", err)
		return 1
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, created)
		return 0
	}

	fmt.Fprintf(stdout, "Suppressed finding %s in workspace %s.\n", created.Fingerprint, work.Name)
	return 0
}

func runSuppressRemove(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("suppress remove", flag.ContinueOnError)
	flags.SetOutput(stderr)
	fpFlag := flags.String("fingerprint", "", "finding fingerprint (SHA-256 hash)")
	jsonOut := flags.Bool("json", false, "output JSON")
	positional, err := parseFlagsInterspersed(flags, args)
	if err != nil {
		return 2
	}

	target := ""
	if len(positional) > 0 {
		target = positional[0]
	}

	if *fpFlag == "" {
		fmt.Fprintln(stderr, "bluntcode suppress remove: --fingerprint is required")
		return 2
	}

	work, err := resolveWorkspace(ctx, app.db, target)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode suppress remove: %v\n", err)
		return 1
	}

	if err := app.db.RemoveSuppression(ctx, work.ID, *fpFlag); err != nil {
		fmt.Fprintf(stderr, "bluntcode suppress remove: %v\n", err)
		return 1
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, map[string]any{"removed": true, "fingerprint": *fpFlag})
		return 0
	}

	fmt.Fprintf(stdout, "Unsuppressed finding %s in workspace %s.\n", *fpFlag, work.Name)
	return 0
}

func runSuppressImport(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "usage: bluntcode suppress import <workspace-id|path> <csv-file>")
		return 2
	}
	target := args[0]
	filePath := args[1]

	work, err := resolveWorkspace(ctx, app.db, target)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode suppress import: %v\n", err)
		return 1
	}

	file, err := os.Open(filePath)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode suppress import: open file: %v\n", err)
		return 1
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode suppress import: parse CSV: %v\n", err)
		return 1
	}

	count := 0
	for i, row := range records {
		if len(row) == 0 {
			continue
		}
		fp := strings.TrimSpace(row[0])
		if fp == "" || strings.EqualFold(fp, "fingerprint") {
			continue // skip header or empty row
		}
		reason := "Imported from CSV"
		if len(row) > 1 && strings.TrimSpace(row[1]) != "" {
			reason = strings.TrimSpace(row[1])
		}
		if _, addErr := app.db.AddSuppression(ctx, work.ID, fp, reason); addErr == nil {
			count++
		} else {
			fmt.Fprintf(stderr, "warning: line %d: %v\n", i+1, addErr)
		}
	}

	fmt.Fprintf(stdout, "Imported %d suppression(s) into workspace %s.\n", count, work.Name)
	return 0
}

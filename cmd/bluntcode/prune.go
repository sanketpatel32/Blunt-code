package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

const pruneUsage = "usage: bluntcode prune <path> [--keep N]"

func parsePruneFlags(args []string, errOut io.Writer) (string, int, error) {
	flags := flag.NewFlagSet("prune", flag.ContinueOnError)
	flags.SetOutput(errOut)
	keep := flags.Int("keep", 20, "keep the N most recent terminal scans (1-100)")
	flags.Usage = func() {
		fmt.Fprintln(errOut, pruneUsage)
		fmt.Fprintln(errOut)
		fmt.Fprintln(errOut, "Prunes old scans for the workspace at <path>, keeping the N most recent")
		fmt.Fprintln(errOut, "terminal scans. Running scans are never deleted.")
		flags.PrintDefaults()
	}
	var positional []string
	remaining := args
	for {
		if err := flags.Parse(remaining); err != nil {
			return "", 0, err
		}
		if flags.NArg() == 0 {
			break
		}
		positional = append(positional, flags.Arg(0))
		remaining = flags.Args()[1:]
	}
	usageError := func(message string) (string, int, error) {
		fmt.Fprintf(errOut, "bluntcode prune: %s\n", message)
		flags.Usage()
		return "", 0, errors.New(message)
	}
	if len(positional) != 1 {
		return usageError("exactly one workspace path is required")
	}
	if *keep < 1 || *keep > 100 {
		return usageError("keep must be between 1 and 100")
	}
	return positional[0], *keep, nil
}

func runPrune(args []string) {
	path, keep, err := parsePruneFlags(args, os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		os.Exit(2)
	}
	app, release, err := openCore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bluntcode prune: %v\n", err)
		os.Exit(1)
	}
	defer release()
	ctx := context.Background()
	work, err := ensureWorkspace(ctx, app.db, path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bluntcode prune: %v\n", err)
		os.Exit(1)
	}
	deleted, err := app.db.PruneOldScans(ctx, work.ID, keep)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bluntcode prune: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stdout, "pruned %d scan(s), kept %d most recent for workspace %s\n", deleted, keep, work.Name)
}

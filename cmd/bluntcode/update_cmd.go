package main

import (
	"flag"
	"fmt"
	"io"

	"bluntcode/internal/api"
)

func printUpdateHelp(w io.Writer) {
	fmt.Fprintln(w, "Blunt Code update check")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  bluntcode update check [--json]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  bluntcode update check")
	fmt.Fprintln(w, "  bluntcode update check --json")
}

func runUpdate(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printUpdateHelp(stdout)
		return 0
	}

	switch args[0] {
	case "check":
		return runUpdateCheck(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "bluntcode update: unknown subcommand %q\n", args[0])
		printUpdateHelp(stderr)
		return 2
	}
}

func runUpdateCheck(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("update check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOut := flags.Bool("json", false, "output JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	res, err := api.CheckUpdateDirect(version)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode update check: %v\n", err)
		return 1
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, res)
		return 0
	}

	fmt.Fprintf(stdout, "Blunt Code Update Status:\n\n")
	fmt.Fprintf(stdout, "  Current Version:  %s\n", res.Current)
	fmt.Fprintf(stdout, "  Latest Version:   %s\n", res.Latest)
	if res.Available {
		fmt.Fprintf(stdout, "  Status:           Update Available!\n")
		fmt.Fprintf(stdout, "  Release URL:      %s\n", res.ReleaseURL)
		if res.ReleaseNotes != "" {
			fmt.Fprintf(stdout, "\nRelease Notes:\n%s\n", res.ReleaseNotes)
		}
	} else {
		fmt.Fprintf(stdout, "  Status:           You are up to date.\n")
	}
	return 0
}

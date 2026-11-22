package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"bluntcode/internal/api"
	"bluntcode/internal/selfupdate"
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
	case "apply-staged":
		// Internal: started by POST /update/apply from a copy of the
		// installed binary. Not listed in help.
		return runUpdateApplyStaged(args[1:], stderr)
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

	res, err := api.CheckUpdateDirect(context.Background(), version)
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

func runUpdateApplyStaged(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("update apply-staged", flag.ContinueOnError)
	flags.SetOutput(stderr)
	payload := flags.String("payload", "", "verified, extracted release folder")
	installDir := flags.String("install-dir", "", "installation directory to replace")
	waitPID := flags.Int("wait-pid", 0, "process to wait for before swapping")
	logPath := flags.String("log", "", "append progress and errors to this file")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *payload == "" || *installDir == "" {
		fmt.Fprintln(stderr, "bluntcode update apply-staged: --payload and --install-dir are required")
		return 2
	}
	var log io.Writer = stderr
	if *logPath != "" {
		if file, err := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			defer file.Close()
			log = file
		}
	}
	err := selfupdate.Apply(selfupdate.ApplyOptions{
		PayloadDir:  *payload,
		InstallDir:  *installDir,
		WaitPID:     *waitPID,
		WaitTimeout: 60 * time.Second,
		Relaunch:    true,
		Log:         log,
	})
	if err != nil {
		fmt.Fprintf(log, "update failed: %v\n", err)
		return 1
	}
	return 0
}

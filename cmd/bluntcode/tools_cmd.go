package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"bluntcode/internal/tools"
)

func printToolsHelp(w io.Writer) {
	fmt.Fprintln(w, "Blunt Code managed analyzer toolchain")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  bluntcode tools list [--json]")
	fmt.Fprintln(w, "  bluntcode tools install <tool-id> [--json]")
	fmt.Fprintln(w, "  bluntcode tools repair <tool-id> [--json]")
	fmt.Fprintln(w, "  bluntcode tools update <tool-id> [--json]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Tools:")
	fmt.Fprintln(w, "  ruff               Python linter and formatter")
	fmt.Fprintln(w, "  biome              JavaScript/TypeScript analyzer")
	fmt.Fprintln(w, "  gitleaks-secrets   Git secret scanning")
	fmt.Fprintln(w, "  osv-dependencies   Dependency vulnerability scanner")
	fmt.Fprintln(w, "  container-trivy    Container and filesystem scanner")
	fmt.Fprintln(w, "  iac-checkov        Infrastructure-as-Code security scanner")
	fmt.Fprintln(w, "  semgrep            Multi-language AST security rules engine")
	fmt.Fprintln(w, "  sonarqube          Deep code quality and security analysis")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  bluntcode tools list")
	fmt.Fprintln(w, "  bluntcode tools install ruff")
	fmt.Fprintln(w, "  bluntcode tools repair semgrep")
}

func runTools(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printToolsHelp(stdout)
		return 0
	}

	app, release, err := openDBOnly()
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode tools: %v\n", err)
		return 1
	}
	defer release()
	ctx := context.Background()

	manifest, err := tools.DefaultManifest()
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode tools: manifest error: %v\n", err)
		return 1
	}

	settings, _ := app.db.AppSettings(ctx)
	toolService := tools.NewService(app.paths.ToolsDir, manifest, settings.Offline)

	switch args[0] {
	case "list", "ls":
		return runToolsList(toolService, args[1:], stdout, stderr)
	case "install", "add":
		return runToolsInstall(ctx, toolService, args[1:], "install", stdout, stderr)
	case "repair":
		return runToolsInstall(ctx, toolService, args[1:], "repair", stdout, stderr)
	case "update":
		return runToolsInstall(ctx, toolService, args[1:], "update", stdout, stderr)
	default:
		fmt.Fprintf(stderr, "bluntcode tools: unknown subcommand %q\n", args[0])
		printToolsHelp(stderr)
		return 2
	}
}

func runToolsList(svc *tools.Service, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("tools list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOut := flags.Bool("json", false, "output JSON array")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	all := svc.All()
	if *jsonOut {
		_ = writeJSONOutput(stdout, all)
		return 0
	}

	fmt.Fprintln(stdout, "Blunt Code Managed Analyzers:")
	fmt.Fprintln(stdout)
	headers := []string{"ID", "NAME", "VERSION", "STATUS", "DETAILS"}
	rows := make([][]string, len(all))
	for i, s := range all {
		status := "Ready"
		if !s.Ready {
			status = "Not Installed"
		}
		rows[i] = []string{s.ID, s.Name, s.Version, status, s.Detail}
	}
	writeTable(stdout, headers, rows)
	return 0
}

func runToolsInstall(ctx context.Context, svc *tools.Service, args []string, action string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("tools "+action, flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOut := flags.Bool("json", false, "output JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if flags.NArg() == 0 {
		fmt.Fprintf(stderr, "bluntcode tools %s: missing tool ID\n", action)
		fmt.Fprintf(stderr, "usage: bluntcode tools %s <tool-id> [--json]\n", action)
		return 2
	}
	toolID := strings.TrimSpace(flags.Arg(0))

	status := svc.Status(toolID)
	if status.Name == toolID && !status.CanInstall && status.Detail == "No pinned managed artifact is configured." {
		fmt.Fprintf(stderr, "bluntcode tools %s: unknown tool ID %q\n", action, toolID)
		return 2
	}

	if !status.CanInstall {
		fmt.Fprintf(stderr, "bluntcode tools %s: %s cannot be installed: %s\n", action, toolID, status.Detail)
		return 1
	}

	fmt.Fprintf(stderr, "Preparing %s %s...\n", toolID, action)
	if err := svc.Ensure(ctx, toolID); err != nil {
		fmt.Fprintf(stderr, "bluntcode tools %s failed: %v\n", action, err)
		return 1
	}

	updatedStatus := svc.Status(toolID)
	if *jsonOut {
		_ = writeJSONOutput(stdout, updatedStatus)
		return 0
	}

	fmt.Fprintf(stdout, "Successfully completed %s for %s (%s).\n", action, updatedStatus.Name, updatedStatus.Version)
	return 0
}

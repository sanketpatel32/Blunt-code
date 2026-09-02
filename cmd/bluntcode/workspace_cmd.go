package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"bluntcode/internal/core"
	"bluntcode/internal/discovery"
)

const workspaceUsage = "usage: bluntcode workspace <list|add|show|delete|tree|tags> [arguments]"

func printWorkspaceHelp(w io.Writer) {
	fmt.Fprintln(w, "Blunt Code workspace management")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  bluntcode workspace list [--json]")
	fmt.Fprintln(w, "  bluntcode workspace add <path> [--name <name>] [--profile quick|standard|deep] [--json]")
	fmt.Fprintln(w, "  bluntcode workspace show <id|path> [--json]")
	fmt.Fprintln(w, "  bluntcode workspace delete <id|path> [--json]")
	fmt.Fprintln(w, "  bluntcode workspace tree <id|path> [--path <subpath>] [--json]")
	fmt.Fprintln(w, "  bluntcode workspace tags <id|path> [--set \"tag1,tag2\"] [--json]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  bluntcode workspace list")
	fmt.Fprintln(w, "  bluntcode workspace add . --name my-app")
	fmt.Fprintln(w, "  bluntcode workspace show my-app")
	fmt.Fprintln(w, "  bluntcode workspace tree my-app --path src")
	fmt.Fprintln(w, "  bluntcode workspace tags my-app --set \"frontend,react\"")
	fmt.Fprintln(w, "  bluntcode workspace delete my-app")
}

func runWorkspace(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printWorkspaceHelp(stdout)
		return 0
	}

	app, release, err := openDBOnly()
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode workspace: %v\n", err)
		return 1
	}
	defer release()
	ctx := context.Background()

	switch args[0] {
	case "list", "ls":
		return runWorkspaceList(ctx, app, args[1:], stdout, stderr)
	case "add", "create":
		return runWorkspaceAdd(ctx, app, args[1:], stdout, stderr)
	case "show", "get", "info":
		return runWorkspaceShow(ctx, app, args[1:], stdout, stderr)
	case "delete", "remove", "rm":
		return runWorkspaceDelete(ctx, app, args[1:], stdout, stderr)
	case "tree":
		return runWorkspaceTree(ctx, app, args[1:], stdout, stderr)
	case "tags", "tag":
		return runWorkspaceTags(ctx, app, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "bluntcode workspace: unknown subcommand %q\n", args[0])
		printWorkspaceHelp(stderr)
		return 2
	}
}

func runWorkspaceList(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("workspace list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOut := flags.Bool("json", false, "output JSON array")
	if _, err := parseFlagsInterspersed(flags, args); err != nil {
		return 2
	}

	items, err := app.db.Workspaces(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode workspace list: %v\n", err)
		return 1
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, items)
		return 0
	}

	if len(items) == 0 {
		fmt.Fprintln(stdout, "No workspaces registered. Run `bluntcode workspace add <path>` to add one.")
		return 0
	}

	headers := []string{"ID", "NAME", "ROOT PATH", "DEFAULT PROFILE", "LAST SCANNED"}
	rows := make([][]string, len(items))
	for i, w := range items {
		lastScan := "-"
		if w.LastScanAt != nil {
			lastScan = formatTime(*w.LastScanAt)
		}
		rows[i] = []string{w.ID, w.Name, w.RootPath, w.DefaultProfile, lastScan}
	}
	writeTable(stdout, headers, rows)
	return 0
}

func runWorkspaceAdd(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("workspace add", flag.ContinueOnError)
	flags.SetOutput(stderr)
	nameFlag := flags.String("name", "", "workspace display name")
	profileFlag := flags.String("profile", "standard", "default scan profile: quick, standard, deep")
	jsonOut := flags.Bool("json", false, "output JSON")
	positional, err := parseFlagsInterspersed(flags, args)
	if err != nil {
		return 2
	}

	if len(positional) == 0 {
		fmt.Fprintln(stderr, "bluntcode workspace add: missing workspace path")
		fmt.Fprintln(stderr, "usage: bluntcode workspace add <path> [--name <name>] [--profile quick|standard|deep] [--json]")
		return 2
	}
	pathArg := positional[0]

	work, err := ensureWorkspace(ctx, app.db, pathArg)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode workspace add: %v\n", err)
		return 1
	}

	// Update name or profile if explicitly set
	needUpdate := false
	if *nameFlag != "" && work.Name != *nameFlag {
		work.Name = *nameFlag
		needUpdate = true
	}
	if *profileFlag != "" && work.DefaultProfile != *profileFlag {
		work.DefaultProfile = *profileFlag
		needUpdate = true
	}
	if needUpdate {
		if updated, upErr := app.db.UpdateWorkspace(ctx, work); upErr == nil {
			work = updated
		}
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, work)
		return 0
	}

	fmt.Fprintf(stdout, "Workspace %s registered successfully.\n", work.Name)
	fmt.Fprintf(stdout, "  ID:       %s\n", work.ID)
	fmt.Fprintf(stdout, "  Root:     %s\n", work.RootPath)
	fmt.Fprintf(stdout, "  Profile:  %s\n", work.DefaultProfile)
	return 0
}

func runWorkspaceShow(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("workspace show", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOut := flags.Bool("json", false, "output JSON")
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
		fmt.Fprintf(stderr, "bluntcode workspace show: %v\n", err)
		return 1
	}

	tags, _ := app.db.GetWorkspaceTags(ctx, work.ID)
	scans, _ := app.db.Scans(ctx, work.ID)

	type workspaceDetail struct {
		core.Workspace
		Tags       []string   `json:"tags"`
		TotalScans int        `json:"total_scans"`
		LatestScan *core.Scan `json:"latest_scan,omitempty"`
	}

	detail := workspaceDetail{
		Workspace:  work,
		Tags:       tags,
		TotalScans: len(scans),
	}
	if len(scans) > 0 {
		detail.LatestScan = &scans[0]
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, detail)
		return 0
	}

	fmt.Fprintf(stdout, "Workspace: %s\n", work.Name)
	fmt.Fprintf(stdout, "  ID:              %s\n", work.ID)
	fmt.Fprintf(stdout, "  Root Path:       %s\n", work.RootPath)
	fmt.Fprintf(stdout, "  Default Profile: %s\n", work.DefaultProfile)
	fmt.Fprintf(stdout, "  Created At:      %s\n", formatTime(work.CreatedAt))
	if work.LastScanAt != nil {
		fmt.Fprintf(stdout, "  Last Scanned:    %s\n", formatTime(*work.LastScanAt))
	} else {
		fmt.Fprintf(stdout, "  Last Scanned:    -\n")
	}
	fmt.Fprintf(stdout, "  Total Scans:     %d\n", len(scans))
	if len(tags) > 0 {
		fmt.Fprintf(stdout, "  Tags:            %s\n", strings.Join(tags, ", "))
	}
	if len(scans) > 0 {
		latest := scans[0]
		startTime := "-"
		if latest.StartedAt != nil {
			startTime = formatTime(*latest.StartedAt)
		}
		fmt.Fprintf(stdout, "  Latest Scan:     %s (state: %s, profile: %s, started: %s)\n", latest.ID, latest.State, latest.Profile, startTime)
	}
	return 0
}

func runWorkspaceDelete(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("workspace delete", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOut := flags.Bool("json", false, "output JSON")
	positional, err := parseFlagsInterspersed(flags, args)
	if err != nil {
		return 2
	}

	if len(positional) == 0 {
		fmt.Fprintln(stderr, "bluntcode workspace delete: missing workspace identifier")
		return 2
	}

	work, err := resolveWorkspace(ctx, app.db, positional[0])
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode workspace delete: %v\n", err)
		return 1
	}

	if err := app.db.DeleteWorkspace(ctx, work.ID); err != nil {
		fmt.Fprintf(stderr, "bluntcode workspace delete: %v\n", err)
		return 1
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, map[string]any{"deleted": true, "id": work.ID, "name": work.Name})
		return 0
	}

	fmt.Fprintf(stdout, "Deleted workspace %s (%s).\n", work.Name, work.ID)
	return 0
}

func runWorkspaceTree(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("workspace tree", flag.ContinueOnError)
	flags.SetOutput(stderr)
	subpath := flags.String("path", "", "subpath within workspace to inspect")
	jsonOut := flags.Bool("json", false, "output JSON")
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
		fmt.Fprintf(stderr, "bluntcode workspace tree: %v\n", err)
		return 1
	}

	patterns := userExcludePatterns(ctx, app.db, work.ID)
	files, err := discovery.Tree(ctx, work.RootPath, *subpath, patterns)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode workspace tree: %v\n", err)
		return 1
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, map[string]any{
			"workspace": work.Name,
			"path":      *subpath,
			"items":     files,
		})
		return 0
	}

	fmt.Fprintf(stdout, "Tree for workspace %s (path: %q, %d items):\n", work.Name, *subpath, len(files))
	headers := []string{"TYPE", "PATH", "SIZE"}
	rows := make([][]string, 0, len(files))
	for _, f := range files {
		t := "file"
		sz := fmt.Sprintf("%d B", f.SizeBytes)
		if f.IsDir {
			t = "dir"
			sz = "-"
		}
		rows = append(rows, []string{t, f.RelativePath, sz})
	}
	writeTable(stdout, headers, rows)
	return 0
}

func runWorkspaceTags(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("workspace tags", flag.ContinueOnError)
	flags.SetOutput(stderr)
	setFlag := flags.String("set", "", "comma-separated list of tags to assign")
	jsonOut := flags.Bool("json", false, "output JSON")
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
		fmt.Fprintf(stderr, "bluntcode workspace tags: %v\n", err)
		return 1
	}

	if *setFlag != "" || isFlagPassed(flags, "set") {
		var newTags []string
		for _, part := range strings.Split(*setFlag, ",") {
			p := strings.TrimSpace(part)
			if p != "" {
				newTags = append(newTags, p)
			}
		}
		if err := app.db.SetWorkspaceTags(ctx, work.ID, newTags); err != nil {
			fmt.Fprintf(stderr, "bluntcode workspace tags: %v\n", err)
			return 1
		}
		if *jsonOut {
			_ = writeJSONOutput(stdout, map[string]any{"workspace_id": work.ID, "tags": newTags})
			return 0
		}
		fmt.Fprintf(stdout, "Updated tags for %s: %s\n", work.Name, strings.Join(newTags, ", "))
		return 0
	}

	tags, err := app.db.GetWorkspaceTags(ctx, work.ID)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode workspace tags: %v\n", err)
		return 1
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, map[string]any{"workspace_id": work.ID, "tags": tags})
		return 0
	}

	if len(tags) == 0 {
		fmt.Fprintf(stdout, "Workspace %s has no tags.\n", work.Name)
	} else {
		fmt.Fprintf(stdout, "Tags for %s: %s\n", work.Name, strings.Join(tags, ", "))
	}
	return 0
}

func isFlagPassed(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"bluntcode/internal/core"
)

func printRulesHelp(w io.Writer) {
	fmt.Fprintln(w, "Blunt Code workspace analysis rules and path overrides")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  bluntcode rules list <workspace-id|path> [--json]")
	fmt.Fprintln(w, "  bluntcode rules disable <workspace-id|path> <pattern>")
	fmt.Fprintln(w, "  bluntcode rules enable <workspace-id|path> <pattern>")
	fmt.Fprintln(w, "  bluntcode rules overrides <workspace-id|path> [--set \"path1,path2\"] [--json]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  bluntcode rules list .")
	fmt.Fprintln(w, "  bluntcode rules disable . \"test/**\"")
	fmt.Fprintln(w, "  bluntcode rules enable . \"test/**\"")
	fmt.Fprintln(w, "  bluntcode rules overrides . --set \"dist,build,node_modules\"")
}

func runRules(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printRulesHelp(stdout)
		return 0
	}

	app, release, err := openDBOnly()
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode rules: %v\n", err)
		return 1
	}
	defer release()
	ctx := context.Background()

	switch args[0] {
	case "list", "ls":
		return runRulesList(ctx, app, args[1:], stdout, stderr)
	case "disable":
		return runRulesToggle(ctx, app, args[1:], false, stdout, stderr)
	case "enable":
		return runRulesToggle(ctx, app, args[1:], true, stdout, stderr)
	case "overrides":
		return runRulesOverrides(ctx, app, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "bluntcode rules: unknown subcommand %q\n", args[0])
		printRulesHelp(stderr)
		return 2
	}
}

func runRulesList(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("rules list", flag.ContinueOnError)
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
		fmt.Fprintf(stderr, "bluntcode rules list: %v\n", err)
		return 1
	}

	rules, err := app.db.Rules(ctx, work.ID)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode rules list: %v\n", err)
		return 1
	}

	overrides, _ := app.db.PathOverrides(ctx, work.ID)

	if *jsonOut {
		_ = writeJSONOutput(stdout, map[string]any{
			"workspace":      work.Name,
			"rules":          rules,
			"path_overrides": overrides,
		})
		return 0
	}

	fmt.Fprintf(stdout, "Rules configuration for workspace %s:\n\n", work.Name)
	if len(rules) == 0 {
		fmt.Fprintln(stdout, "  No custom rules configured.")
	} else {
		headers := []string{"TYPE", "PATTERN", "SOURCE", "ENABLED"}
		rows := make([][]string, len(rules))
		for i, r := range rules {
			en := "yes"
			if !r.Enabled {
				en = "no (disabled)"
			}
			rows[i] = []string{r.RuleType, r.Pattern, r.Source, en}
		}
		writeTable(stdout, headers, rows)
	}

	fmt.Fprintf(stdout, "\nPath Overrides:\n")
	if len(overrides) == 0 {
		fmt.Fprintln(stdout, "  No path overrides configured.")
	} else {
		headers := []string{"RELATIVE PATH", "MODE"}
		rows := make([][]string, len(overrides))
		for i, o := range overrides {
			rows[i] = []string{o.RelativePath, o.Mode}
		}
		writeTable(stdout, headers, rows)
	}
	return 0
}

func runRulesToggle(ctx context.Context, app *cliCore, args []string, enable bool, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		action := "disable"
		if enable {
			action = "enable"
		}
		fmt.Fprintf(stderr, "usage: bluntcode rules %s <workspace-id|path> <pattern>\n", action)
		return 2
	}
	target := args[0]
	pattern := strings.TrimSpace(args[1])

	work, err := resolveWorkspace(ctx, app.db, target)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode rules: %v\n", err)
		return 1
	}

	existing, err := app.db.Rules(ctx, work.ID)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode rules: %v\n", err)
		return 1
	}

	found := false
	for i := range existing {
		if strings.EqualFold(existing[i].Pattern, pattern) {
			existing[i].Enabled = enable
			found = true
			break
		}
	}
	if !found {
		existing = append(existing, core.WorkspaceRule{
			WorkspaceID: work.ID,
			RuleType:    "exclude",
			Pattern:     pattern,
			Source:      "user",
			Enabled:     enable,
			CreatedAt:   time.Now().UTC(),
		})
	}

	if err := app.db.ReplaceUserRules(ctx, work.ID, existing); err != nil {
		fmt.Fprintf(stderr, "bluntcode rules: save rules: %v\n", err)
		return 1
	}

	state := "disabled"
	if enable {
		state = "enabled"
	}
	fmt.Fprintf(stdout, "Rule pattern %q is now %s in workspace %s.\n", pattern, state, work.Name)
	return 0
}

func runRulesOverrides(ctx context.Context, app *cliCore, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("rules overrides", flag.ContinueOnError)
	flags.SetOutput(stderr)
	setFlag := flags.String("set", "", "comma-separated paths to exclude from scans")
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
		fmt.Fprintf(stderr, "bluntcode rules overrides: %v\n", err)
		return 1
	}

	if *setFlag != "" || isFlagPassed(flags, "set") {
		var newOverrides []core.PathOverride
		for _, part := range strings.Split(*setFlag, ",") {
			p := strings.TrimSpace(part)
			if p != "" {
				newOverrides = append(newOverrides, core.PathOverride{
					WorkspaceID:  work.ID,
					RelativePath: p,
					Mode:         "exclude",
				})
			}
		}
		if err := app.db.ReplacePathOverrides(ctx, work.ID, newOverrides); err != nil {
			fmt.Fprintf(stderr, "bluntcode rules overrides: %v\n", err)
			return 1
		}
		if *jsonOut {
			_ = writeJSONOutput(stdout, map[string]any{"workspace_id": work.ID, "overrides": newOverrides})
			return 0
		}
		fmt.Fprintf(stdout, "Updated exclude overrides for %s (%d paths).\n", work.Name, len(newOverrides))
		return 0
	}

	overrides, err := app.db.PathOverrides(ctx, work.ID)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode rules overrides: %v\n", err)
		return 1
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, overrides)
		return 0
	}

	if len(overrides) == 0 {
		fmt.Fprintf(stdout, "Workspace %s has no path overrides.\n", work.Name)
	} else {
		fmt.Fprintf(stdout, "Path overrides for %s:\n", work.Name)
		for _, o := range overrides {
			fmt.Fprintf(stdout, "  - %s (%s)\n", o.RelativePath, o.Mode)
		}
	}
	return 0
}

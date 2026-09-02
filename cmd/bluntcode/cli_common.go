package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"bluntcode/internal/config"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/workspace"
)

type cliCore struct {
	paths  config.Paths
	db     *database.DB
	logger *slog.Logger
}

func openDBOnly() (*cliCore, func(), error) {
	paths, err := config.DefaultPaths()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve default paths: %w", err)
	}
	db, err := database.Open(context.Background(), paths.DBPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open database at %s: %w", paths.DBPath, err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	release := func() {
		_ = db.Close()
	}
	return &cliCore{
		paths:  paths,
		db:     db,
		logger: logger,
	}, release, nil
}

func resolveWorkspace(ctx context.Context, db *database.DB, identifier string) (core.Workspace, error) {
	trimmed := strings.TrimSpace(identifier)
	all, err := db.Workspaces(ctx)
	if err != nil {
		return core.Workspace{}, fmt.Errorf("load workspaces: %w", err)
	}
	if len(all) == 0 {
		return core.Workspace{}, fmt.Errorf("no workspaces registered; add one with `bluntcode workspace add <path>`")
	}

	// 1. If empty or dot, attempt current working directory first
	if trimmed == "" || trimmed == "." {
		cwd, getErr := os.Getwd()
		if getErr == nil {
			if norm, normErr := workspace.NormalizeRoot(cwd); normErr == nil {
				key := workspace.CanonicalKey(norm)
				for _, w := range all {
					if workspace.CanonicalKey(w.RootPath) == key {
						return w, nil
					}
				}
			}
		}
		if trimmed == "" && len(all) == 1 {
			return all[0], nil
		}
	}

	// 2. Exact ID match (UUID)
	if len(trimmed) == 36 {
		for _, w := range all {
			if strings.EqualFold(w.ID, trimmed) {
				return w, nil
			}
		}
	}

	// 3. Path match
	if norm, normErr := workspace.NormalizeRoot(trimmed); normErr == nil {
		key := workspace.CanonicalKey(norm)
		for _, w := range all {
			if workspace.CanonicalKey(w.RootPath) == key {
				return w, nil
			}
		}
	}

	// 4. Exact name match (case-insensitive)
	lower := strings.ToLower(trimmed)
	for _, w := range all {
		if strings.ToLower(w.Name) == lower {
			return w, nil
		}
	}

	// 5. Partial name match or prefix match
	for _, w := range all {
		if strings.HasPrefix(strings.ToLower(w.Name), lower) || strings.HasPrefix(strings.ToLower(w.ID), lower) {
			return w, nil
		}
	}

	return core.Workspace{}, fmt.Errorf("workspace %q not found (use `bluntcode workspace list` to see available workspaces)", trimmed)
}

func resolveScan(ctx context.Context, db *database.DB, identifier string) (core.Scan, error) {
	trimmed := strings.TrimSpace(identifier)
	if len(trimmed) == 36 {
		scan, err := db.Scan(ctx, trimmed)
		if err == nil {
			return scan, nil
		}
	}

	// If not a scan ID, try finding the latest scan of the referenced workspace
	work, err := resolveWorkspace(ctx, db, trimmed)
	if err != nil {
		return core.Scan{}, fmt.Errorf("could not resolve scan or workspace for %q: %w", identifier, err)
	}

	scans, err := db.Scans(ctx, work.ID)
	if err != nil {
		return core.Scan{}, fmt.Errorf("load scans for workspace %s: %w", work.Name, err)
	}
	if len(scans) == 0 {
		return core.Scan{}, fmt.Errorf("workspace %s has no scan history; run `bluntcode scan %s` first", work.Name, work.RootPath)
	}

	// Pick the latest scan (first in list)
	return scans[0], nil
}

func writeJSONOutput(w io.Writer, val any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(val)
}

func writeTable(w io.Writer, headers []string, rows [][]string) {
	if len(headers) == 0 {
		return
	}
	colWidths := make([]int, len(headers))
	for i, h := range headers {
		colWidths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	// Print header
	var headerLine strings.Builder
	for i, h := range headers {
		if i > 0 {
			headerLine.WriteString("  ")
		}
		headerLine.WriteString(fmt.Sprintf("%-*s", colWidths[i], h))
	}
	fmt.Fprintln(w, headerLine.String())

	// Print separator
	var sepLine strings.Builder
	for i := range headers {
		if i > 0 {
			sepLine.WriteString("  ")
		}
		sepLine.WriteString(strings.Repeat("-", colWidths[i]))
	}
	fmt.Fprintln(w, sepLine.String())

	// Print rows
	for _, row := range rows {
		var line strings.Builder
		for i := range headers {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			if i > 0 {
				line.WriteString("  ")
			}
			line.WriteString(fmt.Sprintf("%-*s", colWidths[i], cell))
		}
		fmt.Fprintln(w, line.String())
	}
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

// parseFlagsInterspersed parses flags even when positional arguments appear before or between flags.
func parseFlagsInterspersed(flags *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	remaining := args
	for {
		if err := flags.Parse(remaining); err != nil {
			return nil, err
		}
		if flags.NArg() == 0 {
			break
		}
		positional = append(positional, flags.Arg(0))
		remaining = flags.Args()[1:]
	}
	return positional, nil
}

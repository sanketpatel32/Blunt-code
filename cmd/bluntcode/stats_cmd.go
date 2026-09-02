package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math"

	"bluntcode/internal/database"
)

func printStatsHelp(w io.Writer) {
	fmt.Fprintln(w, "Blunt Code statistics, severity trends, and risk metrics")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  bluntcode stats [workspace-id|path] [--json]")
	fmt.Fprintln(w, "  bluntcode trends <workspace-id|path> [--limit N] [--json]")
	fmt.Fprintln(w, "  bluntcode risk <workspace-id|path> [--json]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  bluntcode stats")
	fmt.Fprintln(w, "  bluntcode stats .")
	fmt.Fprintln(w, "  bluntcode trends . --limit 10")
	fmt.Fprintln(w, "  bluntcode risk .")
}

func runStats(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		printStatsHelp(stdout)
		return 0
	}

	flags := flag.NewFlagSet("stats", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOut := flags.Bool("json", false, "output JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	app, release, err := openDBOnly()
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode stats: %v\n", err)
		return 1
	}
	defer release()
	ctx := context.Background()

	// Workspace-specific stats
	if flags.NArg() > 0 {
		work, err := resolveWorkspace(ctx, app.db, flags.Arg(0))
		if err != nil {
			fmt.Fprintf(stderr, "bluntcode stats: %v\n", err)
			return 1
		}
		scansList, _ := app.db.Scans(ctx, work.ID)
		snaps, _ := app.db.RecentCompletedSeverityCounts(ctx, work.ID, 1)

		type wsStats struct {
			WorkspaceName string               `json:"workspace_name"`
			WorkspacePath string               `json:"workspace_path"`
			TotalScans    int                  `json:"total_scans"`
			LatestCounts  *database.RiskCounts `json:"latest_counts,omitempty"`
		}
		res := wsStats{
			WorkspaceName: work.Name,
			WorkspacePath: work.RootPath,
			TotalScans:    len(scansList),
		}
		if len(snaps) > 0 {
			res.LatestCounts = &snaps[0]
		}

		if *jsonOut {
			_ = writeJSONOutput(stdout, res)
			return 0
		}

		fmt.Fprintf(stdout, "Stats for workspace: %s\n", work.Name)
		fmt.Fprintf(stdout, "  Root Path:    %s\n", work.RootPath)
		fmt.Fprintf(stdout, "  Total Scans:  %d\n", len(scansList))
		if len(snaps) > 0 {
			s := snaps[0]
			fmt.Fprintf(stdout, "  Latest Scan:  %s (Critical: %d, High: %d, Medium: %d, Low: %d, Info: %d)\n",
				s.ScanID, s.Critical, s.High, s.Medium, s.Low, s.Info)
		}
		return 0
	}

	// Global statistics
	stats, err := app.db.GlobalStats(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode stats: %v\n", err)
		return 1
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, stats)
		return 0
	}

	fmt.Fprintln(stdout, "Blunt Code Global Statistics:")
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "  Workspaces:          %d\n", stats.Workspaces)
	fmt.Fprintf(stdout, "  Total Scans:         %d\n", stats.Scans.Total)
	fmt.Fprintf(stdout, "  Active Suppressions: %d\n", stats.Suppressions)
	fmt.Fprintf(stdout, "  Latest Findings:     %d total across all workspaces\n", stats.Findings.Total)
	fmt.Fprintf(stdout, "    Critical:          %d\n", stats.Findings.Severity.Critical)
	fmt.Fprintf(stdout, "    High:              %d\n", stats.Findings.Severity.High)
	fmt.Fprintf(stdout, "    Medium:            %d\n", stats.Findings.Severity.Medium)
	fmt.Fprintf(stdout, "    Low:               %d\n", stats.Findings.Severity.Low)
	fmt.Fprintf(stdout, "    Info:              %d\n", stats.Findings.Severity.Info)
	return 0
}

func runTrends(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprintln(stderr, "usage: bluntcode trends <workspace-id|path> [--limit N] [--json]")
		return 0
	}

	flags := flag.NewFlagSet("trends", flag.ContinueOnError)
	flags.SetOutput(stderr)
	limitFlag := flags.Int("limit", 10, "maximum number of scans to trace")
	jsonOut := flags.Bool("json", false, "output JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if flags.NArg() == 0 {
		fmt.Fprintln(stderr, "bluntcode trends: missing workspace path or ID")
		return 2
	}

	app, release, err := openDBOnly()
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode trends: %v\n", err)
		return 1
	}
	defer release()
	ctx := context.Background()

	work, err := resolveWorkspace(ctx, app.db, flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode trends: %v\n", err)
		return 1
	}

	limit := *limitFlag
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}

	points, err := app.db.SeverityTrend(ctx, work.ID, limit)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode trends: %v\n", err)
		return 1
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, map[string]any{
			"workspace": work.Name,
			"points":    points,
		})
		return 0
	}

	if len(points) == 0 {
		fmt.Fprintf(stdout, "No completed scan data for workspace %s.\n", work.Name)
		return 0
	}

	fmt.Fprintf(stdout, "Severity trend for %s (%d scans):\n\n", work.Name, len(points))
	headers := []string{"DATE", "SCAN ID", "CRIT", "HIGH", "MED", "LOW", "INFO", "TOTAL"}
	rows := make([][]string, len(points))
	for i, p := range points {
		dateStr := p.FinishedAt.Local().Format("2006-01-02 15:04")
		rows[i] = []string{
			dateStr,
			p.ScanID,
			fmt.Sprintf("%d", p.Severity.Critical),
			fmt.Sprintf("%d", p.Severity.High),
			fmt.Sprintf("%d", p.Severity.Medium),
			fmt.Sprintf("%d", p.Severity.Low),
			fmt.Sprintf("%d", p.Severity.Info),
			fmt.Sprintf("%d", p.Total),
		}
	}
	writeTable(stdout, headers, rows)
	return 0
}

func runRisk(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Fprintln(stderr, "usage: bluntcode risk <workspace-id|path> [--json]")
		return 0
	}

	flags := flag.NewFlagSet("risk", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOut := flags.Bool("json", false, "output JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if flags.NArg() == 0 {
		fmt.Fprintln(stderr, "bluntcode risk: missing workspace path or ID")
		return 2
	}

	app, release, err := openDBOnly()
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode risk: %v\n", err)
		return 1
	}
	defer release()
	ctx := context.Background()

	work, err := resolveWorkspace(ctx, app.db, flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode risk: %v\n", err)
		return 1
	}

	snaps, err := app.db.RecentCompletedSeverityCounts(ctx, work.ID, 2)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode risk: %v\n", err)
		return 1
	}

	if len(snaps) == 0 {
		fmt.Fprintf(stdout, "No completed scan data available for %s.\n", work.Name)
		return 0
	}

	latest := snaps[0]
	score := computeRiskScore(latest)
	grade := computeRiskGrade(score)

	trend := "flat"
	prevScore := 0
	if len(snaps) > 1 {
		prevScore = computeRiskScore(snaps[1])
		if score > prevScore {
			trend = "up (risk increased)"
		} else if score < prevScore {
			trend = "down (risk decreased)"
		}
	}

	if *jsonOut {
		_ = writeJSONOutput(stdout, map[string]any{
			"workspace":      work.Name,
			"scan_id":        latest.ScanID,
			"risk_score":     score,
			"risk_grade":     grade,
			"trend":          trend,
			"previous_score": prevScore,
			"counts": map[string]int{
				"critical": latest.Critical,
				"high":     latest.High,
				"medium":   latest.Medium,
				"low":      latest.Low,
				"info":     latest.Info,
			},
		})
		return 0
	}

	fmt.Fprintf(stdout, "Risk Assessment for %s:\n\n", work.Name)
	fmt.Fprintf(stdout, "  Scan ID:     %s\n", latest.ScanID)
	fmt.Fprintf(stdout, "  Risk Score:  %d / 100\n", score)
	fmt.Fprintf(stdout, "  Risk Grade:  %s\n", grade)
	fmt.Fprintf(stdout, "  Trend:       %s\n\n", trend)
	fmt.Fprintf(stdout, "Findings Breakdown:\n")
	fmt.Fprintf(stdout, "  Critical:    %d\n", latest.Critical)
	fmt.Fprintf(stdout, "  High:        %d\n", latest.High)
	fmt.Fprintf(stdout, "  Medium:      %d\n", latest.Medium)
	fmt.Fprintf(stdout, "  Low:         %d\n", latest.Low)
	fmt.Fprintf(stdout, "  Info:        %d\n", latest.Info)
	return 0
}

func computeRiskScore(c database.RiskCounts) int {
	weighted := float64(c.Critical)*10.0 + float64(c.High)*5.0 + float64(c.Medium)*2.0 + float64(c.Low)*0.5
	score := int(math.Round(100.0 * (1.0 - math.Exp(-weighted/25.0))))
	if score > 100 {
		return 100
	}
	return score
}

func computeRiskGrade(score int) string {
	switch {
	case score < 20:
		return "A (Low Risk)"
	case score < 45:
		return "B (Moderate Risk)"
	case score < 70:
		return "C (Substantial Risk)"
	default:
		return "D (High Risk)"
	}
}

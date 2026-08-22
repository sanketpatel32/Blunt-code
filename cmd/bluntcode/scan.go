package main

// `bluntcode scan` runs one full headless scan of a workspace without the
// browser UI or the HTTP server. It is aimed at automation and CI: analyzer
// progress streams to stderr, a summary (human or JSON) goes to stdout, and the
// exit code reflects the terminal scan state.

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/events"
	"bluntcode/internal/instance"
	"bluntcode/internal/reports"
	"bluntcode/internal/scans"
)

const (
	scanDefaultTimeout = 30 * time.Minute
	// scanCancelGrace bounds how long the CLI waits for the service to move a
	// cancelled scan to its terminal state before exiting anyway.
	scanCancelGrace = 60 * time.Second
	// scanStatePollInterval is the database fallback for terminal-state
	// detection; the event bus is the primary signal.
	scanStatePollInterval = 2 * time.Second
)

const scanUsage = "usage: bluntcode scan <path> [--profile quick|standard|deep] [--json] [--timeout 30m] [--quiet]"

// scanConfig is the validated command line of `bluntcode scan`.
type scanConfig struct {
	path    string
	profile string
	json    bool
	timeout time.Duration
	quiet   bool
}

// parseScanFlags parses and validates the scan command line. Parse and
// validation failures print the reason followed by the usage block to errOut.
//
// Flags may appear before or after the workspace path so the documented
// `bluntcode scan <path> --json` order works even though flag.FlagSet stops
// at the first positional argument: parsing restarts after each collected
// positional until the command line is exhausted.
func parseScanFlags(args []string, errOut io.Writer) (scanConfig, error) {
	flags := flag.NewFlagSet("scan", flag.ContinueOnError)
	flags.SetOutput(errOut)
	profile := flags.String("profile", "standard", "scan profile: quick, standard, or deep")
	jsonOut := flags.Bool("json", false, "print a machine-readable JSON summary instead of human text")
	timeout := flags.Duration("timeout", scanDefaultTimeout, "abort the scan when it exceeds this duration (for example 5m or 90s)")
	quiet := flags.Bool("quiet", false, "suppress progress lines on stderr; the summary is still printed")
	flags.Usage = func() {
		fmt.Fprintln(errOut, scanUsage)
		fmt.Fprintln(errOut)
		fmt.Fprintln(errOut, "Scans the workspace at <path> headlessly: no browser, no server. Progress is")
		fmt.Fprintln(errOut, "written to stderr and a summary to stdout. Missing managed tools are")
		fmt.Fprintln(errOut, "installed automatically unless offline mode is enabled. Exit code is 0 for a")
		fmt.Fprintln(errOut, "completed scan (warnings included), 1 for failed/cancelled/interrupted,")
		fmt.Fprintln(errOut, "130 when stopped with Ctrl+C.")
		flags.PrintDefaults()
	}
	var positional []string
	remaining := args
	for {
		if err := flags.Parse(remaining); err != nil {
			return scanConfig{}, err
		}
		if flags.NArg() == 0 {
			break
		}
		positional = append(positional, flags.Arg(0))
		remaining = flags.Args()[1:]
	}
	// Every non-usage error below prints the reason followed by the usage
	// block (the same shape the flag package uses) so a mistyped invocation
	// says what was wrong and is immediately correctable.
	usageError := func(message string) (scanConfig, error) {
		fmt.Fprintf(errOut, "bluntcode scan: %s\n", message)
		flags.Usage()
		return scanConfig{}, errors.New(message)
	}
	if len(positional) != 1 {
		return usageError("exactly one workspace path is required")
	}
	cfg := scanConfig{path: positional[0], profile: *profile, json: *jsonOut, timeout: *timeout, quiet: *quiet}
	if cfg.profile != analyzers.ProfileQuick && cfg.profile != analyzers.ProfileStandard && cfg.profile != analyzers.ProfileDeep {
		return usageError("profile must be quick, standard, or deep")
	}
	if cfg.timeout <= 0 {
		return usageError("timeout must be a positive duration such as 5m")
	}
	return cfg, nil
}

// runScan is the process entry point; runScanCommand is the testable core.
func runScan(args []string) {
	os.Exit(runScanCommand(args, os.Stdout, os.Stderr))
}

// scanSummary is everything the CLI prints after a scan reaches a terminal
// state. It is kept as plain data so output rendering can be tested without
// live services.
type scanSummary struct {
	scanID        string
	workspace     string
	workspacePath string
	profile       string
	state         string
	startedAt     time.Time
	finishedAt    time.Time
	critical      int
	high          int
	medium        int
	low           int
	info          int
	total         int
	hasPrevious   bool
	newCount      int
	fixedCount    int
	persistent    int
	runs          []reports.Run
	reportPath    string
	errorSummary  string
	timedOut      bool
}

func (s scanSummary) duration() time.Duration {
	if !s.startedAt.IsZero() {
		if !s.finishedAt.IsZero() {
			return s.finishedAt.Sub(s.startedAt)
		}
		return time.Since(s.startedAt)
	}
	return 0
}

// buildScanSummary loads the terminal scan record plus the same
// comparison data the Markdown report writer uses.
func buildScanSummary(ctx context.Context, db *database.DB, work core.Workspace, scanID string, timedOut bool) (scanSummary, error) {
	scan, err := db.Scan(ctx, scanID)
	if err != nil {
		return scanSummary{}, err
	}
	findings, err := db.Findings(ctx, scanID)
	if err != nil {
		return scanSummary{}, err
	}
	runs, err := db.AnalyzerRuns(ctx, scanID)
	if err != nil {
		return scanSummary{}, err
	}
	reportPath, err := db.ReportPath(ctx, scanID)
	if err != nil {
		return scanSummary{}, err
	}
	summary := scanSummary{
		scanID:        scan.ID,
		workspace:     work.Name,
		workspacePath: work.RootPath,
		profile:       scan.Profile,
		state:         scan.State,
		runs:          runs,
		reportPath:    reportPath,
		errorSummary:  scan.ErrorSummary,
		timedOut:      timedOut,
	}
	if scan.StartedAt != nil {
		summary.startedAt = *scan.StartedAt
	}
	if scan.FinishedAt != nil {
		summary.finishedAt = *scan.FinishedAt
	}
	for _, finding := range findings {
		summary.total++
		switch finding.Severity {
		case analyzers.SeverityCritical:
			summary.critical++
		case analyzers.SeverityHigh:
			summary.high++
		case analyzers.SeverityMedium:
			summary.medium++
		case analyzers.SeverityLow:
			summary.low++
		default:
			summary.info++
		}
	}
	// New/fixed/persistent against the previous completed scan, with exactly
	// the same coverage rules as the Markdown report.
	if previousID, prevErr := db.PreviousCompletedScanID(ctx, work.ID, scanID); prevErr == nil {
		previousFindings, findErr := db.Findings(ctx, previousID)
		coverage, coverageErr := db.SuccessfulAnalyzerIDs(ctx, scanID)
		if findErr == nil && coverageErr == nil {
			diff := scans.Compare(findings, previousFindings, coverage)
			summary.hasPrevious = true
			summary.newCount = len(diff.New)
			summary.fixedCount = len(diff.Fixed)
			summary.persistent = len(diff.Persistent)
		}
	}
	return summary, nil
}

// runScanCommand executes one headless scan and returns the process exit code.
func runScanCommand(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseScanFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	app, release, err := openCore()
	if err != nil {
		if errors.Is(err, instance.ErrAlreadyRunning) {
			fmt.Fprintln(stderr, "bluntcode scan: another Blunt Code process (app or scan) is already using the data directory; close it and retry")
			return 1
		}
		fmt.Fprintf(stderr, "bluntcode scan: %v\n", err)
		return 1
	}
	defer release()
	ctx := context.Background()
	work, err := ensureWorkspace(ctx, app.db, cfg.path)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode scan: %v\n", err)
		return 1
	}
	scan, err := app.scans.DiscoverAndStart(ctx, work, cfg.profile, userExcludePatterns(ctx, app.db, work.ID))
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode scan: could not start scan: %v\n", err)
		return 1
	}
	if !cfg.quiet {
		fmt.Fprintf(stderr, "scanning %s (profile %s, timeout %s)\n", work.RootPath, cfg.profile, cfg.timeout)
	}
	// Subscribe after start: the bus replays its per-scan history, so no
	// already-emitted event (including an instant terminal one) is lost.
	eventCh, unsubscribe := app.bus.Subscribe(scan.ID)
	defer unsubscribe()

	// The interrupt channel buffers two signals so a rapid Ctrl+C double-tap
	// cannot drop the second press while the first is still being handled.
	interrupt := make(chan os.Signal, 2)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)

	outcome := awaitScanTerminal(scanWaitInput{
		events:     eventCh,
		interrupts: interrupt,
		timeout:    time.After(cfg.timeout),
		poll: func() (string, bool) {
			current, err := app.db.Scan(ctx, scan.ID)
			if err != nil {
				return "", false
			}
			return current.State, terminalScanState(current.State)
		},
		cancel:       func() { _ = app.scans.Cancel(ctx, scan.ID) },
		timeoutLimit: cfg.timeout,
		quiet:        cfg.quiet,
		errOut:       stderr,
		pollEvery:    scanStatePollInterval,
		cancelGrace:  scanCancelGrace,
	})
	if outcome.exitNow {
		// Second Ctrl+C: exit immediately. The deferred unsubscribe and
		// release above still run, cancelling analyzers and closing the
		// database cleanly on the way out.
		return 130
	}
	summary, err := buildScanSummary(ctx, app.db, work, scan.ID, outcome.timedOut)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode scan: could not load scan result: %v\n", err)
		return 1
	}
	summary.state = outcome.finalState
	if cfg.json {
		if err := writeScanJSON(stdout, summary); err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: could not write summary: %v\n", err)
			return 1
		}
	} else {
		writeScanHuman(stdout, summary)
	}
	if outcome.interruptSeen {
		return 130
	}
	return scanExitCode(summary.state, summary.timedOut)
}

// scanWaitInput bundles the injectable inputs of the scan event loop so the
// terminal-state decision (double-interrupt fast exit, timeout cancellation,
// and the database polling fallback) can be exercised in tests without OS
// signals, production tick cadences, or a live database.
type scanWaitInput struct {
	events     <-chan events.Event
	interrupts <-chan os.Signal
	timeout    <-chan time.Time
	// poll reports the authoritative scan row state between events.
	poll func() (state string, terminal bool)
	// cancel asks the scan service to cancel the running scan.
	cancel       func()
	timeoutLimit time.Duration // only used for the progress message
	quiet        bool
	errOut       io.Writer
	pollEvery    time.Duration
	cancelGrace  time.Duration
}

// scanWaitOutcome is the event loop's decision.
type scanWaitOutcome struct {
	finalState    string
	interruptSeen bool
	timedOut      bool
	// exitNow means a second Ctrl+C arrived: return 130 immediately, without
	// waiting for the service or printing a summary. Deferred cleanup still
	// runs in the caller.
	exitNow bool
}

// awaitScanTerminal waits until the scan reaches a terminal state, the timeout
// grace expires, or the user double-taps Ctrl+C.
func awaitScanTerminal(in scanWaitInput) scanWaitOutcome {
	ticker := time.NewTicker(in.pollEvery)
	defer ticker.Stop()
	var (
		outcome         scanWaitOutcome
		cancelRequested bool
		cancelDeadline  <-chan time.Time
	)
	for outcome.finalState == "" && !outcome.exitNow {
		select {
		case event := <-in.events:
			if !in.quiet {
				printScanEvent(in.errOut, event)
			}
			switch event.Type {
			case "scan.completed":
				outcome.finalState = "completed"
				if state, ok := scanEventData(event)["state"].(string); ok && state != "" {
					outcome.finalState = state
				}
			case "scan.cancelled":
				outcome.finalState = "cancelled"
			}
		case <-ticker.C:
			// Fallback: subscribers can drop events under load, so the scan
			// row is polled as the authoritative state.
			if state, terminal := in.poll(); terminal {
				outcome.finalState = state
			}
		case <-in.timeout:
			if !cancelRequested {
				cancelRequested = true
				outcome.timedOut = true
				in.cancel()
				cancelDeadline = time.After(in.cancelGrace)
				if !in.quiet {
					fmt.Fprintf(in.errOut, "timeout after %s: cancelling scan\n", in.timeoutLimit)
				}
			}
		case <-cancelDeadline:
			outcome.finalState = "cancelled"
		case <-in.interrupts:
			if !in.quiet {
				fmt.Fprintln(in.errOut, "interrupt received: cancelling scan")
			}
			if cancelRequested {
				// A second Ctrl+C exits immediately; the caller's deferred
				// release still cancels running analyzers and closes the
				// database cleanly.
				outcome.exitNow = true
				break
			}
			cancelRequested = true
			outcome.interruptSeen = true
			in.cancel()
			cancelDeadline = time.After(in.cancelGrace)
		}
	}
	return outcome
}

// scanExitCode maps a terminal scan state to the CLI exit code. completed and
// completed_with_warnings both exit 0 (warnings do not fail a build); every
// other terminal state and any timeout exit 1.
func scanExitCode(state string, timedOut bool) int {
	if timedOut {
		return 1
	}
	switch state {
	case "completed", "completed_with_warnings":
		return 0
	case "failed", "cancelled", "interrupted":
		return 1
	}
	return 1
}

func terminalScanState(state string) bool {
	switch state {
	case "completed", "completed_with_warnings", "failed", "cancelled", "interrupted":
		return true
	}
	return false
}

// userExcludePatterns mirrors the server's user exclude rules for a workspace.
func userExcludePatterns(ctx context.Context, db *database.DB, workspaceID string) []string {
	rules, err := db.Rules(ctx, workspaceID)
	if err != nil {
		return nil
	}
	var patterns []string
	for _, rule := range rules {
		if rule.RuleType == "exclude" && rule.Enabled {
			patterns = append(patterns, rule.Pattern)
		}
	}
	return patterns
}

// scanEventData returns the event payload as a map; events without payload
// data yield an empty map so callers can index unconditionally.
func scanEventData(event events.Event) map[string]any {
	data, _ := event.Data.(map[string]any)
	if data == nil {
		return map[string]any{}
	}
	return data
}

// terminalSafe strips control characters from analyzer-derived text before it
// is written to the terminal. Analyzer error summaries embed untrusted tool
// output from the scanned workspace (for example `ruff exited 1: <stderr>`),
// and raw ESC sequences, BEL, or embedded newlines could otherwise manipulate
// the hosting terminal or forge additional log lines. The JSON output path is
// unaffected: encoding/json always escapes control characters itself.
func terminalSafe(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			r = ' '
		}
		b.WriteRune(r)
	}
	return b.String()
}

// printScanEvent renders one progress line per scan event to stderr.
func printScanEvent(w io.Writer, event events.Event) {
	data := scanEventData(event)
	id, _ := data["analyzer_id"].(string)
	switch event.Type {
	case "scan.started", "scan.cancelled", "analyzer.command_started":
		// lifecycle noise; stage and result lines carry the same information
	case "scan.stage":
		if stage, _ := data["stage"].(string); stage != "" {
			fmt.Fprintf(w, "==> %s\n", terminalSafe(stage))
		}
	case "analyzer.started":
		if name, _ := data["name"].(string); name != "" {
			fmt.Fprintf(w, "[%s] started\n", terminalSafe(name))
		} else {
			fmt.Fprintf(w, "[%s] started\n", terminalSafe(id))
		}
	case "analyzer.completed":
		count, _ := data["findings"].(int)
		fmt.Fprintf(w, "[%s] completed: %d findings\n", terminalSafe(id), count)
	case "analyzer.failed":
		message, _ := data["error"].(string)
		if message == "" {
			message = "unknown error"
		}
		fmt.Fprintf(w, "[%s] FAILED: %s\n", terminalSafe(id), terminalSafe(message))
	case "analyzer.skipped":
		reason, _ := data["reason"].(string)
		fmt.Fprintf(w, "[%s] skipped: %s\n", terminalSafe(id), terminalSafe(reason))
	case "scan.warning":
		if message, _ := data["message"].(string); message != "" {
			fmt.Fprintf(w, "warning: %s\n", terminalSafe(message))
		}
	}
}

func scanAnalyzerDisplayName(id string) string {
	switch id {
	case "ruff":
		return "Ruff"
	case "biome":
		return "Biome"
	case "semgrep":
		return "Semgrep"
	case "sonarqube":
		return "SonarQube"
	}
	return id
}

// writeScanHuman prints the end-of-scan summary to stdout.
func writeScanHuman(w io.Writer, s scanSummary) {
	stateLabel := strings.ReplaceAll(s.state, "_", " ")
	reason := ""
	if s.timedOut {
		reason = " (aborted: timeout exceeded)"
	}
	fmt.Fprintf(w, "Blunt Code scan of %s (%s profile) %s in %s%s\n", s.workspace, s.profile, stateLabel, formatScanDuration(s.duration()), reason)
	switch s.state {
	case "completed", "completed_with_warnings":
		fmt.Fprintf(w, "Findings: %d critical, %d high, %d medium, %d low, %d info - %d total\n", s.critical, s.high, s.medium, s.low, s.info, s.total)
		if s.hasPrevious {
			fmt.Fprintf(w, "Versus previous scan: %d new, %d fixed, %d persistent\n", s.newCount, s.fixedCount, s.persistent)
		}
	}
	for _, run := range s.runs {
		line := fmt.Sprintf("  %s", scanAnalyzerDisplayName(run.AnalyzerID))
		if run.Version != "" {
			line += " " + terminalSafe(run.Version)
		}
		switch run.State {
		case "succeeded":
			line += fmt.Sprintf(": succeeded, %d findings (%s)", run.FindingCount, formatScanDuration(run.Duration))
		default:
			line += fmt.Sprintf(": FAILED (%s)", formatScanDuration(run.Duration))
			if run.ErrorSummary != "" {
				line += " - " + terminalSafe(run.ErrorSummary)
			}
		}
		fmt.Fprintln(w, line)
	}
	if s.state == "completed_with_warnings" {
		fmt.Fprintln(w, "Warnings:")
		warned := false
		for _, run := range s.runs {
			if run.State == "succeeded" {
				continue
			}
			warned = true
			line := fmt.Sprintf("  - %s did not complete", scanAnalyzerDisplayName(run.AnalyzerID))
			if run.ErrorSummary != "" {
				line += ": " + terminalSafe(run.ErrorSummary)
			}
			fmt.Fprintln(w, line)
		}
		if !warned {
			fmt.Fprintln(w, "  - analyzer warnings were reported")
		}
	}
	if s.errorSummary != "" && s.state != "completed" && s.state != "completed_with_warnings" {
		fmt.Fprintf(w, "Error: %s\n", terminalSafe(s.errorSummary))
	}
	if s.reportPath != "" {
		fmt.Fprintf(w, "Report: %s\n", s.reportPath)
	}
}

func formatScanDuration(d time.Duration) string {
	if d >= time.Minute {
		return d.Round(time.Second).String()
	}
	if d >= time.Second {
		return d.Round(100 * time.Millisecond).String()
	}
	return d.Round(time.Millisecond).String()
}

// severityJSON, comparisonJSON, analyzerRunJSON, and scanResultJSON define the
// stable snake_case shape of `bluntcode scan --json`.
type severityJSON struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
	Total    int `json:"total"`
}

type comparisonJSON struct {
	Available  bool `json:"available"`
	New        int  `json:"new"`
	Fixed      int  `json:"fixed"`
	Persistent int  `json:"persistent"`
}

type analyzerRunJSON struct {
	ID         string `json:"id"`
	Version    string `json:"version"`
	State      string `json:"state"`
	Findings   int    `json:"findings"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error"`
}

type scanResultJSON struct {
	ScanID             string            `json:"scan_id"`
	Workspace          string            `json:"workspace"`
	WorkspacePath      string            `json:"workspace_path"`
	Profile            string            `json:"profile"`
	State              string            `json:"state"`
	TimedOut           bool              `json:"timed_out"`
	StartedAt          string            `json:"started_at"`
	FinishedAt         string            `json:"finished_at"`
	DurationMS         int64             `json:"duration_ms"`
	TotalFindings      int               `json:"total_findings"`
	Severity           severityJSON      `json:"severity"`
	Comparison         comparisonJSON    `json:"comparison"`
	Analyzers          []analyzerRunJSON `json:"analyzers"`
	ReportMarkdownPath string            `json:"report_markdown_path"`
	Error              string            `json:"error"`
}

func formatScanTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func writeScanJSON(w io.Writer, s scanSummary) error {
	result := scanResultJSON{
		ScanID:        s.scanID,
		Workspace:     s.workspace,
		WorkspacePath: s.workspacePath,
		Profile:       s.profile,
		State:         s.state,
		TimedOut:      s.timedOut,
		StartedAt:     formatScanTime(s.startedAt),
		FinishedAt:    formatScanTime(s.finishedAt),
		DurationMS:    s.duration().Milliseconds(),
		TotalFindings: s.total,
		Severity: severityJSON{
			Critical: s.critical,
			High:     s.high,
			Medium:   s.medium,
			Low:      s.low,
			Info:     s.info,
			Total:    s.total,
		},
		Comparison: comparisonJSON{
			Available:  s.hasPrevious,
			New:        s.newCount,
			Fixed:      s.fixedCount,
			Persistent: s.persistent,
		},
		Analyzers:          make([]analyzerRunJSON, 0, len(s.runs)),
		ReportMarkdownPath: s.reportPath,
		Error:              s.errorSummary,
	}
	for _, run := range s.runs {
		result.Analyzers = append(result.Analyzers, analyzerRunJSON{
			ID:         run.AnalyzerID,
			Version:    run.Version,
			State:      run.State,
			Findings:   run.FindingCount,
			DurationMS: run.Duration.Milliseconds(),
			Error:      run.ErrorSummary,
		})
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

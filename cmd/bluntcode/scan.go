package main

// `bluntcode scan` runs one full headless scan of a workspace without the
// browser UI or the HTTP server. It is aimed at automation and CI: analyzer
// progress streams to stderr, a summary (human or JSON) goes to stdout, and the
// exit code reflects the terminal scan state.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
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

const scanUsage = "usage: bluntcode scan <path> [--profile quick|standard|deep] [--format text|json|github|sarif] [--json] [--timeout 30m] [--quiet] [--fail-on high+] [--max-findings N] [--baseline <scan-id-or-sarif>] [--jobs N] [--incremental] [--watch]"

// The stdout report formats of `bluntcode scan`. text (the default) keeps the
// historical human summary; json prints the full versioned JSON report
// document that GET /api/v1/scans/{id}/findings.json also serves; github
// prints GitHub Actions workflow-command annotations that the runner turns
// into inline PR annotations; sarif prints the SARIF 2.1.0 log that
// GET /api/v1/scans/{id}/report.sarif also serves, so a CI can capture a
// baseline or feed GitHub code scanning without running the server; csv and
// jsonl print the spreadsheet and newline-delimited findings exports;
// markdown prints the same Markdown document GET /api/v1/scans/{id}/report.md
// renders. The compact summary behind --json stays a separate, unchanged
// output.
const (
	scanFormatText     = "text"
	scanFormatJSON     = "json"
	scanFormatGitHub   = "github"
	scanFormatSARIF    = "sarif"
	scanFormatCSV      = "csv"
	scanFormatJSONL    = "jsonl"
	scanFormatMarkdown = "markdown"
)

// scanConfig is the validated command line of `bluntcode scan`.
type scanConfig struct {
	path    string
	profile string
	json    bool
	// format selects the stdout report: the human summary (text, the
	// historical default), the full JSON report document (json), the GitHub
	// Actions annotation stream (github), or the SARIF 2.1.0 code-scanning
	// document (sarif). The empty value means text, so an omitted flag keeps
	// the pre-flag behavior.
	format  string
	timeout time.Duration
	quiet   bool
	// baseline names the reference the CI gate treats as known findings: a
	// path to a SARIF 2.1.0 file (Blunt Code's own export) or the ID of a
	// previous scan of the same workspace. Empty disables baseline mode.
	baseline string
	// output writes the selected document format (json/github/sarif/csv/
	// jsonl/markdown) to a file instead of stdout; empty means stdout.
	output string
	// saveBaseline writes the scan's SARIF 2.1.0 document to this path after a
	// completed scan, one flag instead of a `--format sarif >` shell redirect.
	saveBaseline string
	// Gate scoping: when non-empty, only findings from these analyzers or in
	// these categories feed the CI gate (--gate-analyzer/--gate-category).
	gateAnalyzers  map[string]bool
	gateCategories map[string]bool
	// githubCap overrides how many annotations are shown per severity level
	// in the --format github stream (default 10, GitHub's own hard limit).
	githubCap int
	// Watch loop tuning; zero values fall back to the package defaults.
	watchPoll  time.Duration
	watchQuiet time.Duration
	// jobs bounds how many analyzers may run concurrently; 0 (the default)
	// keeps the sequential execution model.
	jobs int
	// incremental opts this one-shot scan into incremental mode: analyzers
	// run only on files that changed since the previous completed scan and
	// findings for unchanged files are reused from it. The default stays a
	// full scan for backward compatibility; --watch turns incremental on by
	// itself from its second scan onward.
	incremental bool
	// watch keeps the command running after the first scan: the workspace is
	// polled and the scan reruns whenever files change. Gate flags still
	// report per scan but never exit the process; Ctrl+C (exit 130) stops it.
	watch bool
	// gate is the CI fail-gate assembled from --fail-on and --max-findings;
	// the zero value keeps the historical gate-free exit codes.
	gate scans.GateConfig
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
	format := flags.String("format", "", "stdout report format: text (human summary, the default), json (the full JSON report document), github (GitHub Actions annotations), sarif (the SARIF 2.1.0 code-scanning document), csv, jsonl, or markdown")
	jsonOut := flags.Bool("json", false, "print a machine-readable JSON summary instead of human text")
	timeout := flags.Duration("timeout", scanDefaultTimeout, "abort the scan when it exceeds this duration (for example 5m or 90s)")
	quiet := flags.Bool("quiet", false, "suppress progress lines on stderr; the summary is still printed")
	failOn := flags.String("fail-on", "", "fail (exit 1) when unresolved findings remain at these severities: comma-separated critical, high, medium, low, info (case-insensitive); a trailing + means \"and above\" (for example high+)")
	maxFindings := flags.Int("max-findings", 0, "fail (exit 1) when the scan reports more than N unresolved findings (positive integer)")
	baseline := flags.String("baseline", "", "exclude known findings from the gate: a previous scan ID or the path of a SARIF 2.1.0 file exported by Blunt Code")
	jobs := flags.Int("jobs", 0, "run at most N analyzers concurrently (positive integer); by default analyzers run one after another")
	incremental := flags.Bool("incremental", false, "reuse the previous completed scan's findings for unchanged files and run analyzers only on changed files (a boolean flag: --incremental, never --incremental <value>); composes with gate, baseline, format, and jobs flags")
	output := flags.String("output", "", "write the selected document format (json, github, sarif, csv, jsonl, or markdown) to this file instead of stdout; progress stays on stderr")
	saveBaseline := flags.String("save-baseline", "", "write the scan's SARIF 2.1.0 document to this file after a completed scan (same bytes as --format sarif)")
	gateAnalyzers := flags.String("gate-analyzer", "", "scope the CI gate to these analyzers only: comma-separated IDs such as semgrep,secrets")
	gateCategories := flags.String("gate-category", "", "scope the CI gate to these categories only: comma-separated security, correctness, style, performance")
	watchPollFlag := flags.Duration("watch-poll", 0, "watch mode filesystem poll interval (250ms to 1m; default 2s)")
	watchQuietFlag := flags.Duration("watch-quiet", 0, "watch mode quiet window before a rescan starts (100ms to 2m; default 1.5s)")
	githubCap := flags.Int("github-cap", 10, "max annotations shown per GitHub severity level before a truncation notice (1-50)")
	watch := flags.Bool("watch", false, "keep running and rescan automatically when workspace files change (polls every 2s; a rescan starts after 1.5s without further changes); gate flags never exit the process; Ctrl+C stops it")
	flags.Usage = func() {
		fmt.Fprintln(errOut, scanUsage)
		fmt.Fprintln(errOut)
		fmt.Fprintln(errOut, "Scans the workspace at <path> headlessly: no browser, no server. Progress is")
		fmt.Fprintln(errOut, "written to stderr and a summary to stdout. Missing managed tools are")
		fmt.Fprintln(errOut, "installed automatically unless offline mode is enabled. Exit code is 0 for a")
		fmt.Fprintln(errOut, "completed scan (warnings included), 1 for failed/cancelled/interrupted scans")
		fmt.Fprintln(errOut, "and for a tripped CI gate (--fail-on/--max-findings), 130 when stopped")
		fmt.Fprintln(errOut, "with Ctrl+C, and 2 for usage errors. With --baseline, the gate counts")
		fmt.Fprintln(errOut, "only findings that are new since the baseline (a previous scan ID or a")
		fmt.Fprintln(errOut, "SARIF file exported by Blunt Code). With --watch the command keeps")
		fmt.Fprintln(errOut, "running and rescans whenever workspace files change (poll every 2s,")
		fmt.Fprintln(errOut, "rescan after 1.5s of quiet); gates never exit the process and Ctrl+C")
		fmt.Fprintln(errOut, "(exit 130) stops it.")
		flags.PrintDefaults()
	}
	var positional []string
	maxFindingsSet := false
	baselineSet := false
	jobsSet := false
	remaining := args
	for {
		if err := flags.Parse(remaining); err != nil {
			return scanConfig{}, err
		}
		// flags.Visit only covers the parse call it follows, so presence is
		// recorded after every restart of the parser; this distinguishes an
		// omitted --max-findings from an explicit (invalid) --max-findings 0,
		// an omitted --baseline from an explicit (invalid) --baseline "", and
		// an omitted --jobs from an explicit (invalid) --jobs 0.
		flags.Visit(func(f *flag.Flag) {
			if f.Name == "max-findings" {
				maxFindingsSet = true
			}
			if f.Name == "baseline" {
				baselineSet = true
			}
			if f.Name == "jobs" {
				jobsSet = true
			}
		})
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
	cfg := scanConfig{path: positional[0], profile: *profile, json: *jsonOut, format: *format, timeout: *timeout, quiet: *quiet, baseline: *baseline, jobs: *jobs, incremental: *incremental, watch: *watch, output: *output, saveBaseline: *saveBaseline}
	if cfg.profile != analyzers.ProfileQuick && cfg.profile != analyzers.ProfileStandard && cfg.profile != analyzers.ProfileDeep {
		return usageError("profile must be quick, standard, or deep")
	}
	if cfg.format != "" && cfg.format != scanFormatText && cfg.format != scanFormatJSON && cfg.format != scanFormatGitHub && cfg.format != scanFormatSARIF && cfg.format != scanFormatCSV && cfg.format != scanFormatJSONL && cfg.format != scanFormatMarkdown {
		return usageError("format must be text, json, github, sarif, csv, jsonl, or markdown")
	}
	if cfg.json && cfg.format == scanFormatJSON {
		return usageError("--json cannot be combined with --format json: --json prints the compact summary, --format json the full report")
	}
	if cfg.json && cfg.format == scanFormatGitHub {
		return usageError("--json cannot be combined with --format github: --json prints the compact summary, --format github the annotation stream")
	}
	if cfg.json && cfg.format == scanFormatSARIF {
		return usageError("--json cannot be combined with --format sarif: --json prints the compact summary, --format sarif the SARIF document")
	}
	if cfg.json && cfg.format == scanFormatMarkdown {
		return usageError("--json cannot be combined with --format markdown: --json prints the compact summary, --format markdown the full report")
	}
	if cfg.watch && cfg.format == scanFormatGitHub {
		return usageError("--watch cannot be combined with --format github: annotations make no sense in a watch loop; use text, json, or sarif")
	}
	if cfg.output != "" && cfg.format != scanFormatJSON && cfg.format != scanFormatGitHub && cfg.format != scanFormatSARIF && cfg.format != scanFormatCSV && cfg.format != scanFormatJSONL && cfg.format != scanFormatMarkdown {
		return usageError("--output requires a document format: json, github, sarif, csv, jsonl, or markdown")
	}
	if cfg.saveBaseline != "" && cfg.watch {
		return usageError("--save-baseline cannot be combined with --watch: each rescan would overwrite the file; use --format sarif and redirect instead")
	}
	parseList := func(raw string) map[string]bool {
		out := map[string]bool{}
		for _, part := range strings.Split(raw, ",") {
			if value := strings.ToLower(strings.TrimSpace(part)); value != "" {
				out[value] = true
			}
		}
		return out
	}
	if ga := strings.TrimSpace(*gateAnalyzers); ga != "" {
		cfg.gateAnalyzers = parseList(ga)
	}
	if gc := strings.TrimSpace(*gateCategories); gc != "" {
		cfg.gateCategories = parseList(gc)
	}
	if *watchPollFlag != 0 {
		if *watchPollFlag < 250*time.Millisecond || *watchPollFlag > time.Minute {
			return usageError("watch-poll must be between 250ms and 1m")
		}
		cfg.watchPoll = *watchPollFlag
	}
	if *watchQuietFlag != 0 {
		if *watchQuietFlag < 100*time.Millisecond || *watchQuietFlag > 2*time.Minute {
			return usageError("watch-quiet must be between 100ms and 2m")
		}
		cfg.watchQuiet = *watchQuietFlag
	}
	if cfg.watch && cfg.watchPoll != 0 && cfg.watchQuiet != 0 && cfg.watchQuiet < cfg.watchPoll/4 {
		return usageError("watch-quiet should stay at least a quarter of watch-poll so polls can observe the quiet window")
	}
	if *githubCap < 1 || *githubCap > 50 {
		return usageError("github-cap must be between 1 and 50")
	}
	cfg.githubCap = *githubCap
	if cfg.timeout <= 0 {
		return usageError("timeout must be a positive duration such as 5m")
	}
	if baselineSet && cfg.baseline == "" {
		return usageError("baseline must be a scan ID or the path of a SARIF file")
	}
	if *failOn != "" {
		spec, specErr := scans.ParseSeveritySpec(*failOn)
		if specErr != nil {
			return usageError(specErr.Error())
		}
		cfg.gate.FailOn = spec
	}
	if maxFindingsSet {
		if *maxFindings <= 0 {
			return usageError("max-findings must be a positive integer")
		}
		cfg.gate.MaxFindings = *maxFindings
	}
	if jobsSet && cfg.jobs <= 0 {
		return usageError("jobs must be a positive integer")
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
	// findings keeps the raw rows behind the counts above so the CI gate can
	// evaluate severity and comparison status without a second query.
	findings []analyzers.Finding
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
// comparison data the Markdown report writer uses. Suppressed findings are
// filtered out here so the severity counts, the --json summary, the JSON
// report model, and the CI gate all operate on unsuppressed findings only.
func buildScanSummary(ctx context.Context, db *database.DB, work core.Workspace, scanID string, timedOut bool) (scanSummary, error) {
	scan, err := db.Scan(ctx, scanID)
	if err != nil {
		return scanSummary{}, err
	}
	findings, err := db.Findings(ctx, scanID)
	if err != nil {
		return scanSummary{}, err
	}
	suppressed, err := db.SuppressedFingerprints(ctx, work.ID)
	if err != nil {
		return scanSummary{}, err
	}
	findings = scans.FilterSuppressed(findings, suppressed)
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
		findings:      findings,
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
	// the same coverage rules as the Markdown report. Suppressed fingerprints
	// are filtered from both sides so a dismissed finding is never counted as
	// fixed either.
	if previousID, prevErr := db.PreviousCompletedScanID(ctx, work.ID, scanID); prevErr == nil {
		previousFindings, findErr := db.Findings(ctx, previousID)
		coverage, coverageErr := db.SuccessfulAnalyzerIDs(ctx, scanID)
		if findErr == nil && coverageErr == nil {
			diff := scans.Compare(findings, scans.FilterSuppressed(previousFindings, suppressed), coverage)
			summary.hasPrevious = true
			summary.newCount = len(diff.New)
			summary.fixedCount = len(diff.Fixed)
			summary.persistent = len(diff.Persistent)
		}
	}
	return summary, nil
}

// runScanCommand executes one headless scan — or, with --watch, a scan loop
// that reruns whenever the workspace changes — and returns the process exit
// code.
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
	// The baseline resolves before the scan starts: an unknown scan ID or an
	// unreadable/invalid SARIF file is a usage error (exit 2), not a failed
	// scan, so CI fails fast without running any analyzer. In watch mode the
	// same baseline then applies to every scan of the loop.
	var baseline scans.Baseline
	if cfg.baseline != "" {
		baseline, err = loadScanBaseline(ctx, app.db, cfg.baseline, work.ID)
		if err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: %v\n", err)
			fmt.Fprintln(stderr, scanUsage)
			return 2
		}
	}
	// The interrupt channel buffers two signals so a rapid Ctrl+C double-tap
	// cannot drop the second press while the first is still being handled. In
	// watch mode exactly one party selects on it at a time: the in-flight scan
	// (first press cancels it, second exits immediately) or the idle watch
	// loop (a single press stops it between scans).
	interrupt := make(chan os.Signal, 2)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	if cfg.watch {
		poll, quiet := cfg.watchPoll, cfg.watchQuiet
		if poll == 0 {
			poll = watchPollInterval
		}
		if quiet == 0 {
			quiet = watchQuietWindow
		}
		return runWatchLoop(watchEnv{
			quiet:     cfg.quiet,
			stderr:    stderr,
			interrupt: interrupt,
			snapshot: func() (watchSnapshot, error) {
				return buildWatchSnapshot(ctx, work.RootPath, userExcludePatterns(ctx, app.db, work.ID))
			},
			runScan: func(interrupts <-chan os.Signal, incremental bool) scanRunResult {
				// --watch switches to incremental rescans from its second
				// scan onward (the first has nothing to reuse); an explicit
				// --incremental opts every scan in.
				scanCfg := cfg
				scanCfg.incremental = incremental || cfg.incremental
				return runSingleScan(app, scanCfg, work, baseline, stdout, stderr, interrupts)
			},
		}, watchLoopOptions{pollInterval: poll, quietWindow: quiet})
	}
	return runSingleScan(app, cfg, work, baseline, stdout, stderr, interrupt).code
}

// scanRunResult is the outcome of one complete scan invocation: the exit code
// the single-scan CLI path returns plus the interrupt flags the watch loop
// needs to stop promptly.
type scanRunResult struct {
	code          int
	interruptSeen bool
	// exitNow means a second Ctrl+C arrived: return 130 immediately, without
	// printing a summary.
	exitNow bool
}

// runSingleScan executes one scan end to end — start, terminal-state wait,
// summary output, and CI gate — and reports the exit code `bluntcode scan`
// without --watch would print. It is the shared body of the one-shot command
// and of every iteration of the --watch loop; every failure is printed here
// and reflected in the result, never returned as an error, so the watch loop
// can keep watching after a failed scan.
func runSingleScan(app *appCore, cfg scanConfig, work core.Workspace, baseline scans.Baseline, stdout, stderr io.Writer, interrupts <-chan os.Signal) scanRunResult {
	ctx := context.Background()
	scan, err := app.scans.DiscoverAndStartWithOptions(ctx, work, cfg.profile, userExcludePatterns(ctx, app.db, work.ID), scans.ScanOptions{Jobs: cfg.jobs, Incremental: cfg.incremental})
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode scan: could not start scan: %v\n", err)
		return scanRunResult{code: 1}
	}
	if !cfg.quiet {
		fmt.Fprintf(stderr, "scanning %s (profile %s, timeout %s)\n", work.RootPath, cfg.profile, cfg.timeout)
	}
	// Subscribe after start: the bus replays its per-scan history, so no
	// already-emitted event (including an instant terminal one) is lost.
	eventCh, unsubscribe := app.bus.Subscribe(scan.ID)
	defer unsubscribe()

	outcome := awaitScanTerminal(scanWaitInput{
		events:     eventCh,
		interrupts: interrupts,
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
		// Second Ctrl+C: exit immediately. The deferred unsubscribe above (and
		// the caller's release) still run, cancelling analyzers and closing
		// the database cleanly on the way out.
		return scanRunResult{code: 130, exitNow: true}
	}
	summary, err := buildScanSummary(ctx, app.db, work, scan.ID, outcome.timedOut)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode scan: could not load scan result: %v\n", err)
		return scanRunResult{code: 1}
	}
	summary.state = outcome.finalState
	// Document formats share one report model; --save-baseline needs the same
	// model, so it is built once whenever any of them is in play.
	documentFormats := cfg.format == scanFormatJSON || cfg.format == scanFormatGitHub || cfg.format == scanFormatSARIF || cfg.format == scanFormatCSV || cfg.format == scanFormatJSONL || cfg.format == scanFormatMarkdown
	var model reports.Model
	if documentFormats || cfg.saveBaseline != "" {
		built, err := buildScanReportModel(ctx, app.db, work, summary)
		if err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: could not load report: %v\n", err)
			return scanRunResult{code: 1}
		}
		model = built
	}
	switch {
	case documentFormats:
		// All four document formats share the report model and go to stdout
		// (or to --output); the annotation stream especially must stay on
		// stdout because that is the only stream GitHub Actions scans for
		// workflow commands. The SARIF bytes are the exact serialization GET
		// /api/v1/scans/{id}/report.sarif serves (reports.SARIFBytes), so
		// `--format sarif > baseline.sarif` round-trips through --baseline
		// without the server. Every document ends with its own single LF, so
		// in --watch mode each rescan emits one complete, newline-separated
		// document, mirroring the json format. The gate and baseline summaries
		// below keep writing to stderr, so the formats compose with the CI
		// flags without interleaving.
		var document []byte
		switch cfg.format {
		case scanFormatGitHub:
			document = reports.GitHubAnnotationsWithCap(model, cfg.githubCap)
		case scanFormatSARIF:
			document = reports.SARIFBytes(model)
		case scanFormatCSV:
			document = reports.CSV(model)
		case scanFormatJSONL:
			document = reports.JSONL(model)
		case scanFormatMarkdown:
			document = []byte(reports.Markdown(model))
		default:
			document = reports.JSON(model)
		}
		if cfg.output != "" {
			if err := os.WriteFile(cfg.output, document, 0o600); err != nil {
				fmt.Fprintf(stderr, "bluntcode scan: could not write %s: %v\n", cfg.output, err)
				return scanRunResult{code: 1}
			}
			fmt.Fprintf(stderr, "report written to %s\n", cfg.output)
		} else if _, err := stdout.Write(document); err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: could not write report: %v\n", err)
			return scanRunResult{code: 1}
		}
	}
	if cfg.saveBaseline != "" && summary.state == "completed" {
		if err := os.WriteFile(cfg.saveBaseline, reports.SARIFBytes(model), 0o600); err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: could not write baseline %s: %v\n", cfg.saveBaseline, err)
			return scanRunResult{code: 1}
		}
		fmt.Fprintf(stderr, "baseline written to %s\n", cfg.saveBaseline)
	}
	switch {
	case !documentFormats && cfg.json:
		if err := writeScanJSON(stdout, summary); err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: could not write summary: %v\n", err)
			return scanRunResult{code: 1}
		}
	default:
		writeScanHuman(stdout, summary)
	}
	if outcome.interruptSeen {
		return scanRunResult{code: 130, interruptSeen: true}
	}
	code := scanExitCode(summary.state, summary.timedOut)
	// Baseline mode always reports how much debt was inherited versus what the
	// scan added, even without gate flags (which leave the exit code alone).
	var gatedFindings []analyzers.Finding
	if cfg.baseline != "" {
		known, fresh := baseline.Split(summary.findings)
		gatedFindings = fresh
		fmt.Fprintf(stderr, "baseline: %d known finding(s) excluded from gate, %d new finding(s)\n", len(known), len(fresh))
	} else {
		gatedFindings = summary.findings
	}
	// Gate scoping narrows which findings the gate sees: only the listed
	// analyzers and/or categories are counted. An empty flag means "all".
	if len(cfg.gateAnalyzers) > 0 || len(cfg.gateCategories) > 0 {
		narrowed := scopeGateFindings(gatedFindings, cfg.gateAnalyzers, cfg.gateCategories)
		fmt.Fprintf(stderr, "gate scope: %d of %d finding(s) match --gate-analyzer/--gate-category\n", len(narrowed), len(gatedFindings))
		gatedFindings = narrowed
	}
	// The CI gate applies only to scans that completed and would otherwise
	// exit 0; failed, cancelled, and interrupted scans keep their own exit
	// path regardless of gate flags. With a baseline, only findings whose
	// fingerprints it does not know are counted. In watch mode the caller
	// ignores this code and keeps watching.
	if code == 0 && cfg.gate.Enabled() {
		if gate := scans.EvaluateGate(gatedFindings, cfg.gate); gate.Failed {
			fmt.Fprintln(stderr, gate.FailureMessage())
			return scanRunResult{code: 1}
		}
	}
	return scanRunResult{code: code}
}

// buildScanReportModel assembles the full report model behind
// `--format json` and `--format github`. It mirrors the API server's reportModel loader (same
// metrics, comparison coverage rule, suppression filter, and file-count
// placeholders) so the CLI document and the GET /api/v1/scans/{id}/findings.json
// download carry the same data through the same renderer.
func buildScanReportModel(ctx context.Context, db *database.DB, work core.Workspace, summary scanSummary) (reports.Model, error) {
	scan, err := db.Scan(ctx, summary.scanID)
	if err != nil {
		return reports.Model{}, err
	}
	metrics, err := db.Metrics(ctx, summary.scanID)
	if err != nil {
		return reports.Model{}, err
	}
	// summary.findings is already suppression-filtered by buildScanSummary;
	// the previous scan's findings need the same treatment here.
	suppressed, err := db.SuppressedFingerprints(ctx, work.ID)
	if err != nil {
		return reports.Model{}, err
	}
	comparison := reports.Comparison{}
	if previousID, prevErr := db.PreviousCompletedScanID(ctx, work.ID, summary.scanID); prevErr == nil {
		previousFindings, findErr := db.Findings(ctx, previousID)
		coverage, coverageErr := db.SuccessfulAnalyzerIDs(ctx, summary.scanID)
		if findErr == nil && coverageErr == nil {
			diff := scans.Compare(summary.findings, scans.FilterSuppressed(previousFindings, suppressed), coverage)
			comparison = reports.Comparison{New: diff.New, Fixed: diff.Fixed, Persistent: diff.Persistent, UnknownAnalyzerIDs: diff.UnknownAnalyzerIDs}
		}
	}
	files := []string(nil)
	bluntCodeVersion := ""
	if scan.Snapshot != nil {
		files = append(files, scan.Snapshot.SelectedFiles...)
		bluntCodeVersion = scan.Snapshot.BluntCodeVersion
	}
	if len(files) == 0 && scan.SelectedFileCount > 0 {
		files = make([]string, scan.SelectedFileCount)
	}
	skipped := scan.CandidateFileCount - scan.SelectedFileCount
	if skipped < 0 {
		skipped = 0
	}
	return reports.Build(reports.Input{
		WorkspaceName: work.Name, WorkspacePath: work.RootPath, ScanID: summary.scanID, Profile: summary.profile,
		State: summary.state, BluntCodeVersion: bluntCodeVersion,
		StartedAt: summary.startedAt, FinishedAt: summary.finishedAt,
		Files: files, SkippedFiles: make([]string, skipped),
		Findings: summary.findings, Metrics: metrics, Runs: summary.runs, Comparison: comparison,
	}), nil
}

// loadScanBaseline resolves a --baseline reference: an existing file on disk
// is a SARIF 2.1.0 baseline (Blunt Code's own export), anything else must be
// the ID of a previous scan of the same workspace. Every failure is a
// usage-style error the caller reports with exit code 2.
func loadScanBaseline(ctx context.Context, db *database.DB, ref, workspaceID string) (scans.Baseline, error) {
	if info, err := os.Stat(ref); err == nil {
		if info.IsDir() {
			return scans.Baseline{}, fmt.Errorf("baseline %q is a directory, not a SARIF file", ref)
		}
		file, err := os.Open(ref)
		if err != nil {
			return scans.Baseline{}, fmt.Errorf("cannot read baseline file %q: %v", ref, err)
		}
		defer file.Close()
		baseline, err := scans.BaselineFromSARIF(file)
		if err != nil {
			return scans.Baseline{}, fmt.Errorf("baseline file %q: %v", ref, err)
		}
		return baseline, nil
	}
	scan, err := db.Scan(ctx, ref)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return scans.Baseline{}, fmt.Errorf("baseline %q not found: it is neither an existing file nor a known scan ID", ref)
		}
		return scans.Baseline{}, fmt.Errorf("could not load baseline scan %q: %v", ref, err)
	}
	if scan.WorkspaceID != workspaceID {
		return scans.Baseline{}, fmt.Errorf("baseline scan %q belongs to a different workspace", ref)
	}
	findings, err := db.Findings(ctx, ref)
	if err != nil {
		return scans.Baseline{}, fmt.Errorf("could not load findings of baseline scan %q: %v", ref, err)
	}
	return scans.BaselineFromFindings(findings), nil
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
// scopeGateFindings keeps only findings whose analyzer and category pass the
// gate-scoping allow-lists; a nil/empty set allows everything on that axis.
// Both the findings and the allow-list entries are compared lowercased, so
// --gate-analyzer SEMGREP matches analyzer_id "semgrep".
func scopeGateFindings(findings []analyzers.Finding, analyzersAllow, categoriesAllow map[string]bool) []analyzers.Finding {
	if len(analyzersAllow) == 0 && len(categoriesAllow) == 0 {
		return findings
	}
	lower := func(set map[string]bool) map[string]bool {
		out := make(map[string]bool, len(set))
		for k := range set {
			out[strings.ToLower(k)] = true
		}
		return out
	}
	analyzersAllow = lower(analyzersAllow)
	categoriesAllow = lower(categoriesAllow)
	out := make([]analyzers.Finding, 0, len(findings))
	for _, f := range findings {
		if len(analyzersAllow) > 0 && !analyzersAllow[strings.ToLower(f.AnalyzerID)] {
			continue
		}
		if len(categoriesAllow) > 0 && !categoriesAllow[strings.ToLower(string(f.Category))] {
			continue
		}
		out = append(out, f)
	}
	return out
}

// writeTopFindings prints up to n of the most serious findings under the
// counts so the human summary answers "what should I fix first" without
// opening the full report. Ordering is severity rank, then path.
func writeTopFindings(w io.Writer, findings []analyzers.Finding, n int) {
	if len(findings) == 0 || n <= 0 {
		return
	}
	rank := map[analyzers.Severity]int{analyzers.SeverityCritical: 5, analyzers.SeverityHigh: 4, analyzers.SeverityMedium: 3, analyzers.SeverityLow: 2, analyzers.SeverityInfo: 1}
	top := make([]analyzers.Finding, len(findings))
	copy(top, findings)
	sort.SliceStable(top, func(i, j int) bool {
		if rank[top[i].Severity] != rank[top[j].Severity] {
			return rank[top[i].Severity] > rank[top[j].Severity]
		}
		return top[i].RelativePath < top[j].RelativePath
	})
	fmt.Fprintf(w, "Top findings:\n")
	for i, f := range top {
		if i >= n {
			break
		}
		rule := f.RuleID
		if rule == "" {
			rule = string(f.Category)
		}
		fmt.Fprintf(w, "  %s %s - %s:%d\n", strings.ToUpper(string(f.Severity)), rule, f.RelativePath, f.StartLine)
	}
}

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
		writeTopFindings(w, s.findings, 3)
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

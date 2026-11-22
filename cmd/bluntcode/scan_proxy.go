package main

// Remote `bluntcode scan`: when the desktop app holds the data-directory
// lock, a headless scan used to refuse outright. The running server is the
// same engine behind loopback auth, so the CLI now drives its scan API for
// the flag subset the API can express — "scan this folder" keeps working
// while the app is open. Flags the API cannot honor (--watch/--watch-poll/
// --watch-quiet, --baseline, --format github) still refuse, naming exactly
// what cannot be proxied; --jobs and --incremental ride the scan body.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/api"
	"bluntcode/internal/events"
	"bluntcode/internal/reports"
	"bluntcode/internal/scans"
	"bluntcode/internal/workspace"
)

// remoteUnsupported lists the scan flags a running server cannot honor over
// the API. Naming them in the refusal beats a generic "close the app" hint.
func remoteUnsupported(cfg scanConfig) []string {
	var flags []string
	if cfg.watch {
		flags = append(flags, "--watch")
	}
	if cfg.watchPoll != 0 {
		flags = append(flags, "--watch-poll")
	}
	if cfg.watchQuiet != 0 {
		flags = append(flags, "--watch-quiet")
	}
	if cfg.baseline != "" {
		flags = append(flags, "--baseline")
	}
	if cfg.format == scanFormatGitHub {
		flags = append(flags, "--format github")
	}
	return flags
}

// remoteClient is the authenticated loopback client for the running server's
// API. The bearer token comes from the same server.token file the second-
// launch handoff reads; other local users cannot read it (0600).
type remoteClient struct {
	base  string
	token string
	http  *http.Client
}

func (c *remoteClient) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(request)
}

// request performs one call and returns the status and body; API error
// envelopes stay inspectable for the caller (e.g. SCAN_ALREADY_ACTIVE's
// details.scan_id).
func (c *remoteClient) request(ctx context.Context, method, path, payload string) (int, []byte, error) {
	var body io.Reader
	if payload != "" {
		body = strings.NewReader(payload)
	}
	response, err := c.do(ctx, method, path, body)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 256<<20))
	return response.StatusCode, data, err
}

func (c *remoteClient) getJSON(ctx context.Context, path string, out any) error {
	status, data, err := c.request(ctx, http.MethodGet, path, "")
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("%s: %s", path, apiErrorMessage(data, status))
	}
	return json.Unmarshal(data, out)
}

func (c *remoteClient) getBytes(ctx context.Context, path string) ([]byte, error) {
	status, data, err := c.request(ctx, http.MethodGet, path, "")
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", path, apiErrorMessage(data, status))
	}
	return data, nil
}

// apiErrorMessage renders the server's {"error":{"code","message"}} envelope,
// falling back to the status line for unexpected bodies.
func apiErrorMessage(body []byte, status int) string {
	var payload struct {
		Err struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.Err.Code != "" {
		return payload.Err.Message + " (" + payload.Err.Code + ")"
	}
	return http.StatusText(status)
}

// apiErrorCode returns just the envelope's code field, or "".
func apiErrorCode(body []byte) string {
	var payload struct {
		Err struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil {
		return payload.Err.Code
	}
	return ""
}

// apiConflictScanID extracts details.scan_id from a 409 SCAN_ALREADY_ACTIVE
// reply so the CLI can attach to the running scan instead of failing.
func apiConflictScanID(body []byte) string {
	var payload struct {
		Err struct {
			Details struct {
				ScanID string `json:"scan_id"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil {
		return payload.Err.Details.ScanID
	}
	return ""
}

// remoteScan mirrors the scan record fields the API returns.
type remoteScan struct {
	ID           string `json:"id"`
	WorkspaceID  string `json:"workspace_id"`
	State        string `json:"state"`
	Profile      string `json:"profile"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at"`
	New          int    `json:"new_count"`
	ErrorSummary string `json:"error_summary"`
	AnalyzerRuns []struct {
		AnalyzerID string `json:"analyzer_id"`
		DurationMS int64  `json:"duration_ms"`
		Message    string `json:"message"`
		Status     string `json:"status"`
		Version    string `json:"version"`
	} `json:"analyzer_runs"`
}

type remoteWorkspace struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	RootPath string `json:"root_path"`
}

type remoteFindingsPage struct {
	Items   []analyzers.Finding `json:"items"`
	HasNext bool                `json:"has_next"`
}

type remoteFixed struct {
	ComparisonAvailable bool `json:"comparison_available"`
	TotalFixed          int  `json:"total_fixed"`
}

// remoteClientFor verifies the recorded port is a live Blunt Code server
// (the same api_version probe the second-launch handoff uses, so a foreign
// listener or stale hint file never captures the proxy) and returns an
// authenticated client for it.
func remoteClientFor(ctx context.Context) (*remoteClient, bool) {
	portPath, err := serverPortPath()
	if err != nil {
		return nil, false
	}
	raw, err := os.ReadFile(portPath)
	if err != nil {
		return nil, false
	}
	var port int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(raw)), "%d", &port); err != nil || port <= 0 {
		return nil, false
	}
	client := &remoteClient{
		base:  fmt.Sprintf("http://127.0.0.1:%d/", port),
		token: readServerToken(),
		http:  &http.Client{Timeout: 30 * time.Second},
	}
	var meta struct {
		APIVersion string `json:"api_version"`
		Version    string `json:"version"`
	}
	if err := client.getJSON(ctx, "api/v1/meta", &meta); err != nil {
		return nil, false
	}
	if meta.APIVersion != api.APIVersion || meta.Version == "" {
		return nil, false
	}
	return client, true
}

// streamRemoteEvents tails the scan's SSE feed and renders each event through
// the same printer local scans use, so remote progress lines look identical.
func streamRemoteEvents(ctx context.Context, c *remoteClient, scanID string, w io.Writer) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"api/v1/scans/"+scanID+"/events", nil)
	if err != nil {
		return
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err := c.http.Do(request)
	if err != nil {
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event events.Event
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event) != nil {
			continue
		}
		printScanEvent(w, event)
	}
}

// runRemoteScan drives a scan through the already-running app over its
// loopback API. It reports (code, true) when the invocation was handled
// remotely — including handled failures — and (_, false) when no reachable
// server exists, so the caller can print the classic lock message.
func runRemoteScan(cfg scanConfig, stdout, stderr io.Writer) (int, bool) {
	if unsupported := remoteUnsupported(cfg); len(unsupported) > 0 {
		fmt.Fprintf(stderr, "bluntcode scan: the app is already running and holds the data directory; remote scans cannot honor %s — close the app or drop those flags\n", strings.Join(unsupported, ", "))
		return 3, true
	}
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()
	client, ok := remoteClientFor(rootCtx)
	if !ok {
		return 0, false
	}
	root, err := workspace.NormalizeRoot(cfg.path)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode scan: %v\n", err)
		return 2, true
	}
	var work remoteWorkspace
	workspaceBody, _ := json.Marshal(map[string]string{"root_path": root})
	status, data, err := client.request(rootCtx, http.MethodPost, "api/v1/workspaces", string(workspaceBody))
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode scan: could not reach the running app: %v\n", err)
		return 3, true
	}
	if status != http.StatusOK && status != http.StatusCreated {
		fmt.Fprintf(stderr, "bluntcode scan: the running app refused the workspace: %s\n", apiErrorMessage(data, status))
		return 3, true
	}
	if err := json.Unmarshal(data, &work); err != nil || work.ID == "" {
		fmt.Fprintf(stderr, "bluntcode scan: the running app returned an unreadable workspace reply\n")
		return 3, true
	}
	if !cfg.quiet {
		fmt.Fprintf(stderr, "bluntcode scan: scanning via the running app at %s\n", client.base)
	}
	scanBody := `{"profile":` + jsonString(cfg.profile)
	if cfg.target != "" {
		scanBody += `,"target_url":` + jsonString(cfg.target)
	}
	if cfg.jobs != 0 {
		scanBody += fmt.Sprintf(`,"jobs":%d`, cfg.jobs)
	}
	if cfg.incremental {
		scanBody += `,"incremental":true`
	}
	scanBody += `}`
	var scanID string
	status, data, err = client.request(rootCtx, http.MethodPost, "api/v1/workspaces/"+work.ID+"/scans", scanBody)
	switch {
	case err != nil:
		fmt.Fprintf(stderr, "bluntcode scan: could not start scan via the running app: %v\n", err)
		return 3, true
	case status == http.StatusOK || status == http.StatusCreated || status == http.StatusAccepted:
		var started remoteScan
		if json.Unmarshal(data, &started) != nil || started.ID == "" {
			fmt.Fprintf(stderr, "bluntcode scan: the running app returned an unreadable scan reply\n")
			return 3, true
		}
		scanID = started.ID
	case apiErrorCode(data) == "SCAN_ALREADY_ACTIVE":
		// A scan already running on this workspace answers 409 with its ID —
		// attach to it, the remote cousin of the second-launch handoff.
		scanID = apiConflictScanID(data)
		if scanID == "" {
			fmt.Fprintf(stderr, "bluntcode scan: %s\n", apiErrorMessage(data, status))
			return 3, true
		}
		if !cfg.quiet {
			fmt.Fprintf(stderr, "bluntcode scan: a scan is already running on %s in the app; following it\n", work.Name)
		}
	default:
		fmt.Fprintf(stderr, "bluntcode scan: could not start scan via the running app: %s\n", apiErrorMessage(data, status))
		return 3, true
	}
	return finishRemoteScan(rootCtx, client, cfg, work, scanID, stdout, stderr)
}

// finishRemoteScan waits for the remote scan to reach a terminal state —
// polling the scan record while an SSE tail renders progress — honoring
// --timeout (POSTs cancel) and Ctrl+C (first cancels, second exits 130).
func finishRemoteScan(ctx context.Context, client *remoteClient, cfg scanConfig, work remoteWorkspace, scanID string, stdout, stderr io.Writer) (int, bool) {
	eventsCtx, stopEvents := context.WithCancel(ctx)
	defer stopEvents()
	if !cfg.quiet {
		go streamRemoteEvents(eventsCtx, client, scanID, stderr)
	}
	interrupt := make(chan os.Signal, 2)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	deadline := time.After(cfg.timeout)
	poll := time.NewTicker(scanStatePollInterval)
	defer poll.Stop()
	// grace bounds the wait for the server to land the cancel the timeout
	// asked for — matching the local runner's scanCancelGrace.
	grace := time.NewTimer(scanCancelGrace)
	grace.Stop()
	defer grace.Stop()
	timedOut, interrupted := false, false
	var scan remoteScan
	for {
		select {
		case <-deadline:
			if timedOut {
				break
			}
			timedOut = true
			cancelRemoteScan(client, scanID)
			grace.Reset(scanCancelGrace)
		case <-grace.C:
			// The cancel never landed; report what we last saw, still marked
			// timed out so the exit code stays 3.
			if scan.State == "" {
				scan.State = "timeout"
				scan.ErrorSummary = "scan never reached a terminal state after cancellation"
			}
			return reportRemoteScan(ctx, client, cfg, work, scan, timedOut, interrupted, stdout, stderr)
		case <-interrupt:
			if interrupted {
				return 130, true
			}
			interrupted = true
			cancelRemoteScan(client, scanID)
			grace.Reset(scanCancelGrace)
		case <-poll.C:
			if err := client.getJSON(ctx, "api/v1/scans/"+scanID, &scan); err != nil {
				continue
			}
			if !terminalScanState(scan.State) {
				continue
			}
			return reportRemoteScan(ctx, client, cfg, work, scan, timedOut, interrupted, stdout, stderr)
		}
	}
}

func cancelRemoteScan(client *remoteClient, scanID string) {
	cancelCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, _ = client.request(cancelCtx, http.MethodPost, "api/v1/scans/"+scanID+"/cancel", "")
}

// fetchRemoteFindings pages through every finding of the finished scan.
// Suppressed rows are dropped so counts, gate input, and the printed summary
// match the local path's unsuppressed-only rule.
func fetchRemoteFindings(ctx context.Context, client *remoteClient, scanID string) []analyzers.Finding {
	var findings []analyzers.Finding
	for page := 1; ; page++ {
		var result remoteFindingsPage
		if err := client.getJSON(ctx, fmt.Sprintf("api/v1/scans/%s/findings?page=%d&page_size=200", scanID, page), &result); err != nil {
			break
		}
		for _, finding := range result.Items {
			if finding.Status == scans.StatusSuppressed {
				continue
			}
			findings = append(findings, finding)
		}
		if !result.HasNext {
			break
		}
	}
	return findings
}

// reportRemoteScan assembles the same scanSummary the local path prints —
// analyzer runs, severity counts, and previous-scan comparison — then writes
// the requested output and resolves the exit code through the shared gate.
func reportRemoteScan(ctx context.Context, client *remoteClient, cfg scanConfig, work remoteWorkspace, scan remoteScan, timedOut, interrupted bool, stdout, stderr io.Writer) (int, bool) {
	findings := fetchRemoteFindings(ctx, client, scan.ID)
	perAnalyzer := map[string]int{}
	summary := scanSummary{
		scanID:        scan.ID,
		workspace:     work.Name,
		workspacePath: work.RootPath,
		profile:       scan.Profile,
		state:         scan.State,
		errorSummary:  scan.ErrorSummary,
		timedOut:      timedOut,
		findings:      findings,
		reportPath:    client.base + "api/v1/scans/" + scan.ID + "/report.md",
	}
	if t, err := time.Parse(time.RFC3339Nano, scan.StartedAt); err == nil {
		summary.startedAt = t
	}
	if t, err := time.Parse(time.RFC3339Nano, scan.FinishedAt); err == nil {
		summary.finishedAt = t
	}
	for _, finding := range findings {
		summary.total++
		perAnalyzer[finding.AnalyzerID]++
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
	for _, run := range scan.AnalyzerRuns {
		summary.runs = append(summary.runs, reports.Run{
			AnalyzerID:   run.AnalyzerID,
			Version:      run.Version,
			State:        run.Status,
			ErrorSummary: run.Message,
			FindingCount: perAnalyzer[run.AnalyzerID],
			Duration:     time.Duration(run.DurationMS) * time.Millisecond,
		})
	}
	var fixed remoteFixed
	if err := client.getJSON(ctx, "api/v1/scans/"+scan.ID+"/fixed", &fixed); err == nil {
		summary.hasPrevious = fixed.ComparisonAvailable
		summary.fixedCount = fixed.TotalFixed
	}
	summary.newCount = scan.New
	summary.persistent = summary.total - summary.newCount
	if summary.persistent < 0 {
		summary.persistent = 0
	}
	// Document formats fetch the same export the server writes, then land on
	// --output or stdout exactly like the local renderers' bytes would.
	documentFormats := remoteDocumentPath(cfg.format) != ""
	if documentFormats {
		doc, err := client.getBytes(ctx, fmt.Sprintf("api/v1/scans/%s/%s", scan.ID, remoteDocumentPath(cfg.format)))
		if err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: could not fetch %s report: %v\n", cfg.format, err)
			return 3, true
		}
		if cfg.output != "" {
			if err := reports.WriteBytesAtomic(cfg.output, doc, 0o600); err != nil {
				fmt.Fprintf(stderr, "bluntcode scan: could not write %s: %v\n", cfg.output, err)
				return 3, true
			}
			fmt.Fprintf(stderr, "report written to %s\n", cfg.output)
		} else if _, err := stdout.Write(doc); err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: could not write report: %v\n", err)
			return 3, true
		}
	}
	if cfg.saveBaseline != "" && summary.state == "completed" {
		doc, err := client.getBytes(ctx, fmt.Sprintf("api/v1/scans/%s/report.sarif", scan.ID))
		if err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: could not write baseline %s: %v\n", cfg.saveBaseline, err)
			return 3, true
		}
		if err := reports.WriteBytesAtomic(cfg.saveBaseline, doc, 0o600); err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: could not write baseline %s: %v\n", cfg.saveBaseline, err)
			return 3, true
		}
		fmt.Fprintf(stderr, "baseline written to %s\n", cfg.saveBaseline)
	}
	// The human summary stays off stdout only when a document occupied it —
	// otherwise the stream is free for it (e.g. --format sarif --output).
	switch {
	case !documentFormats && cfg.json:
		if err := writeScanJSON(stdout, summary); err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: could not write summary: %v\n", err)
			return 3, true
		}
	case !documentFormats || cfg.output != "":
		writeScanHuman(stdout, summary)
	}
	if interrupted {
		return 130, true
	}
	code := scanExitCode(summary.state, summary.timedOut)
	gated := findings
	if len(cfg.gateAnalyzers) > 0 || len(cfg.gateCategories) > 0 {
		gated = scopeGateFindings(gated, cfg.gateAnalyzers, cfg.gateCategories)
	}
	if code == 0 && cfg.gate.Enabled() {
		if gate := scans.EvaluateGate(gated, cfg.gate); gate.Failed {
			fmt.Fprintln(stderr, gate.FailureMessage())
			return 1, true
		}
	}
	return code, true
}

// remoteDocumentPath maps --format values to the API export paths that carry
// the same bytes the local renderers produce; text and github never reach
// here (text renders the summary, github is refused up front).
func remoteDocumentPath(format string) string {
	switch format {
	case scanFormatJSON:
		return "findings.json"
	case scanFormatSARIF:
		return "report.sarif"
	case scanFormatCSV:
		return "findings.csv"
	case scanFormatJSONL:
		return "findings.jsonl"
	case scanFormatMarkdown:
		return "report.md"
	}
	return ""
}

func jsonString(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

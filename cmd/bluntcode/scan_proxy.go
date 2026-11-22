package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
	"bluntcode/internal/events"
	"bluntcode/internal/reports"
	"bluntcode/internal/scans"
	"bluntcode/internal/workspace"
)

// runScanThroughServer handles `bluntcode scan` when the data-directory lock
// is held by the live web app. It drives the same scan service through the
// app's authenticated loopback API and renders the CLI contract locally.
func runScanThroughServer(cfg scanConfig, stdout, stderr io.Writer) (int, bool) {
	server, ok := discoverRunningServer()
	if !ok {
		return 0, false
	}
	if cfg.watch {
		fmt.Fprintln(stderr, "bluntcode scan: --watch cannot run while the Blunt Code app holds the data directory; close the app or run without --watch")
		return 2, true
	}
	root, err := workspace.NormalizeRoot(cfg.path)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode scan: %v\n", err)
		return 3, true
	}
	work, err := server.ensureWorkspace(root)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode scan: could not register workspace with the running app: %v\n", err)
		return 3, true
	}
	// Resolve the baseline before the scan starts, mirroring the local path:
	// --save-baseline must not be able to overwrite the reference this scan
	// is compared against.
	var baseline scans.Baseline
	if cfg.baseline != "" {
		baseline, err = server.loadBaseline(cfg.baseline, work)
		if err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: %v\n", err)
			fmt.Fprintln(stderr, scanUsage)
			return 2, true
		}
	}
	scan, err := server.startScan(work.ID, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode scan: could not start scan: %v\n", err)
		return 3, true
	}
	if !cfg.quiet {
		fmt.Fprintf(stderr, "scanning %s (profile %s, timeout %s)\n", work.RootPath, cfg.profile, cfg.timeout)
	}
	eventCh, stopEvents := server.streamEvents(scan.ID)
	defer stopEvents()
	interrupt := make(chan os.Signal, 2)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	ctx := context.Background()
	outcome := awaitScanTerminal(scanWaitInput{
		events:     eventCh,
		interrupts: interrupt,
		timeout:    time.After(cfg.timeout),
		poll: func() (string, bool) {
			state, err := server.scanState(scan.ID)
			if err != nil {
				return "", false
			}
			return state, terminalScanState(state)
		},
		cancel:       func() { _ = server.cancelScan(ctx, scan.ID) },
		timeoutLimit: cfg.timeout,
		quiet:        cfg.quiet,
		errOut:       stderr,
		pollEvery:    scanStatePollInterval,
		cancelGrace:  scanCancelGrace,
	})
	if outcome.exitNow {
		return 130, true
	}
	report, err := server.loadReport(scan.ID)
	if err != nil {
		fmt.Fprintf(stderr, "bluntcode scan: could not load scan result: %v\n", err)
		return 3, true
	}
	summary := report.summary(outcome.finalState, outcome.timedOut)
	code := scanExitCode(summary.state, summary.timedOut)
	documentFormats := cfg.format == scanFormatJSON || cfg.format == scanFormatGitHub || cfg.format == scanFormatSARIF || cfg.format == scanFormatCSV || cfg.format == scanFormatJSONL || cfg.format == scanFormatMarkdown
	if documentFormats {
		document, err := server.document(cfg.format, report, cfg.githubCap)
		if err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: could not load report: %v\n", err)
			return 3, true
		}
		if cfg.output != "" {
			if err := reports.WriteBytesAtomic(cfg.output, document, 0o600); err != nil {
				fmt.Fprintf(stderr, "bluntcode scan: could not write %s: %v\n", cfg.output, err)
				return 3, true
			}
			fmt.Fprintf(stderr, "report written to %s\n", cfg.output)
		} else if _, err := stdout.Write(document); err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: could not write report: %v\n", err)
			return 3, true
		}
	}
	if cfg.saveBaseline != "" && summary.state == "completed" {
		document, err := server.rawDocument("/api/v1/scans/" + url.PathEscape(scan.ID) + "/report.sarif")
		if err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: could not load baseline report: %v\n", err)
			return 3, true
		}
		if err := reports.WriteBytesAtomic(cfg.saveBaseline, document, 0o600); err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: could not write baseline %s: %v\n", cfg.saveBaseline, err)
			return 3, true
		}
		fmt.Fprintf(stderr, "baseline written to %s\n", cfg.saveBaseline)
	}
	switch {
	case !documentFormats && cfg.json:
		if err := writeScanJSON(stdout, summary); err != nil {
			fmt.Fprintf(stderr, "bluntcode scan: could not write summary: %v\n", err)
			return 3, true
		}
	default:
		writeScanHuman(stdout, summary)
	}
	if outcome.interruptSeen {
		return 130, true
	}
	var gatedFindings []analyzers.Finding
	if cfg.baseline != "" {
		known, fresh := baseline.Split(summary.findings)
		gatedFindings = fresh
		fmt.Fprintf(stderr, "baseline: %d known finding(s) excluded from gate, %d new finding(s)\n", len(known), len(fresh))
	} else {
		gatedFindings = summary.findings
	}
	if code == 0 && cfg.gate.Enabled() {
		if len(cfg.gateAnalyzers) > 0 || len(cfg.gateCategories) > 0 {
			narrowed := scopeGateFindings(gatedFindings, cfg.gateAnalyzers, cfg.gateCategories)
			fmt.Fprintf(stderr, "gate scope: %d of %d finding(s) match --gate-analyzer/--gate-category\n", len(narrowed), len(gatedFindings))
			gatedFindings = narrowed
		}
		if gate := scans.EvaluateGate(gatedFindings, cfg.gate); gate.Failed {
			fmt.Fprintln(stderr, gate.FailureMessage())
			return 1, true
		}
	}
	return code, true
}

type runningServer struct {
	baseURL string
	token   string
	client  *http.Client
}

func discoverRunningServer() (runningServer, bool) {
	token := strings.TrimSpace(readServerToken())
	if token == "" {
		return runningServer{}, false
	}
	var ports []int
	if path, err := serverPortPath(); err == nil {
		if raw, err := os.ReadFile(path); err == nil {
			if port, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && port > 0 {
				ports = append(ports, port)
			}
		}
	}
	ports = append(ports, defaultPort)
	for _, port := range ports {
		base := fmt.Sprintf("http://127.0.0.1:%d", port)
		server := runningServer{baseURL: base, token: token, client: &http.Client{}}
		if server.ping() {
			return server, true
		}
	}
	return runningServer{}, false
}

func (s runningServer) ping() bool {
	resp, err := s.request(http.MethodGet, "/api/v1/meta", nil)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return false
	}
	var meta struct {
		APIVersion string `json:"api_version"`
	}
	return json.NewDecoder(resp.Body).Decode(&meta) == nil && meta.APIVersion == "v1"
}

func (s runningServer) request(method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = strings.NewReader(string(data))
	}
	req, err := http.NewRequest(method, s.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return s.client.Do(req)
}

func (s runningServer) do(method, path string, body, out any) error {
	resp, err := s.request(method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var failure struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		if failure.Error.Message != "" {
			return errors.New(failure.Error.Message)
		}
		return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (s runningServer) rawDocument(path string) ([]byte, error) {
	resp, err := s.request(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (s runningServer) ensureWorkspace(path string) (core.Workspace, error) {
	var work core.Workspace
	err := s.do(http.MethodPost, "/api/v1/workspaces", map[string]string{"root_path": path}, &work)
	return work, err
}

func (s runningServer) startScan(workspaceID string, cfg scanConfig) (core.Scan, error) {
	var scan core.Scan
	body := map[string]any{
		"profile":     cfg.profile,
		"target_url":  cfg.target,
		"jobs":        cfg.jobs,
		"incremental": cfg.incremental,
	}
	err := s.do(http.MethodPost, "/api/v1/workspaces/"+url.PathEscape(workspaceID)+"/scans", body, &scan)
	return scan, err
}

func (s runningServer) scanState(scanID string) (string, error) {
	var detail struct {
		State string `json:"state"`
	}
	if err := s.do(http.MethodGet, "/api/v1/scans/"+url.PathEscape(scanID), nil, &detail); err != nil {
		return "", err
	}
	return detail.State, nil
}

func (s runningServer) cancelScan(ctx context.Context, scanID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/api/v1/scans/"+url.PathEscape(scanID)+"/cancel", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (s runningServer) streamEvents(scanID string) (<-chan events.Event, func()) {
	ch := make(chan events.Event, 32)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(ch)
		defer close(done)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/api/v1/scans/"+url.PathEscape(scanID)+"/events", nil)
		if err != nil {
			return
		}
		req.Header.Set("Authorization", "Bearer "+s.token)
		req.Header.Set("Accept", "text/event-stream")
		resp, err := s.client.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			_, _ = io.Copy(io.Discard, resp.Body)
			return
		}
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var eventName, data strings.Builder
		flush := func() {
			if eventName.Len() == 0 || data.Len() == 0 {
				eventName.Reset()
				data.Reset()
				return
			}
			var event events.Event
			if err := json.Unmarshal([]byte(data.String()), &event); err == nil {
				if event.Type == "" {
					event.Type = eventName.String()
				}
				select {
				case ch <- event:
				case <-ctx.Done():
				}
			}
			eventName.Reset()
			data.Reset()
		}
		for scanner.Scan() {
			line := strings.TrimSuffix(scanner.Text(), "\r")
			switch {
			case line == "":
				flush()
			case strings.HasPrefix(line, "event:"):
				eventName.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "event:")))
			case strings.HasPrefix(line, "data:"):
				if data.Len() > 0 {
					data.WriteByte('\n')
				}
				data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		flush()
	}()
	return ch, func() {
		cancel()
		<-done
	}
}

type remoteReport struct {
	Workspace struct {
		Name string `json:"name"`
		Path string `json:"path"`
	} `json:"workspace"`
	Scan struct {
		ID         string `json:"id"`
		Profile    string `json:"profile"`
		State      string `json:"state"`
		StartedAt  string `json:"started_at"`
		FinishedAt string `json:"finished_at"`
	} `json:"scan"`
	Severity struct {
		Critical int `json:"critical"`
		High     int `json:"high"`
		Medium   int `json:"medium"`
		Low      int `json:"low"`
		Info     int `json:"info"`
		Total    int `json:"total"`
	} `json:"severity"`
	Analyzers []struct {
		ID           string `json:"id"`
		Version      string `json:"version"`
		State        string `json:"state"`
		Findings     int    `json:"findings"`
		WarningCount int    `json:"warning_count"`
		DurationMS   int64  `json:"duration_ms"`
		Error        string `json:"error"`
	} `json:"analyzers"`
	Findings   []remoteFinding `json:"findings"`
	Comparison struct {
		New        int `json:"new"`
		Fixed      int `json:"fixed"`
		Persistent int `json:"persistent"`
	} `json:"comparison"`
}

type remoteFinding struct {
	Severity         string `json:"severity"`
	Category         string `json:"category"`
	AnalyzerID       string `json:"analyzer_id"`
	RuleID           string `json:"rule_id"`
	Title            string `json:"title"`
	Message          string `json:"message"`
	File             string `json:"file"`
	StartLine        int    `json:"start_line"`
	StartColumn      int    `json:"start_column"`
	EndLine          int    `json:"end_line"`
	EndColumn        int    `json:"end_column"`
	Status           string `json:"status"`
	Fingerprint      string `json:"fingerprint"`
	Remediation      string `json:"remediation"`
	DocumentationURL string `json:"documentation_url"`
}

func (r remoteFinding) finding() analyzers.Finding {
	return analyzers.Finding{
		AnalyzerID:       r.AnalyzerID,
		RuleID:           r.RuleID,
		Fingerprint:      r.Fingerprint,
		Severity:         analyzers.Severity(r.Severity),
		Category:         analyzers.Category(r.Category),
		Title:            r.Title,
		Message:          r.Message,
		RelativePath:     r.File,
		StartLine:        r.StartLine,
		StartColumn:      r.StartColumn,
		EndLine:          r.EndLine,
		EndColumn:        r.EndColumn,
		Remediation:      r.Remediation,
		DocumentationURL: r.DocumentationURL,
		Status:           r.Status,
	}
}

func (s runningServer) loadReport(scanID string) (remoteReport, error) {
	var report remoteReport
	data, err := s.rawDocument("/api/v1/scans/" + url.PathEscape(scanID) + "/findings.json")
	if err != nil {
		return report, err
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return report, err
	}
	return report, nil
}

func (r remoteReport) summary(state string, timedOut bool) scanSummary {
	summary := scanSummary{
		scanID:        r.Scan.ID,
		workspace:     r.Workspace.Name,
		workspacePath: r.Workspace.Path,
		profile:       r.Scan.Profile,
		state:         state,
		timedOut:      timedOut,
		critical:      r.Severity.Critical,
		high:          r.Severity.High,
		medium:        r.Severity.Medium,
		low:           r.Severity.Low,
		info:          r.Severity.Info,
		total:         r.Severity.Total,
		hasPrevious:   r.Comparison.New+r.Comparison.Fixed+r.Comparison.Persistent > 0,
		newCount:      r.Comparison.New,
		fixedCount:    r.Comparison.Fixed,
		persistent:    r.Comparison.Persistent,
		runs:          make([]reports.Run, 0, len(r.Analyzers)),
		findings:      make([]analyzers.Finding, 0, len(r.Findings)),
	}
	if started, err := time.Parse(time.RFC3339Nano, r.Scan.StartedAt); err == nil {
		summary.startedAt = started
	}
	if finished, err := time.Parse(time.RFC3339Nano, r.Scan.FinishedAt); err == nil {
		summary.finishedAt = finished
	}
	for _, run := range r.Analyzers {
		summary.runs = append(summary.runs, reports.Run{
			AnalyzerID:   run.ID,
			DisplayName:  scanAnalyzerDisplayName(run.ID),
			Version:      run.Version,
			State:        run.State,
			FindingCount: run.Findings,
			WarningCount: run.WarningCount,
			Duration:     time.Duration(run.DurationMS) * time.Millisecond,
			ErrorSummary: run.Error,
		})
	}
	for _, finding := range r.Findings {
		summary.findings = append(summary.findings, finding.finding())
	}
	return summary
}

func (s runningServer) document(format string, report remoteReport, githubCap int) ([]byte, error) {
	switch format {
	case scanFormatJSON:
		return s.rawDocument("/api/v1/scans/" + url.PathEscape(report.Scan.ID) + "/findings.json")
	case scanFormatSARIF:
		return s.rawDocument("/api/v1/scans/" + url.PathEscape(report.Scan.ID) + "/report.sarif")
	case scanFormatCSV:
		return s.rawDocument("/api/v1/scans/" + url.PathEscape(report.Scan.ID) + "/findings.csv")
	case scanFormatJSONL:
		return s.rawDocument("/api/v1/scans/" + url.PathEscape(report.Scan.ID) + "/findings.jsonl")
	case scanFormatMarkdown:
		return s.rawDocument("/api/v1/scans/" + url.PathEscape(report.Scan.ID) + "/report.md")
	case scanFormatGitHub:
		model := reports.Model{Input: reports.Input{WorkspaceName: report.Workspace.Name, ScanID: report.Scan.ID, Findings: report.findings()}, Counts: report.counts()}
		return reports.GitHubAnnotationsWithCap(model, githubCap), nil
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

func (r remoteReport) findings() []analyzers.Finding {
	out := make([]analyzers.Finding, 0, len(r.Findings))
	for _, finding := range r.Findings {
		out = append(out, finding.finding())
	}
	return out
}

func (r remoteReport) counts() map[analyzers.Severity]int {
	counts := map[analyzers.Severity]int{}
	for _, finding := range r.Findings {
		counts[analyzers.Severity(finding.Severity)]++
	}
	return counts
}

func (s runningServer) loadBaseline(ref string, work core.Workspace) (scans.Baseline, error) {
	if ref == "" {
		return scans.Baseline{}, nil
	}
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
	report, err := s.loadReport(ref)
	if err != nil {
		return scans.Baseline{}, fmt.Errorf("baseline %q not found: it is neither an existing file nor a known scan ID", ref)
	}
	if report.Workspace.Path != "" && work.RootPath != "" && report.Workspace.Path != work.RootPath {
		return scans.Baseline{}, fmt.Errorf("baseline scan %q belongs to a different workspace", ref)
	}
	return scans.BaselineFromFindings(report.findings()), nil
}

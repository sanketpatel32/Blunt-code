package scans

// This file holds the analyzer execution core shared by both scheduling
// models: the per-analyzer pipeline (executeAnalyzer) and the two drivers
// that schedule it - the historical sequential loop (the default) and the
// bounded parallel runner behind ScanOptions.Jobs.

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
)

// ScanOptions tunes the execution of a single scan. The zero value keeps the
// historical behavior for every option.
type ScanOptions struct {
	// Jobs bounds how many analyzer runs may execute concurrently. Values
	// below 1 keep the default execution model (analyzers run one after
	// another in registry order); a value of N >= 1 runs at most N analyzers
	// at once. Every analyzer still runs exactly once - only the scheduling
	// changes, and events, persistence, and cancellation semantics stay the
	// same.
	Jobs int
	// Incremental reuses the previous completed scan's findings for files
	// whose content is unchanged and runs the analyzers only on changed or
	// added files. Reuse is refused (and the scan silently degrades to a full
	// run) when there is no previous completed scan, when it recorded no file
	// hashes, or when the analyzer identity (ids, versions, profile, Blunt
	// Code build) changed. Totals, suppression, and comparison semantics are
	// identical to a full scan.
	Incremental bool
}

// analyzerOutcome classifies how one analyzer execution ended.
type analyzerOutcome int

const (
	analyzerSucceeded analyzerOutcome = iota
	analyzerFailed
	analyzerSkipped
	// analyzerAborted means the scan context was cancelled during or right
	// after the analyzer ran: the run result must not be persisted and the
	// coordinator has to finish the scan as cancelled.
	analyzerAborted
)

// scanCounters aggregates per-scan analyzer outcomes. Sequential scans mutate
// it from a single goroutine, bounded scans from many, so the counters stay
// atomic and the succeeded-analyzer set is mutex-guarded.
type scanCounters struct {
	successful   atomic.Int64
	failed       atomic.Int64
	mu           sync.Mutex
	succeededIDs map[string]bool
}

func newScanCounters() *scanCounters {
	return &scanCounters{succeededIDs: map[string]bool{}}
}

func (c *scanCounters) markSucceeded(analyzerID string) {
	c.successful.Add(1)
	c.mu.Lock()
	c.succeededIDs[analyzerID] = true
	c.mu.Unlock()
}
func (c *scanCounters) markFailed() { c.failed.Add(1) }

// succeeded reports whether the analyzer completed a successful run in this
// scan. Incremental reuse consults it to distinguish "skipped because nothing
// changed" from "ran and failed".
func (c *scanCounters) succeeded(analyzerID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.succeededIDs[analyzerID]
}

func (c *scanCounters) snapshot() (successful, failed int) {
	return int(c.successful.Load()), int(c.failed.Load())
}

// executeAnalyzer runs the full pipeline for one adapter: language gating,
// profile filtering, tool readiness (with managed installation), planning,
// execution, normalization, persistence, and events. It is the exact body the
// sequential loop historically ran inline, extracted so the bounded driver can
// share it; the loop's continue/return statements became outcome returns.
func (s *Service) executeAnalyzer(ctx context.Context, scan core.Scan, work core.Workspace, languages []analyzers.Language, filesByLanguage map[analyzers.Language][]string, adapter analyzers.Analyzer, counters *scanCounters) analyzerOutcome {
	// Each adapter is handed only the selected files whose language it
	// supports. Gating on the workspace language list alone was not
	// enough: a mixed-language workspace used to pass every file to
	// every analyzer, so ruff received minified JavaScript and failed
	// parsing it as Python.
	adapterFiles := filesForLanguages(filesByLanguage, adapter.SupportedLanguages()...)
	if len(adapterFiles) == 0 {
		return analyzerSkipped
	}
	// Profile tiers:
	//   quick    - ruff and biome only: a fast language-specific pass that
	//              skips semgrep and sonarqube entirely.
	//   standard - every code analyzer with its default rules, including
	//              ruff's built-in default rule set (E4, E7, E9, F);
	//              deep-only analyzers (osv-dependencies, later trivy and
	//              checkov) are recorded as skipped.
	//   deep     - every analyzer, with ruff's rule selection widened via
	//              --select for a broader correctness and maintainability
	//              sweep. Biome, semgrep, and sonarqube already run their
	//              full configuration in every non-quick tier, so ruff is
	//              the only analyzer that changes behavior here.
	if !profileAllowsAnalyzer(scan.Profile, adapter.ID()) {
		reason := "Quick profile runs language-specific analyzers only."
		if scan.Profile != analyzers.ProfileQuick {
			reason = "Deep-only analyzer: runs on deep scans."
		}
		s.emit(scan.ID, "analyzer.skipped", map[string]any{"analyzer_id": adapter.ID(), "reason": reason})
		return analyzerSkipped
	}
	started := time.Now()
	s.emit(scan.ID, "analyzer.started", map[string]any{"analyzer_id": adapter.ID(), "name": adapter.DisplayName()})
	_ = s.db.UpdateScanState(context.Background(), scan.ID, "running", "")
	status := adapter.Check(ctx, analyzers.ToolEnvironment{ToolsDir: s.toolsDir})
	if !status.Ready && s.tools != nil && (adapter.ID() == "ruff" || adapter.ID() == "biome" || adapter.ID() == "gitleaks-secrets" || adapter.ID() == "osv-dependencies" || adapter.ID() == "container-trivy" || adapter.ID() == "iac-checkov" || adapter.ID() == "semgrep" || adapter.ID() == "sonarqube") {
		s.emit(scan.ID, "scan.stage", map[string]any{"stage": "Preparing " + adapter.DisplayName()})
		if installErr := s.tools.Ensure(ctx, adapter.ID()); installErr == nil {
			status = adapter.Check(ctx, analyzers.ToolEnvironment{ToolsDir: s.toolsDir})
		} else {
			status.Detail = installErr.Error()
		}
	}
	if !status.Ready {
		errorText := status.Detail
		if errorText == "" {
			errorText = "Analyzer is not ready."
		}
		_, _ = s.db.SaveAnalyzerResult(context.Background(), scan.ID, database.AnalyzerRunInput{AnalyzerID: adapter.ID(), Version: status.Version, State: "failed", StartedAt: started, FinishedAt: time.Now(), Error: errorText}, nil, nil)
		counters.markFailed()
		s.emit(scan.ID, "analyzer.failed", map[string]any{"analyzer_id": adapter.ID(), "error": errorText})
		return analyzerFailed
	}
	plan, err := adapter.Plan(ctx, analyzers.ScanRequest{WorkspaceID: work.ID, ScanID: scan.ID, WorkspaceRoot: work.RootPath, Files: adapterFiles, Languages: languages, Profile: scan.Profile})
	if err != nil {
		_, _ = s.db.SaveAnalyzerResult(context.Background(), scan.ID, database.AnalyzerRunInput{AnalyzerID: adapter.ID(), Version: status.Version, State: "failed", StartedAt: started, FinishedAt: time.Now(), Error: err.Error()}, nil, nil)
		counters.markFailed()
		s.emit(scan.ID, "analyzer.failed", map[string]any{"analyzer_id": adapter.ID(), "error": err.Error()})
		return analyzerFailed
	}
	analyzerCtx, analyzerCancel := context.WithTimeout(ctx, analyzerTimeout(adapter.ID()))
	result, err := adapter.Run(analyzerCtx, plan, eventEmitter{service: s, scanID: scan.ID})
	analyzerCancel()
	finished := time.Now()
	s.writeDiagnosticLog(scan.ID, adapter.ID(), result)
	if ctx.Err() != nil {
		return analyzerAborted
	}
	if err == nil {
		var findings []analyzers.Finding
		var metrics []analyzers.Metric
		findings, metrics, err = adapter.Normalize(ctx, result)
		if err == nil {
			_, err = s.db.SaveAnalyzerResult(context.Background(), scan.ID, database.AnalyzerRunInput{AnalyzerID: adapter.ID(), Version: plan.Version, State: "succeeded", StartedAt: started, FinishedAt: finished, ExitCode: result.ExitCode}, findings, metrics)
			if err == nil {
				counters.markSucceeded(adapter.ID())
				s.emit(scan.ID, "analyzer.completed", map[string]any{"analyzer_id": adapter.ID(), "findings": len(findings)})
				return analyzerSucceeded
			}
		}
	}
	errorText := err.Error()
	_, _ = s.db.SaveAnalyzerResult(context.Background(), scan.ID, database.AnalyzerRunInput{AnalyzerID: adapter.ID(), Version: plan.Version, State: "failed", StartedAt: started, FinishedAt: finished, ExitCode: result.ExitCode, Error: errorText}, nil, nil)
	counters.markFailed()
	s.emit(scan.ID, "analyzer.failed", map[string]any{"analyzer_id": adapter.ID(), "error": errorText})
	return analyzerFailed
}

// runAnalyzersSequential is the historical execution model and stays the
// default: analyzers run one at a time, in registry order, and a cancellation
// observed between runs or right after a run stops the whole scan. It returns
// false when the scan was cancelled; the scan is already marked cancelled, so
// the caller must stop without writing a report.
func (s *Service) runAnalyzersSequential(ctx context.Context, scan core.Scan, work core.Workspace, languages []analyzers.Language, filesByLanguage map[analyzers.Language][]string, counters *scanCounters) bool {
	for _, adapter := range s.registry.All() {
		if ctx.Err() != nil {
			s.finishCancelled(scan.ID)
			return false
		}
		if s.executeAnalyzer(ctx, scan, work, languages, filesByLanguage, adapter, counters) == analyzerAborted {
			s.finishCancelled(scan.ID)
			return false
		}
	}
	return true
}

// runAnalyzersBounded runs every registered analyzer with at most jobs runs in
// flight at once. A run waiting for a slot aborts as soon as the scan context
// is cancelled; in-flight runs observe cancellation through their per-analyzer
// contexts and skip persisting their results. The return value is the message
// of a recovered adapter panic (empty when none panicked): the caller fails
// the scan with it, mirroring what run()'s recover does for sequential scans.
func (s *Service) runAnalyzersBounded(ctx context.Context, scan core.Scan, work core.Workspace, languages []analyzers.Language, filesByLanguage map[analyzers.Language][]string, counters *scanCounters, jobs int) string {
	sem := make(chan struct{}, jobs)
	var wg sync.WaitGroup
	var panicValue atomic.Value
	for _, adapter := range s.registry.All() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A panicking adapter must not take the process down. run()'s
			// recover cannot see worker goroutines, so a panic is contained
			// here and reported to the coordinator.
			defer func() {
				if r := recover(); r != nil {
					panicValue.Store(fmt.Sprintf("%v", r))
				}
			}()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			s.executeAnalyzer(ctx, scan, work, languages, filesByLanguage, adapter, counters)
		}()
	}
	wg.Wait()
	message, _ := panicValue.Load().(string)
	return message
}

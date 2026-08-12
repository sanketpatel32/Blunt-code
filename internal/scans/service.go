// Package scans orchestrates analyzer-independent scan lifecycle work.
package scans

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/discovery"
	"bluntcode/internal/events"
	"bluntcode/internal/reports"
	"bluntcode/internal/tools"
)

type Service struct {
	db         *database.DB
	registry   *analyzers.Registry
	bus        *events.Bus
	reportsDir string
	toolsDir   string
	tools      *tools.Service
	mu         sync.Mutex
	cancels    map[string]context.CancelFunc
}

const (
	fastAnalyzerTimeout  = 10 * time.Minute
	sonarAnalyzerTimeout = 30 * time.Minute
	diagnosticLogCap     = int64(500 << 20)
)

func New(db *database.DB, registry *analyzers.Registry, bus *events.Bus, reportsDir, toolsDir string, toolService *tools.Service) *Service {
	return &Service{db: db, registry: registry, bus: bus, reportsDir: reportsDir, toolsDir: toolsDir, tools: toolService, cancels: map[string]context.CancelFunc{}}
}

func (s *Service) Start(ctx context.Context, work core.Workspace, profile string, files []core.FileEntry) (core.Scan, error) {
	return s.start(ctx, work, profile, files, nil)
}

func (s *Service) start(ctx context.Context, work core.Workspace, profile string, files []core.FileEntry, excludes []string) (core.Scan, error) {
	if profile == "" {
		profile = "standard"
	}
	snapshot := s.snapshot(ctx, work, profile, files, excludes)
	scan, err := s.db.CreateScanWithFiles(ctx, core.Scan{WorkspaceID: work.ID, Profile: profile, State: "queued", CandidateFileCount: len(files), SelectedFileCount: snapshot.SelectedFileCount, Snapshot: snapshot}, files)
	if err != nil {
		return core.Scan{}, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[scan.ID] = cancel
	s.mu.Unlock()
	go s.run(runCtx, scan, work, files)
	return scan, nil
}

func (s *Service) snapshot(ctx context.Context, work core.Workspace, profile string, files []core.FileEntry, excludes []string) *core.ScanSnapshot {
	languages := map[string]int{}
	selectedFiles := make([]string, 0, len(files))
	for _, file := range files {
		if file.Language != "" {
			languages[file.Language]++
		}
		if file.Selected {
			selectedFiles = append(selectedFiles, file.RelativePath)
		}
	}
	sort.Strings(selectedFiles)
	rules, err := s.db.Rules(ctx, work.ID)
	if err != nil {
		rules = nil
	}
	overrides, err := s.db.PathOverrides(ctx, work.ID)
	if err != nil {
		overrides = nil
	}
	analyzerIDs := make([]string, 0)
	analyzerVersions := map[string]string{}
	for _, adapter := range s.registry.All() {
		analyzerIDs = append(analyzerIDs, adapter.ID())
		analyzerVersions[adapter.ID()] = adapter.Check(ctx, analyzers.ToolEnvironment{ToolsDir: s.toolsDir}).Version
	}
	sort.Strings(analyzerIDs)
	return &core.ScanSnapshot{
		WorkspaceID:        work.ID,
		WorkspaceRoot:      work.RootPath,
		WorkspaceName:      work.Name,
		BluntCodeVersion:   "0.1.0-dev",
		Profile:            profile,
		CandidateFileCount: len(files),
		SelectedFileCount:  len(selectedFiles),
		SelectedFiles:      selectedFiles,
		Languages:          languages,
		Rules:              rules,
		Exclusions:         append([]string(nil), excludes...),
		PathOverrides:      overrides,
		EnabledAnalyzers:   analyzerIDs,
		AnalyzerVersions:   analyzerVersions,
		Git:                gitSnapshot(ctx, work.RootPath),
	}
}

func gitSnapshot(ctx context.Context, root string) core.ScanGitSnapshot {
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	inside, err := gitOutput(checkCtx, root, "rev-parse", "--is-inside-work-tree")
	if err != nil || inside != "true" {
		return core.ScanGitSnapshot{}
	}
	branch, _ := gitOutput(checkCtx, root, "branch", "--show-current")
	commit, _ := gitOutput(checkCtx, root, "rev-parse", "HEAD")
	status, _ := gitOutput(checkCtx, root, "status", "--porcelain")
	return core.ScanGitSnapshot{Repository: true, Branch: branch, Commit: commit, Dirty: status != ""}
}

func gitOutput(ctx context.Context, root string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	output, err := command.Output()
	return strings.TrimSpace(string(output)), err
}
func (s *Service) Cancel(ctx context.Context, scanID string) error {
	s.mu.Lock()
	cancel := s.cancels[scanID]
	s.mu.Unlock()
	if cancel == nil {
		return fmt.Errorf("scan is not active")
	}
	cancel()
	return nil
}

// Shutdown cancels every running scan before managed analyzers are stopped.
func (s *Service) Shutdown() {
	s.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.cancels))
	for _, cancel := range s.cancels {
		cancels = append(cancels, cancel)
	}
	s.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}
func (s *Service) emit(scanID, event string, data map[string]any) {
	s.bus.Publish(events.Event{Type: event, ScanID: scanID, Data: data})
}
func (s *Service) run(ctx context.Context, scan core.Scan, work core.Workspace, files []core.FileEntry) {
	defer func() { s.mu.Lock(); delete(s.cancels, scan.ID); s.mu.Unlock() }()
	s.emit(scan.ID, "scan.started", map[string]any{"state": "queued"})
	s.transition(scan.ID, "preparing", map[string]any{"stage": "Preparing workspace"})
	languages, absoluteFiles := scanInputs(work.RootPath, files)
	if len(absoluteFiles) == 0 {
		_ = s.db.UpdateScanState(context.Background(), scan.ID, "failed", "No supported source files were selected.")
		s.emit(scan.ID, "scan.completed", map[string]any{"state": "failed"})
		return
	}
	var successful, failed int
	for _, adapter := range s.registry.All() {
		if ctx.Err() != nil {
			s.finishCancelled(scan.ID)
			return
		}
		if !analyzers.HasLanguage(languages, adapter.SupportedLanguages()...) {
			continue
		}
		if scan.Profile == "quick" && adapter.ID() != "ruff" && adapter.ID() != "biome" {
			s.emit(scan.ID, "analyzer.skipped", map[string]any{"analyzer_id": adapter.ID(), "reason": "Quick profile runs language-specific analyzers only."})
			continue
		}
		started := time.Now()
		s.emit(scan.ID, "analyzer.started", map[string]any{"analyzer_id": adapter.ID(), "name": adapter.DisplayName()})
		_ = s.db.UpdateScanState(context.Background(), scan.ID, "running", "")
		status := adapter.Check(ctx, analyzers.ToolEnvironment{ToolsDir: s.toolsDir})
		if !status.Ready && s.tools != nil && (adapter.ID() == "ruff" || adapter.ID() == "biome" || adapter.ID() == "semgrep" || adapter.ID() == "sonarqube") {
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
			failed++
			s.emit(scan.ID, "analyzer.failed", map[string]any{"analyzer_id": adapter.ID(), "error": errorText})
			continue
		}
		plan, err := adapter.Plan(ctx, analyzers.ScanRequest{WorkspaceID: work.ID, ScanID: scan.ID, WorkspaceRoot: work.RootPath, Files: absoluteFiles, Languages: languages})
		if err != nil {
			_, _ = s.db.SaveAnalyzerResult(context.Background(), scan.ID, database.AnalyzerRunInput{AnalyzerID: adapter.ID(), Version: status.Version, State: "failed", StartedAt: started, FinishedAt: time.Now(), Error: err.Error()}, nil, nil)
			failed++
			s.emit(scan.ID, "analyzer.failed", map[string]any{"analyzer_id": adapter.ID(), "error": err.Error()})
			continue
		}
		analyzerCtx, analyzerCancel := context.WithTimeout(ctx, analyzerTimeout(adapter.ID()))
		result, err := adapter.Run(analyzerCtx, plan, eventEmitter{service: s, scanID: scan.ID})
		analyzerCancel()
		finished := time.Now()
		s.writeDiagnosticLog(scan.ID, adapter.ID(), result)
		if ctx.Err() != nil {
			s.finishCancelled(scan.ID)
			return
		}
		if err == nil {
			var findings []analyzers.Finding
			var metrics []analyzers.Metric
			findings, metrics, err = adapter.Normalize(ctx, result)
			if err == nil {
				_, err = s.db.SaveAnalyzerResult(context.Background(), scan.ID, database.AnalyzerRunInput{AnalyzerID: adapter.ID(), Version: plan.Version, State: "succeeded", StartedAt: started, FinishedAt: finished, ExitCode: result.ExitCode}, findings, metrics)
				if err == nil {
					successful++
					s.emit(scan.ID, "analyzer.completed", map[string]any{"analyzer_id": adapter.ID(), "findings": len(findings)})
					continue
				}
			}
		}
		errorText := err.Error()
		_, _ = s.db.SaveAnalyzerResult(context.Background(), scan.ID, database.AnalyzerRunInput{AnalyzerID: adapter.ID(), Version: plan.Version, State: "failed", StartedAt: started, FinishedAt: finished, ExitCode: result.ExitCode, Error: errorText}, nil, nil)
		failed++
		s.emit(scan.ID, "analyzer.failed", map[string]any{"analyzer_id": adapter.ID(), "error": errorText})
	}
	state := "completed"
	if successful == 0 {
		state = "failed"
	}
	if successful > 0 && failed > 0 {
		state = "completed_with_warnings"
	}
	s.emit(scan.ID, "scan.stage", map[string]any{"stage": "Generating report"})
	reportPath, err := s.writeReport(scan, work, files)
	if err != nil {
		state = "completed_with_warnings"
		s.emit(scan.ID, "scan.warning", map[string]any{"message": "Could not write Markdown report."})
	}
	if err := s.db.CompleteScan(context.Background(), scan.ID, state, reportPath); err != nil {
		return
	}
	s.emit(scan.ID, "scan.completed", map[string]any{"state": state})
}

func analyzerTimeout(id string) time.Duration {
	if id == "sonarqube" {
		return sonarAnalyzerTimeout
	}
	return fastAnalyzerTimeout
}

func (s *Service) writeDiagnosticLog(scanID, analyzerID string, result analyzers.AnalyzerResult) {
	if len(result.Stdout) == 0 && len(result.Stderr) == 0 {
		return
	}
	dir := filepath.Join(filepath.Dir(s.reportsDir), "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	content := "stdout:\n" + redactDiagnosticOutput(string(result.Stdout)) + "\n\nstderr:\n" + redactDiagnosticOutput(string(result.Stderr)) + "\n"
	if result.OutputTruncated {
		content += "\n[output truncated at the managed safety limit]\n"
	}
	_ = os.WriteFile(filepath.Join(dir, scanID+"-"+analyzerID+".log"), []byte(content), 0o600)
	pruneDiagnosticLogs(dir)
}

func redactDiagnosticOutput(output string) string {
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "authorization") {
			if index := strings.IndexAny(line, "=:"); index >= 0 {
				lines[i] = line[:index+1] + " ***"
			}
		}
	}
	return strings.Join(lines, "\n")
}

func pruneDiagnosticLogs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type file struct {
		path string
		info os.FileInfo
	}
	files := make([]file, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".log" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		total += info.Size()
		files = append(files, file{path: filepath.Join(dir, entry.Name()), info: info})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].info.ModTime().Before(files[j].info.ModTime()) })
	for _, item := range files {
		if total <= diagnosticLogCap {
			return
		}
		if err := os.Remove(item.path); err == nil {
			total -= item.info.Size()
		}
	}
}
func (s *Service) transition(scanID, state string, data map[string]any) {
	_ = s.db.UpdateScanState(context.Background(), scanID, state, "")
	s.emit(scanID, "scan.stage", data)
}
func (s *Service) finishCancelled(scanID string) {
	_ = s.db.UpdateScanState(context.Background(), scanID, "cancelled", "Cancelled by user.")
	s.emit(scanID, "scan.cancelled", nil)
}
func scanInputs(root string, files []core.FileEntry) ([]analyzers.Language, []string) {
	languageSet := map[analyzers.Language]bool{}
	var absolute []string
	for _, file := range files {
		if !file.Selected || file.Language == "" {
			continue
		}
		languageSet[analyzers.Language(file.Language)] = true
		absolute = append(absolute, filepath.Join(root, file.RelativePath))
	}
	var languages []analyzers.Language
	for _, lang := range []analyzers.Language{analyzers.LanguagePython, analyzers.LanguageJavaScript, analyzers.LanguageTypeScript} {
		if languageSet[lang] {
			languages = append(languages, lang)
		}
	}
	return languages, absolute
}

type eventEmitter struct {
	service *Service
	scanID  string
}

func (e eventEmitter) Emit(_ context.Context, event string, data map[string]any) {
	e.service.emit(e.scanID, event, data)
}
func (s *Service) writeReport(scan core.Scan, work core.Workspace, files []core.FileEntry) (string, error) {
	findings, err := s.db.Findings(context.Background(), scan.ID)
	if err != nil {
		return "", err
	}
	metrics, err := s.db.Metrics(context.Background(), scan.ID)
	if err != nil {
		return "", err
	}
	runs, err := s.db.AnalyzerRuns(context.Background(), scan.ID)
	if err != nil {
		return "", err
	}
	comparison := reports.Comparison{}
	if previousID, previousErr := s.db.PreviousCompletedScanID(context.Background(), work.ID, scan.ID); previousErr == nil {
		previousFindings, findErr := s.db.Findings(context.Background(), previousID)
		coverage, coverageErr := s.db.SuccessfulAnalyzerIDs(context.Background(), scan.ID)
		if findErr == nil && coverageErr == nil {
			diff := Compare(findings, previousFindings, coverage)
			comparison = reports.Comparison{New: diff.New, Fixed: diff.Fixed, Persistent: diff.Persistent, UnknownAnalyzerIDs: diff.UnknownAnalyzerIDs}
		}
	}
	var selected, skipped []string
	for _, file := range files {
		if file.Selected {
			selected = append(selected, file.RelativePath)
		} else {
			skipped = append(skipped, file.RelativePath)
		}
	}
	markdown := reports.Markdown(reports.Build(reports.Input{WorkspaceName: work.Name, WorkspacePath: work.RootPath, ScanID: scan.ID, Profile: scan.Profile, StartedAt: *scan.StartedAt, Files: selected, SkippedFiles: skipped, Findings: findings, Metrics: metrics, Runs: runs, Comparison: comparison}))
	if err := os.MkdirAll(s.reportsDir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(s.reportsDir, reports.Filename(work.Name, time.Now()))
	return path, os.WriteFile(path, []byte(markdown), 0o600)
}

// DiscoverAndStart is the API-friendly entry that keeps discovery out of HTTP handlers.
func (s *Service) DiscoverAndStart(ctx context.Context, work core.Workspace, profile string, excludes []string) (core.Scan, error) {
	found, err := discovery.Discover(ctx, work.RootPath, excludes)
	if err != nil {
		return core.Scan{}, err
	}
	overrides, err := s.db.PathOverrides(ctx, work.ID)
	if err != nil {
		return core.Scan{}, err
	}
	applyPathOverrides(found.Files, overrides)
	return s.start(ctx, work, profile, found.Files, excludes)
}

func applyPathOverrides(files []core.FileEntry, overrides []core.PathOverride) {
	for index := range files {
		bestLength := -1
		bestMode := ""
		path := filepath.ToSlash(files[index].RelativePath)
		for _, override := range overrides {
			overridePath := strings.Trim(filepath.ToSlash(override.RelativePath), "/")
			if overridePath == "" || (path != overridePath && !strings.HasPrefix(path, overridePath+"/")) {
				continue
			}
			if len(overridePath) > bestLength {
				bestLength, bestMode = len(overridePath), override.Mode
			}
		}
		if bestMode != "" {
			files[index].Selected = bestMode == "include"
		}
	}
}

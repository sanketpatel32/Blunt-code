package scans

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
	"bluntcode/internal/discovery"
)

// ErrWorkspaceRootMissing marks failures caused by the workspace's configured
// root path no longer being a readable directory — a deleted or moved folder,
// not a database error. Handlers map it to a 404-style response so a stale
// workspace fails scans with the real reason instead of "could not create
// scan".
var ErrWorkspaceRootMissing = errors.New("workspace root unavailable")

// ScanPlan describes current inputs and routing without starting a scan.
// Files may change between this review and the subsequent scan's discovery.
type ScanPlan struct {
	Profile          string            `json:"profile"`
	CandidateFiles   int               `json:"candidate_files"`
	SelectedFiles    int               `json:"selected_files"`
	DependencyInputs []string          `json:"dependency_inputs"`
	Exclusions       []string          `json:"exclusions"`
	SkipCounts       map[string]int    `json:"skip_counts"`
	Analyzers        []PlannedAnalyzer `json:"analyzers"`
	// Setup is present when the planned analyzers need managed tools that are
	// not installed yet; it carries the pinned-download estimate for review.
	Setup *ScanSetup `json:"setup,omitempty"`
}

// ScanSetup summarizes first-run managed-tool downloads for a planned scan.
type ScanSetup struct {
	Tools         int   `json:"tools"`
	DownloadBytes int64 `json:"download_bytes"`
	Estimated     bool  `json:"estimated"`
}
type PlannedAnalyzer struct {
	AnalyzerStatus
	Planned bool   `json:"planned"`
	Reason  string `json:"reason"`
}

func (s *Service) discoverInputs(ctx context.Context, work core.Workspace, excludes []string) (discovery.Result, []string, error) {
	var found discovery.Result
	info, err := os.Stat(work.RootPath)
	switch {
	case err == nil && !info.IsDir():
		return found, excludes, fmt.Errorf("%w: %s is a file, not a directory", ErrWorkspaceRootMissing, work.RootPath)
	case errors.Is(err, fs.ErrNotExist):
		return found, excludes, fmt.Errorf("%w: %s no longer exists", ErrWorkspaceRootMissing, work.RootPath)
	case err != nil:
		return found, excludes, fmt.Errorf("%w: %s", ErrWorkspaceRootMissing, err)
	}
	excludes = discovery.WorkspaceExcludes(work.RootPath, excludes)
	found, err = discovery.Discover(ctx, work.RootPath, excludes)
	if err != nil {
		return found, excludes, err
	}
	overrides, err := s.db.PathOverrides(ctx, work.ID)
	if err != nil {
		return found, excludes, err
	}
	applyPathOverrides(found.Files, overrides)
	return found, excludes, nil
}

func (s *Service) Plan(ctx context.Context, work core.Workspace, profile string, excludes []string) (ScanPlan, error) {
	found, excludes, err := s.discoverInputs(ctx, work, excludes)
	if err != nil {
		return ScanPlan{}, err
	}
	_, selected, byLanguage := scanInputs(work.RootPath, found.Files)
	plan := ScanPlan{Profile: profile, CandidateFiles: len(found.Files), SelectedFiles: len(selected), DependencyInputs: found.DependencyInputs, Exclusions: excludes, SkipCounts: found.SkipCounts, Analyzers: []PlannedAnalyzer{}}
	setupSeen := map[string]bool{}
	setupIDs := []string{}
	for _, status := range s.AnalyzerStatuses(ctx) {
		row := PlannedAnalyzer{AnalyzerStatus: status}
		adapter, registered := s.registry.Get(status.ID)
		switch {
		case !registered:
			row.Reason = "Unavailable in the current runtime"
		case !analyzers.ProfileAllows(profile, status.ID):
			row.Reason = "Excluded by profile"
		case analyzers.RequiresTarget(status.ID):
			row.Planned = true
			row.Reason = "Runs only when the scan names a target URL"
		case len(filesForLanguages(byLanguage, adapter.SupportedLanguages()...)) == 0 && !(analyzers.TakesDependencyInputs(status.ID) && len(found.DependencyInputs) > 0):
			row.Reason = "No applicable selected inputs"
		default:
			row.Planned = true
			row.Reason = "Ready to run"
			if !status.Ready {
				row.Reason = "Setup required; managed tools may be installed when the scan starts"
			}
		}
		if row.Planned && !row.Ready && row.ManagedTool != "" && !setupSeen[row.ManagedTool] {
			setupSeen[row.ManagedTool] = true
			setupIDs = append(setupIDs, row.ManagedTool)
		}
		plan.Analyzers = append(plan.Analyzers, row)
	}
	if len(setupIDs) > 0 {
		setup := &ScanSetup{Tools: len(setupIDs), Estimated: true}
		if s.tools != nil {
			estimate := s.tools.InstallEstimates(setupIDs)
			setup.DownloadBytes = estimate.Bytes
			setup.Estimated = estimate.Estimated
		}
		plan.Setup = setup
	}
	return plan, nil
}

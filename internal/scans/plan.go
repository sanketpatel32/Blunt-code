package scans

import (
	"bluntcode/internal/analyzers"
	"bluntcode/internal/core"
	"bluntcode/internal/discovery"
	"context"
)

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
}
type PlannedAnalyzer struct {
	AnalyzerStatus
	Planned bool   `json:"planned"`
	Reason  string `json:"reason"`
}

func (s *Service) discoverInputs(ctx context.Context, work core.Workspace, excludes []string) (discovery.Result, []string, error) {
	excludes = discovery.WorkspaceExcludes(work.RootPath, excludes)
	found, err := discovery.Discover(ctx, work.RootPath, excludes)
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
	for _, status := range s.AnalyzerStatuses(ctx) {
		row := PlannedAnalyzer{AnalyzerStatus: status}
		adapter, registered := s.registry.Get(status.ID)
		switch {
		case !registered:
			row.Reason = "Unavailable in the current runtime"
		case !analyzers.ProfileAllows(profile, status.ID):
			row.Reason = "Excluded by profile"
		case len(filesForLanguages(byLanguage, adapter.SupportedLanguages()...)) == 0 && !(analyzers.TakesDependencyInputs(status.ID) && len(found.DependencyInputs) > 0):
			row.Reason = "No applicable selected inputs"
		default:
			row.Planned = true
			row.Reason = "Ready to run"
			if !status.Ready {
				row.Reason = "Setup required; managed tools may be installed when the scan starts"
			}
		}
		plan.Analyzers = append(plan.Analyzers, row)
	}
	return plan, nil
}

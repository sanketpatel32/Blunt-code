// workspaces handlers.
package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/discovery"
	"bluntcode/internal/reports"
	"bluntcode/internal/scans"
	"bluntcode/internal/workspace"
)

func (s *Server) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()
	paged := strings.TrimSpace(query.Get("page")) != "" || strings.TrimSpace(query.Get("page_size")) != ""

	var items []core.Workspace
	var total int
	var err error
	if paged {
		page, pageSize, filt, err := workspaceListQuery(r)
		if err != nil {
			fail(w, 400, "INVALID_WORKSPACE_QUERY", err.Error())
			return
		}
		if filt.Query != "" {
			s.warmUnscannedLanguages(ctx)
			filt.QueryLanguageIDs = s.cache.languageMatches(filt.Query)
		}
		items, total, err = s.db.WorkspacesPage(ctx, page, pageSize, filt)
		if err != nil {
			if strings.Contains(err.Error(), "out of range") {
				fail(w, 400, "INVALID_WORKSPACE_QUERY", err.Error())
				return
			}
			fail(w, 500, "DATABASE_ERROR", "Could not list workspaces.")
			return
		}
		views, err := s.buildWorkspaceListViews(ctx, items)
		if err != nil {
			fail(w, 500, "DATABASE_ERROR", "Could not load workspace analysis.")
			return
		}
		hasNext := page*pageSize < total
		writeJSON(w, 200, map[string]any{"items": views, "total": total, "page": page, "page_size": pageSize, "has_next": hasNext})
		return
	}

	items, err = s.db.Workspaces(ctx)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not list workspaces.")
		return
	}
	views, err := s.buildWorkspaceListViews(ctx, items)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load workspace analysis.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": views})
}

// buildWorkspaceListViews turns raw workspace rows into dashboard views with
// latest scan, coverage, trends, and tags. Shared by the legacy full list and
// the paged GET /workspaces handler.
func (s *Server) buildWorkspaceListViews(ctx context.Context, items []core.Workspace) ([]workspaceView, error) {
	if len(items) == 0 {
		return []workspaceView{}, nil
	}
	latestByWorkspace, err := s.db.LatestScans(ctx)
	if err != nil {
		return nil, err
	}
	coverageByWorkspace, err := s.db.LatestScanCoverage(ctx)
	if err != nil {
		return nil, err
	}
	var completedByWorkspace map[string]core.Scan
	completedLoaded := false
	type pair struct{ latest, lastCompleted *core.Scan }
	pairs := make([]pair, len(items))
	var scanIDs []string
	for i, item := range items {
		var latest *core.Scan
		if scan, ok := latestByWorkspace[item.ID]; ok {
			latest = &scan
			scanIDs = append(scanIDs, scan.ID)
		}
		var lastCompleted *core.Scan
		if latest == nil || (latest.State != "completed" && latest.State != "completed_with_warnings") {
			if !completedLoaded {
				completedByWorkspace, err = s.db.LatestCompletedScans(ctx)
				if err != nil {
					return nil, err
				}
				completedLoaded = true
			}
			if scan, ok := completedByWorkspace[item.ID]; ok {
				lastCompleted = &scan
				scanIDs = append(scanIDs, scan.ID)
			}
		}
		pairs[i] = pair{latest, lastCompleted}
	}
	runsByScan, err := s.db.AnalyzerRunsForScans(ctx, scanIDs)
	if err != nil {
		return nil, err
	}
	trendByWorkspace, err := s.db.RecentCompletedTotals(ctx, database.DefaultWorkspaceSparkline)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	tagsByWorkspace, err := s.db.WorkspaceTagsMap(ctx, ids)
	if err != nil {
		return nil, err
	}
	roots := make([]string, len(items))
	for i, item := range items {
		roots[i] = item.RootPath
	}
	problems := s.rootProblems(roots)
	views := make([]workspaceView, 0, len(items))
	for i, item := range items {
		view, err := s.buildWorkspaceView(ctx, item, pairs[i].latest, pairs[i].lastCompleted, &viewPrefetch{runs: runsByScan, list: true}, problems[i])
		if err != nil {
			return nil, err
		}
		if coverage, ok := coverageByWorkspace[item.ID]; ok {
			cov := coverage
			view.LatestScanCov = &cov
		}
		view.Trend = trendByWorkspace[item.ID]
		if tags := tagsByWorkspace[item.ID]; len(tags) > 0 {
			view.Tags = tags
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RootPath string `json:"root_path"`
		Name     string `json:"name"`
	}
	if err := decode(r, &input); err != nil {
		fail(w, 400, "INVALID_JSON", invalidJSONMessage)
		return
	}
	root, err := apiWorkspaceRoot(input.RootPath)
	if err != nil {
		failField(w, 400, "INVALID_PATH", err.Error(), "root_path")
		return
	}
	if input.Name == "" {
		input.Name = filepath.Base(root)
	}
	if existing, ok := s.registeredWorkspace(r.Context(), root); ok {
		writeJSON(w, 200, existing)
		return
	}
	created, err := s.db.CreateWorkspace(r.Context(), core.Workspace{Name: input.Name, RootPath: root, GitRepository: workspace.IsGitRepository(root)})
	if err != nil {
		// A concurrent add of the same folder can win the UNIQUE race between
		// the lookup above and the insert; answer with the row it created.
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			if existing, ok := s.registeredWorkspace(r.Context(), root); ok {
				writeJSON(w, 200, existing)
				return
			}
		}
		fail(w, 500, "DATABASE_ERROR", "Could not save workspace.")
		return
	}
	// Warm the language cache so the new card's chips appear on the next
	// list refresh without the list itself walking the folder.
	s.workspaceLanguages(context.WithoutCancel(r.Context()), created, false)
	writeJSON(w, 201, created)
}

// registeredWorkspace finds the workspace already registered for root. The
// exact lookup is exact-byte in SQLite, so case and 8.3 spelling variants of
// a registered Windows root are matched by canonical key, and re-adding a
// folder never duplicates it.
func (s *Server) registeredWorkspace(ctx context.Context, root string) (core.Workspace, bool) {
	if existing, err := s.db.WorkspaceByRoot(ctx, root); err == nil {
		return existing, true
	}
	key := workspace.CanonicalKey(root)
	all, err := s.db.Workspaces(ctx)
	if err != nil {
		return core.Workspace{}, false
	}
	for _, existing := range all {
		if workspace.CanonicalKey(existing.RootPath) == key {
			return existing, true
		}
	}
	return core.Workspace{}, false
}

func (s *Server) getWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	// Go 1.22 routes HEAD to the GET handler; a HEAD probe must stay
	// side-effect free, so only real GETs reorder the workspace list.
	if r.Method != http.MethodHead {
		_ = s.db.TouchWorkspace(r.Context(), workspace.ID)
	}
	latest, lastCompleted, err := s.db.WorkspaceLatestScans(r.Context(), workspace.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load workspace analysis.")
		return
	}
	view, err := s.workspaceView(r.Context(), workspace, latest, lastCompleted)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load workspace analysis.")
		return
	}
	writeJSON(w, 200, view)
}

func (s *Server) workspaceView(ctx context.Context, workspace core.Workspace, latest *core.Scan, lastCompleted *core.Scan) (workspaceView, error) {
	return s.buildWorkspaceView(ctx, workspace, latest, lastCompleted, nil, s.rootProblem(workspace.RootPath))
}

func (s *Server) buildWorkspaceView(ctx context.Context, workspace core.Workspace, latest *core.Scan, lastCompleted *core.Scan, pre *viewPrefetch, rootProblem string) (workspaceView, error) {
	// A probe timeout is not proof the folder is gone — a stalled share must
	// not be reported as missing (or block the scan button), only labelled.
	view := workspaceView{Workspace: workspace, RootMissing: rootProblem != "" && rootProblem != "probe_timeout", RootProblem: rootProblem}
	attach := func(scan *core.Scan) (*scanDetail, error) {
		if scan == nil {
			return nil, nil
		}
		if scan.Snapshot != nil {
			view.Languages = languageNames(scan.Snapshot.Languages)
		}
		// The card pairs the scan with its analyzer runs and its new/fixed
		// comparison (the same attach and comparison logic GET /scans/{id}
		// serves), so both are loaded while the immutable snapshot is still
		// at hand; the snapshot itself is then dropped because the dashboard
		// does not need every selected path from it.
		var runs []reports.Run
		if pre != nil && pre.runs != nil {
			runs = pre.runs[scan.ID]
		} else {
			var err error
			if runs, err = s.db.AnalyzerRuns(ctx, scan.ID); err != nil {
				return nil, err
			}
		}
		newCount, fixedCount, err := s.cachedComparisonCounts(ctx, *scan)
		if err != nil {
			return nil, err
		}
		scan.Snapshot = nil
		return &scanDetail{Scan: *scan, AnalyzerRuns: analyzerRuns(runs), NewCount: newCount, FixedCount: fixedCount}, nil
	}
	detail, err := attach(latest)
	if err != nil {
		return workspaceView{}, err
	}
	view.LatestScan = detail
	// When the latest run never finished, serve the last one that did so
	// boards keep grading on real data (nil when none exists yet).
	if latest == nil || (latest.State != "completed" && latest.State != "completed_with_warnings") {
		if view.LastCompletedScan, err = attach(lastCompleted); err != nil {
			return workspaceView{}, err
		}
	}
	assessment := latest
	if assessment == nil || (assessment.State != "completed" && assessment.State != "completed_with_warnings") {
		assessment = lastCompleted
	}
	if assessment != nil {
		coverage, err := s.db.ScanAnalyzerCoverage(ctx, assessment.ID)
		if err != nil {
			return workspaceView{}, err
		}
		view.AssessmentCoverage = &coverage
	}
	// A missing root has nothing to walk; the snapshot languages above still
	// describe what the folder held when it was last scanned.
	if len(view.Languages) == 0 && !view.RootMissing {
		view.Languages = s.workspaceLanguages(ctx, workspace, pre == nil || !pre.list)
	}
	return view, nil
}

// scanComparisonCounts computes the new/fixed pair for one scan exactly the
// way the report model does (server.go reportModel): suppressed fingerprints
// are filtered from both sides, and scans.Compare's coverage rule counts a
// previous finding as fixed only when its analyzer succeeded in the current
// scan. Without a previous completed scan the pair is 0/0 — the same
// "no comparison" outcome the report shows.
func (s *Server) scanComparisonCounts(ctx context.Context, scan core.Scan) (int, int, error) {
	findings, err := s.db.FindingIdentities(ctx, scan.ID)
	if err != nil {
		return 0, 0, err
	}
	suppressed, err := s.db.SuppressedFingerprints(ctx, scan.WorkspaceID)
	if err != nil {
		return 0, 0, err
	}
	findings = scans.FilterSuppressed(findings, suppressed)
	previousID, err := s.db.PreviousCompletedScanID(ctx, scan.WorkspaceID, scan.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	previous, err := s.db.FindingIdentities(ctx, previousID)
	if err != nil {
		return 0, 0, err
	}
	succeeded, err := s.db.SuccessfulAnalyzerIDs(ctx, scan.ID)
	if err != nil {
		return 0, 0, err
	}
	var selectedPaths []string
	if scan.Snapshot != nil {
		selectedPaths = scan.Snapshot.SelectedFiles
	}
	comparison := scans.Compare(findings, scans.FilterSuppressed(previous, suppressed), scans.NewComparisonCoverage(succeeded, selectedPaths))
	return len(comparison.New), len(comparison.Fixed), nil
}

func languageNames(counts map[string]int) []string {
	result := make([]string, 0, len(counts))
	for language, count := range counts {
		if count > 0 {
			result = append(result, language)
		}
	}
	sort.Strings(result)
	return result
}

func (s *Server) updateWorkspace(w http.ResponseWriter, r *http.Request) {
	current, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	var input struct {
		Name           *string `json:"name"`
		DefaultProfile *string `json:"default_profile"`
		// TrustRepoAnalyzerConfig is a trust decision, not a preference:
		// true lets the scanned repository's own analyzer config files
		// disable rules or exclude paths from its audit.
		TrustRepoAnalyzerConfig *bool `json:"trust_repo_analyzer_config"`
	}
	if err := decode(r, &input); err != nil {
		fail(w, 400, "INVALID_JSON", "Invalid workspace update.")
		return
	}
	if input.TrustRepoAnalyzerConfig != nil {
		current.TrustRepoAnalyzerConfig = *input.TrustRepoAnalyzerConfig
	}
	if input.Name != nil {
		current.Name = strings.TrimSpace(*input.Name)
	}
	if input.DefaultProfile != nil {
		// The stored default feeds the workspaces list's one-click Scan button
		// and POST /workspaces/{id}/scans already validates the profile, so a
		// bogus value must die at this boundary instead of persisting and
		// failing every later default scan. Empty resets to the same default
		// CreateWorkspace applies.
		profile := *input.DefaultProfile
		if profile == "" {
			profile = "standard"
		}
		if !oneOf(profile, "quick", "standard", "deep", "pentest") {
			fail(w, 400, "INVALID_PROFILE", "default_profile must be quick, standard, deep, or pentest.")
			return
		}
		current.DefaultProfile = profile
	}
	if current.Name == "" {
		fail(w, 400, "INVALID_WORKSPACE", "Name is required.")
		return
	}
	updated, err := s.db.UpdateWorkspace(r.Context(), current)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not update workspace.")
		return
	}
	writeJSON(w, 200, updated)
}

func (s *Server) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	// Deleting mid-scan would cascade the scan row out from under a running
	// engine: its writes would land against deleted rows and the scan page
	// would poll a vanished scan. Same contract as DELETE /scans/{id} — cancel
	// the scan first. The per-workspace lock serializes this check-and-act
	// against startScan on the same workspace, closing the race between the
	// active-scan read and the delete.
	unlock := s.lockWorkspace(workspace.ID)
	defer unlock()
	active, err := s.db.ActiveScanID(r.Context(), workspace.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not check active scans.")
		return
	}
	if active != "" {
		fail(w, 409, "SCAN_IN_PROGRESS", "A scan is still running on this workspace. Cancel it before removing the workspace.")
		return
	}
	if err := s.db.DeleteWorkspace(r.Context(), workspace.ID); err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not delete workspace.")
		return
	}
	s.cache.invalidate(workspace.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) userExcludes(ctx context.Context, id string) ([]string, error) {
	rules, err := s.db.Rules(ctx, id)
	if err != nil {
		return nil, err
	}
	var patterns []string
	for _, rule := range rules {
		if rule.RuleType == "exclude" && rule.Enabled {
			patterns = append(patterns, rule.Pattern)
		}
	}
	return patterns, nil
}

func (s *Server) discoverWorkspace(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	if s.failRootUnavailable(w, work) {
		return
	}
	patterns, err := s.userExcludes(r.Context(), work.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load rules.")
		return
	}
	result, err := discovery.Discover(r.Context(), work.RootPath, patterns)
	if err != nil {
		fail(w, 500, "DISCOVERY_FAILED", "Could not discover workspace files.")
		return
	}
	// Re-detect version control here too: a folder can gain or lose its .git
	// entry after being added, and the workspace's git_repository flag is the
	// one place that fact lives. A failed stamp degrades to a stale flag —
	// warn rather than block discovery over metadata.
	if err := s.db.SetWorkspaceGitRepository(r.Context(), work.ID, workspace.IsGitRepository(work.RootPath)); err != nil {
		s.log.Warn("discover: could not refresh git_repository", "workspace", work.ID, "error", err)
	}
	writeJSON(w, 200, result)
}

// workspaceTrends serves the severity-over-time series for the workspace
// detail page's trend chart: one point per completed scan, oldest first,
// carrying the severity counts persisted at completion. Only the producing
// terminal states count as completed (the same rule as the dashboard summary),
// so failed, cancelled, and interrupted scans never chart.
func (s *Server) workspaceTrends(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	limit := database.DefaultSeverityTrendLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > database.MaxSeverityTrendLimit {
			fail(w, 400, "INVALID_TREND_QUERY", fmt.Sprintf("limit must be a positive integer of at most %d", database.MaxSeverityTrendLimit))
			return
		}
		limit = value
	}
	points, err := s.db.SeverityTrend(r.Context(), work.ID, limit)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load severity trends.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": trendViews(points)})
}

// riskGrade buckets a raw score into the A-D letter the UI renders.
func riskGrade(score float64) string {
	switch {
	case score < 5:
		return "A"
	case score < 20:
		return "B"
	case score < 50:
		return "C"
	default:
		return "D"
	}
}

func trendViews(points []database.SeverityTrendPoint) []trendView {
	out := make([]trendView, len(points))
	for i, point := range points {
		score := riskScoreOf(database.RiskCounts{Critical: point.Severity.Critical, High: point.Severity.High, Medium: point.Severity.Medium, Low: point.Severity.Low, Info: point.Severity.Info})
		out[i] = trendView{SeverityTrendPoint: point, Score: score, Grade: riskGrade(score)}
	}
	return out
}

func riskScoreOf(c database.RiskCounts) float64 {
	score := 0.0
	score += float64(c.Critical) * riskSeverityWeights["critical"]
	score += float64(c.High) * riskSeverityWeights["high"]
	score += float64(c.Medium) * riskSeverityWeights["medium"]
	score += float64(c.Low) * riskSeverityWeights["low"]
	return score
}

// workspaceRisk serves the workspace's weighted risk score computed from its
// latest completed scan, plus the delta against the previous completed scan
// so the UI can show whether risk is heading up or down.
func (s *Server) workspaceRisk(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	snaps, err := s.db.RecentCompletedSeverityCounts(r.Context(), work.ID, 2)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load risk data.")
		return
	}
	if len(snaps) == 0 {
		writeJSON(w, 200, map[string]any{"available": false})
		return
	}
	latest := snaps[0]
	score := riskScoreOf(latest)
	response := map[string]any{
		"available": true,
		"scan_id":   latest.ScanID,
		"score":     score,
		"grade":     riskGrade(score),
		"counts":    map[string]int{"critical": latest.Critical, "high": latest.High, "medium": latest.Medium, "low": latest.Low, "info": latest.Info},
		"weights":   riskSeverityWeights,
	}
	// The grade is only as good as the scan behind it: pair it with the
	// scan's state, when it finished, and how much of its analyzer set
	// completed (IMP-13). A partial scan's low score must not read as the
	// assurance of a complete one.
	if scan, err := s.db.Scan(r.Context(), latest.ScanID); err == nil {
		response["scan_state"] = scan.State
		if scan.FinishedAt != nil && !scan.FinishedAt.IsZero() {
			response["finished_at"] = scan.FinishedAt.UTC().Format(time.RFC3339)
		}
	}
	if coverage, err := s.db.ScanAnalyzerCoverage(r.Context(), latest.ScanID); err == nil && coverage.Total > 0 {
		response["coverage"] = map[string]int{
			"total": coverage.Total, "succeeded": coverage.Succeeded, "failed": coverage.Failed, "warned": coverage.Warned,
		}
		response["complete"] = coverage.Complete()
	}
	if len(snaps) > 1 {
		previous := riskScoreOf(snaps[1])
		trend := "flat"
		switch {
		case score > previous:
			trend = "up"
		case score < previous:
			trend = "down"
		}
		response["previous_score"] = previous
		response["previous_scan_id"] = snaps[1].ScanID
		response["trend"] = trend
	} else {
		response["trend"] = "flat"
	}
	writeJSON(w, 200, response)
}

// pruneWorkspaceScans deletes every terminal scan of the workspace beyond the
// newest keep (query parameter, 1..100). Running scans are never touched. It
// backs the history-retention control so old reports stop growing forever.
func (s *Server) pruneWorkspaceScans(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("keep"))
	keep, err := strconv.Atoi(raw)
	if err != nil || keep < 1 || keep > 100 {
		fail(w, 400, "INVALID_PRUNE_REQUEST", "keep must be an integer between 1 and 100.")
		return
	}
	deleted, orphanedReports, err := s.db.PruneOldScans(r.Context(), work.ID, keep)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not prune scans.")
		return
	}
	s.cache.invalidate(work.ID)
	response := map[string]any{"deleted": deleted, "kept": keep}
	if err := reports.RemoveFiles(s.paths.ReportsDir, orphanedReports); err != nil {
		s.log.Warn("prune: could not remove report files", "workspace", work.ID, "error", err)
		response["warning"] = "Scans were deleted, but some report files could not be removed."
	}
	writeJSON(w, 200, response)
}

// getWorkspaceTags lists the workspace's tags sorted alphabetically.
func (s *Server) getWorkspaceTags(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	tags, err := s.db.GetWorkspaceTags(r.Context(), work.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load tags.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": tags})
}

// putWorkspaceTags atomically replaces the workspace's tag set. Tags are
// lowercase freeform labels (see database.ValidateTag); an empty set clears
// every tag.
func (s *Server) putWorkspaceTags(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	var input struct {
		Tags []string `json:"tags"`
	}
	if err := decode(r, &input); err != nil {
		fail(w, 400, "INVALID_JSON", "tags must be a JSON array of strings.")
		return
	}
	for _, tag := range input.Tags {
		if err := database.ValidateTag(strings.ToLower(strings.TrimSpace(tag))); err != nil {
			fail(w, 400, "INVALID_TAG", err.Error())
			return
		}
	}
	if err := s.db.SetWorkspaceTags(r.Context(), work.ID, input.Tags); err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not save tags.")
		return
	}
	tags, err := s.db.GetWorkspaceTags(r.Context(), work.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load tags.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": tags})
}

// scan handlers handlers.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bluntcode/internal/database"
	"bluntcode/internal/reports"
	"bluntcode/internal/scans"
)

// listScans serves the workspace's scan history as one server-paged slice.
// The envelope matches the findings list's page contract —
// {items,total,page,page_size,has_next} — with page 1-based and page_size
// capped at MaxScanPageSize, which is what the web client already expects.
func (s *Server) listScans(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	page, pageSize, err := scanPageFilter(r)
	if err != nil {
		fail(w, 400, "INVALID_SCAN_QUERY", err.Error())
		return
	}
	dateFrom := strings.TrimSpace(r.URL.Query().Get("date_from"))
	dateTo := strings.TrimSpace(r.URL.Query().Get("date_to"))
	filter := database.ScansPageFilter{DateFrom: dateFrom, DateTo: dateTo}
	if err := database.ValidateScansPageFilter(filter); err != nil {
		fail(w, 400, "INVALID_SCAN_QUERY", err.Error())
		return
	}
	items, total, err := s.db.ScansPage(r.Context(), work.ID, page, pageSize, filter)
	if err != nil {
		if strings.Contains(err.Error(), "out of range") {
			fail(w, 400, "INVALID_SCAN_QUERY", err.Error())
			return
		}
		fail(w, 500, "DATABASE_ERROR", "Could not list scans.")
		return
	}
	payload := map[string]any{"items": items, "total": total, "page": page, "page_size": pageSize, "has_next": (page-1)*pageSize+len(items) < total}
	if dateFrom != "" {
		payload["date_from"] = dateFrom
	}
	if dateTo != "" {
		payload["date_to"] = dateTo
	}
	if dateFrom != "" || dateTo != "" {
		unfiltered, err := s.db.CountScansForWorkspace(r.Context(), work.ID)
		if err != nil {
			fail(w, 500, "DATABASE_ERROR", "Could not list scans.")
			return
		}
		payload["total_unfiltered"] = unfiltered
	}
	writeJSON(w, 200, payload)
}

// scanPageFilter parses the scan history's paging controls the way
// findingFilter does: page is 1-based and defaults to 1, page_size defaults
// to DefaultScanPageSize and is rejected outside 1..MaxScanPageSize.
func scanPageFilter(r *http.Request) (int, int, error) {
	query := r.URL.Query()
	page := 1
	if raw := strings.TrimSpace(query.Get("page")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return 0, 0, fmt.Errorf("page must be a positive integer")
		}
		page = value
	}
	pageSize := database.DefaultScanPageSize
	if raw := strings.TrimSpace(query.Get("page_size")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > database.MaxScanPageSize {
			return 0, 0, fmt.Errorf("page_size must be between 1 and %d", database.MaxScanPageSize)
		}
		pageSize = value
	}
	return page, pageSize, nil
}

// recentScans serves the dashboard's cross-workspace activity feed: the most
// recent scans across all workspaces plus a global summary object. The summary
// is folded into this response instead of living at /api/v1/scans/summary so
// the dashboard loads its feed and aggregates in a single request; it stays
// global regardless of the list filters.
func (s *Server) recentScans(w http.ResponseWriter, r *http.Request) {
	filter, err := recentScanFilter(r)
	if err != nil {
		fail(w, 400, "INVALID_SCAN_QUERY", err.Error())
		return
	}
	items, total, err := s.db.RecentScans(r.Context(), filter)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not list scans.")
		return
	}
	summary, err := s.db.ScanSummary(r.Context())
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not summarize scans.")
		return
	}
	hasMore := filter.Offset+len(items) < total
	writeJSON(w, 200, map[string]any{"scans": items, "total": total, "limit": filter.Limit, "offset": filter.Offset, "has_more": hasMore, "summary": summary})
}

// recentScanFilter parses the global scan list controls: limit defaults to 10,
// is rejected below 1, and is clamped to 50; optional offset pages through the
// filtered result set; state optionally filters by a single lifecycle state.
func recentScanFilter(r *http.Request) (database.RecentScansFilter, error) {
	query := r.URL.Query()
	filter := database.RecentScansFilter{Limit: database.DefaultRecentScansLimit, State: strings.ToLower(strings.TrimSpace(query.Get("state"))), WorkspaceID: strings.TrimSpace(query.Get("workspace_id"))}
	if filter.State != "" && !knownScanState(filter.State) {
		return filter, fmt.Errorf("state must be a known scan state such as completed or running")
	}
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return filter, fmt.Errorf("limit must be a positive integer of at most %d", database.MaxRecentScansLimit)
		}
		if value > database.MaxRecentScansLimit {
			value = database.MaxRecentScansLimit
		}
		filter.Limit = value
	}
	if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return filter, fmt.Errorf("offset must be a non-negative integer")
		}
		filter.Offset = value
	}
	return filter, nil
}

// knownScanState reports whether state belongs to the scan lifecycle: the
// progression states a scan moves through plus the terminal states covered by
// terminalScanState.
func knownScanState(state string) bool {
	switch state {
	case "queued", "preparing", "installing_tools", "discovering", "running", "normalizing", "generating_report",
		"completed", "completed_with_warnings", "failed", "cancelled", "interrupted":
		return true
	}
	return false
}

func (s *Server) startScan(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	var input struct {
		Profile     string `json:"profile"`
		TargetURL   string `json:"target_url"`
		Jobs        int    `json:"jobs"`
		Incremental bool   `json:"incremental"`
	}
	if err := decode(r, &input); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		fail(w, 400, "INVALID_JSON", "Invalid scan request.")
		return
	}
	if input.Profile != "" && input.Profile != "quick" && input.Profile != "standard" && input.Profile != "deep" && input.Profile != "pentest" {
		fail(w, 400, "INVALID_PROFILE", "profile must be quick, standard, deep, or pentest.")
		return
	}
	targetURL, err := scans.NormalizeScanTarget(input.Profile, strings.TrimSpace(input.TargetURL))
	if err != nil {
		fail(w, 400, "INVALID_TARGET", err.Error())
		return
	}
	if input.Jobs < 0 || input.Jobs > 32 {
		fail(w, 400, "INVALID_JOBS", "jobs must be between 0 (default concurrency) and 32.")
		return
	}
	// Bounded EXISTS probe instead of loading the whole history (and every
	// row's snapshot_json) for a boolean; the workspace lock makes the
	// check-and-act atomic so two POSTs can't both see "no active scan".
	unlock := s.lockWorkspace(work.ID)
	defer unlock()
	active, err := s.db.ActiveScanID(r.Context(), work.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not check active scan.")
		return
	}
	if active != "" {
		writeJSON(w, 409, map[string]any{"error": map[string]any{"code": "SCAN_ALREADY_ACTIVE", "message": "A scan is already active.", "details": map[string]string{"scan_id": active}}})
		return
	}
	if s.failRootUnavailable(w, work) {
		return
	}
	if s.scans == nil {
		fail(w, 503, "SCAN_ENGINE_UNAVAILABLE", "The scan engine is unavailable.")
		return
	}
	patterns, err := s.userExcludes(r.Context(), work.ID)
	if err != nil {
		// A failed rules read must not silently run the scan unfiltered.
		fail(w, 500, "DATABASE_ERROR", "Could not load exclusion rules.")
		return
	}
	scan, err := s.scans.DiscoverAndStartWithOptions(r.Context(), work, input.Profile, patterns, scans.ScanOptions{Jobs: input.Jobs, Incremental: input.Incremental, TargetURL: targetURL})
	if err != nil {
		switch {
		case errors.Is(err, scans.ErrTooManyScans):
			fail(w, 503, "SCAN_CAPACITY", "Too many scans are already running. Wait for one to finish.")
		case errors.Is(err, scans.ErrWorkspaceRootMissing):
			fail(w, http.StatusUnprocessableEntity, "WORKSPACE_ROOT_MISSING", err.Error())
		default:
			fail(w, 500, "DATABASE_ERROR", "Could not create scan.")
		}
		return
	}
	writeJSON(w, 202, scan)
}

func terminalScanState(state string) bool {
	return state == "completed" || state == "completed_with_warnings" || state == "failed" || state == "cancelled" || state == "interrupted"
}

func (s *Server) getScan(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if !validID(id) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	scan, err := s.db.Scan(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load scan.")
		return
	}
	runs, err := s.db.AnalyzerRuns(r.Context(), id)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load analyzer progress.")
		return
	}
	newCount, fixedCount, err := s.scanComparisonCounts(r.Context(), scan)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load scan comparison.")
		return
	}
	writeJSON(w, 200, scanDetail{Scan: scan, AnalyzerRuns: analyzerRuns(runs), NewCount: newCount, FixedCount: fixedCount})
}

func analyzerRuns(runs []reports.Run) []map[string]any {
	items := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		item := map[string]any{
			"analyzer_id": run.AnalyzerID,
			"status":      run.State,
			"version":     run.Version,
			"message":     run.ErrorSummary,
			"duration_ms": run.Duration.Milliseconds(),
		}
		if run.WarningCount > 0 {
			// Degraded coverage (e.g. an unparseable output batch): surfaced so
			// clients can explain why a succeeded run may under-report.
			item["warning_count"] = run.WarningCount
		}
		items = append(items, item)
	}
	return items
}

// deleteScan removes one terminal scan and everything stored under it
// (findings, metrics, per-file hashes, analyzer runs). Active scans must be
// cancelled first; they are rejected with 409 so a delete can never race the
// analyzer pipeline mid-write.
func (s *Server) deleteScan(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if !validID(id) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	scan, err := s.db.Scan(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load scan.")
		return
	}
	if !terminalScanState(scan.State) {
		writeJSON(w, 409, map[string]any{"error": map[string]string{"code": "SCAN_NOT_TERMINAL", "message": "Cancel this scan before deleting it."}})
		return
	}
	deleted, orphanedReport, err := s.db.DeleteScan(r.Context(), id)
	if err != nil || !deleted {
		fail(w, 500, "DATABASE_ERROR", "Could not delete scan.")
		return
	}
	s.cache.invalidate(scan.WorkspaceID)
	if err := reports.RemoveFiles(s.paths.ReportsDir, []string{orphanedReport}); err != nil {
		s.log.Warn("delete scan: could not remove report file", "scan", id, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) cancelScan(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if !validID(id) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	scan, err := s.db.Scan(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	if terminalScanState(scan.State) {
		writeJSON(w, 409, map[string]any{"error": map[string]string{"code": "SCAN_NOT_ACTIVE", "message": "This scan has already finished."}})
		return
	}
	if s.scans == nil {
		fail(w, 503, "SCAN_ENGINE_UNAVAILABLE", "The scan engine is unavailable.")
		return
	}
	if err := s.scans.Cancel(r.Context(), id); err != nil {
		fail(w, 409, "SCAN_NOT_ACTIVE", "This scan has already finished.")
		return
	}
	writeJSON(w, 202, map[string]string{"state": "cancelling"})
}

// fixedFindings lists what the fixed count on the scan report actually refers
// to: findings present in the previous completed scan and gone from this one,
// with the same coverage-aware rule the report comparison applies. The list is
// capped for the panel while total_fixed always reports the exact count.
// scanCompare diffs one scan against another (or its previous completed
// scan when `with` is omitted) using the same coverage-aware Compare the
// report model uses: a previous finding counts as fixed only if the analyzer
// that produced it succeeded in the current scan, and suppressed fingerprints
// are filtered from both sides before diffing.
func (s *Server) scanCompare(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if !validID(id) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	scan, err := s.db.Scan(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load scan.")
		return
	}
	otherID := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("with")))
	if otherID == "" {
		resolved, resolveErr := s.db.PreviousCompletedScanID(r.Context(), scan.WorkspaceID, scan.ID)
		if errors.Is(resolveErr, sql.ErrNoRows) {
			writeJSON(w, 200, map[string]any{"available": false, "reason": "no previous completed scan"})
			return
		}
		if resolveErr != nil {
			fail(w, 500, "DATABASE_ERROR", "Could not resolve the comparison scan.")
			return
		}
		otherID = resolved
	} else if !validID(otherID) {
		fail(w, 400, "INVALID_SCAN_ID", "with must be a valid scan ID.")
		return
	}
	other, err := s.db.Scan(r.Context(), otherID)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "SCAN_NOT_FOUND", "Comparison scan was not found.")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load the comparison scan.")
		return
	}
	if other.WorkspaceID != scan.WorkspaceID {
		fail(w, 400, "WORKSPACE_MISMATCH", "Both scans must belong to the same workspace.")
		return
	}
	currentFindings, err := s.db.Findings(r.Context(), scan.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load findings.")
		return
	}
	previousFindings, err := s.db.Findings(r.Context(), other.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load the comparison findings.")
		return
	}
	suppressed, err := s.db.SuppressedFingerprints(r.Context(), scan.WorkspaceID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load suppressions.")
		return
	}
	succeeded, err := s.db.SuccessfulAnalyzerIDs(r.Context(), scan.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load analyzer coverage.")
		return
	}
	currentSelected := []string(nil)
	if scan.Snapshot != nil {
		currentSelected = scan.Snapshot.SelectedFiles
	}
	cmp := scans.Compare(scans.FilterSuppressed(currentFindings, suppressed), scans.FilterSuppressed(previousFindings, suppressed), scans.NewComparisonCoverage(succeeded, currentSelected))
	writeJSON(w, 200, map[string]any{
		"available":        true,
		"current_scan_id":  scan.ID,
		"previous_scan_id": other.ID,
		"new":              cmp.New,
		"fixed":            cmp.Fixed,
		"persistent":       cmp.Persistent,
		"not_evaluated":    cmp.NotEvaluatedAnalyzerIDs,
		"summary":          map[string]int{"new": len(cmp.New), "fixed": len(cmp.Fixed), "persistent": len(cmp.Persistent)},
	})
}

func (s *Server) fixedFindings(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if !validID(id) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	scan, err := s.db.Scan(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load scan.")
		return
	}
	limit := database.DefaultFixedFindingsLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > database.MaxFixedFindingsLimit {
			fail(w, 400, "INVALID_FINDING_QUERY", fmt.Sprintf("limit must be a positive integer of at most %d", database.MaxFixedFindingsLimit))
			return
		}
		limit = value
	}
	// The fixed panel must agree with the verdict's fixed count (computed via
	// scanComparisonCounts → scans.Compare), so the same file-coverage half of
	// the rule — only files this scan actually evaluated can report fixes —
	// rides along here.
	var selectedPaths []string
	if scan.Snapshot != nil {
		selectedPaths = scan.Snapshot.SelectedFiles
	}
	result, err := s.db.FixedFindings(r.Context(), scan, limit, selectedPaths)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load fixed findings.")
		return
	}
	var previousID any
	if result.PreviousScanID != "" {
		previousID = result.PreviousScanID
	}
	writeJSON(w, 200, map[string]any{"fixed": result.Items, "total_fixed": result.Total, "comparison_available": result.ComparisonAvailable, "previous_scan_id": previousID})
}

func (s *Server) scanEvents(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if !validID(id) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	if _, err := s.db.Scan(r.Context(), id); err != nil {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, "SSE_UNAVAILABLE", "Streaming is unavailable.")
		return
	}
	ch, unsubscribe := s.bus.Subscribe(id)
	defer unsubscribe()
	fmt.Fprint(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()
	// Heartbeat comments keep idle streams alive: some clients and intermediaries
	// drop silent connections, and the ping doubles as a liveness signal between
	// scan events. A failed write ends the stream via the request context below.
	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case event := <-ch:
			body, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, body)
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// scanPlan uses the same discovery and routing as a full scan, without creating a job.
func (s *Server) scanPlan(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	profile := r.URL.Query().Get("profile")
	if profile == "" {
		profile = work.DefaultProfile
	}
	if profile == "" {
		profile = "standard"
	}
	switch profile {
	case "quick", "standard", "deep", "pentest":
	default:
		fail(w, 400, "INVALID_PROFILE", "Choose quick, standard, deep, or pentest.")
		return
	}
	if s.failRootUnavailable(w, work) {
		return
	}
	if s.scans == nil {
		fail(w, 503, "SCAN_UNAVAILABLE", "Scan service is unavailable.")
		return
	}
	patterns, err := s.userExcludes(r.Context(), work.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load exclusions.")
		return
	}
	plan, err := s.scans.Plan(r.Context(), work, profile, patterns)
	if err != nil {
		if errors.Is(err, scans.ErrWorkspaceRootMissing) {
			fail(w, http.StatusUnprocessableEntity, "WORKSPACE_ROOT_MISSING", err.Error())
			return
		}
		fail(w, 500, "DISCOVERY_FAILED", "Could not review workspace inputs.")
		return
	}
	writeJSON(w, 200, plan)
}

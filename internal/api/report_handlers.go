package api

import (
	"bluntcode/internal/core"
	"bluntcode/internal/reports"
	"bluntcode/internal/scans"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

func (s *Server) reportModel(ctx context.Context, scan core.Scan, work core.Workspace) (reports.Model, error) {
	findings, err := s.db.Findings(ctx, scan.ID)
	if err != nil {
		return reports.Model{}, err
	}
	// Suppressed findings never enter the model, so every renderer built from
	// it (JSON, SARIF, HTML, Markdown regeneration) excludes them. Filtering
	// both sides of the comparison also keeps a dismissed fingerprint from
	// being reported as fixed.
	suppressed, err := s.db.SuppressedFingerprints(ctx, work.ID)
	if err != nil {
		return reports.Model{}, err
	}
	findings = scans.FilterSuppressed(findings, suppressed)
	metrics, err := s.db.Metrics(ctx, scan.ID)
	if err != nil {
		return reports.Model{}, err
	}
	runs, err := s.db.AnalyzerRuns(ctx, scan.ID)
	if err != nil {
		return reports.Model{}, err
	}
	comparison := scans.Comparison{}
	var selectedPaths []string
	if scan.Snapshot != nil {
		selectedPaths = scan.Snapshot.SelectedFiles
	}
	if previousID, previousErr := s.db.PreviousCompletedScanID(ctx, scan.WorkspaceID, scan.ID); previousErr == nil {
		// The comparison only reads fingerprint + analyzer + path of the
		// previous scan's findings; loading full rows doubled the decode
		// (metadata_json unmarshal and ORDER BY included) for fields the
		// renderers never touch.
		previous, previousFindingsErr := s.db.FindingIdentities(ctx, previousID)
		succeeded, succeededErr := s.db.SuccessfulAnalyzerIDs(ctx, scan.ID)
		if previousFindingsErr == nil && succeededErr == nil {
			comparison = scans.Compare(findings, scans.FilterSuppressed(previous, suppressed), scans.NewComparisonCoverage(succeeded, selectedPaths))
		}
	}
	startedAt := scan.StartedAt
	if startedAt == nil && scan.Snapshot != nil {
		startedAt = &scan.Snapshot.CapturedAt
	}
	files := []string(nil)
	bluntCodeVersion := ""
	var provenance *reports.Provenance
	if scan.Snapshot != nil {
		files = append(files, scan.Snapshot.SelectedFiles...)
		bluntCodeVersion = scan.Snapshot.BluntCodeVersion
		provenance = &reports.Provenance{
			InputDigest:            scan.Snapshot.InputDigest,
			ConfigDigest:           scan.Snapshot.ConfigDigest,
			GitCommit:              scan.Snapshot.Git.Commit,
			GitDirty:               scan.Snapshot.Git.Dirty,
			DiscoveryPolicyVersion: scan.Snapshot.DiscoveryPolicyVersion,
			FingerprintVersion:     scan.Snapshot.FingerprintVersion,
			Platform:               scan.Snapshot.Platform,
			DriftDetected:          scan.Snapshot.DriftDetected,
			Incremental:            scan.Snapshot.Incremental,
		}
	}
	if len(files) == 0 && scan.SelectedFileCount > 0 {
		files = make([]string, scan.SelectedFileCount)
	}
	skippedCount := scan.CandidateFileCount - scan.SelectedFileCount
	if skippedCount < 0 {
		skippedCount = 0
	}
	// The findings list and CSV derive a per-row comparison status (new /
	// persistent) from the previous completed scan; the report model needs the
	// same stamp so UI status chips can count from the report payload instead
	// of every row arriving without a status. Compare() classifies with the
	// identical fingerprint-membership rule the SQL CASE uses.
	previousByFingerprint := make(map[string]bool, len(comparison.Persistent))
	for _, finding := range comparison.Persistent {
		previousByFingerprint[finding.Fingerprint] = true
	}
	for i := range findings {
		if previousByFingerprint[findings[i].Fingerprint] {
			findings[i].Status = "persistent"
		} else {
			findings[i].Status = "new"
		}
	}
	return reports.Build(reports.Input{
		WorkspaceName: work.Name, WorkspacePath: work.RootPath, ScanID: scan.ID, Profile: scan.Profile, State: scan.State, BluntCodeVersion: bluntCodeVersion,
		StartedAt: startedAtValue(startedAt), FinishedAt: finishedAtValue(scan.FinishedAt), Files: files, SkippedFiles: make([]string, skippedCount), Findings: findings, Metrics: metrics, Runs: runs,
		Comparison: reports.Comparison{New: comparison.New, Fixed: comparison.Fixed, Persistent: comparison.Persistent, NotEvaluatedAnalyzerIDs: comparison.NotEvaluatedAnalyzerIDs},
		Provenance: provenance,
	}), nil
}

func startedAtValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func finishedAtValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
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
		fail(w, 500, "DATABASE_ERROR", "Could not load report.")
		return
	}
	work, err := s.db.Workspace(r.Context(), scan.WorkspaceID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load report.")
		return
	}
	model, err := s.reportModel(r.Context(), scan, work)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load report.")
		return
	}
	writeJSON(w, 200, map[string]any{
		"scan":      scan,
		"workspace": work,
		"findings":  model.Findings,
		"metrics":   model.Metrics,
		"warnings":  model.Warnings,
		"comparison": map[string]int{
			"new_count":        len(model.Comparison.New),
			"fixed_count":      len(model.Comparison.Fixed),
			"persistent_count": len(model.Comparison.Persistent),
		},
	})
}

func (s *Server) reportMarkdown(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if !validID(id) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	path, err := s.db.ReportPath(r.Context(), id)
	if err != nil || path == "" {
		fail(w, 404, "REPORT_NOT_FOUND", "Markdown report is not available.")
		return
	}
	if _, err := os.Stat(path); err != nil {
		fail(w, 404, "REPORT_NOT_FOUND", "Markdown report is not available.")
		return
	}
	scan, err := s.db.Scan(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load report.")
		return
	}
	work, err := s.db.Workspace(r.Context(), scan.WorkspaceID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load report.")
		return
	}
	model, err := s.reportModel(r.Context(), scan, work)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load report.")
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	reportDisposition(w, r, "blunt-code-report.md", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(reports.Markdown(model)))
}

// reportSARIF exports the scan as a SARIF 2.1.0 attachment, the interchange
// format understood by VS Code, GitHub code scanning, and other standard
// tooling. Like the JSON report it is regenerated from stored scan data, not
// from a persisted artifact.
func (s *Server) reportSARIF(w http.ResponseWriter, r *http.Request) {
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
		fail(w, 500, "DATABASE_ERROR", "Could not load report.")
		return
	}
	work, err := s.db.Workspace(r.Context(), scan.WorkspaceID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load report.")
		return
	}
	model, err := s.reportModel(r.Context(), scan, work)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load report.")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	reportDisposition(w, r, fmt.Sprintf("bluntcode-scan-%s.sarif", shortID(scan.ID)), "")
	_ = json.NewEncoder(w).Encode(reports.SARIF(model))
}

// reportHTML exports the scan as a standalone HTML document: one
// self-contained file with no external assets or scripts that can be shared,
// archived, or printed as-is. Like the other exporters it is regenerated from
// stored scan data, not from a persisted artifact.
func (s *Server) reportHTML(w http.ResponseWriter, r *http.Request) {
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
		fail(w, 500, "DATABASE_ERROR", "Could not load report.")
		return
	}
	work, err := s.db.Workspace(r.Context(), scan.WorkspaceID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load report.")
		return
	}
	model, err := s.reportModel(r.Context(), scan, work)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load report.")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	reportDisposition(w, r, fmt.Sprintf("bluntcode-scan-%s.html", shortID(scan.ID)), "")
	_, _ = w.Write(reports.HTML(model))
}

// reportDisposition sends a report as a download, or, with ?inline=1, for
// viewing in a browser tab ("Open in new tab"). Inline responses are
// sandboxed: the document gets an opaque origin, so its own script (the HTML
// report's filter) runs but cannot reach the local API. inlineType replaces
// the Content-Type for formats browsers would otherwise download (Markdown).
func reportDisposition(w http.ResponseWriter, r *http.Request, filename, inlineType string) {
	if r.URL.Query().Get("inline") != "1" {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		return
	}
	if inlineType != "" {
		w.Header().Set("Content-Type", inlineType)
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	w.Header().Set("Content-Security-Policy", "sandbox allow-scripts; default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; img-src data:")
}

// shortID keeps attachment filenames readable while staying scan-unique.
func shortID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

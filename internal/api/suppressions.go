package api

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"bluntcode/internal/database"
)

// maxSuppressionReasonLength bounds the optional dismissal note; a reason is a
// short human annotation, not a document.
const maxSuppressionReasonLength = 500

// validFingerprint accepts exactly the shape Finding.SetFingerprint produces:
// a 64-character lowercase sha256 hex string.
func validFingerprint(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// getSuppressions lists the workspace's dismissed finding fingerprints.
func (s *Server) getSuppressions(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	items, err := s.db.Suppressions(r.Context(), work.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load suppressions.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

// addSuppression records a dismissed finding fingerprint (the "ignore this
// finding forever" workflow). The suppression is keyed by workspace and
// fingerprint, so every future scan of this workspace still stores matching
// findings but excludes them from totals, reports, exports, and the CI gate.
// Suppressing while a scan is running is safe: that scan keeps its
// already-stored findings and the suppression applies from the next scan.
func (s *Server) addSuppression(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	var input struct {
		Fingerprint string `json:"fingerprint"`
		Reason      string `json:"reason"`
	}
	if err := decode(r, &input); err != nil {
		fail(w, 400, "INVALID_JSON", "fingerprint is required.")
		return
	}
	input.Fingerprint = strings.ToLower(strings.TrimSpace(input.Fingerprint))
	if !validFingerprint(input.Fingerprint) {
		fail(w, 400, "INVALID_FINGERPRINT", "fingerprint must be a 64-character sha256 hex string.")
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if len(input.Reason) > maxSuppressionReasonLength {
		fail(w, 400, "INVALID_REASON", fmt.Sprintf("reason must be at most %d characters.", maxSuppressionReasonLength))
		return
	}
	item, err := s.db.AddSuppression(r.Context(), work.ID, input.Fingerprint, input.Reason)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not save suppression.")
		return
	}
	s.suppressionsChanged(r.Context(), work.ID)
	writeJSON(w, 201, item)
}

// suppressionsChanged re-derives the workspace's persisted scan counts after a
// suppression is added, removed or imported. A failure only leaves the old
// counts in place until the next scan, so it is logged, not surfaced.
func (s *Server) suppressionsChanged(ctx context.Context, workspaceID string) {
	s.cache.invalidate(workspaceID)
	if err := s.db.RecountWorkspaceScans(ctx, workspaceID); err != nil {
		s.log.Warn("recount scans after suppression change failed", "workspace", workspaceID, "error", err)
	}
}

// removeSuppression deletes one dismissal; the finding counts again in every
// scan of the workspace right away.
func (s *Server) removeSuppression(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	fingerprint := strings.ToLower(strings.TrimSpace(r.PathValue("fingerprint")))
	if !validFingerprint(fingerprint) {
		fail(w, 400, "INVALID_FINGERPRINT", "fingerprint must be a 64-character sha256 hex string.")
		return
	}
	if err := s.db.RemoveSuppression(r.Context(), work.ID, fingerprint); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			fail(w, 404, "SUPPRESSION_NOT_FOUND", "This fingerprint was not suppressed.")
			return
		}
		fail(w, 500, "DATABASE_ERROR", "Could not remove suppression.")
		return
	}
	s.suppressionsChanged(r.Context(), work.ID)
	w.WriteHeader(http.StatusNoContent)
}

// suppressionsCSVHeader is the fixed export column order of the suppression
// CSV; it and the row writer below must stay in sync. The import endpoint
// only requires the fingerprint column, so the header stays importable
// as-is.
var suppressionsCSVHeader = []string{"fingerprint", "reason", "created_at"}

// suppressionsCSV exports the workspace's dismissals as a spreadsheet-friendly
// CSV attachment: UTF-8 BOM for Excel, formula-neutralized cells (the same
// csvCell transform the findings export applies), and RFC3339 timestamps.
// The file is a valid input for suppressions/import, so dismissals can move
// between machines and workspaces.
func (s *Server) suppressionsCSV(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	items, err := s.db.Suppressions(r.Context(), work.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load suppressions.")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="bluntcode-workspace-%s-suppressions.csv"`, shortID(work.ID)))
	_, _ = w.Write([]byte(csvBOM))
	writer := csv.NewWriter(w)
	if err := writer.Write(suppressionsCSVHeader); err != nil {
		s.log.Warn("csv header write failed", "error", err)
		return
	}
	for _, item := range items {
		if err := writer.Write([]string{
			csvCell(item.Fingerprint), csvCell(item.Reason), item.CreatedAt.UTC().Format(time.RFC3339),
		}); err != nil {
			s.log.Warn("csv row write failed", "error", err)
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		s.log.Warn("csv flush failed", "error", err)
	}
}

// importSuppressions bulk-loads dismissal fingerprints from the CSV text the
// suppressions.csv export produces. Only the fingerprint column is read —
// reason and created_at are ignored — fingerprints are normalized to
// lowercase before validation, invalid rows are skipped without failing the
// whole request, and rows already suppressed (or repeated within the file)
// are reported as duplicates rather than rewritten.
func (s *Server) importSuppressions(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	var input struct {
		CSV string `json:"csv"`
	}
	if err := decode(r, &input); err != nil || strings.TrimSpace(input.CSV) == "" {
		fail(w, 400, "INVALID_CSV", "csv is required.")
		return
	}
	records, err := csv.NewReader(strings.NewReader(input.CSV)).ReadAll()
	if err != nil || len(records) == 0 {
		fail(w, 400, "INVALID_CSV", "The upload is not parseable CSV text.")
		return
	}
	column := -1
	for index, name := range records[0] {
		if strings.EqualFold(strings.TrimSpace(name), "fingerprint") {
			column = index
			break
		}
	}
	if column < 0 {
		fail(w, 400, "INVALID_CSV", `The CSV must have a "fingerprint" column.`)
		return
	}
	existing, err := s.db.SuppressedFingerprints(r.Context(), work.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load suppressions.")
		return
	}
	skippedInvalid, duplicate := 0, 0
	seen := map[string]bool{}
	var batch []database.SuppressionInput
	for _, row := range records[1:] {
		if column >= len(row) {
			skippedInvalid++
			continue
		}
		fingerprint := strings.ToLower(strings.TrimSpace(row[column]))
		if !validFingerprint(fingerprint) {
			skippedInvalid++
			continue
		}
		if existing[fingerprint] || seen[fingerprint] {
			duplicate++
			continue
		}
		seen[fingerprint] = true
		batch = append(batch, database.SuppressionInput{Fingerprint: fingerprint})
	}
	// One transaction for the whole file: a failure imports nothing, so the
	// 500 below is the truth and a retry starts from the same state.
	if err := s.db.AddSuppressions(r.Context(), work.ID, batch); err != nil {
		s.log.Warn("suppression import failed", "workspace", work.ID, "error", err)
		fail(w, 500, "DATABASE_ERROR", "Could not save suppressions; nothing was imported.")
		return
	}
	imported := len(batch)
	if imported > 0 {
		s.suppressionsChanged(r.Context(), work.ID)
	}
	writeJSON(w, 200, map[string]int{"imported": imported, "skipped_invalid": skippedInvalid, "duplicate": duplicate})
}

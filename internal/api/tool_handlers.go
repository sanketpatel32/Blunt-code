// tool handlers handlers.
package api

import (
	"net/http"

	"bluntcode/internal/analyzers"
	"bluntcode/internal/tools"
)

func (s *Server) listTools(w http.ResponseWriter, r *http.Request) {
	if s.tools == nil {
		fail(w, 503, "TOOLS_UNAVAILABLE", "Tool service is unavailable.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": s.tools.All()})
}

// listAnalyzers serves the capability inventory merged with live registry
// state: one row per analyzer with category, execution kind, profiles, input
// kinds, network use, languages, version, and readiness. The Tools page and
// docs read this instead of keeping hand-maintained analyzer lists.
func (s *Server) listAnalyzers(w http.ResponseWriter, r *http.Request) {
	if s.scans == nil {
		fail(w, 503, "SCANS_UNAVAILABLE", "Scan service is unavailable.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": s.scans.AnalyzerStatuses(r.Context())})
}

func (s *Server) installTool(w http.ResponseWriter, r *http.Request) {
	if s.tools == nil {
		fail(w, 503, "TOOLS_UNAVAILABLE", "Tool service is unavailable.")
		return
	}
	id := tools.CanonicalToolID(r.PathValue("id"))
	if !analyzers.InstallableTool(id) {
		fail(w, 404, "TOOL_NOT_FOUND", "Tool was not found.")
		return
	}
	if err := s.tools.Ensure(r.Context(), id); err != nil {
		fail(w, 409, "TOOL_NOT_READY", err.Error())
		return
	}
	if s.scans != nil {
		s.scans.InvalidateStatusCache()
	}
	writeJSON(w, 200, s.tools.Status(id))
}

func (s *Server) uninstallTool(w http.ResponseWriter, r *http.Request) {
	if s.tools == nil {
		fail(w, 503, "TOOLS_UNAVAILABLE", "Tool service is unavailable.")
		return
	}
	id := tools.CanonicalToolID(r.PathValue("id"))
	if !analyzers.InstallableTool(id) {
		fail(w, 404, "TOOL_NOT_FOUND", "Tool was not found.")
		return
	}
	if s.db != nil {
		if summary, err := s.db.ScanSummary(r.Context()); err == nil && summary.ActiveScans > 0 {
			fail(w, 409, "SCAN_IN_PROGRESS", "Cannot uninstall tools while scans are running. Please wait for active scans to finish.")
			return
		}
	}
	if s.scans != nil {
		_ = s.scans.StopAnalyzer(r.Context(), id)
	}
	if err := s.tools.Uninstall(r.Context(), id); err != nil {
		fail(w, 500, "UNINSTALL_FAILED", err.Error())
		return
	}
	if s.scans != nil {
		s.scans.InvalidateStatusCache()
	}
	writeJSON(w, 200, s.tools.Status(id))
}

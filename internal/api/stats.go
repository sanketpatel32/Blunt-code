package api

import (
	"net/http"
	"time"

	"bluntcode/internal/database"
)

// statsTools is the tooling readiness slice of the global overview: a count
// pair over the tools service's status machinery, not the full per-tool
// statuses, which GET /api/v1/tools already serves.
type statsTools struct {
	Total int `json:"total"`
	Ready int `json:"ready"`
}

// statsResponse frames the repository aggregates with the fields the overview
// adds per request: tool readiness (omitted when no tools service is wired)
// and the RFC3339 generation timestamp. The embedded GlobalStats keeps the
// payload flat: workspaces, scans, findings, and suppressions inline.
type statsResponse struct {
	database.GlobalStats
	Tools       *statsTools `json:"tools,omitempty"`
	GeneratedAt string      `json:"generated_at"`
}

// globalStats serves the dashboard's global overview: workspace, scan, and
// suppression counters plus the latest-completed-per-workspace severity rollup.
// Every database figure comes from GlobalStats' two aggregate queries (no
// per-workspace loops), and tool readiness is folded in only when a tools
// service is wired into the server.
func (s *Server) globalStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.db.GlobalStats(r.Context())
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load global stats.")
		return
	}
	response := statsResponse{GlobalStats: stats, GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
	if s.tools != nil {
		statuses := s.tools.All()
		ready := 0
		for _, status := range statuses {
			if status.Ready {
				ready++
			}
		}
		response.Tools = &statsTools{Total: len(statuses), Ready: ready}
	}
	writeJSON(w, 200, response)
}

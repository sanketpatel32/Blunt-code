// Package api exposes the local-only HTTP API.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"bluntcode/internal/config"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/events"
	"bluntcode/internal/reports"
	"bluntcode/internal/scans"
	"bluntcode/internal/tools"
)

const APIVersion = "v1"

// sseHeartbeatInterval paces the SSE keep-alive comments written to idle scan
// event streams. Package-level so tests can shorten it; never mutate outside
// tests.
var sseHeartbeatInterval = 15 * time.Second

type Server struct {
	db         *database.DB
	bus        *events.Bus
	scans      *scans.Service
	tools      *tools.Service
	paths      config.Paths
	version    string
	log        *slog.Logger
	mux        *http.ServeMux
	shutdown   func()
	openFolder func(dir string) error
	cache      *workspaceCaches
	token      string
	tickets    ticketStore
	// wsLocks serializes per-workspace check-and-act sequences (active-scan
	// probe then delete, active-scan probe then start) that are otherwise a
	// read-then-act race on the scans table.
	wsLocks sync.Map // workspaceID -> *sync.Mutex
	// treeCache memoizes the file-tree walk per workspace+path; discovery on
	// every tree UI click re-walked the whole directory and cross-multiplied
	// overrides per item. Entries expire quickly so rule/scan changes show up
	// within ~30s.
	treeCache sync.Map // workspaceID|path -> cachedTree
	// checkRoot and rootTimeout back the root_missing probe on workspace
	// views; tests swap them to simulate missing or hanging folders.
	checkRoot   func(root string) string
	rootTimeout time.Duration
	probes      rootProber
}

// scanDetail is a scan payload enriched for single-scan views: the analyzer
// run rows GET /scans/{id} serves, plus the new/fixed comparison counts the
// report model computes. The workspace view's latest_scan shares it so the
// workspace page's "Latest analysis" card renders the same detail.
type cachedTree struct {
	payload map[string]any
	expires time.Time
}

type scanDetail struct {
	core.Scan
	AnalyzerRuns []map[string]any `json:"analyzer_runs"`
	NewCount     int              `json:"new_count"`
	FixedCount   int              `json:"fixed_count"`
}

// workspaceView keeps the dashboard payload small while including the two
// pieces of context people need before opening a workspace: source languages
// and the most recent analysis. latest_scan_coverage pairs that analysis
// with how much of its analyzer set actually completed (IMP-13): a grade
// from a partial scan must not read as the assurance of a complete one.
// last_completed_scan is the newest scan that finished: boards fall back to
// it when latest_scan is a cancelled/interrupted run, so a killed re-scan
// cannot erase a workspace's last good numbers.
type workspaceView struct {
	core.Workspace
	Tags               []string               `json:"tags,omitempty"`
	Languages          []string               `json:"languages,omitempty"`
	LatestScan         *scanDetail            `json:"latest_scan,omitempty"`
	LatestScanCov      *database.ScanCoverage `json:"latest_scan_coverage,omitempty"`
	LastCompletedScan  *scanDetail            `json:"last_completed_scan,omitempty"`
	AssessmentCoverage *database.ScanCoverage `json:"assessment_coverage,omitempty"`
	// Trend is total_findings of the newest completed scans, oldest first
	// (list endpoint only), for the card sparkline.
	Trend []int `json:"trend,omitempty"`
	// RootMissing is true when the registered folder is gone, is no longer a
	// directory, or cannot be read; RootProblem names which ("missing",
	// "not_directory", "unreadable"). Probed on every list/GET response.
	RootMissing bool   `json:"root_missing"`
	RootProblem string `json:"root_problem,omitempty"`
}

func New(db *database.DB, bus *events.Bus, scanService *scans.Service, toolService *tools.Service, paths config.Paths, version string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{db: db, bus: bus, scans: scanService, tools: toolService, paths: paths, version: version, log: logger, mux: http.NewServeMux(), openFolder: openFolderInExplorer, cache: newWorkspaceCaches(), token: mustToken(), checkRoot: checkWorkspaceRoot, rootTimeout: rootCheckTimeout}
	s.routes()
	return s
}

// lockWorkspace takes the per-workspace mutation lock and returns its
// release. Callers hold it across a check-and-act span only.
func (s *Server) lockWorkspace(id string) (unlock func()) {
	v, _ := s.wsLocks.LoadOrStore(id, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (s *Server) Handler() http.Handler {
	return securityMiddleware(s.apiGuard(rateLimitMiddleware(s.mux, jsonRouteErrors(s.mux))))
}

// StaticGuard applies the API's loopback Host check and baseline security
// headers to a non-API handler (the embedded UI). The UI has no
// state-changing routes of its own, but without the guard it could be
// embedded in a foreign page or reached through a non-loopback Host
// (DNS rebinding) while sharing origin with the API.
func StaticGuard(next http.Handler) http.Handler { return securityMiddleware(next) }

// SetShutdown connects the local stop action to the host application's
// graceful shutdown lifecycle. It is deliberately supplied by cmd, not HTTP.
func (s *Server) SetShutdown(fn func()) { s.shutdown = fn }
func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/v1/health", s.health)
	s.mux.HandleFunc("GET /api/v1/session", s.session)
	s.mux.HandleFunc("POST /api/v1/session/tickets", s.sessionTicket)
	s.mux.HandleFunc("GET /api/v1/meta", s.meta)
	s.mux.HandleFunc("GET /api/v1/stats", s.globalStats)
	s.mux.HandleFunc("GET /api/v1/settings", s.getSettings)
	s.mux.HandleFunc("PATCH /api/v1/settings", s.updateSettings)
	s.mux.HandleFunc("GET /api/v1/workspaces", s.listWorkspaces)
	s.mux.HandleFunc("POST /api/v1/system/seed-demo", s.seedDemo)
	s.mux.HandleFunc("POST /api/v1/system/unseed-demo", s.unseedDemo)
	s.mux.HandleFunc("POST /api/v1/workspaces", s.createWorkspace)
	s.mux.HandleFunc("POST /api/v1/system/select-folder", s.selectFolder)
	s.mux.HandleFunc("POST /api/v1/system/open-folder", s.openSystemFolder)
	s.mux.HandleFunc("POST /api/v1/system/stop", s.stopServer)
	s.mux.HandleFunc("POST /api/v1/system/clean", s.cleanSystem)
	s.mux.HandleFunc("GET /api/v1/update/check", s.updateCheck)
	s.mux.HandleFunc("POST /api/v1/update/apply", s.updateApply)
	s.mux.HandleFunc("GET /api/v1/workspaces/{id}", s.getWorkspace)
	s.mux.HandleFunc("PATCH /api/v1/workspaces/{id}", s.updateWorkspace)
	s.mux.HandleFunc("DELETE /api/v1/workspaces/{id}", s.deleteWorkspace)
	s.mux.HandleFunc("POST /api/v1/workspaces/{id}/discover", s.discoverWorkspace)
	s.mux.HandleFunc("GET /api/v1/workspaces/{id}/scan-plan", s.scanPlan)
	s.mux.HandleFunc("GET /api/v1/workspaces/{id}/notes/{fingerprint}", s.findingNotes)
	s.mux.HandleFunc("POST /api/v1/workspaces/{id}/notes/{fingerprint}", s.findingNotes)
	s.mux.HandleFunc("DELETE /api/v1/workspaces/{id}/notes/{fingerprint}/{note}", s.findingNotes)
	s.mux.HandleFunc("GET /api/v1/workspaces/{id}/tree", s.tree)
	s.mux.HandleFunc("GET /api/v1/workspaces/{id}/path-overrides", s.getPathOverrides)
	s.mux.HandleFunc("PUT /api/v1/workspaces/{id}/path-overrides", s.putPathOverrides)
	s.mux.HandleFunc("GET /api/v1/workspaces/{id}/tags", s.getWorkspaceTags)
	s.mux.HandleFunc("PUT /api/v1/workspaces/{id}/tags", s.putWorkspaceTags)
	s.mux.HandleFunc("GET /api/v1/workspaces/{id}/rules", s.getRules)
	s.mux.HandleFunc("PUT /api/v1/workspaces/{id}/rules", s.putRules)
	s.mux.HandleFunc("GET /api/v1/workspaces/{id}/suppressions", s.getSuppressions)
	s.mux.HandleFunc("POST /api/v1/workspaces/{id}/suppressions", s.addSuppression)
	s.mux.Handle("GET /api/v1/workspaces/{id}/suppressions.csv", exportDeadline(http.HandlerFunc(s.suppressionsCSV)))
	s.mux.HandleFunc("POST /api/v1/workspaces/{id}/suppressions/import", s.importSuppressions)
	s.mux.HandleFunc("DELETE /api/v1/workspaces/{id}/suppressions/{fingerprint}", s.removeSuppression)
	s.mux.HandleFunc("GET /api/v1/workspaces/{id}/trends", s.workspaceTrends)
	s.mux.HandleFunc("GET /api/v1/workspaces/{id}/risk", s.workspaceRisk)
	s.mux.HandleFunc("GET /api/v1/workspaces/{id}/scans", s.listScans)
	s.mux.HandleFunc("DELETE /api/v1/workspaces/{id}/scans", s.pruneWorkspaceScans)
	s.mux.HandleFunc("POST /api/v1/workspaces/{id}/scans", s.startScan)
	s.mux.HandleFunc("POST /api/v1/workspaces/{id}/pentest/probe", s.pentestProbe)
	s.mux.HandleFunc("POST /api/v1/pentest/probe", s.pentestProbe)
	s.mux.HandleFunc("GET /api/v1/scans", s.recentScans)
	s.mux.HandleFunc("GET /api/v1/scans/{id}", s.getScan)
	s.mux.HandleFunc("DELETE /api/v1/scans/{id}", s.deleteScan)
	s.mux.HandleFunc("GET /api/v1/findings/search", s.searchFindings)
	s.mux.HandleFunc("POST /api/v1/scans/{id}/cancel", s.cancelScan)
	s.mux.HandleFunc("GET /api/v1/scans/{id}/events", s.scanEvents)
	s.mux.HandleFunc("GET /api/v1/scans/{id}/findings", s.findings)
	// Export routes get a per-handler write deadline: a stalled reader holds
	// the handler goroutine — and, with MaxOpenConns(1), its DB slot — forever
	// otherwise. SSE and the JSON API stay deadline-free by design.
	s.mux.Handle("GET /api/v1/scans/{id}/findings.csv", exportDeadline(http.HandlerFunc(s.findingsCSV)))
	s.mux.Handle("GET /api/v1/scans/{id}/findings.json", exportDeadline(http.HandlerFunc(s.findingsJSON)))
	// report.json is the same versioned document under the report.* naming
	// the other full-report exports use.
	s.mux.Handle("GET /api/v1/scans/{id}/report.json", exportDeadline(http.HandlerFunc(s.findingsJSON)))
	s.mux.Handle("GET /api/v1/scans/{id}/findings.jsonl", exportDeadline(http.HandlerFunc(s.findingsJSONL)))
	s.mux.HandleFunc("GET /api/v1/scans/{id}/fixed", s.fixedFindings)
	s.mux.HandleFunc("GET /api/v1/scans/{id}/compare", s.scanCompare)
	s.mux.HandleFunc("GET /api/v1/scans/{id}/findings/{findingID}/preview", s.findingPreview)
	s.mux.HandleFunc("GET /api/v1/scans/{id}/report", s.report)
	s.mux.Handle("GET /api/v1/scans/{id}/report.md", exportDeadline(http.HandlerFunc(s.reportMarkdown)))
	s.mux.Handle("GET /api/v1/scans/{id}/report.sarif", exportDeadline(http.HandlerFunc(s.reportSARIF)))
	s.mux.Handle("GET /api/v1/scans/{id}/report.html", exportDeadline(http.HandlerFunc(s.reportHTML)))
	s.mux.HandleFunc("GET /api/v1/analyzers", s.listAnalyzers)
	s.mux.HandleFunc("GET /api/v1/tools", s.listTools)
	s.mux.HandleFunc("POST /api/v1/tools/{id}/install", s.installTool)
	s.mux.HandleFunc("POST /api/v1/tools/{id}/repair", s.installTool)
	s.mux.HandleFunc("POST /api/v1/tools/{id}/update", s.installTool)
	s.mux.HandleFunc("DELETE /api/v1/tools/{id}", s.uninstallTool)
	s.mux.HandleFunc("POST /api/v1/tools/{id}/uninstall", s.uninstallTool)
}

// exportWriteBound caps how long an export handler may write to one client.
// Generous for a local API: it exists to reap stalled readers, not to time
// honest downloads.
const exportWriteBound = 5 * time.Minute

// exportDeadline wraps a heavy, non-streaming handler with an absolute write
// deadline. ReadTimeout already bounds the body read; this bounds the writes
// so a stalled reader can't hold the connection — and its DB slot — forever.
// Not usable for SSE or other long-lived streams.
func exportDeadline(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ErrUnsupported writers (httptest recorders, non-HTTP/1 transports)
		// simply can't take a deadline — serve them without one. Any other
		// failure means the server accepted a socket it cannot bound.
		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(exportWriteBound)); err != nil && !errors.Is(err, errors.ErrUnsupported) {
			http.Error(w, "write deadline failed", http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message}})
}

// failField is fail plus the request field the error belongs to, so a form
// can show the message under the input that caused it instead of a toast.
func failField(w http.ResponseWriter, status int, code, message, field string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "field": field}})
}

// invalidJSONMessage is the one message every handler answers a body that
// does not decode with; naming a single field there misled callers whose
// JSON was simply malformed.
const invalidJSONMessage = "Request body is not valid JSON for this endpoint."

func decode(r *http.Request, into any) error {
	d := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	d.DisallowUnknownFields()
	return d.Decode(into)
}

// validID accepts the canonical lowercase UUID shape ID storage uses, plus
// its uppercase spelling: IDs are normalized with pathID before validation
// and lookup so /workspaces/2538AA38-… resolves the same row as its
// lowercase form.
func validID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// pathID returns the {id} path value lowercased: UUIDs are stored lowercase
// and SQLite lookups are byte-exact, so an uppercase spelling must be
// normalized before it reaches the database.
func pathID(r *http.Request) string {
	return strings.ToLower(r.PathValue("id"))
}
func (s *Server) workspace(r *http.Request) (core.Workspace, bool) {
	id := pathID(r)
	if !validID(id) {
		return core.Workspace{}, false
	}
	w, err := s.db.Workspace(r.Context(), id)
	if err != nil {
		return core.Workspace{}, false
	}
	return w, true
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "api_version": APIVersion})
}
func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	demoEnv := strings.EqualFold(strings.TrimSpace(os.Getenv("BLUNTCODE_DEMO_SEED")), "1")
	demoToken := strings.TrimSpace(os.Getenv("BLUNTCODE_DEMO_TOKEN"))
	writeJSON(w, http.StatusOK, map[string]any{
		"version": s.version, "api_version": APIVersion, "os": runtime.GOOS, "architecture": runtime.GOARCH, "data_directory": s.paths.DataDir,
		"demo_seed_env_enabled":    demoEnv,
		"demo_seed_requires_token": demoEnv && demoToken != "",
	})
}
func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	value, err := s.db.AppSettings(r.Context())
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load application settings.")
		return
	}
	writeJSON(w, 200, value)
}
func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Offline     *bool `json:"offline"`
		OpenBrowser *bool `json:"open_browser"`
		UpdateCheck *bool `json:"update_check"`
	}
	if err := decode(r, &input); err != nil {
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			fail(w, 400, "INVALID_JSON", invalidJSONMessage)
			return
		}
		fail(w, 400, "INVALID_SETTINGS", "Provide offline, open_browser, or update_check as a boolean.")
		return
	}
	if input.Offline == nil && input.OpenBrowser == nil && input.UpdateCheck == nil {
		fail(w, 400, "INVALID_SETTINGS", "Provide offline, open_browser, or update_check as a boolean.")
		return
	}
	value, err := s.db.AppSettings(r.Context())
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load application settings.")
		return
	}
	if input.Offline != nil {
		value.Offline = *input.Offline
	}
	if input.OpenBrowser != nil {
		value.OpenBrowser = *input.OpenBrowser
	}
	if input.UpdateCheck != nil {
		value.UpdateCheck = *input.UpdateCheck
	}
	if err := s.db.SaveAppSettings(r.Context(), value); err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not save application settings.")
		return
	}
	if s.tools != nil {
		s.tools.SetOffline(value.Offline)
	}
	writeJSON(w, 200, value)
}

// viewPrefetch carries what the list endpoint loaded in bulk: analyzer runs
// keyed by scan id, and list mode, which never walks the filesystem.
type viewPrefetch struct {
	runs map[string][]reports.Run
	list bool
}

// openFolderTimeout bounds the Explorer launch: explorer.exe hands the folder
// off to the shell and exits almost immediately, so a launch that has not
// finished within this window is treated as a failure instead of blocking the
// request indefinitely.
const openFolderTimeout = 5 * time.Second

type treeItem struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Type string `json:"type"`
	// Language classifies files (the discovery table, mirrored by Tree) and is
	// empty for directories; the Files page renders it as a chip and filters
	// on it. ExcludedReason explains a selection-excluded node using the same
	// vocabulary as the snapshot's skip_counts.
	Language       string `json:"language,omitempty"`
	Included       bool   `json:"included"`
	ExcludedReason string `json:"excluded_reason,omitempty"`
	Partial        bool   `json:"partial,omitempty"`
	HasChildren    bool   `json:"has_children,omitempty"`
}

// riskSeverityWeights weight one finding of each severity for the workspace
// risk score. The scale is deliberately simple and documented: a critical
// issue is worth two highs, a high two-and-a-half mediums, so the score moves
// when the serious tail moves without drowning in low-severity noise.
var riskSeverityWeights = map[string]float64{
	"critical": 10,
	"high":     5,
	"medium":   2,
	"low":      1,
	"info":     0,
}

// trendView is one trend point plus the weighted risk score and A-D grade of
// that scan, computed with the same weights and bands as /risk, so the UI can
// draw grade history without re-implementing the scale.
type trendView struct {
	database.SeverityTrendPoint
	Score float64 `json:"score"`
	Grade string  `json:"grade"`
}

// csvBOM prefixes CSV exports so Excel on Windows detects UTF-8 and renders
// non-ASCII finding text correctly instead of mangling it.
const csvBOM = "\xef\xbb\xbf"

// findingsCSVHeader is the fixed export column order; findingsCSVHeader and
// the row writer below must stay in sync.
var findingsCSVHeader = []string{"severity", "category", "analyzer", "rule_id", "title", "message", "file", "line", "column", "end_line", "status", "remediation", "documentation_url"}

type sourcePreviewLine struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

type sourcePreview struct {
	Path               string              `json:"path"`
	Lines              []sourcePreviewLine `json:"lines"`
	HighlightStartLine int                 `json:"highlight_start_line,omitempty"`
	HighlightEndLine   int                 `json:"highlight_end_line,omitempty"`
	Note               string              `json:"note,omitempty"`
}

const maxPreviewFileBytes = 1 << 20

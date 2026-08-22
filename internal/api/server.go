// Package api exposes the local-only HTTP API.
package api

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"bluntcode/internal/config"
	"bluntcode/internal/core"
	"bluntcode/internal/database"
	"bluntcode/internal/discovery"
	"bluntcode/internal/events"
	"bluntcode/internal/process"
	"bluntcode/internal/reports"
	"bluntcode/internal/scans"
	"bluntcode/internal/tools"
	"bluntcode/internal/windows/folderpicker"
	"bluntcode/internal/workspace"
)

const APIVersion = "v1"

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
}

// workspaceView keeps the dashboard payload small while including the two
// pieces of context people need before opening a workspace: source languages
// and the most recent analysis.
type workspaceView struct {
	core.Workspace
	Languages  []string   `json:"languages,omitempty"`
	LatestScan *core.Scan `json:"latest_scan,omitempty"`
}

func New(db *database.DB, bus *events.Bus, scanService *scans.Service, toolService *tools.Service, paths config.Paths, version string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{db: db, bus: bus, scans: scanService, tools: toolService, paths: paths, version: version, log: logger, mux: http.NewServeMux(), openFolder: openFolderInExplorer}
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler { return securityMiddleware(s.mux) }

// SetShutdown connects the local stop action to the host application's
// graceful shutdown lifecycle. It is deliberately supplied by cmd, not HTTP.
func (s *Server) SetShutdown(fn func()) { s.shutdown = fn }
func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/v1/health", s.health)
	s.mux.HandleFunc("GET /api/v1/meta", s.meta)
	s.mux.HandleFunc("GET /api/v1/settings", s.getSettings)
	s.mux.HandleFunc("PATCH /api/v1/settings", s.updateSettings)
	s.mux.HandleFunc("GET /api/v1/workspaces", s.listWorkspaces)
	s.mux.HandleFunc("POST /api/v1/workspaces", s.createWorkspace)
	s.mux.HandleFunc("POST /api/v1/system/select-folder", s.selectFolder)
	s.mux.HandleFunc("POST /api/v1/system/open-folder", s.openSystemFolder)
	s.mux.HandleFunc("POST /api/v1/system/stop", s.stopServer)
	s.mux.HandleFunc("GET /api/v1/workspaces/{id}", s.getWorkspace)
	s.mux.HandleFunc("PATCH /api/v1/workspaces/{id}", s.updateWorkspace)
	s.mux.HandleFunc("DELETE /api/v1/workspaces/{id}", s.deleteWorkspace)
	s.mux.HandleFunc("POST /api/v1/workspaces/{id}/discover", s.discoverWorkspace)
	s.mux.HandleFunc("GET /api/v1/workspaces/{id}/tree", s.tree)
	s.mux.HandleFunc("GET /api/v1/workspaces/{id}/path-overrides", s.getPathOverrides)
	s.mux.HandleFunc("PUT /api/v1/workspaces/{id}/path-overrides", s.putPathOverrides)
	s.mux.HandleFunc("GET /api/v1/workspaces/{id}/rules", s.getRules)
	s.mux.HandleFunc("PUT /api/v1/workspaces/{id}/rules", s.putRules)
	s.mux.HandleFunc("GET /api/v1/workspaces/{id}/scans", s.listScans)
	s.mux.HandleFunc("POST /api/v1/workspaces/{id}/scans", s.startScan)
	s.mux.HandleFunc("GET /api/v1/scans", s.recentScans)
	s.mux.HandleFunc("GET /api/v1/scans/{id}", s.getScan)
	s.mux.HandleFunc("POST /api/v1/scans/{id}/cancel", s.cancelScan)
	s.mux.HandleFunc("GET /api/v1/scans/{id}/events", s.scanEvents)
	s.mux.HandleFunc("GET /api/v1/scans/{id}/findings", s.findings)
	s.mux.HandleFunc("GET /api/v1/scans/{id}/findings.csv", s.findingsCSV)
	s.mux.HandleFunc("GET /api/v1/scans/{id}/findings/{findingID}/preview", s.findingPreview)
	s.mux.HandleFunc("GET /api/v1/scans/{id}/report", s.report)
	s.mux.HandleFunc("GET /api/v1/scans/{id}/report.md", s.reportMarkdown)
	s.mux.HandleFunc("GET /api/v1/scans/{id}/report.sarif", s.reportSARIF)
	s.mux.HandleFunc("GET /api/v1/scans/{id}/report.html", s.reportHTML)
	s.mux.HandleFunc("GET /api/v1/tools", s.listTools)
	s.mux.HandleFunc("POST /api/v1/tools/{id}/install", s.installTool)
	s.mux.HandleFunc("POST /api/v1/tools/{id}/repair", s.installTool)
	s.mux.HandleFunc("POST /api/v1/tools/{id}/update", s.installTool)
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
func decode(r *http.Request, into any) error {
	d := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	d.DisallowUnknownFields()
	return d.Decode(into)
}
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
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func (s *Server) workspace(r *http.Request) (core.Workspace, bool) {
	id := r.PathValue("id")
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
	writeJSON(w, http.StatusOK, map[string]any{"version": s.version, "api_version": APIVersion, "os": runtime.GOOS, "architecture": runtime.GOARCH, "data_directory": s.paths.DataDir})
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
	}
	if err := decode(r, &input); err != nil || (input.Offline == nil && input.OpenBrowser == nil) {
		fail(w, 400, "INVALID_SETTINGS", "Provide offline or open_browser as a boolean.")
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
	if err := s.db.SaveAppSettings(r.Context(), value); err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not save application settings.")
		return
	}
	if s.tools != nil {
		s.tools.SetOffline(value.Offline)
	}
	writeJSON(w, 200, value)
}
func (s *Server) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	items, err := s.db.Workspaces(r.Context())
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not list workspaces.")
		return
	}
	views := make([]workspaceView, 0, len(items))
	for _, item := range items {
		view, err := s.workspaceView(r.Context(), item)
		if err != nil {
			fail(w, 500, "DATABASE_ERROR", "Could not load workspace analysis.")
			return
		}
		views = append(views, view)
	}
	writeJSON(w, 200, map[string]any{"items": views})
}
func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RootPath string `json:"root_path"`
		Name     string `json:"name"`
	}
	if err := decode(r, &input); err != nil {
		fail(w, 400, "INVALID_JSON", "root_path is required.")
		return
	}
	root, err := workspace.NormalizeRoot(input.RootPath)
	if err != nil {
		fail(w, 400, "INVALID_PATH", err.Error())
		return
	}
	if input.Name == "" {
		input.Name = filepath.Base(root)
	}
	if existing, err := s.db.WorkspaceByRoot(r.Context(), root); err == nil {
		writeJSON(w, 200, existing)
		return
	}
	created, err := s.db.CreateWorkspace(r.Context(), core.Workspace{Name: input.Name, RootPath: root})
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not save workspace.")
		return
	}
	writeJSON(w, 201, created)
}
func (s *Server) getWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	_ = s.db.TouchWorkspace(r.Context(), workspace.ID)
	view, err := s.workspaceView(r.Context(), workspace)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load workspace analysis.")
		return
	}
	writeJSON(w, 200, view)
}

func (s *Server) workspaceView(ctx context.Context, workspace core.Workspace) (workspaceView, error) {
	view := workspaceView{Workspace: workspace}
	scans, err := s.db.Scans(ctx, workspace.ID)
	if err != nil {
		return view, err
	}
	if len(scans) > 0 {
		latest := scans[0]
		if latest.Snapshot != nil {
			view.Languages = languageNames(latest.Snapshot.Languages)
		}
		// The dashboard does not need every selected path from the immutable
		// snapshot; returning it would make the workspace list unnecessarily big.
		latest.Snapshot = nil
		view.LatestScan = &latest
	}
	if len(view.Languages) == 0 {
		patterns, err := s.userExcludes(ctx, workspace.ID)
		if err == nil {
			if discovered, err := discovery.Discover(ctx, workspace.RootPath, patterns); err == nil {
				view.Languages = languageNames(discovered.Languages)
			}
		}
	}
	return view, nil
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
	}
	if err := decode(r, &input); err != nil {
		fail(w, 400, "INVALID_JSON", "Invalid workspace update.")
		return
	}
	if input.Name != nil {
		current.Name = strings.TrimSpace(*input.Name)
	}
	if input.DefaultProfile != nil {
		current.DefaultProfile = *input.DefaultProfile
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
	if err := s.db.DeleteWorkspace(r.Context(), workspace.ID); err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not delete workspace.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) selectFolder(w http.ResponseWriter, r *http.Request) {
	path, cancelled, err := folderpicker.Select(r.Context())
	if err != nil {
		fail(w, 500, "FOLDER_PICKER_FAILED", "Could not open the native folder picker.")
		return
	}
	writeJSON(w, 200, map[string]any{"cancelled": cancelled, "path": path})
}
func (s *Server) stopServer(w http.ResponseWriter, r *http.Request) {
	if s.shutdown == nil {
		fail(w, 503, "SERVER_STOP_UNAVAILABLE", "The server cannot be stopped from this session.")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"state": "stopping"})
	go s.shutdown()
}

// openFolderTimeout bounds the Explorer launch: explorer.exe hands the folder
// off to the shell and exits almost immediately, so a launch that has not
// finished within this window is treated as a failure instead of blocking the
// request indefinitely.
const openFolderTimeout = 5 * time.Second

// openSystemFolder opens one of Blunt Code's own data folders in Windows
// Explorer. The request carries a fixed folder enum, never a path: the server
// resolves the kind against its configured directories, so a caller can never
// direct Explorer (or the process launcher) at an arbitrary location.
func (s *Server) openSystemFolder(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Kind string `json:"kind"`
	}
	// decode enforces DisallowUnknownFields, so smuggled extras such as a raw
	// path key are rejected; every malformed shape here is an invalid kind.
	if err := decode(r, &input); err != nil {
		fail(w, 400, "INVALID_FOLDER_KIND", "kind must be data, reports, logs, or tools.")
		return
	}
	dir, ok := folderForKind(s.paths, input.Kind)
	if !ok {
		fail(w, 400, "INVALID_FOLDER_KIND", "kind must be data, reports, logs, or tools.")
		return
	}
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) || err == nil && !info.IsDir() {
		fail(w, 409, "FOLDER_NOT_FOUND", "This folder has not been created yet.")
		return
	}
	if err != nil {
		fail(w, 500, "FOLDER_OPEN_FAILED", "This folder could not be opened.")
		return
	}
	if err := s.openFolder(dir); err != nil {
		s.log.Warn("open folder failed", "kind", input.Kind, "error", err)
		fail(w, 500, "FOLDER_OPEN_FAILED", "Windows Explorer could not open this folder.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// folderForKind resolves the fixed folder enum to server-held config paths.
func folderForKind(paths config.Paths, kind string) (string, bool) {
	switch kind {
	case "data":
		return paths.DataDir, true
	case "reports":
		return paths.ReportsDir, true
	case "logs":
		return paths.LogsDir, true
	case "tools":
		return paths.ToolsDir, true
	}
	return "", false
}

// openFolderInExplorer launches Explorer on dir with a direct argument vector
// (no shell), so directories containing spaces are safe by construction.
// explorer.exe routinely exits nonzero (1) even after a successful hand-off to
// the shell; process.Run reports start and timeout failures as errors and exit
// codes as successful results, so that nonzero exit is correctly ignored.
func openFolderInExplorer(dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), openFolderTimeout)
	defer cancel()
	_, err := process.Run(ctx, process.Request{Command: "explorer.exe", Args: []string{dir}})
	return err
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
	writeJSON(w, 200, result)
}
func (s *Server) tree(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	relative := r.URL.Query().Get("path")
	if _, err := workspace.ValidateRelativePath(work.RootPath, relative); err != nil {
		fail(w, 400, "INVALID_PATH", err.Error())
		return
	}
	patterns, _ := s.userExcludes(r.Context(), work.ID)
	files, err := discovery.Tree(r.Context(), work.RootPath, relative, patterns)
	if err != nil {
		fail(w, 500, "DISCOVERY_FAILED", "Could not read workspace tree.")
		return
	}
	overrides, err := s.db.PathOverrides(r.Context(), work.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load file selections.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": treeItems(files, overrides), "path": relative})
}

type treeItem struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Included    bool   `json:"included"`
	Partial     bool   `json:"partial,omitempty"`
	HasChildren bool   `json:"has_children,omitempty"`
}

func treeItems(files []core.FileEntry, overrides []core.PathOverride) []treeItem {
	result := make([]treeItem, 0, len(files))
	for _, file := range files {
		path := filepath.ToSlash(file.RelativePath)
		item := treeItem{Path: path, Name: filepath.Base(filepath.FromSlash(path)), Included: pathIncluded(path, overrides)}
		if file.IsDir {
			item.Type = "directory"
			item.HasChildren = true
			item.Partial = pathPartiallyIncluded(path, item.Included, overrides)
		} else {
			item.Type = "file"
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Type != result[j].Type {
			return result[i].Type == "directory"
		}
		return strings.ToLower(result[i].Path) < strings.ToLower(result[j].Path)
	})
	return result
}

func pathPartiallyIncluded(path string, included bool, overrides []core.PathOverride) bool {
	prefix := strings.Trim(filepath.ToSlash(path), "/") + "/"
	for _, override := range overrides {
		if strings.HasPrefix(strings.Trim(filepath.ToSlash(override.RelativePath), "/"), prefix) && (override.Mode == "include") != included {
			return true
		}
	}
	return false
}

func pathIncluded(path string, overrides []core.PathOverride) bool {
	path = strings.Trim(filepath.ToSlash(path), "/")
	bestLength, mode := -1, ""
	for _, override := range overrides {
		overridePath := strings.Trim(filepath.ToSlash(override.RelativePath), "/")
		if overridePath != "" && (path == overridePath || strings.HasPrefix(path, overridePath+"/")) && len(overridePath) > bestLength {
			bestLength, mode = len(overridePath), override.Mode
		}
	}
	return mode != "exclude"
}

func (s *Server) getPathOverrides(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	items, err := s.db.PathOverrides(r.Context(), work.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load file selections.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) putPathOverrides(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	var input struct {
		Overrides []core.PathOverride `json:"overrides"`
	}
	if err := decode(r, &input); err != nil {
		fail(w, 400, "INVALID_JSON", "Invalid file selections.")
		return
	}
	seen := map[string]bool{}
	for index := range input.Overrides {
		override := &input.Overrides[index]
		if override.Mode != "include" && override.Mode != "exclude" {
			fail(w, 400, "INVALID_OVERRIDE", "Each selection must be include or exclude.")
			return
		}
		path, err := workspace.ValidateRelativePath(work.RootPath, override.RelativePath)
		if err != nil || path == "" {
			fail(w, 400, "INVALID_OVERRIDE", "Each selection must be an existing workspace-relative path.")
			return
		}
		override.WorkspaceID = work.ID
		override.RelativePath = filepath.ToSlash(path)
		if seen[override.RelativePath] {
			fail(w, 400, "INVALID_OVERRIDE", "Duplicate file selections are not allowed.")
			return
		}
		seen[override.RelativePath] = true
	}
	if err := s.db.ReplacePathOverrides(r.Context(), work.ID, input.Overrides); err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not save file selections.")
		return
	}
	s.getPathOverrides(w, r)
}
func (s *Server) getRules(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	rules, err := s.db.Rules(r.Context(), work.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load rules.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": rules})
}
func (s *Server) putRules(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	var input struct {
		Rules []struct {
			RuleType string `json:"rule_type"`
			Pattern  string `json:"pattern"`
			Enabled  *bool  `json:"enabled"`
		} `json:"rules"`
	}
	if err := decode(r, &input); err != nil {
		fail(w, 400, "INVALID_JSON", "Invalid rules.")
		return
	}
	rules := make([]core.WorkspaceRule, 0, len(input.Rules))
	for _, item := range input.Rules {
		if item.RuleType != "include" && item.RuleType != "exclude" || strings.TrimSpace(item.Pattern) == "" {
			fail(w, 400, "INVALID_RULE", "Each rule requires include/exclude and a pattern.")
			return
		}
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		rules = append(rules, core.WorkspaceRule{WorkspaceID: work.ID, RuleType: item.RuleType, Pattern: filepath.ToSlash(item.Pattern), Enabled: enabled})
	}
	if err := s.db.ReplaceUserRules(r.Context(), work.ID, rules); err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not save rules.")
		return
	}
	s.getRules(w, r)
}
func (s *Server) listScans(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	items, err := s.db.Scans(r.Context(), work.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not list scans.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
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
	writeJSON(w, 200, map[string]any{"scans": items, "total": total, "summary": summary})
}

// recentScanFilter parses the global scan list controls: limit defaults to 10,
// is rejected below 1, and is clamped to 50; state optionally filters by a
// single lifecycle state.
func recentScanFilter(r *http.Request) (database.RecentScansFilter, error) {
	query := r.URL.Query()
	filter := database.RecentScansFilter{Limit: database.DefaultRecentScansLimit, State: strings.ToLower(strings.TrimSpace(query.Get("state")))}
	if filter.State != "" && !knownScanState(filter.State) {
		return filter, fmt.Errorf("state must be a known scan state such as completed or running")
	}
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return filter, fmt.Errorf("limit must be a positive integer of at most 50")
		}
		filter.Limit = value
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
		Profile string `json:"profile"`
	}
	if err := decode(r, &input); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
		fail(w, 400, "INVALID_JSON", "Invalid scan request.")
		return
	}
	if input.Profile != "" && input.Profile != "quick" && input.Profile != "standard" && input.Profile != "deep" {
		fail(w, 400, "INVALID_PROFILE", "profile must be quick, standard, or deep.")
		return
	}
	scans, err := s.db.Scans(r.Context(), work.ID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not check active scan.")
		return
	}
	for _, scan := range scans {
		if !terminalScanState(scan.State) {
			writeJSON(w, 409, map[string]any{"error": map[string]any{"code": "SCAN_ALREADY_ACTIVE", "message": "A scan is already active.", "details": map[string]string{"scan_id": scan.ID}}})
			return
		}
	}
	if s.scans == nil {
		fail(w, 503, "SCAN_ENGINE_UNAVAILABLE", "The scan engine is unavailable.")
		return
	}
	patterns, _ := s.userExcludes(r.Context(), work.ID)
	scan, err := s.scans.DiscoverAndStart(r.Context(), work, input.Profile, patterns)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not create scan.")
		return
	}
	writeJSON(w, 202, scan)
}
func terminalScanState(state string) bool {
	return state == "completed" || state == "completed_with_warnings" || state == "failed" || state == "cancelled" || state == "interrupted"
}
func (s *Server) getScan(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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
	writeJSON(w, 200, struct {
		core.Scan
		AnalyzerRuns []map[string]any `json:"analyzer_runs"`
	}{Scan: scan, AnalyzerRuns: analyzerRuns(runs)})
}

func analyzerRuns(runs []reports.Run) []map[string]any {
	items := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		items = append(items, map[string]any{
			"analyzer_id": run.AnalyzerID,
			"status":      run.State,
			"version":     run.Version,
			"message":     run.ErrorSummary,
			"duration_ms": run.Duration.Milliseconds(),
		})
	}
	return items
}
func (s *Server) cancelScan(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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

func (s *Server) findings(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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
	filter, err := findingFilter(r)
	if err != nil {
		fail(w, 400, "INVALID_FINDING_QUERY", err.Error())
		return
	}
	page, err := s.db.FindingsPage(r.Context(), scan, filter)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load findings.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": page.Items, "total": page.Total, "limit": page.Limit, "offset": page.Offset, "next_offset": page.NextOffset, "has_more": page.HasMore})
}

// csvBOM prefixes CSV exports so Excel on Windows detects UTF-8 and renders
// non-ASCII finding text correctly instead of mangling it.
const csvBOM = "\xef\xbb\xbf"

// findingsCSVHeader is the fixed export column order; findingsCSVHeader and
// the row writer below must stay in sync.
var findingsCSVHeader = []string{"severity", "category", "analyzer", "rule_id", "title", "message", "file", "line", "column", "end_line", "status", "remediation", "documentation_url"}

// findingsCSV exports the scan's findings as a spreadsheet-friendly CSV
// attachment. It shares the JSON endpoint's filter parsing so an export always
// matches what the findings list shows; limit and offset are ignored because
// every matching row is written, up to the repository safety cap.
func (s *Server) findingsCSV(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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
	filter, err := findingQueryFilter(r)
	if err != nil {
		fail(w, 400, "INVALID_FINDING_QUERY", err.Error())
		return
	}
	findings, err := s.db.FindingsCSV(r.Context(), scan, filter)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load findings.")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="bluntcode-scan-%s-findings.csv"`, shortID(scan.ID)))
	_, _ = w.Write([]byte(csvBOM))
	writer := csv.NewWriter(w)
	_ = writer.Write(findingsCSVHeader)
	for _, f := range findings {
		_ = writer.Write([]string{
			string(f.Severity), string(f.Category), f.AnalyzerID, f.RuleID, f.Title, f.Message, f.RelativePath,
			csvNumber(f.StartLine), csvNumber(f.StartColumn), csvNumber(f.EndLine), f.Status, f.Remediation, f.DocumentationURL,
		})
	}
	writer.Flush()
}

// csvNumber renders optional finding positions; zero means "not stored" and
// exports as an empty cell.
func csvNumber(value int) string {
	if value == 0 {
		return ""
	}
	return strconv.Itoa(value)
}

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

// findingPreview returns a small, current-source excerpt. The stored path is
// validated again so a finding can never be used to read outside its workspace.
func (s *Server) findingPreview(w http.ResponseWriter, r *http.Request) {
	scanID, findingID := r.PathValue("id"), r.PathValue("findingID")
	if !validID(scanID) || !validID(findingID) {
		fail(w, 404, "FINDING_NOT_FOUND", "Finding was not found.")
		return
	}
	scan, err := s.db.Scan(r.Context(), scanID)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "SCAN_NOT_FOUND", "Scan was not found.")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load scan.")
		return
	}
	finding, err := s.db.Finding(r.Context(), scanID, findingID)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "FINDING_NOT_FOUND", "Finding was not found.")
		return
	}
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load finding.")
		return
	}
	work, err := s.db.Workspace(r.Context(), scan.WorkspaceID)
	if err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not load workspace.")
		return
	}
	relative, err := workspace.ValidateRelativePath(work.RootPath, filepath.FromSlash(finding.RelativePath))
	if err != nil {
		fail(w, 422, "SOURCE_PATH_UNAVAILABLE", "The finding path is no longer available inside this workspace.")
		return
	}
	path := filepath.Join(work.RootPath, relative)
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		fail(w, 404, "SOURCE_FILE_NOT_FOUND", "This source file no longer exists.")
		return
	}
	if err != nil {
		fail(w, 422, "SOURCE_PATH_UNAVAILABLE", "This source file cannot be opened.")
		return
	}
	if !info.Mode().IsRegular() {
		fail(w, 422, "SOURCE_NOT_A_FILE", "Only source files can be previewed.")
		return
	}
	if info.Size() > maxPreviewFileBytes {
		fail(w, 413, "SOURCE_FILE_TOO_LARGE", "This source file is too large to preview.")
		return
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		fail(w, 422, "SOURCE_PATH_UNAVAILABLE", "This source file cannot be opened.")
		return
	}
	lines := strings.Split(strings.ReplaceAll(string(contents), "\r\n", "\n"), "\n")
	start, end, note := finding.StartLine, finding.EndLine, ""
	if start < 1 || start > len(lines) {
		start, end = 0, 0
		note = "This finding has no saved location; showing the start of the current file."
	} else if end < start || end > len(lines) {
		end = start
	}
	first := 1
	if start > 0 {
		first = start - 6
		if first < 1 {
			first = 1
		}
	}
	last := first + 17
	if end > 0 && end+12 > last {
		last = end + 12
	}
	if last > len(lines) {
		last = len(lines)
	}
	preview := sourcePreview{Path: filepath.ToSlash(relative), HighlightStartLine: start, HighlightEndLine: end, Note: note, Lines: make([]sourcePreviewLine, 0, last-first+1)}
	for index := first - 1; index < last; index++ {
		preview.Lines = append(preview.Lines, sourcePreviewLine{Number: index + 1, Text: strings.TrimSuffix(lines[index], "\r")})
	}
	writeJSON(w, http.StatusOK, preview)
}

// findingQueryFilter parses the finding controls shared by the JSON list and
// the CSV export: severity, category, analyzer, path, status, q, sort, and
// order. Both endpoints reject the same bad input because they parse through
// this one function.
func findingQueryFilter(r *http.Request) (database.FindingFilter, error) {
	query := r.URL.Query()
	read := func(name string) string { return strings.TrimSpace(query.Get(name)) }
	filter := database.FindingFilter{
		Severity: strings.ToLower(read("severity")),
		Category: strings.ToLower(read("category")),
		Analyzer: strings.ToLower(read("analyzer")),
		Path:     read("path"),
		Status:   strings.ToLower(read("status")),
		Query:    read("q"),
		Sort:     strings.ToLower(read("sort")),
		Order:    strings.ToLower(read("order")),
	}
	if filter.Severity != "" && !oneOf(filter.Severity, "critical", "high", "medium", "low", "info") {
		return filter, fmt.Errorf("severity must be critical, high, medium, low, or info")
	}
	if filter.Status != "" && !oneOf(filter.Status, "new", "persistent") {
		return filter, fmt.Errorf("status must be new or persistent")
	}
	if filter.Sort != "" && !oneOf(filter.Sort, "severity", "path", "analyzer", "status") {
		return filter, fmt.Errorf("sort must be severity, path, analyzer, or status")
	}
	if filter.Order != "" && !oneOf(filter.Order, "asc", "desc") {
		return filter, fmt.Errorf("order must be asc or desc")
	}
	return filter, nil
}

// findingFilter extends the shared controls with the JSON list's pagination.
func findingFilter(r *http.Request) (database.FindingFilter, error) {
	filter, err := findingQueryFilter(r)
	if err != nil {
		return filter, err
	}
	filter.Limit = 50
	query := r.URL.Query()
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return filter, fmt.Errorf("limit must be 25, 50, or 100")
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
	if !oneOf(strconv.Itoa(filter.Limit), "25", "50", "100") {
		return filter, fmt.Errorf("limit must be 25, 50, or 100")
	}
	return filter, nil
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func (s *Server) reportModel(ctx context.Context, scan core.Scan, work core.Workspace) (reports.Model, error) {
	findings, err := s.db.Findings(ctx, scan.ID)
	if err != nil {
		return reports.Model{}, err
	}
	metrics, err := s.db.Metrics(ctx, scan.ID)
	if err != nil {
		return reports.Model{}, err
	}
	runs, err := s.db.AnalyzerRuns(ctx, scan.ID)
	if err != nil {
		return reports.Model{}, err
	}
	comparison := scans.Comparison{}
	if previousID, previousErr := s.db.PreviousCompletedScanID(ctx, scan.WorkspaceID, scan.ID); previousErr == nil {
		previous, previousFindingsErr := s.db.Findings(ctx, previousID)
		succeeded, succeededErr := s.db.SuccessfulAnalyzerIDs(ctx, scan.ID)
		if previousFindingsErr == nil && succeededErr == nil {
			comparison = scans.Compare(findings, previous, succeeded)
		}
	}
	startedAt := scan.StartedAt
	if startedAt == nil && scan.Snapshot != nil {
		startedAt = &scan.Snapshot.CapturedAt
	}
	files := []string(nil)
	bluntCodeVersion := ""
	if scan.Snapshot != nil {
		files = append(files, scan.Snapshot.SelectedFiles...)
		bluntCodeVersion = scan.Snapshot.BluntCodeVersion
	}
	if len(files) == 0 && scan.SelectedFileCount > 0 {
		files = make([]string, scan.SelectedFileCount)
	}
	skippedCount := scan.CandidateFileCount - scan.SelectedFileCount
	if skippedCount < 0 {
		skippedCount = 0
	}
	return reports.Build(reports.Input{
		WorkspaceName: work.Name, WorkspacePath: work.RootPath, ScanID: scan.ID, Profile: scan.Profile, BluntCodeVersion: bluntCodeVersion,
		StartedAt: startedAtValue(startedAt), FinishedAt: finishedAtValue(scan.FinishedAt), Files: files, SkippedFiles: make([]string, skippedCount), Findings: findings, Metrics: metrics, Runs: runs,
		Comparison: reports.Comparison{New: comparison.New, Fixed: comparison.Fixed, Persistent: comparison.Persistent, UnknownAnalyzerIDs: comparison.UnknownAnalyzerIDs},
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
	id := r.PathValue("id")
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
	id := r.PathValue("id")
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
	w.Header().Set("Content-Disposition", `attachment; filename="blunt-code-report.md"`)
	_, _ = w.Write([]byte(reports.Markdown(model)))
}

// reportSARIF exports the scan as a SARIF 2.1.0 attachment, the interchange
// format understood by VS Code, GitHub code scanning, and other standard
// tooling. Like the JSON report it is regenerated from stored scan data, not
// from a persisted artifact.
func (s *Server) reportSARIF(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="bluntcode-scan-%s.sarif"`, shortID(scan.ID)))
	_ = json.NewEncoder(w).Encode(reports.SARIF(model))
}

// reportHTML exports the scan as a standalone HTML document: one
// self-contained file with no external assets or scripts that can be shared,
// archived, or printed as-is. Like the other exporters it is regenerated from
// stored scan data, not from a persisted artifact.
func (s *Server) reportHTML(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="bluntcode-scan-%s.html"`, shortID(scan.ID)))
	_, _ = w.Write(reports.HTML(model))
}

// shortID keeps attachment filenames readable while staying scan-unique.
func shortID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

func (s *Server) listTools(w http.ResponseWriter, r *http.Request) {
	if s.tools == nil {
		fail(w, 503, "TOOLS_UNAVAILABLE", "Tool service is unavailable.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": s.tools.All()})
}
func (s *Server) installTool(w http.ResponseWriter, r *http.Request) {
	if s.tools == nil {
		fail(w, 503, "TOOLS_UNAVAILABLE", "Tool service is unavailable.")
		return
	}
	id := r.PathValue("id")
	if id != "ruff" && id != "biome" && id != "semgrep" && id != "sonarqube" {
		fail(w, 404, "TOOL_NOT_FOUND", "Tool was not found.")
		return
	}
	if err := s.tools.Ensure(r.Context(), id); err != nil {
		fail(w, 409, "TOOL_NOT_READY", err.Error())
		return
	}
	writeJSON(w, 200, s.tools.Status(id))
}
func (s *Server) scanEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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
	for {
		select {
		case event := <-ch:
			body, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, body)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			fail(w, 421, "LOCAL_ONLY", "Blunt Code accepts loopback requests only.")
			return
		}
		if isStateChanging(r.Method) && !safeOrigin(r) {
			fail(w, 403, "CROSS_ORIGIN_FORBIDDEN", "State-changing requests must come from the local Blunt Code origin.")
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
func loopbackHost(host string) bool {
	hostname, _, err := net.SplitHostPort(host)
	if err != nil {
		hostname = host
	}
	hostname = strings.Trim(hostname, "[]")
	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
}
func isStateChanging(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}
func safeOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme != "http" {
		return false
	}
	return loopbackHost(u.Host)
}

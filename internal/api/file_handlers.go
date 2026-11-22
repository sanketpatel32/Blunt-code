// file handlers handlers.
package api

import (
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bluntcode/internal/core"
	"bluntcode/internal/discovery"
	"bluntcode/internal/workspace"
)

func (s *Server) tree(w http.ResponseWriter, r *http.Request) {
	work, ok := s.workspace(r)
	if !ok {
		fail(w, 404, "WORKSPACE_NOT_FOUND", "Workspace was not found.")
		return
	}
	relative := r.URL.Query().Get("path")
	// A workspace whose root directory was deleted (or moved) would otherwise
	// surface as a 500 DISCOVERY_FAILED on the empty path while subpaths 400
	// through path validation; report the real condition instead.
	if s.failRootUnavailable(w, work) {
		return
	}
	if _, err := workspace.ValidateRelativePath(work.RootPath, relative); err != nil {
		fail(w, 400, "INVALID_PATH", err.Error())
		return
	}
	cacheKey := work.ID + "|" + relative
	if cached, ok := s.treeCache.Load(cacheKey); ok {
		if entry := cached.(cachedTree); time.Now().Before(entry.expires) {
			writeJSON(w, 200, entry.payload)
			return
		}
		s.treeCache.Delete(cacheKey)
	}
	patterns, err := s.userExcludes(r.Context(), work.ID)
	if err != nil {
		// A failed rules read must not silently present an unfiltered tree.
		fail(w, 500, "DATABASE_ERROR", "Could not load exclusion rules.")
		return
	}
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
	payload := map[string]any{"items": treeItems(files, overrides), "path": relative}
	s.treeCache.Store(cacheKey, cachedTree{payload: payload, expires: time.Now().Add(30 * time.Second)})
	writeJSON(w, 200, payload)
}

func treeItems(files []core.FileEntry, overrides []core.PathOverride) []treeItem {
	result := make([]treeItem, 0, len(files))
	for _, file := range files {
		path := filepath.ToSlash(file.RelativePath)
		item := treeItem{Path: path, Name: filepath.Base(filepath.FromSlash(path)), Included: pathIncluded(path, overrides), Language: file.Language}
		if file.IsDir {
			item.Type = "directory"
			item.HasChildren = true
			item.Partial = pathPartiallyIncluded(path, item.Included, overrides)
		} else {
			item.Type = "file"
		}
		if !item.Included {
			item.ExcludedReason = discovery.SkipUserExcluded
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
		pattern := strings.TrimSpace(item.Pattern)
		if item.RuleType != "include" && item.RuleType != "exclude" || pattern == "" {
			fail(w, 400, "INVALID_RULE", "Each rule requires include/exclude and a pattern.")
			return
		}
		if len(pattern) > 512 || strings.Contains(pattern, "..") && (strings.Contains(pattern, ".."+string(filepath.Separator)) || strings.Contains(pattern, "/..") || pattern == "..") {
			fail(w, 400, "INVALID_RULE", "Pattern must not contain directory traversal.")
			return
		}
		if filepath.IsAbs(pattern) || strings.Contains(pattern, ":") {
			fail(w, 400, "INVALID_RULE", "Pattern must be relative and must not contain drive letters.")
			return
		}
		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}
		rules = append(rules, core.WorkspaceRule{WorkspaceID: work.ID, RuleType: item.RuleType, Pattern: filepath.ToSlash(pattern), Enabled: enabled})
	}
	if err := s.db.ReplaceUserRules(r.Context(), work.ID, rules); err != nil {
		fail(w, 500, "DATABASE_ERROR", "Could not save rules.")
		return
	}
	s.getRules(w, r)
}

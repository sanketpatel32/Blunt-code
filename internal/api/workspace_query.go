package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"bluntcode/internal/database"
)

// workspaceListQuery parses GET /api/v1/workspaces paging and filter query params.
func workspaceListQuery(r *http.Request) (page, pageSize int, filter database.WorkspacePageFilter, err error) {
	query := r.URL.Query()
	page = 1
	if raw := strings.TrimSpace(query.Get("page")); raw != "" {
		value, convErr := strconv.Atoi(raw)
		if convErr != nil || value < 1 {
			return 0, 0, filter, fmt.Errorf("page must be a positive integer")
		}
		page = value
	}
	pageSize = database.DefaultWorkspacePageSize
	if raw := strings.TrimSpace(query.Get("page_size")); raw != "" {
		value, convErr := strconv.Atoi(raw)
		if convErr != nil || value < 1 || value > database.MaxWorkspacePageSize {
			return 0, 0, filter, fmt.Errorf("page_size must be between 1 and %d", database.MaxWorkspacePageSize)
		}
		pageSize = value
	}
	sortKey := strings.TrimSpace(query.Get("sort"))
	order := strings.TrimSpace(query.Get("order"))
	switch strings.ToLower(sortKey) {
	case "", "last_opened", "last_scan", "findings", "name":
		// ok
	default:
		return 0, 0, filter, fmt.Errorf("sort must be name, last_opened, last_scan, or findings")
	}
	if order != "" && strings.ToLower(order) != "asc" && strings.ToLower(order) != "desc" {
		return 0, 0, filter, fmt.Errorf("order must be asc or desc")
	}
	filter = database.WorkspacePageFilter{
		Query: strings.TrimSpace(query.Get("search")),
		Tag:   strings.TrimSpace(query.Get("tag")),
		Sort:  strings.ToLower(sortKey),
		Order: strings.ToLower(order),
	}
	return page, pageSize, filter, nil
}

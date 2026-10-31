package database

import (
	"context"
	"database/sql"
	"time"

	"bluntcode/internal/core"
)

// DefaultWorkspaceSparkline is how many completed scans each workspace card's
// sparkline carries in GET /api/v1/workspaces.
const DefaultWorkspaceSparkline = 8

// RecentCompletedTotals returns, for every workspace with at least one
// producing scan (completed or completed_with_warnings), the total_findings of
// its newest limit such scans ordered oldest first, so the series reads left
// to right. Failed, cancelled, interrupted, and in-flight scans never appear:
// a failed scan's zero is "no data", not "no findings". One windowed query
// serves every workspace instead of a per-card trend request.
func (d *DB) RecentCompletedTotals(ctx context.Context, limit int) (map[string][]int, error) {
	if limit < 1 {
		limit = DefaultWorkspaceSparkline
	}
	rows, err := d.SQL.QueryContext(ctx, `SELECT workspace_id,total FROM (
		SELECT workspace_id,COALESCE(total_findings,0) AS total,COALESCE(finished_at,started_at) AS at,id,ROW_NUMBER() OVER (PARTITION BY workspace_id ORDER BY COALESCE(finished_at,started_at) DESC,id DESC) AS recency
		FROM scans WHERE state IN ('completed','completed_with_warnings')
	) ranked WHERE recency<=? ORDER BY workspace_id, at ASC, id ASC`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]int{}
	for rows.Next() {
		var id string
		var total int
		if err := rows.Scan(&id, &total); err != nil {
			return nil, err
		}
		out[id] = append(out[id], total)
	}
	return out, rows.Err()
}

// ScanDay is one calendar day of scan activity: every scan started that day
// in loc, and how many of them produced a result.
type ScanDay struct {
	Date      string `json:"date"`
	Total     int    `json:"total"`
	Completed int    `json:"completed"`
}

// ScansByDay buckets the scans started during the last days calendar days
// (today included, in loc) by local date, oldest first. Every day is present,
// zero-filled, so the overview strip always has a fixed width.
func (d *DB) ScansByDay(ctx context.Context, now time.Time, loc *time.Location, days int) ([]ScanDay, error) {
	if days < 1 {
		days = 7
	}
	if loc == nil {
		loc = time.Local
	}
	local := now.In(loc)
	first := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(days - 1))
	out := make([]ScanDay, days)
	index := map[string]int{}
	for i := range out {
		key := first.AddDate(0, 0, i).Format("2006-01-02")
		out[i] = ScanDay{Date: key}
		index[key] = i
	}
	// started_at is RFC3339 UTC text; a day of slack on the lower bound keeps
	// the string comparison safe across zones, and Go does the exact bucketing.
	since := first.Add(-24 * time.Hour).UTC().Format(time.RFC3339Nano)
	rows, err := d.SQL.QueryContext(ctx, `SELECT started_at,state FROM scans WHERE started_at IS NOT NULL AND started_at>=?`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var started sql.NullString
		var state string
		if err := rows.Scan(&started, &state); err != nil {
			return nil, err
		}
		at, err := parseNullableTime(started)
		if err != nil || at == nil {
			continue
		}
		i, ok := index[at.In(loc).Format("2006-01-02")]
		if !ok {
			continue
		}
		out[i].Total++
		if state == "completed" || state == "completed_with_warnings" {
			out[i].Completed++
		}
	}
	return out, rows.Err()
}

// FindingFacets counts the scan's findings by category and by analyzer under
// the same non-status filters the findings list applies, so a scan report can
// chart "issues by category" correctly past the first page of rows.
type FindingFacets struct {
	Category map[string]int `json:"category"`
	Analyzer map[string]int `json:"analyzer"`
}

func (d *DB) FindingFacets(ctx context.Context, scan core.Scan, filter FindingFilter) (FindingFacets, error) {
	filter.Category, filter.Analyzer = "", ""
	filter.Sort, filter.Order = "", ""
	query, err := d.buildFindingQuery(ctx, scan, filter)
	if err != nil {
		return FindingFacets{}, err
	}
	args := query.args
	facets := FindingFacets{Category: map[string]int{}, Analyzer: map[string]int{}}
	for _, facet := range []struct {
		column string
		target map[string]int
	}{{"findings.category", facets.Category}, {"findings.analyzer_id", facets.Analyzer}} {
		column, target := facet.column, facet.target
		rows, err := d.SQL.QueryContext(ctx, `SELECT `+column+`, COUNT(*) FROM findings WHERE `+query.whereSQL+` GROUP BY `+column, args...)
		if err != nil {
			return FindingFacets{}, err
		}
		for rows.Next() {
			var key string
			var n int
			if err := rows.Scan(&key, &n); err != nil {
				rows.Close()
				return FindingFacets{}, err
			}
			target[key] += n
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return FindingFacets{}, err
		}
		rows.Close()
	}
	return facets, nil
}

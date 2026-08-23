// Package core contains analyzer-independent application models.
package core

import "time"

type Workspace struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	RootPath          string         `json:"root_path"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	LastOpenedAt      *time.Time     `json:"last_opened_at,omitempty"`
	LastScanAt        *time.Time     `json:"last_scan_at,omitempty"`
	DefaultProfile    string         `json:"default_profile"`
	SettingsJSON      string         `json:"-"`
	DetectedLanguages map[string]int `json:"detected_languages,omitempty"`
	GitRepository     bool           `json:"git_repository"`
}

type WorkspaceRule struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	RuleType    string    `json:"rule_type"`
	Pattern     string    `json:"pattern"`
	Source      string    `json:"source"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

type PathOverride struct {
	WorkspaceID  string `json:"workspace_id"`
	RelativePath string `json:"relative_path"`
	Mode         string `json:"mode"`
}

// Suppression records one dismissed finding fingerprint for a workspace (the
// "ignore this finding forever" workflow). Suppressed findings keep being
// stored by future scans, but they are excluded from scan totals, severity
// counts, reports, exports, and the CI gate; the findings list still shows
// them with the status "suppressed".
type Suppression struct {
	WorkspaceID string    `json:"workspace_id"`
	Fingerprint string    `json:"fingerprint"`
	Reason      string    `json:"reason,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type Scan struct {
	ID                 string        `json:"id"`
	WorkspaceID        string        `json:"workspace_id"`
	State              string        `json:"state"`
	Profile            string        `json:"profile"`
	StartedAt          *time.Time    `json:"started_at,omitempty"`
	FinishedAt         *time.Time    `json:"finished_at,omitempty"`
	CandidateFileCount int           `json:"candidate_file_count"`
	SelectedFileCount  int           `json:"selected_file_count"`
	TotalFindings      int           `json:"total_findings"`
	ErrorSummary       string        `json:"error_summary,omitempty"`
	Snapshot           *ScanSnapshot `json:"snapshot,omitempty"`
	SnapshotJSON       string        `json:"-"`
}

// ScanSnapshot is captured once, before any analyzer is started. It is kept
// separate from mutable workspace settings so historical reports remain
// reproducible and understandable.
type ScanSnapshot struct {
	WorkspaceID        string            `json:"workspace_id"`
	WorkspaceRoot      string            `json:"workspace_root"`
	WorkspaceName      string            `json:"workspace_name"`
	CapturedAt         time.Time         `json:"captured_at"`
	BluntCodeVersion   string            `json:"bluntcode_version"`
	Profile            string            `json:"profile"`
	CandidateFileCount int               `json:"candidate_file_count"`
	SelectedFileCount  int               `json:"selected_file_count"`
	SelectedFiles      []string          `json:"selected_files"`
	Languages          map[string]int    `json:"languages"`
	Rules              []WorkspaceRule   `json:"rules"`
	Exclusions         []string          `json:"exclusions"`
	PathOverrides      []PathOverride    `json:"path_overrides"`
	EnabledAnalyzers   []string          `json:"enabled_analyzers"`
	AnalyzerVersions   map[string]string `json:"analyzer_versions"`
	Git                ScanGitSnapshot   `json:"git"`
}

type ScanGitSnapshot struct {
	Repository bool   `json:"repository"`
	Branch     string `json:"branch,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Dirty      bool   `json:"dirty"`
}

type FileEntry struct {
	RelativePath string `json:"relative_path"`
	Language     string `json:"language,omitempty"`
	SizeBytes    int64  `json:"size_bytes"`
	Selected     bool   `json:"selected"`
	SkipReason   string `json:"skip_reason,omitempty"`
	IsDir        bool   `json:"is_dir"`
}

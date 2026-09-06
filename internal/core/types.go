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
	// Severity split persisted once by CompleteScan; zero until the scan
	// finishes. Suppressed findings are excluded, so the split can disagree
	// with a raw findings query on the same scan.
	CriticalCount int           `json:"critical_count"`
	HighCount     int           `json:"high_count"`
	MediumCount   int           `json:"medium_count"`
	LowCount      int           `json:"low_count"`
	InfoCount     int           `json:"info_count"`
	ErrorSummary  string        `json:"error_summary,omitempty"`
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
	// SkipCounts explains the discovery walk's coverage by reason (symlink,
	// excluded_default, excluded_user, outside_root, generated_content).
	SkipCounts map[string]int `json:"skip_counts,omitempty"`
	// DependencyInputs lists the workspace-relative dependency manifests and
	// lockfiles discovery saw (lockfiles included even though smart-skip
	// keeps them out of SelectedFiles). Capped; empty means none were found.
	DependencyInputs []string `json:"dependency_inputs,omitempty"`
	// InputDigest digests the content hashes of every analyzed file, recorded
	// before analyzers start: it identifies the bytes a scan actually
	// describes. A recomputation at completion that disagrees means the
	// workspace drifted mid-scan (DriftDetected).
	InputDigest string `json:"input_digest,omitempty"`
	// ConfigDigest digests the rules, exclusions, and path overrides that
	// shaped the scan, so two scans are comparable only under the same
	// configuration.
	ConfigDigest string `json:"config_digest,omitempty"`
	// DependencyDigest digests DependencyInputs; dependency-analysis results
	// are only comparable when this matches.
	DependencyDigest string `json:"dependency_digest,omitempty"`
	// DiscoveryPolicyVersion is the discovery classifier's schema version;
	// a classification change between scans is visible instead of implied.
	DiscoveryPolicyVersion int `json:"discovery_policy_version,omitempty"`
	// FingerprintVersion is the finding-identity schema version in effect
	// when the scan's fingerprints were computed.
	FingerprintVersion int `json:"fingerprint_version,omitempty"`
	// Platform records os/arch/go of the machine that ran the scan.
	Platform map[string]string `json:"platform,omitempty"`
	// DriftDetected marks a scan whose workspace content changed between
	// start and completion; its results may mix two versions of the tree.
	DriftDetected bool `json:"drift_detected,omitempty"`
	Git           ScanGitSnapshot `json:"git"`
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

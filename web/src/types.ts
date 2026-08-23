export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'info';

export interface Workspace {
  id: string;
  name: string;
  root_path: string;
  languages?: string[];
  last_scan_at?: string | null;
  last_opened_at?: string | null;
  latest_scan?: Scan;
}

export interface Scan {
  id: string;
  workspace_id: string;
  state: string;
  profile?: string;
  started_at?: string | null;
  finished_at?: string | null;
  total_findings?: number;
  critical_count?: number;
  high_count?: number;
  medium_count?: number;
  low_count?: number;
  info_count?: number;
  new_count?: number;
  fixed_count?: number;
  duration_ms?: number;
  error_summary?: string | null;
  analyzer_runs?: AnalyzerRun[];
}

export interface AnalyzerRun {
  analyzer_id: string;
  status: string;
  version?: string;
  message?: string;
  duration_ms?: number;
}

export interface RecentScanItem {
  id: string;
  workspace_id: string;
  workspace_name: string;
  state: string;
  profile?: string;
  started_at?: string | null;
  finished_at?: string | null;
  candidate_file_count?: number;
  selected_file_count?: number;
  total_findings?: number;
  critical_count?: number;
  high_count?: number;
  medium_count?: number;
  low_count?: number;
  info_count?: number;
}

export interface ScanSummary {
  workspaces_total: number;
  workspaces_scanned: number;
  critical_count: number;
  high_count: number;
  medium_count: number;
  low_count: number;
  info_count: number;
  total_findings: number;
  scans_total: number;
  scans_last_7d: number;
  active_scans: number;
}

export interface RecentScansResponse {
  scans: RecentScanItem[];
  total: number;
  summary?: ScanSummary;
}

/** One completed scan on the workspace severity trend chart; points arrive oldest-first and `finished_at` falls back to the start time. */
export interface SeverityTrendPoint {
  scan_id: string;
  finished_at?: string | null;
  profile?: string;
  state: string;
  severity: { critical: number; high: number; medium: number; low: number; info: number };
  total: number;
}

export interface Finding {
  id: string;
  analyzer_id: string;
  rule_id?: string;
  fingerprint?: string;
  severity: Severity;
  category: string;
  title?: string;
  message: string;
  relative_path?: string;
  start_line?: number;
  start_column?: number;
  remediation?: string;
  documentation_url?: string;
  /** Computed per scan by the backend; `suppressed` marks a fingerprint dismissed for this workspace. */
  status?: 'new' | 'persistent' | 'suppressed';
}

/** One dismissed fingerprint on a workspace; `reason` is the optional human note (≤500 chars). */
export interface Suppression {
  workspace_id?: string;
  fingerprint: string;
  reason?: string;
  created_at: string;
}

export interface FindingPage {
  items: Finding[];
  total: number;
  limit: number;
  offset: number;
  next_offset?: number;
  has_more: boolean;
}

/** Findings fixed versus the previous completed scan; `fixed` is severity-ordered and capped at 100 while `total_fixed` is exact. */
export interface FixedFindingsResponse {
  fixed: Finding[];
  total_fixed: number;
  comparison_available: boolean;
  previous_scan_id: string | null;
}

export interface SourcePreview {
  path: string;
  lines: Array<{ number: number; text: string }>;
  highlight_start_line?: number;
  highlight_end_line?: number;
  note?: string;
}

export interface TreeNode {
  path: string;
  name: string;
  type: 'file' | 'directory';
  included?: boolean;
  excluded_reason?: string;
  partial?: boolean;
  has_children?: boolean;
  children?: TreeNode[];
}

export interface PathOverride {
  workspace_id?: string;
  relative_path: string;
  mode: 'include' | 'exclude';
}

export interface Tool {
  id: string;
  name?: string;
  version?: string;
  ready: boolean;
  can_install?: boolean;
  description?: string;
  detail?: string;
}

export interface Report {
  scan: Scan;
  workspace?: Workspace;
  findings?: Finding[];
  metrics?: Array<{ label: string; value_text?: string; value_number?: number; unit?: string }>;
  warnings?: string[];
  comparison?: { new_count?: number; fixed_count?: number; persistent_count?: number };
}

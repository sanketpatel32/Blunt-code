export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'info';

export interface Workspace {
  id: string;
  name: string;
  root_path: string;
  languages?: string[];
  /** Free-form project labels shown as chips on the workspaces page; the API omits the field until tagging ships server-side. */
  tags?: string[];
  /** Weighted risk rollup mirroring RiskProfile's grade bands; optional because no endpoint populates it yet — render defensively. */
  risk?: { grade: 'A' | 'B' | 'C' | 'D'; score: number };
  default_profile?: string;
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

/** One server-paged slice of a workspace's scan history (`GET /workspaces/{id}/scans?page=…`). */
export interface ScanPage {
  items: Scan[];
  total: number;
  page: number;
  page_size: number;
  has_next: boolean;
}

/** A finding from the cross-workspace search, enriched with its origin identity so results can link back to their report. */
export type SearchedFinding = Finding & { scan_id: string; workspace_id: string };

/** One page of `GET /api/v1/findings/search`. */
export interface SearchFindingsPage {
  items: SearchedFinding[];
  total: number;
  page: number;
  page_size: number;
  has_next: boolean;
}

/** Weighted risk score from `GET /workspaces/{id}/risk`. Weights: critical 10, high 5, medium 2, low 1. Grade A<5, B<20, C<50, D otherwise. */
export interface RiskProfile {
  available: boolean;
  scan_id?: string;
  score?: number;
  grade?: 'A' | 'B' | 'C' | 'D';
  trend?: 'up' | 'down' | 'flat';
  previous_score?: number;
  previous_scan_id?: string;
  counts?: Record<string, number>;
}

/** Tool readiness pair on the global overview; the whole field is absent when no tools service is wired into the server. */
export interface StatsTools {
  total: number;
  ready: number;
}

/** Global overview from GET /api/v1/stats. Findings roll up the latest completed scan per workspace (rescanning one project never double-counts it), `tools` is omitted when unwired, and `generated_at` is the RFC3339 snapshot time. */
export interface GlobalStats {
  workspaces: number;
  scans: { total: number; completed: number; running: number };
  findings: { severity: Record<Severity, number>; total: number };
  suppressions: number;
  tools?: StatsTools;
  generated_at: string;
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
  /** Present only when the request used `page`/`page_size`: the served page, its size, and whether another page follows. */
  page?: number;
  page_size?: number;
  has_next?: boolean;
}

/** Query for GET /scans/{id}/findings. `severity`/`status` accept comma lists, `analyzer`/`rule` match exactly (rule case-insensitively), `path` is a substring match and `path_prefix` anchors at the start, `page`/`page_size` (1..200) page the results (never mixed with legacy `limit`/`offset`), and `sort`/`order` pick the ordering (`sort` also accepts a `-` prefix for descending). */
export type FindingsQuery = Record<string, string> & {
  severity?: string;
  status?: string;
  analyzer?: string;
  rule?: string;
  path?: string;
  path_prefix?: string;
  page?: string;
  page_size?: string;
  sort?: string;
  order?: string;
};

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
  language?: string;
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

export interface PentestFindingItem {
  id: string;
  title: string;
  severity: 'critical' | 'high' | 'medium' | 'low' | 'info';
  category: string;
  owasp?: string;
  cwe?: string;
  message: string;
  remediation: string;
  evidence?: string;
}

export interface PentestProbeResult {
  target_url: string;
  status_code: number;
  response_time_ms: number;
  server?: string;
  powered_by?: string;
  grade: string;
  risk_score: number;
  headers_found: string[];
  headers_missing: string[];
  findings: PentestFindingItem[];
  probed_at: string;
}


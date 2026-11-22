import type { AnalyzerStatus, ScanPlan, CompareResult, FindingPage, FindingsQuery, FixedFindingsResponse, GlobalStats, PathOverride, PentestProbeResult, RecentScansResponse, Report, RiskProfile, Scan, ScanPage, SearchFindingsPage, SeverityTrendPoint, SourcePreview, Suppression, Tool, TreeNode, Workspace, WorkspaceListPage } from './types';

const PREFIX = '/api/v1';

export class ApiError extends Error {
  constructor(public readonly code: string, message: string, public readonly status?: number, /** Optional machine-readable payload from the error envelope, e.g. the active scan id on 409 SCAN_ALREADY_ACTIVE. */ public readonly details?: Record<string, unknown>) {
    super(message);
  }
}

/** Reads (GET) give up after this long so a wedged backend surfaces as an error instead of an endless skeleton. Mutations run unbounded: installs, scans and the native folder picker legitimately take minutes. */
export const READ_TIMEOUT_MS = 30_000;

/** Caller-supplied cancellation for reads, typically the AbortSignal useLoad hands to its loader. */
export type ReadOptions = { signal?: AbortSignal };

async function requestWithStatus<T>(path: string, init?: RequestInit): Promise<{ data: T; status: number }> {
  const method = (init?.method ?? 'GET').toUpperCase();
  const timeoutMs = method === 'GET' ? READ_TIMEOUT_MS : 0;
  const controller = new AbortController();
  const callerSignal = init?.signal ?? undefined;
  let timedOut = false;
  const onCallerAbort = () => controller.abort(callerSignal?.reason);
  if (callerSignal?.aborted) controller.abort(callerSignal.reason);
  else callerSignal?.addEventListener('abort', onCallerAbort, { once: true });
  const timer = timeoutMs > 0 ? setTimeout(() => { timedOut = true; controller.abort(); }, timeoutMs) : undefined;
  try {
    const response = await fetch(`${PREFIX}${path}`, {
      ...init,
      signal: controller.signal,
      headers: { Accept: 'application/json', ...(init?.body ? { 'Content-Type': 'application/json' } : {}), ...init?.headers },
    });
    if (!response.ok) {
      const body = await response.json().catch(() => null) as { error?: { code?: string; message?: string; details?: Record<string, unknown> } } | null;
      throw new ApiError(body?.error?.code ?? 'REQUEST_FAILED', body?.error?.message ?? `Request failed (${response.status})`, response.status, body?.error?.details);
    }
    if (response.status === 204) return { data: undefined as T, status: response.status };
    // An empty 200/201/202 body is "no payload", not a JSON parse crash.
    const text = await response.text();
    const data = (text.trim() ? JSON.parse(text) : null) as T;
    return { data, status: response.status };
  } catch (e) {
    // The deadline can land while headers or the body are still arriving; both surface as TIMEOUT.
    if (timedOut) throw new ApiError('TIMEOUT', `Request timed out after ${Math.round(timeoutMs / 1000)}s`);
    throw e;
  } finally {
    if (timer) clearTimeout(timer);
    callerSignal?.removeEventListener('abort', onCallerAbort);
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  return (await requestWithStatus<T>(path, init)).data;
}

/** List endpoints wrap their array in an envelope; a null body (or null envelope field) reads as "nothing yet". */
function list<T>(value: T[] | { items?: T[]; workspaces?: T[]; scans?: T[]; findings?: T[]; tools?: T[] } | null | undefined): T[] {
  if (Array.isArray(value)) return value;
  if (!value) return [];
  return value.items ?? value.workspaces ?? value.scans ?? value.findings ?? value.tools ?? [];
}

/** A workspace result from a create call plus `existing`: the backend answers 201
 *  for a freshly registered folder and 200 when it was already registered, so
 *  callers can greet a duplicate with "opening it" instead of "added". */
export type CreatedWorkspace = Workspace & { existing: boolean };

export type FindingNote = { id: string; text: string; createdAt: string };
export const api = {
  findingNotes: async (id: string, fingerprint: string, opts?: ReadOptions) => list<FindingNote>(await request<{ items: FindingNote[] }>(`/workspaces/${encodeURIComponent(id)}/notes/${encodeURIComponent(fingerprint)}`, opts)),
  addFindingNote: (id: string, fingerprint: string, text: string, importKey?: string) => request<FindingNote>(`/workspaces/${encodeURIComponent(id)}/notes/${encodeURIComponent(fingerprint)}`, { method: 'POST', body: JSON.stringify({ text, import_key: importKey }) }),
  deleteFindingNote: (id: string, fingerprint: string, note: string) => request<void>(`/workspaces/${encodeURIComponent(id)}/notes/${encodeURIComponent(fingerprint)}/${encodeURIComponent(note)}`, { method: 'DELETE' }),
  scanPlan: (id: string, profile: string, opts?: ReadOptions) => request<ScanPlan>(`/workspaces/${encodeURIComponent(id)}/scan-plan?profile=${encodeURIComponent(profile)}`, opts),
  health: () => request<{ status?: string }>('/health'),
  meta: () => request<{
    version: string;
    api_version: string;
    os: string;
    architecture: string;
    data_directory: string;
    demo_seed_env_enabled?: boolean;
    demo_seed_requires_token?: boolean;
  }>('/meta'),
  settings: () => request<{ offline: boolean; open_browser: boolean }>('/settings'),
  saveSettings: (input: Partial<{ offline: boolean; open_browser: boolean }>) => request<{ offline: boolean; open_browser: boolean }>('/settings', { method: 'PATCH', body: JSON.stringify(input) }),
  workspaces: async () => list<Workspace>(await request<Workspace[] | { workspaces?: Workspace[] }>('/workspaces')),
  /** Server-paged workspace list; sort key: name | last_opened | last_scan | findings */
  workspacesPage: (params: { page: number; page_size?: number; sort?: string; order?: 'asc' | 'desc'; search?: string; tag?: string }, opts?: ReadOptions) => {
    const q = new URLSearchParams();
    q.set('page', String(params.page));
    if (params.page_size) q.set('page_size', String(params.page_size));
    if (params.sort) q.set('sort', params.sort);
    if (params.order) q.set('order', params.order);
    if (params.search) q.set('search', params.search);
    if (params.tag) q.set('tag', params.tag);
    return request<WorkspaceListPage>(`/workspaces?${q}`, opts);
  },
  workspace: (id: string, opts?: ReadOptions) => request<Workspace>(`/workspaces/${encodeURIComponent(id)}`, opts),
  selectFolder: () => request<{ cancelled: boolean; path?: string }>('/system/select-folder', { method: 'POST' }),
  createWorkspace: async (input: { root_path: string; name?: string }): Promise<CreatedWorkspace> => {
    const { data, status } = await requestWithStatus<Workspace>('/workspaces', { method: 'POST', body: JSON.stringify(input) });
    return { ...data, existing: status === 200 };
  },
  deleteWorkspace: (id: string) => request<void>(`/workspaces/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  /** Rename a workspace and/or change its default scan profile (PATCH). */
  updateWorkspace: (id: string, patch: { name?: string; default_profile?: string }) => request<Workspace>(`/workspaces/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(patch) }),
  discover: (id: string) => request<Workspace>(`/workspaces/${encodeURIComponent(id)}/discover`, { method: 'POST' }),
  tree: async (id: string, path?: string) => {
    const query = path ? `?path=${encodeURIComponent(path)}` : '';
    const value = await request<TreeNode[] | { items?: TreeNode[]; children?: TreeNode[] } | null>(`/workspaces/${encodeURIComponent(id)}/tree${query}`);
    return Array.isArray(value) ? value : value?.items ?? value?.children ?? [];
  },
  pathOverrides: async (id: string, opts?: ReadOptions) => list<PathOverride>(await request<PathOverride[] | { items?: PathOverride[] }>(`/workspaces/${encodeURIComponent(id)}/path-overrides`, opts)),
  /** Fingerprints dismissed for this workspace; suppressed findings stay stored but drop out of future scan totals, reports, and the CI gate. */
  suppressions: async (id: string, opts?: ReadOptions) => list<Suppression>(await request<Suppression[] | { items?: Suppression[] }>(`/workspaces/${encodeURIComponent(id)}/suppressions`, opts)),
  addSuppression: (id: string, fingerprint: string, reason?: string) => request<Suppression>(`/workspaces/${encodeURIComponent(id)}/suppressions`, { method: 'POST', body: JSON.stringify({ fingerprint, reason: reason ?? '' }) }),
  removeSuppression: (id: string, fingerprint: string) => request<void>(`/workspaces/${encodeURIComponent(id)}/suppressions/${encodeURIComponent(fingerprint)}`, { method: 'DELETE' }),
  savePathOverrides: (id: string, overrides: PathOverride[]) => request<{ items: PathOverride[] }>(`/workspaces/${encodeURIComponent(id)}/path-overrides`, { method: 'PUT', body: JSON.stringify({ overrides }) }),
  rules: async (id: string, opts?: ReadOptions) => {
    const value = await request<{ items?: unknown[]; rules?: unknown[] } | null>(`/workspaces/${encodeURIComponent(id)}/rules`, opts);
    return { rules: value?.rules ?? value?.items ?? [] };
  },
  saveRules: (id: string, rules: unknown) => request(`/workspaces/${encodeURIComponent(id)}/rules`, { method: 'PUT', body: JSON.stringify({ rules }) }),
  scans: async (id: string, opts?: ReadOptions) => list<Scan>(await request<Scan[] | { scans?: Scan[] }>(`/workspaces/${encodeURIComponent(id)}/scans`, opts)),
  /** Server-paged scan history for one workspace; `page` is 1-based and capped at page_size 100 by the API. */
  scansPage: (id: string, page: number, pageSize: number, opts?: { date_from?: string; date_to?: string; signal?: AbortSignal }) => {
    const q = new URLSearchParams();
    q.set('page', String(page));
    q.set('page_size', String(pageSize));
    if (opts?.date_from) q.set('date_from', opts.date_from);
    if (opts?.date_to) q.set('date_to', opts.date_to);
    return request<ScanPage>(`/workspaces/${encodeURIComponent(id)}/scans?${q}`, { signal: opts?.signal });
  },
  /** Cross-workspace findings search; empty param values are dropped exactly like the per-scan findings query. */
  searchFindings: (params: FindingsQuery, opts?: ReadOptions) => {
    const query = new URLSearchParams(Object.entries(params).filter(([, v]) => v));
    return request<SearchFindingsPage>(`/findings/search?${query}`, opts);
  },
  /** Weighted risk profile for one workspace from its latest completed scans. */
  risk: (id: string, opts?: ReadOptions) => request<RiskProfile>(`/workspaces/${encodeURIComponent(id)}/risk`, opts),
  /** Delete every terminal scan beyond the newest `keep` (1..100). Running scans are never touched. */
  pruneScans: (id: string, keep: number) => request<{ deleted: number; kept: number }>(`/workspaces/${encodeURIComponent(id)}/scans?keep=${keep}`, { method: 'DELETE' }),
  /** Severity trend over completed scan history, oldest first; limit defaults to 20 on the server. */
  trends: async (id: string, limit?: number, opts?: ReadOptions) => {
    const query = limit ? `?limit=${limit}` : '';
    return list<SeverityTrendPoint>(await request<SeverityTrendPoint[] | { items?: SeverityTrendPoint[] }>(`/workspaces/${encodeURIComponent(id)}/trends${query}`, opts));
  },
  recentScans: (params?: { limit?: number; offset?: number; state?: string; workspace_id?: string }, opts?: ReadOptions) => {
    if (!params) return request<RecentScansResponse>('/scans', opts);
    const q = new URLSearchParams();
    if (params.limit != null) q.set('limit', String(params.limit));
    if (params.offset != null) q.set('offset', String(params.offset));
    if (params.state) q.set('state', params.state);
    if (params.workspace_id) q.set('workspace_id', params.workspace_id);
    const s = q.toString();
    return request<RecentScansResponse>(`/scans${s ? `?${s}` : ''}`, opts);
  },
  /** Cross-workspace overview for the dashboard: counters plus the latest-completed-per-workspace severity rollup. */
  stats: () => request<GlobalStats>('/stats'),
  startScan: (id: string, profile?: string) => request<Scan>(`/workspaces/${encodeURIComponent(id)}/scans`, { method: 'POST', body: JSON.stringify(profile ? { profile } : {}) }),
  scan: async (id: string, opts?: ReadOptions) => {
    const response = await request<Scan | { scan: Scan; analyzer_runs?: Scan['analyzer_runs'] } | null>(`/scans/${encodeURIComponent(id)}`, opts);
    return response && typeof response === 'object' && 'scan' in response ? { ...response.scan, analyzer_runs: response.analyzer_runs ?? [] } : response;
  },
  cancelScan: (id: string) => request<Scan>(`/scans/${encodeURIComponent(id)}/cancel`, { method: 'POST' }),
  /** Filtered/sorted/paged findings for a scan; empty values are dropped from the query (see FindingsQuery for the param contract). */
  findings: (id: string, params: FindingsQuery, opts?: ReadOptions) => {
    const query = new URLSearchParams(Object.entries(params).filter(([, v]) => v));
    return request<FindingPage>(`/scans/${encodeURIComponent(id)}/findings?${query}`, opts);
  },
  findingPreview: (scanId: string, findingId: string, opts?: ReadOptions) => request<SourcePreview>(`/scans/${encodeURIComponent(scanId)}/findings/${encodeURIComponent(findingId)}/preview`, opts),
  report: (id: string, opts?: ReadOptions) => request<Report>(`/scans/${encodeURIComponent(id)}/report`, opts),
  fixedFindings: (scanId: string, opts?: ReadOptions) => request<FixedFindingsResponse>(`/scans/${encodeURIComponent(scanId)}/fixed`, opts),
  /** Diff one scan against another; pass (newer, older) so "new"/"fixed" read correctly. `available:false` means no diff could be computed. */
  compareScans: (id: string, withId: string) => request<CompareResult>(`/scans/${encodeURIComponent(id)}/compare?with=${encodeURIComponent(withId)}`),
  tools: async () => list<Tool>(await request<Tool[] | { tools?: Tool[] }>('/tools')),
  /** Backend capability inventory: every analyzer with category, profiles, network use, and readiness. */
  analyzers: async () => list<AnalyzerStatus>(await request<AnalyzerStatus[] | { items?: AnalyzerStatus[] }>('/analyzers')),
  toolAction: (id: string, action: 'install' | 'repair' | 'update') => request<Tool>(`/tools/${encodeURIComponent(id)}/${encodeURIComponent(action)}`, { method: 'POST' }),
  uninstallTool: (id: string) => request<Tool>(`/tools/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  cleanSystem: (options?: { logs?: boolean; cache?: boolean; vacuum?: boolean; all?: boolean }) =>
    request<{ reclaimed_bytes: number; logs_removed: number; cache_cleared: boolean; db_vacuumed: boolean; details?: string[]; errors?: string[] }>(
      '/system/clean',
      { method: 'POST', body: JSON.stringify(options ?? {}) }
    ),
  /** Opt-in synthetic demo data for UI / harness testing; server returns 403 unless BLUNTCODE_DEMO_SEED=1 (and optional token header). */
  seedDemo: (preset: 'small' | 'large' | 'stress', opts?: { demoToken?: string }) =>
    request<{ preset: string; workspaces?: number; workspace_ids?: string[]; label?: string; approx_findings?: number; findings_cap?: number; data_directory?: string }>(
      '/system/seed-demo',
      {
        method: 'POST',
        body: JSON.stringify({ preset }),
        headers: opts?.demoToken ? { 'X-Bluntcode-Demo-Token': opts.demoToken } : undefined,
      }
    ),
  stopServer: () => request<{ state: string }>('/system/stop', { method: 'POST' }),
  /** In-app updater: check GitHub releases, then hand off to the staged installer. */
  checkUpdate: () => request<{ current: string; latest: string; available: boolean; release_url: string; release_notes: string }>('/update/check'),
  applyUpdate: () => request<{ started: boolean; staged_at: string }>('/update/apply', { method: 'POST' }),
  openFolder: (kind: 'data' | 'reports' | 'logs' | 'tools') => request<void>('/system/open-folder', { method: 'POST', body: JSON.stringify({ kind }) }),
  pentestProbe: (workspaceId: string, input: { target_url: string; auth_mode?: string; auth_token?: string; scope?: string; insecure_skip_tls_verify?: boolean }) => request<PentestProbeResult>(`/workspaces/${encodeURIComponent(workspaceId)}/pentest/probe`, { method: 'POST', body: JSON.stringify(input) }),
  probeTarget: (input: { target_url: string; auth_mode?: string; auth_token?: string; scope?: string; insecure_skip_tls_verify?: boolean }) => request<PentestProbeResult>('/pentest/probe', { method: 'POST', body: JSON.stringify(input) }),
  markdownUrl: (scanId: string) => `${PREFIX}/scans/${encodeURIComponent(scanId)}/report.md`,
  /** URL for the attachment exports (plain GET navigations, not JSON requests). CSV accepts the same filter/sort params as findings — minus limit/offset — so the file matches the on-screen list. findings.json is the versioned full-report document. */
  exportUrl: (scanId: string, format: 'html' | 'sarif' | 'csv' | 'json', params?: Record<string, string>) => {
    const base = `${PREFIX}/scans/${encodeURIComponent(scanId)}/${format === 'csv' ? 'findings.csv' : format === 'json' ? 'findings.json' : `report.${format}`}`;
    if (!params) return base;
    const query = new URLSearchParams(Object.entries(params).filter(([, value]) => value)).toString();
    return query ? `${base}?${query}` : base;
  },
};

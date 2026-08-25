import type { FindingPage, FindingsQuery, FixedFindingsResponse, GlobalStats, PathOverride, RecentScansResponse, Report, RiskProfile, Scan, ScanPage, SearchFindingsPage, SeverityTrendPoint, SourcePreview, Suppression, Tool, TreeNode, Workspace } from './types';

const PREFIX = '/api/v1';

export class ApiError extends Error {
  constructor(public readonly code: string, message: string, public readonly status?: number) {
    super(message);
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${PREFIX}${path}`, {
    ...init,
    headers: { Accept: 'application/json', ...(init?.body ? { 'Content-Type': 'application/json' } : {}), ...init?.headers },
  });
  if (!response.ok) {
    const body = await response.json().catch(() => null) as { error?: { code?: string; message?: string } } | null;
    throw new ApiError(body?.error?.code ?? 'REQUEST_FAILED', body?.error?.message ?? `Request failed (${response.status})`, response.status);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

/** List endpoints wrap their array in an envelope; a null body (or null envelope field) reads as "nothing yet". */
function list<T>(value: T[] | { items?: T[]; workspaces?: T[]; scans?: T[]; findings?: T[]; tools?: T[] } | null | undefined): T[] {
  if (Array.isArray(value)) return value;
  if (!value) return [];
  return value.items ?? value.workspaces ?? value.scans ?? value.findings ?? value.tools ?? [];
}

export const api = {
  health: () => request<{ status?: string }>('/health'),
  meta: () => request<Record<string, string>>('/meta'),
  settings: () => request<{ offline: boolean; open_browser: boolean }>('/settings'),
  saveSettings: (input: Partial<{ offline: boolean; open_browser: boolean }>) => request<{ offline: boolean; open_browser: boolean }>('/settings', { method: 'PATCH', body: JSON.stringify(input) }),
  workspaces: async () => list<Workspace>(await request<Workspace[] | { workspaces?: Workspace[] }>('/workspaces')),
  workspace: (id: string) => request<Workspace>(`/workspaces/${encodeURIComponent(id)}`),
  selectFolder: () => request<{ cancelled: boolean; path?: string }>('/system/select-folder', { method: 'POST' }),
  createWorkspace: (input: { root_path: string; name?: string }) => request<Workspace>('/workspaces', { method: 'POST', body: JSON.stringify(input) }),
  deleteWorkspace: (id: string) => request<void>(`/workspaces/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  discover: (id: string) => request<Workspace>(`/workspaces/${encodeURIComponent(id)}/discover`, { method: 'POST' }),
  tree: async (id: string, path?: string) => {
    const query = path ? `?path=${encodeURIComponent(path)}` : '';
    const value = await request<TreeNode[] | { items?: TreeNode[]; children?: TreeNode[] } | null>(`/workspaces/${encodeURIComponent(id)}/tree${query}`);
    return Array.isArray(value) ? value : value?.items ?? value?.children ?? [];
  },
  pathOverrides: async (id: string) => list<PathOverride>(await request<PathOverride[] | { items?: PathOverride[] }>(`/workspaces/${encodeURIComponent(id)}/path-overrides`)),
  /** Fingerprints dismissed for this workspace; suppressed findings stay stored but drop out of future scan totals, reports, and the CI gate. */
  suppressions: async (id: string) => list<Suppression>(await request<Suppression[] | { items?: Suppression[] }>(`/workspaces/${encodeURIComponent(id)}/suppressions`)),
  addSuppression: (id: string, fingerprint: string, reason?: string) => request<Suppression>(`/workspaces/${encodeURIComponent(id)}/suppressions`, { method: 'POST', body: JSON.stringify({ fingerprint, reason: reason ?? '' }) }),
  removeSuppression: (id: string, fingerprint: string) => request<void>(`/workspaces/${encodeURIComponent(id)}/suppressions/${encodeURIComponent(fingerprint)}`, { method: 'DELETE' }),
  savePathOverrides: (id: string, overrides: PathOverride[]) => request<{ items: PathOverride[] }>(`/workspaces/${encodeURIComponent(id)}/path-overrides`, { method: 'PUT', body: JSON.stringify({ overrides }) }),
  rules: async (id: string) => {
    const value = await request<{ items?: unknown[]; rules?: unknown[] } | null>(`/workspaces/${encodeURIComponent(id)}/rules`);
    return { rules: value?.rules ?? value?.items ?? [] };
  },
  saveRules: (id: string, rules: unknown) => request(`/workspaces/${encodeURIComponent(id)}/rules`, { method: 'PUT', body: JSON.stringify(rules) }),
  scans: async (id: string) => list<Scan>(await request<Scan[] | { scans?: Scan[] }>(`/workspaces/${encodeURIComponent(id)}/scans`)),
  /** Server-paged scan history for one workspace; `page` is 1-based and capped at page_size 100 by the API. */
  scansPage: (id: string, page: number, pageSize: number) => request<ScanPage>(`/workspaces/${encodeURIComponent(id)}/scans?page=${page}&page_size=${pageSize}`),
  /** Cross-workspace findings search; empty param values are dropped exactly like the per-scan findings query. */
  searchFindings: (params: FindingsQuery) => {
    const query = new URLSearchParams(Object.entries(params).filter(([, v]) => v));
    return request<SearchFindingsPage>(`/findings/search?${query}`);
  },
  /** Weighted risk profile for one workspace from its latest completed scans. */
  risk: (id: string) => request<RiskProfile>(`/workspaces/${encodeURIComponent(id)}/risk`),
  /** Severity trend over completed scan history, oldest first; limit defaults to 20 on the server. */
  trends: async (id: string, limit?: number) => {
    const query = limit ? `?limit=${limit}` : '';
    return list<SeverityTrendPoint>(await request<SeverityTrendPoint[] | { items?: SeverityTrendPoint[] }>(`/workspaces/${encodeURIComponent(id)}/trends${query}`));
  },
  recentScans: () => request<RecentScansResponse>('/scans'),
  /** Cross-workspace overview for the dashboard: counters plus the latest-completed-per-workspace severity rollup. */
  stats: () => request<GlobalStats>('/stats'),
  startScan: (id: string, profile?: string) => request<Scan>(`/workspaces/${encodeURIComponent(id)}/scans`, { method: 'POST', body: JSON.stringify(profile ? { profile } : {}) }),
  scan: async (id: string) => {
    const response = await request<Scan | { scan: Scan; analyzer_runs?: Scan['analyzer_runs'] } | null>(`/scans/${encodeURIComponent(id)}`);
    return response && typeof response === 'object' && 'scan' in response ? { ...response.scan, analyzer_runs: response.analyzer_runs ?? [] } : response;
  },
  cancelScan: (id: string) => request<Scan>(`/scans/${encodeURIComponent(id)}/cancel`, { method: 'POST' }),
  /** Filtered/sorted/paged findings for a scan; empty values are dropped from the query (see FindingsQuery for the param contract). */
  findings: (id: string, params: FindingsQuery) => {
    const query = new URLSearchParams(Object.entries(params).filter(([, v]) => v));
    return request<FindingPage>(`/scans/${encodeURIComponent(id)}/findings?${query}`);
  },
  findingPreview: (scanId: string, findingId: string) => request<SourcePreview>(`/scans/${encodeURIComponent(scanId)}/findings/${encodeURIComponent(findingId)}/preview`),
  report: (id: string) => request<Report>(`/scans/${encodeURIComponent(id)}/report`),
  fixedFindings: (scanId: string) => request<FixedFindingsResponse>(`/scans/${encodeURIComponent(scanId)}/fixed`),
  tools: async () => list<Tool>(await request<Tool[] | { tools?: Tool[] }>('/tools')),
  toolAction: (id: string, action: 'install' | 'repair' | 'update') => request<Tool>(`/tools/${encodeURIComponent(id)}/${action}`, { method: 'POST' }),
  stopServer: () => request<{ state: string }>('/system/stop', { method: 'POST' }),
  openFolder: (kind: 'data' | 'reports' | 'logs' | 'tools') => request<void>('/system/open-folder', { method: 'POST', body: JSON.stringify({ kind }) }),
  markdownUrl: (scanId: string) => `${PREFIX}/scans/${encodeURIComponent(scanId)}/report.md`,
  /** URL for the attachment exports (plain GET navigations, not JSON requests). CSV accepts the same filter/sort params as findings — minus limit/offset — so the file matches the on-screen list. */
  exportUrl: (scanId: string, format: 'html' | 'sarif' | 'csv', params?: Record<string, string>) => {
    const base = `${PREFIX}/scans/${encodeURIComponent(scanId)}/${format === 'csv' ? 'findings.csv' : `report.${format}`}`;
    if (!params) return base;
    const query = new URLSearchParams(Object.entries(params).filter(([, value]) => value)).toString();
    return query ? `${base}?${query}` : base;
  },
};

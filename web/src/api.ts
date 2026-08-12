import type { FindingPage, PathOverride, Report, Scan, SourcePreview, Tool, TreeNode, Workspace } from './types';

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

function list<T>(value: T[] | { items?: T[]; workspaces?: T[]; scans?: T[]; findings?: T[]; tools?: T[] }): T[] {
  if (Array.isArray(value)) return value;
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
    const value = await request<TreeNode[] | { items?: TreeNode[]; children?: TreeNode[] }>(`/workspaces/${encodeURIComponent(id)}/tree${query}`);
    return Array.isArray(value) ? value : value.items ?? value.children ?? [];
  },
  pathOverrides: async (id: string) => list<PathOverride>(await request<PathOverride[] | { items?: PathOverride[] }>(`/workspaces/${encodeURIComponent(id)}/path-overrides`)),
  savePathOverrides: (id: string, overrides: PathOverride[]) => request<{ items: PathOverride[] }>(`/workspaces/${encodeURIComponent(id)}/path-overrides`, { method: 'PUT', body: JSON.stringify({ overrides }) }),
  rules: async (id: string) => {
    const value = await request<{ items?: unknown[]; rules?: unknown[] }>(`/workspaces/${encodeURIComponent(id)}/rules`);
    return { rules: value.rules ?? value.items ?? [] };
  },
  saveRules: (id: string, rules: unknown) => request(`/workspaces/${encodeURIComponent(id)}/rules`, { method: 'PUT', body: JSON.stringify(rules) }),
  scans: async (id: string) => list<Scan>(await request<Scan[] | { scans?: Scan[] }>(`/workspaces/${encodeURIComponent(id)}/scans`)),
  startScan: (id: string, profile?: string) => request<Scan>(`/workspaces/${encodeURIComponent(id)}/scans`, { method: 'POST', body: JSON.stringify(profile ? { profile } : {}) }),
  scan: async (id: string) => {
    const response = await request<Scan | { scan: Scan; analyzer_runs?: Scan['analyzer_runs'] }>(`/scans/${encodeURIComponent(id)}`);
    return 'scan' in response ? { ...response.scan, analyzer_runs: response.analyzer_runs ?? [] } : response;
  },
  cancelScan: (id: string) => request<Scan>(`/scans/${encodeURIComponent(id)}/cancel`, { method: 'POST' }),
  findings: (id: string, params: Record<string, string>) => {
    const query = new URLSearchParams(Object.entries(params).filter(([, v]) => v));
    return request<FindingPage>(`/scans/${encodeURIComponent(id)}/findings?${query}`);
  },
  findingPreview: (scanId: string, findingId: string) => request<SourcePreview>(`/scans/${encodeURIComponent(scanId)}/findings/${encodeURIComponent(findingId)}/preview`),
  report: (id: string) => request<Report>(`/scans/${encodeURIComponent(id)}/report`),
  tools: async () => list<Tool>(await request<Tool[] | { tools?: Tool[] }>('/tools')),
  toolAction: (id: string, action: 'install' | 'repair' | 'update') => request<Tool>(`/tools/${encodeURIComponent(id)}/${action}`, { method: 'POST' }),
  stopServer: () => request<{ state: string }>('/system/stop', { method: 'POST' }),
  markdownUrl: (scanId: string) => `${PREFIX}/scans/${encodeURIComponent(scanId)}/report.md`,
};

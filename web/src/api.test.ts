import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError } from './api';

describe('API client', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('sends workspace creation to the documented endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: 'ws-1', name: 'Demo', root_path: 'C:\\Demo' }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    await expect(api.createWorkspace({ root_path: 'C:\\Demo', name: 'Demo' })).resolves.toMatchObject({ id: 'ws-1' });
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/workspaces', expect.objectContaining({ method: 'POST', body: JSON.stringify({ root_path: 'C:\\Demo', name: 'Demo' }) }));
  });

  it('uses structured API errors', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { code: 'TOOL_NOT_READY', message: 'Ruff is not ready.' } }), { status: 409 })));
    await expect(api.tools()).rejects.toEqual(expect.objectContaining<ApiError>({ name: 'Error', code: 'TOOL_NOT_READY', message: 'Ruff is not ready.', status: 409 }));
  });

  it('persists offline mode through the local settings API', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ offline: true, open_browser: false }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    await expect(api.saveSettings({ offline: true, open_browser: false })).resolves.toEqual({ offline: true, open_browser: false });
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/settings', expect.objectContaining({ method: 'PATCH', body: JSON.stringify({ offline: true, open_browser: false }) }));
  });

  it('saves explicit file selections through the local API', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ items: [] }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    await expect(api.savePathOverrides('ws-1', [{ relative_path: 'src', mode: 'exclude' }])).resolves.toEqual({ items: [] });
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/workspaces/ws-1/path-overrides', expect.objectContaining({ method: 'PUT', body: JSON.stringify({ overrides: [{ relative_path: 'src', mode: 'exclude' }] }) }));
  });

  it('normalizes the backend rules envelope for the file page', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ items: [{ rule_type: 'exclude', pattern: 'dist/**' }] }), { status: 200 })));
    await expect(api.rules('ws-1')).resolves.toEqual({ rules: [{ rule_type: 'exclude', pattern: 'dist/**' }] });
  });

  it('reads analyzer progress returned with a scan', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: 'scan-1', state: 'completed', analyzer_runs: [{ analyzer_id: 'biome', status: 'succeeded' }] }), { status: 200 })));
    await expect(api.scan('scan-1')).resolves.toMatchObject({ id: 'scan-1', analyzer_runs: [{ analyzer_id: 'biome', status: 'succeeded' }] });
  });

  it('removes only the saved workspace record through the local API', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);
    await expect(api.deleteWorkspace('ws-1')).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/workspaces/ws-1', expect.objectContaining({ method: 'DELETE' }));
  });

  it('reads the global overview from the stats endpoint', async () => {
    const payload = { workspaces: 3, scans: { total: 5, completed: 3, running: 1 }, findings: { severity: { critical: 2, high: 0, medium: 1, low: 0, info: 0 }, total: 3 }, suppressions: 3, generated_at: '2026-08-24T12:00:00Z' };
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    await expect(api.stats()).resolves.toEqual(payload);
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/stats', expect.anything());
  });

  it('requests a graceful local server stop', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ state: 'stopping' }), { status: 202 }));
    vi.stubGlobal('fetch', fetchMock);
    await expect(api.stopServer()).resolves.toEqual({ state: 'stopping' });
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/system/stop', expect.objectContaining({ method: 'POST' }));
  });

  it('manages finding suppressions through the workspace endpoints', async () => {
    const fingerprint = 'a'.repeat(64);
    const item = { workspace_id: 'ws-1', fingerprint, reason: 'False positive', created_at: '2026-08-24T00:00:00Z' };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [item] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(item), { status: 201 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(api.suppressions('ws-1')).resolves.toEqual([item]); // {items} envelope reads as a list
    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/workspaces/ws-1/suppressions', expect.anything());

    await expect(api.addSuppression('ws-1', fingerprint, 'False positive')).resolves.toEqual(item);
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/workspaces/ws-1/suppressions', expect.objectContaining({ method: 'POST', body: JSON.stringify({ fingerprint, reason: 'False positive' }) }));

    await expect(api.removeSuppression('ws-1', fingerprint)).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenNthCalledWith(3, `/api/v1/workspaces/ws-1/suppressions/${fingerprint}`, expect.objectContaining({ method: 'DELETE' }));
  });
});

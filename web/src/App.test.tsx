import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from './App';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

async function render(fetchMock: ReturnType<typeof vi.fn>) {
  vi.stubGlobal('fetch', fetchMock);
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<App />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return host;
}

describe('Blunt Code home', () => {
  beforeEach(() => { window.history.replaceState({}, '', '/'); });
  afterEach(async () => { await act(async () => { root?.unmount(); }); document.body.replaceChildren(); vi.unstubAllGlobals(); });

  it('renders local workspaces returned by the API', async () => {
    const fetchMock = vi.fn((input: string) => {
      if (input.endsWith('/workspaces')) return Promise.resolve(json({ items: [{ id: 'ws-1', name: 'Example API', root_path: 'C:\\code\\example-api', languages: ['python', 'typescript'], latest_scan: { id: 'scan-1', state: 'completed', finished_at: '2026-08-12T00:00:00Z', total_findings: 8 } }] }));
      if (input.endsWith('/tools')) return Promise.resolve(json({ items: [{ id: 'ruff', ready: true, can_install: true }] }));
      return Promise.resolve(json({}));
    });

    const host = await render(fetchMock);
    expect(host.textContent).toContain('Example API');
    expect(host.textContent).toContain('C:\\code\\example-api');
    expect(host.textContent).toContain('Python');
    expect(host.textContent).toContain('TypeScript');
    expect(host.textContent).toContain('8 findings');
    expect(host.textContent).not.toContain('No analysis yet');
    expect(host.textContent).toContain('1 of 1 tools ready');
    expect(host.querySelector('.workspace-table table')).not.toBeNull();
    expect(host.textContent).toContain('Last scan');
    const remove = [...host.querySelectorAll('button')].find((button) => button.textContent === 'Remove');
    expect(remove).toBeDefined();
    await act(async () => { remove!.click(); });
    expect(host.textContent).toContain('Remove this workspace?');
    expect(host.textContent).toContain('Your project files will not be changed.');
    const closeApp = [...host.querySelectorAll('button')].find((button) => button.textContent === 'Close app');
    expect(closeApp).toBeDefined();
    await act(async () => { closeApp!.click(); });
    expect(host.textContent).toContain('Close Blunt Code?');
    expect(host.textContent).toContain('Any active scan will be cancelled');
  });

  it('shows a useful empty state when no workspaces exist', async () => {
    const fetchMock = vi.fn((input: string) => Promise.resolve(json(input.endsWith('/workspaces') ? { items: [] } : { items: [] })));
    const host = await render(fetchMock);
    expect(host.textContent).toContain('No workspaces yet');
    expect(host.textContent).toContain('Add workspace');
  });

  it('shows the backend error without exposing implementation detail', async () => {
    const fetchMock = vi.fn((input: string) => Promise.resolve(input.endsWith('/workspaces')
      ? json({ error: { code: 'WORKSPACE_PATH_INACCESSIBLE', message: 'Workspace directory is unavailable.' } }, 422)
      : json({ items: [] })));
    const host = await render(fetchMock);
    expect(host.textContent).toContain('Could not load this view');
    expect(host.textContent).toContain('WORKSPACE_PATH_INACCESSIBLE: Workspace directory is unavailable.');
  });

  it('loads the real rules envelope and saves a file exclusion', async () => {
    window.history.replaceState({}, '', '/workspaces/ws-1/files');
    const fetchMock = vi.fn((input: string, init?: RequestInit) => {
      if (input.endsWith('/workspaces/ws-1')) return Promise.resolve(json({ id: 'ws-1', name: 'Example', root_path: 'C:\\code\\example' }));
      if (input.endsWith('/tree')) return Promise.resolve(json({ items: [{ path: 'src', name: 'src', type: 'directory', included: true, has_children: true }] }));
      if (input.endsWith('/rules')) return Promise.resolve(json({ items: [] }));
      if (input.endsWith('/path-overrides') && init?.method === 'PUT') return Promise.resolve(json({ items: [{ relative_path: 'src', mode: 'exclude' }] }));
      if (input.endsWith('/path-overrides')) return Promise.resolve(json({ items: [] }));
      return Promise.resolve(json({ items: [] }));
    });

    const host = await render(fetchMock);
    const checkbox = host.querySelector<HTMLInputElement>('input[aria-label="Include src"]');
    expect(checkbox).not.toBeNull();
    await act(async () => { checkbox!.click(); });
    const save = [...host.querySelectorAll('button')].find((button) => button.textContent === 'Save selection');
    await act(async () => { save!.click(); await Promise.resolve(); await Promise.resolve(); });
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/workspaces/ws-1/path-overrides', expect.objectContaining({ method: 'PUT', body: JSON.stringify({ overrides: [{ relative_path: 'src', mode: 'exclude' }] }) }));
  });

  it('uses the tool id when an older API response has no display name', async () => {
    window.history.replaceState({}, '', '/tools');
    const fetchMock = vi.fn((input: string) => {
      if (input.endsWith('/tools/ruff/install')) return Promise.resolve(json({ id: 'ruff', ready: true, can_install: true }));
      if (input.endsWith('/tools')) return Promise.resolve(json({ items: [{ id: 'ruff', ready: true, can_install: true }] }));
      return Promise.resolve(json({ items: [] }));
    });

    const host = await render(fetchMock);
    const install = [...host.querySelectorAll('button')].find((button) => button.textContent === 'Install');
    expect(install).toBeDefined();
    await act(async () => { install!.click(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
    expect(host.textContent).toContain('ruff: installed.');
    expect(host.textContent).not.toContain('undefined');
  });

  it('paginates scan history in the workspace view', async () => {
    window.history.replaceState({}, '', '/workspaces/ws-1');
    const scans = Array.from({ length: 7 }, (_, index) => ({
      id: `scan-${index + 1}`,
      workspace_id: 'ws-1',
      state: 'completed',
      started_at: `2026-08-${String(index + 1).padStart(2, '0')}T10:00:00Z`,
      finished_at: `2026-08-${String(index + 1).padStart(2, '0')}T10:01:00Z`,
      new_count: index + 1,
    }));
    const fetchMock = vi.fn((input: string) => {
      if (input.endsWith('/workspaces/ws-1/scans')) return Promise.resolve(json({ items: scans }));
      if (input.endsWith('/workspaces/ws-1')) return Promise.resolve(json({ id: 'ws-1', name: 'Example API', root_path: 'C:\\code\\example-api', latest_scan: scans[0] }));
      return Promise.resolve(json({ items: [] }));
    });

    const host = await render(fetchMock);
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    expect(host.textContent).toContain('Showing 1–6 of 7 scans');
    expect([...host.querySelectorAll('button')].filter((button) => button.textContent === 'Open report')).toHaveLength(6);
    const next = [...host.querySelectorAll('button')].find((button) => button.textContent === 'Next');
    expect(next).toBeDefined();
    await act(async () => { next!.click(); });
    expect(host.textContent).toContain('Showing 7–7 of 7 scans');
    expect([...host.querySelectorAll('button')].filter((button) => button.textContent === 'Open report')).toHaveLength(1);
  });

  it('renders scan progress and a combined report from documented endpoints', async () => {
    window.history.replaceState({}, '', '/scans/scan-1');
    class FakeEventSource {
      static instances: FakeEventSource[] = [];
      listeners = new Map<string, (event: MessageEvent) => void>();
      onerror?: () => void;
      constructor() { FakeEventSource.instances.push(this); }
      addEventListener(type: string, listener: (event: MessageEvent) => void) { this.listeners.set(type, listener); }
      emit(type: string, body: unknown) { this.listeners.get(type)?.({ type, data: JSON.stringify(body) } as MessageEvent); }
      close() {}
    }
    vi.stubGlobal('EventSource', FakeEventSource);
    const scan = { id: 'scan-1', workspace_id: 'ws-1', state: 'completed', started_at: '2026-08-12T00:00:00Z', finished_at: '2026-08-12T00:00:02Z', total_findings: 1, analyzer_runs: [{ analyzer_id: 'biome', status: 'succeeded' }, { analyzer_id: 'semgrep', status: 'succeeded' }, { analyzer_id: 'sonarqube', status: 'succeeded' }] };
    const finding = { id: 'finding-1', analyzer_id: 'biome', severity: 'high', category: 'correctness', title: 'Example finding', message: 'Undefined name', relative_path: 'src/main.py', start_line: 4 };
    const fetchMock = vi.fn((input: string) => {
      if (input.endsWith('/scans/scan-1')) return Promise.resolve(json(scan));
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan, comparison: { new_count: 1, fixed_count: 0, persistent_count: 0 }, warnings: [], findings: [finding] }));
      if (input.endsWith('/scans/scan-1/findings/finding-1/preview')) return Promise.resolve(json({ path: 'src/main.py', highlight_start_line: 4, highlight_end_line: 4, lines: [{ number: 3, text: 'before()' }, { number: 4, text: 'undefinedName()' }, { number: 5, text: 'after()' }] }));
      if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json(input.includes('analyzer=semgrep') ? { items: [], total: 0, limit: 25, offset: 0, has_more: false } : { items: [finding], total: 1, limit: 25, offset: 0, has_more: false }));
      return Promise.resolve(json({ items: [] }));
    });
    const host = await render(fetchMock);
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    await act(async () => { FakeEventSource.instances[0].emit('analyzer.started', { type: 'analyzer.started', data: { analyzer_id: 'ruff', name: 'Ruff' } }); });
    expect(host.textContent).toContain('Analysis overview');
    expect(host.textContent).toContain('Example finding');
    expect(host.textContent).toContain('Analyzer results');
    expect(host.textContent).toContain('Semgrep');
    expect(host.textContent).toContain('0 findings');
    expect(host.querySelector('.findings-table table')).not.toBeNull();
    expect(host.querySelector<HTMLInputElement>('[placeholder="All categories"]')).toBeNull();
    const filters = [...host.querySelectorAll('button')].find((button) => button.textContent === 'Filters');
    expect(filters).toBeDefined();
    await act(async () => { filters!.click(); });
    expect(host.querySelector<HTMLInputElement>('[placeholder="All categories"]')).not.toBeNull();
    const toolFilter = host.querySelector<HTMLSelectElement>('#finding-filters label:nth-of-type(3) select');
    expect(toolFilter).not.toBeNull();
    expect([...toolFilter!.options].map((option) => option.value)).toEqual(['', 'biome', 'semgrep', 'sonarqube']);
    expect(host.textContent).toContain('Showing 1–1 of 1');
    expect(host.textContent).toContain('completed');
    expect(host.textContent).toContain('Ruff started');
    await act(async () => { toolFilter!.value = 'semgrep'; toolFilter!.dispatchEvent(new Event('change', { bubbles: true })); await Promise.resolve(); await Promise.resolve(); });
    expect(host.textContent).toContain('Semgrep reported no findings');
    await act(async () => { toolFilter!.value = ''; toolFilter!.dispatchEvent(new Event('change', { bubbles: true })); await Promise.resolve(); await Promise.resolve(); });
    const preview = host.querySelector<HTMLButtonElement>('[aria-label="Preview src/main.py:4"]');
    expect(preview).not.toBeNull();
    await act(async () => { preview!.click(); await Promise.resolve(); await Promise.resolve(); });
    expect(host.textContent).toContain('Source preview');
    expect(host.textContent).toContain('undefinedName()');
    expect(host.querySelector('.code-preview code.highlight')?.textContent).toContain('undefinedName()');
  });

  it('shows an interrupted scan as a saved partial report, not a live scan', async () => {
    window.history.replaceState({}, '', '/scans/scan-interrupted');
    class FakeEventSource { addEventListener() {} close() {} }
    vi.stubGlobal('EventSource', FakeEventSource);
    const scan = { id: 'scan-interrupted', workspace_id: 'ws-1', state: 'interrupted', started_at: '2026-08-12T00:00:00Z', finished_at: '2026-08-12T00:00:02Z', total_findings: 3, error_summary: 'Previous application process exited before scan completion.' };
    const fetchMock = vi.fn((input: string) => {
      if (input.endsWith('/scans/scan-interrupted')) return Promise.resolve(json(scan));
      if (input.endsWith('/scans/scan-interrupted/report')) return Promise.resolve(json({ scan, comparison: {}, warnings: ['Analysis was interrupted.'], findings: [] }));
      if (input.includes('/scans/scan-interrupted/findings')) return Promise.resolve(json({ items: [], total: 0, limit: 25, offset: 0, has_more: false }));
      return Promise.resolve(json({ items: [] }));
    });
    const host = await render(fetchMock);
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    expect(host.textContent).toContain('Saved report');
    expect(host.textContent).toContain('Analysis overview');
    expect(host.textContent).toContain('Scan interrupted — completed checks are still available.');
    expect(host.textContent).not.toContain('Cancel scan');
  });
});

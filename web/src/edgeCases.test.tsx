import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { App } from './App';
import { compactDuration, date, elapsed, relativeTime } from './lib/format';

/**
 * Hostile-fixture harness: every page rendered through <App /> with API payloads
 * that are null, missing keys, or otherwise malformed. A page only passes when
 * nothing crashes the ErrorBoundary and no broken value (NaN / "Invalid Date" /
 * "undefined") reaches the DOM. These tests are permanent regression coverage.
 */

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

/** Terminal scans never open one; non-terminal scans get an inert stream. */
class NoopEventSource {
  addEventListener() {}
  close() {}
}

/** String patterns match the exact URL, regexes match anywhere (query strings). */
function routeMock(routes: Array<[string | RegExp, unknown]>, fallback: unknown = { items: [] }) {
  return vi.fn((input: string) => {
    for (const [pattern, body] of routes) {
      if (typeof pattern === 'string' ? input === pattern : pattern.test(input)) return Promise.resolve(json(body));
    }
    return Promise.resolve(json(fallback));
  });
}

async function renderAt(path: string, fetchMock: ReturnType<typeof vi.fn>) {
  window.history.replaceState({}, '', path);
  vi.stubGlobal('fetch', fetchMock);
  vi.stubGlobal('EventSource', NoopEventSource);
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<App />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return host;
}

/** Clean means: no boundary crash, and no NaN/Invalid Date/undefined text reached the DOM. */
function expectClean(host: HTMLElement) {
  expect(host.textContent).not.toContain('Something went wrong');
  expect(host.textContent).not.toContain('NaN');
  expect(host.textContent).not.toContain('Invalid Date');
  expect(host.textContent).not.toContain('undefined');
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

describe('hostile API fixtures', () => {
  it('HomePage survives the backend-empty scans payload (null scans/summary)', async () => {
    const fetchMock = routeMock([
      ['/api/v1/scans', { scans: null, total: 0, summary: null }],
      ['/api/v1/tools', { tools: null }],
      ['/api/v1/workspaces', { workspaces: null }],
    ]);
    const host = await renderAt('/', fetchMock);
    expectClean(host);
    // Null workspaces + null scans is the first-run state: the dashboard shows
    // its onboarding hero instead of the stats/activity sections.
    expect(host.textContent).toContain('Point Blunt Code at a project');
    expect(host.textContent).toContain('Add your first workspace');
  });

  it('HomePage renders zeros, never NaN, for a summary with missing fields', async () => {
    const fetchMock = routeMock([
      ['/api/v1/scans', { scans: [{ id: 'scan-1', workspace_id: 'ws-1', state: 'running' }], total: 1, summary: { active_scans: 1 } }],
      ['/api/v1/tools', { items: [] }],
      ['/api/v1/workspaces', { items: [] }],
    ]);
    const host = await renderAt('/', fetchMock);
    expectClean(host);
    expect(host.textContent).toContain('Active scans1');
    expect(host.textContent).toContain('Scans this week0');
    expect(host.textContent).toContain('Workspaces scanned0 of 0');
  });

  it('HomePage survives feed rows with invalid dates, missing counts, unknown states', async () => {
    const fetchMock = routeMock([
      ['/api/v1/scans', { scans: [{ id: 'scan-x', workspace_id: 'ws-1', workspace_name: '', state: 'archiving_artifacts', started_at: 'not-a-date', finished_at: 'not-a-date' }], total: 1, summary: null }],
      ['/api/v1/workspaces', { items: [{ id: 'ws-1', name: '', root_path: '', languages: null, latest_scan: null }] }],
      ['/api/v1/tools', { items: [] }],
    ]);
    const host = await renderAt('/', fetchMock);
    expectClean(host);
    expect(host.textContent).toContain('Not analyzed yet');
    expect(host.textContent).toContain('Workspace'); // empty workspace_name falls back
  });

  it('WorkspacesPage survives null latest_scan/languages and empty name/path', async () => {
    const fetchMock = routeMock([
      ['/api/v1/workspaces', { items: [
        { id: 'ws-1', name: '', root_path: '', languages: null, latest_scan: null },
        { id: 'ws-2', name: 'Hostile', root_path: 'C:\\hostile', languages: [], latest_scan: { id: 'scan-2', state: 'halfway_done', analyzer_runs: null, duration_ms: null, finished_at: 'not-a-date', started_at: 'not-a-date', error_summary: null } },
      ] }],
    ]);
    const host = await renderAt('/workspaces', fetchMock);
    expectClean(host);
    expect(host.textContent).toContain('No supported source languages found');
    // Unknown states still render — sentence-cased and neutral-toned by scanStateDisplay.
    expect(host.textContent).toContain('Halfway done');
  });

  it('WorkspaceDetailPage survives a null scans payload plus a hostile latest scan', async () => {
    const hostileScan = { id: 'scan-1', workspace_id: 'ws-1', state: 'queued', started_at: 'not-a-date', finished_at: 'not-a-date', duration_ms: null, analyzer_runs: null, error_summary: null };
    const fetchMock = routeMock([
      ['/api/v1/workspaces/ws-1/scans', { scans: null }],
      ['/api/v1/workspaces/ws-1', { id: 'ws-1', name: 'Hostile WS', root_path: '', languages: null, latest_scan: hostileScan }],
    ]);
    const host = await renderAt('/workspaces/ws-1', fetchMock);
    expectClean(host);
    expect(host.textContent).toContain('Analyzer detail appears after a scan.'); // analyzer_runs: null
    expect(host.textContent).toContain('Not analyzed yet'); // invalid date string
  });

  it('HistoryPage survives scans with missing counts, null duration and invalid dates', async () => {
    const fetchMock = routeMock([
      [/^\/api\/v1\/workspaces\/ws-1\/scans\?page=/, { items: [
        { id: 'scan-1', workspace_id: 'ws-1', state: 'completed_with_warnings', started_at: 'not-a-date', finished_at: 'not-a-date', duration_ms: null, analyzer_runs: null },
        { id: 'scan-2', workspace_id: 'ws-1', state: 'paused', profile: 'weird' },
      ], total: 2, page: 1, page_size: 6, has_next: false }],
    ]);
    const host = await renderAt('/workspaces/ws-1/scans', fetchMock);
    expectClean(host);
    expect(host.textContent).toContain('Not analyzed yet');
    expect(host.textContent).not.toContain('0ms'); // null duration is unknown, not zero
  });

  it('FilesPage survives null tree, rules and path-override payloads', async () => {
    const fetchMock = routeMock([
      ['/api/v1/workspaces/ws-1/tree', null],
      ['/api/v1/workspaces/ws-1/rules', null],
      ['/api/v1/workspaces/ws-1/path-overrides', null],
      ['/api/v1/workspaces/ws-1', { id: 'ws-1', name: 'Hostile', root_path: 'C:\\h' }],
    ]);
    const host = await renderAt('/workspaces/ws-1/files', fetchMock);
    expectClean(host);
    expect(host.querySelector('.tree-panel [role="alert"]')).toBeNull(); // a null tree is empty, not an error
  });

  it('ScanPage renders a zero-everything report with null findings/comparison/warnings', async () => {
    const scan = { id: 'scan-1', workspace_id: 'ws-1', state: 'completed_with_warnings', started_at: 'not-a-date', finished_at: 'not-a-date', duration_ms: null, analyzer_runs: null, total_findings: 2, fixed_count: 3, error_summary: null };
    const zeroScan = { id: 'scan-1', workspace_id: 'ws-1', state: 'completed', analyzer_runs: null, total_findings: 0, started_at: 'not-a-date', finished_at: 'not-a-date' };
    const fetchMock = routeMock([
      ['/api/v1/scans/scan-1', { scan, analyzer_runs: null }],
      ['/api/v1/scans/scan-1/report', { scan: zeroScan, findings: null, warnings: null, comparison: null }],
      ['/api/v1/scans/scan-1/fixed', { fixed: null, total_fixed: 0, comparison_available: true, previous_scan_id: null }],
      [/\/scans\/scan-1\/findings/, { items: null, total: 0, limit: 25, offset: 0, has_more: false }],
    ]);
    const host = await renderAt('/scans/scan-1', fetchMock);
    expectClean(host);
    expect(host.textContent).toContain('Analysis overview');
    expect(host.textContent).toContain('All clear'); // zero findings, no filters
    expect(host.textContent).toContain('0 findings fixed'); // fixed: null stays at zero
  });

  it('ReportView counts findings with missing/unknown severities without leaking NaN', async () => {
    const scan = { id: 'scan-1', workspace_id: 'ws-1', state: 'completed', started_at: '2026-08-01T00:00:00Z', finished_at: '2026-08-01T00:00:05Z', total_findings: 3, analyzer_runs: [{ analyzer_id: 'biome', status: 'succeeded' }] };
    const noFields = { id: 'f1', analyzer_id: 'biome', category: 'unknown' }; // no severity/rule/message/path
    const fetchMock = routeMock([
      ['/api/v1/scans/scan-1', scan],
      ['/api/v1/scans/scan-1/report', { scan, findings: [noFields, { id: 'f2', analyzer_id: 'biome', severity: 'catastrophic', message: 'x', start_line: 0 }, { id: 'f3', analyzer_id: 'biome', severity: 'high', message: 'y', start_line: 9876543 }] }],
      [/\/scans\/scan-1\/findings/, { items: [noFields], total: 1, limit: 25, offset: 0, has_more: false }],
    ]);
    const host = await renderAt('/scans/scan-1', fetchMock);
    expectClean(host);
    expect(host.textContent).toContain('Biome');
    expect(host.textContent).toContain('1 finding'); // only the known 'high' severity counts
  });

  it('ReportView pagination survives a findings page missing total/limit/offset', async () => {
    const scan = { id: 'scan-1', workspace_id: 'ws-1', state: 'completed', total_findings: 5, analyzer_runs: null, started_at: '2026-08-01T00:00:00Z' };
    const fetchMock = routeMock([
      ['/api/v1/scans/scan-1', scan],
      ['/api/v1/scans/scan-1/report', { scan, findings: [], warnings: null, comparison: null }],
      [/\/scans\/scan-1\/findings/, { items: [] }],
    ]);
    const host = await renderAt('/scans/scan-1', fetchMock);
    expectClean(host);
    expect(host.textContent).toContain('Showing 0–0 of 0');
  });

  it('ReportView survives a report payload with no scan at all', async () => {
    const fetchMock = routeMock([
      ['/api/v1/scans/scan-1', { id: 'scan-1', workspace_id: 'ws-1', state: 'completed', total_findings: 1, started_at: '2026-08-01T00:00:00Z' }],
      ['/api/v1/scans/scan-1/report', {}],
      [/\/scans\/scan-1\/findings/, { items: [], total: 0, limit: 25, offset: 0, has_more: false }],
    ]);
    const host = await renderAt('/scans/scan-1', fetchMock);
    expectClean(host);
    expect(host.textContent).toContain('Analysis overview');
  });

  it('ToolsPage survives tools missing name/version/can_install', async () => {
    const fetchMock = routeMock([
      ['/api/v1/tools', { tools: [{ id: 'ghost', name: null, version: null, detail: null, ready: false }, { id: 'ruff', ready: true }] }],
    ]);
    const host = await renderAt('/tools', fetchMock);
    expectClean(host);
    expect(host.textContent).toContain('ghost'); // name: null falls back to id
    expect(host.textContent).toContain('Managed version');
  });

  it('ToolsPage treats a null tools response as an empty list', async () => {
    const fetchMock = routeMock([['/api/v1/tools', null]]);
    const host = await renderAt('/tools', fetchMock);
    expectClean(host);
    expect(host.textContent).toContain('No managed tools');
  });

  it('SettingsPage survives meta/settings payloads with missing keys', async () => {
    const fetchMock = routeMock([
      ['/api/v1/meta', {}],
      ['/api/v1/settings', {}],
    ]);
    const host = await renderAt('/settings', fetchMock);
    expectClean(host);
    expect(host.textContent).toContain('Available from local metadata');
    expect(host.querySelector('button[role="switch"][aria-label="Open browser automatically"]')!.getAttribute('aria-checked')).toBe('true'); // open_browser default
  });

  it('AboutPage survives empty meta and health payloads', async () => {
    const fetchMock = routeMock([
      ['/api/v1/meta', {}],
      ['/api/v1/health', {}],
    ]);
    const host = await renderAt('/about', fetchMock);
    expectClean(host);
    expect(host.textContent).toContain('Unknown');
    expect(host.textContent).toContain('Ready');
  });

  it('unknown routes render the 404 page without page-level API calls', async () => {
    const fetchMock = routeMock([]);
    const host = await renderAt('/definitely/not/here', fetchMock);
    expectClean(host);
    expect(host.textContent).toContain('Page not found');
    // The once-per-session update check is app chrome and still fires; no
    // page-level data was requested for an unknown route.
    const requested = fetchMock.mock.calls.map(([input]) => String(input));
    expect(requested.filter((url) => !url.endsWith('/update/check'))).toEqual([]);
  });
});

describe('format helpers with hostile timestamps', () => {
  it('date() falls back instead of leaking Invalid Date', () => {
    expect(date('not-a-date')).toBe('Not analyzed yet');
    expect(date('')).toBe('Not analyzed yet');
    expect(date(null)).toBe('Not analyzed yet');
    expect(date(undefined)).toBe('Not analyzed yet');
  });

  it('relativeTime() falls back for unparseable timestamps', () => {
    expect(relativeTime('not-a-date')).toBe('Not analyzed yet');
  });

  it('compactDuration()/elapsed() treat hostile durations as unknown', () => {
    expect(compactDuration(null as unknown as undefined)).toBe('—');
    expect(elapsed('not-a-date', 'not-a-date')).toBe('—');
    expect(elapsed('2026-08-01T00:00:00Z', 'not-a-date')).toBe('—');
  });
});

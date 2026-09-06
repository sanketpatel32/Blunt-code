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

function key(target: EventTarget, k: string) {
  target.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true }));
}

/**
 * The IMP-16 acceptance journey, driven by keyboard semantics end to end:
 * reach a workspace, start a scan, understand an incomplete result, inspect a
 * finding, and come back out without a focus trap. g-sequences, row walking,
 * pane Escape and popstate navigation use real key events; native button
 * activation (Enter on a focused <button>) is exercised through focus + the
 * click it produces, since jsdom does not synthesize activation behavior.
 */
describe('keyboard journey through the core flow', () => {
  beforeEach(() => { window.history.replaceState({}, '', '/'); });
  afterEach(async () => { await act(async () => { root?.unmount(); }); document.body.replaceChildren(); vi.unstubAllGlobals(); });

  it('goes workspace → scan → incomplete result → finding → back out', async () => {
    vi.stubGlobal('EventSource', class { addEventListener() {} close() {} });
    const terminalScan = {
      id: 'scan-9', workspace_id: 'ws-1', state: 'completed_with_warnings',
      started_at: '2026-09-06T00:00:00Z', finished_at: '2026-09-06T00:00:09Z',
      total_findings: 1, error_summary: 'biome failed: analyzer exited with invalid JSON',
      analyzer_runs: [
        { analyzer_id: 'ruff', status: 'succeeded' },
        { analyzer_id: 'semgrep', status: 'succeeded' },
        { analyzer_id: 'biome', status: 'failed' },
      ],
    };
    const finding = { id: 'finding-1', analyzer_id: 'semgrep', severity: 'high', category: 'security', title: 'Unsafe eval', message: 'eval() of user input', relative_path: 'src/main.py', start_line: 4 };
    const fetchMock = vi.fn((input: string, init?: RequestInit) => {
      if (input.endsWith('/api/v1/workspaces') && init?.method !== 'POST') return Promise.resolve(json({ workspaces: [{ id: 'ws-1', name: 'Journey', root_path: 'C:\\journey', default_profile: 'standard' }] }));
      if (input.endsWith('/workspaces/ws-1/scans') && init?.method === 'POST') return Promise.resolve(json({ ...terminalScan, state: 'running', analyzer_runs: [], error_summary: '' }));
      if (input.endsWith('/workspaces/ws-1/scans')) return Promise.resolve(json({ items: [], total: 0, limit: 25, offset: 0, has_more: false }));
      if (input.endsWith('/workspaces/ws-1')) return Promise.resolve(json({ id: 'ws-1', name: 'Journey', root_path: 'C:\\journey', default_profile: 'standard', created_at: '2026-09-01T00:00:00Z' }));
      if (input.endsWith('/scans/scan-9')) return Promise.resolve(json(terminalScan));
      if (input.endsWith('/scans/scan-9/report')) return Promise.resolve(json({ scan: terminalScan, comparison: { new_count: 1, fixed_count: 0, persistent_count: 0 }, warnings: ['biome failed'], findings: [finding] }));
      if (input.endsWith('/scans/scan-9/fixed')) return Promise.resolve(json({ fixed: [], total_fixed: 0, comparison_available: false, previous_scan_id: null }));
      if (input.includes('/scans/scan-9/findings/finding-1/preview')) return Promise.resolve(json({ path: 'src/main.py', highlight_start_line: 4, highlight_end_line: 4, lines: [{ number: 3, text: 'before()' }, { number: 4, text: 'eval(userInput)' }, { number: 5, text: 'after()' }] }));
      if (input.includes('/scans/scan-9/findings')) return Promise.resolve(json({ items: [finding], total: 1, limit: 25, offset: 0, has_more: false }));
      return Promise.resolve(json({ items: [] }));
    });
    vi.stubGlobal('fetch', fetchMock);
    const host = document.createElement('div');
    document.body.append(host);
    root = createRoot(host);
    await act(async () => { root.render(<App />); });
    for (let i = 0; i < 6; i += 1) await act(async () => { await Promise.resolve(); });

    // The keyboard journey starts at a skip link, not by hunting with the mouse.
    expect(host.querySelector('a')).toBe(host.querySelector('a.skip-link'));

    // 1) Keyboard-only navigation to the workspaces page (g w sequence).
    await act(async () => { key(window, 'g'); key(window, 'w'); });
    expect(window.location.pathname).toBe('/workspaces');
    expect(host.textContent).toContain('Journey');

    // 2) Open the workspace from a focused control.
    const openDetails = [...host.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent === 'Open details')!;
    expect(openDetails).toBeDefined();
    await act(async () => { openDetails.focus(); expect(document.activeElement).toBe(openDetails); openDetails.click(); });
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    expect(window.location.pathname).toBe('/workspaces/ws-1');

    // 3) Start the scan from the focused primary action.
    const runScan = [...host.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent === 'Run scan')!;
    expect(runScan).toBeDefined();
    await act(async () => { runScan.focus(); expect(document.activeElement).toBe(runScan); runScan.click(); });
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    const started = fetchMock.mock.calls.filter(([input, init]) => input.endsWith('/workspaces/ws-1/scans') && init?.method === 'POST');
    expect(started).toHaveLength(1);
    expect(window.location.pathname).toBe('/scans/scan-9');

    // 4) The incomplete result is understandable: state badge, failure reason, engine tally.
    expect(host.querySelector('.scan-state-badge')?.textContent).toBe('Completed with warnings');
    expect(host.querySelector('.scan-hero-reason')?.textContent).toContain('biome failed');
    expect(host.textContent).toContain('2 of 3 engines succeeded');

    // 5) Inspect a finding by keyboard: the row is focusable, Enter docks the source pane.
    const row = host.querySelector<HTMLTableRowElement>('.findings-table tbody tr')!;
    expect(row).toBeDefined();
    await act(async () => { row.focus(); expect(document.activeElement).toBe(row); key(row, 'ArrowDown'); key(row, 'Enter'); });
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    expect(host.querySelector('.analysis-split')?.getAttribute('data-pane')).toBe('open');
    expect(host.textContent).toContain('eval(userInput)');

    // 6) Escape closes the pane and hands focus back to the page, not to a dead node.
    await act(async () => { key(window, 'Escape'); });
    expect(host.querySelector('.analysis-split')?.getAttribute('data-pane')).toBe('closed');
    expect(host.contains(document.activeElement) || document.activeElement === document.body).toBe(true);
    expect(host.querySelector('dialog[open]')).toBeNull(); // nothing modal left open

    // 7) Back out to the workspaces page via browser history; content returns, no trap.
    window.history.replaceState({}, '', '/workspaces');
    await act(async () => { window.dispatchEvent(new PopStateEvent('popstate')); });
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    expect(window.location.pathname).toBe('/workspaces');
    expect(host.textContent).toContain('Journey');
    expect(host.querySelector('dialog[open]')).toBeNull();
  });
});

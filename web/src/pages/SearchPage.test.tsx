import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Route } from '../lib/router';
import { SearchPage } from './SearchPage';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function json(body: unknown) {
  return { ok: true, status: 200, json: () => Promise.resolve(body) };
}

/** Search page fetch mock keyed by URL prefix; the last matching route wins. */
function searchMock(routes: Array<[RegExp | string, unknown]>) {
  return vi.fn((input: string) => {
    for (const [pattern, body] of routes) {
      if (typeof pattern === 'string' ? input.startsWith(pattern) : pattern.test(input)) return Promise.resolve(json(body));
    }
    return Promise.resolve(json({ items: [], total: 0, page: 1, page_size: 25, has_next: false }));
  });
}

async function renderPage(fetchMock: ReturnType<typeof vi.fn>) {
  vi.stubGlobal('fetch', fetchMock);
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<SearchPage go={vi.fn<(r: Route) => void>()} />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return host;
}

beforeEach(() => {
  window.history.replaceState(null, '', '/');
});

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

const hit = (id: string, scanId: string, severity: string, rule: string) => ({
  id, analyzer_id: 'semgrep', rule_id: rule, fingerprint: `f-${id}`, severity, category: 'security',
  title: '', message: `${rule} issue in app/run.py`, relative_path: 'app/run.py', start_line: 12,
  scan_id: scanId, workspace_id: 'ws-1',
});

describe('SearchPage', () => {
  it('renders results with severity pills and links back to the originating report', async () => {
    const host = await renderPage(searchMock([
      ['/api/v1/findings/search', {
        items: [hit('f1', 'scan-9', 'critical', 'py-eval'), hit('f2', 'scan-3', 'low', 'E501')],
        total: 2, page: 1, page_size: 25, has_next: false,
      }],
    ]));
    expect(host.querySelectorAll('tbody tr')).toHaveLength(2);
    expect(host.querySelector('.severity.critical')?.textContent).toBe('critical');
    const links = [...host.querySelectorAll<HTMLAnchorElement>('a')].filter((a) => a.textContent === 'Open report');
    expect(links.map((a) => a.getAttribute('href'))).toEqual(['/scans/scan-9', '/scans/scan-3']);
    expect(host.querySelector('table caption')?.textContent).toBe('Global search results'); // Loop W6
  });

  it('shows the empty state when nothing matches', async () => {
    const host = await renderPage(searchMock([]));
    expect(host.textContent).toContain('No matching findings');
  });

  it('debounces typing into one request that carries q and paging params', async () => {
    vi.useFakeTimers();
    let fetchMock: ReturnType<typeof vi.fn>;
    let host: HTMLElement;
    vi.stubGlobal('fetch', fetchMock = vi.fn(() => Promise.resolve(json({ items: [], total: 0, page: 1, page_size: 25, has_next: false }))));
    host = document.createElement('div');
    document.body.append(host);
    root = createRoot(host);
    await act(async () => { root.render(<SearchPage go={vi.fn()} />); });
    await act(async () => { await Promise.resolve(); });
    fetchMock.mockClear();
    const input = host.querySelector<HTMLInputElement>('input[type="search"]')!;
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!;
    await act(async () => {
      setter.call(input, 'race');
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await act(async () => { vi.advanceTimersByTime(249); });
    expect(fetchMock).not.toHaveBeenCalled(); // still inside the debounce window
    await act(async () => { vi.advanceTimersByTime(10); await Promise.resolve(); });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(String(fetchMock.mock.calls[0][0])).toContain('q=race');
    expect(String(fetchMock.mock.calls[0][0])).toContain('page=1');
    expect(String(fetchMock.mock.calls[0][0])).toContain('page_size=25');
  });

  it('pages forward and back through server windows and resets to page 1 on a new filter', async () => {
    vi.useFakeTimers();
    const calls: string[] = [];
    const fetchMock = vi.fn((input: string) => {
      calls.push(String(input));
      const url = String(input);
      const page = Number(new URL(url, 'http://x').searchParams.get('page') ?? '1');
      return Promise.resolve(json({
        items: page === 1 ? [hit('f1', 'scan-1', 'high', 'r1')] : [],
        total: 26, page, page_size: 25, has_next: page < 2,
      }));
    });
    vi.stubGlobal('fetch', fetchMock);
    const host = document.createElement('div');
    document.body.append(host);
    root = createRoot(host);
    await act(async () => { root.render(<SearchPage go={vi.fn()} />); });
    await act(async () => { await Promise.resolve(); });
    const next = [...host.querySelectorAll<HTMLButtonElement>('.findings-pagination button')].find((b) => b.textContent === 'Next')!;
    await act(async () => { next.click(); });
    await act(async () => { await Promise.resolve(); });
    expect(calls.at(-1)).toContain('page=2');
    const previous = [...host.querySelectorAll<HTMLButtonElement>('.findings-pagination button')].find((b) => b.textContent === 'Previous')!;
    await act(async () => { previous.click(); });
    await act(async () => { await Promise.resolve(); });
    expect(calls.at(-1)).toContain('page=1');
    // A severity pill click must reset to page 1.
    calls.length = 0;
    const pill = host.querySelector<HTMLButtonElement>('.severity-pill')!;
    await act(async () => { pill.click(); });
    await act(async () => { await Promise.resolve(); });
    expect(calls.at(-1)).toContain('page=1');
    expect(calls.at(-1)).toContain('severity=critical');
  });
});

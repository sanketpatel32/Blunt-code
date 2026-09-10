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

  it('keeps a deep-linked page on mount and clamps an out-of-range page to the last served page', async () => {
    const calls: string[] = [];
    const fetchMock = vi.fn((input: string) => {
      calls.push(String(input));
      const page = Number(new URL(String(input), 'http://x').searchParams.get('page') ?? '1');
      return Promise.resolve(json({
        items: page === 2 ? [hit('f2', 'scan-2', 'high', 'r2')] : [hit('f1', 'scan-1', 'high', 'r1')],
        total: 26, page, page_size: 25, has_next: page < 2,
      }));
    });
    window.history.replaceState(null, '', '/search?q=error&page=999999');
    const host = await renderPage(fetchMock);
    const searchCalls = () => calls.filter((c) => c.includes('/findings/search'));
    expect(searchCalls()[0]).toContain('page=999999'); // the deep link survives the mount (no reset to 1)
    expect(searchCalls()[0]).toContain('q=error');
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    expect(searchCalls().at(-1)).toContain('page=2'); // out-of-range page snapped to the last page
    expect(window.location.search).toContain('page=2');
    expect(host.querySelectorAll('tbody tr')).toHaveLength(1);
  });

  it('renders facet counts from API severity_counts and omits the badges on legacy payloads', async () => {
    const host = await renderPage(searchMock([
      ['/api/v1/findings/search', {
        items: [hit('f1', 'scan-9', 'critical', 'py-eval')],
        total: 1, page: 1, page_size: 25, has_next: false,
        severity_counts: { critical: 77897, high: 12 },
      }],
    ]));
    expect(host.querySelector('.severity-pill')?.textContent).toContain('77897'); // whole-result count, not the page's rows

    await act(async () => { root.unmount(); });
    document.body.replaceChildren();

    const legacy = await renderPage(searchMock([
      ['/api/v1/findings/search', {
        items: [hit('f1', 'scan-9', 'critical', 'py-eval')],
        total: 1, page: 1, page_size: 25, has_next: false,
      }],
    ]));
    expect(legacy.querySelector('.severity-pill')?.textContent).toBe('critical'); // no wrong page-local counts
  });

  it('opens the quick-look drawer from a result row with the keyboard', async () => {
    const host = await renderPage(searchMock([
      ['/api/v1/findings/search', {
        items: [hit('f1', 'scan-9', 'critical', 'py-eval')],
        total: 1, page: 1, page_size: 25, has_next: false,
      }],
    ]));
    const row = host.querySelector('tbody tr') as HTMLTableRowElement;
    expect(row.tabIndex).toBe(0);
    expect(row.getAttribute('aria-label')).toContain('app/run.py');
    await act(async () => {
      row.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    });
    expect(document.body.textContent).toContain('Diagnostic Message'); // drawer is portaled to the body
  });
});

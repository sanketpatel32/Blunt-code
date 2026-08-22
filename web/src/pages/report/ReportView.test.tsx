import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ReportView } from './ReportView';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

const scan = {
  id: 'scan-1',
  workspace_id: 'ws-1',
  state: 'completed',
  started_at: '2026-08-12T00:00:00Z',
  finished_at: '2026-08-12T00:00:02Z',
  total_findings: 3,
  critical_count: 0,
  high_count: 2,
  medium_count: 1,
  low_count: 0,
  info_count: 0,
  analyzer_runs: [{ analyzer_id: 'ruff', status: 'succeeded' }],
};
const finding = { id: 'finding-1', analyzer_id: 'ruff', severity: 'high', category: 'correctness', title: 'Example finding', message: 'Undefined name', relative_path: 'src/main.py', start_line: 4 };

function findingsPage() {
  return { items: [finding], total: 1, limit: 25, offset: 0, has_more: false };
}

const fetchMock = vi.fn((input: string) => {
  if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan, comparison: { new_count: 1, fixed_count: 0, persistent_count: 0 }, warnings: [], findings: [finding] }));
  if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json(findingsPage()));
  return Promise.resolve(json({ items: [] }));
});

function findingUrls() {
  return fetchMock.mock.calls.map(([input]) => String(input)).filter((url) => url.includes('/findings'));
}

async function render() {
  vi.stubGlobal('fetch', fetchMock);
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<ReportView scanId="scan-1" />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return host;
}

/** Types into a React-controlled input the way a real user does (native value setter). */
async function type(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!;
  await act(async () => {
    setter.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

async function click(element: Element) {
  await act(async () => { (element as HTMLButtonElement).click(); });
}

async function advance(ms: number) {
  await act(async () => { vi.advanceTimersByTime(ms); });
}

function buttonByText(host: HTMLElement, text: string) {
  return [...host.querySelectorAll('button')].find((button) => button.textContent === text)!;
}

beforeEach(() => { vi.useFakeTimers(); fetchMock.mockClear(); });

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('ReportView finding filters', () => {
  it('debounces text filters so typing does not refetch on every keystroke', async () => {
    const host = await render();
    await click(buttonByText(host, 'Filters'));
    const search = host.querySelector<HTMLInputElement>('[placeholder="Message or rule"]')!;
    expect(findingUrls()).toHaveLength(1);

    await type(search, 'ev');
    await advance(150);
    await type(search, 'eval');
    expect(findingUrls()).toHaveLength(1); // typing never triggers an extra request
    expect(host.textContent).toContain('search: "eval"'); // the chip echoes instantly
    expect(host.querySelector('[aria-label="Remove search filter"]')).not.toBeNull();

    await advance(300);
    expect(findingUrls()).toHaveLength(2);
    expect(findingUrls()[1]).toContain('q=eval');
  });

  it('applies select filters immediately without waiting for the debounce', async () => {
    const host = await render();
    await click(buttonByText(host, 'Filters'));
    const severity = host.querySelector<HTMLSelectElement>('#finding-filters label:nth-of-type(1) select')!;
    await act(async () => { severity.value = 'high'; severity.dispatchEvent(new Event('change', { bubbles: true })); });
    expect(findingUrls()).toHaveLength(2);
    expect(findingUrls()[1]).toContain('severity=high');
    expect(host.textContent).toContain('severity: high');
  });

  it('clears all filters immediately instead of waiting out the debounce', async () => {
    const host = await render();
    await click(buttonByText(host, 'Filters'));
    await type(host.querySelector<HTMLInputElement>('[placeholder="Message or rule"]')!, 'eval');
    await advance(300);
    expect(findingUrls().at(-1)).toContain('q=eval');

    await click(buttonByText(host, 'Clear'));
    expect(findingUrls()).toHaveLength(3);
    expect(findingUrls()[2]).not.toContain('q=');
    expect(host.querySelector('.filter-chips')).toBeNull();
  });

  it('renders one removable chip per active filter and removes only that filter', async () => {
    const host = await render();
    await click(buttonByText(host, 'Filters'));
    await type(host.querySelector<HTMLInputElement>('[placeholder="Message or rule"]')!, 'eval');
    await click(host.querySelector<HTMLButtonElement>('.severity-pill.high')!);
    await click(host.querySelector<HTMLButtonElement>('.analyzer-result')!);
    expect([...host.querySelectorAll('.filter-chip')].map((chip) => chip.textContent)).toEqual(['severity: high×', 'tool: Ruff×', 'search: "eval"×']);

    await click(host.querySelector<HTMLButtonElement>('[aria-label="Remove severity filter"]')!);
    expect([...host.querySelectorAll('.filter-chip')].map((chip) => chip.textContent)).toEqual(['tool: Ruff×', 'search: "eval"×']);
    expect(findingUrls().at(-1)).not.toContain('severity=');
    expect(findingUrls().at(-1)).toContain('analyzer=ruff');

    await click(host.querySelector<HTMLButtonElement>('[aria-label="Remove tool filter"]')!);
    expect([...host.querySelectorAll('.filter-chip')].map((chip) => chip.textContent)).toEqual(['search: "eval"×']);
    expect(findingUrls().at(-1)).not.toContain('analyzer=');
  });

  it('shows scan severity counts as toggleable pills and toggles the severity filter', async () => {
    const host = await render();
    const pills = [...host.querySelectorAll<HTMLButtonElement>('.severity-pill')];
    expect(pills.map((pill) => pill.className.replace('severity-pill ', '').replace(' selected', ''))).toEqual(['critical', 'high', 'medium', 'low', 'info']);
    expect(pills.map((pill) => pill.textContent)).toEqual(['critical0', 'high2', 'medium1', 'low0', 'info0']);
    expect(pills[0].disabled).toBe(true); // zero-count severities are muted and disabled
    expect(pills[1].disabled).toBe(false);

    await click(pills[1]);
    expect(pills[1].getAttribute('aria-pressed')).toBe('true');
    expect(pills[1].className).toContain('selected');
    expect(findingUrls().at(-1)).toContain('severity=high');

    await click(pills[1]);
    expect(host.querySelector('.severity-pill.high')!.getAttribute('aria-pressed')).toBe('false');
    expect(findingUrls().at(-1)).not.toContain('severity=');
  });
});

describe('ReportView findings sorting', () => {
  function sortButton(host: HTMLElement, label: string) {
    return [...host.querySelectorAll<HTMLButtonElement>('.findings-table thead .th-sort')].find((button) => button.textContent!.startsWith(label))!;
  }

  function headerCell(host: HTMLElement, label: string) {
    return [...host.querySelectorAll('.findings-table thead th')].find((th) => th.textContent!.startsWith(label))!;
  }

  it('keeps the Finding column non-sortable while the others expose sort buttons', async () => {
    const host = await render();
    const headers = [...host.querySelectorAll('.findings-table thead th')];
    expect(headers.map((th) => th.querySelector('button.th-sort') !== null)).toEqual([true, false, true, true, true]);
    expect(headers.map((th) => th.textContent!.replace(/[^A-Za-z]/g, ''))).toEqual(['Severitysortedascending', 'Finding', 'File', 'Tool', 'Status']);
    expect(headers[1].getAttribute('aria-sort')).toBeNull();
    expect(headers.slice(2).map((th) => th.getAttribute('aria-sort'))).toEqual(['none', 'none', 'none']);
  });

  it('toggles severity between ascending and descending, updating aria-sort and the request', async () => {
    const host = await render();
    expect(findingUrls()[0]).toContain('sort=severity');
    expect(findingUrls()[0]).toContain('order=asc');
    expect(headerCell(host, 'Severity').getAttribute('aria-sort')).toBe('ascending');

    await click(sortButton(host, 'Severity'));
    expect(findingUrls().at(-1)).toContain('sort=severity');
    expect(findingUrls().at(-1)).toContain('order=desc'); // desc rank puts critical findings on top
    expect(headerCell(host, 'Severity').getAttribute('aria-sort')).toBe('descending');
    expect(sortButton(host, 'Severity').textContent).toContain('sorted descending'); // direction is part of the accessible name
    expect(sortButton(host, 'Severity').textContent).toContain('▼');

    await click(sortButton(host, 'Severity'));
    expect(findingUrls().at(-1)).toContain('order=asc');
    expect(headerCell(host, 'Severity').getAttribute('aria-sort')).toBe('ascending');
  });

  it('switches to another column with an ascending default and clears the previous sort', async () => {
    const host = await render();
    await click(sortButton(host, 'File'));
    expect(findingUrls().at(-1)).toContain('sort=path');
    expect(findingUrls().at(-1)).toContain('order=asc');
    expect(headerCell(host, 'File').getAttribute('aria-sort')).toBe('ascending');
    expect(headerCell(host, 'Severity').getAttribute('aria-sort')).toBe('none');

    await click(sortButton(host, 'Tool'));
    expect(findingUrls().at(-1)).toContain('sort=analyzer');
    expect(findingUrls().at(-1)).toContain('order=asc');
    expect(headerCell(host, 'Tool').getAttribute('aria-sort')).toBe('ascending');
    expect(headerCell(host, 'File').getAttribute('aria-sort')).toBe('none');
  });

  it('re-selecting severity from another column applies the descending default so critical comes first', async () => {
    const host = await render();
    await click(sortButton(host, 'Status'));
    expect(findingUrls().at(-1)).toContain('sort=status');
    expect(findingUrls().at(-1)).toContain('order=asc');

    await click(sortButton(host, 'Severity'));
    expect(findingUrls().at(-1)).toContain('sort=severity');
    expect(findingUrls().at(-1)).toContain('order=desc');
    expect(headerCell(host, 'Severity').getAttribute('aria-sort')).toBe('descending');
  });

  it('restarts on the first page when the sort changes', async () => {
    await fetchMock.withImplementation((input: string) => {
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan, comparison: { new_count: 1, fixed_count: 0, persistent_count: 0 }, warnings: [], findings: [finding] }));
      if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json({ items: Array.from({ length: 25 }, (_, index) => ({ ...finding, id: `finding-${index}` })), total: 30, limit: 25, offset: 0, next_offset: 25, has_more: true }));
      return Promise.resolve(json({ items: [] }));
    }, async () => {
      const host = await render();
      const next = [...host.querySelectorAll('button')].find((button) => button.textContent === 'Next')!;
      await click(next);
      expect(findingUrls().at(-1)).toContain('offset=25');

      await click(sortButton(host, 'Tool'));
      expect(findingUrls().at(-1)).toContain('sort=analyzer');
      expect(findingUrls().at(-1)).toContain('offset=0');
    });
  });
});

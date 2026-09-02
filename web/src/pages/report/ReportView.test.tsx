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

const previewBody = {
  path: 'src/main.py',
  lines: [
    { number: 3, text: 'import os' },
    { number: 4, text: 'x = undefined_name' },
    { number: 5, text: 'print(x)' },
  ],
  highlight_start_line: 4,
  highlight_end_line: 4,
};

function findingsPage() {
  return { items: [finding], total: 1, limit: 100, offset: 0, has_more: false, page: 1, page_size: 100, has_next: false };
}

const fetchMock = vi.fn((input: string, _init?: RequestInit) => {
  if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan, comparison: { new_count: 1, fixed_count: 0, persistent_count: 0 }, warnings: [], findings: [finding] }));
  if (input.includes('/preview')) return Promise.resolve(json(previewBody));
  if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json(findingsPage()));
  return Promise.resolve(json({ items: [] }));
});

/** Only list requests — the preview endpoint also matches a loose /findings substring. */
function findingUrls() {
  return fetchMock.mock.calls.map(([input]) => String(input)).filter((url) => url.includes('/findings?'));
}

async function render() {
  vi.stubGlobal('fetch', fetchMock);
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<ReportView scanId="scan-1" />); });
  await settle();
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

async function press(target: Element, key: string) {
  await act(async () => { target.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })); });
}

async function advance(ms: number) {
  await act(async () => { await vi.advanceTimersByTimeAsync(ms); });
}

/** Drains enough microtask turns for a fetch -> json -> setState chain under fake timers; two turns can be one short. */
async function settle() {
  for (let i = 0; i < 8; i += 1) await act(async () => { await Promise.resolve(); });
}

/** One chip button inside a labelled toolbar group, matched by its own text. */
function chipIn(host: HTMLElement, groupLabel: string, text: string) {
  const group = host.querySelector(`fieldset[aria-label="${groupLabel}"]`)!;
  return [...group.querySelectorAll<HTMLButtonElement>('.chip')].find((button) => button.textContent!.startsWith(text))!;
}

function rows(host: HTMLElement) {
  return [...host.querySelectorAll<HTMLTableRowElement>('tbody tr[data-index]')];
}

/** The foot carries two status spans ("Showing N of M" plus "End of list" when done) — the last one is the terminal state. */
function footStatus(host: HTMLElement) {
  return [...host.querySelectorAll('.load-more-status')].at(-1)!.textContent;
}

beforeEach(() => {
  vi.useFakeTimers();
  fetchMock.mockClear();
  window.history.replaceState(null, '', window.location.pathname);
  window.localStorage.clear();
});

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('ReportView toolbar filters', () => {
  it('debounces the search box so typing does not refetch on every keystroke', async () => {
    const host = await render();
    const search = host.querySelector<HTMLInputElement>('[placeholder="Search message, rule, or file"]')!;
    expect(findingUrls()).toHaveLength(1);

    await type(search, 'ev');
    await advance(150);
    await type(search, 'eval');
    expect(findingUrls()).toHaveLength(1); // typing never triggers an extra request
    expect(host.textContent).toContain('search: "eval"'); // the chip echoes instantly

    await advance(300);
    expect(findingUrls()).toHaveLength(2);
    expect(findingUrls()[1]).toContain('q=eval');
    expect(window.location.search).toContain('q=eval'); // shareable links carry the search
  });

  it('severity chips are multi-select and send the comma list the server accepts', async () => {
    const host = await render();
    const high = chipIn(host, 'Severity', 'high');
    const medium = chipIn(host, 'Severity', 'medium');
    expect(high.querySelector('.count')!.textContent).toBe('2'); // chips double as the legend
    expect(high.getAttribute('aria-pressed')).toBe('false');

    await click(high);
    expect(findingUrls().at(-1)).toContain('severity=high');
    expect(high.getAttribute('aria-pressed')).toBe('true');
    expect(host.textContent).toContain('severity: high');

    await click(medium);
    expect(findingUrls().at(-1)).toContain('severity=high%2Cmedium'); // comma list, one request
    expect(host.textContent).toContain('severity: high, medium');

    await click(high);
    expect(findingUrls().at(-1)).toContain('severity=medium');
    expect(findingUrls().at(-1)).not.toContain('medium,'); // unselecting keeps the rest
  });

  it('disables zero-count severities and keeps an active chip selectable', async () => {
    const host = await render();
    expect(chipIn(host, 'Severity', 'critical').disabled).toBe(true);
    expect(chipIn(host, 'Severity', 'high').disabled).toBe(false);
  });

  it('status chips replace Any status with one status at a time', async () => {
    const host = await render();
    await click(chipIn(host, 'Status', 'new'));
    expect(findingUrls().at(-1)).toContain('status=new');
    expect(host.textContent).toContain('status: new');

    await click(chipIn(host, 'Status', 'persistent'));
    expect(findingUrls().at(-1)).toContain('status=persistent');
    expect(findingUrls().at(-1)).not.toContain('persistent,'); // single-select, not a list

    await click(chipIn(host, 'Status', 'persistent'));
    expect(findingUrls().at(-1)).not.toContain('status='); // clicking the active chip clears it
  });

  it('tool chips carry per-tool finding counts and toggle the analyzer filter', async () => {
    await fetchMock.withImplementation((input: string) => {
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan: { ...scan, analyzer_runs: [{ analyzer_id: 'ruff', status: 'succeeded', duration_ms: 1500 }, { analyzer_id: 'biome', status: 'succeeded', duration_ms: 40 }] }, warnings: [], findings: [finding, { ...finding, id: 'finding-2', analyzer_id: 'biome' }, { ...finding, id: 'finding-3', analyzer_id: 'biome' }] }));
      if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json({ items: [finding, { ...finding, id: 'finding-2', analyzer_id: 'biome' }, { ...finding, id: 'finding-3', analyzer_id: 'biome' }], total: 3, limit: 100, offset: 0, has_more: false, has_next: false }));
      return Promise.resolve(json({ items: [] }));
    }, async () => {
      const host = await render();
      const ruff = chipIn(host, 'Tool', 'Ruff');
      expect(ruff.querySelector('.count')!.textContent).toBe('1'); // count from the report payload
      expect(ruff.getAttribute('title')).toContain('1.5s'); // run duration rides along as hover context
      expect(chipIn(host, 'Tool', 'Biome').querySelector('.count')!.textContent).toBe('2');

      await click(ruff);
      expect(findingUrls().at(-1)).toContain('analyzer=ruff');
      expect(host.textContent).toContain('tool: Ruff');

      await click(ruff);
      expect(findingUrls().at(-1)).not.toContain('analyzer='); // clicking the active chip clears it
    });
  });

  it('the Type rail lists only categories present, busiest first, and filters by category', async () => {
    await fetchMock.withImplementation((input: string) => {
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan, warnings: [], findings: [finding, { ...finding, id: 'f2', category: 'security' }, { ...finding, id: 'f3' }] }));
      if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json({ items: [finding], total: 1, limit: 100, offset: 0, has_more: false, has_next: false }));
      return Promise.resolve(json({ items: [] }));
    }, async () => {
      const host = await render();
      const rail = host.querySelector('fieldset[aria-label="Type"]')!;
      const chips = [...rail.querySelectorAll<HTMLButtonElement>('.chip')];
      expect(chips.map((chip) => chip.querySelector('.count')!.textContent)).toEqual(['2', '1']); // correctness first
      expect(chips[0].textContent).toContain('correctness'); // unknown labels fall back to the raw category id

      await click(chips[0]);
      expect(findingUrls().at(-1)).toContain('category=correctness');
      expect(host.textContent).toContain('type: correctness');
    });
  });

  it('Clear resets every filter at once instead of waiting out the debounce', async () => {
    const host = await render();
    await type(host.querySelector<HTMLInputElement>('[placeholder="Search message, rule, or file"]')!, 'eval');
    await advance(300);
    await click(chipIn(host, 'Severity', 'high'));
    expect(host.querySelector('.filter-chips')!.children).toHaveLength(2);

    await click([...host.querySelectorAll('button')].find((button) => button.textContent === 'Clear')!);
    expect(findingUrls().at(-1)).not.toContain('q=');
    expect(findingUrls().at(-1)).not.toContain('severity=');
    expect(host.querySelector('.filter-chips')).toBeNull();
  });

  it('renders one removable chip per active filter and removes only that filter', async () => {
    const host = await render();
    await type(host.querySelector<HTMLInputElement>('[placeholder="Search message, rule, or file"]')!, 'eval');
    await click(chipIn(host, 'Severity', 'high'));
    await click(chipIn(host, 'Tool', 'Ruff'));
    expect([...host.querySelectorAll('.filter-chip')].map((chip) => chip.textContent)).toEqual(['severity: high×', 'tool: Ruff×', 'search: "eval"×']);

    await click(host.querySelector<HTMLButtonElement>('[aria-label="Remove severity filter"]')!);
    expect([...host.querySelectorAll('.filter-chip')].map((chip) => chip.textContent)).toEqual(['tool: Ruff×', 'search: "eval"×']);
    expect(findingUrls().at(-1)).not.toContain('severity=');
    expect(findingUrls().at(-1)).toContain('analyzer=ruff');

    await click(host.querySelector<HTMLButtonElement>('[aria-label="Remove tool filter"]')!);
    expect(findingUrls().at(-1)).not.toContain('analyzer=');
  });

  it('keeps rendering chips for path/rule params from old shared URLs even though no inputs exist for them', async () => {
    window.history.replaceState(null, '', `${window.location.pathname}?path=src%2Fmain.py&rule=F821`);
    const host = await render();
    expect(host.textContent).toContain('file: src/main.py');
    expect(host.textContent).toContain('rule: F821');

    await click(host.querySelector<HTMLButtonElement>('[aria-label="Remove file filter"]')!);
    expect(findingUrls().at(-1)).not.toContain('path=');
    expect(host.textContent).toContain('rule: F821'); // the untouched filter survives
  });

  it('toggles row density and remembers the choice', async () => {
    const host = await render();
    expect(host.querySelector('.finding-list')!.className).not.toContain('finding-dense');

    await click([...host.querySelectorAll('button')].find((button) => button.textContent === 'Compact')!);
    expect(host.querySelector('.finding-list')!.className).toContain('finding-dense');
    expect(window.localStorage.getItem('bluntcode.findingsDensity')).toBe('compact');
    expect([...host.querySelectorAll('button')].some((button) => button.textContent === 'Comfortable')).toBe(true);
  });
});

describe('ReportView sorting', () => {
  function sortButton(host: HTMLElement, label: string) {
    return [...host.querySelectorAll<HTMLButtonElement>('.findings-table thead .th-sort')].find((button) => button.textContent!.startsWith(label))!;
  }

  function headerCell(host: HTMLElement, label: string) {
    return [...host.querySelectorAll('.findings-table thead th')].find((th) => th.textContent!.startsWith(label))!;
  }

  it('defaults to severity descending so critical findings load on top', async () => {
    const host = await render();
    expect(findingUrls()[0]).toContain('sort=severity');
    expect(findingUrls()[0]).toContain('order=desc'); // the old build shipped asc — critical landed last
    expect(headerCell(host, 'Severity').getAttribute('aria-sort')).toBe('descending');
  });

  it('keeps the Finding column non-sortable while the others expose sort buttons', async () => {
    const host = await render();
    const headers = [...host.querySelectorAll('.findings-table thead th')];
    expect(headers.map((th) => th.querySelector('button.th-sort') !== null)).toEqual([true, false, true, true, true]);
    expect(headers.slice(0, 5).map((th) => th.textContent!.replace(/[^A-Za-z]/g, ''))).toEqual(['Severitysorteddescending', 'Finding', 'File', 'Tool', 'Status']);
  });

  it('toggles severity between descending and ascending, updating aria-sort and the request', async () => {
    const host = await render();
    await click(sortButton(host, 'Severity'));
    expect(findingUrls().at(-1)).toContain('order=asc');
    expect(headerCell(host, 'Severity').getAttribute('aria-sort')).toBe('ascending');
    expect(sortButton(host, 'Severity').textContent).toContain('▲');

    await click(sortButton(host, 'Severity'));
    expect(findingUrls().at(-1)).toContain('order=desc');
    expect(headerCell(host, 'Severity').getAttribute('aria-sort')).toBe('descending');
  });

  it('switching columns applies that column default and clears the previous sort', async () => {
    const host = await render();
    await click(sortButton(host, 'File'));
    expect(findingUrls().at(-1)).toContain('sort=path');
    expect(findingUrls().at(-1)).toContain('order=asc');
    expect(headerCell(host, 'File').getAttribute('aria-sort')).toBe('ascending');
    expect(headerCell(host, 'Severity').getAttribute('aria-sort')).toBe('none');

    await click(sortButton(host, 'Severity')); // back from another column: descending default again
    expect(findingUrls().at(-1)).toContain('order=desc');
  });
});

describe('ReportView load-more pagination', () => {
  const TOTAL = 250;

  /** Page-mode mock: answers whichever page/page_size the URL asks for, mirroring the server envelope. */
  function pageMock(body: (host: HTMLElement) => Promise<void>, overrides?: { has_next?: boolean; has_more?: boolean }) {
    return fetchMock.withImplementation((input: string) => {
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan, warnings: [], findings: [finding] }));
      if (input.includes('/preview')) return Promise.resolve(json(previewBody));
      if (input.includes('/scans/scan-1/findings')) {
        const url = new URL(input, 'http://localhost');
        const page = Number(url.searchParams.get('page') ?? '1');
        const pageSize = Number(url.searchParams.get('page_size') ?? '100');
        const start = (page - 1) * pageSize;
        const items = Array.from({ length: Math.min(pageSize, Math.max(0, TOTAL - start)) }, (_, index) => ({ ...finding, id: `finding-${start + index}` }));
        const hasNext = overrides?.has_next ?? start + pageSize < TOTAL;
        return Promise.resolve(json({ items, total: TOTAL, limit: pageSize, offset: start, has_more: overrides?.has_more ?? hasNext, page, page_size: pageSize, has_next: hasNext }));
      }
      return Promise.resolve(json({ items: [] }));
    }, async () => { await body(await render()); });
  }

  it('fetches a 100-row window, then Show more appends the next window without duplicates', async () => {
    await pageMock(async (host) => {
      expect(findingUrls()[0]).toContain('page=1');
      expect(findingUrls()[0]).toContain('page_size=100');
      expect(footStatus(host)).toBe('Showing 100 of 250');

      await click([...host.querySelectorAll('button')].find((button) => button.textContent === 'Show more')!);
      await settle();
      expect(findingUrls().at(-1)).toContain('page=2');
      expect(rows(host)).toHaveLength(200);
      expect(new Set(rows(host).map((row) => row.getAttribute('data-index'))).size).toBe(200); // no duplicate rows
      expect(footStatus(host)).toBe('Showing 200 of 250');

      await click([...host.querySelectorAll('button')].find((button) => button.textContent === 'Show more')!);
      await settle();
      expect(rows(host)).toHaveLength(250);
      expect(footStatus(host)).toBe('End of list');
      expect([...host.querySelectorAll('button')].some((button) => button.textContent === 'Show more')).toBe(false);
    });
  });

  it('treats the envelope has_next as authoritative over the legacy has_more flag', async () => {
    await pageMock(async (host) => {
      expect(footStatus(host)).toBe('End of list'); // page mode wins over the lingering has_more flag
      expect([...host.querySelectorAll('button')].some((button) => button.textContent === 'Show more')).toBe(false);
    }, { has_next: false, has_more: true });
  });

  it('a filter change restarts the accumulated list on window one', async () => {
    await pageMock(async (host) => {
      await click([...host.querySelectorAll('button')].find((button) => button.textContent === 'Show more')!);
      await settle();
      expect(rows(host)).toHaveLength(200);

      await click(chipIn(host, 'Severity', 'high'));
      expect(findingUrls().at(-1)).toContain('severity=high');
      expect(findingUrls().at(-1)).toContain('page=1');
      await settle();
      expect(rows(host)).toHaveLength(100); // replaced, not merged on top of the old windows
    });
  });

  it('the rows-per-fetch selector offers API-legal windows and restarts on window one', async () => {
    await pageMock(async (host) => {
      const sizeButtons = [...host.querySelectorAll<HTMLButtonElement>('.load-more .page-size button')];
      expect(sizeButtons.map((button) => button.textContent)).toEqual(['25', '50', '100', '200']);
      expect(sizeButtons.find((button) => button.getAttribute('aria-pressed') === 'true')!.textContent).toBe('100');
      expect(window.location.search).not.toContain('page_size='); // the default stays out of shareable URLs

      await click([...host.querySelectorAll('button')].find((button) => button.textContent === 'Show more')!);
      await settle();

      await click(sizeButtons.find((button) => button.textContent === '50')!);
      const last = findingUrls().at(-1)!;
      expect(last).toContain('page_size=50');
      expect(last).toContain('page=1');
      expect(window.location.search).toContain('page_size=50'); // shareable links reproduce the window
    });
  });
});

describe('ReportView split pane', () => {
  function twoFindings(body: (host: HTMLElement) => Promise<void>) {
    return fetchMock.withImplementation((input: string) => {
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan, warnings: [], findings: [finding, { ...finding, id: 'finding-2', title: 'Second finding', start_line: 9 }] }));
      if (input.includes('/preview')) return Promise.resolve(json(previewBody));
      if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json({ items: [finding, { ...finding, id: 'finding-2', title: 'Second finding', start_line: 9 }], total: 2, limit: 100, offset: 0, has_more: false, has_next: false }));
      return Promise.resolve(json({ items: [] }));
    }, async () => { await body(await render()); });
  }

  it('clicking a row docks the source pane with the finding code beside the list', async () => {
    await twoFindings(async (host) => {
      expect(host.querySelector('.analysis-split')!.getAttribute('data-pane')).toBe('closed');
      await click(rows(host)[0]);
      expect(host.querySelector('.analysis-split')!.getAttribute('data-pane')).toBe('open');
      const previewCalls = fetchMock.mock.calls.map(([input]) => String(input)).filter((url) => url.includes('/preview'));
      expect(previewCalls.at(-1)).toContain('/scans/scan-1/findings/finding-1/preview');
      await settle();
      const pane = host.querySelector('.source-pane')!;
      expect(pane.textContent).toContain('src/main.py:4');
      expect(pane.querySelectorAll('pre.code-preview code').length).toBe(3);
      expect(pane.querySelectorAll('pre.code-preview code.highlight').length).toBe(1); // only the flagged line
      expect(pane.textContent).toContain('Undefined name');
      expect(rows(host)[0].className).toContain('active'); // the list marks the row the pane shows
    });
  });

  it('prev/next walk the selection along the list and disable at the ends', async () => {
    await twoFindings(async (host) => {
      await click(rows(host)[0]);
      await settle();
      const pane = host.querySelector('.source-pane')!;
      const prev = pane.querySelector<HTMLButtonElement>('[aria-label="Previous finding"]')!;
      const next = pane.querySelector<HTMLButtonElement>('[aria-label="Next finding"]')!;
      expect(prev.disabled).toBe(true);
      expect(next.disabled).toBe(false);

      await click(next);
      await settle();
      expect(rows(host)[1].className).toContain('active');
      expect(host.querySelector('.source-pane')!.textContent).toContain('src/main.py:9');
      expect(host.querySelector('.source-pane')!.querySelector<HTMLButtonElement>('[aria-label="Next finding"]')!.disabled).toBe(true);

      await click(host.querySelector('.source-pane')!.querySelector<HTMLButtonElement>('[aria-label="Previous finding"]')!);
      await settle();
      expect(rows(host)[0].className).toContain('active');
    });
  });

  it('Escape closes the pane and the list returns to full width', async () => {
    await twoFindings(async (host) => {
      await click(rows(host)[0]);
      expect(host.querySelector('.analysis-split')!.getAttribute('data-pane')).toBe('open');
      await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
      expect(host.querySelector('.analysis-split')!.getAttribute('data-pane')).toBe('closed');
      expect(host.querySelector('.source-pane')).toBeNull();
    });
  });

  it('rows are keyboard-walkable: ArrowDown moves focus, Enter opens the pane', async () => {
    await twoFindings(async (host) => {
      const first = rows(host)[0];
      first.focus();
      await press(first, 'ArrowDown');
      expect(document.activeElement).toBe(rows(host)[1]);

      await press(rows(host)[1], 'Enter');
      expect(host.querySelector('.analysis-split')!.getAttribute('data-pane')).toBe('open');
      expect(rows(host)[1].className).toContain('active');
    });
  });

  it('preview errors map to friendly copy instead of a raw error panel', async () => {
    await fetchMock.withImplementation((input: string) => {
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan, warnings: [], findings: [finding] }));
      if (input.includes('/preview')) return Promise.resolve(json({ error: { code: 'SOURCE_FILE_NOT_FOUND', message: 'gone' } }, 404));
      if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json(findingsPage()));
      return Promise.resolve(json({ items: [] }));
    }, async () => {
      const host = await render();
      await click(rows(host)[0]);
      await settle();
      const note = host.querySelector('.source-pane-error')!;
      expect(note.textContent).toContain('Preview unavailable');
      expect(note.textContent).toContain('The file moved or was deleted after this scan ran');
    });
  });
});

describe('ReportView suppression actions', () => {
  const FINGERPRINT = 'f'.repeat(64);
  const fingerprinted = { ...finding, fingerprint: FINGERPRINT };
  const suppressedItem = { ...finding, id: 'finding-suppressed', fingerprint: FINGERPRINT, status: 'suppressed' };

  async function flush() {
    await act(async () => { await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
  }

  function callsFor(method: string) {
    return fetchMock.mock.calls.filter(([, init]) => init?.method === method);
  }

  it('suppresses from a row: opens the dialog, posts the fingerprint, and refreshes the findings', async () => {
    await fetchMock.withImplementation((input: string) => {
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan, warnings: [], findings: [fingerprinted] }));
      if (input.includes('/preview')) return Promise.resolve(json(previewBody));
      if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json({ items: [fingerprinted], total: 1, limit: 100, offset: 0, has_more: false, has_next: false }));
      return Promise.resolve(json({ fingerprint: FINGERPRINT, created_at: '2026-08-24T00:00:00Z' }, 201));
    }, async () => {
      const host = await render();
      const rowButton = host.querySelector<HTMLButtonElement>('[aria-label="Suppress Example finding"]')!;
      expect(rowButton.textContent).toBe('Suppress…');
      await click(rowButton);
      expect(host.querySelector('dialog')!.textContent).toContain('Suppressing hides this finding from future scans, reports, and the CI gate.');

      await click([...host.querySelectorAll('button')].find((button) => button.textContent === 'Suppress finding')!);
      await flush();
      const posts = callsFor('POST');
      expect(posts).toHaveLength(1);
      expect(posts[0][0]).toBe('/api/v1/workspaces/ws-1/suppressions');
      expect(JSON.parse(String(posts[0][1]!.body))).toEqual({ fingerprint: FINGERPRINT, reason: '' });
      expect(host.querySelector('dialog')).toBeNull(); // closes on success
      expect(findingUrls()).toHaveLength(2); // the list refetches so the row status flips
    });
  });

  it('restores a suppressed finding via DELETE and refreshes the findings', async () => {
    await fetchMock.withImplementation((input: string) => {
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan, warnings: [], findings: [suppressedItem] }));
      if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json({ items: [suppressedItem], total: 1, limit: 100, offset: 0, has_more: false, has_next: false }));
      return Promise.resolve(new Response(null, { status: 204 }));
    }, async () => {
      const host = await render();
      expect(host.querySelector('.status-text.suppressed')!.textContent).toBe('suppressed');
      expect(host.querySelector('[aria-label="Suppress Example finding"]')).toBeNull(); // suppressed rows restore instead
      await click(host.querySelector<HTMLButtonElement>('[aria-label="Restore Example finding"]')!);
      await flush();
      const deletes = callsFor('DELETE');
      expect(deletes).toHaveLength(1);
      expect(deletes[0][0]).toBe(`/api/v1/workspaces/ws-1/suppressions/${FINGERPRINT}`);
      expect(findingUrls()).toHaveLength(2);
    });
  });

  it('offers no suppress action when a finding carries no fingerprint', async () => {
    const host = await render();
    expect(host.querySelector('.suppress-finding')).toBeNull();
    expect(host.querySelector('.restore-finding')).toBeNull();
  });
});

describe('ReportView bulk actions', () => {
  it('select-all and per-row checkboxes drive a bulk toolbar with a live count', async () => {
    await fetchMock.withImplementation((input: string) => {
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan, warnings: [], findings: [finding, { ...finding, id: 'finding-2' }] }));
      if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json({ items: [finding, { ...finding, id: 'finding-2' }], total: 2, limit: 100, offset: 0, has_more: false, has_next: false }));
      return Promise.resolve(json({ items: [] }));
    }, async () => {
      const host = await render();
      expect(host.querySelector('[role="toolbar"][aria-label="Bulk actions"]')).toBeNull();

      await click(rows(host)[0].querySelector<HTMLInputElement>('input[type="checkbox"]')!);
      const toolbar = host.querySelector('[role="toolbar"][aria-label="Bulk actions"]')!;
      expect(toolbar.textContent).toContain('1 selected');
      expect(toolbar.textContent).toContain('Copy');

      await click(host.querySelector<HTMLInputElement>('[aria-label="Select all"]')!);
      expect(toolbar.querySelector('[aria-live="polite"]')!.textContent).toBe('2 selected');
    });
  });
});

describe('ReportView export foot', () => {
  it('lists the five export targets as always-visible links and carries the active filter in the CSV href', async () => {
    const host = await render();
    await click(chipIn(host, 'Severity', 'high')); // filter first so the CSV link carries it
    const items = [...host.querySelectorAll('.export-menu.export-inline .export-item')] as HTMLElement[];
    expect(items.map((item) => item.textContent)).toEqual(['Markdown.md', 'HTML.html', 'SARIF.sarif', 'CSV (current filters).csv', 'Jira CSV.csv']);
    expect(items[0].getAttribute('href')).toBe('/api/v1/scans/scan-1/report.md');
    const csvHref = (items[3] as HTMLAnchorElement).getAttribute('href')!;
    expect(csvHref).toContain('/api/v1/scans/scan-1/findings.csv?');
    expect(csvHref).toContain('severity=high'); // the active filter rides along
    expect(csvHref).toContain('sort=severity');
    expect(csvHref).toContain('order=desc'); // the export mirrors the fixed default sort
    expect(csvHref).not.toContain('page='); // paging params stay off the export
    expect(items.slice(0, 4).every((item) => (item as HTMLAnchorElement).hasAttribute('download'))).toBe(true); // plain GET navigation, no fetch
  });

  it('the foot counts findings and engines from the report payload', async () => {
    const host = await render();
    expect(host.querySelector('.report-foot-count')!.textContent).toContain('1 finding');
    expect(host.querySelector('.report-foot-count')!.textContent).toContain('1 engine');
  });
});

describe('ReportView finding row copy', () => {
  it('copies a readable multi-line summary to the clipboard and confirms inline', async () => {
    const rich = { ...finding, rule_id: 'F821', start_column: 7, remediation: 'Define the name before use.', status: 'new' };
    await fetchMock.withImplementation((input: string) => {
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan, warnings: [], findings: [rich] }));
      if (input.includes('/preview')) return Promise.resolve(json(previewBody));
      if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json({ items: [rich], total: 1, limit: 100, offset: 0, has_more: false, has_next: false }));
      return Promise.resolve(json({ items: [] }));
    }, async () => {
      const host = await render();
      const writeText = vi.fn(() => Promise.resolve());
      Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true }); // jsdom ships no clipboard API
      try {
        const copyButton = host.querySelector<HTMLButtonElement>('[aria-label="Copy finding details"]')!;
        await click(copyButton);
        await settle();
        expect(writeText).toHaveBeenCalledTimes(1);
        expect(writeText).toHaveBeenCalledWith('[high] F821 — Undefined name\nsrc/main.py:4:7\nanalyzer: ruff\nremediation: Define the name before use.');
        expect(copyButton.className).toContain('copied'); // brief check-mark swap instead of a toast

        await advance(800);
        expect(copyButton.className).not.toContain('copied');
      } finally {
        delete (navigator as { clipboard?: unknown }).clipboard;
      }
    });
  });
});

describe('ReportView states and guards', () => {
  it('renders the all-clear panel for a completed zero-finding scan', async () => {
    await fetchMock.withImplementation((input: string) => {
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan: { ...scan, total_findings: 0, high_count: 0, medium_count: 0 }, warnings: [], findings: [] }));
      if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json({ items: [], total: 0, limit: 100, offset: 0, has_more: false, has_next: false }));
      return Promise.resolve(json({ items: [] }));
    }, async () => {
      const host = await render();
      expect(host.textContent).toContain('All clear — no findings');
      expect(host.querySelector('.findings-table tbody tr')).toBeNull();
    });
  });

  it('keeps filter-empty messaging when filters exclude everything', async () => {
    await fetchMock.withImplementation((input: string) => {
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan, warnings: [], findings: [finding] }));
      if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json({ items: [], total: 0, limit: 100, offset: 0, has_more: false, has_next: false }));
      return Promise.resolve(json({ items: [] }));
    }, async () => {
      const host = await render();
      expect(host.textContent).toContain('No findings match these filters');
      expect(host.textContent).toContain('Try clearing one or more filters.');
    });
  });

  it('names the tool when one engine reported nothing', async () => {
    await fetchMock.withImplementation((input: string) => {
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan: { ...scan, analyzer_runs: [{ analyzer_id: 'biome', status: 'succeeded' }] }, warnings: [], findings: [] }));
      if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json({ items: [], total: 0, limit: 100, offset: 0, has_more: false, has_next: false }));
      return Promise.resolve(json({ items: [] }));
    }, async () => {
      const host = await render();
      await click(chipIn(host, 'Tool', 'Biome'));
      await settle();
      expect(host.textContent).toContain('Biome reported no findings');
    });
  });

  it('renders report warnings inline above the toolbar', async () => {
    await fetchMock.withImplementation((input: string) => {
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan, warnings: ['semgrep exited 1'], findings: [finding] }));
      if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json(findingsPage()));
      return Promise.resolve(json({ items: [] }));
    }, async () => {
      const host = await render();
      expect(host.querySelector('.inline-warning')!.textContent).toContain('Incomplete analysis');
      expect(host.querySelector('.inline-warning')!.textContent).toContain('semgrep exited 1');
    });
  });

  it('surfaces a failed report load as an error panel with retry', async () => {
    await fetchMock.withImplementation(() => Promise.resolve(json({ error: { code: 'BOOM', message: 'report exploded' } }, 500)), async () => {
      const host = await render();
      expect(host.textContent).toContain('report exploded');
    });
  });

  it('renders a full page of hostile rows (10k-char messages, huge paths) without blowing up', async () => {
    const hostile = Array.from({ length: 100 }, (_, index) => ({
      ...finding,
      id: `hostile-${index}`,
      message: 'x'.repeat(10_000) + ` tail ${index}`,
      relative_path: `src/${'deep/'.repeat(200)}file${index}.py`,
      remediation: 'y'.repeat(5_000),
    }));
    await fetchMock.withImplementation((input: string) => {
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan, warnings: [], findings: hostile }));
      if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json({ items: hostile, total: 100, limit: 100, offset: 0, has_more: false, has_next: false }));
      return Promise.resolve(json({ items: [] }));
    }, async () => {
      const host = await render();
      expect(host.querySelectorAll('.findings-table tbody tr')).toHaveLength(100);
      expect(host.querySelector('.finding-message')?.textContent).toContain('tail 0');
    });
  });

  it('tints critical/high/medium rows and leaves low/info rows clean', async () => {
    const host = await render();
    expect(host.querySelector('.findings-table tbody tr.row-high')).not.toBeNull();
    expect(host.querySelector('.findings-table tbody tr.row-critical')).toBeNull();
    expect(host.querySelector('.findings-table tbody tr.row-medium')).toBeNull();
  });

  it('names the findings table for screen readers', async () => {
    const host = await render();
    expect(host.querySelector('.findings-table caption')?.textContent).toBe('Findings matching the current filters');
  });
});

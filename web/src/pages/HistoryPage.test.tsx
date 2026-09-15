import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api';
import type { Route } from '../lib/router';
import type { Finding, Scan } from '../types';
import { HistoryPage, historyDateBands, HistoryTable } from './HistoryPage';

function compareFinding(id: string): Finding {
  return { id, analyzer_id: 'biome', severity: 'medium', category: 'maintainability', title: `Finding ${id}`, message: 'Something to fix.', relative_path: 'src/a.ts', start_line: 3 };
}

vi.mock('../api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api')>();
  return { ...actual, api: { ...actual.api, workspace: vi.fn(), scansPage: vi.fn(), scan: vi.fn(), compareScans: vi.fn() } };
});

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function scan(overrides: Partial<Scan> & Pick<Scan, 'id' | 'state'>): Scan {
  return { workspace_id: 'ws-1', ...overrides };
}

async function renderTable(scans: Scan[]) {
  const host = document.createElement('div');
  document.body.append(host);
  const go = vi.fn<(route: Route) => void>();
  root = createRoot(host);
  await act(async () => { root.render(<HistoryTable scans={scans} go={go} />); });
  return { host, go };
}

function dataRows(host: HTMLElement) {
  return [...host.querySelectorAll('tbody tr')].filter((row) => !row.matches('.history-band-row, .history-detail-row'));
}

function bandHeaders(host: HTMLElement) {
  return [...host.querySelectorAll<HTMLTableCellElement>('tbody tr.history-band-row th')];
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.useRealTimers();
});

describe('historyDateBands buckets', () => {
  const now = new Date('2026-03-15T12:00:00').getTime();

  it('sorts scans into Today / Yesterday / This week / Earlier by finished_at', () => {
    const bands = historyDateBands([
      scan({ id: 'old', state: 'completed', finished_at: '2026-03-01T10:00:00' }),
      scan({ id: 'week', state: 'completed', finished_at: '2026-03-12T10:00:00' }),
      scan({ id: 'yesterday', state: 'completed', finished_at: '2026-03-14T23:59:00' }),
      scan({ id: 'today', state: 'completed', finished_at: '2026-03-15T08:30:00' }),
    ], now);
    expect(bands.map(({ band }) => band)).toEqual(['Today', 'Yesterday', 'This week', 'Earlier']);
    expect(bands.map(({ scans }) => scans.map((entry) => entry.id))).toEqual([['today'], ['yesterday'], ['week'], ['old']]);
  });

  it('prefers finished_at over started_at when both exist', () => {
    const bands = historyDateBands([scan({ id: 's1', state: 'completed', started_at: '2026-02-20T10:00:00', finished_at: '2026-03-15T09:00:00' })], now);
    expect(bands[0]?.band).toBe('Today');
  });

  it('falls back to started_at when a scan never finished', () => {
    const bands = historyDateBands([scan({ id: 's1', state: 'running', started_at: '2026-03-13T10:00:00', finished_at: null })], now);
    expect(bands[0]?.band).toBe('This week');
  });

  it('puts missing or invalid dates into Earlier', () => {
    const bands = historyDateBands([
      scan({ id: 'undated', state: 'queued' }),
      scan({ id: 'broken', state: 'failed', started_at: 'not-a-date' }),
    ], now);
    expect(bands.map(({ band }) => band)).toEqual(['Earlier']);
    expect(bands[0]?.scans.map((entry) => entry.id)).toEqual(['undated', 'broken']);
  });

  it('drops empty bands instead of rendering bare headers', () => {
    const bands = historyDateBands([scan({ id: 'today-only', state: 'completed', finished_at: '2026-03-15T11:00:00' })], now);
    expect(bands).toHaveLength(1);
    expect(bands[0]?.band).toBe('Today');
  });
});

describe('HistoryTable date bands', () => {
  beforeEach(() => { vi.useFakeTimers(); vi.setSystemTime(new Date('2026-03-15T12:00:00').getTime()); });

  it('groups rows under spanning band headers in chronological order', async () => {
    const iso = (offsetHoursAgo: number) => new Date(new Date('2026-03-15T12:00:00').getTime() - offsetHoursAgo * 3_600_000).toISOString();
    const { host } = await renderTable([
      scan({ id: 'scan-now', state: 'completed', profile: 'now', finished_at: iso(2) }),
      scan({ id: 'scan-yesterday', state: 'completed', profile: 'yesterday', finished_at: iso(26) }),
      scan({ id: 'scan-week', state: 'completed', profile: 'week', finished_at: iso(72) }),
      scan({ id: 'scan-old', state: 'completed', profile: 'old', finished_at: iso(24 * 30) }),
    ]);
    const bands = bandHeaders(host);
    expect(bands.map((header) => header.textContent)).toEqual(['Today', 'Yesterday', 'This week', 'Earlier']);
    expect(bands.map((header) => header.colSpan)).toEqual([6, 6, 6, 6]);
    const sequence = [...host.querySelectorAll('tbody tr')].map((row) => row.classList.contains('history-band-row') ? `band:${row.textContent}` : `row:${row.querySelector('.profile-badge')?.textContent}`);
    expect(sequence).toEqual(['band:Today', 'row:now', 'band:Yesterday', 'row:yesterday', 'band:This week', 'row:week', 'band:Earlier', 'row:old']);
  });
});

describe('HistoryTable expandable rows', () => {
  const detailed = (id: string, overrides: Partial<Scan> = {}): Scan => scan({
    id,
    state: 'completed_with_warnings',
    profile: 'deep',
    started_at: '2026-03-15T09:00:00Z',
    finished_at: '2026-03-15T09:04:30Z',
    total_findings: 4,
    error_summary: 'ruff crashed on generated files',
    analyzer_runs: [{ analyzer_id: 'biome', status: 'success' }, { analyzer_id: 'ruff', status: 'failed' }],
    ...overrides,
  });

  it('reveals a spanning detail strip with timestamps, profile, warning, and analyzer pills on demand', async () => {
    const { host } = await renderTable([detailed('scan-1'), detailed('scan-2', { error_summary: null, analyzer_runs: [] })]);
    expect(host.querySelector('.history-detail-row')).toBeNull();
    const disclose = host.querySelector<HTMLButtonElement>('.history-disclose')!;
    expect(disclose.getAttribute('aria-expanded')).toBe('false');
    await act(async () => { disclose.click(); });
    expect(disclose.getAttribute('aria-expanded')).toBe('true');
    expect(host.querySelectorAll('.history-detail-row')).toHaveLength(1);
    const cell = host.querySelector<HTMLTableCellElement>('.history-detail-row td')!;
    expect(cell.colSpan).toBe(6);
    const detail = cell.querySelector('.history-detail')!;
    const meta = detail.textContent!;
    expect(meta).toContain('Started');
    expect(meta).toContain('Finished');
    expect(meta).toContain('deep');
    expect(detail.querySelector('.inline-warning')?.textContent).toBe('Warning: ruff crashed on generated files');
    const pills = [...detail.querySelectorAll('.history-analyzers .state')];
    expect(pills.map((pill) => pill.className)).toEqual(['state success', 'state failed']);
    expect(pills.map((pill) => pill.textContent)).toEqual(['success', 'failed']);
  });

  it('toggles closed again without leaving the strip behind', async () => {
    const { host } = await renderTable([detailed('scan-1')]);
    const disclose = host.querySelector<HTMLButtonElement>('.history-disclose')!;
    await act(async () => { disclose.click(); });
    await act(async () => { disclose.click(); });
    expect(host.querySelector('.history-detail-row')).toBeNull();
    expect(disclose.getAttribute('aria-expanded')).toBe('false');
  });

  it('omits the warning block and analyzer list when a scan has neither', async () => {
    const { host } = await renderTable([scan({ id: 'scan-1', state: 'completed' })]);
    await act(async () => { host.querySelector<HTMLButtonElement>('.history-disclose')!.click(); });
    const detail = host.querySelector('.history-detail')!;
    expect(detail.querySelector('.inline-warning')).toBeNull();
    expect(detail.querySelector('.history-analyzers')).toBeNull();
    expect(detail.textContent).toContain('Started');
  });

  it('keeps expansions independent per row', async () => {
    const { host } = await renderTable([detailed('scan-1'), detailed('scan-2', { error_summary: null, analyzer_runs: [] })]);
    const [first, second] = [...host.querySelectorAll('.history-disclose')] as HTMLButtonElement[];
    await act(async () => { first.click(); });
    expect(first.getAttribute('aria-expanded')).toBe('true');
    expect(second.getAttribute('aria-expanded')).toBe('false');
    expect(host.querySelectorAll('.history-detail-row')).toHaveLength(1);
  });

  it('explains discovery coverage from the snapshot: selected vs candidate, skip reasons, exclusions', async () => {
    const withSnapshot = detailed('scan-1', {
      snapshot: {
        candidate_file_count: 120,
        selected_file_count: 84,
        skip_counts: { symlink: 2, generated_content: 31, excluded_user: 3 },
        exclusions: ['build/**', 'vendor/**'],
      },
    });
    const { host } = await renderTable([withSnapshot]);
    await act(async () => { host.querySelector<HTMLButtonElement>('.history-disclose')!.click(); });
    const coverage = host.querySelector('.history-coverage')!;
    expect(coverage.textContent).toContain('Selected 84 of 120 candidate files');
    expect(coverage.textContent).toContain('2 symlinks');
    expect(coverage.textContent).toContain('31 as generated artifacts');
    expect(coverage.textContent).toContain('3 by your exclusions');
    expect(coverage.textContent).toContain('2 exclusions in effect');
  });

  it('omits the coverage line when the scan carries no snapshot', async () => {
    const { host } = await renderTable([detailed('scan-1')]);
    await act(async () => { host.querySelector<HTMLButtonElement>('.history-disclose')!.click(); });
    expect(host.querySelector('.history-coverage')).toBeNull();
  });
});

describe('HistoryTable scan rows', () => {
  it('drops the always-zero New/Fixed columns (list items never carry those counts)', async () => {
    const { host } = await renderTable([scan({ id: 's1', state: 'completed', new_count: 5, fixed_count: 2 })]);
    const headers = [...host.querySelectorAll('thead th')].map((th) => th.textContent!.trim());
    expect(headers).not.toContain('New');
    expect(headers).not.toContain('Fixed');
    expect(dataRows(host)[0]!.querySelectorAll('td')).toHaveLength(5);
    expect((host.querySelector('.history-band') as HTMLTableCellElement | null)?.colSpan).toBe(6);
  });

  it('shows the scan profile as a badge beside the state pill, only when present', async () => {
    const { host } = await renderTable([
      scan({ id: 'scan-1', state: 'completed', profile: 'deep' }),
      scan({ id: 'scan-2', state: 'completed' }),
    ]);
    const badges = [...host.querySelectorAll('.profile-badge')];
    expect(badges.map((badge) => badge.textContent)).toEqual(['deep']);
    // The state pill uses the shared scanStateDisplay label (sentence case), not raw snake_case
    expect(badges[0].closest('td')?.textContent).toContain('Completed');
  });

  it('stacks the severity mini-bar with widths proportional to each severity count', async () => {
    const { host } = await renderTable([scan({ id: 'scan-1', state: 'completed', total_findings: 20, critical_count: 2, high_count: 5, medium_count: 10, low_count: 3 })]);
    expect(host.querySelector('.findings-total')?.textContent).toBe('20');
    const bar = host.querySelector<HTMLElement>('.severity-bar')!;
    expect(bar.getAttribute('title')).toBe('2 critical · 5 high · 10 medium · 3 low');
    const segments = [...bar.querySelectorAll<HTMLElement>('i')];
    expect(segments.map((segment) => segment.className)).toEqual(['bar-critical', 'bar-high', 'bar-medium', 'bar-low']);
    expect(segments.map((segment) => segment.style.width)).toEqual(['10%', '25%', '50%', '15%']);
    expect(segments.map((segment) => segment.style.minWidth)).toEqual(['2px', '2px', '2px', '2px']); // nonzero segments stay visible at small counts
  });

  it('gives tiny severity segments a 2px floor so they never round to invisible', async () => {
    const { host } = await renderTable([scan({ id: 'scan-1', state: 'completed', total_findings: 11228, critical_count: 4, high_count: 11224 })]);
    const bar = host.querySelector<HTMLElement>('.severity-bar')!;
    const segments = [...bar.querySelectorAll<HTMLElement>('i')];
    expect(segments.map((segment) => segment.style.width)).toEqual(['0%', '100%']); // 4 in 11228 rounds to 0%
    expect(segments[0].style.minWidth).toBe('2px'); // …but the critical sliver stays on screen
  });

  it('renders a muted zero instead of a bar when a scan found nothing', async () => {
    const { host } = await renderTable([scan({ id: 'scan-1', state: 'completed', total_findings: 0 })]);
    expect(host.querySelector('.findings-zero')?.textContent).toBe('0');
    expect(host.querySelector('.severity-bar')).toBeNull();
  });

  it('offers the markdown export link only for terminal scans that reported findings', async () => {
    const { host } = await renderTable([
      scan({ id: 'scan-1', state: 'completed', total_findings: 5 }),
      scan({ id: 'scan-2', state: 'running', total_findings: 5 }),
      scan({ id: 'scan-3', state: 'completed', total_findings: 0 }),
    ]);
    const links = [...host.querySelectorAll<HTMLAnchorElement>('a')].filter((link) => link.textContent === 'Export .md');
    expect(links).toHaveLength(1);
    expect(links[0].getAttribute('href')).toBe('/api/v1/scans/scan-1/report.md');
  });

  it('marks only the newest warning or failed scan row with a status hint', async () => {
    const warned = await renderTable([
      scan({ id: 'scan-1', state: 'completed_with_warnings', total_findings: 4 }),
      scan({ id: 'scan-2', state: 'completed_with_warnings', total_findings: 4 }),
    ]);
    const rows = dataRows(warned.host);
    expect(rows[0].className).toBe('row-warning');
    expect(rows[1].className).toBe('');

    const failed = await renderTable([scan({ id: 'scan-1', state: 'failed', total_findings: 0 })]);
    expect(dataRows(failed.host)[0]?.className).toBe('row-danger');

    const clean = await renderTable([scan({ id: 'scan-1', state: 'completed', total_findings: 0 })]);
    expect(dataRows(clean.host)[0]?.className).toBe('');
  });

  it('still routes Open report to the scan page', async () => {
    const { host, go } = await renderTable([scan({ id: 'scan-1', state: 'completed', total_findings: 1 })]);
    const open = [...host.querySelectorAll('button')].find((button) => button.textContent === 'Open report')!;
    await act(async () => { open.click(); });
    expect(go).toHaveBeenCalledWith({ page: 'scan', id: 'scan-1' });
  });
});

describe('HistoryTable date filters', () => {
  // Built from LOCAL Date parts, then serialized — the assertions hold in every timezone.
  const midnight = new Date(2026, 2, 15, 0, 0, 0, 0); // local midnight of Mar 15
  const justBefore = new Date(midnight.getTime() - 60_000); // 23:59 local Mar 14

  async function renderFiltered(scans: Scan[], dateFrom?: string, dateTo?: string) {
    const host = document.createElement('div');
    document.body.append(host);
    root = createRoot(host);
    await act(async () => { root.render(<HistoryTable scans={scans} go={vi.fn()} dateFrom={dateFrom} dateTo={dateTo} />); });
    return host;
  }

  it('parses From/To as local day bounds, not UTC midnights', async () => {
    const atMidnight = scan({ id: 'midnight', state: 'completed', total_findings: 1, finished_at: midnight.toISOString() });
    const beforeMidnight = scan({ id: 'late', state: 'completed', total_findings: 2, finished_at: justBefore.toISOString() });

    const fromHost = await renderFiltered([atMidnight, beforeMidnight], '2026-03-15');
    expect(fromHost.querySelector('.findings-total')?.textContent).toBe('1'); // local 00:00 belongs to From-day

    const toHost = await renderFiltered([atMidnight, beforeMidnight], undefined, '2026-03-14');
    expect(toHost.querySelector('.findings-total')?.textContent).toBe('2'); // 23:59 belongs to To-day
  });

  it('shows a filter empty state instead of a bare table when no scan matches the dates', async () => {
    const host = await renderFiltered([scan({ id: 's1', state: 'completed', finished_at: midnight.toISOString() })], '2027-01-01');
    expect(host.textContent).toContain('No scans match these dates');
    expect(host.querySelector('table')).toBeNull();
  });

  it('keeps the table when no date filters are set', async () => {
    const host = await renderFiltered([scan({ id: 's1', state: 'completed', finished_at: midnight.toISOString() })]);
    expect(host.querySelector('table')).not.toBeNull();
  });
});

describe('HistoryTable server paging', () => {
  const basePaging = (overrides: Partial<import('./HistoryPage').HistoryPaging> = {}): import('./HistoryPage').HistoryPaging => ({
    page: 1, pageSize: 6, total: 8, hasNext: true, onPage: vi.fn(), ...overrides,
  });

  it('renders the server slice untouched and drives onPage instead of slicing locally', async () => {
    const onPage = vi.fn();
    const scans = Array.from({ length: 6 }, (_, index) => scan({ id: `s${index}`, state: 'completed', finished_at: '2026-03-15T10:00:00Z' }));
    const host = document.createElement('div');
    document.body.append(host);
    root = createRoot(host);
    await act(async () => { root.render(<HistoryTable scans={scans} go={vi.fn()} paging={basePaging({ page: 2, total: 14, hasNext: true, onPage })} />); });
    expect(dataRows(host)).toHaveLength(6); // no double slicing of the served page
    expect(host.querySelector('output')!.textContent).toBe('Page 2 of 3');
    expect(host.querySelector('.history-pagination span')?.textContent).toBe('Showing 7–12 of 14 scans');
    const [previous, next] = [...host.querySelectorAll<HTMLButtonElement>('.history-pagination button')];
    await act(async () => { previous.click(); });
    await act(async () => { next.click(); });
    expect(onPage).toHaveBeenNthCalledWith(1, 1);
    expect(onPage).toHaveBeenNthCalledWith(2, 3);
  });

  it('disables Previous on the first page and Next when the server reports no further pages', async () => {
    const host = document.createElement('div');
    document.body.append(host);
    root = createRoot(host);
    await act(async () => { root.render(<HistoryTable scans={[scan({ id: 's0', state: 'completed' })]} go={vi.fn()} paging={basePaging({ page: 1, total: 13, hasNext: false })} />); });
    const buttons = [...host.querySelectorAll<HTMLButtonElement>('.history-pagination button')];
    expect(buttons[0].disabled).toBe(true);
    expect(buttons[1].disabled).toBe(true);
  });

  it('shows the empty state only when the whole history is empty, not on a transient over-range page', async () => {
    const host = document.createElement('div');
    document.body.append(host);
    root = createRoot(host);
    await act(async () => { root.render(<HistoryTable scans={[]} go={vi.fn()} paging={basePaging({ page: 4, total: 18, hasNext: false })} />); });
    expect(host.querySelector('.history-pagination')).not.toBeNull();
    expect(host.textContent).not.toContain('No scans yet');
  });
});

describe('HistoryTable accessibility caption (Loop W6)', () => {
  it('names the history table for screen readers', async () => {
    const { host } = await renderTable([scan({ id: 's1', state: 'completed' })]);
    expect(host.querySelector('table caption')?.textContent).toBe('Scan history for this workspace');
  });
});

describe('HistoryPage', () => {
  // Built from LOCAL Date parts so the filter assertions hold in every timezone.
  const localIso = (year: number, month: number, day: number) => new Date(year, month - 1, day, 12, 0, 0).toISOString();
  const onAug29 = (id: string): Scan => scan({ id, state: 'completed', started_at: localIso(2026, 8, 29), finished_at: localIso(2026, 8, 29) });
  const offRange = (id: string, day: number): Scan => scan({ id, state: 'completed', started_at: localIso(2026, 8, day), finished_at: localIso(2026, 8, day) });

  function mockPages(pages: Scan[][], total: number) {
    vi.mocked(api.workspace).mockResolvedValue({ id: 'ws-1', name: 'Example API', root_path: 'C:\\code\\example-api' });
    vi.mocked(api.scansPage).mockImplementation(async (_id: string, page: number) => ({
      items: pages[page - 1] ?? [],
      total,
      page,
      page_size: 6,
      has_next: page < pages.length,
    }));
  }

  async function renderPage() {
    const host = document.createElement('div');
    document.body.append(host);
    root = createRoot(host);
    await act(async () => { root.render(<HistoryPage workspaceId="ws-1" go={vi.fn<(route: Route) => void>()} />); });
    await act(async () => {});
    return host;
  }

  beforeEach(() => {
    vi.mocked(api.workspace).mockReset();
    vi.mocked(api.scansPage).mockReset();
    vi.mocked(api.scan).mockReset();
    vi.mocked(api.compareScans).mockReset();
    window.history.replaceState(null, '', '/');
  });

  afterEach(() => { window.history.replaceState(null, '', '/'); });

  it('restores the page from ?page= on mount and keeps the param in sync on navigation', async () => {
    mockPages([Array.from({ length: 6 }, (_, i) => offRange(`a${i}`, 10 + i)), Array.from({ length: 6 }, (_, i) => offRange(`b${i}`, 20 + i)), [offRange('c0', 28)]], 13);
    window.history.replaceState(null, '', '/workspaces/ws-1/scans?page=3&sort=asc');
    const host = await renderPage();
    expect(api.scansPage).toHaveBeenCalledWith('ws-1', 3, 6);
    expect(host.querySelector('output')!.textContent).toBe('Page 3 of 3');
    const [previous] = [...host.querySelectorAll<HTMLButtonElement>('.history-pagination button')];
    await act(async () => { previous!.click(); });
    await act(async () => {});
    expect(window.location.search).toBe('?page=2&sort=asc'); // other params survive
    expect(host.querySelector('output')!.textContent).toBe('Page 2 of 3');
  });

  it('walks every server page while a date filter is active and totals reflect the filtered set', async () => {
    mockPages([
      [onAug29('a1'), offRange('x1', 10), onAug29('a2'), offRange('x2', 11), offRange('x3', 12), offRange('x4', 13)],
      [offRange('x5', 14), onAug29('a3')],
    ], 8);
    const host = await renderPage();
    vi.mocked(api.scansPage).mockClear();

    const from = host.querySelector<HTMLInputElement>('input[aria-label="Filter from date"]')!;
    const nativeSetter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
    await act(async () => {
      nativeSetter.call(from, '2026-08-29');
      from.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await act(async () => {});

    // Match on page 2 was invisible before; the walk must have fetched it.
    expect(api.scansPage).toHaveBeenCalledWith('ws-1', 2, 6);
    expect(dataRows(host)).toHaveLength(3);
    expect(host.querySelector('.history-filter-count')!.textContent).toBe('3 scans');
    expect(host.querySelector('.history-pagination-count')!.textContent).toBe('Showing 1–3 of 8 scans');
    expect(host.querySelector('.history-pagination button')).toBeNull(); // page controls hidden while filtered
  });

  it('clears the filter back to normal server paging', async () => {
    mockPages([[onAug29('a1'), offRange('x1', 10)]], 2);
    const host = await renderPage();
    const clear = host.querySelector<HTMLButtonElement>('.history-filter-clear');
    expect(clear).toBeNull(); // no filter yet, nothing to clear
    const from = host.querySelector<HTMLInputElement>('input[aria-label="Filter from date"]')!;
    const nativeSetter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!;
    await act(async () => {
      nativeSetter.call(from, '2026-08-29');
      from.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await act(async () => {});
    expect(host.querySelector('.history-pagination-count')!.textContent).toBe('Showing 1–1 of 2 scans');
    await act(async () => { host.querySelector<HTMLButtonElement>('.history-filter-clear')!.click(); });
    await act(async () => {});
    expect(host.querySelector('.history-pagination button')).not.toBeNull(); // controls return
    expect(host.querySelector('.history-pagination-count')!.textContent).toBe('Showing 1–2 of 2 scans'); // normal server paging again
  });

  describe('compare workflow', () => {
    const olderScan = scan({ id: 'older', state: 'completed', finished_at: localIso(2026, 8, 20), total_findings: 40 });
    const newerScan = scan({ id: 'newer', state: 'completed', finished_at: localIso(2026, 8, 29), total_findings: 35 });

    function mockComparePages(pages: Scan[][]) {
      mockPages(pages, pages.flat().length);
      vi.mocked(api.scan).mockImplementation(async (id: string) => pages.flat().find((entry) => entry.id === id) ?? null);
      vi.mocked(api.compareScans).mockResolvedValue({
        available: true,
        current_scan_id: 'newer',
        previous_scan_id: 'older',
        summary: { new: 1, fixed: 2, persistent: 3 },
        new: [compareFinding('n1')],
        fixed: [compareFinding('f1'), compareFinding('f2')],
        persistent: [compareFinding('p1')],
        not_evaluated: [],
      });
    }

    async function clickButton(host: HTMLElement, label: string) {
      const button = [...host.querySelectorAll('button')].find((candidate) => candidate.textContent === label);
      expect(button, `no "${label}" button`).toBeDefined();
      await act(async () => { button!.click(); });
      await act(async () => {});
    }

    it('walks selection: strip appears on Compare, other rows offer "with this", picking completes the pair', async () => {
      mockComparePages([[newerScan, olderScan]]);
      const host = await renderPage();
      expect([...host.querySelectorAll('button')].filter((button) => button.textContent === 'Compare')).toHaveLength(2);

      await clickButton(host, 'Compare'); // first row = newer
      const strip = host.querySelector('.compare-strip')!;
      expect(strip.getAttribute('role')).toBe('region');
      expect(strip.getAttribute('aria-label')).toBe('Scan comparison selection');
      expect(strip.textContent).toContain('Comparing the');
      expect(strip.textContent).toContain('35'); // the base scan's finding count
      expect(strip.textContent).toContain('pick a second scan below');
      expect(window.location.search).toBe('?compare=newer');
      expect(host.querySelector('.compare-base-tag')?.textContent).toBe('Base scan');
      expect([...host.querySelectorAll('button')].filter((button) => button.textContent === 'with this')).toHaveLength(1);

      await clickButton(host, 'with this'); // second row = older
      expect(api.compareScans).toHaveBeenCalledWith('newer', 'older'); // ordered (newer, older)
      expect(window.location.search).toBe('?compare=newer&with=older');
      expect(host.querySelector('.compare-panel')).not.toBeNull();
      expect(host.querySelector('.compare-headline')!.textContent).toContain('1 new');
      expect(host.querySelector('.compare-strip')).toBeNull();
    });

    it('exits via Cancel compare and via Escape', async () => {
      mockComparePages([[newerScan, olderScan]]);
      const host = await renderPage();
      await clickButton(host, 'Compare');
      expect(host.querySelector('.compare-strip')).not.toBeNull();
      await clickButton(host, 'Cancel compare');
      expect(host.querySelector('.compare-strip')).toBeNull();
      expect(window.location.search).toBe('');
      expect([...host.querySelectorAll('button')].filter((button) => button.textContent === 'Compare')).toHaveLength(2);

      await clickButton(host, 'Compare'); // selection again…
      await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); }); // …Esc is the keyboard way out
      await act(async () => {});
      expect(host.querySelector('.compare-strip')).toBeNull();
      expect(window.location.search).toBe('');
    });

    it('restores a completed comparison from a ?compare=&with= deep link, ordering the request newest-first', async () => {
      mockComparePages([[newerScan, olderScan]]);
      window.history.replaceState(null, '', '/?compare=older&with=newer'); // deliberately reversed
      const host = await renderPage();
      expect(api.compareScans).toHaveBeenCalledWith('newer', 'older');
      expect(host.querySelector('.compare-panel')).not.toBeNull();
      expect(host.querySelector('.compare-headline')!.textContent).toContain('2 fixed');
      expect(host.querySelector('.compare-strip')).toBeNull();
    });

    it('gates cancelled scans behind a partial-scan acceptance and skips non-comparable states', async () => {
      const cancelled = scan({ id: 'part', state: 'cancelled', finished_at: localIso(2026, 8, 28), total_findings: 9 });
      const failed = scan({ id: 'bad', state: 'failed', finished_at: localIso(2026, 8, 27), total_findings: 2 });
      mockComparePages([[newerScan, cancelled, failed]]);
      const host = await renderPage();
      // failed is terminal but never comparable — only completed + cancelled offer Compare
      const compareButtons = [...host.querySelectorAll('button')].filter((button) => button.textContent === 'Compare');
      expect(compareButtons).toHaveLength(2);
      await act(async () => { compareButtons[1]!.click(); }); // the cancelled row's button (rows sort newest-first)
      await act(async () => {});
      const strip = host.querySelector('.compare-strip--warning')!;
      expect(strip.textContent).toContain('partial scan');
      expect(window.location.search).toBe(''); // nothing committed yet
      await clickButton(host, 'Compare anyway');
      expect(window.location.search).toBe('?compare=part');
      await clickButton(host, 'with this');
      expect(api.compareScans).toHaveBeenCalledWith('newer', 'part'); // newer scan is still "current"
    });
  });
});

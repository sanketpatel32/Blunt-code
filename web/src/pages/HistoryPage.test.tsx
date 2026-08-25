import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Route } from '../lib/router';
import type { Scan } from '../types';
import { historyDateBands, HistoryTable } from './HistoryPage';

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
    expect(bands.map((header) => header.colSpan)).toEqual([7, 7, 7, 7]);
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
    expect(cell.colSpan).toBe(7);
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
});

describe('HistoryTable scan rows', () => {
  it('shows the scan profile as a badge beside the state pill, only when present', async () => {
    const { host } = await renderTable([
      scan({ id: 'scan-1', state: 'completed', profile: 'deep' }),
      scan({ id: 'scan-2', state: 'completed' }),
    ]);
    const badges = [...host.querySelectorAll('.profile-badge')];
    expect(badges.map((badge) => badge.textContent)).toEqual(['deep']);
    expect(badges[0].closest('td')?.querySelector('.state.completed')?.textContent).toBe('completed');
  });

  it('stacks the severity mini-bar with widths proportional to each severity count', async () => {
    const { host } = await renderTable([scan({ id: 'scan-1', state: 'completed', total_findings: 20, critical_count: 2, high_count: 5, medium_count: 10, low_count: 3 })]);
    expect(host.querySelector('.findings-total')?.textContent).toBe('20');
    const bar = host.querySelector<HTMLElement>('.severity-bar')!;
    expect(bar.getAttribute('title')).toBe('2 critical · 5 high · 10 medium · 3 low');
    const segments = [...bar.querySelectorAll<HTMLElement>('i')];
    expect(segments.map((segment) => segment.className)).toEqual(['bar-critical', 'bar-high', 'bar-medium', 'bar-low']);
    expect(segments.map((segment) => segment.style.width)).toEqual(['10%', '25%', '50%', '15%']);
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
    const [previous, next] = [...host.querySelectorAll('.history-pagination button')];
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

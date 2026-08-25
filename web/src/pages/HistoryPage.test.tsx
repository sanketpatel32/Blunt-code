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
  return [...host.querySelectorAll('tbody tr')].filter((row) => !row.classList.contains('history-band-row'));
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

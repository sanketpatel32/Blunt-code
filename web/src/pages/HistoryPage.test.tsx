import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Route } from '../lib/router';
import type { Scan } from '../types';
import { HistoryTable } from './HistoryPage';

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

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
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
    const rows = [...warned.host.querySelectorAll('tbody tr')];
    expect(rows[0].className).toBe('row-warning');
    expect(rows[1].className).toBe('');

    const failed = await renderTable([scan({ id: 'scan-1', state: 'failed', total_findings: 0 })]);
    expect(failed.host.querySelector('tbody tr')?.className).toBe('row-danger');

    const clean = await renderTable([scan({ id: 'scan-1', state: 'completed', total_findings: 0 })]);
    expect(clean.host.querySelector('tbody tr')?.className).toBe('');
  });

  it('still routes Open report to the scan page', async () => {
    const { host, go } = await renderTable([scan({ id: 'scan-1', state: 'completed', total_findings: 1 })]);
    const open = [...host.querySelectorAll('button')].find((button) => button.textContent === 'Open report')!;
    await act(async () => { open.click(); });
    expect(go).toHaveBeenCalledWith({ page: 'scan', id: 'scan-1' });
  });
});

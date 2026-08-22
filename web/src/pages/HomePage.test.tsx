import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import type { RecentScanItem, ScanSummary } from '../types';
import { HomePage } from './HomePage';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

const go = vi.fn<(route: Route) => void>();
const notify = vi.fn<(notice: Notice) => void>();

function anHourAgo() {
  return new Date(Date.now() - 3_600_000).toISOString();
}

function scanItem(overrides: Partial<RecentScanItem> = {}): RecentScanItem {
  return {
    id: 'scan-1',
    workspace_id: 'ws-1',
    workspace_name: 'Example API',
    state: 'completed',
    profile: 'standard',
    started_at: anHourAgo(),
    finished_at: anHourAgo(),
    total_findings: 21,
    critical_count: 3,
    high_count: 4,
    medium_count: 9,
    low_count: 5,
    info_count: 0,
    ...overrides,
  };
}

const summary: ScanSummary = {
  workspaces_total: 5,
  workspaces_scanned: 3,
  critical_count: 3,
  high_count: 4,
  medium_count: 9,
  low_count: 5,
  info_count: 0,
  total_findings: 21,
  scans_total: 12,
  scans_last_7d: 6,
  active_scans: 1,
};

function homeFetchMock(scansPayload: unknown) {
  return vi.fn((input: string, init?: RequestInit) => {
    if (input === '/api/v1/scans') return Promise.resolve(json(scansPayload));
    if (input.endsWith('/workspaces') && init?.method === 'POST') return Promise.resolve(json({ id: 'ws-new', name: 'New', root_path: 'C:\\code\\new' }));
    if (input.endsWith('/workspaces')) return Promise.resolve(json({ items: [] }));
    if (input.endsWith('/tools')) return Promise.resolve(json({ items: [] }));
    return Promise.resolve(json({ items: [] }));
  });
}

async function render(fetchMock: ReturnType<typeof vi.fn>) {
  vi.stubGlobal('fetch', fetchMock);
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<HomePage go={go} onAdd={() => {}} notify={notify} />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return host;
}

function findButton(host: HTMLElement, text: string) {
  return [...host.querySelectorAll('button')].find((button) => button.textContent === text);
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
  go.mockClear();
  notify.mockClear();
});

describe('HomePage dashboard', () => {
  it('renders the activity summary strip and recent scans from the global scans endpoint', async () => {
    const fetchMock = homeFetchMock({ scans: [scanItem()], total: 1, summary });
    const host = await render(fetchMock);
    expect(fetchMock.mock.calls.some(([input]) => input === '/api/v1/scans')).toBe(true);

    const cards = [...host.querySelectorAll('.dashboard-summary .summary-card')];
    expect(cards).toHaveLength(5);
    expect(cards.some((card) => card.textContent === '7Critical + high')).toBe(true); // 3 critical + 4 high
    expect(cards.some((card) => card.textContent === '21Total findings')).toBe(true);
    expect(cards.some((card) => card.textContent === '6Scans this week')).toBe(true);
    expect(cards.some((card) => card.textContent === '3 of 5Workspaces scanned')).toBe(true);
    expect(host.querySelector('.summary-card .pulse-dot')).not.toBeNull(); // active scans get the live dot

    const row = host.querySelector('.activity-feed .activity-row');
    expect(row).not.toBeNull();
    expect(row?.querySelector('.activity-workspace')?.textContent).toBe('Example API');
    expect(row?.querySelector('.state.completed')?.textContent).toContain('completed');
    expect(row?.querySelector('.profile-badge')?.textContent).toBe('standard');
    expect(row?.querySelector('.activity-findings')?.textContent).toContain('21 findings');
    expect(row?.querySelectorAll('.severity-dots i')).toHaveLength(4); // critical, high, medium, low present; info is zero
    expect(row?.querySelector('.activity-time')?.textContent).toBe('1 hour ago');
  });

  it('navigates to the scan from a feed row and to the workspace from its name', async () => {
    const host = await render(homeFetchMock({ scans: [scanItem()], total: 1, summary }));
    const row = host.querySelector('.activity-feed .activity-row')!;
    await act(async () => { row.querySelector<HTMLButtonElement>('.activity-detail')!.click(); });
    expect(go).toHaveBeenCalledWith({ page: 'scan', id: 'scan-1' });
    await act(async () => { row.querySelector<HTMLButtonElement>('.activity-workspace')!.click(); });
    expect(go).toHaveBeenCalledWith({ page: 'workspace', id: 'ws-1' });
  });

  it('shows the empty feed state and a disabled quick scan when there are no scans', async () => {
    const host = await render(homeFetchMock({ scans: [], total: 0, summary: { ...summary, active_scans: 0 } }));
    expect(host.textContent).toContain('No scans yet');
    expect(host.querySelector('.empty .empty-icon svg')).not.toBeNull();
    expect(host.querySelector('.summary-card .pulse-dot')).toBeNull();
    const quickScan = findButton(host, 'Scan latest workspace');
    expect(quickScan).toBeDefined();
    expect(quickScan!.disabled).toBe(true);
  });

  it('starts a scan on the most recently scanned workspace from the quick action', async () => {
    const fetchMock = vi.fn((input: string, init?: RequestInit) => {
      if (input === '/api/v1/workspaces/ws-2/scans' && init?.method === 'POST') return Promise.resolve(json({ id: 'scan-new', workspace_id: 'ws-2', state: 'queued' }));
      if (input === '/api/v1/scans') return Promise.resolve(json({ scans: [scanItem({ id: 'scan-9', workspace_id: 'ws-2', workspace_name: 'Second Project' }), scanItem()], total: 2, summary }));
      if (input.endsWith('/workspaces')) return Promise.resolve(json({ items: [] }));
      if (input.endsWith('/tools')) return Promise.resolve(json({ items: [] }));
      return Promise.resolve(json({ items: [] }));
    });
    const host = await render(fetchMock);
    const quickScan = findButton(host, 'Scan latest workspace');
    expect(quickScan).toBeDefined();
    expect(quickScan!.disabled).toBe(false);
    await act(async () => { quickScan!.click(); await Promise.resolve(); await Promise.resolve(); });
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/workspaces/ws-2/scans', expect.objectContaining({ method: 'POST' }));
    expect(go).toHaveBeenCalledWith({ page: 'scan', id: 'scan-new' });
  });
});

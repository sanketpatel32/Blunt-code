import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Workspace } from '../types';
import { filterWorkspacesByTag, sortWorkspaces, WorkspacesPage } from './WorkspacesPage';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function workspace(overrides: Partial<Workspace> & Pick<Workspace, 'id' | 'name'>): Workspace {
  return { root_path: `C:\\code\\${overrides.id}`, ...overrides };
}

const fixtures = [
  workspace({ id: 'ws-alpha', name: 'Alpha', tags: ['go', 'cli', 'windows', 'legacy'], risk: { grade: 'B', score: 41.6 }, last_scan_at: '2026-03-12T10:00:00', latest_scan: { id: 's-a', workspace_id: 'ws-alpha', state: 'completed', finished_at: '2026-03-12T10:00:00', total_findings: 8, critical_count: 1, high_count: 4, medium_count: 1, low_count: 2 } }),
  workspace({ id: 'ws-beta', name: 'beta', tags: ['Windows'], risk: { grade: 'A', score: 3.2 }, last_scan_at: '2026-03-14T09:00:00', latest_scan: { id: 's-b', workspace_id: 'ws-beta', state: 'completed_with_warnings', finished_at: '2026-03-14T09:00:00', total_findings: 1, critical_count: 0, high_count: 0, medium_count: 0, low_count: 1 } }),
  workspace({ id: 'ws-gamma', name: 'Gamma' }),
];

function json(body: unknown) {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
}

const fetchMock = vi.fn((input: string) => input.endsWith('/workspaces') ? Promise.resolve(json({ workspaces: fixtures })) : Promise.resolve(json([])));

async function renderPage() {
  vi.stubGlobal('fetch', fetchMock);
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<WorkspacesPage go={() => {}} onAdd={() => {}} notify={() => {}} />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return host;
}

function rowNames(host: HTMLElement) {
  return [...host.querySelectorAll('.ws-table tbody .ws-name')].map((name) => name.textContent);
}

function sortButton(host: HTMLElement, label: string) {
  return [...host.querySelectorAll<HTMLButtonElement>('.ws-table thead .th-sort')].find((button) => button.textContent!.startsWith(label))!;
}

async function click(host: HTMLElement, label: string) {
  await act(async () => { sortButton(host, label).click(); });
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  fetchMock.mockClear();
});

describe('sortWorkspaces ordering', () => {
  it('defaults to last scan descending and keeps never-scanned workspaces last in both directions', () => {
    expect(sortWorkspaces(fixtures, 'last_scan', 'desc').map((entry) => entry.id)).toEqual(['ws-beta', 'ws-alpha', 'ws-gamma']);
    expect(sortWorkspaces(fixtures, 'last_scan', 'asc').map((entry) => entry.id)).toEqual(['ws-alpha', 'ws-beta', 'ws-gamma']);
  });

  it('compares names case-insensitively and falls back to zero findings for unscanned workspaces', () => {
    expect(sortWorkspaces(fixtures, 'name', 'asc').map((entry) => entry.name)).toEqual(['Alpha', 'beta', 'Gamma']);
    expect(sortWorkspaces(fixtures, 'name', 'desc').map((entry) => entry.name)).toEqual(['Gamma', 'beta', 'Alpha']);
    expect(sortWorkspaces(fixtures, 'findings', 'desc').map((entry) => entry.id)).toEqual(['ws-alpha', 'ws-beta', 'ws-gamma']);
    expect(sortWorkspaces(fixtures, 'findings', 'asc').map((entry) => entry.id)).toEqual(['ws-gamma', 'ws-beta', 'ws-alpha']);
  });
});

describe('WorkspacesPage sortable columns', () => {
  it('renders newest-scanned first before any interaction', async () => {
    const host = await renderPage();
    expect(rowNames(host)).toEqual(['beta', 'Alpha', 'Gamma']);
    const active = host.querySelector('.ws-table thead .th-sort.active');
    expect(active?.textContent).toContain('Last scan');
    expect(host.querySelector('.ws-table thead th[aria-sort="descending"]')).not.toBeNull();
    expect(active?.querySelector('.sort-arrow')?.textContent).toBe('▼');
  });

  it('sorts by name ascending on first click, flips to descending on the second, and moves the arrow', async () => {
    const host = await renderPage();
    await click(host, 'Workspace');
    expect(rowNames(host)).toEqual(['Alpha', 'beta', 'Gamma']);
    expect(sortButton(host, 'Workspace').className).toBe('th-sort active');
    expect(sortButton(host, 'Workspace').querySelector('.sort-arrow')?.textContent).toBe('▲');
    expect(host.querySelector('.ws-table thead th[aria-sort="ascending"]')).not.toBeNull();
    await click(host, 'Workspace');
    expect(rowNames(host)).toEqual(['Gamma', 'beta', 'Alpha']);
    expect(sortButton(host, 'Workspace').querySelector('.sort-arrow')?.textContent).toBe('▼');
  });

  it('switches columns with a fresh descending sort for counts and dates', async () => {
    const host = await renderPage();
    await click(host, 'Findings');
    expect(rowNames(host)).toEqual(['Alpha', 'beta', 'Gamma']);
    expect(sortButton(host, 'Findings').className).toBe('th-sort active');
    expect(sortButton(host, 'Last scan').className).toBe('th-sort');
    await click(host, 'Findings');
    expect(rowNames(host)).toEqual(['Gamma', 'beta', 'Alpha']);
    await click(host, 'Last scan');
    expect(rowNames(host)).toEqual(['beta', 'Alpha', 'Gamma']);
  });
});

/** Types into the tag filter the way a real user does (native value setter bypasses React's value tracking). */
async function typeTagQuery(host: HTMLElement, value: string) {
  const input = host.querySelector<HTMLInputElement>('input[aria-label="Filter by tag"]')!;
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!;
  await act(async () => {
    setter.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

describe('WorkspacesPage row tags and filter (Loop W2)', () => {
  it('shows up to two tag chips per row and collapses the rest into +N', async () => {
    vi.useFakeTimers();
    const host = await renderPage();
    const rows = [...host.querySelectorAll('.ws-table tbody tr')];
    const alphaTags = rows[1]!.querySelectorAll('.ws-tags .tag');
    // Alpha carries four tags; only two render plus a +N chip whose title lists the hidden ones.
    expect([...alphaTags].map((chip) => chip.textContent)).toEqual(['go', 'cli', '+2']);
    expect(alphaTags[2]!.getAttribute('title')).toBe('windows, legacy');
    expect(rows[0]!.querySelectorAll('.ws-tags .tag')).toHaveLength(1);
    expect(rows[2]!.querySelector('.ws-tags')).toBeNull(); // no tags, no list
  });

  it('filters rows once typing settles and restores every row when cleared', async () => {
    vi.useFakeTimers();
    const host = await renderPage();
    expect(rowNames(host)).toEqual(['beta', 'Alpha', 'Gamma']);

    await typeTagQuery(host, 'win');
    await act(async () => { vi.advanceTimersByTime(150); });
    expect(rowNames(host)).toEqual(['beta', 'Alpha', 'Gamma']); // debounce still pending
    await act(async () => { vi.advanceTimersByTime(100); });
    expect(rowNames(host)).toEqual(['beta', 'Alpha']); // both carry a *windows* tag (case-insensitive)
    expect(host.querySelector('.ws-count')?.textContent).toBe('2 of 3 workspaces shown');

    await typeTagQuery(host, '');
    expect(rowNames(host)).toEqual(['beta', 'Alpha', 'Gamma']); // clearing bypasses the debounce
    expect(host.querySelector('.ws-count')?.textContent).toBe('3 workspaces');
  });

  it('keeps the sort order inside the filtered slice and shows the empty state for unknown tags', async () => {
    vi.useFakeTimers();
    const host = await renderPage();
    expect(filterWorkspacesByTag(fixtures, 'CLI').map((entry) => entry.id)).toEqual(['ws-alpha']); // pure helper is case-insensitive too

    await typeTagQuery(host, 'zzz-not-a-tag');
    await act(async () => { vi.advanceTimersByTime(250); });
    expect(host.querySelectorAll('.ws-table tbody tr')).toHaveLength(0);
    expect(host.querySelector('.empty h2')?.textContent).toBe('No workspaces match this tag');
    expect(host.textContent).toContain('zzz-not-a-tag');
  });
});

describe('WorkspacesPage risk chips (Loop W3)', () => {
  it('grades the RISK column from the latest finished scan — the same math as the board', async () => {
    vi.useFakeTimers();
    const host = await renderPage();
    const rows = [...host.querySelectorAll('.ws-table tbody tr')];
    const betaRisk = rows[0]!.querySelector('.ws-risk')!;
    expect(betaRisk.textContent).toBe('A'); // grade only; compact
    expect(betaRisk.className).toContain('state success');
    expect(betaRisk.getAttribute('title')).toBe('Weighted risk score 1 from the latest finished scan');
    // The fixture's stale payload `risk: { grade: 'B', score: 41.6 }` MUST lose to
    // the computed grade — no endpoint populates that field, the client does the math.
    const alphaRisk = rows[1]!.querySelector('.ws-risk')!;
    expect(alphaRisk.textContent).toBe('C');
    expect(alphaRisk.className).toContain('state failed');
    expect(alphaRisk.getAttribute('title')).toBe('Weighted risk score 34 from the latest finished scan');
    expect(rows[2]!.querySelector('.ws-risk')).toBeNull(); // never scanned → no chip
    expect(rows[2]!.querySelector('.ws-cell-risk')?.textContent).toBe('Never scanned');
  });
});

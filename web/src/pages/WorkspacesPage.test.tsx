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
  workspace({ id: 'ws-alpha', name: 'Alpha', tags: ['go', 'cli', 'windows', 'legacy'], risk: { grade: 'B', score: 41.6 }, last_scan_at: '2026-03-12T10:00:00', latest_scan: { id: 's-a', workspace_id: 'ws-alpha', state: 'completed', finished_at: '2026-03-12T10:00:00', total_findings: 7 } }),
  workspace({ id: 'ws-beta', name: 'beta', tags: ['Windows'], risk: { grade: 'A', score: 3.2 }, last_scan_at: '2026-03-14T09:00:00', latest_scan: { id: 's-b', workspace_id: 'ws-beta', state: 'completed_with_warnings', finished_at: '2026-03-14T09:00:00', total_findings: 2 } }),
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

function cardNames(host: HTMLElement) {
  return [...host.querySelectorAll('.workspace-card h3')].map((heading) => heading.textContent);
}

function sortButton(host: HTMLElement, label: string) {
  return [...host.querySelectorAll<HTMLButtonElement>('.workspace-sortbar .th-sort')].find((button) => button.textContent!.startsWith(label))!;
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
    expect(cardNames(host)).toEqual(['beta', 'Alpha', 'Gamma']);
    const active = host.querySelector('.workspace-sortbar .th-sort.active');
    expect(active?.textContent).toContain('Last scan');
    expect(active?.querySelector('.sort-arrow')?.textContent).toBe('▼');
  });

  it('sorts by name ascending on first click, flips to descending on the second, and moves the arrow', async () => {
    const host = await renderPage();
    await click(host, 'Name');
    expect(cardNames(host)).toEqual(['Alpha', 'beta', 'Gamma']);
    expect(sortButton(host, 'Name').className).toBe('th-sort active');
    expect(sortButton(host, 'Name').querySelector('.sort-arrow')?.textContent).toBe('▲');
    await click(host, 'Name');
    expect(cardNames(host)).toEqual(['Gamma', 'beta', 'Alpha']);
    expect(sortButton(host, 'Name').querySelector('.sort-arrow')?.textContent).toBe('▼');
  });

  it('switches columns with a fresh descending sort for counts and dates', async () => {
    const host = await renderPage();
    await click(host, 'Findings');
    expect(cardNames(host)).toEqual(['Alpha', 'beta', 'Gamma']);
    expect(sortButton(host, 'Findings').className).toBe('th-sort active');
    expect(sortButton(host, 'Last scan').className).toBe('th-sort');
    await click(host, 'Findings');
    expect(cardNames(host)).toEqual(['Gamma', 'beta', 'Alpha']);
    await click(host, 'Last scan');
    expect(cardNames(host)).toEqual(['beta', 'Alpha', 'Gamma']);
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

describe('WorkspacesPage tag chips and filter (Loop W2)', () => {
  it('shows up to three tag chips per card and collapses the rest into +N', async () => {
    vi.useFakeTimers();
    const host = await renderPage();
    const alphaTags = [...host.querySelectorAll('.workspace-card')][1]!.querySelectorAll('.workspace-tags .badge');
    // Alpha carries four tags; only three render plus a +N chip whose title lists the hidden one.
    expect([...alphaTags].map((badge) => badge.textContent)).toEqual(['go', 'cli', 'windows', '+1']);
    expect(alphaTags[3]!.getAttribute('title')).toBe('legacy');
    expect([...host.querySelectorAll('.workspace-card')][0]!.querySelectorAll('.workspace-tags .badge')).toHaveLength(1);
    expect([...host.querySelectorAll('.workspace-card')][2]!.querySelector('.workspace-tags')).toBeNull(); // no tags, no list
  });

  it('filters cards once typing settles and restores every card when cleared', async () => {
    vi.useFakeTimers();
    const host = await renderPage();
    expect(cardNames(host)).toEqual(['beta', 'Alpha', 'Gamma']);

    await typeTagQuery(host, 'win');
    await act(async () => { vi.advanceTimersByTime(150); });
    expect(cardNames(host)).toEqual(['beta', 'Alpha', 'Gamma']); // debounce still pending
    await act(async () => { vi.advanceTimersByTime(100); });
    expect(cardNames(host)).toEqual(['beta', 'Alpha']); // both carry a *windows* tag (case-insensitive)
    expect(host.querySelector('.workspace-filter-count')?.textContent).toBe('2 of 3 workspaces shown');

    await typeTagQuery(host, '');
    expect(cardNames(host)).toEqual(['beta', 'Alpha', 'Gamma']); // clearing bypasses the debounce
    expect(host.querySelector('.workspace-filter-count')).toBeNull();
  });

  it('keeps the sort order inside the filtered slice and shows the empty state for unknown tags', async () => {
    vi.useFakeTimers();
    const host = await renderPage();
    expect(filterWorkspacesByTag(fixtures, 'CLI').map((entry) => entry.id)).toEqual(['ws-alpha']); // pure helper is case-insensitive too

    await typeTagQuery(host, 'zzz-not-a-tag');
    await act(async () => { vi.advanceTimersByTime(250); });
    expect(host.querySelectorAll('.workspace-card')).toHaveLength(0);
    expect(host.querySelector('.empty h2')?.textContent).toBe('No workspaces match this tag');
    expect(host.textContent).toContain('zzz-not-a-tag');
  });
});

describe('WorkspaceCard risk badges (Loop W3)', () => {
  it('renders an optional graded badge with the score rounded and severity tones', async () => {
    vi.useFakeTimers();
    const host = await renderPage();
    const cards = [...host.querySelectorAll('.workspace-card')];
    const betaRisk = cards[0]!.querySelector('.risk-badge')!;
    expect(betaRisk.textContent).toBe('A · 3'); // 3.2 rounds down
    expect(betaRisk.className).toContain('state success');
    const alphaRisk = cards[1]!.querySelector('.risk-badge')!;
    expect(alphaRisk.textContent).toBe('B · 42'); // 41.6 rounds up
    expect(alphaRisk.className).toContain('state warning');
    expect(cards[2]!.querySelector('.risk-badge')).toBeNull(); // risk absent → no badge
  });
});

import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Workspace } from '../types';
import { sortWorkspaces, WorkspacesPage } from './WorkspacesPage';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function workspace(overrides: Partial<Workspace> & Pick<Workspace, 'id' | 'name'>): Workspace {
  return { root_path: `C:\\code\\${overrides.id}`, ...overrides };
}

const fixtures = [
  workspace({ id: 'ws-alpha', name: 'Alpha', last_scan_at: '2026-03-12T10:00:00', latest_scan: { id: 's-a', workspace_id: 'ws-alpha', state: 'completed', finished_at: '2026-03-12T10:00:00', total_findings: 7 } }),
  workspace({ id: 'ws-beta', name: 'beta', last_scan_at: '2026-03-14T09:00:00', latest_scan: { id: 's-b', workspace_id: 'ws-beta', state: 'completed_with_warnings', finished_at: '2026-03-14T09:00:00', total_findings: 2 } }),
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

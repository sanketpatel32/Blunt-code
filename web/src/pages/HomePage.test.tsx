import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import type { RecentScanItem, Scan, ScanSummary, Workspace } from '../types';
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

/** Latest-scan payload hung off a workspace — what the ledger and verdict grade. */
function latestScan(overrides: Partial<Scan> = {}): Scan {
  return {
    id: 'scan-1',
    workspace_id: 'ws-1',
    state: 'completed',
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

function workspaceItem(overrides: Partial<Workspace> = {}): Workspace {
  return {
    id: 'ws-1',
    name: 'Example API',
    root_path: 'C:\\code\\example',
    languages: ['Python'],
    latest_scan: latestScan(),
    ...overrides,
  };
}

function homeFetchMock(
  scansPayload: unknown,
  workspacesPayload: unknown = { items: [workspaceItem()] },
  toolsPayload: unknown = { items: [{ id: 'ruff', ready: true }, { id: 'semgrep', ready: true }] },
) {
  return vi.fn((input: string, init?: RequestInit) => {
    if (input === '/api/v1/scans') return Promise.resolve(json(scansPayload));
    if (input.endsWith('/workspaces') && init?.method === 'POST') return Promise.resolve(json({ id: 'ws-new', name: 'New', root_path: 'C:\\code\\new' }));
    if (input.endsWith('/workspaces')) return Promise.resolve(json(workspacesPayload));
    if (input.endsWith('/tools')) return Promise.resolve(json(toolsPayload));
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
  return [...host.querySelectorAll('button')].find((button) => button.textContent?.includes(text));
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
  go.mockClear();
  notify.mockClear();
});

describe('HomePage risk board — verdict', () => {
  it('grades the whole board with the backend formula (3c+4h+9m+5l → score 73, grade D)', async () => {
    const host = await render(homeFetchMock({ scans: [scanItem()], total: 1, summary }));

    expect(host.querySelector('.verdict-letter')?.textContent).toBe('D');
    expect(host.querySelector('.verdict-score')?.textContent).toContain('73');
    expect(host.textContent).toContain('Critical risk.');
    // Band ruler marks D active.
    const activeBand = host.querySelector('.verdict-band[data-active="true"]');
    expect(activeBand?.textContent).toContain('D');
    expect(activeBand?.textContent).toContain('50+');
  });

  it('tallies severity from the latest completed scan of each workspace', async () => {
    const host = await render(homeFetchMock({ scans: [scanItem()], total: 1, summary }));
    expect(host.textContent).toContain('21 findings across the latest completed scan of 1 workspace.');

    const bar = host.querySelector('.verdict-bar')!;
    expect(bar.querySelectorAll('i')).toHaveLength(4); // critical, high, medium, low — info is zero
    const legend = [...host.querySelectorAll('.verdict-legend li')];
    expect(legend.map((li) => li.textContent)).toEqual(['critical3', 'high4', 'medium9', 'low5', 'info0']);
  });

  it('shows the real activity stats in the rail, not scores invented by the UI', async () => {
    const host = await render(homeFetchMock({ scans: [scanItem()], total: 1, summary }));

    const rail = host.querySelector('.verdict-rail')!;
    expect(rail.textContent).toContain('Active scans');
    expect(rail.textContent).toContain('1');
    expect(rail.querySelector('.pulse-dot')).not.toBeNull(); // live dot while scans run
    expect(rail.textContent).toContain('Scans this week');
    expect(rail.textContent).toContain('6');
    expect(rail.textContent).toContain('Workspaces scanned');
    expect(rail.textContent).toContain('1 of 1'); // from the workspace list, not the stale summary shape
    expect(rail.textContent).toContain('Engines');
    expect(rail.textContent).toContain('2 of 2');
  });

  it('flags running scans in the page header badge', async () => {
    const host = await render(homeFetchMock({ scans: [scanItem()], total: 1, summary: { ...summary, active_scans: 2 } }));
    expect(host.querySelector('.board-live')?.textContent).toContain('2 scans running');
  });

  it('says not graded yet when workspaces exist but none finished a scan', async () => {
    const host = await render(homeFetchMock(
      { scans: [], total: 0, summary: { ...summary, active_scans: 0 } },
      { items: [workspaceItem({ latest_scan: undefined })] },
    ));
    expect(host.querySelector('.verdict-grade')?.getAttribute('data-grade')).toBe('none');
    expect(host.textContent).toContain('No completed scans yet');
    expect(host.querySelector('.verdict-band[data-active="true"]')).toBeNull();
  });

  it('drills into findings search from the severity tally', async () => {
    const host = await render(homeFetchMock({ scans: [scanItem()], total: 1, summary }));
    const explore = findButton(host, 'Explore findings');
    expect(explore).toBeDefined();
    await act(async () => { explore!.click(); });
    expect(go).toHaveBeenCalledWith({ page: 'search' });
  });
});

describe('HomePage risk board — ledger', () => {
  it('ranks workspaces by weighted risk and pushes never-scanned to the bottom', async () => {
    const risky = workspaceItem({ id: 'ws-risky', name: 'Risky Core', latest_scan: latestScan({ critical_count: 1, high_count: 1, medium_count: 0, low_count: 0, total_findings: 2 }) }); // 10+5 = 15 → B
    const mild = workspaceItem({ id: 'ws-mild', name: 'Mild Web', latest_scan: latestScan({ critical_count: 0, high_count: 0, medium_count: 0, low_count: 2, total_findings: 2 }) }); // 2 → A
    const fresh = workspaceItem({ id: 'ws-fresh', name: 'Fresh Add', latest_scan: undefined });
    const host = await render(homeFetchMock(
      { scans: [scanItem()], total: 1, summary },
      { items: [fresh, mild, risky] }, // shuffled on purpose
    ));

    const rows = [...host.querySelectorAll('.ledger-row')];
    expect(rows.map((row) => row.querySelector('.ledger-name')?.textContent)).toEqual(['Risky Core', 'Mild Web', 'Fresh Add']);
    expect(rows[0].querySelector('.ledger-grade')?.textContent).toBe('B');
    expect(rows[0].querySelector('.ledger-score')?.textContent).toContain('15');
    expect(rows[1].querySelector('.ledger-grade')?.textContent).toBe('A');
    expect(rows[2].querySelector('.ledger-grade')?.textContent).toBe('–');
    expect(rows[2].textContent).toContain('Never scanned');
    expect(rows[2].querySelector('.ledger-score-none')?.textContent).toBe('—');
  });

  it('ignores non-terminal latest scans for risk and shows their live state instead', async () => {
    const running = workspaceItem({ id: 'ws-live', name: 'Live One', latest_scan: latestScan({ state: 'running', finished_at: null, critical_count: 9 }) });
    const host = await render(homeFetchMock({ scans: [], total: 0, summary }, { items: [running] }));

    expect(host.querySelector('.ledger-grade')?.textContent).toBe('–'); // running counts are not risk
    expect(host.textContent).toContain('Scan running');
    expect(host.querySelector('.verdict-grade')?.getAttribute('data-grade')).toBe('none');
  });

  it('navigates to the workspace from the ledger name and to workspaces from the footer', async () => {
    const host = await render(homeFetchMock({ scans: [scanItem()], total: 1, summary }));
    await act(async () => { host.querySelector<HTMLButtonElement>('.ledger-name')!.click(); });
    expect(go).toHaveBeenCalledWith({ page: 'workspace', id: 'ws-1' });
    const all = findButton(host, 'All workspaces');
    await act(async () => { all!.click(); });
    expect(go).toHaveBeenCalledWith({ page: 'workspaces' });
  });

  it('offers a scan action and a guarded remove per row', async () => {
    const host = await render(homeFetchMock({ scans: [scanItem()], total: 1, summary }));
    const remove = [...host.querySelectorAll('button')].find((button) => button.textContent === 'Remove');
    expect(remove).toBeDefined();
    await act(async () => { remove!.click(); });
    expect(host.textContent).toContain('Remove this workspace?');
    expect(host.textContent).toContain('Your project files will not be changed.');
    expect(host.querySelector('.ledger-row .ledger-actions button'));// scan action dropdown trigger lives in the row
  });
});

describe('HomePage risk board — activity feed', () => {
  it('renders recent scans with state, profile, findings and relative time', async () => {
    const host = await render(homeFetchMock({ scans: [scanItem()], total: 1, summary }));

    const row = host.querySelector('.feed-list .feed-row')!;
    expect(row.querySelector('.feed-workspace')?.textContent).toBe('Example API');
    expect(row.querySelector('.feed-state.success')?.textContent).toBe('Completed');
    expect(row.querySelector('.feed-profile')?.textContent).toBe('standard');
    expect(row.querySelector('.feed-findings')?.textContent).toContain('21 findings');
    expect(row.querySelectorAll('.severity-dots i')).toHaveLength(4); // critical, high, medium, low; info is zero
    expect(row.querySelector('.feed-time')?.textContent).toBe('1 hour ago');
  });

  it('draws the trend from real per-scan totals, oldest to newest', async () => {
    const scans = [
      scanItem({ id: 'a', total_findings: 10 }),
      scanItem({ id: 'b', total_findings: 4 }),
      scanItem({ id: 'c', total_findings: 21 }),
    ];
    const host = await render(homeFetchMock({ scans, total: 3, summary }));
    const bars = host.querySelectorAll('.trend-bars i');
    expect(bars).toHaveLength(3);
    expect(host.querySelector('.board-trend-label')?.textContent).toContain('Findings per recent scan');
  });

  it('hides the trend until at least two scans exist', async () => {
    const host = await render(homeFetchMock({ scans: [scanItem()], total: 1, summary }));
    expect(host.querySelector('.trend-bars')).toBeNull();
  });

  it('filters the feed by status chip', async () => {
    const scans = [
      scanItem({ id: 'scan-1', state: 'completed' }),
      scanItem({ id: 'scan-2', state: 'running', finished_at: null }),
      scanItem({ id: 'scan-3', state: 'completed_with_warnings' }),
    ];
    const host = await render(homeFetchMock({ scans, total: 3, summary }));
    const runningTab = [...host.querySelectorAll<HTMLButtonElement>('.feed-filter-btn')].find((b) => b.textContent === 'Running');
    expect(runningTab).toBeDefined();

    await act(async () => { runningTab!.click(); });
    const rows = host.querySelectorAll('.feed-list .feed-row');
    expect(rows).toHaveLength(1);
    expect(rows[0].querySelector('.feed-state.accent')?.textContent).toBe('Running');
  });

  it('navigates to the scan report and the workspace from a feed row', async () => {
    const host = await render(homeFetchMock({ scans: [scanItem()], total: 1, summary }));
    const row = host.querySelector('.feed-list .feed-row')!;
    await act(async () => { row.querySelector<HTMLButtonElement>('.feed-detail')!.click(); });
    expect(go).toHaveBeenCalledWith({ page: 'scan', id: 'scan-1' });
    await act(async () => { row.querySelector<HTMLButtonElement>('.feed-workspace')!.click(); });
    expect(go).toHaveBeenCalledWith({ page: 'workspace', id: 'ws-1' });
  });

  it('shows the empty feed state and a disabled quick scan when there are no scans', async () => {
    const host = await render(homeFetchMock(
      { scans: [], total: 0, summary: { ...summary, active_scans: 0 } },
      { items: [workspaceItem()] },
    ));
    expect(host.textContent).toContain('No scans yet');
    expect(host.querySelector('.empty .empty-icon svg')).not.toBeNull();
    expect(host.querySelector('.verdict-rail .pulse-dot')).toBeNull();
    const quickScan = findButton(host, 'Scan latest workspace');
    expect(quickScan).toBeDefined();
    expect(quickScan!.disabled).toBe(true);
  });
});

describe('HomePage quick actions', () => {
  it('starts a scan on the most recently scanned workspace and follows it', async () => {
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

  it('shows the first-run onboarding when nothing exists yet', async () => {
    const host = await render(homeFetchMock({ scans: [], total: 0 }, { items: [] }));
    expect(host.textContent).toContain('Point Blunt Code at a project');
    expect(host.textContent).toContain('Add your first workspace');
    expect(host.querySelector('.board-verdict')).toBeNull();
  });
});

describe('HomePage engines foot', () => {
  it('reports real tool readiness with per-engine chips', async () => {
    const host = await render(homeFetchMock(
      { scans: [scanItem()], total: 1, summary },
      undefined,
      { items: [{ id: 'ruff', ready: true }, { id: 'semgrep', ready: true }, { id: 'gitleaks', ready: false }] },
    ));
    expect(host.querySelector('.board-foot-note')?.textContent).toContain('2 of 3 engines ready');
    const chips = [...host.querySelectorAll('.board-foot-chip')];
    expect(chips.map((chip) => chip.textContent)).toEqual(['ruff', 'semgrep', 'gitleaks']);
    expect(chips[2].className).toContain('pending');
  });
});

describe('HomePage workspace paths and languages', () => {
  const DEEP_PATH = 'C:\\Users\\sanpa\\OneDrive\\Desktop\\Claire\\claire-core\\src\\Suremed_agent\\complete_prod';

  function stubClipboard() {
    const writeText = vi.fn(() => Promise.resolve());
    Object.defineProperty(window.navigator, 'clipboard', { value: { writeText }, configurable: true });
    return writeText;
  }

  it('keeps shallow paths whole and offers a copy button for the full path', async () => {
    const host = await render(homeFetchMock({ scans: [], total: 0, summary }));
    const path = host.querySelector('.ledger-identity .path-copy code')!;
    expect(path.textContent).toBe('C:\\code\\example');
    expect(path.getAttribute('title')).toBe('C:\\code\\example');
    const copy = host.querySelector<HTMLButtonElement>('.ledger-identity .path-copy-button')!;
    expect(copy.getAttribute('aria-label')).toBe('Copy full path');
  });

  it('shows deep paths as their last two segments and copies the full path on click', async () => {
    const writeText = stubClipboard();
    const host = await render(homeFetchMock(
      { scans: [], total: 0, summary },
      { items: [workspaceItem({ name: 'Deep Project', root_path: DEEP_PATH })] },
    ));
    const path = host.querySelector('.ledger-identity .path-copy code')!;
    expect(path.textContent).toBe('…\\Suremed_agent\\complete_prod');
    expect(path.getAttribute('title')).toBe(DEEP_PATH);
    const copy = host.querySelector<HTMLButtonElement>('.ledger-identity .path-copy-button')!;
    await act(async () => { copy.click(); await Promise.resolve(); await Promise.resolve(); });
    expect(writeText).toHaveBeenCalledWith(DEEP_PATH);
    expect(copy.getAttribute('aria-label')).toBe('Copied to clipboard');
    expect(copy.className).toContain('copied');
    Reflect.deleteProperty(window.navigator, 'clipboard');
  });

  it('renders one colored-dot badge per detected language', async () => {
    const host = await render(homeFetchMock(
      { scans: [], total: 0, summary },
      { items: [workspaceItem({ name: 'Polyglot', root_path: 'C:\\code\\poly', languages: ['python', 'dockerfile', 'exoticlang'] })] },
    ));
    const badges = [...host.querySelectorAll('.ledger-identity .badges li')];
    expect(badges.map((badge) => badge.textContent)).toEqual(['Python', 'Dockerfile', 'exoticlang']);
    const dots = [...host.querySelectorAll('.ledger-identity .badges .lang-dot')] as HTMLElement[];
    expect(dots).toHaveLength(3);
    expect(dots[0].style.background).toBe('rgb(53, 114, 165)'); // python #3572A5
    expect(dots[2].style.background).not.toContain('#'); // unknown language falls back to the accent token
  });

  it('caps ledger language chips at four plus an overflow count', async () => {
    const many = ['python', 'typescript', 'go', 'rust', 'ruby', 'haskell', 'lua'];
    const host = await render(homeFetchMock(
      { scans: [], total: 0, summary },
      { items: [workspaceItem({ name: 'Polyglot', root_path: 'C:\\code\\poly', languages: many })] },
    ));
    const badges = [...host.querySelectorAll('.ledger-identity .badges li')];
    expect(badges.map((badge) => badge.textContent)).toEqual(['Python', 'TypeScript', 'Go', 'Rust', '+3']);
    const overflow = badges[4] as HTMLElement;
    expect(overflow.title).toContain('Also detected: Ruby, Haskell, Lua');
  });

  it('tones completed_with_warnings as a warning and labels it in sentence case', async () => {
    const warned = workspaceItem({
      name: 'Warned',
      latest_scan: latestScan({ state: 'completed_with_warnings', total_findings: 5, critical_count: 0, high_count: 0, medium_count: 2, low_count: 3 }),
    });
    const host = await render(homeFetchMock({ scans: [], total: 0, summary }, { items: [warned] }));
    const badge = host.querySelector('.ledger-last > div');
    expect(badge?.textContent).toBe('Completed with warnings');
    expect(badge?.className).toContain('text-[var(--color-warning)]');
  });
});

import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ReactElement } from 'react';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import type { RiskProfile, Scan, Workspace } from '../types';
import { RiskCard, WorkspacePage } from './WorkspaceDetailPage';

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

/** The workspace payload's latest_scan never carries `snapshot` — that field only ships on scan-list rows. */
function workspacePayload(overrides: Partial<Workspace> = {}): Workspace {
  return {
    id: 'ws-1',
    name: 'Claire Frontend',
    root_path: 'C:\\code\\claire-frontend',
    languages: ['TypeScript', 'Go'],
    latest_scan: { id: 'scan-1', workspace_id: 'ws-1', state: 'completed', total_findings: 3 },
    ...overrides,
  };
}

function scanRow(overrides: Partial<Scan> = {}): Scan {
  return { id: 'scan-1', workspace_id: 'ws-1', state: 'completed', total_findings: 3, ...overrides };
}

function detailFetchMock(scanList: unknown, workspace: Workspace = workspacePayload(), risk: unknown = { available: false }) {
  return vi.fn((input: string) => {
    if (input.endsWith('/workspaces/ws-1/scans')) return Promise.resolve(json(scanList));
    if (input.endsWith('/workspaces/ws-1/risk')) return Promise.resolve(json(risk));
    if (input.endsWith('/workspaces/ws-1')) return Promise.resolve(json(workspace));
    return Promise.resolve(json({ items: [] }));
  });
}

async function render(fetchMock: ReturnType<typeof vi.fn>) {
  vi.stubGlobal('fetch', fetchMock);
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<WorkspacePage id="ws-1" go={go} notify={notify} />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return host;
}

/** Direct render for exported subcomponents (RiskCard trend words) without page-level fetches. */
async function renderElement(ui: ReactElement) {
  vi.stubGlobal('fetch', vi.fn());
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(ui); });
  await act(async () => { await Promise.resolve(); });
  return host;
}

async function openLanguagesTab(host: HTMLElement) {
  const tab = [...host.querySelectorAll<HTMLButtonElement>('[role="tab"]')].find((button) => button.textContent === 'Languages');
  expect(tab).toBeDefined();
  // Radix tabs activate on mousedown, which jsdom's click() does not synthesize.
  await act(async () => {
    tab!.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, button: 0 }));
    await Promise.resolve();
    await Promise.resolve();
  });
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
  go.mockClear();
  notify.mockClear();
});

describe('WorkspaceDetailPage per-language coverage', () => {
  it('backfills the workspace latest_scan snapshot from the scans list so the Languages tab shows real counts', async () => {
    const host = await render(detailFetchMock({
      scans: [scanRow({ snapshot: { languages: { TypeScript: 42, Go: 28 } } })],
    }));
    await openLanguagesTab(host);

    // The donut replaced bare pills: per-language files render with a real total.
    const donut = host.querySelector('section[aria-label="Language distribution"]');
    expect(donut?.textContent).toContain('TypeScript');
    expect(donut?.textContent).toContain('42');
    expect(donut?.textContent).toContain('70'); // total files in the donut center
    expect(donut?.querySelector('svg[role="img"]')).not.toBeNull();
    expect(donut?.textContent).not.toContain('No discovery snapshot available');
  });

  it('keeps the honest empty note when no scan in the history recorded a snapshot', async () => {
    const host = await render(detailFetchMock({ scans: [scanRow()] }));
    await openLanguagesTab(host);

    const donut = host.querySelector('section[aria-label="Language distribution"]');
    expect(donut?.querySelector('svg[role="img"]')).toBeNull(); // pills, not an invented chart
    expect(donut?.textContent).toContain('No discovery snapshot available for this scan yet.');
  });

  it('backfills the snapshot from the completed fallback scan when the latest scan was cancelled', async () => {
    // The coverage backfill must follow the effective (completed) scan, or the
    // Languages tab loses its counts exactly on the cancelled-latest workspaces.
    const workspace = workspacePayload({
      latest_scan: { id: 'scan-2', workspace_id: 'ws-1', state: 'cancelled', total_findings: 0 },
    });
    const host = await render(detailFetchMock({
      scans: [
        scanRow({ id: 'scan-2', state: 'cancelled', total_findings: 0, finished_at: '2026-09-15T18:59:26Z' }),
        scanRow({ id: 'scan-1', state: 'completed', total_findings: 493, finished_at: '2026-09-15T04:20:51Z', snapshot: { languages: { TypeScript: 42, Go: 28 } } }),
      ],
    }, workspace));
    await openLanguagesTab(host);

    const donut = host.querySelector('section[aria-label="Language distribution"]');
    expect(donut?.textContent).toContain('70'); // counts came from the completed scan's snapshot
    expect(donut?.querySelector('svg[role="img"]')).not.toBeNull();
  });
});

describe('WorkspaceDetailPage header language summary (D2 audit)', () => {
  it('caps the language dots at three and folds the rest into a "+N more" tag', async () => {
    const workspace = workspacePayload({ languages: ['TypeScript', 'Go', 'Python', 'Rust', 'Shell'] });
    const host = await render(detailFetchMock({ scans: [scanRow()] }, workspace));

    const badge = host.querySelector('[aria-label="Detected languages"]');
    expect(badge).not.toBeNull();
    expect(badge?.querySelectorAll('.ws-lang-dot')).toHaveLength(3); // color is signal: max three dots
    expect(badge?.querySelector('.tag')?.textContent).toBe('+2 more');
    expect(badge?.querySelector('.tag')?.getAttribute('title')).toContain('Rust'); // the full list survives on hover
  });
});

describe('WorkspaceDetailPage cancelled-latest fallback (C2)', () => {
  it('grades the metric cards on the last completed scan and says so in plain words', async () => {
    const workspace = workspacePayload({
      latest_scan: { id: 'scan-2', workspace_id: 'ws-1', state: 'cancelled', total_findings: 0, critical_count: 0, high_count: 0, error_summary: 'Cancelled by user.' },
    });
    const risk: RiskProfile = { available: true, score: 950, grade: 'D', trend: 'down', previous_score: 954 };
    const host = await render(detailFetchMock({
      scans: [
        scanRow({ id: 'scan-2', state: 'cancelled', total_findings: 0, critical_count: 0, high_count: 0, finished_at: '2026-09-15T18:59:26Z', analyzer_runs: [{ analyzer_id: 'gitleaks-secrets', status: 'succeeded' }] }),
        scanRow({ id: 'scan-1', state: 'completed', total_findings: 493, critical_count: 12, high_count: 30, finished_at: '2026-09-15T04:20:51Z' }),
      ],
    }, workspace, risk));

    const verdict = host.querySelector('section[aria-label="Latest scan summary"]');
    const values = [...verdict!.querySelectorAll('.summary-grid strong')].map((el) => el.textContent);
    expect(values).toContain('493'); // total findings from the completed scan…
    expect(values).toContain('42'); // …and its 12 critical + 30 high, not the cancelled zeros
    expect(values).not.toContain('0'); // the cancelled scan's zeros never reach the cards

    // The one-line bridge near the Latest-scan panel.
    expect(host.textContent).toContain('Last scan was cancelled — showing the last completed scan (493 findings,');
    // The panel still truthfully reports the attempt that ran last.
    expect(host.textContent).toContain('cancelled ·');

    // C4: analyzer rows read like products; the raw id never leaks into the text.
    expect(host.textContent).toContain('Gitleaks');
    expect(host.textContent).not.toContain('gitleaks-secrets');
  });

  it('renders an unscanned workspace as "—" with a "no scan yet" caption instead of zeros', async () => {
    const workspace = workspacePayload({ latest_scan: undefined });
    const host = await render(detailFetchMock({ scans: [] }, workspace));

    const verdict = host.querySelector('section[aria-label="Latest scan summary"]');
    const values = [...verdict!.querySelectorAll('.summary-grid strong')].map((el) => el.textContent);
    expect(values.length).toBeGreaterThan(0);
    expect(values.every((value) => value === '—')).toBe(true); // risk + crit/high + total: no fake 0s
    expect(verdict?.textContent).toContain('no scan yet');
  });
});

describe('RiskCard trend words (C8, D2 audit: grade owns the color)', () => {
  it('says "worsened" with a neutral delta note and a decorative arrow when the trend is up', async () => {
    const host = await renderElement(<RiskCard risk={{ available: true, score: 60, grade: 'C', trend: 'up', previous_score: 55 }} />);
    expect(host.textContent).toContain('worsened 5 pts');
    const note = host.querySelector('.risk-trend');
    expect(note?.querySelector('svg[aria-hidden="true"]')).not.toBeNull(); // arrow icon, hidden from screen readers
    expect(note?.className).not.toContain('text-[var(--color-success)]');
    expect(note?.className).not.toContain('text-[var(--color-warning)]'); // neutral ink — no second verdict color beside the grade
    expect(host.textContent).not.toContain('▲');
  });

  it('says "improved … since last scan" with a neutral delta note when the trend is down', async () => {
    const host = await renderElement(<RiskCard risk={{ available: true, score: 950, grade: 'D', trend: 'down', previous_score: 954 }} />);
    expect(host.textContent).toContain('improved 4 pts since last scan');
    const note = host.querySelector('.risk-trend');
    expect(note?.querySelector('svg[aria-hidden="true"]')).not.toBeNull();
    expect(note?.className).not.toContain('text-[var(--color-success)]'); // neutral ink — a green note beside a red grade read as two verdicts
    expect(host.textContent).not.toContain('▼');
  });

  it('says "no change" for a flat trend', async () => {
    const host = await renderElement(<RiskCard risk={{ available: true, score: 50, grade: 'C', trend: 'flat', previous_score: 50 }} />);
    expect(host.textContent).toContain('no change');
  });

  it('explains the score on hover (C3) and rewords partial coverage', async () => {
    const host = await renderElement(<RiskCard risk={{ available: true, score: 60, grade: 'C', complete: false, coverage: { total: 10, succeeded: 9, failed: 1, warned: 0 } }} />);
    expect(host.querySelector('.risk-score')?.getAttribute('title')).toContain('critical ×10');
    expect(host.textContent).toContain('9 of 10 analyzers completed');
    expect(host.textContent).not.toContain('9/10');
  });
});

import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import type { Scan, Workspace } from '../types';
import { WorkspacePage } from './WorkspaceDetailPage';

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
function workspacePayload(): Workspace {
  return {
    id: 'ws-1',
    name: 'Claire Frontend',
    root_path: 'C:\\code\\claire-frontend',
    languages: ['TypeScript', 'Go'],
    latest_scan: { id: 'scan-1', workspace_id: 'ws-1', state: 'completed', total_findings: 3 },
  };
}

function scanRow(overrides: Partial<Scan> = {}): Scan {
  return { id: 'scan-1', workspace_id: 'ws-1', state: 'completed', total_findings: 3, ...overrides };
}

function detailFetchMock(scanList: unknown) {
  return vi.fn((input: string) => {
    if (input.endsWith('/workspaces/ws-1/scans')) return Promise.resolve(json(scanList));
    if (input.endsWith('/workspaces/ws-1/risk')) return Promise.resolve(json({ available: false }));
    if (input.endsWith('/workspaces/ws-1')) return Promise.resolve(json(workspacePayload()));
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
});

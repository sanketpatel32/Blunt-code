import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { SuppressionsSection } from './SuppressionsPanel';
import type { Notice } from '../lib/notice';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

const ALPHA = 'a'.repeat(64);
const BETA = 'b'.repeat(64);
const items = [
  { workspace_id: 'ws-1', fingerprint: ALPHA, reason: 'False positive for this project', created_at: '2026-08-20T10:00:00Z' },
  { workspace_id: 'ws-1', fingerprint: BETA, created_at: '2026-08-21T11:30:00Z' },
];

const fetchMock = vi.fn((input: string, _init?: RequestInit) => {
  if (input.endsWith('/workspaces/ws-1/suppressions')) return Promise.resolve(json({ items }));
  return Promise.resolve(json({ items: [] }));
});

function suppressionCalls() {
  return fetchMock.mock.calls.map(([input]) => String(input)).filter((url) => url.includes('/suppressions'));
}

async function render() {
  vi.stubGlobal('fetch', fetchMock);
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<SuppressionsSection workspaceId="ws-1" notify={(_n: Notice) => {}} />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return host;
}

async function click(element: Element) {
  await act(async () => { (element as HTMLButtonElement).click(); });
}

beforeEach(() => { fetchMock.mockClear(); });

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

describe('SuppressionsSection', () => {
  it('lists suppressions with a shortened fingerprint, the reason, and the created date', async () => {
    const host = await render();
    const rows = [...host.querySelectorAll('.suppression-row')];
    expect(rows).toHaveLength(2);
    expect(rows[0].querySelector('.suppression-fingerprint')!.textContent).toBe(`${'a'.repeat(12)}…`); // full 64-char hex is unreadable; hover carries it
    expect(rows[0].querySelector('.suppression-fingerprint')!.getAttribute('title')).toBe(ALPHA);
    expect(rows[0].querySelector('.suppression-reason')!.textContent).toBe('False positive for this project');
    expect(rows[0].querySelector('time')!.getAttribute('datetime')).toBe('2026-08-20T10:00:00Z');
    expect(rows[1].querySelector('.suppression-reason')!.textContent).toBe('No reason given'); // blank reasons read honestly
    expect(host.textContent).toContain('Findings hidden from future scans, reports, and the CI gate.');
  });

  it('restores a suppression via DELETE and refreshes the list', async () => {
    const host = await render();
    expect(suppressionCalls()).toHaveLength(1);
    await click(host.querySelector<HTMLButtonElement>('[aria-label^="Restore"]')!);
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    const restore = fetchMock.mock.calls.find(([, init]) => (init as RequestInit | undefined)?.method === 'DELETE');
    expect(restore?.[0]).toBe(`/api/v1/workspaces/ws-1/suppressions/${ALPHA}`);
    expect(suppressionCalls().length).toBeGreaterThan(1); // the list refetches after the delete
  });

  it('shows an empty-state message when nothing is suppressed', async () => {
    await fetchMock.withImplementation(() => Promise.resolve(json({ items: [] })), async () => {
      const host = await render();
      expect(host.querySelector('.suppression-row')).toBeNull();
      expect(host.textContent).toContain('Nothing is suppressed.');
      expect(host.textContent).toContain('future scans');
    });
  });

  it('demotes load failures to a quiet inline retry that never blocks the page', async () => {
    let failed = false;
    await fetchMock.withImplementation(() => {
      failed = !failed;
      return failed ? Promise.resolve(json({ error: { code: 'DATABASE_ERROR', message: 'Could not load suppressions.' } }, 500)) : Promise.resolve(json({ items }));
    }, async () => {
      const host = await render();
      expect(host.textContent).toContain('Suppressions are unavailable right now.');
      expect(host.querySelector('[role="alert"]')).toBeNull(); // supplementary data never shouts
      await click(host.querySelector<HTMLButtonElement>('.text-button')!); // Try again
      await act(async () => { await Promise.resolve(); await Promise.resolve(); });
      expect(host.querySelectorAll('.suppression-row')).toHaveLength(2);
    });
  });
});

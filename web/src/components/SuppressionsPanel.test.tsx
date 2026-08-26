import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { SuppressionsSection, suppressionsCsv } from './SuppressionsPanel';
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

/** Drives the controlled search input with a native input event, the way typing lands in React (the prototype setter bypasses React's value tracking). */
async function type(host: HTMLElement, value: string) {
  const input = host.querySelector<HTMLInputElement>('.suppressions-section input');
  expect(input).not.toBeNull();
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')?.set;
  await act(async () => {
    setter?.call(input, value);
    input?.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

const settle = () => act(async () => { await new Promise((resolve) => setTimeout(resolve, 200)); });

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

  it('narrows rows only after the search settles and reports a live count', async () => {
    const host = await render();
    await type(host, 'FALSE POSITIVE'); // case-insensitive against the reason text
    expect(host.querySelectorAll('.suppression-row')).toHaveLength(2); // debounce still pending, nothing filtered yet
    await settle();
    const rows = [...host.querySelectorAll('.suppression-row')];
    expect(rows).toHaveLength(1);
    expect(rows[0].querySelector('.suppression-fingerprint')?.textContent).toContain('a');
    expect(host.querySelector('.suppressions-count')?.textContent).toBe('1 of 2 suppressions shown');
  });

  it('matches fingerprint fragments as well as reasons', async () => {
    const host = await render();
    await type(host, BETA.slice(0, 6));
    await settle();
    const rows = [...host.querySelectorAll('.suppression-row')];
    expect(rows).toHaveLength(1);
    expect(rows[0].querySelector('.suppression-fingerprint')?.getAttribute('title')).toBe(BETA);
  });

  it('restores every row as soon as the search is cleared with Escape', async () => {
    const host = await render();
    await type(host, 'zzz-nothing');
    await settle();
    expect(host.querySelectorAll('.suppression-row')).toHaveLength(0);
    const input = host.querySelector<HTMLInputElement>('.suppressions-section input');
    await act(async () => { input?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })); });
    // Clearing bypasses the debounce, so the full list is back without waiting.
    expect(host.querySelectorAll('.suppression-row')).toHaveLength(2);
    expect(host.querySelector('.suppressions-count')).toBeNull();
    expect(host.querySelector<HTMLInputElement>('.suppressions-section input')?.value).toBe('');
  });

  it('keeps a no-match state distinct from the nothing-suppressed empty state', async () => {
    const host = await render();
    await type(host, 'zzz-nothing');
    await settle();
    expect(host.textContent).toContain('No suppressions match “zzz-nothing”.');
    expect(host.textContent).not.toContain('Nothing is suppressed.');
  });
});

describe('SuppressionsSection table accessibility (Loop W6)', () => {
  it('names the suppressions table for screen readers', async () => {
    const host = await render();
    expect(host.querySelector('.suppressions-table caption')?.textContent).toBe('Dismissed findings for this workspace');
    // The list became a real table: header columns describe fingerprint/reason/date.
    const headers = [...host.querySelectorAll('.suppressions-table thead th')].map((th) => th.textContent);
    expect(headers).toEqual(['Fingerprint', 'Reason', 'Suppressed', 'Actions']);
  });
});

describe('SuppressionsSection CSV export (Loop W4)', () => {
  /** Swaps jsdom's URL blob methods for spies without relying on spy configurability. */
  function stubBlobUrls() {
    const created = vi.fn((_blob: Blob) => 'blob:test-url');
    const revoked = vi.fn();
    const originalCreate = URL.createObjectURL;
    const originalRevoke = URL.revokeObjectURL;
    URL.createObjectURL = created as unknown as typeof URL.createObjectURL;
    URL.revokeObjectURL = revoked as unknown as typeof URL.revokeObjectURL;
    return { created, revoked, restore: () => { URL.createObjectURL = originalCreate; URL.revokeObjectURL = originalRevoke; } };
  }

  it('exposes a download link built from a BOM-prefixed CSV of the loaded rows', async () => {
    // jsdom's Blob cannot be read back with .text(), so the file content is pinned
    // through the pure helper the blob is built from; the spy checks wiring.
    const nasty = { workspace_id: 'ws-1', fingerprint: 'c'.repeat(64), reason: 'Contains, comma and "quotes"', created_at: '2026-08-22T09:00:00Z' };
    const rows = [...items, nasty];
    expect(suppressionsCsv(rows)).toBe(`\ufefffingerprint,reason,created_at\n${ALPHA},False positive for this project,2026-08-20T10:00:00Z\n${BETA},,2026-08-21T11:30:00Z\n${'c'.repeat(64)},"Contains, comma and ""quotes""",2026-08-22T09:00:00Z`);
    const blobUrls = stubBlobUrls();
    try {
      await fetchMock.withImplementation((input: string) => input.endsWith('/workspaces/ws-1/suppressions')
        ? Promise.resolve(json({ items: rows }))
        : Promise.resolve(json({ items: [] })), async () => {
        const host = await render();
        await act(async () => { await Promise.resolve(); }); // let the export effect land after the data arrives
        const link = host.querySelector<HTMLAnchorElement>('a[download="suppressions.csv"]')!;
        expect(link.textContent).toBe('Export CSV');
        expect(link.getAttribute('href')).toBe('blob:test-url');
        expect(blobUrls.created).toHaveBeenCalledTimes(1);
        const blob = blobUrls.created.mock.calls[0]![0];
        expect(blob).toBeInstanceOf(Blob);
        expect(blob.type).toBe('text/csv;charset=utf-8');
      });
    } finally {
      blobUrls.restore();
    }
  });

  it('degrades to a disabled button when there is nothing to export', async () => {
    await fetchMock.withImplementation(() => Promise.resolve(json({ items: [] })), async () => {
      const host = await render();
      expect(host.querySelector('a[download]')).toBeNull();
      const fallback = [...host.querySelectorAll('button')].find((button) => button.textContent === 'Export CSV')! as HTMLButtonElement;
      expect(fallback.disabled).toBe(true);
    });
  });
});

import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Notice } from '../lib/notice';
import { FilesPage } from './FilesPage';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

const fetchMock = vi.fn((input: string) => {
  if (input.endsWith('/workspaces/ws-1')) return Promise.resolve(json({ id: 'ws-1', name: 'Example', root_path: 'C:\\code\\example' }));
  if (input.endsWith('/tree')) return Promise.resolve(json({ items: [
    { path: 'src', name: 'src', type: 'directory', included: true },
    { path: 'docs', name: 'docs', type: 'directory', included: true },
  ] }));
  if (input.endsWith('/rules')) return Promise.resolve(json({ items: [] }));
  if (input.endsWith('/path-overrides')) return Promise.resolve(json({ items: [] }));
  return Promise.resolve(json({ items: [] }));
});

async function render() {
  vi.stubGlobal('fetch', fetchMock);
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<FilesPage id="ws-1" notify={(_notice: Notice) => {}} />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return host;
}

function visiblePaths(host: HTMLElement) {
  return [...host.querySelectorAll<HTMLInputElement>('[aria-label^="Include "]')].map((input) => input.getAttribute('aria-label'));
}

async function type(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!;
  await act(async () => {
    setter.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

beforeEach(() => { vi.useFakeTimers(); });

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('FilesPage tree search', () => {
  it('debounces tree filtering so typing stays responsive and clearing applies immediately', async () => {
    const host = await render();
    const search = host.querySelector<HTMLInputElement>('[placeholder="src or package.json"]')!;
    expect(visiblePaths(host)).toEqual(['Include src', 'Include docs']);

    await type(search, 'src');
    expect(visiblePaths(host)).toEqual(['Include src', 'Include docs']); // debounce window keeps the full tree

    await act(async () => { vi.advanceTimersByTime(200); });
    expect(visiblePaths(host)).toEqual(['Include src']);

    await type(search, '');
    expect(visiblePaths(host)).toEqual(['Include src', 'Include docs']); // clearing flushes without the delay
  });
});

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

/** Queued responses for lazy child loads (`/tree?path=…`); each expansion shifts one, defaulting to an empty folder. */
const childResponses = new Map<string, Array<Response | Promise<Response>>>();
function enqueueChild(path: string, body: unknown, status = 200) {
  const queue = childResponses.get(path) ?? [];
  queue.push(json(body, status));
  childResponses.set(path, queue);
}

const fetchMock = vi.fn((input: string) => {
  if (input.endsWith('/workspaces/ws-1')) return Promise.resolve(json({ id: 'ws-1', name: 'Example', root_path: 'C:\\code\\example' }));
  const child = input.match(/\/tree\?path=([^&]+)$/);
  if (child) { const queued = childResponses.get(decodeURIComponent(child[1]!))?.shift(); return Promise.resolve(queued ?? json({ items: [] })); }
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

function toggle(host: HTMLElement, label: string) {
  return host.querySelector<HTMLButtonElement>(`[aria-label="${label}"]`);
}

async function type(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')!.set!;
  await act(async () => {
    setter.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

/** Flushes the fetch → json → setState microtask chain behind a lazy child load. */
async function flush() {
  for (let i = 0; i < 6; i += 1) await Promise.resolve();
}

beforeEach(() => { childResponses.clear(); vi.useFakeTimers(); });

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

  it('searches loaded children recursively, auto-expands ancestors, and restores collapse when cleared', async () => {
    enqueueChild('src', { items: [{ path: 'src/main.py', name: 'main.py', type: 'file', included: true }] });
    const host = await render();
    const search = host.querySelector<HTMLInputElement>('[placeholder="src or package.json"]')!;
    await act(async () => { toggle(host, 'Expand src')!.click(); await flush(); });
    await act(async () => { toggle(host, 'Collapse src')!.click(); }); // children stay cached while the folder sits collapsed
    expect(visiblePaths(host)).toEqual(['Include src', 'Include docs']);

    await type(search, 'main.py');
    await act(async () => { vi.advanceTimersByTime(200); });
    expect(visiblePaths(host)).toEqual(['Include src', 'Include src/main.py']); // nested match found, ancestor auto-expanded
    expect(toggle(host, 'Collapse src')).not.toBeNull();
    expect(host.textContent).toContain('1 matching path');
    expect(host.textContent).toContain('Searching loaded folders');

    await type(search, '');
    expect(visiblePaths(host)).toEqual(['Include src', 'Include docs']); // manual (collapsed) state restored
    expect(toggle(host, 'Expand src')).not.toBeNull();
    expect(toggle(host, 'Collapse src')).toBeNull();
  });

  it('highlights the matching substring inside node names', async () => {
    enqueueChild('src', { items: [{ path: 'src/main.py', name: 'main.py', type: 'file', included: true }] });
    const host = await render();
    const search = host.querySelector<HTMLInputElement>('[placeholder="src or package.json"]')!;
    await act(async () => { toggle(host, 'Expand src')!.click(); await flush(); });
    await type(search, 'ain');
    await act(async () => { vi.advanceTimersByTime(200); });
    const marks = [...host.querySelectorAll('.tree-name mark')];
    expect(marks).toHaveLength(1);
    expect(marks[0]!.textContent).toBe('ain');
    expect(marks[0]!.parentElement?.textContent).toBe('main.py');
  });

  it('marks failed child loads inline and retries them', async () => {
    enqueueChild('src', { error: { code: 'TREE_READ_FAILED', message: 'Folder could not be read.' } }, 500);
    const host = await render();
    await act(async () => { toggle(host, 'Expand src')!.click(); await flush(); });
    expect(host.textContent).toContain('Could not load');
    const retry = [...host.querySelectorAll('button')].find((button) => button.textContent === 'Retry');
    expect(retry).toBeDefined();

    enqueueChild('src', { items: [{ path: 'src/main.py', name: 'main.py', type: 'file', included: true }] });
    await act(async () => { retry!.click(); await flush(); });
    expect(host.textContent).not.toContain('Could not load');
    expect(visiblePaths(host)).toContain('Include src/main.py');
  });

  it('shows a spinner on the toggle while a folder load is pending', async () => {
    childResponses.set('src', [new Promise<Response>(() => {})]);
    const host = await render();
    await act(async () => { toggle(host, 'Expand src')!.click(); await flush(); });
    expect(host.querySelector('.tree-toggle .spinner')).not.toBeNull();
  });

  it('focuses the search box with "/" and clears it with Escape', async () => {
    const host = await render();
    const search = host.querySelector<HTMLInputElement>('[placeholder="src or package.json"]')!;
    expect(document.activeElement).not.toBe(search);

    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: '/', bubbles: true, cancelable: true })); });
    expect(document.activeElement).toBe(search);

    await type(search, 'src');
    await act(async () => { vi.advanceTimersByTime(200); });
    expect(visiblePaths(host)).toEqual(['Include src']);

    await act(async () => { search.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })); });
    expect(search.value).toBe('');
    expect(visiblePaths(host)).toEqual(['Include src', 'Include docs']); // cleared immediately, no debounce wait
    expect(document.activeElement).not.toBe(search); // blurred
  });
});

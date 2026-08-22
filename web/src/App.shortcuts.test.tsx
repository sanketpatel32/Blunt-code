import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from './App';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

async function renderApp(fetchMock: ReturnType<typeof vi.fn> = vi.fn((input: string) => Promise.resolve(json({ items: [] })))) {
  vi.stubGlobal('fetch', fetchMock);
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<App />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return host;
}

function key(key: string, shiftKey = false) {
  window.dispatchEvent(new KeyboardEvent('keydown', { key, shiftKey, bubbles: true, cancelable: true }));
}

describe('global keyboard shortcuts', () => {
  beforeEach(() => { window.history.replaceState({}, '', '/'); });
  afterEach(async () => { await act(async () => { root?.unmount(); }); document.body.replaceChildren(); vi.unstubAllGlobals(); });

  it('navigates with g-sequences and shows the armed hint only while waiting', async () => {
    const host = await renderApp();
    await act(async () => { key('g'); });
    expect(host.querySelector('.seq-hint')?.textContent).toBe('g…');
    await act(async () => { key('x'); }); // any other key ends the sequence
    expect(host.querySelector('.seq-hint')).toBeNull();
    expect(window.location.pathname).toBe('/');
    await act(async () => { key('g'); key('t'); });
    expect(window.location.pathname).toBe('/tools');
    await act(async () => { key('g'); key('w'); });
    expect(window.location.pathname).toBe('/workspaces');
    await act(async () => { key('h'); }); // sequence targets only act when armed
    expect(window.location.pathname).toBe('/workspaces');
  });

  it('opens the add-workspace dialog with n and suspends shortcuts while it is open', async () => {
    const host = await renderApp();
    await act(async () => { key('g'); key('t'); });
    expect(window.location.pathname).toBe('/tools');
    await act(async () => { key('n'); });
    expect(host.textContent).toContain('Choose a project folder.');
    await act(async () => { key('g'); key('h'); });
    expect(window.location.pathname).toBe('/tools'); // dialog owns the keyboard
    await act(async () => { key('Escape'); });
    expect(host.textContent).not.toContain('Choose a project folder.');
    await act(async () => { key('g'); key('h'); });
    expect(window.location.pathname).toBe('/');
    expect(host.textContent).toContain('No workspaces yet');
  });

  it('opens the shortcuts help with ? and closes it with Escape', async () => {
    const host = await renderApp();
    await act(async () => { key('?', true); });
    const dialog = host.querySelector('dialog[data-shortcuts-dialog]');
    expect(dialog).not.toBeNull();
    expect(dialog?.getAttribute('aria-labelledby')).toBe('shortcuts-title');
    expect(host.textContent).toContain('Go to Home');
    expect(host.querySelector('.seq-hint')).toBeNull();
    await act(async () => { key('g'); }); // help dialog does not suspend shortcuts
    expect(host.querySelector('.seq-hint')?.textContent).toBe('g…');
    await act(async () => { key('Escape'); });
    expect(host.querySelector('dialog[data-shortcuts-dialog]')).toBeNull();
    expect(host.querySelector('.seq-hint')).toBeNull(); // Escape also ended the armed sequence
  });

  it('ignores shortcut keys typed into a field', async () => {
    const host = await renderApp();
    const field = document.createElement('input');
    host.append(field);
    const type = (key: string) => field.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
    await act(async () => { field.focus(); type('n'); });
    expect(host.textContent).not.toContain('Choose a project folder.');
    await act(async () => { type('g'); type('h'); });
    expect(window.location.pathname).toBe('/');
    field.remove();
  });

  it('focuses the tree search with / on the files page', async () => {
    window.history.replaceState({}, '', '/workspaces/ws-1/files');
    const fetchMock = vi.fn((input: string) => {
      if (input.endsWith('/workspaces/ws-1')) return Promise.resolve(json({ id: 'ws-1', name: 'Example', root_path: 'C:\\code\\example' }));
      if (input.endsWith('/tree')) return Promise.resolve(json({ items: [{ path: 'src', name: 'src', type: 'directory', included: true, has_children: true }] }));
      return Promise.resolve(json({ items: [] }));
    });
    const host = await renderApp(fetchMock);
    const search = host.querySelector<HTMLInputElement>('[placeholder="src or package.json"]')!;
    expect(search).not.toBeNull();
    expect(document.activeElement).not.toBe(search);
    await act(async () => { key('/'); });
    expect(document.activeElement).toBe(search);
  });
});

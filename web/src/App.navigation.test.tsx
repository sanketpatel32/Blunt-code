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

async function renderApp(fetchMock?: ReturnType<typeof vi.fn>) {
  vi.stubGlobal('fetch', fetchMock ?? vi.fn(() => Promise.resolve(json({ items: [] }))));
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<App />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return host;
}

describe('navigation resilience', () => {
  beforeEach(() => { window.history.replaceState({}, '', '/'); });
  afterEach(async () => { await act(async () => { root?.unmount(); }); document.body.replaceChildren(); vi.unstubAllGlobals(); });

  it('renders the 404 page for an unknown path instead of Home', async () => {
    window.history.replaceState({}, '', '/definitely/not/a/page');
    const host = await renderApp();
    expect(host.textContent).toContain('Page not found');
    expect(host.textContent).toContain('Nothing here');
    expect(host.textContent).not.toContain('Recent projects');
    const home = [...host.querySelectorAll('button')].find((button) => button.textContent === 'Go to Home');
    expect(home).toBeDefined();
    await act(async () => { home!.click(); await Promise.resolve(); await Promise.resolve(); });
    expect(window.location.pathname).toBe('/');
    expect(host.textContent).toContain('Point Blunt Code at a project');
  });

  it('adds About as the last navigation item and routes to the About page', async () => {
    const host = await renderApp();
    const labels = [...host.querySelectorAll('.app-nav nav a')].map((link) => link.textContent);
    expect(labels).toEqual(['Home', 'Workspaces', 'Search', 'Tools', 'Settings', 'About']);
    const about = host.querySelector<HTMLAnchorElement>('.app-nav nav a[href="/about"]');
    expect(about?.textContent).toBe('About');
    await act(async () => { about!.click(); await Promise.resolve(); await Promise.resolve(); });
    expect(window.location.pathname).toBe('/about');
    expect(host.textContent).toContain('Local by default');
  });

  it('exposes a skip link to main content as the first link in the app', async () => {
    const host = await renderApp();
    const skip = host.querySelector<HTMLAnchorElement>('a.skip-link');
    expect(skip?.getAttribute('href')).toBe('#main-content');
    expect(skip?.textContent).toBe('Skip to main content');
    expect(host.querySelector('a')).toBe(skip);
    expect(host.querySelector('main#main-content')).not.toBeNull();
  });

  it('appends lazily loaded workspace entries (labels starting "Go to ") behind the static commands when the palette opens', async () => {
    const fetchMock = vi.fn((input: string) => {
      if (input.endsWith('/api/v1/workspaces')) {
        return Promise.resolve(json({ workspaces: [
          { id: 'ws-alpha', name: 'Alpha', root_path: 'C:\\code\\alpha' },
          { id: 'ws-beta', name: 'Beta', root_path: 'C:\\code\\beta' },
        ] }));
      }
      return Promise.resolve(json({ items: [] }));
    });
    const host = await renderApp(fetchMock);
    expect(host.querySelector('.command-palette')).toBeNull(); // closed until Ctrl+K

    await act(async () => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, bubbles: true, cancelable: true })); });
    await act(async () => { await Promise.resolve(); await Promise.resolve(); }); // let the lazy workspace load settle
    const options = [...host.querySelectorAll('[role="option"] .palette-label')].map((option) => option.textContent);
    // Static commands stay first; the two workspace entries follow them.
    expect(options[0]).toBe('Go to Home');
    for (const label of ['Go to Alpha', 'Go to Beta']) {
      expect(options).toContain(label);
      expect(options.indexOf(label)).toBeGreaterThan(options.indexOf('Go to Home'));
    }
    // Count hint appears once the lazy load completes.
    expect(host.querySelector('.palette-note')?.textContent).toBe('2 workspaces indexed');
  });
});

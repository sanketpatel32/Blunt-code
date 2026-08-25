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

async function renderApp() {
  vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(json({ items: [] }))));
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
    expect(labels).toEqual(['Home', 'Workspaces', 'Tools', 'Settings', 'About']);
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
});

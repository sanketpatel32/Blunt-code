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

  it('shows every top-level page in the rail, with no page hidden behind a menu', async () => {
    const host = await renderApp();
    // The old shell showed four links and hid Rules, CLI docs, Settings and
    // About behind a "More" menu, so half the app was invisible unless you
    // happened to know it existed. A vertical rail has room for all of them.
    const labels = [...host.querySelectorAll('.rail-nav .rail-link-label')].map((link) => link.textContent);
    expect(labels).toEqual(expect.arrayContaining(['Home', 'Workspaces', 'Search', 'Tools', 'Rules', 'CLI Docs']));
    // Settings/About sit in the rail's foot group, still one click away.
    const foot = [...host.querySelectorAll('.rail-foot .rail-link-label')].map((link) => link.textContent);
    expect(foot).toEqual(expect.arrayContaining(['Settings', 'About']));
    // The desktop rail hides nothing, so it has no More menu. The mobile bar
    // still carries one — below 64rem the rail is gone and the bar has to fit
    // Settings and About into a scrollable strip.
    expect(host.querySelector('.app-rail .nav-more-toggle')).toBeNull();

    // And it routes.
    const about = [...host.querySelectorAll<HTMLAnchorElement>('.rail-link')].find((a) => a.textContent === 'About')!;
    await act(async () => { about.click(); await Promise.resolve(); await Promise.resolve(); });
    expect(window.location.pathname).toBe('/about');
    expect(document.title).toBe('About · Blunt Code'); // per-page tab title, no ids
    expect(host.textContent).toContain('Local by default');
  });

  it('lists the open workspace’s own pages in the rail, so there is one nav on screen', async () => {
    const host = await renderApp();
    // No workspace open: the rail has no workspace section.
    expect(host.querySelector('.rail-workspace')).toBeNull();
    // And the second nav bar is gone from the page body entirely.
    expect(host.querySelector('.workspace-context')).toBeNull();
  });

  it.each(['.brand', '.rail-nav .rail-link[href="/workspaces"]'])('preserves modified clicks on %s', async (selector) => {
    window.history.replaceState({}, '', '/about');
    const host = await renderApp();
    const link = host.querySelector<HTMLAnchorElement>(selector)!;
    for (const modifier of ['ctrlKey', 'metaKey', 'shiftKey', 'altKey']) {
      let intercepted = true;
      // Observe after React's delegated handler, then suppress jsdom's native
      // navigation (it cannot open a new browser tab).
      const observe = (event: MouseEvent) => { intercepted = event.defaultPrevented; event.preventDefault(); };
      document.addEventListener('click', observe, { once: true });
      await act(async () => { link.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, [modifier]: true })); });
      expect(intercepted).toBe(false);
      expect(window.location.pathname).toBe('/about');
    }
    await act(async () => { link.click(); });
    expect(window.location.pathname).toBe(link.getAttribute('href'));
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

describe('browser history and hostile URLs', () => {
  beforeEach(() => { window.history.replaceState({}, '', '/'); });
  afterEach(async () => { await act(async () => { root?.unmount(); }); document.body.replaceChildren(); vi.unstubAllGlobals(); });

  /** Models the browser's Back/Forward buttons: the URL is already the target
   *  entry and popstate tells the app to re-parse it. */
  async function popTo(path: string, host: HTMLElement) {
    window.history.replaceState({}, '', path);
    await act(async () => { window.dispatchEvent(new PopStateEvent('popstate')); });
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    return host;
  }

  it('follows back and forward navigation (popstate) without remounting the app', async () => {
    const host = await renderApp();
    const aboutItem = host.querySelector<HTMLAnchorElement>('.app-rail a[href="/about"]')!;
    expect(aboutItem).toBeDefined();
    await act(async () => { aboutItem.click(); await Promise.resolve(); await Promise.resolve(); });
    expect(window.location.pathname).toBe('/about');
    expect(host.textContent).toContain('Local by default');

    await popTo('/', host); // Back: home again
    expect(window.location.pathname).toBe('/');
    expect(host.textContent).toContain('Point Blunt Code at a project');
    expect(host.textContent).not.toContain('Local by default');

    await popTo('/about', host); // Forward: About again
    expect(window.location.pathname).toBe('/about');
    expect(host.textContent).toContain('Local by default');
  });

  it('shows the error panel for an unknown scan id and never executes the id as markup', async () => {
    window.history.replaceState({}, '', '/scans/zzz%3Cscript%3Ealert(1)%3C%2Fscript%3E');
    vi.stubGlobal('EventSource', class { addEventListener() {} close() {} }); // jsdom has no SSE; the page must still render its error state
    const fetchMock = vi.fn(() => Promise.resolve(json({}, 404)));
    const host = await renderApp(fetchMock);
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    expect(host.querySelector('[role="alert"].error-panel')).not.toBeNull();
    expect(host.textContent).toContain('Could not load this view');
    expect(host.querySelectorAll('script, img').length).toBe(0); // the hostile id stayed inert data
  });

  it('renders hostile URL parameters as inert input values, never markup', async () => {
    window.history.replaceState({}, '', '/search?q=%3Cimg%20src%3Dx%20onerror%3Dalert(1)%3E');
    const host = await renderApp();
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    expect(host.querySelectorAll('img[onerror], script').length).toBe(0);
    const box = host.querySelector<HTMLInputElement>('[role="search"] input[aria-label="Search findings"]')!;
    expect(box.value).toBe('<img src=x onerror=alert(1)>');
    expect(host.querySelector('#search-results')?.textContent ?? '').not.toContain('<img');
  });
});

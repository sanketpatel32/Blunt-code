import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AboutPage } from './AboutPage';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function json(body: unknown) {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
}

function aboutMock() {
  return vi.fn((input: string) => {
    if (input.endsWith('/meta')) return Promise.resolve(json({ version: '1.2.3', api_version: 'v1', os: 'windows', architecture: 'amd64' }));
    if (input.endsWith('/health')) return Promise.resolve(json({ status: 'ready' }));
    return Promise.resolve(json({}));
  });
}

async function renderPage(fetchMock: ReturnType<typeof vi.fn>, onUpdateHandoff?: (version: string) => void) {
  vi.stubGlobal('fetch', fetchMock);
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<AboutPage onUpdateHandoff={onUpdateHandoff} />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return host;
}

/** jsdom has no clipboard; inject a stub and return it for call assertions. */
function stubClipboard() {
  const writeText = vi.fn((_text: string) => Promise.resolve(true));
  Object.defineProperty(window.navigator, 'clipboard', { value: { writeText }, configurable: true });
  return writeText;
}

function copyButton(host: HTMLElement) {
  return [...host.querySelectorAll('button')].find((button) => button.textContent === 'Copy version info' || button.textContent === 'Copied')!;
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
  // Remove the clipboard injection so later tests exercise the fallback path.
  Reflect.deleteProperty(window.navigator, 'clipboard');
});

describe('AboutPage at-a-glance card', () => {
  it('renders the version line, privacy bullets, and server details', async () => {
    const host = await renderPage(aboutMock());
    expect(host.querySelector('.about-card .badge')!.textContent).toBe('v1.2.3');
    expect(host.textContent).toContain('Local-only analysis');
    expect(host.textContent).toContain('No account required');
    expect(host.textContent).toContain('No telemetry');
    expect([...host.querySelectorAll('.about-points li svg')].length).toBe(3);
    expect(host.querySelector('.about-card dl')!.textContent).toContain('amd64');
    expect(host.textContent).toContain('Copy version info');
  });

  it('copies version info to the clipboard and confirms inline before reverting', async () => {
    const writeText = stubClipboard();
    const host = await renderPage(aboutMock());
    await act(async () => { copyButton(host).click(); await Promise.resolve(); await Promise.resolve(); });
    expect(writeText).toHaveBeenCalledTimes(1);
    expect(writeText.mock.calls[0][0]).toContain('Version: 1.2.3');
    expect(writeText.mock.calls[0][0]).toContain('Platform: windows / amd64');
    expect(copyButton(host).textContent).toBe('Copied');
    await act(async () => { await new Promise((resolve) => setTimeout(resolve, 1600)); });
    expect(copyButton(host).textContent).toBe('Copy version info');
  });

  it('skips the confirmation when the clipboard copy fails', async () => {
    Object.defineProperty(window.navigator, 'clipboard', { value: { writeText: vi.fn(() => Promise.reject(new Error('denied'))) }, configurable: true });
    const host = await renderPage(aboutMock());
    await act(async () => { copyButton(host).click(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
    // The rejected async path falls back to execCommand, which jsdom lacks, so no confirmation shows.
    expect(copyButton(host).textContent).toBe('Copy version info');
  });
});

describe('AboutPage updates card', () => {
  function updateFetch(options: { available: boolean; fail?: boolean }) {
    return vi.fn((input: string, init?: RequestInit) => {
      if (input.endsWith('/meta')) return Promise.resolve(json({ version: '1.2.3', api_version: 'v1', os: 'windows', architecture: 'amd64' }));
      if (input.endsWith('/health')) return Promise.resolve(json({ status: 'ready' }));
      if (input.endsWith('/update/check')) {
        if (options.fail) return Promise.resolve(new Response(JSON.stringify({ error: { code: 'UPDATE_OFFLINE', message: 'Offline mode is enabled.' } }), { status: 409, headers: { 'Content-Type': 'application/json' } }));
        return Promise.resolve(json({ current: '1.2.3', latest: '2.0.0', available: options.available, release_url: 'https://example.com/release', release_notes: 'Faster scans.' }));
      }
      if (input.endsWith('/update/apply')) return Promise.resolve(json({ started: true, staged_at: 'C:\\temp\\x' }));
      if (input.endsWith('/system/stop')) return Promise.resolve(json({ state: 'stopping' }));
      return Promise.resolve(json({}));
    });
  }

  async function clickButton(host: HTMLElement, label: string) {
    const button = [...host.querySelectorAll('button')].find((candidate) => candidate.textContent === label);
    expect(button).toBeDefined();
    await act(async () => { button!.click(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
  }

  it('reports up to date after a check', async () => {
    const fetchMock = updateFetch({ available: false });
    const host = await renderPage(fetchMock);
    await clickButton(host, 'Check for updates');
    expect(host.textContent).toContain('You are on the latest release (2.0.0)');
    expect(fetchMock.mock.calls.some(([input]) => String(input).endsWith('/update/check'))).toBe(true);
  });

  it('offers the update and hands off to the installer before stopping', async () => {
    const fetchMock = updateFetch({ available: true });
    const host = await renderPage(fetchMock);
    await clickButton(host, 'Check for updates');
    expect(host.textContent).toContain('v2.0.0 is available');
    expect([...host.querySelectorAll('a')].some((link) => link.textContent === 'Release notes')).toBe(true);
    await clickButton(host, 'Update now');
    const calls = fetchMock.mock.calls.map(([input, init]) => ({ url: String(input), method: (init as RequestInit | undefined)?.method }));
    expect(calls.filter((call) => call.url.endsWith('/update/apply')).length).toBe(1);
    // Without a handoff (standalone render), the card still stops the server:
    // the installer waits for our exit.
    expect(calls.some((call) => call.url.endsWith('/system/stop') && call.method === 'POST')).toBe(true);
    expect(host.textContent).toContain('Installer launched');
  });

  it('delegates the shutdown to the app shell when the handoff is provided', async () => {
    const fetchMock = updateFetch({ available: true });
    const handoff = vi.fn();
    const host = await renderPage(fetchMock, handoff);
    await clickButton(host, 'Check for updates');
    await clickButton(host, 'Update now');
    // The shell owns the stop + the update screen from here; the card must not
    // race it with a second stop call.
    expect(handoff).toHaveBeenCalledWith('2.0.0');
    expect(fetchMock.mock.calls.some(([input]) => String(input).endsWith('/system/stop'))).toBe(false);
  });

  it('surfaces offline and network failures with a retry', async () => {
    const host = await renderPage(updateFetch({ available: false, fail: true }));
    await clickButton(host, 'Check for updates');
    expect(host.textContent).toContain('Offline mode is enabled.');
    await clickButton(host, 'Try again');
    expect(host.textContent).toContain('Offline mode is enabled.');
  });
});

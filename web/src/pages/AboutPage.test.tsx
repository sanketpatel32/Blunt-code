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

async function renderPage(fetchMock: ReturnType<typeof vi.fn>) {
  vi.stubGlobal('fetch', fetchMock);
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<AboutPage />); });
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

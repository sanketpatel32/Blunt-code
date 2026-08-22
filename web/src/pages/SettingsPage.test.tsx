import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Notice } from '../lib/notice';
import { SettingsPage } from './SettingsPage';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

/** Settings fetch stub with a data directory and a switchable open-folder responder. */
function settingsMock(respond: () => Response | Promise<Response> = () => new Response(null, { status: 204 })) {
  return vi.fn((input: string) => {
    if (input.endsWith('/meta')) return Promise.resolve(json({ data_directory: 'C:\\Users\\you\\BluntCode' }));
    if (input.endsWith('/settings')) return Promise.resolve(json({ offline: false, open_browser: true }));
    if (input.endsWith('/system/open-folder')) return Promise.resolve(respond());
    return Promise.resolve(json({}));
  });
}

async function renderPage(fetchMock: ReturnType<typeof vi.fn>) {
  vi.stubGlobal('fetch', fetchMock);
  const host = document.createElement('div');
  document.body.append(host);
  const notify = vi.fn<(notice: Notice) => void>();
  root = createRoot(host);
  await act(async () => { root.render(<SettingsPage notify={notify} />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return { host, notify };
}

function button(host: HTMLElement, label: string) {
  return [...host.querySelectorAll('button')].find((candidate) => candidate.textContent === label);
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

describe('SettingsPage data folders', () => {
  it('renders every folder button beside the stored data directory', async () => {
    const { host } = await renderPage(settingsMock());
    expect(host.textContent).toContain('Data folders');
    expect(host.textContent).toContain('Open the folders where Blunt Code keeps local data.');
    expect(host.textContent).toContain('Stored in: C:\\Users\\you\\BluntCode');
    for (const label of ['Reports', 'Logs', 'Tools', 'App data']) expect(button(host, label)).toBeDefined();
  });

  it('opens a folder through the endpoint and reports success', async () => {
    const fetchMock = settingsMock();
    const { host, notify } = await renderPage(fetchMock);
    await act(async () => { button(host, 'Reports')!.click(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/system/open-folder', expect.objectContaining({ method: 'POST', body: JSON.stringify({ kind: 'reports' }) }));
    expect(notify).toHaveBeenCalledWith({ kind: 'success', text: 'Opened the reports folder.' });
    expect(button(host, 'Reports')!.textContent).toBe('Reports');
  });

  it('explains a missing folder as information, not an error', async () => {
    const fetchMock = settingsMock(() => json({ error: { code: 'FOLDER_NOT_FOUND', message: 'Reports folder does not exist yet.' } }, 409));
    const { host, notify } = await renderPage(fetchMock);
    await act(async () => { button(host, 'Reports')!.click(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
    expect(notify).toHaveBeenCalledWith({ kind: 'info', text: 'The reports folder is created after your first scan.' });
    expect(notify).not.toHaveBeenCalledWith(expect.objectContaining({ kind: 'error' }));
  });

  it('surfaces other open-folder failures as error notices', async () => {
    const fetchMock = settingsMock(() => json({ error: { code: 'FOLDER_OPEN_FAILED', message: 'Explorer could not be started.' } }, 500));
    const { host, notify } = await renderPage(fetchMock);
    await act(async () => { button(host, 'Logs')!.click(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
    expect(notify).toHaveBeenCalledWith({ kind: 'error', text: 'FOLDER_OPEN_FAILED: Explorer could not be started.' });
  });

  it('marks only the opening button busy while the request is pending', async () => {
    let release!: (response: Response) => void;
    const pending = new Promise<Response>((resolve) => { release = resolve; });
    const { host, notify } = await renderPage(settingsMock(() => pending));
    const reports = button(host, 'Reports')!;
    await act(async () => { reports.click(); });
    expect(reports.textContent).toContain('Opening…');
    expect(reports.disabled).toBe(true);
    for (const label of ['Logs', 'Tools', 'App data']) expect(button(host, label)!.disabled).toBe(false);
    await act(async () => { release(new Response(null, { status: 204 })); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
    expect(reports.textContent).toBe('Reports');
    expect(reports.disabled).toBe(false);
    expect(notify).toHaveBeenCalledWith({ kind: 'success', text: 'Opened the reports folder.' });
  });
});

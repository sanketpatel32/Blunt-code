import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Route } from '../lib/router';
import { NotFoundPage } from './NotFoundPage';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

async function renderPage(path: string) {
  window.history.replaceState({}, '', path);
  const host = document.createElement('div');
  document.body.append(host);
  const go = vi.fn<(route: Route) => void>();
  root = createRoot(host);
  await act(async () => { root.render(<NotFoundPage go={go} />); });
  await act(async () => { await Promise.resolve(); });
  return { host, go };
}

function button(host: HTMLElement, label: string) {
  return [...host.querySelectorAll('button')].find((candidate) => candidate.textContent === label)!;
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  window.history.replaceState({}, '', '/');
});

describe('NotFoundPage', () => {
  it('shows the generic 404 content for an unshaped unknown path', async () => {
    const { host } = await renderPage('/definitely/not/a/page');
    expect(host.textContent).toContain('Page not found');
    expect(host.textContent).toContain('Nothing here');
    expect(host.textContent).toContain('The link may be mistyped or out of date.');
    expect(host.textContent).not.toContain('may have been removed');
    expect(button(host, 'Go to Home')).toBeDefined();
    expect(button(host, 'Open Workspaces')).toBeDefined();
  });

  it('offers the removed-content suggestion for a workspace-shaped address', async () => {
    const { host } = await renderPage('/workspaces/ws-42/some-typo-section');
    expect(host.textContent).toContain('This workspace or scan may have been removed');
  });

  it('offers the removed-content suggestion for a bare scan-shaped address', async () => {
    const { host } = await renderPage('/scan/abc123');
    expect(host.textContent).toContain('This workspace or scan may have been removed');
  });

  it('navigates through the go prop from both action buttons', async () => {
    const { host, go } = await renderPage('/');
    await act(async () => { button(host, 'Go to Home').click(); });
    expect(go).toHaveBeenCalledWith({ page: 'home' });
    await act(async () => { button(host, 'Open Workspaces').click(); });
    expect(go).toHaveBeenCalledWith({ page: 'workspaces' });
    expect(go).toHaveBeenCalledTimes(2);
  });
});

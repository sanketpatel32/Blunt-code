import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Notice } from '../lib/notice';
import { ToolsPage } from './ToolsPage';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

const toolsBody = { items: [{ id: 'ruff', name: 'Ruff', version: '0.6.9', ready: true, can_install: true }, { id: 'semgrep', ready: false, can_install: true }] };

/** Fetch stub serving the default two-tool list plus a switchable action responder. */
function toolsMock(action: (input: string) => Response | Promise<Response> = () => json({})) {
  return vi.fn((input: string) => {
    if (input.endsWith('/tools')) return Promise.resolve(json(toolsBody));
    return Promise.resolve(action(input));
  });
}

async function renderPage(fetchMock: ReturnType<typeof vi.fn>) {
  vi.stubGlobal('fetch', fetchMock);
  const host = document.createElement('div');
  document.body.append(host);
  const notify = vi.fn<(notice: Notice) => void>();
  root = createRoot(host);
  await act(async () => { root.render(<ToolsPage notify={notify} />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return { host, notify };
}

function row(host: HTMLElement, id: string) {
  return [...host.querySelectorAll('.tool-table tbody tr')].find((candidate) => candidate.textContent?.includes(id))!;
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

describe('ToolsPage readiness strip', () => {
  it('summarizes ready counts and chips every non-ready tool with its version badges on rows', async () => {
    const { host } = await renderPage(toolsMock());
    const strip = host.querySelector('.tools-readiness')!;
    expect(strip.textContent).toContain('1 of 2 ready');
    expect(host.querySelector('.tools-all-ready')).toBeNull();
    const chip = [...strip.querySelectorAll('.badge')].find((badge) => badge.textContent !== '1 of 2 ready')!;
    expect(chip.querySelector('.dot.not-ready')).not.toBeNull();
    expect(chip.querySelector('.spinner')).toBeNull();
    expect(chip.textContent).toContain('semgrep not installed');
    expect(row(host, 'Ruff').querySelector('.tool-version')!.textContent).toBe('v0.6.9');
    expect(row(host, 'semgrep').querySelector('.tool-version')!.textContent).toBe('Managed version');
  });

  it('marks an all-ready tool set with the success counter and no pending chips', async () => {
    const fetchMock = vi.fn((input: string) => (input.endsWith('/tools') ? Promise.resolve(json({ items: [{ id: 'ruff', name: 'Ruff', version: '0.6.9', ready: true }] })) : Promise.resolve(json({}))));
    const { host } = await renderPage(fetchMock);
    expect(host.querySelector('.tools-all-ready')!.textContent).toBe('1 of 1 ready');
    expect(host.querySelectorAll('.tools-readiness .dot').length).toBe(0);
  });

  it('turns the non-ready chip into a spinner while that install is in flight', async () => {
    const pending = new Promise<Response>(() => {});
    const { host } = await renderPage(toolsMock(() => pending));
    await act(async () => { [...row(host, 'semgrep').querySelectorAll('button')].find((button) => button.textContent === 'Install')!.click(); });
    const chip = [...host.querySelectorAll('.tools-readiness .badge')].find((badge) => badge.textContent!.includes('semgrep'))!;
    expect(chip.querySelector('.spinner')).not.toBeNull();
    expect(chip.textContent).toContain('semgrep installing…');
    expect(row(host, 'semgrep').querySelector('.table-actions')!.getAttribute('aria-busy')).toBe('true');
    expect(host.querySelector('.tools-readiness')!.textContent).toContain('1 of 2 ready');
  });
});

describe('ToolsPage built-in analyzers', () => {
  it('lists secrets and todo as built-in with no install actions, not as coming soon', async () => {
    const { host } = await renderPage(toolsMock());
    const section = host.querySelector('section[aria-label="Built-in analyzers"]')!;
    expect(section.querySelector('h3')!.textContent).toBe('Built-in analyzers');
    const rows = [...section.querySelectorAll('tbody tr')];
    expect(rows.map((r) => r.textContent)).toEqual(expect.arrayContaining([expect.stringContaining('Secrets'), expect.stringContaining('Todo Scanner')]));
    for (const row of rows) {
      expect(row.querySelector('.state.ready')!.textContent).toBe('Built-in');
      expect(row.textContent).not.toContain('Coming soon');
    }
    const actions = [...section.querySelectorAll('button')];
    expect(actions.filter((button) => ['Install', 'Repair', 'Update'].includes(button.textContent!))).toHaveLength(0);
  });

  it('keeps the coming-soon section free of the built-in analyzers', async () => {
    const { host } = await renderPage(toolsMock());
    const comingSoon = host.querySelector('section[aria-label="Coming soon analyzers"]');
    if (comingSoon) {
      // Category headers legitimately repeat words like "Secrets"; only the
      // tool-name cells must not contain the built-ins.
      const names = [...comingSoon.querySelectorAll('tbody tr td:first-child strong')].map((cell) => cell.childNodes[0].textContent);
      expect(names).not.toContain('Secrets');
      expect(names).not.toContain('Todo Scanner');
    }
    expect(host.textContent).not.toContain('Secrets not installed');
  });
});

describe('ToolsPage actions', () => {
  it('keeps install firing the same POST endpoint and success notice as before', async () => {
    const fetchMock = toolsMock((input) => (input.endsWith('/tools/semgrep/install') ? json({ id: 'semgrep', ready: true, can_install: false }) : json({})));
    const { host, notify } = await renderPage(fetchMock);
    await act(async () => { [...row(host, 'semgrep').querySelectorAll('button')].find((button) => button.textContent === 'Install')!.click(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/tools/semgrep/install', expect.objectContaining({ method: 'POST' }));
    expect(notify).toHaveBeenCalledWith({ kind: 'info', text: 'semgrep: installed.' });
    expect(notify).not.toHaveBeenCalledWith(expect.objectContaining({ kind: 'error' }));
  });

  it('fires repair and update against their own endpoints', async () => {
    const fetchMock = toolsMock((input) => (input.endsWith('/tools/ruff/update') || input.endsWith('/tools/ruff/repair') ? json({ id: 'ruff', ready: true }) : json({})));
    const { host } = await renderPage(fetchMock);
    await act(async () => { [...row(host, 'Ruff').querySelectorAll('button')].find((button) => button.textContent === 'Update')!.click(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/tools/ruff/update', expect.objectContaining({ method: 'POST' }));
    await act(async () => { [...row(host, 'Ruff').querySelectorAll('button')].find((button) => button.textContent === 'Repair')!.click(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/tools/ruff/repair', expect.objectContaining({ method: 'POST' }));
  });

  it('retries the tools load from the error panel', async () => {
    let failing = true;
    const fetchMock = vi.fn((input: string) => {
      if (input.endsWith('/tools')) return failing ? Promise.resolve(json({ error: { code: 'TOOLS_OFFLINE', message: 'Tool service unavailable.' } }, 503)) : Promise.resolve(json(toolsBody));
      return Promise.resolve(json({}));
    });
    const { host } = await renderPage(fetchMock);
    expect(host.querySelector('.error-panel')).not.toBeNull();
    expect(host.querySelector('.tools-readiness')).toBeNull();
    failing = false;
    await act(async () => { [...host.querySelectorAll('button')].find((button) => button.textContent === 'Try again')!.click(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
    expect(host.querySelector('.error-panel')).toBeNull();
    expect(host.querySelector('.tools-readiness')!.textContent).toContain('1 of 2 ready');
  });
});

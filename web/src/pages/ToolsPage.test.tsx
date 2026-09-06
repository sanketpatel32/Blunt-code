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

/** A representative slice of the backend capability inventory: two managed
 *  tools (one ready, one not), two in-process built-ins, and one in-process
 *  analyzer withheld by offline mode. */
const analyzersBody = {
  items: [
    { id: 'ruff', display_name: 'Ruff', category: 'code-quality', execution: 'external', profiles: ['quick', 'standard', 'deep', 'pentest'], input_kinds: ['source'], managed_tool: 'ruff', network: 'none', keep_artifact_findings: false, timeout_class: 'fast', description: 'Fast Python linter.', languages: ['python'], version: '0.6.9', ready: true, registered: true },
    { id: 'semgrep', display_name: 'Semgrep', category: 'security', execution: 'external', profiles: ['standard', 'deep', 'pentest'], input_kinds: ['source'], managed_tool: 'semgrep', network: 'none', keep_artifact_findings: false, timeout_class: 'fast', description: 'Pattern-based security scanner.', languages: ['python', 'javascript', 'typescript'], ready: false, detail: 'Executable is missing.', registered: true },
    { id: 'secrets', display_name: 'Secrets', category: 'secrets', execution: 'in-process', profiles: ['standard', 'deep', 'pentest'], input_kinds: ['source'], managed_tool: '', network: 'none', keep_artifact_findings: true, timeout_class: 'fast', description: 'Built-in credential detection.', languages: ['python'], ready: true, registered: true },
    { id: 'todo', display_name: 'Todo Scanner', category: 'maintainability', execution: 'in-process', profiles: ['standard', 'deep', 'pentest'], input_kinds: ['source'], managed_tool: '', network: 'none', keep_artifact_findings: false, timeout_class: 'fast', description: 'Tracks TODO/FIXME markers.', languages: ['python'], ready: true, registered: true },
    { id: 'license-scan', display_name: 'License Scanner', category: 'compliance', execution: 'in-process', profiles: ['standard', 'deep', 'pentest'], input_kinds: ['license-files', 'dependencies'], managed_tool: '', network: 'none', keep_artifact_findings: false, timeout_class: 'fast', description: 'License detection.', languages: ['text'], ready: false, registered: false },
  ],
};

/** Fetch stub serving the capability inventory plus a switchable action responder. */
function analyzersMock(action: (input: string) => Response | Promise<Response> = () => json({})) {
  return vi.fn((input: string) => {
    if (input.endsWith('/analyzers')) return Promise.resolve(json(analyzersBody));
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
  it('counts managed tools only and chips every non-ready managed tool', async () => {
    const { host } = await renderPage(analyzersMock());
    const strip = host.querySelector('.tools-readiness')!;
    expect(strip.textContent).toContain('1 of 2 ready');
    expect(host.querySelector('.tools-all-ready')).toBeNull();
    const chip = [...strip.querySelectorAll('.badge')].find((badge) => badge.textContent !== '1 of 2 ready')!;
    expect(chip.querySelector('.dot.not-ready')).not.toBeNull();
    expect(chip.querySelector('.spinner')).toBeNull();
    expect(chip.textContent).toContain('Semgrep not installed');
    expect(row(host, 'Ruff').querySelector('.tool-version')!.textContent).toBe('v0.6.9');
    expect(row(host, 'Semgrep').querySelector('.tool-version')!.textContent).toBe('Managed version');
  });

  it('marks an all-ready managed set with the success counter and no pending chips', async () => {
    const allReady = { items: analyzersBody.items.map((a) => (a.managed_tool ? { ...a, ready: true } : a)) };
    const fetchMock = vi.fn((input: string) => (input.endsWith('/analyzers') ? Promise.resolve(json(allReady)) : Promise.resolve(json({}))));
    const { host } = await renderPage(fetchMock);
    expect(host.querySelector('.tools-all-ready')!.textContent).toBe('2 of 2 ready');
    expect(host.querySelectorAll('.tools-readiness .dot').length).toBe(0);
  });

  it('turns the non-ready chip into a spinner while that install is in flight', async () => {
    const pending = new Promise<Response>(() => {});
    const { host } = await renderPage(analyzersMock(() => pending));
    await act(async () => { [...row(host, 'Semgrep').querySelectorAll('button')].find((button) => button.textContent === 'Install')!.click(); });
    const chip = [...host.querySelectorAll('.tools-readiness .badge')].find((badge) => badge.textContent!.includes('Semgrep'))!;
    expect(chip.querySelector('.spinner')).not.toBeNull();
    expect(chip.textContent).toContain('Semgrep installing…');
    expect(row(host, 'Semgrep').querySelector('.table-actions')!.getAttribute('aria-busy')).toBe('true');
    expect(host.querySelector('.tools-readiness')!.textContent).toContain('1 of 2 ready');
  });
});

describe('ToolsPage inventory table', () => {
  it('lists every analyzer from the inventory in one table, built-ins with no install actions', async () => {
    const { host } = await renderPage(analyzersMock());
    const rows = [...host.querySelectorAll('.tool-table tbody tr')];
    // Five inventory rows render: managed, built-in, and offline-withheld.
    expect(rows.length).toBe(5);
    expect(rows.map((r) => r.textContent)).toEqual(expect.arrayContaining([expect.stringContaining('Ruff'), expect.stringContaining('Secrets'), expect.stringContaining('Todo Scanner')]));
    const secrets = row(host, 'Secrets');
    expect(secrets.querySelector('.state.ready')!.textContent).toBe('Built-in');
    const actions = [...secrets.querySelectorAll('button')].filter((button) => ['Install', 'Repair', 'Update'].includes(button.textContent!));
    expect(actions).toHaveLength(0);
    expect(secrets.textContent).not.toContain('Coming soon');
    expect(host.textContent).not.toContain('Secrets not installed');
  });

  it('marks analyzers withheld by offline mode as unavailable instead of silently hiding them', async () => {
    const { host } = await renderPage(analyzersMock());
    const license = row(host, 'License Scanner');
    expect(license.querySelector('.state.not-ready')!.textContent).toBe('Offline mode');
  });

  it('shows scan tiers and network use from the inventory', async () => {
    const { host } = await renderPage(analyzersMock());
    expect(row(host, 'Ruff').textContent).toContain('Quick, Standard, Deep, Pentest');
    expect(row(host, 'Ruff').textContent).toContain('No network');
  });
});

describe('ToolsPage actions', () => {
  it('keeps install firing the same POST endpoint with a success notice naming the analyzer', async () => {
    const fetchMock = analyzersMock((input) => (input.endsWith('/tools/semgrep/install') ? json({ id: 'semgrep', ready: true, can_install: false }) : json({})));
    const { host, notify } = await renderPage(fetchMock);
    await act(async () => { [...row(host, 'Semgrep').querySelectorAll('button')].find((button) => button.textContent === 'Install')!.click(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/tools/semgrep/install', expect.objectContaining({ method: 'POST' }));
    expect(notify).toHaveBeenCalledWith({ kind: 'info', text: 'Semgrep: installed.' });
    expect(notify).not.toHaveBeenCalledWith(expect.objectContaining({ kind: 'error' }));
  });

  it('fires repair and update against their own endpoints', async () => {
    const fetchMock = analyzersMock((input) => (input.endsWith('/tools/ruff/update') || input.endsWith('/tools/ruff/repair') ? json({ id: 'ruff', ready: true }) : json({})));
    const { host } = await renderPage(fetchMock);
    await act(async () => { [...row(host, 'Ruff').querySelectorAll('button')].find((button) => button.textContent === 'Update')!.click(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/tools/ruff/update', expect.objectContaining({ method: 'POST' }));
    await act(async () => { [...row(host, 'Ruff').querySelectorAll('button')].find((button) => button.textContent === 'Repair')!.click(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/tools/ruff/repair', expect.objectContaining({ method: 'POST' }));
  });

  it('retries the analyzers load from the error panel', async () => {
    let failing = true;
    const fetchMock = vi.fn((input: string) => {
      if (input.endsWith('/analyzers')) return failing ? Promise.resolve(json({ error: { code: 'SCANS_UNAVAILABLE', message: 'Scan service unavailable.' } }, 503)) : Promise.resolve(json(analyzersBody));
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

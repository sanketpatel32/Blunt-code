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

/** The Install/Repair/Update buttons inside the open ConfirmationDialog. */
function dialogButton(host: HTMLElement, label: string): HTMLElement | undefined {
  return [...host.querySelectorAll<HTMLElement>('dialog button')].find((button) => button.textContent === label);
}

/** Arms the confirmation dialog from the row's visible Install button, then
 *  confirms it — the visible control only arms the dialog; the POST fires on
 *  confirm. */
async function confirmVisibleAction(host: HTMLElement, id: string, operation: string) {
  await act(async () => { [...row(host, id).querySelectorAll('button')].find((button) => button.textContent === operation)!.click(); });
  await act(async () => { dialogButton(host, operation)!.click(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
}

/** Row actions live in a Radix overflow menu: open the row's menu (its trigger
 *  is the kebab beside the row's single contextual control). */
async function openToolMenu(host: HTMLElement, id: string) {
  const trigger = row(host, id).querySelector<HTMLButtonElement>('button[aria-label^="Actions for"]');
  expect(trigger, 'row exposes an actions menu').toBeDefined();
  await act(async () => {
    trigger!.dispatchEvent(new MouseEvent('pointerdown', { button: 0, bubbles: true, cancelable: true }));
  });
  await act(async () => {});
  return [...document.body.querySelectorAll<HTMLElement>('[role="menuitem"]')];
}

/** Arms the confirmation dialog from an overflow-menu item, then confirms it. */
async function confirmMenuAction(host: HTMLElement, id: string, operation: string) {
  await openToolMenu(host, id);
  const item = [...document.body.querySelectorAll<HTMLElement>('[role="menuitem"]')].find((candidate) => candidate.textContent === operation);
  expect(item, `no "${operation}" menu item`).toBeDefined();
  await act(async () => {
    item!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    item!.click();
  });
  await act(async () => { dialogButton(host, operation)!.click(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
}

/** Expands a row's details via the tool-name disclosure. */
async function expandDetails(host: HTMLElement, id: string) {
  await act(async () => { row(host, id).querySelector<HTMLButtonElement>('button.tools-name')!.click(); });
  await act(async () => {});
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

describe('ToolsPage readiness summary', () => {
  it('summarizes optional tools once in the toolbar meta and keeps readiness in the row, not as chip wallpaper', async () => {
    const { host } = await renderPage(analyzersMock());
    const meta = host.querySelector('.tools-readiness')!;
    expect(meta.textContent).toContain('1 of 2 optional tools ready');
    expect(host.querySelector('.tools-all-ready')).toBeNull();
    // Readiness shows in the row itself: quiet dot + plain text, no spinner yet.
    const semgrep = row(host, 'Semgrep');
    expect(semgrep.querySelector('.dot.not-ready')).not.toBeNull();
    expect(semgrep.querySelector('.tools-state')!.textContent).toBe('Not installed');
    expect(semgrep.querySelector('.spinner')).toBeNull();
    expect(semgrep.querySelector('.tools-version')!.textContent).toBe('—');
    expect(row(host, 'Ruff').querySelector('.tools-version')!.textContent).toBe('v0.6.9');
  });

  it('marks an all-ready managed set with the success counter', async () => {
    const allReady = { items: analyzersBody.items.map((a) => (a.managed_tool ? { ...a, ready: true } : a)) };
    const fetchMock = vi.fn((input: string) => (input.endsWith('/analyzers') ? Promise.resolve(json(allReady)) : Promise.resolve(json({}))));
    const { host } = await renderPage(fetchMock);
    expect(host.querySelector('.tools-all-ready')!.textContent).toBe('2 of 2 optional tools ready');
    expect(host.querySelectorAll('.tools-readiness .dot').length).toBe(0);
  });

  it('shows the operation spinner in the row while that action is in flight', async () => {
    const pending = new Promise<Response>(() => {});
    const { host } = await renderPage(analyzersMock(() => pending));
    await confirmVisibleAction(host, 'Semgrep', 'Install');
    const busyCell = row(host, 'Semgrep').querySelector('.tools-busy')!;
    expect(busyCell.querySelector('.spinner')).not.toBeNull();
    expect(busyCell.textContent).toBe('Installing…');
    expect(row(host, 'Semgrep').querySelector('.table-actions')!.getAttribute('aria-busy')).toBe('true');
    expect(host.querySelector('.tools-readiness')!.textContent).toContain('1 of 2 optional tools ready');
  });
});

describe('ToolsPage inventory table', () => {
  it('lists every analyzer from the inventory in one table, built-ins with no install actions', async () => {
    const { host } = await renderPage(analyzersMock());
    const rows = [...host.querySelectorAll('.tool-table tbody tr:not(.tools-details-row)')];
    // Five inventory rows render: managed, built-in, and offline-withheld.
    expect(rows.length).toBe(5);
    expect(rows.map((r) => r.textContent)).toEqual(expect.arrayContaining([expect.stringContaining('Ruff'), expect.stringContaining('Secrets'), expect.stringContaining('Todo Scanner')]));
    const secrets = row(host, 'Secrets');
    expect(secrets.querySelector('.tools-state')!.textContent).toBe('Ready');
    expect(secrets.textContent).toContain('Built-in');
    const actions = [...secrets.querySelectorAll('button')].filter((button) => ['Install', 'Repair', 'Update'].includes(button.textContent!));
    expect(actions).toHaveLength(0);
    expect(secrets.textContent).not.toContain('Coming soon');
    expect(host.textContent).not.toContain('Secrets not installed');
  });

  it('keeps one contextual control per row: Install visible when missing, overflow menu otherwise', async () => {
    const { host } = await renderPage(analyzersMock());
    const install = row(host, 'Semgrep').querySelector('.table-actions .button.secondary')!;
    expect(install.textContent).toBe('Install');
    // A ready managed tool needs no visible action — everything but the menu lives in overflow.
    expect(row(host, 'Ruff').querySelectorAll('.table-actions button')).toHaveLength(1);
    expect(row(host, 'Ruff').querySelector('.table-actions button')!.getAttribute('aria-label')).toBe('Actions for Ruff');
    const menu = await openToolMenu(host, 'Ruff');
    expect(menu.map((item) => item.textContent)).toEqual(['Update', 'Uninstall', 'Repair']);
  });

  it('marks analyzers withheld by offline mode as unavailable instead of silently hiding them', async () => {
    const { host } = await renderPage(analyzersMock());
    const license = row(host, 'License Scanner');
    expect(license.querySelector('.tools-state.warn')!.textContent).toBe('Offline mode');
  });

  it('collapses secondary detail into an expandable row behind the tool name', async () => {
    const { host } = await renderPage(analyzersMock());
    // Collapsed: the main table stays single-line — no profiles/network text.
    expect(row(host, 'Ruff').textContent).not.toContain('Quick, Standard, Deep, Pentest');
    await expandDetails(host, 'Ruff');
    expect(row(host, 'Ruff').querySelector('button.tools-name')!.getAttribute('aria-expanded')).toBe('true');
    const details = host.querySelector('.tools-details-row')!;
    expect(details.textContent).toContain('Quick, Standard, Deep, Pentest');
    expect(details.textContent).toContain('No network');
    expect(details.textContent).toContain('Code quality');
    await expandDetails(host, 'Ruff');
    expect(host.querySelector('.tools-details-row')).toBeNull();
  });

  it('labels the category with the API inventory vocabulary, not frontend catalog labels', async () => {
    const { host } = await renderPage(analyzersMock());
    await expandDetails(host, 'Ruff');
    const ruff = host.querySelector('.tools-details-row')!.textContent;
    expect(ruff).toContain('Code quality');
    expect(ruff).not.toContain('Lint');
    await expandDetails(host, 'License Scanner');
    expect(host.querySelector('.tools-details-row')!.textContent).toContain('Compliance');
  });
});

describe('ToolsPage toolbar count', () => {
  it('shows an ellipsis count while the inventory loads instead of "0 analyzers"', async () => {
    const fetchMock = vi.fn((input: string) => (input.endsWith('/analyzers') ? new Promise<Response>(() => {}) : Promise.resolve(json({}))));
    const { host } = await renderPage(fetchMock);
    expect(host.querySelector('.tools-readiness')!.textContent).toBe('… analyzers');
    expect(host.querySelector('.segmented button[aria-pressed="true"]')).not.toBeNull();
  });
});

describe('ToolsPage actions', () => {
  it('keeps install firing the same POST endpoint with a success notice naming the analyzer', async () => {
    const fetchMock = analyzersMock((input) => (input.endsWith('/tools/semgrep/install') ? json({ id: 'semgrep', ready: true, can_install: false }) : json({})));
    const { host, notify } = await renderPage(fetchMock);
    await confirmVisibleAction(host, 'Semgrep', 'Install');
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/tools/semgrep/install', expect.objectContaining({ method: 'POST' }));
    expect(notify).toHaveBeenCalledWith({ kind: 'info', text: 'Semgrep: installed.' });
    expect(notify).not.toHaveBeenCalledWith(expect.objectContaining({ kind: 'error' }));
  });

  it('arms a confirmation dialog naming the tool + operation, and cancel fires no POST', async () => {
    const fetchMock = analyzersMock();
    const { host } = await renderPage(fetchMock);
    await openToolMenu(host, 'Semgrep');
    const item = [...document.body.querySelectorAll<HTMLElement>('[role="menuitem"]')].find((candidate) => candidate.textContent === 'Repair');
    expect(item, 'no "Repair" menu item').toBeDefined();
    await act(async () => {
      item!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
      item!.click();
    });
    await act(async () => {});
    const dialog = host.querySelector('dialog[open]')!;
    expect(dialog).not.toBeNull();
    expect(dialog.textContent).toContain('Repair Semgrep');
    expect(dialog.textContent).toContain('Repair re-runs setup for Semgrep');
    expect(fetchMock).not.toHaveBeenCalledWith('/api/v1/tools/semgrep/repair', expect.anything());
    await act(async () => { dialogButton(host, 'Cancel')!.click(); });
    expect(host.querySelector('dialog')).toBeNull();
    expect(fetchMock).not.toHaveBeenCalledWith('/api/v1/tools/semgrep/repair', expect.anything());
  });

  it('fires repair and update against their own endpoints', async () => {
    const fetchMock = analyzersMock((input) => (input.endsWith('/tools/ruff/update') || input.endsWith('/tools/ruff/repair') ? json({ id: 'ruff', ready: true }) : json({})));
    const { host } = await renderPage(fetchMock);
    await confirmMenuAction(host, 'Ruff', 'Update');
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/tools/ruff/update', expect.objectContaining({ method: 'POST' }));
    await confirmMenuAction(host, 'Ruff', 'Repair');
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/tools/ruff/repair', expect.objectContaining({ method: 'POST' }));
  });

  it('fires uninstall against DELETE endpoint and shows success notice', async () => {
    const fetchMock = analyzersMock((input) => (input.endsWith('/tools/ruff') ? json({ id: 'ruff', ready: false, can_install: true }) : json({})));
    const { host, notify } = await renderPage(fetchMock);
    await confirmMenuAction(host, 'Ruff', 'Uninstall');
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/tools/ruff', expect.objectContaining({ method: 'DELETE' }));
    expect(notify).toHaveBeenCalledWith({ kind: 'info', text: 'Ruff: uninstalled.' });
  });

  it('displays disk usage in expanded details and uninstall confirmation dialog', async () => {
    const withDisk = {
      items: analyzersBody.items.map((a) =>
        a.id === 'ruff' ? { ...a, disk_bytes: 52_428_800 } : a
      ),
    };
    const fetchMock = vi.fn((input: string) =>
      input.endsWith('/analyzers') ? Promise.resolve(json(withDisk)) : Promise.resolve(json({}))
    );
    const { host } = await renderPage(fetchMock);
    await expandDetails(host, 'Ruff');
    expect(host.textContent).toContain('Disk usage');
    expect(host.textContent).toContain('50.0 MB');

    await openToolMenu(host, 'Ruff');
    const item = [...document.body.querySelectorAll<HTMLElement>('[role="menuitem"]')].find(
      (c) => c.textContent === 'Uninstall'
    );
    await act(async () => {
      item!.click();
    });
    const dialog = host.querySelector('dialog[open]')!;
    expect(dialog.textContent).toContain('freeing 50.0 MB of disk space');
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
    expect(host.querySelector('.tools-toolbar')).toBeNull();
    failing = false;
    await act(async () => { [...host.querySelectorAll('button')].find((button) => button.textContent === 'Try again')!.click(); await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
    expect(host.querySelector('.error-panel')).toBeNull();
    expect(host.querySelector('.tools-readiness')!.textContent).toContain('1 of 2 optional tools ready');
  });
});

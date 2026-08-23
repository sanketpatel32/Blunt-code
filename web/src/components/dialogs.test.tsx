import { act, useState, type ReactNode } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AddWorkspaceDialog, ConfirmationDialog, FindingPreviewDialog, SuppressFindingDialog, SUPPRESSION_REASON_MAX } from './dialogs';
import type { Finding } from '../types';
import type { Notice } from '../lib/notice';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function press(target: Element, key: string, shiftKey = false) {
  target.dispatchEvent(new KeyboardEvent('keydown', { key, shiftKey, bubbles: true, cancelable: true }));
}

function pressBackdrop(target: Element) {
  target.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, cancelable: true }));
}

/** Renders a trigger button plus the dialog built by `dialog`, so focus restore has a real trigger. */
function Harness({ dialog }: { dialog: (close: () => void) => ReactNode }) {
  const [open, setOpen] = useState(false);
  return <>
    <button type="button" onClick={() => setOpen(true)}>Open dialog</button>
    {open && dialog(() => setOpen(false))}
  </>;
}

async function renderHarness(dialog: (close: () => void) => ReactNode) {
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<Harness dialog={dialog} />); });
  return host;
}

async function openDialog(host: HTMLElement) {
  const trigger = host.querySelector<HTMLButtonElement>('button')!;
  await act(async () => { trigger.click(); });
  return trigger;
}

function confirmation(close: () => void, busy = false) {
  return <ConfirmationDialog title="Remove this workspace?" description="Your project files will not be changed." confirmLabel="Remove workspace" busy={busy} onCancel={close} onConfirm={() => {}} />;
}

const finding: Finding = { id: 'finding-1', analyzer_id: 'biome', severity: 'high', category: 'correctness', title: 'Example finding', message: 'Undefined name', relative_path: 'src/main.py', start_line: 4 };

const FINGERPRINT = 'a'.repeat(64);
const suppressedFinding: Finding = { ...finding, fingerprint: FINGERPRINT };

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function suppressDialog(close: () => void, onSuppressed: (reason: string) => void = () => {}, notify: (n: Notice) => void = () => {}) {
  return <SuppressFindingDialog workspaceId="ws-1" finding={suppressedFinding} onClose={close} onSuppressed={onSuppressed} notify={notify} />;
}

afterEach(async () => { await act(async () => { root?.unmount(); }); document.body.replaceChildren(); vi.unstubAllGlobals(); });

describe('dialog accessibility', () => {
  it('marks the confirmation dialog modal and labelled, and focuses Cancel on open', async () => {
    const host = await renderHarness(confirmation);
    await openDialog(host);
    const dialog = host.querySelector('dialog')!;
    expect(dialog).not.toBeNull();
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    expect(dialog.getAttribute('aria-labelledby')).toBe('confirmation-title');
    expect(document.getElementById('confirmation-title')).not.toBeNull();
    const cancel = [...dialog.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent === 'Cancel')!;
    expect(document.activeElement).toBe(cancel);
  });

  it('traps Tab: last focusable wraps to first and Shift+Tab from first wraps to last', async () => {
    const host = await renderHarness(confirmation);
    await openDialog(host);
    const dialog = host.querySelector('dialog')!;
    const buttons = [...dialog.querySelectorAll<HTMLButtonElement>('button')];
    const [close, , confirm] = buttons;
    confirm.focus();
    press(confirm, 'Tab');
    expect(document.activeElement).toBe(close);
    press(close, 'Tab', true);
    expect(document.activeElement).toBe(confirm);
  });

  it('closes on Escape and restores focus to the trigger button', async () => {
    const host = await renderHarness(confirmation);
    const trigger = host.querySelector<HTMLButtonElement>('button')!;
    await act(async () => { trigger.focus(); trigger.click(); });
    expect(host.querySelector('dialog')).not.toBeNull();
    await act(async () => { press(document.activeElement!, 'Escape'); });
    expect(host.querySelector('dialog')).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it('closes on backdrop mousedown but not on mousedown inside the dialog', async () => {
    const host = await renderHarness(confirmation);
    await openDialog(host);
    const backdrop = host.querySelector('.dialog-backdrop')!;
    const dialog = host.querySelector('dialog')!;
    const heading = dialog.querySelector('h2')!;
    const cancel = [...dialog.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent === 'Cancel')!;
    await act(async () => { pressBackdrop(cancel); pressBackdrop(heading); pressBackdrop(dialog); });
    expect(host.querySelector('dialog')).not.toBeNull();
    await act(async () => { pressBackdrop(backdrop); });
    expect(host.querySelector('dialog')).toBeNull();
  });

  it('ignores Escape and backdrop mousedown while the confirmation dialog is busy', async () => {
    const host = await renderHarness((close) => confirmation(close, true));
    await openDialog(host);
    const backdrop = host.querySelector('.dialog-backdrop')!;
    await act(async () => { press(document.body, 'Escape'); pressBackdrop(backdrop); });
    expect(host.querySelector('dialog')).not.toBeNull();
    expect(host.textContent).toContain('Remove this workspace?');
  });

  it('gives the add-workspace dialog a trap, modal semantics, Escape and backdrop close', async () => {
    const host = await renderHarness((close) => <AddWorkspaceDialog onClose={close} onCreated={() => {}} notify={() => {}} />);
    await openDialog(host);
    const dialog = host.querySelector('dialog')!;
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    expect(dialog.getAttribute('aria-labelledby')).toBe('add-workspace-title');
    expect(document.getElementById('add-workspace-title')).not.toBeNull();
    const pathInput = host.querySelector<HTMLInputElement>('input[required]')!;
    expect(document.activeElement).toBe(pathInput);
    const focusables = [...dialog.querySelectorAll<HTMLElement>('button, input')].filter((element) => !(element as HTMLButtonElement).disabled);
    const [first, , , nameInput, last] = focusables;
    expect(first.getAttribute('aria-label')).toBe('Close');
    expect(last.textContent).toBe('Cancel'); // the disabled Add button is skipped
    await act(async () => { nameInput.focus(); press(nameInput, 'Tab'); });
    expect(document.activeElement).toBe(last);
    await act(async () => { last.focus(); press(last, 'Tab'); });
    expect(document.activeElement).toBe(first);
    await act(async () => { pressBackdrop(host.querySelector('.dialog-backdrop')!); });
    expect(host.querySelector('dialog')).toBeNull();
    await openDialog(host);
    await act(async () => { press(document.activeElement!, 'Escape'); });
    expect(host.querySelector('dialog')).toBeNull();
  });

  it('gives the source preview dialog modal semantics, trap, Escape and backdrop close', async () => {
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(() => {})));
    const host = await renderHarness((close) => <FindingPreviewDialog scanId="scan-1" finding={finding} onClose={close} />);
    await openDialog(host);
    const dialog = host.querySelector('dialog')!;
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    expect(dialog.getAttribute('aria-labelledby')).toBe('source-preview-title');
    expect(document.getElementById('source-preview-title')).not.toBeNull();
    const headerClose = dialog.querySelector<HTMLButtonElement>('header button')!;
    expect(document.activeElement).toBe(headerClose);
    const footerClose = [...dialog.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent === 'Close')!;
    await act(async () => { footerClose.focus(); press(footerClose, 'Tab'); });
    expect(document.activeElement).toBe(headerClose);
    await act(async () => { pressBackdrop(host.querySelector('.dialog-backdrop')!); });
    expect(host.querySelector('dialog')).toBeNull();
    await openDialog(host);
    await act(async () => { press(document.activeElement!, 'Escape'); });
    expect(host.querySelector('dialog')).toBeNull();
  });
});

describe('suppress finding dialog', () => {
  const fetchMock = vi.fn((_input: string, _init?: RequestInit) => Promise.resolve(json({ workspace_id: 'ws-1', fingerprint: FINGERPRINT, created_at: '2026-08-24T00:00:00Z' }, 201)));

  async function typeReason(textarea: HTMLTextAreaElement, value: string) {
    const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value')!.set!;
    await act(async () => {
      setter.call(textarea, value);
      textarea.dispatchEvent(new Event('input', { bubbles: true }));
    });
  }

  async function flush() {
    await act(async () => { await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); });
  }

  it('is modal and labelled, focuses the reason textarea, and caps the reason at 500 characters', async () => {
    vi.stubGlobal('fetch', fetchMock);
    const host = await renderHarness(suppressDialog);
    await openDialog(host);
    const dialog = host.querySelector('dialog')!;
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    expect(dialog.getAttribute('aria-labelledby')).toBe('suppress-finding-title');
    expect(document.getElementById('suppress-finding-title')).not.toBeNull();
    const textarea = dialog.querySelector<HTMLTextAreaElement>('#suppress-finding-reason')!;
    expect(textarea.maxLength).toBe(500);
    expect(SUPPRESSION_REASON_MAX).toBe(500);
    expect(document.activeElement).toBe(textarea); // the note field is the primary control
    expect(textarea.getAttribute('aria-describedby')).toContain('suppress-finding-hint');
    expect(dialog.textContent).toContain('Suppressing hides this finding from future scans, reports, and the CI gate.');
    expect(dialog.textContent).toContain('Example finding'); // names the finding being hidden
  });

  it('confirms with a POST of the fingerprint and reason, then hands back to the caller', async () => {
    vi.stubGlobal('fetch', fetchMock);
    fetchMock.mockClear();
    const onSuppressed = vi.fn();
    const host = await renderHarness((close) => suppressDialog(close, (reason) => { onSuppressed(reason); close(); })); // callers close on success, like ReportView
    await openDialog(host);
    await typeReason(host.querySelector<HTMLTextAreaElement>('#suppress-finding-reason')!, 'False positive for this project');
    await act(async () => { ([...host.querySelectorAll('button')].find((button) => button.textContent === 'Suppress finding') as HTMLButtonElement).click(); });
    await flush();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [input, init] = fetchMock.mock.calls[0];
    expect(input).toBe('/api/v1/workspaces/ws-1/suppressions');
    expect(init?.method).toBe('POST');
    expect(JSON.parse(String(init?.body))).toEqual({ fingerprint: FINGERPRINT, reason: 'False positive for this project' });
    expect(onSuppressed).toHaveBeenCalledWith('False positive for this project');
    expect(host.querySelector('dialog')).toBeNull(); // caller closed it on success
  });

  it('posts an empty reason when the note is left blank', async () => {
    vi.stubGlobal('fetch', fetchMock);
    fetchMock.mockClear();
    const host = await renderHarness((close) => suppressDialog(close));
    await openDialog(host);
    await act(async () => { ([...host.querySelectorAll('button')].find((button) => button.textContent === 'Suppress finding') as HTMLButtonElement).click(); });
    await flush();
    const [, init] = fetchMock.mock.calls[0];
    expect(JSON.parse(String(init?.body))).toEqual({ fingerprint: FINGERPRINT, reason: '' });
  });

  it('closes on Escape and on Cancel without posting anything', async () => {
    vi.stubGlobal('fetch', fetchMock);
    fetchMock.mockClear();
    const host = await renderHarness(suppressDialog);
    const trigger = host.querySelector<HTMLButtonElement>('button')!;
    await act(async () => { trigger.focus(); trigger.click(); }); // focus the trigger so close can restore focus, like a real row button
    expect(host.querySelector('dialog')).not.toBeNull();
    await act(async () => { press(document.activeElement!, 'Escape'); });
    expect(host.querySelector('dialog')).toBeNull();
    expect(document.activeElement).toBe(trigger); // focus returns to the row that opened it
    await openDialog(host);
    await act(async () => { ([...host.querySelectorAll('button')].find((button) => button.textContent === 'Cancel') as HTMLButtonElement).click(); });
    expect(host.querySelector('dialog')).toBeNull();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('keeps the dialog open and raises the error notice when the post fails', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(json({ error: { code: 'DATABASE_ERROR', message: 'Could not save suppression.' } }, 500))));
    const notify = vi.fn();
    const host = await renderHarness((close) => suppressDialog(close, () => {}, notify));
    await openDialog(host);
    await act(async () => { ([...host.querySelectorAll('button')].find((button) => button.textContent === 'Suppress finding') as HTMLButtonElement).click(); });
    await flush();
    expect(notify).toHaveBeenCalledWith({ kind: 'error', text: 'DATABASE_ERROR: Could not save suppression.' });
    expect(host.querySelector('dialog')).not.toBeNull(); // nothing is lost; the user can retry or cancel
  });
});

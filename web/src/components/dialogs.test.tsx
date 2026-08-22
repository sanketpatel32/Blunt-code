import { act, useState, type ReactNode } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AddWorkspaceDialog, ConfirmationDialog, FindingPreviewDialog } from './dialogs';
import type { Finding } from '../types';

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

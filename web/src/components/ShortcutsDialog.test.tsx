import { act, useState, type ReactNode } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it } from 'vitest';
import { ShortcutsDialog } from './ShortcutsDialog';

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

/** Renders a trigger button plus the dialog, so focus restore has a real trigger. */
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
  await act(async () => { trigger.focus(); trigger.click(); });
  return trigger;
}

afterEach(async () => { await act(async () => { root?.unmount(); }); document.body.replaceChildren(); });

describe('shortcuts help dialog', () => {
  it('is a labelled modal that lists every shortcut grouped by purpose', async () => {
    const host = await renderHarness((close) => <ShortcutsDialog onClose={close} />);
    await openDialog(host);
    const dialog = host.querySelector('dialog')!;
    expect(dialog).not.toBeNull();
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    expect(dialog.getAttribute('aria-labelledby')).toBe('shortcuts-title');
    expect(document.getElementById('shortcuts-title')?.textContent).toBe('Keyboard shortcuts');
    expect([...dialog.querySelectorAll('.shortcut-group h3')].map((heading) => heading.textContent)).toEqual(['Navigation', 'Actions', 'Search']);
    const rows = [...dialog.querySelectorAll('.shortcut-row')];
    expect(rows).toHaveLength(10);
    const descriptions = rows.map((row) => row.querySelector('dd')?.textContent);
    expect(descriptions).toContain('Go to Home');
    expect(descriptions).toContain('Go to Workspaces');
    expect(descriptions).toContain('Go to Tools');
    expect(descriptions).toContain('Go to Settings');
    expect(descriptions).toContain('Go to About');
    expect(descriptions).toContain('Add a workspace');
    expect(descriptions).toContain('Focus the findings or file search');
    const firstRow = rows[0];
    expect([...firstRow.querySelectorAll('dt kbd')].map((kbd) => kbd.textContent)).toEqual(['Ctrl', 'K']);
    expect(firstRow.querySelector('dt kbd')?.className).toBe('kbd-hint');
  });

  it('focuses the Got it button on open and closes on click, restoring the trigger', async () => {
    const host = await renderHarness((close) => <ShortcutsDialog onClose={close} />);
    const trigger = await openDialog(host);
    const gotIt = [...host.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent === 'Got it')!;
    expect(document.activeElement).toBe(gotIt);
    await act(async () => { gotIt.click(); });
    expect(host.querySelector('dialog')).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it('closes on Escape and on backdrop mousedown, but not from mousedown inside', async () => {
    const host = await renderHarness((close) => <ShortcutsDialog onClose={close} />);
    await openDialog(host);
    const heading = host.querySelector('dialog h2')!;
    await act(async () => { pressBackdrop(heading); });
    expect(host.querySelector('dialog')).not.toBeNull();
    await act(async () => { press(document.activeElement!, 'Escape'); });
    expect(host.querySelector('dialog')).toBeNull();
    await openDialog(host);
    await act(async () => { pressBackdrop(host.querySelector('.dialog-backdrop')!); }); // re-queried: reopen remounts the backdrop
    expect(host.querySelector('dialog')).toBeNull();
  });
});

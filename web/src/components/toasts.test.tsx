import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ToastStack, useToasts } from './toasts';
import type { Notice } from '../lib/notice';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let notify: (n: Notice) => void;

function Harness() {
  const { toasts, notify: push, dismiss } = useToasts();
  notify = push;
  return <ToastStack toasts={toasts} onDismiss={dismiss} />;
}

async function render() {
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<Harness />); });
  return host;
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.useRealTimers();
});

describe('toast notifications', () => {
  it('renders the human message with the error code as subtext and alert semantics', async () => {
    const host = await render();
    await act(async () => { notify(null); });
    expect(host.querySelectorAll('.toast')).toHaveLength(0);

    await act(async () => { notify({ kind: 'error', text: 'SCAN_ALREADY_ACTIVE: A scan is already running for this workspace.' }); });
    expect(host.textContent).toContain('A scan is already running for this workspace.');
    expect(host.querySelector('small')?.textContent).toBe('SCAN_ALREADY_ACTIVE');
    const alert = host.querySelector('[role="alert"]');
    expect(alert?.textContent).toContain('SCAN_ALREADY_ACTIVE');

    await act(async () => { notify({ kind: 'info', text: 'ruff: installed.' }); });
    await act(async () => { notify({ kind: 'success', text: 'Workspace removed from Blunt Code.' }); });
    expect(host.textContent).toContain('ruff: installed.');
    expect(host.querySelectorAll('[role="alert"]')).toHaveLength(1);

    const region = host.querySelector('[role="status"]');
    expect(region?.getAttribute('aria-live')).toBe('polite');
    expect(host.querySelector('button[aria-label="Dismiss notification"]')).not.toBeNull();
  });

  it('stacks newest first and silently drops the oldest beyond four', async () => {
    const host = await render();
    for (let index = 1; index <= 5; index++) {
      await act(async () => { notify({ kind: 'info', text: `toast ${index}` }); });
    }
    const texts = [...host.querySelectorAll('.toast .toast-text')].map((node) => node.textContent);
    expect(texts).toEqual(['toast 5', 'toast 4', 'toast 3', 'toast 2']);
    expect(host.textContent).not.toContain('toast 1');
  });

  it('auto-dismisses info toasts after five seconds and errors after twelve', async () => {
    vi.useFakeTimers();
    const host = await render();
    await act(async () => { notify({ kind: 'info', text: 'File selection saved.' }); });
    act(() => { vi.advanceTimersByTime(10); });
    expect(host.querySelector('.toast')?.getAttribute('data-state')).toBe('open');

    act(() => { vi.advanceTimersByTime(4999); });
    expect(host.textContent).toContain('File selection saved.');
    act(() => { vi.advanceTimersByTime(1000); });
    expect(host.textContent).not.toContain('File selection saved.');

    await act(async () => { notify({ kind: 'error', text: 'REQUEST_FAILED: Request failed (500).' }); });
    act(() => { vi.advanceTimersByTime(6000); });
    expect(host.textContent).toContain('Request failed (500).');
    act(() => { vi.advanceTimersByTime(6500); });
    expect(host.textContent).not.toContain('Request failed (500).');
  });

  it('pauses the auto-dismiss countdown while the toast is focused', async () => {
    vi.useFakeTimers();
    const host = await render();
    await act(async () => { notify({ kind: 'info', text: 'Paused toast.' }); });
    const toast = host.querySelector('.toast')!;
    act(() => { toast.dispatchEvent(new FocusEvent('focusin', { bubbles: true })); });
    act(() => { vi.advanceTimersByTime(30000); });
    expect(host.textContent).toContain('Paused toast.');
    act(() => { toast.dispatchEvent(new FocusEvent('focusout', { bubbles: true })); });
    act(() => { vi.advanceTimersByTime(5400); });
    expect(host.textContent).not.toContain('Paused toast.');
  });

  it('Escape dismisses the newest toast and the button dismisses the rest', async () => {
    vi.useFakeTimers();
    const host = await render();
    await act(async () => { notify({ kind: 'info', text: 'first toast' }); });
    await act(async () => { notify({ kind: 'info', text: 'second toast' }); });
    expect([...host.querySelectorAll('.toast .toast-text')].map((node) => node.textContent)).toEqual(['second toast', 'first toast']);

    act(() => { window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })); });
    act(() => { vi.advanceTimersByTime(500); });
    expect(host.textContent).not.toContain('second toast');
    expect(host.textContent).toContain('first toast');

    const button = host.querySelector<HTMLButtonElement>('button[aria-label="Dismiss notification"]');
    expect(button).not.toBeNull();
    act(() => { button!.click(); });
    act(() => { vi.advanceTimersByTime(500); });
    expect(host.textContent).not.toContain('first toast');
  });
});

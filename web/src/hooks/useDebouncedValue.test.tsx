import { act, useState } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useDebouncedValue } from './useDebouncedValue';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let controls: { set: (value: string) => void } | undefined;
let rendered: string | undefined;

/** Renders the hook through a probe component and records what it returned last. */
function Probe({ delayMs }: { delayMs: number }) {
  const [value, set] = useState('first');
  const debounced = useDebouncedValue(value, delayMs);
  controls = { set };
  rendered = debounced;
  return null;
}

async function render(delayMs: number) {
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<Probe delayMs={delayMs} />); });
  return host;
}

async function changeValue(value: string) {
  await act(async () => { controls!.set(value); });
}

async function advance(ms: number) {
  await act(async () => { vi.advanceTimersByTime(ms); });
}

beforeEach(() => { vi.useFakeTimers(); });

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.useRealTimers();
  controls = undefined;
  rendered = undefined;
});

describe('useDebouncedValue', () => {
  it('returns the initial value without waiting', async () => {
    await render(300);
    expect(rendered).toBe('first');
  });

  it('holds the previous value until the delay elapses', async () => {
    await render(300);
    await changeValue('second');
    expect(rendered).toBe('first');
    await advance(299);
    expect(rendered).toBe('first');
    await advance(1);
    expect(rendered).toBe('second');
  });

  it('restarts the timer on every change so only the settled value lands', async () => {
    await render(300);
    await changeValue('second');
    await advance(200);
    await changeValue('third');
    await advance(200);
    expect(rendered).toBe('first');
    await advance(100);
    expect(rendered).toBe('third');
  });

  it('passes values through synchronously when debouncing is disabled', async () => {
    await render(0);
    await changeValue('second');
    expect(rendered).toBe('second');
  });

  it('flushes a pending update immediately when the delay drops to zero', async () => {
    const host = await render(300);
    await changeValue('second');
    await advance(100);
    expect(rendered).toBe('first');
    await act(async () => { root.render(<Probe delayMs={0} />); });
    expect(rendered).toBe('second');
    await advance(300);
    expect(rendered).toBe('second');
    expect(host.textContent).toBe('');
  });

  it('cancels pending timers on unmount', async () => {
    await render(300);
    await changeValue('second');
    const errors = vi.spyOn(console, 'error').mockImplementation(() => {});
    await act(async () => { root.unmount(); });
    root = undefined as unknown as Root;
    vi.advanceTimersByTime(1_000);
    expect(errors).not.toHaveBeenCalled();
    errors.mockRestore();
  });
});

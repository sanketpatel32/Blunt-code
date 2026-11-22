import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it } from 'vitest';
import { useLoad } from './useLoad';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

type Deferred = { key: string; signal: AbortSignal; resolve: (value: string) => void; reject: (error: unknown) => void };

let root: Root | undefined;
let pending: Deferred[] = [];
let seen: Array<{ id: string; data?: string; loading: boolean; error?: string }> = [];
let latest: ReturnType<typeof useLoad<string>> | undefined;

function loader(key: string) {
  return (signal: AbortSignal) => new Promise<string>((resolve, reject) => { pending.push({ key, signal, resolve, reject }); });
}

function Probe({ id }: { id: string }) {
  const state = useLoad(loader(id), [id]);
  latest = state;
  seen.push({ id, data: state.data, loading: state.loading, error: state.error });
  return null;
}

async function render(id: string) {
  if (!root) root = createRoot(document.createElement('div'));
  await act(async () => { root!.render(<Probe id={id} />); });
}

async function settle(index: number, value: string) {
  await act(async () => { pending[index].resolve(value); });
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  root = undefined;
  pending = [];
  seen = [];
  latest = undefined;
});

describe('useLoad', () => {
  it('never reports the previous key\'s data under a new key', async () => {
    await render('a');
    await settle(0, 'data-a');
    expect(latest).toMatchObject({ data: 'data-a', loading: false });

    seen = [];
    await render('b');
    // Every render for key "b" — including the very first one, before any effect runs — is empty and loading.
    expect(seen.filter((s) => s.id === 'b').every((s) => s.data === undefined && s.loading)).toBe(true);
    await settle(1, 'data-b');
    expect(latest).toMatchObject({ data: 'data-b', loading: false });
  });

  it('aborts the in-flight request when deps change and ignores its late response', async () => {
    await render('a');
    await render('b');
    expect(pending[0].signal.aborted).toBe(true);
    expect(pending[1].signal.aborted).toBe(false);
    await settle(0, 'stale-a');
    expect(latest?.data).toBeUndefined();
    await settle(1, 'data-b');
    expect(latest?.data).toBe('data-b');
  });

  it('aborts the in-flight request on unmount', async () => {
    await render('a');
    const { signal } = pending[0];
    await act(async () => { root!.unmount(); });
    root = undefined;
    expect(signal.aborted).toBe(true);
  });

  it('keeps current data on screen during a manual reload of the same key', async () => {
    await render('a');
    await settle(0, 'first');
    await act(async () => { void latest!.reload(); });
    expect(pending[0].signal.aborted).toBe(false);
    expect(latest).toMatchObject({ data: 'first', loading: true });
    await settle(1, 'second');
    expect(latest).toMatchObject({ data: 'second', loading: false });
  });

  it('drops an error from the previous key when deps change', async () => {
    await render('a');
    await act(async () => { pending[0].reject(new Error('boom')); });
    expect(latest?.error).toBe('boom');
    await render('b');
    expect(latest).toMatchObject({ error: undefined, data: undefined, loading: true });
  });
});

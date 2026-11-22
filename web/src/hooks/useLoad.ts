import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { message } from '../lib/notice';

type Snapshot<T> = { key: object; data?: T; error?: string; loading: boolean };

/**
 * Load `load()` whenever `deps` change. Results are stamped with the deps
 * generation that produced them, so the moment deps change the hook reports
 * `data: undefined, loading: true` instead of the previous key's data under
 * the new key. A manual `reload()` keeps the current data on screen while it
 * refreshes. Each run gets an AbortSignal that is aborted when deps change, a
 * newer run starts, or the component unmounts; aborted/stale runs never touch
 * state.
 */
export function useLoad<T>(load: (signal: AbortSignal) => Promise<T>, deps: unknown[]) {
  // A fresh identity per deps generation. The dependency list is supplied by
  // each caller (like useCallback itself), so it cannot be an array literal.
  // biome-ignore lint/correctness/useExhaustiveDependencies: deps is the caller-provided dependency array; each call site lists the exact values it reads.
  const key = useMemo(() => ({}), deps);
  const [snapshot, setSnapshot] = useState<Snapshot<T>>(() => ({ key, loading: true }));
  const runId = useRef(0);
  const controller = useRef<AbortController | null>(null);
  // biome-ignore lint/correctness/useExhaustiveDependencies: `load` is captured per deps generation, exactly like the caller's useCallback.
  const reload = useCallback(async () => {
    controller.current?.abort();
    const ctrl = new AbortController();
    controller.current = ctrl;
    const id = ++runId.current;
    setSnapshot((s) => (s.key === key ? { ...s, error: undefined, loading: true } : { key, loading: true }));
    try {
      const next = await load(ctrl.signal);
      if (id !== runId.current) return;
      setSnapshot({ key, data: next, loading: false });
    } catch (e) {
      if (id !== runId.current) return;
      setSnapshot((s) => ({ key, data: s.key === key ? s.data : undefined, error: message(e), loading: false }));
    } finally {
      if (controller.current === ctrl) controller.current = null;
    }
  }, [key]);
  useEffect(() => {
    void reload();
    return () => {
      ++runId.current;
      controller.current?.abort();
      controller.current = null;
    };
  }, [reload]);
  const current: Snapshot<T> = snapshot.key === key ? snapshot : { key, loading: true };
  return { data: current.data, error: current.error, loading: current.loading, reload };
}

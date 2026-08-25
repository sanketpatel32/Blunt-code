import { useCallback, useEffect, useRef, useState } from 'react';
import { message } from '../lib/notice';

export function useLoad<T>(load: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T>();
  const [error, setError] = useState<string>();
  const [loading, setLoading] = useState(true);
  // Stale-response guard: each invocation gets a sequence number and only the
  // newest one may touch state. Rapid dep changes (typing into a filter,
  // switching workspaces) can otherwise let an older promise resolve last and
  // overwrite fresh data with stale data.
  const runId = useRef(0);
  // The dependency list is supplied by each caller (like useCallback itself),
  // so it cannot be an array literal here by design.
  // biome-ignore lint/correctness/useExhaustiveDependencies: deps is the caller-provided dependency array; each call site lists the exact values it reads.
  const reload = useCallback(async () => {
    const id = ++runId.current;
    setLoading(true);
    setError(undefined);
    try {
      const next = await load();
      if (id !== runId.current) return;
      setData(next);
    } catch (e) {
      if (id !== runId.current) return;
      setError(message(e));
    } finally {
      if (id === runId.current) setLoading(false);
    }
  }, deps);
  useEffect(() => { void reload(); }, [reload]);
  return { data, error, loading, reload };
}

import { useCallback, useEffect, useState } from 'react';
import { message } from '../lib/notice';

export function useLoad<T>(load: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T>();
  const [error, setError] = useState<string>();
  const [loading, setLoading] = useState(true);
  // The dependency list is supplied by each caller (like useCallback itself),
  // so it cannot be an array literal here by design.
  // biome-ignore lint/correctness/useExhaustiveDependencies: deps is the caller-provided dependency array; each call site lists the exact values it reads.
  const reload = useCallback(async () => { setLoading(true); setError(undefined); try { setData(await load()); } catch (e) { setError(message(e)); } finally { setLoading(false); } }, deps);
  useEffect(() => { void reload(); }, [reload]);
  return { data, error, loading, reload };
}

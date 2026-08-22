import { useCallback, useEffect, useState } from 'react';
import { message } from '../lib/notice';

export function useLoad<T>(load: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T>();
  const [error, setError] = useState<string>();
  const [loading, setLoading] = useState(true);
  const reload = useCallback(async () => { setLoading(true); setError(undefined); try { setData(await load()); } catch (e) { setError(message(e)); } finally { setLoading(false); } }, deps); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { void reload(); }, [reload]);
  return { data, error, loading, reload };
}

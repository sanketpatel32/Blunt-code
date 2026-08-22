import { useEffect, useState } from 'react';

/** Returns `value`, but only updates once it has stayed unchanged for `delayMs`.
 *  Fast-changing input (typing) keeps rendering responsively while expensive
 *  work keyed to the returned value (API calls, tree filtering) runs once the
 *  input settles.
 *
 *  Passing `delayMs <= 0` disables debouncing entirely: the value flushes
 *  synchronously with no stale intermediate render. Callers use this to opt
 *  specific transitions (clearing or resetting a filter) out of the delay.
 *  Pending timers are cancelled whenever `value` or `delayMs` changes and on
 *  unmount, so only the settled value ever lands. */
export function useDebouncedValue<T>(value: T, delayMs = 300): T {
  const [debounced, setDebounced] = useState<T>(value);
  // Adjust state during render (React's documented derived-state pattern) so a
  // disabled delay never exposes the previous value for even one render.
  if (delayMs <= 0 && !Object.is(debounced, value)) setDebounced(value);
  useEffect(() => {
    if (Object.is(debounced, value)) return;
    const timer = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(timer);
  }, [value, delayMs, debounced]);
  return debounced;
}

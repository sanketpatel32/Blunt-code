import { useEffect, useState } from 'react';

/** Re-renders the caller once per second while `active`. Used for live elapsed
 *  clocks on running scans; pauses (and clears its timer) the moment the scan
 *  reaches a terminal state so idle pages never wake up to tick. */
export function useTicker(active: boolean, periodMs = 1000): number {
  const [tick, setTick] = useState(0);
  useEffect(() => {
    if (!active) return;
    const timer = window.setInterval(() => setTick((value) => value + 1), periodMs);
    return () => window.clearInterval(timer);
  }, [active, periodMs]);
  return tick;
}

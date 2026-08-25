import { useEffect, useRef, useState } from 'react';

/** Animates a displayed number from its previous value to `target` over
 *  `duration` ms with one rAF tick per frame. Respects prefers-reduced-motion:
 *  under it (or when duration is 0) the value jumps straight to the target.
 *  Non-finite targets render as 0 so hostile API data cannot wedge the timer. */
export function useCountUp(target: number, duration = 480): number {
  const [display, setDisplay] = useState(() => (Number.isFinite(target) ? target : 0));
  const fromRef = useRef(Number.isFinite(target) ? target : 0);
  useEffect(() => {
    const to = Number.isFinite(target) ? target : 0;
    const reduced = typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const from = fromRef.current;
    fromRef.current = to;
    if (reduced || duration <= 0 || from === to) { setDisplay(to); return; }
    let raf = 0;
    const start = performance.now();
    const tick = (now: number) => {
      const t = Math.min((now - start) / duration, 1);
      // ease-out cubic: fast start, gentle landing
      const eased = 1 - (1 - t) ** 3;
      setDisplay(Math.round(from + (to - from) * eased));
      if (t < 1) raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [target, duration]);
  return display;
}

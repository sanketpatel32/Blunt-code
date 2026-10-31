import { useEffect, useRef } from 'react';

/**
 * A count-up number. It renders the final value in the DOM from the first
 * paint (screen readers and tests read the real number), then — only when the
 * user allows motion — tweens the visible text from the previous value. No layout shift: tabular figures and the final string's width.
 */
export function AnimatedNumber({ value, className, format = (n: number) => Math.round(n).toLocaleString(), duration = 0.7 }: { value: number; className?: string; format?: (n: number) => string; duration?: number }) {
  const ref = useRef<HTMLSpanElement>(null);
  const last = useRef(0);
  useEffect(() => {
    const node = ref.current;
    if (!node) return;
    // Animate only when the platform can say motion is welcome; environments
    // without matchMedia (tests, old engines) always show the final value.
    const allowed = typeof window !== 'undefined' && typeof window.matchMedia === 'function' && window.matchMedia('(prefers-reduced-motion: no-preference)').matches;
    if (!allowed) {
      node.textContent = format(value);
      last.current = value;
      return;
    }
    // A tiny rAF tween (ease-out cubic) instead of the motion engine, so the
    // count-up costs nothing in the shell bundle.
    const from = last.current;
    const start = performance.now();
    let frame = 0;
    const step = (now: number) => {
      const t = Math.min(1, (now - start) / (duration * 1000));
      const eased = 1 - Math.pow(1 - t, 3);
      node.textContent = format(from + (value - from) * eased);
      if (t < 1) frame = requestAnimationFrame(step);
    };
    frame = requestAnimationFrame(step);
    last.current = value;
    return () => cancelAnimationFrame(frame);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value, duration]);
  return <span ref={ref} className={`tabular-nums ${className ?? ''}`}>{format(value)}</span>;
}

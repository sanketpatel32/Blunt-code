import { useEffect, useRef } from 'react';
import { animate } from 'motion/react';

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
    const controls = animate(last.current, value, {
      duration,
      ease: [0.22, 1, 0.36, 1],
      onUpdate: (v) => { node.textContent = format(v); },
    });
    last.current = value;
    return () => controls.stop();
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value, duration]);
  return <span ref={ref} className={`tabular-nums ${className ?? ''}`}>{format(value)}</span>;
}

import { useMemo } from 'react';
import { useReducedMotion } from './useReducedMotion';

/** Returns style props for stagger delay: index * 40ms, disabled when reduced-motion. */
export function useStagger(count: number): Array<{ '--stagger-index'?: number; style?: Record<string, string> }> {
  const reduced = useReducedMotion();
  return useMemo(() => {
    if (reduced) return Array.from({ length: count }, () => ({}));
    return Array.from({ length: count }, (_, i) => ({
      style: { '--stagger-delay': `${i * 40}ms` } as Record<string, string>,
    }));
  }, [count, reduced]);
}

/** Helper to get inline style for a single index */
export function staggerStyle(index: number, reduced: boolean): Record<string, string> | undefined {
  if (reduced) return undefined;
  return { '--stagger-delay': `${index * 40}ms` } as Record<string, string>;
}

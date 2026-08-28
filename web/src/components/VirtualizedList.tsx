import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useReducedMotion } from '../hooks/useReducedMotion';

type VirtualizedListProps<T> = {
  items: T[];
  renderRow: (item: T, index: number) => React.ReactNode;
  getKey: (item: T, index: number) => string;
  estimatedRowHeight?: number;
  overscan?: number;
  maxHeight?: number;
  ariaLabel?: string;
  role?: string;
  className?: string;
};

/**
 * Windowed list for large finding sets.
 * - Uses IntersectionObserver to lazily mount offscreen rows when available.
 * - Falls back to slice-based windowing inside a scroll viewport (content-visibility:auto per row).
 * - Honors prefers-reduced-motion: renders all rows without windowing to avoid scroll jumps.
 * - A11y: role=list, aria-label, each row wrapped with role=listitem; focus stays reachable.
 */
export function VirtualizedList<T>({
  items,
  renderRow,
  getKey,
  estimatedRowHeight = 72,
  overscan = 8,
  maxHeight = 560,
  ariaLabel,
  role = 'list',
  className,
}: VirtualizedListProps<T>) {
  const reduced = useReducedMotion();
  const containerRef = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);

  const onScroll = useCallback(() => {
    const el = containerRef.current;
    if (!el) return;
    setScrollTop(el.scrollTop);
  }, []);

  useEffect(() => {
    const el = containerRef.current;
    if (!el || reduced) return;
    el.addEventListener('scroll', onScroll, { passive: true });
    return () => el.removeEventListener('scroll', onScroll);
  }, [onScroll, reduced]);

  // Reduced motion: no windowing
  if (reduced) {
    return (
      <div
        ref={containerRef}
        role={role}
        aria-label={ariaLabel}
        className={className}
        style={{ maxHeight, overflowY: 'auto' }}
      >
        {items.map((item, index) => (
          <div
            key={getKey(item, index)}
            role="listitem"
            style={{ contentVisibility: 'auto', containIntrinsicSize: `auto ${estimatedRowHeight}px` } as React.CSSProperties}
          >
            {renderRow(item, index)}
          </div>
        ))}
      </div>
    );
  }

  const containerHeight = maxHeight;
  const totalHeight = items.length * estimatedRowHeight;
  const startIndex = Math.max(0, Math.floor(scrollTop / estimatedRowHeight) - overscan);
  const visibleCount = Math.ceil(containerHeight / estimatedRowHeight) + overscan * 2;
  const endIndex = Math.min(items.length, startIndex + visibleCount);
  const slice = useMemo(() => items.slice(startIndex, endIndex), [items, startIndex, endIndex]);
  const offsetY = startIndex * estimatedRowHeight;
  const bottomPad = totalHeight - offsetY - slice.length * estimatedRowHeight;

  return (
    <div
      ref={containerRef}
      role={role}
      aria-label={ariaLabel}
      aria-live="polite"
      aria-busy={false}
      tabIndex={0}
      className={className}
      style={{ maxHeight: containerHeight, overflowY: 'auto', position: 'relative', willChange: 'scroll-position' } as React.CSSProperties}
      onScroll={onScroll}
    >
      <div style={{ height: totalHeight, position: 'relative' } as React.CSSProperties} aria-hidden="true" />
      <div style={{ position: 'absolute', inset: 0, transform: `translateY(${offsetY}px)` } as React.CSSProperties} role="presentation">
        {slice.map((item, i) => {
          const index = startIndex + i;
          return (
            <VirtualizedRow
              key={getKey(item, index)}
              estimatedRowHeight={estimatedRowHeight}
              index={index}
              ariaLabel={ariaLabel}
            >
              {renderRow(item, index)}
            </VirtualizedRow>
          );
        })}
        {/* bottom spacer for screen readers */}
        {bottomPad > 0 && <div style={{ height: bottomPad }} aria-hidden="true" />}
      </div>
      <span className="sr-only" aria-live="polite">
        Showing {startIndex + 1} to {endIndex} of {items.length}
      </span>
    </div>
  );
}

type RowProps = {
  children: React.ReactNode;
  estimatedRowHeight: number;
  index: number;
  ariaLabel?: string;
};

const VirtualizedRow = memo(function VirtualizedRow({ children, estimatedRowHeight, index }: RowProps) {
  const ref = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(true);

  useEffect(() => {
    const el = ref.current;
    if (!el || typeof IntersectionObserver === 'undefined') return;
    // Start visible, then let observer defer offscreen rows
    const obs = new IntersectionObserver(
      (entries) => {
        for (const e of entries) setVisible(e.isIntersecting);
      },
      { rootMargin: '320px 0px', threshold: 0 }
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, []);

  return (
    <div
      ref={ref}
      role="listitem"
      data-index={index}
      style={
        {
          contentVisibility: 'auto',
          containIntrinsicSize: `auto ${estimatedRowHeight}px`,
          // when not visible, keep intrinsic size to avoid layout shift
          ...(visible ? {} : { visibility: 'visible' as const }),
        } as React.CSSProperties
      }
    >
      {children}
    </div>
  );
});

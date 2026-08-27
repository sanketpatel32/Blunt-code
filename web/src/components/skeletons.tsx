import type { ReactNode } from 'react';
import { useReducedMotion } from '../hooks/useReducedMotion';

const barWidths = ['74%', '46%', '92%', '58%', '36%'];
const CELL_KEYS = ['c1','c2','c3','c4','c5','c6','c7','c8','c9','c10','c11','c12'];
const ROW_KEYS = ['r1','r2','r3','r4','r5','r6','r7','r8','r9','r10','r11','r12'];
const LINE_KEYS = ['l1','l2','l3','l4','l5','l6','l7','l8','l9','l10','l11','l12'];

function SkeletonStatus({ children }: { children: ReactNode }) {
  return <div className="skeleton-status" role="status" aria-busy="true" aria-live="polite"><span className="sr-only">Loading…</span>{children}</div>;
}

function shimmerStyle(reduced: boolean): string { return reduced ? '' : 'skeleton-anim'; }

export function SkeletonTable({ rows = 5, cols = 5, className }: { rows?: number; cols?: number; className?: string }) {
  const reduced = useReducedMotion();
  const shimmer = shimmerStyle(reduced);
  return <SkeletonStatus>
    <div className={`skeleton-table table-wrap${className ? ` ${className}` : ''}`}><table>
      <thead><tr>{Array.from({ length: cols }, (_, col) => <th key={CELL_KEYS[col]} scope="col"><span className={`skeleton ${shimmer}`} style={{ width: barWidths[(col + 2) % barWidths.length], ...(reduced ? {} : { animationDelay: `${col * 40}ms` } as never) }} /></th>)}</tr></thead>
      <tbody>{Array.from({ length: rows }, (_, row) => <tr key={ROW_KEYS[row]} style={reduced ? undefined : { animationDelay: `${row * 40}ms` } as never} className={reduced ? '' : 'anim-fadeIn anim-stagger'}>{Array.from({ length: cols }, (_, col) => <td key={CELL_KEYS[col]}><span className={`skeleton ${shimmer}`} style={{ width: barWidths[(col + row) % barWidths.length] }} /></td>)}</tr>)}</tbody>
    </table></div>
  </SkeletonStatus>;
}

export function SkeletonCards({ count = 6, variant = 'summary' }: { count?: number; variant?: 'summary' | 'workspace' | 'metric' | 'chart' }) {
  const reduced = useReducedMotion();
  const shimmer = shimmerStyle(reduced);
  if (variant === 'chart') {
    return <SkeletonStatus><div className="skeleton-chart grid gap-3">{Array.from({ length: count }, (_, i) => <div key={CELL_KEYS[i]} className={`h-24 rounded-[var(--radius-lg)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)] p-4 ${reduced ? '' : 'anim-fadeIn anim-stagger'}`} style={reduced ? undefined : { animationDelay: `${i * 40}ms` } as never}><span className={`skeleton ${shimmer} h-3`} style={{ width: '40%' }} /><span className={`skeleton ${shimmer} mt-3 h-12`} style={{ width: '100%' }} /></div>)}</div></SkeletonStatus>;
  }
  if (variant === 'metric') {
    return <SkeletonStatus><div className="summary-grid">{Array.from({ length: count }, (_, i) => <article key={CELL_KEYS[i]} className={`summary-card ${reduced ? '' : 'anim-fadeIn anim-stagger'}`} style={reduced ? undefined : { animationDelay: `${i * 40}ms` } as never}><span className={`skeleton skeleton-value ${shimmer}`} style={{ width: '58%' }} /><span className={`skeleton skeleton-label ${shimmer}`} style={{ width: '72%' }} /><span className={`skeleton ${shimmer} mt-2 h-2`} style={{ width: '92%' }} /></article>)}</div></SkeletonStatus>;
  }
  return <SkeletonStatus>
    {variant === 'workspace'
      ? <div className="workspace-grid">{Array.from({ length: count }, (_, card) => <article key={CELL_KEYS[card]} className={`workspace-card skeleton-card ${reduced ? '' : 'anim-fadeIn anim-stagger'}`} style={reduced ? undefined : { animationDelay: `${card * 40}ms` } as never}>
          <div className="skeleton-stack"><span className={`skeleton ${shimmer}`} style={{ width: '62%' }} /><span className={`skeleton ${shimmer}`} style={{ width: '44%' }} /></div>
          <div className="skeleton-stack"><span className={`skeleton ${shimmer}`} style={{ width: '72%' }} /></div>
          <div className="skeleton-stack"><span className={`skeleton ${shimmer}`} style={{ width: '52%' }} /><span className={`skeleton ${shimmer}`} style={{ width: '34%' }} /></div>
        </article>)}</div>
      : <div className="summary-grid">{Array.from({ length: count }, (_, card) => <article key={CELL_KEYS[card]} className={`summary-card skeleton-card ${reduced ? '' : 'anim-fadeIn anim-stagger'}`} style={reduced ? undefined : { animationDelay: `${card * 40}ms` } as never}><span className={`skeleton skeleton-value ${shimmer}`} style={{ width: '58%' }} /><span className={`skeleton skeleton-label ${shimmer}`} style={{ width: '82%' }} /></article>)}</div>}
  </SkeletonStatus>;
}

export function SkeletonLines({ lines = 3 }: { lines?: number }) {
  const reduced = useReducedMotion();
  const shimmer = shimmerStyle(reduced);
  return <SkeletonStatus><div className="skeleton-lines">{Array.from({ length: lines }, (_, line) => <span key={LINE_KEYS[line]} className={`skeleton ${shimmer} ${reduced ? '' : 'anim-fadeIn anim-stagger'}`} style={{ width: line === lines - 1 ? '55%' : '100%', ...(reduced ? {} : { animationDelay: `${line * 40}ms` } as never) }} />)}</div></SkeletonStatus>;
}

// Explicit chart variant export for dashboard (alias)
export function SkeletonChart({ count = 1 }: { count?: number }) {
  return <SkeletonCards count={count} variant="chart" />;
}
export function SkeletonMetric({ count = 4 }: { count?: number }) {
  return <SkeletonCards count={count} variant="metric" />;
}

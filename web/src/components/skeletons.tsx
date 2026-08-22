import type { ReactNode } from 'react';

/* Layout-aware loading skeletons. Each skeleton mirrors the real markup of the
   view it stands in for (same table/card/line classes) so column widths and
   block positions match the loaded layout, and it carries the screen-reader
   equivalent of the old "Loading…" text. */

const barWidths = ['74%', '46%', '92%', '58%', '36%'];

/* Literal keys for the decorative placeholder cells. They exist only so no
   list here ever keys off an array index; every caller's row/column/line count
   stays well below the array sizes (largest today is 7 columns). */
const CELL_KEYS = ['c1', 'c2', 'c3', 'c4', 'c5', 'c6', 'c7', 'c8', 'c9', 'c10', 'c11', 'c12'];
const ROW_KEYS = ['r1', 'r2', 'r3', 'r4', 'r5', 'r6', 'r7', 'r8', 'r9', 'r10', 'r11', 'r12'];
const LINE_KEYS = ['l1', 'l2', 'l3', 'l4', 'l5', 'l6', 'l7', 'l8', 'l9', 'l10', 'l11', 'l12'];

function SkeletonStatus({ children }: { children: ReactNode }) {
  return <div className="skeleton-status" role="status" aria-busy="true"><span className="sr-only">Loading…</span>{children}</div>;
}

export function SkeletonTable({ rows = 5, cols = 5, className }: { rows?: number; cols?: number; className?: string }) {
  return <SkeletonStatus>
    <div className={`skeleton-table table-wrap${className ? ` ${className}` : ''}`}><table>
      <thead><tr>{Array.from({ length: cols }, (_, col) => <th key={CELL_KEYS[col]} scope="col"><span className="skeleton" style={{ width: barWidths[(col + 2) % barWidths.length] }} /></th>)}</tr></thead>
      <tbody>{Array.from({ length: rows }, (_, row) => <tr key={ROW_KEYS[row]}>{Array.from({ length: cols }, (_, col) => <td key={CELL_KEYS[col]}><span className="skeleton" style={{ width: barWidths[(col + row) % barWidths.length] }} /></td>)}</tr>)}</tbody>
    </table></div>
  </SkeletonStatus>;
}

export function SkeletonCards({ count = 6, variant = 'summary' }: { count?: number; variant?: 'summary' | 'workspace' }) {
  return <SkeletonStatus>
    {variant === 'workspace'
      ? <div className="workspace-grid">{Array.from({ length: count }, (_, card) => <article key={CELL_KEYS[card]} className="workspace-card skeleton-card">
          <div className="skeleton-stack"><span className="skeleton" style={{ width: '62%' }} /><span className="skeleton" style={{ width: '44%' }} /></div>
          <div className="skeleton-stack"><span className="skeleton" style={{ width: '72%' }} /></div>
          <div className="skeleton-stack"><span className="skeleton" style={{ width: '52%' }} /><span className="skeleton" style={{ width: '34%' }} /></div>
        </article>)}</div>
      : <div className="summary-grid">{Array.from({ length: count }, (_, card) => <article key={CELL_KEYS[card]} className="summary-card skeleton-card"><span className="skeleton skeleton-value" style={{ width: '58%' }} /><span className="skeleton skeleton-label" style={{ width: '82%' }} /></article>)}</div>}
  </SkeletonStatus>;
}

export function SkeletonLines({ lines = 3 }: { lines?: number }) {
  return <SkeletonStatus><div className="skeleton-lines">{Array.from({ length: lines }, (_, line) => <span key={LINE_KEYS[line]} className="skeleton" style={{ width: line === lines - 1 ? '55%' : '100%' }} />)}</div></SkeletonStatus>;
}

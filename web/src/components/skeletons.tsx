import type { ReactNode } from 'react';

/* Layout-aware loading skeletons. Each skeleton mirrors the real markup of the
   view it stands in for (same table/card/line classes) so column widths and
   block positions match the loaded layout, and it carries the screen-reader
   equivalent of the old "Loading…" text. */

const barWidths = ['74%', '46%', '92%', '58%', '36%'];

function SkeletonStatus({ children }: { children: ReactNode }) {
  return <div className="skeleton-status" role="status" aria-busy="true"><span className="sr-only">Loading…</span>{children}</div>;
}

export function SkeletonTable({ rows = 5, cols = 5, className }: { rows?: number; cols?: number; className?: string }) {
  return <SkeletonStatus>
    <div className={`skeleton-table table-wrap${className ? ` ${className}` : ''}`}><table>
      <thead><tr>{Array.from({ length: cols }, (_, col) => <th key={col} scope="col"><span className="skeleton" style={{ width: barWidths[(col + 2) % barWidths.length] }} /></th>)}</tr></thead>
      <tbody>{Array.from({ length: rows }, (_, row) => <tr key={row}>{Array.from({ length: cols }, (_, col) => <td key={col}><span className="skeleton" style={{ width: barWidths[(col + row) % barWidths.length] }} /></td>)}</tr>)}</tbody>
    </table></div>
  </SkeletonStatus>;
}

export function SkeletonCards({ count = 6, variant = 'summary' }: { count?: number; variant?: 'summary' | 'workspace' }) {
  const cards = Array.from({ length: count }, (_, index) => index);
  return <SkeletonStatus>
    {variant === 'workspace'
      ? <div className="workspace-grid">{cards.map((card) => <article key={card} className="workspace-card skeleton-card">
          <div className="skeleton-stack"><span className="skeleton" style={{ width: '62%' }} /><span className="skeleton" style={{ width: '44%' }} /></div>
          <div className="skeleton-stack"><span className="skeleton" style={{ width: '72%' }} /></div>
          <div className="skeleton-stack"><span className="skeleton" style={{ width: '52%' }} /><span className="skeleton" style={{ width: '34%' }} /></div>
        </article>)}</div>
      : <div className="summary-grid">{cards.map((card) => <article key={card} className="summary-card skeleton-card"><span className="skeleton skeleton-value" style={{ width: '58%' }} /><span className="skeleton skeleton-label" style={{ width: '82%' }} /></article>)}</div>}
  </SkeletonStatus>;
}

export function SkeletonLines({ lines = 3 }: { lines?: number }) {
  return <SkeletonStatus><div className="skeleton-lines">{Array.from({ length: lines }, (_, line) => <span key={line} className="skeleton" style={{ width: line === lines - 1 ? '55%' : '100%' }} />)}</div></SkeletonStatus>;
}

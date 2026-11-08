import type { ReactNode } from 'react';
import { cn } from '../lib/utils';

export type FilterBarProps = {
  filters: ReactNode;
  meta?: ReactNode;
  onClear?: () => void;
  clearLabel?: string;
  className?: string;
};

export function FilterBar({ filters, meta, onClear, clearLabel = 'Clear', className }: FilterBarProps) {
  return (
    <div className={cn('toolbar-row flex flex-wrap items-center gap-3 justify-between', className)}>
      <div className="toolbar-filters flex flex-wrap items-center gap-2 min-w-0">
        {filters}
        {onClear && (
          <button type="button" className="text-button ws-filter-clear" onClick={onClear}>
            {clearLabel}
          </button>
        )}
      </div>
      {meta != null && (
        <span className="toolbar-meta tabular-nums text-sm text-[var(--color-ink-soft)]" role="status">
          {meta}
        </span>
      )}
    </div>
  );
}

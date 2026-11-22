import type { ReactNode } from 'react';
import { Pagination, type PaginationProps } from './Pagination';
import { cn } from '../lib/utils';

export type ListFooterProps = {
  rangeText?: ReactNode;
  'aria-label'?: string;
  pagination?: PaginationProps | null;
  actions?: ReactNode;
  className?: string;
  liveRange?: boolean;
  /** `panel` matches Overview boards; `toolbar` matches list pages. */
  variant?: 'panel' | 'toolbar';
  rangeClassName?: string;
};

export function ListFooter({
  rangeText,
  pagination,
  actions,
  className,
  liveRange = false,
  variant = 'panel',
  rangeClassName,
}: ListFooterProps) {
  return (
    <footer
      className={cn(
        'flex flex-wrap items-center gap-2',
        variant === 'panel' && 'board-panel-foot',
        variant === 'toolbar' && 'toolbar-row justify-between mt-4',
        className,
      )}
    >
      {rangeText != null && rangeText !== false && (
        <span
          className={cn(
            'tabular-nums',
            rangeClassName ?? (variant === 'panel' ? 'board-panel-foot-note' : undefined),
          )}
          {...(liveRange ? { 'aria-live': 'polite' as const } : {})}
        >
          {rangeText}
        </span>
      )}
      {pagination && (
        <Pagination {...pagination} />
      )}
      {actions}
    </footer>
  );
}

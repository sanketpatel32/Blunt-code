import type { ReactNode } from 'react';
import { cn } from '../lib/utils';

export type DataTableShellProps = {
  children: ReactNode;
  className?: string;
  stickyFirstColumn?: boolean;
  minWidth?: string;
};

export function DataTableShell({
  children,
  className,
  stickyFirstColumn = false,
  minWidth,
}: DataTableShellProps) {
  return (
    <div
      className={cn(
        'table-wrap relative',
        stickyFirstColumn && 'data-table-sticky-first',
        className,
      )}
      data-sticky-first={stickyFirstColumn ? '' : undefined}
    >
      {children}
    </div>
  );
}

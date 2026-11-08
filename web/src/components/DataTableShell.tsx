import * as React from 'react';
import type { ReactNode } from 'react';
import { cn } from '../lib/utils';

export type DataTableShellProps = {
  children: ReactNode;
  className?: string;
  stickyFirstColumn?: boolean;
  minWidth?: string;
};

export const DataTableShell = React.forwardRef<HTMLDivElement, DataTableShellProps>(function DataTableShell(
  { children, className, stickyFirstColumn = false },
  ref,
) {
  return (
    <div
      ref={ref}
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
});

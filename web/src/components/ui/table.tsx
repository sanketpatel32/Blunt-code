import * as React from 'react';
import { cn } from '../../lib/utils';

const Table = React.forwardRef<HTMLTableElement, React.HTMLAttributes<HTMLTableElement>>(({ className, ...props }, ref) => (
  <div
    className={cn(
      'relative w-full overflow-auto overscroll-x-contain rounded-[var(--radius-lg)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)] shadow-[var(--shadow-card)]',
      '[--table-pad:14px]',
      className,
    )}
  >
    <table
      ref={ref}
      className={cn('w-full caption-bottom border-collapse text-sm tabular-nums', className?.includes('w-full') ? undefined : undefined)}
      {...props}
    />
  </div>
));
Table.displayName = 'Table';

const TableHeader = React.forwardRef<
  HTMLTableSectionElement,
  React.HTMLAttributes<HTMLTableSectionElement> & { sticky?: boolean }
>(({ className, sticky, ...props }, ref) => (
  <thead
    ref={ref}
    className={cn(
      '[&_tr]:border-b bg-[var(--color-surface-muted)]',
      sticky && 'sticky top-0 z-[1] shadow-[var(--shadow-xs)]',
      className,
    )}
    {...props}
  />
));
TableHeader.displayName = 'TableHeader';

const TableBody = React.forwardRef<HTMLTableSectionElement, React.HTMLAttributes<HTMLTableSectionElement>>(({ className, ...props }, ref) => (
  <tbody ref={ref} className={cn('[&_tr:last-child]:border-0', className)} {...props} />
));
TableBody.displayName = 'TableBody';

const TableFooter = React.forwardRef<HTMLTableSectionElement, React.HTMLAttributes<HTMLTableSectionElement>>(({ className, ...props }, ref) => (
  <tfoot ref={ref} className={cn('border-t bg-[var(--color-surface-muted)] font-medium [&>tr]:last:border-b-0', className)} {...props} />
));
TableFooter.displayName = 'TableFooter';

const TableRow = React.forwardRef<HTMLTableRowElement, React.HTMLAttributes<HTMLTableRowElement>>(({ className, ...props }, ref) => (
  <tr
    ref={ref}
    className={cn(
      'group border-b border-[var(--color-rule-faint)] tabular-nums transition-colors duration-[var(--dur-fast)] ease-out',
      'hover:bg-[color-mix(in_oklch,var(--color-accent)_5%,transparent)] data-[state=selected]:bg-[var(--color-surface-muted)]',
      'motion-reduce:transition-none',
      // content-visibility for virtualization opt-in via data attribute
      'data-[cv=auto]:[content-visibility:auto] data-[cv=auto]:[contain-intrinsic-size:auto_3rem]',
      className,
    )}
    {...props}
  />
));
TableRow.displayName = 'TableRow';

const TableHead = React.forwardRef<HTMLTableCellElement, React.ThHTMLAttributes<HTMLTableCellElement>>(({ className, ...props }, ref) => (
  <th
    ref={ref}
    className={cn(
      'h-10 px-[var(--table-pad)] text-left align-middle font-mono text-[0.68rem] font-bold tracking-widest uppercase text-[var(--color-ink-faint)] tabular-nums [&:has([role=checkbox])]:pr-0',
      className,
    )}
    {...props}
  />
));
TableHead.displayName = 'TableHead';

const TableCell = React.forwardRef<HTMLTableCellElement, React.TdHTMLAttributes<HTMLTableCellElement>>(({ className, ...props }, ref) => (
  <td
    ref={ref}
    className={cn(
      'p-[var(--table-pad)] align-middle tabular-nums [&:has([role=checkbox])]:pr-0',
      // group-hover actions reveal
      'group-hover:[&_.row-actions]:opacity-100 [&_.row-actions]:opacity-0 [&_.row-actions]:transition-opacity [&_.row-actions:focus-within]:opacity-100',
      className,
    )}
    {...props}
  />
));
TableCell.displayName = 'TableCell';

const TableCaption = React.forwardRef<HTMLTableCaptionElement, React.HTMLAttributes<HTMLTableCaptionElement>>(({ className, ...props }, ref) => (
  <caption ref={ref} className={cn('sr-only', className)} {...props} />
));
TableCaption.displayName = 'TableCaption';

export { Table, TableHeader, TableBody, TableFooter, TableHead, TableRow, TableCell, TableCaption };

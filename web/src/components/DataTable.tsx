import * as React from 'react';
import { Search, Settings2, ChevronLeft, ChevronRight, SlidersHorizontal, Check } from 'lucide-react';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow, TableCaption } from './ui/table';
import { cn } from '../lib/utils';

export type Density = 'comfortable' | 'compact';
const DENSITY_KEY = 'bluntcode.tableDensity';

export function useDensity(defaultDensity: Density = 'comfortable') {
  const [density, setDensity] = React.useState<Density>(() => {
    try {
      const v = window.localStorage.getItem(DENSITY_KEY) as Density | null;
      return v === 'compact' || v === 'comfortable' ? v : defaultDensity;
    } catch {
      return defaultDensity;
    }
  });
  const toggle = React.useCallback(() => {
    setDensity((cur) => {
      const next: Density = cur === 'compact' ? 'comfortable' : 'compact';
      try { window.localStorage.setItem(DENSITY_KEY, next); } catch {}
      return next;
    });
  }, []);
  const set = React.useCallback((d: Density) => {
    setDensity(d);
    try { window.localStorage.setItem(DENSITY_KEY, d); } catch {}
  }, []);
  return { density, setDensity: set, toggleDensity: toggle, isCompact: density === 'compact' };
}

export interface DataTableColumn<T> {
  id: string;
  header: string;
  accessor: (row: T) => React.ReactNode;
  sortable?: boolean;
  sortKey?: string;
  hidden?: boolean;
  className?: string;
}

export interface DataTableProps<T> {
  data: T[];
  columns: DataTableColumn<T>[];
  getRowId?: (row: T, index: number) => string;
  caption?: string;
  stickyHeader?: boolean;
  sortable?: boolean;
  sortKey?: string;
  sortDir?: 'asc' | 'desc';
  onSort?: (key: string) => void;
  // selection
  selectable?: boolean;
  selectedIds?: Set<string>;
  onSelectionChange?: (ids: Set<string>) => void;
  // pagination
  page?: number;
  pageSize?: number;
  total?: number;
  hasNext?: boolean;
  onPageChange?: (page: number) => void;
  // visibility
  columnVisibility?: Record<string, boolean>;
  onColumnVisibilityChange?: (id: string, visible: boolean) => void;
  // density
  density?: Density;
  onDensityToggle?: () => void;
  // states
  loading?: boolean;
  emptyTitle?: string;
  emptyDescription?: string;
  emptyIcon?: React.ReactNode;
  skeletonRows?: number;
  // faceted filter bar slot
  toolbar?: React.ReactNode;
  // row extra class
  getRowClassName?: (row: T) => string;
  // row stagger
  stagger?: boolean;
  // bulk actions slot
  bulkActions?: React.ReactNode;
}

function IndeterminateCheckbox({ checked, indeterminate, ...props }: React.InputHTMLAttributes<HTMLInputElement> & { indeterminate?: boolean }) {
  const ref = React.useRef<HTMLInputElement>(null);
  React.useEffect(() => { if (ref.current) ref.current.indeterminate = !!indeterminate; }, [indeterminate]);
  return <input ref={ref} type="checkbox" role="checkbox" checked={checked} {...props} />;
}

export function DataTable<T>({
  data, columns, getRowId, caption, stickyHeader,
  sortKey, sortDir, onSort,
  selectable, selectedIds, onSelectionChange,
  page, pageSize, total, hasNext, onPageChange,
  columnVisibility, onColumnVisibilityChange,
  density, onDensityToggle,
  loading, emptyTitle, emptyDescription, emptyIcon, skeletonRows = 5,
  toolbar,
  getRowClassName,
  stagger,
  bulkActions,
}: DataTableProps<T>) {
  const visibleColumns = columns.filter((c) => columnVisibility ? columnVisibility[c.id] !== false : !c.hidden);
  const allIds = data.map((r, i) => getRowId ? getRowId(r, i) : String(i));
  const allSelected = selectable && data.length > 0 && allIds.every((id) => selectedIds?.has(id));
  const someSelected = selectable && !!selectedIds && allIds.some((id) => selectedIds.has(id)) && !allSelected;

  const handleSelectAll = (checked: boolean) => {
    if (!onSelectionChange) return;
    if (checked) onSelectionChange(new Set([...(selectedIds ?? new Set()), ...allIds]));
    else {
      const next = new Set(selectedIds);
      allIds.forEach((id) => next.delete(id));
      onSelectionChange(next);
    }
  };

  const toggleColumn = (id: string) => onColumnVisibilityChange?.(id, !(columnVisibility?.[id] !== false));

  const pageCount = page && pageSize && total != null ? Math.max(1, Math.ceil(total / pageSize)) : undefined;

  // density css var
  const densityPad = density === 'compact' ? '10px' : '14px';

  return (
    <div className={cn('space-y-3', stagger && 'page-enter')} style={{ ['--table-pad' as string]: densityPad } as React.CSSProperties}>
      {(toolbar || onDensityToggle || onColumnVisibilityChange) && (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex-1 min-w-[12rem]">{toolbar}</div>
          <div className="flex items-center gap-2">
            {bulkActions}
            {onColumnVisibilityChange && (
              <ColumnVisibilityToggle columns={columns} visibility={columnVisibility} onToggle={toggleColumn} />
            )}
            {onDensityToggle && (
              <button
                type="button"
                className="inline-flex h-8 items-center gap-1.5 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold text-[var(--color-ink)] shadow-[var(--shadow-card)]"
                aria-pressed={density === 'compact'}
                onClick={onDensityToggle}
                title={density === 'compact' ? 'Switch to comfortable' : 'Switch to compact'}
              >
                <SlidersHorizontal className="h-3.5 w-3.5" aria-hidden="true" />
                {density === 'compact' ? 'Comfortable' : 'Compact'}
              </button>
            )}
          </div>
        </div>
      )}

      <Table>
        {caption && <TableCaption>{caption}</TableCaption>}
        <TableHeader sticky={stickyHeader}>
          <TableRow>
            {selectable && (
              <TableHead className="w-8">
                <IndeterminateCheckbox
                  checked={!!allSelected}
                  indeterminate={!!someSelected}
                  aria-label="Select all"
                  onChange={(e) => handleSelectAll(e.target.checked)}
                />
              </TableHead>
            )}
            {visibleColumns.map((col) => (
              <TableHead key={col.id} className={cn(col.className)} aria-sort={col.sortable && sortKey === (col.sortKey ?? col.id) ? (sortDir === 'asc' ? 'ascending' : 'descending') : undefined}>
                {col.sortable && onSort ? (
                  <button
                    type="button"
                    className={cn('th-sort', sortKey === (col.sortKey ?? col.id) && 'active')}
                    onClick={() => onSort(col.sortKey ?? col.id)}
                    aria-label={`Sort by ${col.header}`}
                  >
                    {col.header}
                    <span className="sort-arrow" aria-hidden="true">
                      {sortKey === (col.sortKey ?? col.id) ? (sortDir === 'asc' ? '▲' : '▼') : '↕'}
                    </span>
                  </button>
                ) : col.header}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading ? (
            Array.from({ length: skeletonRows }).map((_, r) => (
              <TableRow key={`sk-${r}`} className="animate-pulse">
                {selectable && <TableCell><span className="skeleton block h-3 w-3 rounded" /></TableCell>}
                {visibleColumns.map((c) => (
                  <TableCell key={c.id}><span className="skeleton block h-3" style={{ width: `${40 + (r * 13 + c.id.length * 7) % 50}%` }} /></TableCell>
                ))}
              </TableRow>
            ))
          ) : data.length === 0 ? (
            <TableRow>
              <TableCell colSpan={visibleColumns.length + (selectable ? 1 : 0)} className="py-12 text-center">
                <div className="grid place-items-center gap-2 py-4 text-[var(--color-ink-soft)]">
                  {emptyIcon && <span className="text-[var(--color-ink-faint)]">{emptyIcon}</span>}
                  <p className="font-semibold text-[var(--color-ink)]">{emptyTitle ?? 'No results'}</p>
                  {emptyDescription && <p className="max-w-[28rem] text-sm">{emptyDescription}</p>}
                </div>
              </TableCell>
            </TableRow>
          ) : (
            data.map((row, idx) => {
              const id = getRowId ? getRowId(row, idx) : String(idx);
              const isSelected = !!selectedIds?.has(id);
              const rowClass = getRowClassName?.(row);
              return (
                <TableRow
                  key={id}
                  data-state={isSelected ? 'selected' : undefined}
                  data-cv="auto"
                  className={cn(rowClass, stagger && 'table-row-stagger')}
                  style={stagger ? { ['--stagger-index' as string]: String(idx) } as React.CSSProperties : undefined}
                >
                  {selectable && (
                    <TableCell>
                      <input
                        type="checkbox"
                        role="checkbox"
                        checked={isSelected}
                        aria-label={`Select row ${id}`}
                        onChange={(e) => {
                          if (!onSelectionChange) return;
                          const next = new Set(selectedIds);
                          if (e.target.checked) next.add(id); else next.delete(id);
                          onSelectionChange(next);
                        }}
                      />
                    </TableCell>
                  )}
                  {visibleColumns.map((col) => (
                    <TableCell key={col.id} className={cn(col.className)}>
                      {col.accessor(row)}
                    </TableCell>
                  ))}
                </TableRow>
              );
            })
          )}
        </TableBody>
      </Table>

      {(page != null && pageCount != null) && (
        <nav className="flex flex-wrap items-center justify-between gap-3 py-2 text-sm text-[var(--color-ink-soft)]" aria-label="Pagination">
          <span className="tabular-nums">
            {total != null
              ? `Showing ${(page - 1) * (pageSize ?? data.length) + 1}–${Math.min(page * (pageSize ?? data.length), total)} of ${total}`
              : `${data.length} rows`}
          </span>
          <div className="flex items-center gap-2">
            <button
              type="button"
              className="inline-flex h-8 items-center gap-1 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-sm font-medium shadow-[var(--shadow-card)] disabled:opacity-50"
              onClick={() => onPageChange?.(page - 1)}
              disabled={page <= 1}
            >
              <ChevronLeft className="h-4 w-4" /> Previous
            </button>
            <output aria-live="polite" className="min-w-[7rem] text-center font-mono text-xs font-semibold text-[var(--color-ink-faint)] tabular-nums">
              Page {page} of {pageCount}
            </output>
            <button
              type="button"
              className="inline-flex h-8 items-center gap-1 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-sm font-medium shadow-[var(--shadow-card)] disabled:opacity-50"
              onClick={() => onPageChange?.(page + 1)}
              disabled={hasNext === false || (total != null && page >= pageCount)}
            >
              Next <ChevronRight className="h-4 w-4" />
            </button>
          </div>
        </nav>
      )}
    </div>
  );
}

function ColumnVisibilityToggle({
  columns, visibility, onToggle,
}: {
  columns: { id: string; header: string }[];
  visibility?: Record<string, boolean>;
  onToggle: (id: string) => void;
}) {
  const [open, setOpen] = React.useState(false);
  const ref = React.useRef<HTMLDivElement>(null);
  React.useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) setOpen(false); };
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false); };
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    return () => { document.removeEventListener('mousedown', onDown); document.removeEventListener('keydown', onKey); };
  }, [open]);
  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        className="inline-flex h-8 items-center gap-1.5 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold shadow-[var(--shadow-card)]"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        <Settings2 className="h-3.5 w-3.5" /> Columns
      </button>
      {open && (
        <div role="menu" className="absolute right-0 z-20 mt-1 grid min-w-[12rem] gap-1 rounded-[var(--radius-lg)] border border-[var(--color-rule)] bg-[var(--color-surface)] p-2 shadow-[var(--shadow-lg)]">
          {columns.map((c) => {
            const vis = visibility ? visibility[c.id] !== false : true;
            return (
              <button
                key={c.id}
                role="menuitemcheckbox"
                aria-checked={vis}
                type="button"
                className="flex items-center gap-2 rounded-[var(--radius-button)] px-2 py-1.5 text-left text-sm hover:bg-[var(--color-surface-muted)]"
                onClick={() => onToggle(c.id)}
              >
                <span className={cn('grid h-4 w-4 place-items-center rounded-[var(--radius-xs)] border', vis ? 'bg-[var(--color-accent)] border-[var(--color-accent)] text-white' : 'border-[var(--color-rule)] bg-[var(--color-surface)]')}>
                  {vis && <Check className="h-3 w-3" />}
                </span>
                {c.header}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

// Simple Search input helper for toolbar
export function DataTableSearch({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder?: string }) {
  return (
    <div className="relative max-w-[22rem] flex-1">
      <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--color-ink-faint)]" aria-hidden="true" />
      <input
        type="search"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder ?? 'Search…'}
        aria-label="Search"
        className="h-8 w-full rounded-[var(--radius-button)] border border-[var(--color-rule-strong)] bg-[var(--color-surface)] py-1 pl-8 pr-3 text-sm focus:border-[var(--color-accent)] focus:outline-none focus:ring-2 focus:ring-[var(--color-accent-glow)]"
      />
    </div>
  );
}

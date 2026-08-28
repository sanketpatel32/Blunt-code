import * as React from 'react';
import { Search, Settings2, ChevronLeft, ChevronRight, SlidersHorizontal, Check, Copy, Download, EyeOff } from 'lucide-react';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow, TableCaption } from './ui/table';
import { cn } from '../lib/utils';

export type Density = 'comfortable' | 'compact';
const DENSITY_KEY = 'bluntcode.tableDensity';
const COL_WIDTHS_KEY = 'bluntcode.colWidths';

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

function useColWidths(storageKey: string = COL_WIDTHS_KEY) {
  const [widths, setWidths] = React.useState<Record<string, number>>(() => {
    try {
      const raw = window.localStorage.getItem(storageKey);
      return raw ? (JSON.parse(raw) as Record<string, number>) : {};
    } catch { return {}; }
  });
  const setWidth = React.useCallback((id: string, w: number) => {
    setWidths((prev) => {
      const next = { ...prev, [id]: Math.max(80, Math.min(600, w)) };
      try { window.localStorage.setItem(storageKey, JSON.stringify(next)); } catch {}
      return next;
    });
  }, [storageKey]);
  return { widths, setWidth };
}

export interface DataTableColumn<T> {
  id: string;
  header: string;
  accessor: (row: T) => React.ReactNode;
  sortable?: boolean;
  sortKey?: string;
  hidden?: boolean;
  className?: string;
  width?: number;
  pinned?: boolean;
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
  selectable?: boolean;
  selectedIds?: Set<string>;
  onSelectionChange?: (ids: Set<string>) => void;
  page?: number;
  pageSize?: number;
  total?: number;
  hasNext?: boolean;
  onPageChange?: (page: number) => void;
  columnVisibility?: Record<string, boolean>;
  onColumnVisibilityChange?: (id: string, visible: boolean) => void;
  density?: Density;
  onDensityToggle?: () => void;
  loading?: boolean;
  emptyTitle?: string;
  emptyDescription?: string;
  emptyIcon?: React.ReactNode;
  skeletonRows?: number;
  toolbar?: React.ReactNode;
  getRowClassName?: (row: T) => string;
  stagger?: boolean;
  bulkActions?: React.ReactNode;
  // advanced
  pinFirstColumn?: boolean;
  enableColumnResizing?: boolean;
  colWidthsKey?: string;
  rowHeightVar?: string;
  expandable?: boolean;
  renderExpandedContent?: (row: T, index: number) => React.ReactNode;
  expandedIds?: Set<string>;
  onExpandedChange?: (ids: Set<string>) => void;
  onBulkSuppress?: () => void;
  onBulkExportCsv?: () => void;
  onBulkCopy?: () => void;
  rowHeight?: number;
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
  pinFirstColumn = true,
  enableColumnResizing = true,
  colWidthsKey = COL_WIDTHS_KEY,
  expandable = false,
  renderExpandedContent,
  expandedIds: controlledExpanded,
  onExpandedChange,
  onBulkSuppress,
  onBulkExportCsv,
  onBulkCopy,
  rowHeight,
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

  const densityPad = density === 'compact' ? '10px' : '14px';
  const rowH = rowHeight ?? (density === 'compact' ? 40 : 52);
  const { widths: colWidths, setWidth } = useColWidths(colWidthsKey);

  const resizingRef = React.useRef<{ id: string; startX: number; startW: number } | null>(null);
  const onResizeStart = (e: React.PointerEvent, id: string, currentW: number) => {
    (e.target as HTMLElement).setPointerCapture?.(e.pointerId);
    resizingRef.current = { id, startX: e.clientX, startW: currentW };
    const onMove = (ev: PointerEvent) => {
      if (!resizingRef.current) return;
      const delta = ev.clientX - resizingRef.current.startX;
      setWidth(resizingRef.current.id, resizingRef.current.startW + delta);
    };
    const onUp = () => {
      resizingRef.current = null;
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
    };
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
  };

  // expansion (uncontrolled fallback)
  const [internalExpanded, setInternalExpanded] = React.useState<Set<string>>(new Set());
  const expanded = controlledExpanded ?? internalExpanded;
  const toggleExpand = (id: string) => {
    const next = new Set(expanded);
    if (next.has(id)) next.delete(id); else next.add(id);
    if (onExpandedChange) onExpandedChange(next); else setInternalExpanded(next);
  };

  const selectedCount = selectedIds?.size ?? 0;
  const showBulkBar = selectable && selectedCount > 0;

  const handleCopy = async () => {
    if (onBulkCopy) { onBulkCopy(); return; }
    try {
      const ids = [...(selectedIds ?? [])].join(', ');
      await navigator.clipboard.writeText(ids);
    } catch {}
  };

  return (
    <div className={cn('space-y-3', stagger && 'page-enter')} style={{ ['--table-pad' as string]: densityPad, ['--row-height' as string]: `${rowH}px` } as React.CSSProperties}>
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
                className="inline-flex h-8 items-center gap-1.5 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold text-[var(--color-ink)] shadow-[var(--shadow-card)] motion-reduce:transition-none"
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

      {showBulkBar && (
        <div role="toolbar" aria-label="Bulk actions" className="flex flex-wrap items-center gap-2 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface-muted)] px-3 py-2 shadow-[var(--shadow-card)]">
          <span className="inline-flex items-center rounded-[var(--radius-button)] bg-[var(--color-accent)] px-2 py-0.5 text-xs font-bold text-white shadow-[var(--shadow-card)]" aria-live="polite">{selectedCount} selected</span>
          <button type="button" onClick={() => onBulkSuppress?.()} className="inline-flex h-7 items-center gap-1.5 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold shadow-[var(--shadow-card)]">
            <EyeOff className="h-3.5 w-3.5" aria-hidden="true" /> Suppress selected
          </button>
          <button type="button" onClick={() => onBulkExportCsv?.()} className="inline-flex h-7 items-center gap-1.5 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold shadow-[var(--shadow-card)]">
            <Download className="h-3.5 w-3.5" aria-hidden="true" /> Export selected CSV
          </button>
          <button type="button" onClick={handleCopy} className="inline-flex h-7 items-center gap-1.5 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold shadow-[var(--shadow-card)]">
            <Copy className="h-3.5 w-3.5" aria-hidden="true" /> Copy
          </button>
        </div>
      )}

      <Table>
        {caption && <TableCaption>{caption}</TableCaption>}
        <TableHeader sticky={stickyHeader}>
          <TableRow>
            {selectable && (
              <TableHead className="w-8 sticky left-0 z-[2] bg-[var(--color-surface-muted)] shadow-[var(--shadow-card)]">
                <IndeterminateCheckbox
                  checked={!!allSelected}
                  indeterminate={!!someSelected}
                  aria-label="Select all"
                  onChange={(e) => handleSelectAll(e.target.checked)}
                />
              </TableHead>
            )}
            {visibleColumns.map((col, idx) => {
              const isPinned = pinFirstColumn && idx === 0;
              const w = colWidths[col.id] ?? col.width;
              return (
                <TableHead
                  key={col.id}
                  className={cn(col.className, isPinned && 'sticky left-0 z-[2] bg-[var(--color-surface-muted)] shadow-[var(--shadow-card)]', 'relative')}
                  style={w ? { width: w, minWidth: w } : undefined}
                  aria-sort={col.sortable && sortKey === (col.sortKey ?? col.id) ? (sortDir === 'asc' ? 'ascending' : 'descending') : undefined}
                >
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
                  {enableColumnResizing && (
                    <span
                      role="separator"
                      aria-orientation="vertical"
                      aria-label={`Resize ${col.header} column`}
                      onPointerDown={(e) => onResizeStart(e, col.id, (w as number) ?? 160)}
                      className="absolute right-0 top-0 h-full w-2 cursor-col-resize touch-none select-none hover:bg-[var(--color-accent-ghost)] focus-visible:bg-[var(--color-accent-ghost)] focus-visible:outline-none"
                      tabIndex={0}
                      onKeyDown={(e) => {
                        if (e.key === 'ArrowLeft') setWidth(col.id, ((w as number) ?? 160) - 16);
                        if (e.key === 'ArrowRight') setWidth(col.id, ((w as number) ?? 160) + 16);
                      }}
                    />
                  )}
                </TableHead>
              );
            })}
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading ? (
            Array.from({ length: skeletonRows }).map((_, r) => (
              <TableRow key={`sk-${r}`} className="animate-pulse motion-reduce:animate-none">
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
              const isExpanded = expanded.has(id);
              return (
                <React.Fragment key={id}>
                  <TableRow
                    data-state={isSelected ? 'selected' : undefined}
                    data-expanded={isExpanded ? 'true' : undefined}
                    data-cv="auto"
                    className={cn(rowClass, stagger && 'table-row-stagger', expandable && 'cursor-pointer', 'h-[var(--row-height)]')}
                    style={stagger ? { ['--stagger-index' as string]: String(idx) } as React.CSSProperties : undefined}
                    onClick={expandable ? () => toggleExpand(id) : undefined}
                    onKeyDown={expandable ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggleExpand(id); } } : undefined}
                    tabIndex={expandable ? 0 : undefined}
                    aria-expanded={expandable ? isExpanded : undefined}
                  >
                    {selectable && (
                      <TableCell className="sticky left-0 z-[1] bg-[var(--color-surface)] shadow-[var(--shadow-card)] group-data-[state=selected]:bg-[var(--color-surface-muted)]" onClick={(e) => e.stopPropagation()}>
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
                          onClick={(e) => e.stopPropagation()}
                        />
                      </TableCell>
                    )}
                    {visibleColumns.map((col, cIdx) => {
                      const isPinned = pinFirstColumn && cIdx === 0;
                      const w = colWidths[col.id] ?? col.width;
                      return (
                        <TableCell key={col.id} className={cn(col.className, isPinned && 'sticky left-0 z-[1] bg-[var(--color-surface)] shadow-[var(--shadow-card)] group-data-[state=selected]:bg-[var(--color-surface-muted)]')} style={w ? { width: w, minWidth: w } : undefined}>
                          {col.accessor(row)}
                        </TableCell>
                      );
                    })}
                  </TableRow>
                  {expandable && isExpanded && renderExpandedContent && (
                    <TableRow className="bg-[var(--color-surface-muted)]">
                      <TableCell colSpan={visibleColumns.length + (selectable ? 1 : 0)} className="p-0">
                        <div className="px-[var(--table-pad)] py-3 text-sm motion-reduce:transition-none animate-[page-enter_var(--dur-slow)_var(--ease-out)] motion-reduce:animate-none">
                          {renderExpandedContent(row, idx)}
                        </div>
                      </TableCell>
                    </TableRow>
                  )}
                </React.Fragment>
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
              className="inline-flex h-8 items-center gap-1 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-sm font-medium shadow-[var(--shadow-card)] disabled:opacity-50 motion-reduce:transition-none"
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
              className="inline-flex h-8 items-center gap-1 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-sm font-medium shadow-[var(--shadow-card)] disabled:opacity-50 motion-reduce:transition-none"
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

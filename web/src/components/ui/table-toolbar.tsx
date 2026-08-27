import * as React from 'react';
import { Search, X, Download, SlidersHorizontal } from 'lucide-react';
import { cn } from '../../lib/utils';

export interface ToolbarFacet {
  key: string;
  label: string;
  value: string;
}

export interface TableToolbarProps {
  searchValue?: string;
  onSearchChange?: (v: string) => void;
  searchPlaceholder?: string;
  severityValue?: string;
  onSeverityChange?: (v: string) => void;
  analyzerValue?: string;
  onAnalyzerChange?: (v: string) => void;
  analyzerOptions?: string[];
  facets?: ToolbarFacet[];
  onRemoveFacet?: (key: string) => void;
  onClear?: () => void;
  density?: 'comfortable' | 'compact';
  onDensityToggle?: () => void;
  onExportCsv?: () => void;
  exportHref?: string;
  className?: string;
}

export function TableToolbar({
  searchValue = '',
  onSearchChange,
  searchPlaceholder = 'Search…',
  severityValue = '',
  onSeverityChange,
  analyzerValue = '',
  onAnalyzerChange,
  analyzerOptions = [],
  facets = [],
  onRemoveFacet,
  onClear,
  density,
  onDensityToggle,
  onExportCsv,
  exportHref,
  className,
}: TableToolbarProps) {
  return (
    <div className={cn('flex flex-wrap items-center gap-2 rounded-[var(--radius-lg)] border border-[var(--color-rule-faint)] bg-[var(--color-surface)] p-2 shadow-[var(--shadow-card)]', className)}>
      {onSearchChange && (
        <div className="relative min-w-[14rem] flex-1 max-w-[22rem]">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--color-ink-faint)]" aria-hidden="true" />
          <input
            type="search"
            value={searchValue}
            onChange={(e) => onSearchChange(e.target.value)}
            placeholder={searchPlaceholder}
            aria-label="Search"
            className="h-8 w-full rounded-[var(--radius-button)] border border-[var(--color-rule-strong)] bg-[var(--color-surface)] pl-8 pr-3 text-sm focus:border-[var(--color-accent)] focus:outline-none"
          />
        </div>
      )}

      {onSeverityChange && (
        <select
          value={severityValue}
          onChange={(e) => onSeverityChange(e.target.value)}
          aria-label="Filter by severity"
          className="h-8 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-2 text-sm"
        >
          <option value="">All severities</option>
          {['critical', 'high', 'medium', 'low', 'info'].map((s) => <option key={s} value={s}>{s}</option>)}
        </select>
      )}

      {onAnalyzerChange && (
        <select
          value={analyzerValue}
          onChange={(e) => onAnalyzerChange(e.target.value)}
          aria-label="Filter by analyzer"
          className="h-8 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-2 text-sm"
        >
          <option value="">All tools</option>
          {analyzerOptions.map((a) => <option key={a} value={a}>{a}</option>)}
        </select>
      )}

      {facets.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          {facets.map((f) => (
            <span key={f.key + f.value} className="inline-flex items-center gap-1 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface-muted)] px-2 py-1 text-xs font-semibold">
              <span className="font-mono text-[var(--color-ink-faint)]">{f.label}:</span> {f.value}
              {onRemoveFacet && (
                <button
                  type="button"
                  aria-label={`Remove ${f.label} filter`}
                  onClick={() => onRemoveFacet(f.key)}
                  className="grid h-4 w-4 place-items-center rounded-full bg-[var(--color-surface)] text-[var(--color-ink-faint)] hover:bg-[var(--color-ink)] hover:text-white"
                >
                  <X className="h-3 w-3" />
                </button>
              )}
            </span>
          ))}
        </div>
      )}

      <div className="ml-auto flex items-center gap-1.5">
        {onClear && facets.length > 0 && (
          <button type="button" onClick={onClear} className="h-8 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold shadow-[var(--shadow-card)]">
            Clear
          </button>
        )}
        {onDensityToggle && density && (
          <button type="button" aria-pressed={density === 'compact'} onClick={onDensityToggle} className="inline-flex h-8 items-center gap-1 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-2 text-xs font-semibold shadow-[var(--shadow-card)]">
            <SlidersHorizontal className="h-3.5 w-3.5" />{density === 'compact' ? 'Comfortable' : 'Compact'}
          </button>
        )}
        {(onExportCsv || exportHref) && (
          exportHref ? (
            <a href={exportHref} download className="inline-flex h-8 items-center gap-1 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold shadow-[var(--shadow-card)]">
              <Download className="h-3.5 w-3.5" /> CSV
            </a>
          ) : (
            <button type="button" onClick={onExportCsv} className="inline-flex h-8 items-center gap-1 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold shadow-[var(--shadow-card)]">
              <Download className="h-3.5 w-3.5" /> CSV
            </button>
          )
        )}
      </div>
    </div>
  );
}

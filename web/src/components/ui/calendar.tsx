import * as React from 'react';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { cn } from '../../lib/utils';

export type CalendarProps = {
  className?: string;
  selected?: Date;
  onSelect?: (date: Date | undefined) => void;
  disabled?: (date: Date) => boolean;
  month?: Date;
  onMonthChange?: (date: Date) => void;
};

function startOfMonth(d: Date) {
  return new Date(d.getFullYear(), d.getMonth(), 1);
}
function addMonths(d: Date, n: number) {
  return new Date(d.getFullYear(), d.getMonth() + n, 1);
}
function isSameDay(a: Date, b: Date) {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

export function Calendar({ className, selected, onSelect, disabled, month: controlledMonth, onMonthChange }: CalendarProps) {
  const [internalMonth, setInternalMonth] = React.useState(() => startOfMonth(selected ?? new Date()));
  const month = controlledMonth ? startOfMonth(controlledMonth) : internalMonth;
  const setMonth = (d: Date) => {
    if (onMonthChange) onMonthChange(d);
    else setInternalMonth(d);
  };

  const year = month.getFullYear();
  const m = month.getMonth();
  const firstDay = new Date(year, m, 1).getDay();
  const daysInMonth = new Date(year, m + 1, 0).getDate();
  const today = new Date();

  const cells: (Date | null)[] = [];
  for (let i = 0; i < firstDay; i++) cells.push(null);
  for (let d = 1; d <= daysInMonth; d++) cells.push(new Date(year, m, d));
  while (cells.length % 7 !== 0) cells.push(null);

  const monthLabel = month.toLocaleDateString('en-US', { month: 'long', year: 'numeric' });

  return (
    <div className={cn('rounded-[var(--radius-lg)] border border-[var(--color-rule)] bg-[var(--color-surface)] p-4 shadow-[var(--shadow-card)] w-[18rem]', className)}>
      <div className="flex items-center justify-between mb-3">
        <button
          type="button"
          aria-label="Previous month"
          onClick={() => setMonth(addMonths(month, -1))}
          className="grid h-7 w-7 place-items-center rounded-[var(--radius-button)] text-[var(--color-ink-soft)] hover:bg-[var(--color-surface-muted)] hover:text-[var(--color-ink)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-focus)]"
        >
          <ChevronLeft className="h-4 w-4" />
        </button>
        <span className="text-sm font-semibold tracking-tight text-[var(--color-ink)]" aria-live="polite">
          {monthLabel}
        </span>
        <button
          type="button"
          aria-label="Next month"
          onClick={() => setMonth(addMonths(month, 1))}
          className="grid h-7 w-7 place-items-center rounded-[var(--radius-button)] text-[var(--color-ink-soft)] hover:bg-[var(--color-surface-muted)] hover:text-[var(--color-ink)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-focus)]"
        >
          <ChevronRight className="h-4 w-4" />
        </button>
      </div>
      <div className="grid grid-cols-7 gap-0 text-center">
        {['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa'].map((d) => (
          <span key={d} className="py-1 text-xs font-medium text-[var(--color-ink-faint)]">
            {d}
          </span>
        ))}
        {cells.map((date, i) =>
          date === null ? (
            <span key={`e-${i}`} className="h-8" />
          ) : (
            (() => {
              const isSelected = selected ? isSameDay(date, selected) : false;
              const isToday = isSameDay(date, today);
              const isDisabled = disabled ? disabled(date) : false;
              return (
                <button
                  key={date.toISOString()}
                  type="button"
                  disabled={isDisabled}
                  aria-pressed={isSelected}
                  aria-label={date.toLocaleDateString()}
                  onClick={() => onSelect?.(isSelected ? undefined : date)}
                  className={cn(
                    'h-8 w-8 rounded-[var(--radius-button)] text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-focus)] focus-visible:ring-offset-1 disabled:opacity-30 disabled:pointer-events-none',
                    isSelected
                      ? 'bg-[var(--color-accent)] text-white font-semibold shadow-[var(--shadow-xs)] hover:bg-[var(--color-accent-strong)]'
                      : isToday
                        ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent-strong)] font-semibold border border-[var(--color-accent)]/20'
                        : 'text-[var(--color-ink)] hover:bg-[var(--color-surface-muted)]',
                  )}
                >
                  {date.getDate()}
                </button>
              );
            })()
          ),
        )}
      </div>
    </div>
  );
}

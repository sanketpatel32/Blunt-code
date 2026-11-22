import { Button } from './ui/button';
import { cn } from '../lib/utils';

export type PaginationProps = {
  /** Accessible name for the navigation landmark. */
  'aria-label': string;
  onPrevious: () => void;
  onNext: () => void;
  hasPrevious: boolean;
  hasNext: boolean;
  busy?: boolean;
  currentPage?: number;
  pageCount?: number;
  className?: string;
  size?: 'sm' | 'default';
  variant?: 'outline' | 'ghost';
};

export function Pagination({
  'aria-label': ariaLabel,
  onPrevious,
  onNext,
  hasPrevious,
  hasNext,
  busy = false,
  currentPage,
  pageCount,
  className,
  size = 'sm',
  variant = 'outline',
}: PaginationProps) {
  const showPageIndicator =
    currentPage !== undefined && pageCount !== undefined;

  return (
    <nav className={cn('flex flex-wrap items-center gap-1', className)} aria-label={ariaLabel}>
      <Button
        type="button"
        variant={variant}
        size={size}
        disabled={!hasPrevious || busy}
        aria-disabled={!hasPrevious || busy}
        onClick={onPrevious}
      >
        Previous
      </Button>
      {showPageIndicator && (
        <output className="tabular-nums text-xs text-[var(--color-ink-soft)]" aria-live="polite">
          Page {currentPage} of {pageCount}
        </output>
      )}
      <Button
        type="button"
        variant={variant}
        size={size}
        disabled={!hasNext || busy}
        aria-disabled={!hasNext || busy}
        onClick={onNext}
      >
        Next
      </Button>
    </nav>
  );
}

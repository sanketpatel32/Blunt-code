import * as React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '../../lib/utils';

const badgeVariants = cva('inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold font-mono transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2', {
  variants: {
    variant: {
      default: 'border-transparent bg-[var(--color-surface-muted)] text-[var(--color-ink-soft)]',
      success: 'border-[color-mix(in_oklch,var(--color-success)_40%,var(--color-rule))] bg-[var(--color-success-soft)] text-[var(--color-success)]',
      warning: 'border-[color-mix(in_oklch,var(--color-warning)_40%,var(--color-rule))] bg-[var(--color-warning-soft)] text-[var(--color-warning)]',
      danger: 'border-[color-mix(in_oklch,var(--color-danger)_40%,var(--color-rule))] bg-[var(--color-danger-soft)] text-[var(--color-danger)]',
      accent: 'border-[color-mix(in_oklch,var(--color-accent)_40%,var(--color-rule))] bg-[var(--color-accent-soft)] text-[var(--color-accent-strong)]',
      outline: 'text-[var(--color-ink-soft)] border-[var(--color-rule)]',
      secondary: 'border-transparent bg-[var(--color-surface-muted)] text-[var(--color-ink-soft)]',
    },
  },
  defaultVariants: { variant: 'default' },
});

export interface BadgeProps extends React.HTMLAttributes<HTMLDivElement>, VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return <div className={cn(badgeVariants({ variant }), className)} {...props} />;
}

export { Badge, badgeVariants };

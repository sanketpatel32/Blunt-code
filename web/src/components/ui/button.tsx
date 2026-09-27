import * as React from 'react';
import { Slot } from '@radix-ui/react-slot';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '../../lib/utils';

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-[var(--radius-button)] text-sm font-semibold ring-offset-background transition-colors duration-150 ease-out focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-focus)] focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0 active:scale-[0.98]',
  {
    variants: {
      variant: {
        /* The two FILLED variants shed their fill when disabled instead of
           fading it. The base class's `disabled:opacity-50` is right for every
           variant with a transparent or neutral background — halving the opacity
           of a ghost button just greys its text, which is exactly what "off"
           should look like. On a filled one it is wrong, because there the fill
           IS the identity: halving a saturated accent does not yield a greyer
           accent, it yields a LAVENDER one, a different hue that still reads as
           filled, prominent and clickable.

           That is how a disabled "Save selection" on the files page came to look
           like the most important control on screen, beside a perfectly healthy
           "Reset" that read as the secondary. Nothing about a disabled primary
           should still be shouting, so the fill goes and the outline stays. */
        default: 'bg-[var(--color-accent)] text-[var(--color-accent-ink)] hover:bg-[var(--color-accent-strong)] shadow-[var(--shadow-accent)] hover:shadow-[var(--shadow-accent)] disabled:opacity-100 disabled:bg-[var(--color-surface)] disabled:text-[var(--color-ink-faint)] disabled:border disabled:border-[var(--color-rule)] disabled:shadow-none',
        destructive: 'bg-[var(--color-danger)] text-[var(--color-danger-ink)] hover:bg-[var(--color-danger)]/90 shadow-sm disabled:opacity-100 disabled:bg-[var(--color-surface)] disabled:text-[var(--color-ink-faint)] disabled:border disabled:border-[var(--color-rule)] disabled:shadow-none',
        outline: 'border border-[var(--color-rule)] bg-[var(--color-surface)] text-[var(--color-ink)] hover:bg-[var(--color-surface-muted)] hover:border-[var(--color-rule-strong)] shadow-xs',
        secondary: 'bg-[var(--color-surface-muted)] text-[var(--color-ink)] hover:bg-[var(--color-surface-subtle)] border border-[var(--color-rule-faint)]',
        ghost: 'hover:bg-[var(--color-accent-soft)] hover:text-[var(--color-accent-strong)] text-[var(--color-ink-soft)]',
        link: 'text-[var(--color-accent-strong)] underline-offset-4 hover:underline shadow-none',
      },
      size: {
        default: 'h-10 px-5 py-2',
        sm: 'h-8 rounded-[var(--radius-button)] px-3.5 text-[13px]',
        lg: 'h-11 rounded-[var(--radius-button)] px-8 text-[15px]',
        icon: 'h-9 w-9 rounded-[var(--radius-button)]',
      },
    },
    defaultVariants: { variant: 'default', size: 'default' },
  },
);

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement>, VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(({ className, variant, size, asChild = false, ...props }, ref) => {
  const Comp = asChild ? Slot : 'button';
  return <Comp className={cn(buttonVariants({ variant, size, className }))} ref={ref} {...props} />;
});
Button.displayName = 'Button';
export { Button, buttonVariants };

import * as React from 'react';

export interface PageHeaderProps {
  eyebrow?: React.ReactNode;
  title: React.ReactNode;
  description?: React.ReactNode;
  badge?: React.ReactNode;
  actions?: React.ReactNode;
  className?: string;
  children?: React.ReactNode;
}

export function PageHeader({
  eyebrow,
  title,
  description,
  badge,
  actions,
  className = '',
  children,
}: PageHeaderProps) {
  return (
    <header className={`page-heading page-header-compact flex flex-wrap items-center justify-between gap-3 pb-2.5 mb-3 border-b border-[var(--color-rule-faint)] ${className}`}>
      <div className="min-w-0 flex-1 flex items-center gap-2.5 flex-wrap">
        {eyebrow && (
          <span className="text-[10px] font-mono font-semibold uppercase tracking-wider text-[var(--color-accent-strong)] bg-[var(--color-accent-soft)] px-2 py-0.5 rounded-[var(--radius-xs)] border border-[var(--color-accent)]/25 shrink-0">
            {eyebrow}
          </span>
        )}
        <div className="flex items-center gap-2">
          <h1 className="text-lg md:text-xl font-bold tracking-tight text-[var(--color-ink)] m-0 leading-tight">
            {title}
          </h1>
          {badge && <div className="shrink-0">{badge}</div>}
        </div>
        {description && (
          <span className="hidden lg:inline-block text-xs text-[var(--color-ink-soft)] truncate pl-2 border-l border-[var(--color-rule)]">
            {description}
          </span>
        )}
        {children}
      </div>
      {actions && (
        <div className="page-heading-actions flex items-center gap-2 shrink-0">
          {actions}
        </div>
      )}
    </header>
  );
}

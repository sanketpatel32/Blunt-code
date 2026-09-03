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
    <header className={`page-heading page-header flex flex-col md:flex-row md:items-start md:justify-between gap-3 pb-3.5 mb-4 border-b border-[var(--color-rule-faint)] ${className}`}>
      <div className="page-heading-main min-w-0 flex-1 space-y-1">
        {eyebrow && (
          <div className="page-heading-eyebrow flex items-center gap-2">
            <span className="text-[10px] font-mono font-semibold uppercase tracking-wider text-[var(--color-accent-strong)] bg-[var(--color-accent-soft)] px-2 py-0.5 rounded-[var(--radius-xs)] border border-[var(--color-accent)]/20 inline-flex items-center leading-normal">
              {eyebrow}
            </span>
          </div>
        )}
        <div className="page-heading-title-row flex items-center gap-2.5 flex-wrap">
          <h1 className="text-xl sm:text-2xl font-bold tracking-tight text-[var(--color-ink)] m-0 leading-tight font-[var(--font-display)]">
            {title}
          </h1>
          {badge && <div className="page-heading-badge shrink-0 flex items-center">{badge}</div>}
        </div>
        {description && (
          <div className="page-heading-description text-xs sm:text-sm text-[var(--color-ink-soft)] leading-relaxed max-w-3xl">
            {description}
          </div>
        )}
        {children}
      </div>
      {actions && (
        <div className="page-heading-actions flex items-center gap-2 shrink-0 self-start md:self-center">
          {actions}
        </div>
      )}
    </header>
  );
}

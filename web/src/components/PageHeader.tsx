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
    <header className={`page-heading page-header ${className}`}>
      <div className="page-heading-main min-w-0 flex-1 space-y-1">
        {eyebrow && (
          <div className="page-heading-eyebrow flex items-center gap-2">
            <span className="text-xs font-medium text-[var(--color-ink-soft)] inline-flex items-center gap-1.5 leading-normal">
              {eyebrow}
            </span>
          </div>
        )}
        <div className="page-heading-title-row flex items-center gap-2.5 flex-wrap">
          <h1 className="text-xl sm:text-2xl font-bold tracking-tight text-[var(--color-ink)] m-0 leading-tight font-[var(--font-display)]">
            {title}
          </h1>
          {/* min-w-0 (not shrink-0) lets the badge shrink so wrapping content —
              e.g. the workspace language dots — never pushes the page wide. */}
          {badge && <div className="page-heading-badge min-w-0 flex items-center">{badge}</div>}
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

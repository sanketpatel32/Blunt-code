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
    <header className={`page-heading flex flex-wrap items-center justify-between gap-3 pb-3 mb-4 border-b border-[var(--color-rule-faint)] ${className}`}>
      <div className="min-w-0 flex-1">
        {eyebrow && (
          typeof eyebrow === 'string' ? (
            <p className="eyebrow">{eyebrow}</p>
          ) : (
            <div className="eyebrow">{eyebrow}</div>
          )
        )}
        <div className="flex items-center gap-2.5 flex-wrap">
          <h1 className="text-xl md:text-2xl font-bold tracking-tight text-[var(--color-ink)] m-0 leading-tight">
            {title}
          </h1>
          {badge}
        </div>
        {description && (
          <p className="text-xs text-[var(--color-ink-soft)] mt-1 max-w-3xl leading-normal">
            {description}
          </p>
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

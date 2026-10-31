import * as React from 'react';

/**
 * Title-only page header. Eyebrows and one-line descriptions were removed in
 * v0.48: every page now leads with its name, and anything that used to be
 * explained in a sentence is shown by the page's charts and tiles instead.
 * `meta` is for compact, functional chips only (a copyable path), never prose.
 */
export interface PageHeaderProps {
  title: React.ReactNode;
  badge?: React.ReactNode;
  meta?: React.ReactNode;
  actions?: React.ReactNode;
  className?: string;
  children?: React.ReactNode;
}

export function PageHeader({ title, badge, meta, actions, className = '', children }: PageHeaderProps) {
  return (
    <header className={`page-heading page-header ${className}`}>
      <div className="page-heading-main min-w-0 flex-1">
        <div className="page-heading-title-row flex items-center gap-2.5 flex-wrap">
          <h1 className="page-title m-0 leading-tight">{title}</h1>
          {/* min-w-0 (not shrink-0) lets the badge shrink so wrapping content —
              e.g. the workspace language dots — never pushes the page wide. */}
          {badge && <div className="page-heading-badge min-w-0 flex items-center">{badge}</div>}
        </div>
        {meta && <div className="page-heading-meta mt-1.5 min-w-0">{meta}</div>}
        {children}
      </div>
      {actions && (
        <div className="page-heading-actions flex flex-wrap items-center gap-2 shrink-0 self-start md:self-center">
          {actions}
        </div>
      )}
    </header>
  );
}

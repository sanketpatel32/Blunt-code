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
  /* The eyebrow is the template tell this header was repeating on all 13
     routes: an uppercase label above a title that already said the same
     thing. "DASHBOARD" over "Risk board", "WORKSPACE" over "Blunt-code",
     "FINDINGS SEARCH" over "Search findings", "FILE SELECTION" over
     "Blunt-code — Files". Nine labels, zero information — the page title is
     20px below and says it better.

     It is now a real metadata slot: small and quiet, sitting UNDER the title
     rather than above it, so it reads as annotating the page instead of
     filing it. Callers that pass something load-bearing still render it; what
     is gone is the shouting.

     The h1 drops its Tailwind size utilities and takes `--text-page-title`,
     because those utilities hardcoded `text-2xl` at the sm+ breakpoint and
     that is how the report page ended up with a 28px title while every other
     page had a different size again. */
  return (
    <header className={`page-heading page-header ${className}`}>
      <div className="page-heading-main min-w-0 flex-1">
        <div className="page-heading-title-row flex items-center gap-2.5 flex-wrap">
          <h1 className="page-title m-0 leading-tight">
            {title}
          </h1>
          {/* min-w-0 (not shrink-0) lets the badge shrink so wrapping content —
              e.g. the workspace language dots — never pushes the page wide. */}
          {badge && <div className="page-heading-badge min-w-0 flex items-center">{badge}</div>}
        </div>
        {eyebrow && (
          <p className="page-eyebrow">{eyebrow}</p>
        )}
        {description && (
          <div className="page-heading-description text-xs sm:text-sm text-[var(--color-ink-soft)] leading-relaxed max-w-3xl mt-1">
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

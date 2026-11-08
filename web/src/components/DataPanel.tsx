import type { ReactNode } from 'react';
import { cn } from '../lib/utils';

export type DataPanelProps = {
  id?: string;
  titleId: string;
  title: ReactNode;
  headerActions?: ReactNode;
  footer?: ReactNode;
  className?: string;
  panelClassName?: string;
  children: ReactNode;
};

export function DataPanel({
  id,
  titleId,
  title,
  headerActions,
  footer,
  className,
  panelClassName = 'board-panel',
  children,
}: DataPanelProps) {
  return (
    <section
      id={id}
      className={cn(panelClassName, className)}
      aria-labelledby={titleId}
    >
      <header className="board-panel-head">
        <div>
          <h2 id={titleId} className="board-panel-title">
            {title}
          </h2>
        </div>
        {headerActions}
      </header>
      {children}
      {footer}
    </section>
  );
}

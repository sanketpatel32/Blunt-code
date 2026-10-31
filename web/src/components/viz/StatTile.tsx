import type { ReactNode } from 'react';
import type { LucideIcon } from 'lucide-react';
import { AnimatedNumber } from './AnimatedNumber';

/** Icon + big number + two-word label. `href`/`onClick` make it a link. */
export function StatTile({ icon: Icon, value, label, tone, href, onClick, extra, title }: { icon: LucideIcon; value: number | string; label: string; tone?: 'danger' | 'warn' | 'ok'; href?: string; onClick?: (event: React.MouseEvent<HTMLAnchorElement>) => void; extra?: ReactNode; title?: string }) {
  const body = (
    <>
      <span className="stat-tile-icon" aria-hidden="true"><Icon /></span>
      <span className="stat-tile-value">{typeof value === 'number' ? <AnimatedNumber value={value} /> : value}</span>
      <span className="stat-tile-label">{label}</span>
      {extra && <span className="stat-tile-extra">{extra}</span>}
    </>
  );
  return href
    ? <a className="stat-tile" data-tone={tone} href={href} onClick={onClick} title={title}>{body}</a>
    : <div className="stat-tile" data-tone={tone} title={title}>{body}</div>;
}

import type { Route } from '../lib/router';
import { Empty } from '../components/ui';
import { MagnifierIcon } from '../components/icons';

export function NotFoundPage({ go }: { go: (r: Route) => void }) {
  return <div className="page"><header className="page-heading"><div><p className="eyebrow">404</p><h1>Page not found</h1><p>This address does not match any page in Blunt Code.</p></div></header>
    <Empty title="Nothing here" icon={<MagnifierIcon />} action={<div className="not-found-actions"><button type="button" className="button primary" onClick={() => go({ page: 'home' })}>Go to Home</button><button type="button" className="button secondary" onClick={() => go({ page: 'workspaces' })}>Open Workspaces</button></div>}>The link may be mistyped or out of date. Your workspaces and reports are still saved on this computer.</Empty>
  </div>;
}

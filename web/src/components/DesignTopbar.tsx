import type { Route } from '../lib/router';
import { href } from '../lib/router';
import { navigateFromLink } from '../lib/navigation';
import type { Workspace } from '../types';
import { Search } from 'lucide-react';

const LABELS: Record<Route['page'], string> = {
  home: 'Overview', workspaces: 'Workspaces', workspace: 'Overview', files: 'Files & rules',
  history: 'Scan history', scan: 'Scan report', search: 'Findings', tools: 'Analyzers', pentest: 'Pentest & security',
  rules: 'Rule drafts', settings: 'Settings', about: 'About', cli: 'CLI reference', 'not-found': 'Page not found',
};
export function DesignTopbar({ route, go, workspace }: { workspace?: Workspace; route: Route; go: (route: Route) => void }) {
  return <header className="design-topbar">
    <nav aria-label="Breadcrumb">
      <a href="/" onClick={(e) => navigateFromLink(e, () => go({ page: 'home' }))}>Blunt Code</a>
      {workspace && <><span aria-hidden="true">/</span><a href={href({ page: 'workspace', id: workspace.id })} onClick={(e) => navigateFromLink(e, () => go({ page: 'workspace', id: workspace.id }))}>{workspace.name}</a></>}
      <span aria-hidden="true">/</span><span aria-current="page">{LABELS[route.page]}</span>
    </nav>
    <button className="topbar-search" onClick={() => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, bubbles: true }))} aria-label="Open command search"><Search size={14} /><span>Search commands</span><kbd>Ctrl K</kbd></button>
  </header>;
}

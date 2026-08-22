import { href, type Route } from '../lib/router';

export function AppShell({ route, onNavigate, onAdd, onClose }: { route: Route; onNavigate: (route: Route) => void; onAdd: () => void; onClose: () => void }) {
  const items: Array<[Route, string]> = [[{ page: 'home' }, 'Home'], [{ page: 'workspaces' }, 'Workspaces'], [{ page: 'tools' }, 'Tools'], [{ page: 'settings' }, 'Settings']];
  return <header className="app-nav">
    <a className="brand" href="/" onClick={(event) => { event.preventDefault(); onNavigate({ page: 'home' }); }}><span aria-hidden="true">BC</span><b>Blunt Code</b></a>
    <nav aria-label="Main navigation">{items.map(([next, label]) => <a key={label} href={href(next)} className={route.page === next.page ? 'active' : ''} onClick={(event) => { event.preventDefault(); onNavigate(next); }}>{label}</a>)}</nav>
    <div className="nav-actions"><button className="button secondary close-app" onClick={onClose}>Close app</button><button className="button primary add" onClick={onAdd}>Add workspace</button></div>
  </header>;
}

export function AppFooter() {
  return <footer className="app-footer"><span>Blunt Code · local code analysis for Windows</span><span>No account. No telemetry.</span></footer>;
}

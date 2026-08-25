import { href, type Route } from '../lib/router';
import type { Theme } from '../hooks/useTheme';

export function AppShell({ route, onNavigate, onAdd, onClose, theme, onToggleTheme, onShowShortcuts, seqArmed = false }: { route: Route; onNavigate: (route: Route) => void; onAdd: () => void; onClose: () => void; theme: Theme; onToggleTheme: () => void; onShowShortcuts?: () => void; seqArmed?: boolean }) {
  const items: Array<[Route, string]> = [[{ page: 'home' }, 'Home'], [{ page: 'workspaces' }, 'Workspaces'], [{ page: 'tools' }, 'Tools'], [{ page: 'settings' }, 'Settings'], [{ page: 'about' }, 'About']];
  return <header className="app-nav">
    <a className="brand" href="/" onClick={(event) => { event.preventDefault(); onNavigate({ page: 'home' }); }}><svg className="brand-mark" viewBox="0 0 32 32" aria-hidden="true" focusable="false"><rect width="32" height="32" rx="7.5" fill="var(--color-accent)" /><g fill="none" stroke="var(--color-accent-ink)" strokeWidth="3.4" strokeLinecap="round" strokeLinejoin="round"><polyline points="9.5 8.5 17 16 9.5 23.5" /><line x1="20.75" y1="23.5" x2="24.25" y2="23.5" /></g></svg><b>Blunt Code</b></a>
    <nav aria-label="Main navigation">{items.map(([next, label]) => <a key={label} href={href(next)} className={route.page === next.page ? 'active' : ''} onClick={(event) => { event.preventDefault(); onNavigate(next); }}>{label}</a>)}</nav>
    <div className="nav-actions">{seqArmed && <span className="seq-hint" aria-hidden="true">g…</span>}<button type="button" className="icon-button nav-shortcuts" onClick={() => onShowShortcuts?.()} title="Keyboard shortcuts" aria-label="Keyboard shortcuts" aria-keyshortcuts="?">?</button><button type="button" className="button secondary theme-toggle" onClick={onToggleTheme} aria-pressed={theme === 'dark'} title={theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}>{theme === 'dark' ? <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" /></svg> : <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" /></svg>}<span className="theme-toggle-label">{theme === 'dark' ? 'Light' : 'Dark'}</span></button><button type="button" className="button secondary close-app" onClick={onClose}>Close app</button><button type="button" className="button primary add" onClick={onAdd}>Add workspace</button></div>
  </header>;
}

export function AppFooter() {
  return <footer className="app-footer"><span>Blunt Code · local code analysis for Windows</span><span>No account. No telemetry.</span></footer>;
}

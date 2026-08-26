import { href, type Route } from '../lib/router';
import type { Theme } from '../hooks/useTheme';
import { Button } from './ui/button';
import { HelpCircle, Moon, Sun, Plus, X } from 'lucide-react';
import { cn } from '../lib/utils';

export function AppShell({ route, onNavigate, onAdd, onClose, theme, onToggleTheme, onShowShortcuts, seqArmed = false }: { route: Route; onNavigate: (route: Route) => void; onAdd: () => void; onClose: () => void; theme: Theme; onToggleTheme: () => void; onShowShortcuts?: () => void; seqArmed?: boolean }) {
  const items: Array<[Route, string, string]> = [
    [{ page: 'home' }, 'Home', 'Home'],
    [{ page: 'workspaces' }, 'Workspaces', 'Workspaces'],
    [{ page: 'search' }, 'Search', 'Search'],
    [{ page: 'tools' }, 'Tools', 'Tools'],
    [{ page: 'settings' }, 'Settings', 'Settings'],
    [{ page: 'about' }, 'About', 'About'],
  ];
  return (
    <header className="app-nav">
      <a className="brand group" href="/" onClick={(event) => { event.preventDefault(); onNavigate({ page: 'home' }); }}>
        <svg className="brand-mark transition-transform group-hover:scale-[1.02]" viewBox="0 0 32 32" aria-hidden="true" focusable="false">
          <rect width="32" height="32" rx="8" fill="var(--color-ink)" />
          <rect x="7" y="7" width="18" height="18" rx="4" fill="none" stroke="white" strokeOpacity="0.18" strokeWidth="1" />
          <g fill="none" stroke="white" strokeWidth="2.8" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="11 9.5 17.2 16 11 22.5" />
            <line x1="20.2" y1="22.5" x2="24" y2="22.5" />
          </g>
        </svg>
        <b>Blunt Code</b>
      </a>
      <nav aria-label="Main navigation">
        {items.map(([next, label]) => (
          <a
            key={label}
            href={href(next)}
            className={cn(route.page === next.page ? 'active' : '')}
            aria-current={route.page === next.page ? 'page' : undefined}
            onClick={(event) => { event.preventDefault(); onNavigate(next); }}
          >
            {label}
          </a>
        ))}
      </nav>
      <div className="nav-actions">
        {seqArmed && <span className="seq-hint" aria-hidden="true">g…</span>}
        <Button variant="outline" size="icon" className="nav-shortcuts rounded-full h-[2.15rem] w-[2.15rem] border-[var(--color-rule)]" onClick={() => onShowShortcuts?.()} title="Keyboard shortcuts (?) " aria-label="Keyboard shortcuts">
          <HelpCircle className="h-4 w-4" />
        </Button>
        <Button variant="outline" size="sm" className="theme-toggle hidden sm:inline-flex" onClick={onToggleTheme} aria-pressed={theme === 'dark'} title={theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}>
          {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          <span className="theme-toggle-label">{theme === 'dark' ? 'Light' : 'Dark'}</span>
        </Button>
        <Button variant="ghost" size="sm" className="close-app" onClick={onClose}>
          Close app
        </Button>
        <Button onClick={onAdd} size="sm" className="add shadow-sm">
          <Plus className="h-4 w-4" /> Add workspace
        </Button>
      </div>
    </header>
  );
}

export function AppFooter() {
  return (
    <footer className="app-footer">
      <span>Blunt Code · local code analysis for Windows</span>
      <span className="hidden sm:inline">No account. No telemetry.</span>
    </footer>
  );
}

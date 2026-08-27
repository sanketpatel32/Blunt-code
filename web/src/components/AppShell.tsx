import { href, type Route } from '../lib/router';
import type { Theme } from '../hooks/useTheme';
import { Button } from './ui/button';
import { HelpCircle, Moon, Sun, Plus, Languages } from 'lucide-react';
import { NotificationsCenter } from './NotificationsCenter';
import { cn } from '../lib/utils';
import { LOCALES, useT } from '../lib/i18n';

export function AppShell({ route, onNavigate, onAdd, onClose, theme, onToggleTheme, onShowShortcuts, seqArmed = false }: { route: Route; onNavigate: (route: Route) => void; onAdd: () => void; onClose: () => void; theme: Theme; onToggleTheme: () => void; onShowShortcuts?: () => void; seqArmed?: boolean }) {
  const { t, locale, setLocale } = useT();
  const items: Array<[Route, string]> = [
    [{ page: 'home' }, t('nav.home')],
    [{ page: 'workspaces' }, t('nav.workspaces')],
    [{ page: 'search' }, t('nav.search')],
    [{ page: 'tools' }, t('nav.tools')],
    [{ page: 'pentest' }, t('nav.pentest')],
    [{ page: 'rules' }, 'Rules'],
    [{ page: 'settings' }, t('nav.settings')],
    [{ page: 'about' }, t('nav.about')],
  ];
  return (
    <header className="app-nav">
      <a className="brand group" href="/" onClick={(event) => { event.preventDefault(); onNavigate({ page: 'home' }); }}>
        <svg className="brand-mark transition-transform group-hover:scale-[1.02] group-active:scale-[0.99]" viewBox="0 0 32 32" aria-hidden="true" focusable="false">
          <rect width="32" height="32" rx="8" fill="var(--color-brand-mark)" />
          <rect x="7" y="7" width="18" height="18" rx="4" fill="none" stroke="white" strokeOpacity="0.10" strokeWidth="1" />
          <g fill="none" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="11 10.2 17.1 16 11 21.8" stroke="white" strokeWidth="2.8" opacity="0.98" />
            <line x1="19.9" y1="22.5" x2="24.2" y2="22.5" stroke="var(--color-brand-accent)" strokeWidth="2.8" />
          </g>
          <circle cx="24.2" cy="8.2" r="1.45" fill="var(--color-brand-accent)" />
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
        <label className="hidden sm:inline-flex items-center gap-1 text-[var(--color-ink-soft)]" aria-label={t('common.language')}>
          <Languages className="h-4 w-4 shrink-0" aria-hidden="true" />
          <select value={locale} onChange={(e) => setLocale(e.target.value as never)} className="h-[2.15rem] rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-2 text-xs font-mono font-semibold">
            {LOCALES.map((l) => <option key={l.value} value={l.value}>{l.label}</option>)}
          </select>
        </label>
        <NotificationsCenter />
        <Button variant="outline" size="icon" className="nav-shortcuts rounded-[var(--radius-button)] h-[2.15rem] w-[2.15rem] border-[var(--color-rule)] hover:border-[var(--color-rule-strong)]" onClick={() => onShowShortcuts?.()} title={t('common.shortcuts')} aria-label={t('common.shortcuts')}>
          <HelpCircle className="h-4 w-4" />
        </Button>
        <Button variant="outline" size="sm" className="theme-toggle hidden sm:inline-flex" onClick={onToggleTheme} aria-pressed={theme === 'dark'} title={theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}>
          {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          <span className="theme-toggle-label">{theme === 'dark' ? 'Light' : 'Dark'}</span>
        </Button>
        <Button variant="ghost" size="sm" className="close-app hidden lg:inline-flex" onClick={onClose}>
          {t('common.closeApp')}
        </Button>
        <Button onClick={onAdd} size="sm" className="add shadow-[var(--shadow-accent)] active:shadow-sm">
          <Plus className="h-4 w-4" /> <span className="hidden sm:inline">{t('common.addWorkspace')}</span><span className="sm:hidden">{t('common.add')}</span>
        </Button>
      </div>
    </header>
  );
}

export function AppFooter() {
  return (
    <footer className="app-footer">
      <span><span className="font-medium">Blunt Code</span><span className="hidden md:inline text-[var(--color-ink-faint)]"> · local code analysis for Windows</span></span>
      <span className="hidden sm:inline">No account. No telemetry. · v0.7</span>
    </footer>
  );
}

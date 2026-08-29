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
          <path d="M16 5.2 L23.6 9 L23.6 17.2 C23.6 21 20.2 24.5 16 26.8 C11.8 24.5 8.4 21 8.4 17.2 L8.4 9 Z" fill="none" stroke="var(--color-paper)" strokeOpacity="0.14" strokeWidth="1" strokeLinejoin="round"/>
          <path d="M11.8 15.9 L15.2 19.1 L20.6 12.1" fill="none" stroke="var(--color-paper)" strokeWidth="2.7" strokeLinecap="round" strokeLinejoin="round" opacity="0.96"/>
          <circle cx="23.4" cy="7.2" r="1.7" fill="var(--color-brand-accent)"/>
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
        {/* Loop 122 · this control was `hidden sm:`, so below 640px there was no
            way to change language at all. It now hides only below md, and
            Settings carries a full-width language row for phone widths. */}
        <label className="hidden md:inline-flex items-center gap-1 text-[var(--color-ink-soft)]" aria-label={t('common.language')}>
          <Languages className="h-4 w-4 shrink-0" aria-hidden="true" />
          <select value={locale} onChange={(e) => setLocale(e.target.value as never)} className="h-[2.15rem] rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-2 text-xs font-mono font-semibold">
            {LOCALES.map((l) => <option key={l.value} value={l.value}>{l.label}</option>)}
          </select>
        </label>
        <NotificationsCenter />
        <Button variant="outline" size="icon" className="nav-shortcuts rounded-[var(--radius-button)] h-[2.15rem] w-[2.15rem] border-[var(--color-rule)] hover:border-[var(--color-rule-strong)]" onClick={() => onShowShortcuts?.()} title={t('common.shortcuts')} aria-label={t('common.shortcuts')}>
          <HelpCircle className="h-4 w-4" />
        </Button>
        {/* Loop 121 · dark mode was unreachable on phones: the only toggle in the
            app carried `hidden sm:`. The label already collapses below 68rem, so
            the button is safe to show at every width as an icon. */}
        <Button variant="outline" size="sm" className="theme-toggle" onClick={onToggleTheme} aria-pressed={theme === 'dark'} title={theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}>
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

/** Injected by Vite from package.json — the footer used to hardcode "v0.7"
 *  while the app shipped 0.15.0. Falls back for non-Vite test runners. */
const APP_VERSION = typeof __APP_VERSION__ === 'string' ? __APP_VERSION__ : 'dev';

export function AppFooter() {
  return (
    <footer className="app-footer">
      <span><span className="font-medium">Blunt Code</span><span className="hidden md:inline text-[var(--color-ink-faint)]"> · local code analysis for Windows</span></span>
      <span className="hidden sm:inline">No account. No telemetry. · v{APP_VERSION}</span>
    </footer>
  );
}

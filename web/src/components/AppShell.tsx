import { useEffect, useRef } from 'react';
import { href, type Route } from '../lib/router';
import type { Theme } from '../hooks/useTheme';
import { Button } from './ui/button';
import { HelpCircle, Moon, Sun, Plus, Languages, ChevronDown } from 'lucide-react';
import { NotificationsCenter } from './NotificationsCenter';
import { cn } from '../lib/utils';
import { LOCALES, useT } from '../lib/i18n';
import { useReducedMotion } from '../hooks/useReducedMotion';

/**
 * The three places people actually live in. Everything else moves behind
 * "More": eight equal-weight links made every navigation a scan-and-choose,
 * and half of them are set-once destinations (Tools, Settings, About).
 */
const PRIMARY_PAGES: ReadonlyArray<Route['page']> = ['home', 'workspaces', 'search'];

export function AppShell({ route, onNavigate, onAdd, onClose, theme, onToggleTheme, onShowShortcuts, seqArmed = false }: { route: Route; onNavigate: (route: Route) => void; onAdd: () => void; onClose: () => void; theme: Theme; onToggleTheme: () => void; onShowShortcuts?: () => void; seqArmed?: boolean }) {
  const { t, locale, setLocale } = useT();
  const reduced = useReducedMotion();
  const moreRef = useRef<HTMLDetailsElement>(null);
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
  // Order is load-bearing: primary links, then the "More" disclosure, then the
  // secondary links — so "About" stays the last anchor in the nav.
  const primary = items.filter(([next]) => PRIMARY_PAGES.includes(next.page));
  const secondary = items.filter(([next]) => !PRIMARY_PAGES.includes(next.page));
  const secondaryActive = secondary.some(([next]) => next.page === route.page);

  // `<details>` alone leaves a floating panel open after you click away, which
  // reads as a stuck overlay. Close on outside pointer, on Escape, and after
  // navigating to one of the links inside.
  useEffect(() => {
    const close = () => { if (moreRef.current) moreRef.current.open = false; };
    const onPointerDown = (event: Event) => {
      if (moreRef.current && !moreRef.current.contains(event.target as Node)) close();
    };
    const onKeyDown = (event: KeyboardEvent) => { if (event.key === 'Escape') close(); };
    document.addEventListener('pointerdown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('pointerdown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, []);

  const link = ([next, label]: [Route, string]) => (
    <a
      key={label}
      href={href(next)}
      className={cn('nav-link', route.page === next.page ? 'active' : '')}
      aria-current={route.page === next.page ? 'page' : undefined}
      onClick={(event) => { event.preventDefault(); onNavigate(next); }}
    >
      {label}
    </a>
  );

  return (
    <header className={cn('app-nav', reduced && 'nav-no-motion')}>
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
        <div className="nav-primary">{primary.map(link)}</div>
        <details
          className="nav-more"
          ref={moreRef}
          data-active={secondaryActive ? 'true' : 'false'}
          onClick={(event) => { if ((event.target as HTMLElement).closest('a, button') && moreRef.current) moreRef.current.open = false; }}
        >
          <summary className="nav-more-toggle">
            {t('nav.more')}
            <ChevronDown className="nav-more-chevron" aria-hidden="true" />
          </summary>
          <div className="nav-more-panel">
            <div className="nav-more-group">{secondary.map(link)}</div>
            {/* Closing the app is terminal and rare, so it leaves the action bar
                for the foot of the overflow — reachable at every width, where
                styles.css used to hide it below 63rem. */}
            <div className="nav-more-foot">
              <Button variant="ghost" size="sm" className="close-app" onClick={onClose}>{t('common.closeApp')}</Button>
            </div>
          </div>
        </details>
      </nav>
      <div className="nav-actions">
        {seqArmed && <span className="seq-hint" aria-hidden="true">g…</span>}
        <NotificationsCenter />
        {/* Preferences are chosen once and then never touched. They read as one
            cohesive group instead of four competing buttons — and every one of
            them is still reachable from the command palette (Ctrl/Cmd+K). */}
        <div className="nav-utils">
          <label className="nav-util nav-lang" aria-label={t('common.language')}>
            <Languages className="h-4 w-4 shrink-0" aria-hidden="true" />
            <select value={locale} onChange={(e) => setLocale(e.target.value as never)} className="nav-lang-select" title={t('common.language')}>
              {LOCALES.map((l) => <option key={l.value} value={l.value}>{l.label}</option>)}
            </select>
          </label>
          <Button variant="ghost" size="icon" className="nav-shortcuts" onClick={() => onShowShortcuts?.()} title={t('common.shortcuts')} aria-label={t('common.shortcuts')}>
            <HelpCircle className="h-4 w-4" />
          </Button>
          <Button variant="ghost" size="sm" className="theme-toggle" onClick={onToggleTheme} aria-pressed={theme === 'dark'} title={theme === 'dark' ? t('common.switchToLight') : t('common.switchToDark')}>
            {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            {/* Icon-only in the cluster, but the name has to stay in the DOM:
                it is the button's accessible label, not just decoration. */}
            <span className="theme-toggle-label sr-only">{theme === 'dark' ? t('common.themeLight') : t('common.themeDark')}</span>
          </Button>
        </div>
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

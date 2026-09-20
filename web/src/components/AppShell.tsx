import { href, type Route } from '../lib/router';
import { navigateFromLink } from '../lib/navigation';
import type { Theme } from '../hooks/useTheme';
import { Button } from './ui/button';
import { Check, ChevronDown, FileCode, HelpCircle, Info, Languages, MoreHorizontal, Moon, Power, Settings, Sun, Terminal } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { NotificationsCenter } from './NotificationsCenter';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuShortcut, DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger } from './ui/dropdown-menu';
import { cn } from '../lib/utils';
import { LOCALES, useT } from '../lib/i18n';
import { useReducedMotion } from '../hooks/useReducedMotion';

/**
 * The four high-frequency pages sit flat in the rail; everything else collapses
 * behind the permanent "More" menu (nav-clarity audit: the rail had eight
 * top-level items, most of them visited rarely). The menu is NOT
 * viewport-dependent — with four links plus "More" the bar fits at every width
 * this app supports, and a static structure cannot oscillate the way the old
 * measure-driven flat/overflow flip could.
 */
const PRIMARY_PAGES: ReadonlyArray<Route['page']> = ['home', 'workspaces', 'search', 'tools'];
/** Collapse the rare pages into "More". History is deliberately absent from
 *  BOTH groups: the route is workspace-scoped (/workspaces/:id/scans), so a
 *  nav link without an id would land on the 404 page. Scan history stays
 *  reachable from each workspace (and the home dashboard's recent activity). */
const MORE_PAGES: ReadonlyArray<Route['page']> = ['rules', 'cli', 'settings', 'about'];

const MORE_ICONS: Partial<Record<Route['page'], LucideIcon>> = {
  rules: FileCode,
  cli: Terminal,
  settings: Settings,
  about: Info,
};

/** One-line tooltip per nav link (nav-clarity audit): what lives behind it,
 *  no essays. Every Route page needs an entry for Record exhaustiveness; the
 *  workspace-scoped pages simply never appear in this bar. */
const NAV_TITLES: Record<Route['page'], string> = {
  home: 'Dashboard and recent activity',
  workspaces: 'Your registered projects',
  workspace: 'Workspace overview',
  files: 'Workspace files',
  history: 'Scan history',
  scan: 'Run a scan',
  search: 'Findings across all scans',
  tools: 'Analyzer status & installs',
  pentest: 'Dynamic web probes',
  rules: 'Your custom YAML rule scratchpad',
  settings: 'App preferences',
  about: 'Version, updates, and privacy',
  cli: 'Command-line reference',
  'not-found': 'Page not found',
};

export function AppShell({ route, onNavigate, onClose, theme, onToggleTheme, onShowShortcuts, seqArmed = false }: { route: Route; onNavigate: (route: Route) => void; /** No longer rendered in the nav (one primary action per view: the Workspaces and Home pages own their Add/Scan CTAs, and the Ctrl+K palette keeps its Add entry). The prop stays in the signature because App.tsx — which owns the AddWorkspaceDialog — still passes it. */ onAdd?: () => void; onClose: () => void; theme: Theme; onToggleTheme: () => void; onShowShortcuts?: () => void; seqArmed?: boolean }) {
  const { t, locale, setLocale } = useT();
  const reduced = useReducedMotion();
  const primary: Array<[Route, string]> = [
    [{ page: 'home' }, t('nav.home')],
    [{ page: 'workspaces' }, t('nav.workspaces')],
    [{ page: 'search' }, t('nav.search')],
    [{ page: 'tools' }, t('nav.tools')],
  ];
  const more: Array<[Route, string]> = [
    [{ page: 'rules' }, t('nav.rules')],
    [{ page: 'cli' }, t('nav.cli')],
    [{ page: 'settings' }, t('nav.settings')],
    [{ page: 'about' }, t('nav.about')],
  ];
  const moreActive = more.some(([next]) => next.page === route.page);

  const link = ([next, label]: [Route, string]) => (
    <a
      key={label}
      href={href(next)}
      className={cn('nav-link', route.page === next.page ? 'active' : '')}
      aria-current={route.page === next.page ? 'page' : undefined}
      title={NAV_TITLES[next.page]}
      onClick={(event) => navigateFromLink(event, () => onNavigate(next))}
    >
      {label}
    </a>
  );

  return (
    <header className={cn('app-nav', reduced && 'nav-no-motion')}>
      <a className="brand group" href="/" onClick={(event) => navigateFromLink(event, () => onNavigate({ page: 'home' }))}>
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
        {/* The collapsed pages keep their route semantics: aria-current rides on
            the active menu item (a menu button itself cannot be "the current
            page"), the dot repeats it visually, and the toggle tints accent so
            the bar still answers "where am I?" while the menu is closed. */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="nav-more-toggle"
              data-active={moreActive ? 'true' : undefined}
            >
              {t('nav.more')}
              <ChevronDown className="nav-more-chevron" aria-hidden="true" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="min-w-[11rem] p-1">
            {more.map(([next, label]) => {
              const Icon = MORE_ICONS[next.page];
              return (
                <DropdownMenuItem
                  key={label}
                  onSelect={() => onNavigate(next)}
                  aria-current={route.page === next.page ? 'page' : undefined}
                  title={NAV_TITLES[next.page]}
                  className="gap-2 cursor-pointer"
                >
                  {Icon ? <Icon className="h-4 w-4 text-[var(--color-ink-faint)]" aria-hidden="true" /> : null}
                  <span>{label}</span>
                  {route.page === next.page && <span className="nav-more-dot" aria-hidden="true" />}
                </DropdownMenuItem>
              );
            })}
          </DropdownMenuContent>
        </DropdownMenu>
      </nav>
      <div className="nav-actions">
        {seqArmed && <span className="seq-hint" aria-hidden="true">g…</span>}
        {/* Preferences are chosen once and then never touched. They read as one
            cohesive group instead of competing buttons — and every one of them
            is still reachable from the command palette (Ctrl/Cmd+K).
            Notifications live here too: it is a utility, and styling it apart
            made it the loudest thing in the row for the wrong reason. */}
        <div className="nav-utils">
          <NotificationsCenter routeKey={href(route)} />
          {/* The command palette is the fastest path to every action, but until
              now its only advertisement was a line inside the "?" dialog. This
              pill both advertises and triggers it — as ONE bordered pill, not
              the two adjacent key boxes that read as broken chrome. App's
              Ctrl/Cmd+K listener is a plain window keydown handler with no
              isTrusted check, so re-dispatching the same synthetic keydown
              opens the palette. */}
          <Button
            variant="ghost"
            size="sm"
            className="nav-util nav-palette px-2 font-mono text-xs font-semibold"
            onClick={() => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, bubbles: true }))}
            title="Open the command palette (Ctrl+K)"
            aria-label="Open the command palette (Ctrl+K)"
          >
            <kbd className="kbd-hint nav-palette-kbd">Ctrl K</kbd>
          </Button>
          {/* The grab-bag overflow: shortcuts help, language, and Close app —
              two of which used to be permanent buttons competing with every
              page's own actions. Close app is the destructive end of the menu
              (danger tone, below the separator) and keeps invoking App's
              confirmation → AppClosedScreen flow unchanged. */}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="nav-util nav-overflow"
                aria-label="More options"
                title="More options"
              >
                <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-[13rem] p-1">
              <DropdownMenuItem onSelect={() => onShowShortcuts?.()} className="gap-2 cursor-pointer">
                <HelpCircle className="h-4 w-4 text-[var(--color-ink-faint)]" aria-hidden="true" />
                <span className="font-medium">{t('common.shortcuts')}</span>
                <DropdownMenuShortcut>?</DropdownMenuShortcut>
              </DropdownMenuItem>
              <DropdownMenuSub>
                <DropdownMenuSubTrigger className="gap-2 cursor-pointer">
                  <Languages className="h-4 w-4 text-[var(--color-ink-faint)]" aria-hidden="true" />
                  <span className="font-medium">{t('common.language')}</span>
                </DropdownMenuSubTrigger>
                <DropdownMenuSubContent className="min-w-[11rem] p-1">
                  {LOCALES.map((l) => (
                    <DropdownMenuItem
                      key={l.value}
                      onSelect={() => setLocale(l.value as never)}
                      className="flex items-center justify-between gap-2 cursor-pointer"
                    >
                      <span className="flex items-center gap-2">
                        <span className="font-mono font-bold">{l.label}</span>
                        <span className="text-[var(--color-ink-soft)]">{l.name}</span>
                      </span>
                      {locale === l.value && <Check className="h-3.5 w-3.5 text-[var(--color-accent-strong)]" />}
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuSubContent>
              </DropdownMenuSub>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                onSelect={onClose}
                className="nav-danger-item gap-2 cursor-pointer text-[var(--color-danger)] focus:text-[var(--color-danger)]"
              >
                <Power className="h-4 w-4" aria-hidden="true" />
                <span className="font-medium">{t('common.closeApp')}</span>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          <Button variant="ghost" size="sm" className="theme-toggle" onClick={onToggleTheme} aria-pressed={theme === 'dark'} title={theme === 'dark' ? t('common.switchToLight') : t('common.switchToDark')} aria-label={theme === 'dark' ? t('common.switchToLight') : t('common.switchToDark')}>
            {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            {/* Icon-only in the cluster. The aria-label is the authoritative
                accessible name — phrased as the ACTION the button performs, so
                screen readers announce "Switch to light theme", never a no-op —
                because styles.css display:none's this sr-only twin below 68rem
                and a hidden span names nothing. It stays in the DOM as the
                visible-state fallback for wide viewports. */}
            <span className="theme-toggle-label sr-only">{theme === 'dark' ? t('common.switchToLight') : t('common.switchToDark')}</span>
          </Button>
        </div>
      </div>
    </header>
  );
}

/** Injected by Vite from package.json — the footer used to hardcode "v0.7"
 *  while the app shipped 0.15.0. Falls back for non-Vite test runners. */
const APP_VERSION = typeof __APP_VERSION__ === 'string' ? __APP_VERSION__ : 'dev';

export function AppFooter() {
  const { t } = useT();
  return (
    <footer className="app-footer">
      <span><span className="font-medium">Blunt Code</span><span className="hidden md:inline text-[var(--color-ink-soft)]"> · local code analysis for Windows</span></span>
      {/* Real legal/version content, not decoration: the footer's inherited
          ghost token fails contrast, so this span carries the readable
          ink-soft color instead. */}
      <span className="hidden sm:inline text-[var(--color-ink-soft)]">{t('common.noAccount')} · v{APP_VERSION}</span>
    </footer>
  );
}

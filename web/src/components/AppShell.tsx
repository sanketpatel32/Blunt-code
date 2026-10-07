import type { Workspace } from '../types';
import { Sheet, SheetTrigger, SheetContent, SheetTitle, SheetDescription } from './ui/sheet';
import * as React from 'react';
import { href, type Route } from '../lib/router';
import { navigateFromLink } from '../lib/navigation';
import type { Theme } from '../hooks/useTheme';
import { Button } from './ui/button';
import {
  Check,
  FileCode,
  FolderCog,
  HelpCircle,
  Info,
  Languages,
  LayoutDashboard,
  Moon,
  Power,
  Search,
  Settings,
  SlidersHorizontal,
  Sun,
  Terminal,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { NotificationsCenter } from './NotificationsCenter';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuShortcut, DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger } from './ui/dropdown-menu';
import { cn } from '../lib/utils';
import { LOCALES, useT } from '../lib/i18n';
import { useReducedMotion } from '../hooks/useReducedMotion';
import { workspaceSections } from './WorkspaceContext';

type NavItem = { route: Route; label: string; icon: LucideIcon; hint: string };

/**
 * BLUNT CODE — APP SHELL
 *
 * A left rail, not a top bar. This is the structural change: a horizontal nav
 * over full-width content is the single most recognisable shape of a generic
 * SaaS dashboard, and no amount of type or colour work makes that shell feel
 * designed. Every serious developer tool puts its navigation in a vertical rail
 * on the left, and there is a practical reason as well as an aesthetic one:
 *
 *   - All EIGHT top-level pages are visible. The old bar showed four and hid
 *     Rules, CLI, Settings and About behind a "More" menu, so half the app was
 *     invisible and you had to know it existed.
 *   - There is room for the workspace's own pages. Overview / Pentest / Files /
 *     History used to occupy a second horizontal bar with a monospace
 *     "IN THIS WORKSPACE" label on top of every one of those pages. Inside a
 *     workspace they now live in the rail under the workspace's name, so
 *     exactly one navigation element exists on screen at any time.
 *
 * Grouped by frequency, not alphabet: the pages you live in are at the top, the
 * reference material (Rules, CLI) sits in the middle, and Settings/About plus
 * the app-level utilities close the rail.
 */
const PRIMARY: ReadonlyArray<Route['page']> = ['home', 'workspaces', 'search', 'tools'];
const SECONDARY: ReadonlyArray<Route['page']> = ['rules', 'cli'];
const SETTINGS_PAGES: ReadonlyArray<Route['page']> = ['settings', 'about'];

const NAV_ICONS: Partial<Record<Route['page'], LucideIcon>> = {
  home: LayoutDashboard,
  workspaces: FolderCog,
  search: Search,
  tools: SlidersHorizontal,
  rules: FileCode,
  cli: Terminal,
  settings: Settings,
  about: Info,
};

const NAV_HINTS: Record<Route['page'], string> = {
  home: 'Risk across every workspace',
  workspaces: 'Your registered projects',
  search: 'Findings across all scans',
  tools: 'Analyzer status and installs',
  rules: 'Your custom YAML rules',
  cli: 'Command-line reference',
  settings: 'App preferences',
  about: 'Version, updates and privacy',
  workspace: 'Workspace overview',
  files: 'Workspace files and rules',
  history: 'Scan history',
  scan: 'One scan in detail',
  pentest: 'Dynamic web probes',
  'not-found': 'Page not found',
};

export function AppShell({ workspace, route, onNavigate, onClose, theme, onToggleTheme, onShowShortcuts, seqArmed = false }: { workspace?: Workspace; route: Route; onNavigate: (route: Route) => void; onAdd?: () => void; onClose: () => void; theme: Theme; onToggleTheme: () => void; onShowShortcuts?: () => void; seqArmed?: boolean }) {
  const { t, locale, setLocale } = useT();
  const reduced = useReducedMotion();
  const [mobileOpen, setMobileOpen] = React.useState(false);

  const items = (pages: ReadonlyArray<Route['page']>): NavItem[] =>
    pages.map((page) => ({
      route: { page } as Route,
      label: t(`nav.${page}` as never),
      icon: NAV_ICONS[page]!,
      hint: NAV_HINTS[page],
    }));

  // Workspace-scoped pages appear in the rail only while inside a workspace,
  // under its name. `route.id` is the workspace id on exactly these routes.
  const inWorkspace = ['workspace', 'files', 'history', 'pentest'].includes(route.page) && 'id' in route;
  const workspaceId = workspace?.id ?? (inWorkspace ? route.id : undefined);
  const workspaceNav = workspaceId ? workspaceSections(workspaceId) : [];

  const link = ({ route: next, label, icon: Icon, hint }: NavItem) => {
    const active = route.page === next.page;
    return (
      <a
        key={next.page}
        href={href(next)}
        className={cn('rail-link', active && 'active')}
        aria-current={active ? 'page' : undefined}
        title={hint}
        onClick={(event) => navigateFromLink(event, () => onNavigate(next))}
      >
        <Icon className="rail-link-icon" aria-hidden="true" />
        <span className="rail-link-label">{label}</span>
      </a>
    );
  };

  return (
    <>
      <aside className={cn('app-rail', reduced && 'nav-no-motion')}>
        <a className="rail-brand" href={href({ page: 'home' })} onClick={(event) => navigateFromLink(event, () => onNavigate({ page: 'home' }))}>
          <svg className="brand-mark" viewBox="0 0 32 32" aria-hidden="true" focusable="false">
            <rect width="32" height="32" rx="8" fill="var(--color-brand-mark)" />
            <path d="M16 5.2 L23.6 9 L23.6 17.2 C23.6 21 20.2 24.5 16 26.8 C11.8 24.5 8.4 21 8.4 17.2 L8.4 9 Z" fill="none" stroke="var(--color-paper)" strokeOpacity="0.14" strokeWidth="1" strokeLinejoin="round" />
            <path d="M11.8 15.9 L15.2 19.1 L20.6 12.1" fill="none" stroke="var(--color-paper)" strokeWidth="2.7" strokeLinecap="round" strokeLinejoin="round" opacity="0.96" />
            <circle cx="23.4" cy="7.2" r="1.7" fill="var(--color-brand-accent)" />
          </svg>
          <b>Blunt Code</b>
        </a>

        <nav className="rail-nav" aria-label="Main navigation">
          <ul className="rail-group">{items(PRIMARY).map((item) => <li key={item.route.page}>{link(item)}</li>)}</ul>

          {workspaceNav.length > 0 && (
            <div className="rail-workspace">
              <p className="rail-group-label">{workspace?.name ?? 'This workspace'}</p>
              <ul className="rail-group">
                {workspaceNav.map((section) => {
                  const active = isSameSection(route, section.route);
                  return (
                    <li key={section.key}>
                      <a
                        href={href(section.route)}
                        className={cn('rail-link', active && 'active')}
                        aria-current={active ? 'page' : undefined}
                        onClick={(event) => navigateFromLink(event, () => onNavigate(section.route))}
                      >
                        {/* section.icon is a ready-made ReactNode (WorkspaceContext
                            owns those icons), not a component reference. */}
                        <span className="rail-link-icon" aria-hidden="true">{section.icon}</span>
                        <span className="rail-link-label">{section.label}</span>
                      </a>
                    </li>
                  );
                })}
              </ul>
            </div>
          )}

          <ul className="rail-group rail-group-secondary">{items(SECONDARY).map((item) => <li key={item.route.page}>{link(item)}</li>)}</ul>
        </nav>

        <div className="rail-foot">
          {/* One search affordance, and it is the thing the palette actually
              does. The old header advertised it as a "Ctrl K" pill — a keyboard
              shortcut badge worn as chrome, which is a template tell.

              It moves HERE, out of the navigation list. This rail is read
              positionally: the user's eye lands in the PRIMARY group (Home,
              Workspaces, Search, Tools) and reads down. Parking a text input
              ~500px above the list's midpoint put a *destination* control in
              the same column as *places*, which is why the rail read as two
              competing things. At the foot it sits with the other utilities,
              below every destination, where a shortcut affordance belongs. */}
          <button
            type="button"
            className="rail-search"
            onClick={() => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', ctrlKey: true, bubbles: true }))}
            title="Search and commands (Ctrl+K)"
          >
            <Search className="rail-search-icon" aria-hidden="true" />
            <span>{t('nav.search')}</span>
            <kbd className="kbd-hint">Ctrl K</kbd>
          </button>

          {/* Settings and About only. The theme toggle used to sit in this list
              as a full-width row with a text label — visually identical to a
              nav destination, so a twice-a-session preference got the same
              weight as Settings. It is now an icon button in the utility
              cluster below, which is what it is: a preference, not a place. */}
          <ul className="rail-group">
            {items(SETTINGS_PAGES).map(link)}
          </ul>

          <div className="rail-util">
            <button
              type="button"
              className="rail-util-btn theme-toggle"
              onClick={onToggleTheme}
              aria-pressed={theme === 'dark'}
              aria-label={theme === 'dark' ? t('common.switchToLight') : t('common.switchToDark')}
              title={theme === 'dark' ? t('common.switchToLight') : t('common.switchToDark')}
            >
              {theme === 'dark' ? <Sun className="h-4 w-4" aria-hidden="true" /> : <Moon className="h-4 w-4" aria-hidden="true" />}
            </button>
            <NotificationsCenter routeKey={href(route)} />
            {seqArmed && <span className="seq-hint" aria-hidden="true">g…</span>}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon" className="rail-util-btn" aria-label="More options" title="More options">
                  <MoreDots />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="min-w-[13rem] p-1">
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
                      <DropdownMenuItem key={l.value} onSelect={() => setLocale(l.value as never)} className="flex items-center justify-between gap-2 cursor-pointer">
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
                <DropdownMenuItem onSelect={onClose} className="nav-danger-item gap-2 cursor-pointer text-[var(--color-danger)] focus:text-[var(--color-danger)]">
                  <Power className="h-4 w-4" aria-hidden="true" />
                  <span className="font-medium">{t('common.closeApp')}</span>
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </aside>

      {/* Mobile bar: the rail collapses below 64rem, and the nav has to go
          somewhere. Same links, horizontal, scrollable. */}
      <header className={cn('app-topbar', reduced && 'nav-no-motion')}>
        <a className="brand" href={href({ page: 'home' })} onClick={(event) => navigateFromLink(event, () => onNavigate({ page: 'home' }))}>
          <svg className="brand-mark" viewBox="0 0 32 32" aria-hidden="true" focusable="false">
            <rect width="32" height="32" rx="8" fill="var(--color-brand-mark)" />
            <path d="M16 5.2 L23.6 9 L23.6 17.2 C23.6 21 20.2 24.5 16 26.8 C11.8 24.5 8.4 21 8.4 17.2 L8.4 9 Z" fill="none" stroke="var(--color-paper)" strokeOpacity="0.14" strokeWidth="1" strokeLinejoin="round" />
            <path d="M11.8 15.9 L15.2 19.1 L20.6 12.1" fill="none" stroke="var(--color-paper)" strokeWidth="2.7" strokeLinecap="round" strokeLinejoin="round" opacity="0.96" />
            <circle cx="23.4" cy="7.2" r="1.7" fill="var(--color-brand-accent)" />
          </svg>
          <b>Blunt Code</b>
        </a>
        <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
          <SheetTrigger asChild><Button variant="outline" size="sm" aria-label="Open navigation">Menu</Button></SheetTrigger>
          <SheetContent side="left" className="w-72 bg-[var(--color-surface)]">
            <SheetTitle>Blunt Code</SheetTitle>
            <SheetDescription>Pages and workspace sections</SheetDescription>
            <nav aria-label="Mobile navigation" className="mt-6" onClick={(event) => { if ((event.target as Element).closest('a')) setMobileOpen(false); }}>
              <ul className="rail-group">{items([...PRIMARY, ...SECONDARY, ...SETTINGS_PAGES]).map((item) => <li key={item.route.page}>{link(item)}</li>)}</ul>
              {workspaceNav.length > 0 && <><p className="rail-group-label mt-6">{workspace?.name ?? 'This workspace'}</p><ul>{workspaceNav.map((section) => <li key={section.key}><a className="rail-link" href={href(section.route)} onClick={(e) => navigateFromLink(e, () => onNavigate(section.route))}>{section.label}</a></li>)}</ul></>}
            </nav>
          </SheetContent>
        </Sheet>
        <div className="nav-actions">
          <Button variant="ghost" size="icon" className="rail-util-btn" onClick={onToggleTheme} aria-pressed={theme === 'dark'} aria-label={theme === 'dark' ? t('common.switchToLight') : t('common.switchToDark')} title={theme === 'dark' ? t('common.switchToLight') : t('common.switchToDark')}>
            {theme === 'dark' ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </Button>
        </div>
      </header>
    </>
  );
}

function MoreDots() {
  return (
    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
      <circle cx="12" cy="5" r="1" fill="currentColor" />
      <circle cx="12" cy="12" r="1" fill="currentColor" />
      <circle cx="12" cy="19" r="1" fill="currentColor" />
    </svg>
  );
}

function isSameSection(a: Route, b: Route): boolean {
  return a.page === b.page && a.id === b.id && ['workspace', 'files', 'history', 'pentest'].includes(a.page);
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

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { api } from './api';
import type { Workspace } from './types';
import { href, parseRoute, type Route } from './lib/router';
import { isTextEntryTarget, parseShortcut } from './lib/shortcuts';
import type { Notice } from './lib/notice';
import { message } from './lib/notice';
import { AppShell, AppFooter } from './components/AppShell';
import { ErrorBoundary } from './components/ErrorBoundary';
import { ToastStack, useToasts } from './components/toasts';
import { AddWorkspaceDialog, ConfirmationDialog } from './components/dialogs';
import { CommandPalette, type Command } from './components/CommandPalette';
import { ShortcutsDialog } from './components/ShortcutsDialog';
import { useTheme } from './hooks/useTheme';
import { I18nProvider } from './lib/i18n';
import { HomePage } from './pages/HomePage';
import { WorkspacesPage } from './pages/WorkspacesPage';
import { WorkspacePage } from './pages/WorkspaceDetailPage';
import { FilesPage } from './pages/FilesPage';
import { HistoryPage } from './pages/HistoryPage';
import { ScanPage } from './pages/ScanPage';
import { SearchPage } from './pages/SearchPage';
import { ToolsPage } from './pages/ToolsPage';
import { PentestPage } from './pages/PentestPage';
import { SettingsPage } from './pages/SettingsPage';
import { AboutPage } from './pages/AboutPage';
import { NotFoundPage } from './pages/NotFoundPage';

/** Keys that complete a "g" navigation sequence: g h/w/t/s -> the matching page. */
const SEQUENCE_TARGETS: Partial<Record<string, Route['page']>> = { h: 'home', w: 'workspaces', t: 'tools', s: 'settings', a: 'about' };
/** How long "g" stays armed waiting for its second key. */
const SEQUENCE_ARM_MS = 800;

export function App() {
  const [route, setRoute] = useState(parseRoute);
  const [addOpen, setAddOpen] = useState(false);
  const [closeOpen, setCloseOpen] = useState(false);
  const [closing, setClosing] = useState(false);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [seqArmed, setSeqArmed] = useState(false);
  const seqTimer = useRef(0);
  const { toasts, notify, dismiss } = useToasts();
  const { theme, toggleTheme } = useTheme();
  const go = useCallback((next: Route) => { window.history.pushState({}, '', href(next)); setRoute(next); }, []);
  const closeApp = async () => { setClosing(true); try { await api.stopServer(); setCloseOpen(false); notify({ kind: 'info', text: 'Blunt Code is closing. You can close this tab.' }); } catch (error) { notify({ kind: 'error', text: message(error) }); setClosing(false); } };
  useEffect(() => { const handler = () => setRoute(parseRoute()); window.addEventListener('popstate', handler); return () => window.removeEventListener('popstate', handler); }, []);
  const disarmSequence = useCallback(() => { if (seqTimer.current) { window.clearTimeout(seqTimer.current); seqTimer.current = 0; } setSeqArmed(false); }, []);
  const armSequence = useCallback(() => { if (seqTimer.current) window.clearTimeout(seqTimer.current); seqTimer.current = window.setTimeout(() => { seqTimer.current = 0; setSeqArmed(false); }, SEQUENCE_ARM_MS); setSeqArmed(true); }, []);
  useEffect(() => () => { if (seqTimer.current) window.clearTimeout(seqTimer.current); }, []);
  // Ctrl/Cmd+K opens the command palette from anywhere — including inside text
  // fields and over other dialogs — so it gets its own listener ahead of (and
  // independent of) the single-key shortcut machinery below.
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (!(event.ctrlKey || event.metaKey) || event.altKey) return;
      if (event.key.toLowerCase() !== 'k') return;
      event.preventDefault();
      setPaletteOpen((open) => !open);
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, []);
  // Global shortcuts: sequence navigation, add-workspace, focus search, help dialog.
  // Keystrokes aimed at a field, already handled elsewhere (defaultPrevented), or
  // owned by an open dialog are left alone; the shortcuts help itself stays live
  // so Escape (via its dialog hook) and the other keys keep working over it.
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (isTextEntryTarget(event.target)) return;
      if (event.defaultPrevented) return;
      if (document.querySelector('dialog[open]:not([data-shortcuts-dialog])')) return;
      const wasArmed = seqTimer.current !== 0;
      if (wasArmed) disarmSequence();
      const key = parseShortcut(event);
      if (!key) return;
      if (key === 'g') { event.preventDefault(); armSequence(); return; }
      const target = wasArmed ? SEQUENCE_TARGETS[key] : undefined;
      if (target) { event.preventDefault(); go({ page: target }); return; }
      if (key === 'n') { event.preventDefault(); setAddOpen(true); return; }
      if (key === '/') {
        const search = document.querySelector<HTMLInputElement>('.filter-search input') ?? document.querySelector<HTMLInputElement>('.tree-panel .search input');
        if (search) { event.preventDefault(); search.focus(); }
        return;
      }
      if (key === '?') { event.preventDefault(); setShortcutsOpen(true); }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [go, armSequence, disarmSequence]);
  const paletteCommands: Command[] = [
    { id: 'nav-home', label: 'Go to Home', keywords: 'dashboard start', hint: 'g h', group: 'Navigation', run: () => go({ page: 'home' }) },
    { id: 'nav-workspaces', label: 'Go to Workspaces', keywords: 'projects', hint: 'g w', group: 'Navigation', run: () => go({ page: 'workspaces' }) },
    { id: 'nav-tools', label: 'Go to Tools', keywords: 'analyzers ruff biome semgrep sonar', hint: 'g t', group: 'Navigation', run: () => go({ page: 'tools' }) },
    { id: 'nav-settings', label: 'Go to Settings', keywords: 'preferences', hint: 'g s', group: 'Navigation', run: () => go({ page: 'settings' }) },
    { id: 'nav-about', label: 'Go to About', keywords: 'version info', hint: 'g a', group: 'Navigation', run: () => go({ page: 'about' }) },
    { id: 'nav-search', label: 'Go to findings', keywords: 'search findings', group: 'Navigation', run: () => go({ page: 'search' }) },
    { id: 'nav-pentest', label: 'Go to Pentest', keywords: 'pentest zap nuclei burp hacking', group: 'Navigation', run: () => go({ page: 'pentest' }) },
    { id: 'filter-critical', label: 'Filter critical', keywords: 'severity critical', group: 'Filters', run: () => go({ page: 'search' }) },
    { id: 'filter-findings', label: 'Go to findings', keywords: 'findings', group: 'Filters', run: () => go({ page: 'search' }) },
    { id: 'action-add-workspace', label: 'Add workspace', keywords: 'new project folder scan', hint: 'N', group: 'Navigation', run: () => setAddOpen(true) },
    { id: 'action-theme', label: theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme', keywords: 'dark light mode appearance', group: 'Navigation', run: toggleTheme },
    { id: 'action-shortcuts', label: 'Show keyboard shortcuts', keywords: 'help keys', hint: '?', group: 'Navigation', run: () => setShortcutsOpen(true) },
  ];
  // Dynamic workspace entries load lazily the first time the palette opens;
  // failures degrade to the static list silently (the palette is navigation
  // sugar, not a report), and no request fires on routes that never open it.
  const [workspacesForPalette, setWorkspacesForPalette] = useState<Workspace[]>([]);
  useEffect(() => {
    if (!paletteOpen || workspacesForPalette.length) return;
    let cancelled = false;
    api.workspaces().then((items) => { if (!cancelled) setWorkspacesForPalette(items); }).catch(() => { });
    return () => { cancelled = true; };
  }, [paletteOpen, workspacesForPalette.length]);
  const workspaceCommands: Command[] = workspacesForPalette.map((workspace) => ({
    id: `nav-workspace-${workspace.id}`,
    label: `Go to ${workspace.name || 'Untitled workspace'}`,
    keywords: `${workspace.name} project open`,
    group: 'Workspaces',
    run: () => go({ page: 'workspace', id: workspace.id }),
  }));
  const allPaletteCommands = useMemo(() => [...paletteCommands, ...workspaceCommands], [paletteCommands, workspaceCommands]);
  return <I18nProvider><div className="app-frame">
    <a href="#main-content" className="skip-link">Skip to main content</a>
    <AppShell route={route} onNavigate={go} onAdd={() => setAddOpen(true)} onClose={() => setCloseOpen(true)} theme={theme} onToggleTheme={toggleTheme} onShowShortcuts={() => setShortcutsOpen(true)} seqArmed={seqArmed} />
    <main className="main" id="main-content" tabIndex={-1}>
      <ErrorBoundary resetKey={href(route)}>
        <Page route={route} go={go} notify={notify} onAdd={() => setAddOpen(true)} />
      </ErrorBoundary>
    </main>
    {addOpen && <AddWorkspaceDialog onClose={() => setAddOpen(false)} onCreated={(workspace) => { setAddOpen(false); go({ page: 'workspace', id: workspace.id }); }} notify={notify} />}
    {closeOpen && <ConfirmationDialog title="Close Blunt Code?" description="This ends the local app. Any active scan will be cancelled; your workspaces and reports stay saved on this computer." confirmLabel="Close app" busy={closing} onCancel={() => setCloseOpen(false)} onConfirm={() => void closeApp()} />}
    {shortcutsOpen && <ShortcutsDialog onClose={() => setShortcutsOpen(false)} />}
    {/* Mounted only while open so useDialogA11y's mount-time focus move/restore actually runs — an always-mounted palette never receives keyboard focus. */}
    {paletteOpen && <CommandPalette open onClose={() => setPaletteOpen(false)} commands={allPaletteCommands} note={workspacesForPalette.length ? `${workspacesForPalette.length} workspace${workspacesForPalette.length === 1 ? '' : 's'} indexed` : undefined} />}
    <AppFooter />
    <ToastStack toasts={toasts} onDismiss={dismiss} />
  </div></I18nProvider>;
}

function Page({ route, go, notify, onAdd }: { route: Route; go: (r: Route) => void; notify: (n: Notice) => void; onAdd: () => void }) {
  // parseRoute guarantees these routes always carry an id; the fallback keeps
  // the type honest if a malformed URL ever slips through.
  const id = 'id' in route ? route.id : undefined;
  switch (route.page) {
    case 'home': return <HomePage go={go} onAdd={onAdd} notify={notify} />;
    case 'workspaces': return <WorkspacesPage go={go} onAdd={onAdd} notify={notify} />;
    case 'workspace': return id ? <WorkspacePage id={id} go={go} notify={notify} /> : <NotFoundPage go={go} />;
    case 'files': return id ? <FilesPage id={id} go={go} notify={notify} /> : <NotFoundPage go={go} />;
    case 'history': return id ? <HistoryPage workspaceId={id} go={go} /> : <NotFoundPage go={go} />;
    case 'scan': return id ? <ScanPage id={id} notify={notify} /> : <NotFoundPage go={go} />;
    case 'search': return <SearchPage go={go} />;
    case 'tools': return <ToolsPage notify={notify} />;
    case 'pentest': return <PentestPage go={go} />;
    case 'settings': return <SettingsPage notify={notify} />;
    case 'about': return <AboutPage />;
    case 'not-found': return <NotFoundPage go={go} />;
  }
}

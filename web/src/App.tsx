import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from './api';
import { href, parseRoute, type Route } from './lib/router';
import { isTextEntryTarget, parseShortcut } from './lib/shortcuts';
import type { Notice } from './lib/notice';
import { message } from './lib/notice';
import { AppShell, AppFooter } from './components/AppShell';
import { ErrorBoundary } from './components/ErrorBoundary';
import { ToastStack, useToasts } from './components/toasts';
import { AddWorkspaceDialog, ConfirmationDialog } from './components/dialogs';
import { ShortcutsDialog } from './components/ShortcutsDialog';
import { useTheme } from './hooks/useTheme';
import { HomePage } from './pages/HomePage';
import { WorkspacesPage } from './pages/WorkspacesPage';
import { WorkspacePage } from './pages/WorkspaceDetailPage';
import { FilesPage } from './pages/FilesPage';
import { HistoryPage } from './pages/HistoryPage';
import { ScanPage } from './pages/ScanPage';
import { ToolsPage } from './pages/ToolsPage';
import { SettingsPage } from './pages/SettingsPage';
import { AboutPage } from './pages/AboutPage';
import { NotFoundPage } from './pages/NotFoundPage';

/** Keys that complete a "g" navigation sequence: g h/w/t/s -> the matching page. */
const SEQUENCE_TARGETS: Partial<Record<string, Route['page']>> = { h: 'home', w: 'workspaces', t: 'tools', s: 'settings' };
/** How long "g" stays armed waiting for its second key. */
const SEQUENCE_ARM_MS = 800;

export function App() {
  const [route, setRoute] = useState(parseRoute);
  const [addOpen, setAddOpen] = useState(false);
  const [closeOpen, setCloseOpen] = useState(false);
  const [closing, setClosing] = useState(false);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
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
  return <div className="app-frame">
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
    <AppFooter />
    <ToastStack toasts={toasts} onDismiss={dismiss} />
  </div>;
}

function Page({ route, go, notify, onAdd }: { route: Route; go: (r: Route) => void; notify: (n: Notice) => void; onAdd: () => void }) {
  // parseRoute guarantees these routes always carry an id; the fallback keeps
  // the type honest if a malformed URL ever slips through.
  const id = 'id' in route ? route.id : undefined;
  switch (route.page) {
    case 'home': return <HomePage go={go} onAdd={onAdd} notify={notify} />;
    case 'workspaces': return <WorkspacesPage go={go} onAdd={onAdd} notify={notify} />;
    case 'workspace': return id ? <WorkspacePage id={id} go={go} notify={notify} /> : <NotFoundPage go={go} />;
    case 'files': return id ? <FilesPage id={id} notify={notify} /> : <NotFoundPage go={go} />;
    case 'history': return id ? <HistoryPage workspaceId={id} go={go} /> : <NotFoundPage go={go} />;
    case 'scan': return id ? <ScanPage id={id} notify={notify} /> : <NotFoundPage go={go} />;
    case 'tools': return <ToolsPage notify={notify} />;
    case 'settings': return <SettingsPage notify={notify} />;
    case 'about': return <AboutPage />;
    case 'not-found': return <NotFoundPage go={go} />;
  }
}

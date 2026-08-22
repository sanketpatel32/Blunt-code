import { useCallback, useEffect, useState } from 'react';
import { api } from './api';
import { href, parseRoute, type Route } from './lib/router';
import type { Notice } from './lib/notice';
import { message } from './lib/notice';
import { AppShell, AppFooter } from './components/AppShell';
import { ErrorBoundary } from './components/ErrorBoundary';
import { ToastStack, useToasts } from './components/toasts';
import { AddWorkspaceDialog, ConfirmationDialog } from './components/dialogs';
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

export function App() {
  const [route, setRoute] = useState(parseRoute);
  const [addOpen, setAddOpen] = useState(false);
  const [closeOpen, setCloseOpen] = useState(false);
  const [closing, setClosing] = useState(false);
  const { toasts, notify, dismiss } = useToasts();
  const { theme, toggleTheme } = useTheme();
  const go = useCallback((next: Route) => { window.history.pushState({}, '', href(next)); setRoute(next); }, []);
  const closeApp = async () => { setClosing(true); try { await api.stopServer(); setCloseOpen(false); notify({ kind: 'info', text: 'Blunt Code is closing. You can close this tab.' }); } catch (error) { notify({ kind: 'error', text: message(error) }); setClosing(false); } };
  useEffect(() => { const handler = () => setRoute(parseRoute()); window.addEventListener('popstate', handler); return () => window.removeEventListener('popstate', handler); }, []);
  return <div className="app-frame">
    <a href="#main-content" className="skip-link">Skip to main content</a>
    <AppShell route={route} onNavigate={go} onAdd={() => setAddOpen(true)} onClose={() => setCloseOpen(true)} theme={theme} onToggleTheme={toggleTheme} />
    <main className="main" id="main-content">
      <ErrorBoundary resetKey={href(route)}>
        <Page route={route} go={go} notify={notify} onAdd={() => setAddOpen(true)} />
      </ErrorBoundary>
    </main>
    {addOpen && <AddWorkspaceDialog onClose={() => setAddOpen(false)} onCreated={(workspace) => { setAddOpen(false); go({ page: 'workspace', id: workspace.id }); }} notify={notify} />}
    {closeOpen && <ConfirmationDialog title="Close Blunt Code?" description="This ends the local app. Any active scan will be cancelled; your workspaces and reports stay saved on this computer." confirmLabel="Close app" busy={closing} onCancel={() => setCloseOpen(false)} onConfirm={() => void closeApp()} />}
    <AppFooter />
    <ToastStack toasts={toasts} onDismiss={dismiss} />
  </div>;
}

function Page({ route, go, notify, onAdd }: { route: Route; go: (r: Route) => void; notify: (n: Notice) => void; onAdd: () => void }) {
  switch (route.page) {
    case 'home': return <HomePage go={go} onAdd={onAdd} notify={notify} />;
    case 'workspaces': return <WorkspacesPage go={go} onAdd={onAdd} notify={notify} />;
    case 'workspace': return <WorkspacePage id={route.id!} go={go} notify={notify} />;
    case 'files': return <FilesPage id={route.id!} notify={notify} />;
    case 'history': return <HistoryPage workspaceId={route.id!} go={go} />;
    case 'scan': return <ScanPage id={route.id!} go={go} notify={notify} />;
    case 'tools': return <ToolsPage notify={notify} />;
    case 'settings': return <SettingsPage notify={notify} />;
    case 'about': return <AboutPage />;
    case 'not-found': return <NotFoundPage go={go} />;
  }
}

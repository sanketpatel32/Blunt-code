import type { ReactNode } from 'react';
import type { Route } from '../lib/router';
import { LayoutDashboard, FolderCog, Clock, ShieldAlert } from 'lucide-react';

export type WorkspaceSection = { key: string; label: string; route: Route; icon?: ReactNode; count?: number };

export function workspaceSections(id: string): WorkspaceSection[] {
  return [
    { key: 'overview', label: 'Overview', route: { page: 'workspace', id }, icon: <LayoutDashboard className="h-4 w-4" /> },
    { key: 'pentest', label: 'Pentest & Security', route: { page: 'pentest', id }, icon: <ShieldAlert className="h-4 w-4" /> },
    { key: 'files', label: 'Files & rules', route: { page: 'files', id }, icon: <FolderCog className="h-4 w-4" /> },
    { key: 'history', label: 'Scan history', route: { page: 'history', id }, icon: <Clock className="h-4 w-4" /> },
  ];
}

export function WorkspaceContextSidebar({ id, current, onNavigate }: { id: string; current: Route; onNavigate: (route: Route) => void }) {
  const sections = workspaceSections(id);
  return <nav className="workspace-context" aria-label="Workspace sections">
    <p className="eyebrow">In this workspace</p>
    <ul>{sections.map((section) => {
      const active = isSameSection(current, section.route);
      return <li key={section.key}>
        <button type="button" className={`workspace-context-link${active ? ' active' : ''}`} aria-current={active ? 'page' : undefined} onClick={() => onNavigate(section.route)}><span aria-hidden="true" className="workspace-context-icon">{section.icon}</span><span className="workspace-context-label">{section.label}</span>{section.count !== undefined && <span className="workspace-context-hint">{section.count}</span>}</button>
      </li>;
    })}</ul>
  </nav>;
}

function isSameSection(a: Route, b: Route): boolean {
  return a.page === b.page && a.id === b.id && ['workspace', 'files', 'history', 'pentest'].includes(a.page);
}

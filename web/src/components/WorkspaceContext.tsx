import type { ReactNode } from 'react';
import type { Route } from '../lib/router';

export type WorkspaceSection = { key: string; label: string; route: Route; icon?: ReactNode };

/** Section definitions for the workspace sidebar: the sub-pages that exist for
 *  a single workspace. All routes share the same workspace `id`. */
export function workspaceSections(id: string): WorkspaceSection[] {
  return [
    { key: 'overview', label: 'Overview', route: { page: 'workspace', id } },
    { key: 'files', label: 'Files & rules', route: { page: 'files', id } },
    { key: 'history', label: 'Scan history', route: { page: 'history', id } },
  ];
}

export function WorkspaceContextSidebar({ id, current, onNavigate }: { id: string; current: Route; onNavigate: (route: Route) => void }) {
  const sections = workspaceSections(id);
  return <nav className="workspace-context" aria-label="Workspace sections">
    <p className="eyebrow">In this workspace</p>
    <ul>{sections.map((section) => {
      const active = isSameSection(current, section.route);
      return <li key={section.key}>
        <button type="button" className={`workspace-context-link${active ? ' active' : ''}`} aria-current={active ? 'page' : undefined} onClick={() => onNavigate(section.route)}>{section.icon && <span aria-hidden="true" className="workspace-context-icon">{section.icon}</span>}<span className="workspace-context-label">{section.label}</span></button>
      </li>;
    })}</ul>
  </nav>;
}

function isSameSection(a: Route, b: Route): boolean {
  return a.page === b.page && a.id === b.id && ['workspace', 'files', 'history'].includes(a.page);
}

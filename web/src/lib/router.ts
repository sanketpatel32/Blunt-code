export type Route = { page: 'home' | 'workspaces' | 'workspace' | 'files' | 'history' | 'scan' | 'search' | 'tools' | 'pentest' | 'settings' | 'about' | 'not-found'; id?: string };

export function parseRoute(pathname = window.location.pathname): Route {
  const segments = pathname.split('/').filter(Boolean);
  if (!segments.length) return { page: 'home' };
  if (segments[0] === 'workspaces' && segments[2] === 'files') return { page: 'files', id: segments[1] };
  if (segments[0] === 'workspaces' && segments[2] === 'scans') return { page: 'history', id: segments[1] };
  if (segments[0] === 'workspaces' && segments[1]) return { page: 'workspace', id: segments[1] };
  if (segments[0] === 'workspaces') return { page: 'workspaces' };
  if (segments[0] === 'scans' && segments[1]) return { page: 'scan', id: segments[1] };
  if (segments[0] === 'tools' || segments[0] === 'pentest' || segments[0] === 'settings' || segments[0] === 'about') return { page: segments[0] };
  if (segments[0] === 'findings') return { page: 'search' };
  return { page: 'not-found' };
}

export function href(route: Route) {
  if (route.page === 'home') return '/';
  if (route.page === 'workspaces') return '/workspaces';
  if (route.page === 'workspace') return `/workspaces/${route.id}`;
  if (route.page === 'files') return `/workspaces/${route.id}/files`;
  if (route.page === 'history') return `/workspaces/${route.id}/scans`;
  if (route.page === 'scan') return `/scans/${route.id}`;
  return `/${route.page}`;
}

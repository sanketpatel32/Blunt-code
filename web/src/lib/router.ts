export type Route = { page: 'home' | 'workspaces' | 'workspace' | 'files' | 'history' | 'scan' | 'search' | 'tools' | 'pentest' | 'rules' | 'settings' | 'about' | 'cli' | 'not-found'; id?: string; q?: string };

/**
 * One path segment, percent-decoded. The browser hands us the still-encoded
 * form ("/scans/%E4%B8%AD%E6%96%87"); passing it on verbatim double-encodes
 * every API call ("%25E4…"). Malformed escapes (lone "%" sequences) fall back
 * to the raw segment so a hostile URL stays inert data instead of throwing.
 */
function decodeSegment(segment: string): string {
  try { return decodeURIComponent(segment); } catch { return segment; }
}

export function parseRoute(pathname = window.location.pathname, search = window.location.search): Route {
  // Raw query string without the leading '?', carried on every parsed route so
  // deep-linked filters survive a reload and back/forward navigation. Absent
  // when there is no search, so routes keep their exact historical shape.
  const rawQuery = search.startsWith('?') ? search.slice(1) : search;
  const withQuery = (route: Route): Route => (rawQuery ? { ...route, q: rawQuery } : route);
  const segments = pathname.split('/').filter(Boolean).map(decodeSegment);
  if (!segments.length) return withQuery({ page: 'home' });
  if (segments[0] === 'workspaces' && segments[2] === 'pentest') return withQuery({ page: 'pentest', id: segments[1] });
  if (segments[0] === 'workspaces' && segments[2] === 'files') return withQuery({ page: 'files', id: segments[1] });
  if (segments[0] === 'workspaces' && segments[2] === 'scans') return withQuery({ page: 'history', id: segments[1] });
  if (segments[0] === 'workspaces' && segments[1]) return withQuery({ page: 'workspace', id: segments[1] });
  if (segments[0] === 'workspaces') return withQuery({ page: 'workspaces' });
  if (segments[0] === 'scans' && segments[1]) return withQuery({ page: 'scan', id: segments[1] });
  if (segments[0] === 'tools' || segments[0] === 'pentest' || segments[0] === 'rules' || segments[0] === 'settings' || segments[0] === 'about' || segments[0] === 'cli' || segments[0] === 'search') return withQuery({ page: segments[0] });
  if (segments[0] === 'docs') return withQuery({ page: 'cli' });
  if (segments[0] === 'findings') return withQuery({ page: 'search' });
  return withQuery({ page: 'not-found' });
}

export function href(route: Route) {
  let path: string;
  if (route.page === 'home') path = '/';
  else if (route.page === 'workspaces') path = '/workspaces';
  else if (route.page === 'workspace') path = `/workspaces/${route.id}`;
  else if (route.page === 'files') path = `/workspaces/${route.id}/files`;
  else if (route.page === 'history') path = `/workspaces/${route.id}/scans`;
  else if (route.page === 'pentest' && route.id) path = `/workspaces/${route.id}/pentest`;
  else if (route.page === 'scan') path = `/scans/${route.id}`;
  else if (route.page === 'cli') path = '/cli';
  else path = `/${route.page}`;
  // Deep links carry their raw query ("lang=typescript", no leading '?').
  return route.q ? `${path}?${route.q}` : path;
}

/**
 * Signs the UI into the local API. The launcher opens `/#token=<per-launch
 * token>`; the fragment never reaches the server or any log, and is removed
 * from the address bar so it is not left in history, bookmarks, or
 * screenshots. The token is kept in localStorage, which is isolated per
 * origin including the port, and sent as `Authorization: Bearer`. A cookie
 * would be sent to every port on 127.0.0.1, i.e. to any other local server
 * the browser visits. Navigations that cannot carry a header (EventSource,
 * downloads, new tabs) use a single-use, path-bound ticket from `signedUrl`,
 * delivered as an HttpOnly cookie so the credential never touches a URL,
 * browser history, or a Referer header.
 */
export type SessionState = 'signed-in' | 'signed-out' | 'unknown';

const SESSION_URL = '/api/v1/session';
const TICKETS_URL = '/api/v1/session/tickets';
const TOKEN_KEY = 'token';
export const STORAGE_KEY = 'bluntcode.apiToken';

function storage(): Storage | null {
  try { return window.localStorage; } catch { return null; }
}

export function storedToken(): string | null {
  return storage()?.getItem(STORAGE_KEY) ?? null;
}

/** Headers that authenticate an API request (empty when not signed in). */
export function authHeaders(): Record<string, string> {
  const token = storedToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

/** Removes the token from the fragment, returning it (or null). */
export function takeFragmentToken(loc: Location = window.location, history: History = window.history): string | null {
  const hash = loc.hash.startsWith('#') ? loc.hash.slice(1) : loc.hash;
  if (!hash) return null;
  const params = new URLSearchParams(hash);
  const token = params.get(TOKEN_KEY);
  if (token === null) return null;
  params.delete(TOKEN_KEY);
  const rest = params.toString();
  history.replaceState(history.state, '', `${loc.pathname}${loc.search}${rest ? `#${rest}` : ''}`);
  return token.trim() || null;
}

export async function establishSession(fetcher: typeof fetch = fetch, loc: Location = window.location, history: History = window.history): Promise<SessionState> {
  const fresh = takeFragmentToken(loc, history);
  if (fresh) storage()?.setItem(STORAGE_KEY, fresh);
  const token = storedToken();
  if (!token) return 'signed-out';
  try {
    const check = await fetcher(SESSION_URL, { headers: { Authorization: `Bearer ${token}` } });
    if (check.ok) return 'signed-in';
    if (check.status !== 401) return 'unknown';
    // A token from an earlier launch; every launch mints a new one.
    storage()?.removeItem(STORAGE_KEY);
    return 'signed-out';
  } catch {
    // The server is unreachable; the app's own error states explain that.
    return 'unknown';
  }
}

/**
 * Mints a single-use, path-bound ticket cookie for `url` (an /api/v1 path,
 * optionally with a query) valid for about a minute, then returns the bare
 * URL — the cookie authenticates the navigation, so the credential never
 * appears in the URL. If minting fails the URL still comes back unsigned, so
 * the request fails visibly with 401 rather than silently.
 */
export async function signedUrl(url: string, fetcher: typeof fetch = fetch): Promise<string> {
  const path = url.split(/[?#]/, 1)[0]!;
  try {
    const response = await fetcher(TICKETS_URL, { method: 'POST', headers: { 'Content-Type': 'application/json', ...authHeaders() }, body: JSON.stringify({ path }) });
    if (!response.ok) return url;
  } catch {
    // The URL goes back unsigned and the navigation fails visibly.
  }
  return url;
}

/** Follows an API link (download or new tab) with a fresh ticket. */
export async function openSigned(url: string, mode: 'download' | 'tab' = 'download'): Promise<void> {
  const link = document.createElement('a');
  link.href = await signedUrl(url);
  link.rel = 'noopener noreferrer';
  if (mode === 'download') link.download = '';
  else link.target = '_blank';
  document.body.append(link);
  link.click();
  link.remove();
}

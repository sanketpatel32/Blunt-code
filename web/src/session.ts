/**
 * Signs the UI into the local API. The launcher opens `/#token=<per-launch
 * token>`; the fragment never reaches the server or any log. The token is
 * traded once for an HttpOnly, SameSite=Strict session cookie (so EventSource
 * streams and download links authenticate too), and removed from the address
 * bar so it is not left in history, bookmarks, or screenshots.
 */
export type SessionState = 'signed-in' | 'signed-out' | 'unknown';

const SESSION_URL = '/api/v1/session';
const TOKEN_KEY = 'token';

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
  const token = takeFragmentToken(loc, history);
  try {
    if (token) {
      const exchanged = await fetcher(SESSION_URL, { method: 'POST', headers: { Authorization: `Bearer ${token}` }, credentials: 'same-origin' });
      if (exchanged.ok) return 'signed-in';
    }
    const check = await fetcher(SESSION_URL, { credentials: 'same-origin' });
    if (check.ok) return 'signed-in';
    return check.status === 401 ? 'signed-out' : 'unknown';
  } catch {
    // The server is unreachable; the app's own error states explain that.
    return 'unknown';
  }
}

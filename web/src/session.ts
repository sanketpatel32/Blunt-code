/**
 * Signs the UI into the local API. The launcher opens `/#token=<per-launch
 * token>`; the fragment never reaches the server or any log, and is removed
 * from the address bar so it is not left in history, bookmarks, or
 * screenshots. The token is kept in localStorage, which is isolated per
 * origin including the port, and sent as `Authorization: Bearer`. A cookie
 * would be sent to every port on 127.0.0.1, i.e. to any other local server
 * the browser visits. Downloads and event streams therefore go through
 * `fetch` with the bearer header (`openSigned`, `streamEvents`) instead of
 * URL tickets or cookies — no credential ever touches a URL, browser
 * history, a Referer header, or an ambient cookie.
 */
export type SessionState = 'signed-in' | 'signed-out' | 'unknown';

const SESSION_URL = '/api/v1/session';
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

/** The server's scan-specific filename rides Content-Disposition; the URL tail is the fallback. */
function downloadName(response: Response, url: string): string {
  const disposition = response.headers.get('Content-Disposition') ?? '';
  const extended = /filename\*=UTF-8''([^;]+)/i.exec(disposition);
  if (extended) return decodeURIComponent(extended[1]);
  const plain = /filename="([^"]+)"/i.exec(disposition) ?? /filename=([^;\s]+)/i.exec(disposition);
  if (plain) return plain[1];
  return url.split('/').pop()?.split('?')[0] || 'download';
}

/**
 * Fetches `url` with the bearer token and delivers the body as a download or
 * a new tab via an object URL. Unlike an `<a href>` navigation, the request
 * carries `Authorization`, so no credential is exposed in a URL, cookie, or
 * Referer.
 */
export async function openSigned(url: string, mode: 'download' | 'tab' = 'download', fetcher: typeof fetch = fetch): Promise<void> {
  // The popup must open inside the click's permission window — a window.open
  // that waits for the fetch is treated as an unrequested pop-up and blocked.
  const tab = mode === 'tab' ? window.open('', '_blank') : null;
  const response = await fetcher(url, { headers: authHeaders() });
  if (!response.ok) {
    // Fail visibly: an unsigned navigation would 401 anyway, so surface it.
    if (tab) tab.location.href = url;
    else window.open(url, '_blank', 'noopener,noreferrer');
    return;
  }
  const objectUrl = URL.createObjectURL(await response.blob());
  if (tab) {
    // The server sandboxes inline reports (CSP `sandbox allow-scripts`); a
    // bare blob: navigation would run the report's script under this origin
    // where it can read the stored token, so the blob is re-wrapped in a
    // sandboxed iframe that forces an opaque origin again.
    tab.document.write(`<!doctype html><html><head><meta charset="utf-8"><title>Blunt Code report</title><style>html,body{margin:0;height:100%}iframe{border:0;width:100%;height:100%}</style></head><body><iframe sandbox="allow-scripts" src="${objectUrl}"></iframe></body></html>`);
    tab.document.close();
    // The new tab owns the blob now; revoking at timeout frees it once the
    // document has loaded without a dangling per-tab reference.
    window.setTimeout(() => URL.revokeObjectURL(objectUrl), 60_000);
    return;
  }
  // 'download' mode — or a 'tab' request whose popup was blocked: save the file.
  try {
    const link = document.createElement('a');
    link.href = objectUrl;
    link.download = downloadName(response, url);
    document.body.append(link);
    link.click();
    link.remove();
  } finally {
    URL.revokeObjectURL(objectUrl);
  }
}

/**
 * Reads a text/event-stream endpoint via fetch — unlike EventSource it can
 * carry the Authorization header, so live progress needs no URL or cookie
 * credential. Parses `event:`/`data:` frames and heartbeat comments
 * (`: ping`). Resolves when the server ends the stream; rejects on HTTP or
 * network failure and on abort (signal-triggered AbortError).
 */
export async function streamEvents(url: string, onEvent: (type: string, data: string) => void, signal: AbortSignal, fetcher: typeof fetch = fetch): Promise<void> {
  const response = await fetcher(url, { headers: authHeaders(), signal });
  if (!response.ok || !response.body) throw new Error(`events stream failed: ${response.status}`);
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  for (;;) {
    const { done, value } = await reader.read();
    if (done) return;
    buffer += decoder.decode(value, { stream: true });
    let cut: number;
    while ((cut = buffer.indexOf('\n\n')) >= 0) {
      const frame = buffer.slice(0, cut);
      buffer = buffer.slice(cut + 2);
      let type = 'message';
      const data: string[] = [];
      for (const line of frame.split('\n')) {
        if (line.startsWith('data:')) data.push(line.slice(5).replace(/^ /, ''));
        else if (line.startsWith('event:')) type = line.slice(6).trim();
      }
      if (data.length > 0) onEvent(type, data.join('\n'));
    }
  }
}

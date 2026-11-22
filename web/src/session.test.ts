import { afterEach, describe, expect, it, vi } from 'vitest';
import { authHeaders, establishSession, signedUrl, STORAGE_KEY, takeFragmentToken } from './session';

function fakeLocation(hash: string): Location {
  return { hash, pathname: '/', search: '' } as Location;
}

function fakeHistory() {
  return { state: null, replaceState: vi.fn() } as unknown as History & { replaceState: ReturnType<typeof vi.fn> };
}

afterEach(() => { window.localStorage.clear(); });

describe('takeFragmentToken', () => {
  it('returns the token and strips it from the address bar', () => {
    const history = fakeHistory();
    expect(takeFragmentToken(fakeLocation('#token=abc'), history)).toBe('abc');
    expect(history.replaceState).toHaveBeenCalledWith(null, '', '/');
  });

  it('keeps unrelated fragment parameters', () => {
    const history = fakeHistory();
    expect(takeFragmentToken(fakeLocation('#token=abc&tab=x'), history)).toBe('abc');
    expect(history.replaceState).toHaveBeenCalledWith(null, '', '/#tab=x');
  });

  it('leaves fragments without a token alone', () => {
    const history = fakeHistory();
    expect(takeFragmentToken(fakeLocation('#tab=x'), history)).toBeNull();
    expect(history.replaceState).not.toHaveBeenCalled();
  });
});

describe('establishSession', () => {
  it('stores the launcher token and verifies it as a bearer', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    expect(await establishSession(fetcher, fakeLocation('#token=abc'), fakeHistory())).toBe('signed-in');
    expect(fetcher).toHaveBeenCalledWith('/api/v1/session', { headers: { Authorization: 'Bearer abc' } });
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe('abc');
    expect(authHeaders()).toEqual({ Authorization: 'Bearer abc' });
  });

  it('reuses a stored token when the URL has none', async () => {
    window.localStorage.setItem(STORAGE_KEY, 'kept');
    const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    expect(await establishSession(fetcher, fakeLocation(''), fakeHistory())).toBe('signed-in');
    expect(fetcher).toHaveBeenCalledWith('/api/v1/session', { headers: { Authorization: 'Bearer kept' } });
  });

  it('is signed out without any token, without calling the API', async () => {
    const fetcher = vi.fn();
    expect(await establishSession(fetcher, fakeLocation(''), fakeHistory())).toBe('signed-out');
    expect(fetcher).not.toHaveBeenCalled();
  });

  it('forgets a token the server refuses', async () => {
    window.localStorage.setItem(STORAGE_KEY, 'stale');
    const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 401 }));
    expect(await establishSession(fetcher, fakeLocation(''), fakeHistory())).toBe('signed-out');
    expect(window.localStorage.getItem(STORAGE_KEY)).toBeNull();
  });

  it('reports unknown when the server is unreachable', async () => {
    window.localStorage.setItem(STORAGE_KEY, 'kept');
    const fetcher = vi.fn().mockRejectedValue(new TypeError('offline'));
    expect(await establishSession(fetcher, fakeLocation(''), fakeHistory())).toBe('unknown');
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe('kept');
  });
});

describe('signedUrl', () => {
  it('mints a ticket cookie for the path and returns the bare URL', async () => {
    window.localStorage.setItem(STORAGE_KEY, 'tok');
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({ ticket: 't/1', expires_in: 60 }), { status: 200 }));
    expect(await signedUrl('/api/v1/scans/s/findings.csv?severity=high', fetcher)).toBe('/api/v1/scans/s/findings.csv?severity=high');
    const [url, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/v1/session/tickets');
    expect(init.method).toBe('POST');
    expect(init.headers).toEqual({ 'Content-Type': 'application/json', Authorization: 'Bearer tok' });
    expect(JSON.parse(init.body as string)).toEqual({ path: '/api/v1/scans/s/findings.csv' });
  });

  it('falls back to the bare URL when minting fails', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 401 }));
    expect(await signedUrl('/api/v1/scans/s/events', fetcher)).toBe('/api/v1/scans/s/events');
  });
});

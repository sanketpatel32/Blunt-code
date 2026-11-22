import { describe, expect, it, vi } from 'vitest';
import { establishSession, takeFragmentToken } from './session';

function response(status: number) {
  return { ok: status >= 200 && status < 300, status } as Response;
}

describe('session bootstrap', () => {
  it('strips the token from the address bar and keeps other state', () => {
    window.history.replaceState(null, '', '/scans/abc?finding=1#token=secret-value&tab=files');
    expect(takeFragmentToken()).toBe('secret-value');
    expect(window.location.pathname + window.location.search + window.location.hash).toBe('/scans/abc?finding=1#tab=files');
    expect(window.location.href).not.toContain('secret-value');
  });

  it('leaves unrelated fragments alone', () => {
    window.history.replaceState(null, '', '/cli#main-content');
    expect(takeFragmentToken()).toBeNull();
    expect(window.location.hash).toBe('#main-content');
  });

  it('trades a launcher token for the session cookie', async () => {
    window.history.replaceState(null, '', '/#token=abc123');
    const fetcher = vi.fn(async () => response(204));
    await expect(establishSession(fetcher as unknown as typeof fetch)).resolves.toBe('signed-in');
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(fetcher).toHaveBeenCalledWith('/api/v1/session', expect.objectContaining({ method: 'POST', headers: { Authorization: 'Bearer abc123' } }));
    expect(window.location.hash).toBe('');
  });

  it('falls back to an existing cookie, and reports signed-out on 401', async () => {
    window.history.replaceState(null, '', '/');
    await expect(establishSession(vi.fn(async () => response(204)) as unknown as typeof fetch)).resolves.toBe('signed-in');
    await expect(establishSession(vi.fn(async () => response(401)) as unknown as typeof fetch)).resolves.toBe('signed-out');
    window.history.replaceState(null, '', '/#token=stale');
    const stale = vi.fn(async () => response(401));
    await expect(establishSession(stale as unknown as typeof fetch)).resolves.toBe('signed-out');
    expect(stale).toHaveBeenCalledTimes(2);
  });

  it('reports unknown when the server is unreachable', async () => {
    window.history.replaceState(null, '', '/');
    await expect(establishSession(vi.fn(async () => { throw new TypeError('offline'); }) as unknown as typeof fetch)).resolves.toBe('unknown');
  });
});

import { afterEach, describe, expect, it, vi } from 'vitest';
import { authHeaders, establishSession, openSigned, STORAGE_KEY, streamEvents, takeFragmentToken } from './session';

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

describe('streamEvents', () => {
  it('sends the bearer header and dispatches parsed frames', async () => {
    window.localStorage.setItem(STORAGE_KEY, 'tok');
    const frames = 'event: connected\ndata: {}\n\nevent: scan.stage\ndata: {"stage":"scan"}\n\n: ping\n\n';
    const fetcher = vi.fn().mockResolvedValue(new Response(frames, { status: 200 }));
    const seen: Array<[string, string]> = [];
    await streamEvents('/api/v1/scans/s/events', (type, data) => seen.push([type, data]), new AbortController().signal, fetcher);
    expect(seen).toEqual([['connected', '{}'], ['scan.stage', '{"stage":"scan"}']]);
    const [url, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/v1/scans/s/events');
    expect(init.headers).toEqual({ Authorization: 'Bearer tok' });
  });

  it('rejects when the endpoint refuses the stream', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 401 }));
    await expect(streamEvents('/api/v1/scans/s/events', () => {}, new AbortController().signal, fetcher)).rejects.toThrow('401');
  });
});

describe('openSigned', () => {
  const stubBlobUrls = () => {
    const created = vi.fn().mockReturnValue('blob:mock');
    const revoked = vi.fn();
    (URL as unknown as { createObjectURL: typeof URL.createObjectURL; revokeObjectURL: typeof URL.revokeObjectURL }).createObjectURL = created;
    (URL as unknown as { createObjectURL: typeof URL.createObjectURL; revokeObjectURL: typeof URL.revokeObjectURL }).revokeObjectURL = revoked;
    return { created, revoked, restore: () => {
      delete (URL as unknown as { createObjectURL?: unknown }).createObjectURL;
      delete (URL as unknown as { revokeObjectURL?: unknown }).revokeObjectURL;
    } };
  };

  it('fetches with the bearer header and downloads a blob URL named by Content-Disposition', async () => {
    window.localStorage.setItem(STORAGE_KEY, 'tok');
    const fetcher = vi.fn().mockResolvedValue(new Response('csv,body', { status: 200, headers: { 'Content-Disposition': 'attachment; filename="bluntcode-scan-abcd1234-findings.csv"' } }));
    const { revoked, restore } = stubBlobUrls();
    const click = vi.fn();
    const anchor = { href: '', download: '', click, remove: vi.fn() } as unknown as HTMLAnchorElement;
    vi.spyOn(document, 'createElement').mockReturnValue(anchor);
    await openSigned('/api/v1/scans/abcd1234/findings.csv?severity=high', 'download', fetcher);
    const [url, init] = fetcher.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/v1/scans/abcd1234/findings.csv?severity=high');
    expect(init.headers).toEqual({ Authorization: 'Bearer tok' });
    expect(anchor.href).toBe('blob:mock');
    // The server's scan-specific name wins; the URL tail is only a fallback.
    expect(anchor.download).toBe('bluntcode-scan-abcd1234-findings.csv');
    expect(click).toHaveBeenCalled();
    expect(revoked).toHaveBeenCalledWith('blob:mock');
    restore();
  });

  it('falls back to the URL tail when the response names no file', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response('csv,body', { status: 200 }));
    const { restore } = stubBlobUrls();
    const anchor = { href: '', download: '', click: vi.fn(), remove: vi.fn() } as unknown as HTMLAnchorElement;
    vi.spyOn(document, 'createElement').mockReturnValue(anchor);
    await openSigned('/api/v1/scans/s/findings.csv?severity=high', 'download', fetcher);
    expect(anchor.download).toBe('findings.csv');
    restore();
  });

  it('opens an unsigned tab rather than fabricating a download on failure', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 401 }));
    const open = vi.spyOn(window, 'open').mockImplementation(() => null);
    await openSigned('/api/v1/scans/s/findings.csv', 'download', fetcher);
    expect(open).toHaveBeenCalledWith('/api/v1/scans/s/findings.csv', '_blank', 'noopener,noreferrer');
    open.mockRestore();
  });

  it('opens the popup synchronously and re-sandboxes the report blob in an iframe', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response('<html><body>report</body></html>', { status: 200 }));
    const { restore } = stubBlobUrls();
    const tab = { document: { write: vi.fn(), close: vi.fn() }, location: { href: '' } } as unknown as Window;
    const open = vi.spyOn(window, 'open').mockImplementation(() => tab);
    await openSigned('/api/v1/scans/s/report.html?inline=1', 'tab', fetcher);
    // Blank popup first (click's permission window), never a bare blob: navigation.
    expect(open).toHaveBeenCalledWith('', '_blank');
    const html = (tab.document.write as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(html).toContain('sandbox="allow-scripts"');
    expect(html).toContain('src="blob:mock"');
    expect(tab.document.close).toHaveBeenCalled();
    open.mockRestore();
    restore();
  });
});

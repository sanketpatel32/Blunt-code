import { describe, expect, it } from 'vitest';
import { href, parseRoute } from './router';

/** parseRoute is pure: every case below runs without touching window.location. */
describe('parseRoute', () => {
  it('maps the empty root to home', () => {
    expect(parseRoute('/')).toEqual({ page: 'home' });
  });

  it('parses every documented page shape', () => {
    expect(parseRoute('/workspaces')).toEqual({ page: 'workspaces' });
    expect(parseRoute('/workspaces/ws-1')).toEqual({ page: 'workspace', id: 'ws-1' });
    expect(parseRoute('/workspaces/ws-1/files')).toEqual({ page: 'files', id: 'ws-1' });
    expect(parseRoute('/workspaces/ws-1/scans')).toEqual({ page: 'history', id: 'ws-1' });
    expect(parseRoute('/workspaces/ws-1/pentest')).toEqual({ page: 'pentest', id: 'ws-1' });
    expect(parseRoute('/scans/scan-9')).toEqual({ page: 'scan', id: 'scan-9' });
    expect(parseRoute('/search')).toEqual({ page: 'search' }); // canonical: href() builds /search, so parseRoute must accept it back
    expect(parseRoute('/tools')).toEqual({ page: 'tools' });
    expect(parseRoute('/settings')).toEqual({ page: 'settings' });
    expect(parseRoute('/about')).toEqual({ page: 'about' });
  });

  it('supports the documented aliases and tolerates trailing separators', () => {
    expect(parseRoute('/docs')).toEqual({ page: 'cli' });
    expect(parseRoute('/findings')).toEqual({ page: 'search' });
    expect(parseRoute('//workspaces//')).toEqual({ page: 'workspaces' });
    expect(parseRoute('/scans/scan-9/')).toEqual({ page: 'scan', id: 'scan-9' });
  });

  it('sends unknown prefixes to the 404 page and keeps any segment as an inert id', () => {
    expect(parseRoute('/definitely/not/a/page')).toEqual({ page: 'not-found' });
    // An id is opaque: hostile characters ride along as data for the page's
    // own error handling, never as markup. Segments arrive percent-ENCODED
    // from the browser and are decoded exactly once (the old raw passthrough
    // double-encoded them into every API call); a malformed escape falls back
    // to the raw segment.
    expect(parseRoute('/scans/zzz%3Cscript%3E')).toEqual({ page: 'scan', id: 'zzz<script>' });
    expect(parseRoute('/scans/%E4%B8%AD%E6%96%87')).toEqual({ page: 'scan', id: '中文' });
    expect(parseRoute('/scans/%E4%A9')).toEqual({ page: 'scan', id: '%E4%A9' });
  });

  it('carries the raw query string (no leading ?) and omits it when absent', () => {
    expect(parseRoute('/workspaces/ws-1/files')).toEqual({ page: 'files', id: 'ws-1' });
    expect(parseRoute('/workspaces/ws-1/files', '?lang=typescript')).toEqual({ page: 'files', id: 'ws-1', q: 'lang=typescript' });
    expect(parseRoute('/search', '?query=eval')).toEqual({ page: 'search', q: 'query=eval' });
    expect(parseRoute('/', '?query=x')).toEqual({ page: 'home', q: 'query=x' });
  });
});

describe('href', () => {
  it('rebuilds the URL for every route that carries an id', () => {
    expect(href({ page: 'home' })).toBe('/');
    expect(href({ page: 'workspaces' })).toBe('/workspaces');
    expect(href({ page: 'workspace', id: 'ws-1' })).toBe('/workspaces/ws-1');
    expect(href({ page: 'files', id: 'ws-1' })).toBe('/workspaces/ws-1/files');
    expect(href({ page: 'history', id: 'ws-1' })).toBe('/workspaces/ws-1/scans');
    expect(href({ page: 'pentest', id: 'ws-1' })).toBe('/workspaces/ws-1/pentest');
    expect(href({ page: 'scan', id: 's-9' })).toBe('/scans/s-9');
    expect(href({ page: 'about' })).toBe('/about');
  });

  it('appends the raw query for deep links only when present', () => {
    expect(href({ page: 'files', id: 'ws-1', q: 'lang=typescript' })).toBe('/workspaces/ws-1/files?lang=typescript');
    expect(href({ page: 'search', q: 'query=eval' })).toBe('/search?query=eval');
    expect(href({ page: 'search' })).toBe('/search');
  });
});

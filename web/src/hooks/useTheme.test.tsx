import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useTheme, THEME_STORAGE_KEY, THEME_COLORS } from './useTheme';
import { AppShell } from '../components/AppShell';
import type { Route } from '../lib/router';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

/** Controllable matchMedia double: jsdom does not implement it. */
function stubMatchMedia(initialMatches: boolean) {
  const listeners = new Set<(event: MediaQueryListEvent) => void>();
  const dark = { matches: initialMatches };
  const media = {
    get matches() { return dark.matches; },
    addEventListener(type: string, listener: (event: MediaQueryListEvent) => void) { if (type === 'change') listeners.add(listener); },
    removeEventListener(type: string, listener: (event: MediaQueryListEvent) => void) { if (type === 'change') listeners.delete(listener); },
  };
  vi.stubGlobal('matchMedia', vi.fn(() => media));
  return {
    emitChange(matches: boolean) {
      dark.matches = matches;
      listeners.forEach((listener) => { listener({ matches } as MediaQueryListEvent); });
    },
  };
}

let api: ReturnType<typeof useTheme> | undefined;
function ThemeProbe() { api = useTheme(); return null; }

async function renderProbe() {
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<ThemeProbe />); });
  return host;
}

const homeRoute: Route = { page: 'home' };

async function renderShell(theme: 'light' | 'dark', onToggleTheme = vi.fn()) {
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<AppShell route={homeRoute} onNavigate={() => {}} onAdd={() => {}} onClose={() => {}} theme={theme} onToggleTheme={onToggleTheme} />); });
  return { host, onToggleTheme };
}

beforeEach(() => {
  localStorage.clear();
  delete document.documentElement.dataset.theme;
  const meta = document.createElement('meta');
  meta.setAttribute('name', 'theme-color');
  document.head.append(meta);
});

afterEach(async () => { await act(async () => { root?.unmount(); }); document.body.replaceChildren(); document.head.replaceChildren(); vi.unstubAllGlobals(); api = undefined; });

describe('useTheme', () => {
  it('falls back to the OS preference when no choice is stored', async () => {
    stubMatchMedia(true);
    await renderProbe();
    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(document.querySelector('meta[name="theme-color"]')?.getAttribute('content')).toBe(THEME_COLORS.dark);
  });
  it('prefers the stored choice over the OS preference', async () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'light');
    stubMatchMedia(true);
    await renderProbe();
    expect(document.documentElement.dataset.theme).toBe('light');
  });

  it('toggleTheme flips the theme, persists it, and syncs the document', async () => {
    stubMatchMedia(false);
    await renderProbe();
    expect(api?.theme).toBe('light');
    await act(async () => { api!.toggleTheme(); });
    expect(api?.theme).toBe('dark');
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(document.querySelector('meta[name="theme-color"]')?.getAttribute('content')).toBe(THEME_COLORS.dark);
  });

  it('follows OS changes while automatic, then locks once the user chooses', async () => {
    const media = stubMatchMedia(false);
    await renderProbe();
    expect(api?.theme).toBe('light');
    await act(async () => { media.emitChange(true); });
    expect(api?.theme).toBe('dark');
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBeNull();
    await act(async () => { api!.setTheme('light'); });
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('light');
    await act(async () => { media.emitChange(true); });
    expect(api?.theme).toBe('light');
  });
});

describe('theme toggle in the rail', () => {
  /* The toggle moved out of the nav link list into the rail's utility cluster,
     and from a full-width labelled row to an icon button. It was a
     twice-a-session preference sitting at the same weight as Settings, and
     as a *destination* in a rail that is read positionally.
     These assertions therefore check the ACCESSIBLE NAME rather than
     textContent: an icon-only button has no text child, and what actually
     has to survive the move is the name a screen reader announces and a
     mouse user gets on hover. */
  it('offers Dark with a moon icon in light mode and toggles on click', async () => {
    const { host, onToggleTheme } = await renderShell('light');
    const toggle = host.querySelector<HTMLButtonElement>('.theme-toggle');
    expect(toggle).not.toBeNull();
    expect(toggle!.getAttribute('aria-label')).toBe('Switch to dark theme');
    expect(toggle!.getAttribute('aria-pressed')).toBe('false');
    expect(toggle!.querySelector('path')).not.toBeNull();
    await act(async () => { toggle!.click(); });
    expect(onToggleTheme).toHaveBeenCalledTimes(1);
  });

  it('offers Light with a sun icon and aria-pressed in dark mode', async () => {
    const { host } = await renderShell('dark');
    const toggle = host.querySelector<HTMLButtonElement>('.theme-toggle');
    expect(toggle).not.toBeNull();
    expect(toggle!.getAttribute('aria-label')).toBe('Switch to light theme');
    expect(toggle!.getAttribute('aria-pressed')).toBe('true');
    expect(toggle!.querySelector('circle')).not.toBeNull();
    expect(toggle!.title).toBe('Switch to light theme');
  });

  it('is not a nav destination — it sits in the utility cluster, not the link list', async () => {
    const { host } = await renderShell('light');
    // A rail is scanned top-to-bottom as a list of places. The toggle is a
    // preference, so it must not render as one of the .rail-link rows.
    expect(host.querySelectorAll('.rail-group .theme-toggle')).toHaveLength(0);
    expect(host.querySelector('.rail-util .theme-toggle')).not.toBeNull();
  });
});

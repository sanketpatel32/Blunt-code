import { useCallback, useEffect, useState } from 'react';

export type Theme = 'light' | 'dark';

export const THEME_STORAGE_KEY = 'bluntcode-theme';

/**
 * sRGB equivalents of the --color-surface token per theme (loop 108).
 * The browser/OS chrome sits directly against the sticky nav, which paints
 * --color-surface — not --color-paper — so these are derived from surface,
 * converted out of oklch(100% 0 0) / oklch(19% 0.013 268).
 */
export const THEME_COLORS: Record<Theme, string> = { light: '#ffffff', dark: '#11141a' };

function readStoredTheme(): Theme | null {
  try {
    const value = window.localStorage.getItem(THEME_STORAGE_KEY);
    return value === 'light' || value === 'dark' ? value : null;
  } catch {
    return null;
  }
}

function systemTheme(): Theme {
  return typeof window.matchMedia === 'function' && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

/** Stored choice wins; otherwise fall back to the OS preference. */
export function resolveTheme(): Theme {
  return readStoredTheme() ?? systemTheme();
}

function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme;
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', THEME_COLORS[theme]);
}

export function useTheme() {
  const [theme, setThemeState] = useState<Theme>(() => {
    const applied = document.documentElement.dataset.theme;
    return applied === 'light' || applied === 'dark' ? applied : resolveTheme();
  });

  useEffect(() => { applyTheme(theme); }, [theme]);

  // While the user has not made an explicit choice, follow OS preference
  // changes. The stored-choice check lives inside the handler (not the effect
  // body), so the effect can stay mount-only and still stop overriding an
  // explicit choice the moment one is saved.
  useEffect(() => {
    if (readStoredTheme() !== null) return;
    if (typeof window.matchMedia !== 'function') return;
    const media = window.matchMedia('(prefers-color-scheme: dark)');
    if (typeof media.addEventListener !== 'function') return;
    const onChange = () => { if (readStoredTheme() === null) setThemeState(systemTheme()); };
    media.addEventListener('change', onChange);
    return () => media.removeEventListener('change', onChange);
  }, []);

  const setTheme = useCallback((next: Theme) => {
    try { window.localStorage.setItem(THEME_STORAGE_KEY, next); } catch { /* storage unavailable; theme still applies for this session */ }
    setThemeState(next);
  }, []);

  const toggleTheme = useCallback(() => { setTheme(theme === 'dark' ? 'light' : 'dark'); }, [theme, setTheme]);

  return { theme, setTheme, toggleTheme };
}

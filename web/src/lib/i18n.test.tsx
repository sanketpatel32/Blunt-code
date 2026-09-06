import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { I18nProvider, LOCALES, dictionaries, useT, type Locale } from './i18n';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let probe: ReturnType<typeof useT> | null = null;

function Probe() {
  probe = useT();
  return null;
}

async function renderProvider() {
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<I18nProvider><Probe /></I18nProvider>); });
  return host;
}

beforeEach(() => {
  window.localStorage.clear();
  document.documentElement.lang = 'en';
});

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
  window.localStorage.clear();
  document.documentElement.lang = 'en';
});

describe('i18n dictionaries', () => {
  it('ships six locales with labels and native names', () => {
    expect(LOCALES.map((locale) => locale.value)).toEqual(['en', 'es', 'fr', 'de', 'ja', 'hi']);
    for (const locale of LOCALES) {
      expect(locale.label.trim()).not.toBe('');
      expect(locale.name.trim()).not.toBe('');
    }
  });

  it('keeps non-English dictionaries free of orphan keys', () => {
    for (const locale of LOCALES.map((locale) => locale.value)) {
      if (locale === 'en') continue;
      for (const key of Object.keys(dictionaries[locale])) {
        expect(dictionaries.en[key], `${locale} key "${key}" is missing from the English source dictionary`).toBeDefined();
      }
    }
  });

  it('never defines translation keys that shadow internal severity or state identifiers', () => {
    // API identifiers (severity names, scan states) must stay stable regardless
    // of the UI language; no dictionary may translate them out from under the
    // components that match on raw values.
    const internalIdentifiers = ['critical', 'high', 'medium', 'low', 'info', 'completed', 'completed_with_warnings', 'failed', 'interrupted', 'timed_out', 'cancelled', 'running'];
    for (const locale of LOCALES.map((locale) => locale.value)) {
      for (const key of Object.keys(dictionaries[locale])) {
        expect(internalIdentifiers).not.toContain(key);
      }
    }
  });
});

describe('locale switching and fallback', () => {
  it('falls back translated-missing → English → raw key', async () => {
    await renderProvider();
    const enOnlyKey = Object.keys(dictionaries.en).find((key) =>
      LOCALES.map((locale) => locale.value).every((locale) => dictionaries[locale][key] === undefined || locale === 'en'));
    expect(enOnlyKey).toBeDefined(); // at least one key only English carries

    await act(async () => { probe!.setLocale('de'); });
    expect(probe!.locale).toBe('de');
    // German translation where one exists…
    if (dictionaries.de['nav.home'] !== undefined) {
      expect(probe!.t('nav.home')).toBe(dictionaries.de['nav.home']);
    }
    // …English for keys German has not translated…
    expect(probe!.t(enOnlyKey!)).toBe(dictionaries.en[enOnlyKey!]);
    // …and the raw key itself for unknown keys, never an empty string.
    expect(probe!.t('nope.does.not.exist')).toBe('nope.does.not.exist');
  });

  it('updates <html lang> and persists the choice per switch', async () => {
    await renderProvider();
    await act(async () => { probe!.setLocale('ja'); });
    expect(document.documentElement.lang).toBe('ja');
    expect(window.localStorage.getItem('bluntcode.lang')).toBe('ja');
    await act(async () => { probe!.setLocale('fr'); });
    expect(document.documentElement.lang).toBe('fr');
    expect(window.localStorage.getItem('bluntcode.lang')).toBe('fr');
  });

  it('restores the persisted locale on mount', async () => {
    window.localStorage.setItem('bluntcode.lang', 'hi');
    await renderProvider();
    expect(probe!.locale).toBe('hi');
    expect(document.documentElement.lang).toBe('hi');
  });
});

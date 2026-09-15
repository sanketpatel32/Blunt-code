import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { lintPattern, RuleStudioPage } from './RuleStudioPage';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

const RULES_KEY = 'bluntcode.customRules';

const VALID_YAML = 'id: no-eval\nlanguages: [python]\npattern: "eval($ARG)"\nseverity: high\nmessage: Avoid eval.\n';
/** Saves fine before the fix: YAML structure is valid, the pattern is not. */
const MALFORMED_PATTERN_YAML = 'id: bad-paren\nlanguages: [python]\npattern: "("\nseverity: high\nmessage: Bad pattern.\n';
/** Red-border invalid YAML that still yields a parsed pattern (pre-fix preview bug). */
const INVALID_YAML_WITH_PATTERN = 'id: unclosed\nlanguages: [python]\npattern: "eval(\nmessage: never validates.\n';

beforeEach(() => {
  localStorage.clear();
});

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

async function renderPage() {
  // The page is client-only (localStorage scratchpad); fetch is stubbed because
  // importing the shared dialogs module pulls in the api client.
  vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json' } }))));
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<RuleStudioPage />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return host;
}

function editor(host: HTMLElement) {
  return host.querySelector<HTMLTextAreaElement>('#rule-yaml-editor')!;
}

async function typeYaml(host: HTMLElement, text: string) {
  const ta = editor(host);
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value')!.set!;
    setter.call(ta, text);
    ta.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

function saveButton(host: HTMLElement) {
  return host.querySelector<HTMLButtonElement>('button[aria-label="Save rule"]')!;
}

function previewFindings(host: HTMLElement) {
  return host.querySelector('[aria-label="Preview findings"]');
}

function pressKey(target: Element, key: string) {
  target.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
}

describe('RuleStudioPage pattern sanity lint', () => {
  it('warns inline about an unbalanced paren, keeps Save disabled, and suppresses the preview', async () => {
    const host = await renderPage();
    await typeYaml(host, MALFORMED_PATTERN_YAML);
    expect(host.textContent).toContain("Pattern looks malformed: unbalanced '('");
    expect(saveButton(host).disabled).toBe(true);
    // the mock Live Preview is suppressed while the pattern is malformed
    expect(previewFindings(host)).toBeNull();
    expect(host.textContent).toContain('preview suppressed until the pattern is fixed');
    expect(host.textContent).not.toContain('src/example.py:12');
  });

  it('gates the preview on outright-invalid YAML even though a pattern still parses', async () => {
    const host = await renderPage();
    await typeYaml(host, INVALID_YAML_WITH_PATTERN);
    expect(saveButton(host).disabled).toBe(true);
    expect(host.textContent).toContain('Preview suppressed — fix the YAML errors');
    expect(previewFindings(host)).toBeNull();
  });

  it('keeps Save enabled and the preview visible for a well-formed rule', async () => {
    const host = await renderPage();
    await typeYaml(host, VALID_YAML);
    expect(saveButton(host).disabled).toBe(false);
    expect(previewFindings(host)).not.toBeNull();
    expect(host.textContent).toContain('src/example.py:12');
  });
});

describe('RuleStudioPage editor focus', () => {
  it('moves focus to the editor container on Escape so Tab can continue forward', async () => {
    const host = await renderPage();
    const ta = editor(host);
    await act(async () => { ta.focus(); });
    expect(document.activeElement).toBe(ta);
    await act(async () => { pressKey(ta, 'Escape'); });
    const container = ta.closest<HTMLDivElement>('div[tabindex="-1"]')!;
    expect(container).not.toBeNull();
    expect(document.activeElement).toBe(container);
    // tabIndex=-1 means the wrapper is a focus target, not a tab stop — the
    // next Tab moves forward through the page instead of re-entering the editor.
    expect(container.getAttribute('tabindex')).toBe('-1');
  });
});

describe('RuleStudioPage delete confirmation', () => {
  const saved = [{ id: 'no-eval', languages: ['python'], pattern: 'eval($ARG)', severity: 'high', message: 'Avoid eval.', enabled: true }];

  it('arms a confirmation naming the rule id, and Cancel keeps the rule', async () => {
    localStorage.setItem(RULES_KEY, JSON.stringify(saved));
    const host = await renderPage();
    await act(async () => { host.querySelector<HTMLButtonElement>('button[aria-label="Delete no-eval"]')!.click(); });
    const dialog = host.querySelector('dialog[open]')!;
    expect(dialog).not.toBeNull();
    expect(dialog.textContent).toContain('no-eval');
    // nothing is deleted until confirmed
    expect(JSON.parse(localStorage.getItem(RULES_KEY)!)).toHaveLength(1);
    const cancel = [...dialog.querySelectorAll<HTMLButtonElement>('button')].find((button) => button.textContent === 'Cancel')!;
    await act(async () => { cancel.click(); });
    expect(host.querySelector('dialog')).toBeNull();
    expect(host.textContent).toContain('no-eval');
    expect(JSON.parse(localStorage.getItem(RULES_KEY)!)).toHaveLength(1);
  });

  it('removes the rule from the DOM and localStorage only after confirming', async () => {
    localStorage.setItem(RULES_KEY, JSON.stringify(saved));
    const host = await renderPage();
    await act(async () => { host.querySelector<HTMLButtonElement>('button[aria-label="Delete no-eval"]')!.click(); });
    const confirm = [...host.querySelectorAll<HTMLButtonElement>('dialog button')].find((button) => button.textContent === 'Delete rule')!;
    await act(async () => { confirm.click(); await Promise.resolve(); await Promise.resolve(); });
    expect(host.querySelector('dialog')).toBeNull();
    expect(host.textContent).not.toContain('no-eval');
    expect(JSON.parse(localStorage.getItem(RULES_KEY)!)).toEqual([]);
  });
});

describe('lintPattern', () => {
  it('flags unbalanced groups, classes, and quotes; passes sane patterns', () => {
    expect(lintPattern('(')).toContain("unbalanced '('");
    expect(lintPattern('eval($ARG')).toContain("unbalanced '('");
    expect(lintPattern('foo)bar')).toContain("unbalanced ')'");
    expect(lintPattern('foo[')).toContain("unbalanced '['");
    expect(lintPattern('eval("x')).toContain('unclosed quote');
    expect(lintPattern('eval($ARG)')).toBeNull();
    expect(lintPattern('^\\([a-z]+\\)$')).toBeNull(); // escaped literal parens are fine
    expect(lintPattern('^(?:foo|bar)$')).toBeNull();
    expect(lintPattern(undefined)).toBeNull();
    expect(lintPattern('')).toBeNull();
  });
});

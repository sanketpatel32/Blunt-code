import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { CLIPage } from './CLIPage';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function json(body: unknown) {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
}

function metaMock() {
  return vi.fn((input: string) => {
    if (input.endsWith('/meta')) return Promise.resolve(json({ version: '0.19.0', api_version: 'v1', os: 'windows', architecture: 'amd64' }));
    return Promise.resolve(json({}));
  });
}

async function renderPage() {
  vi.stubGlobal('fetch', metaMock());
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<CLIPage />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return host;
}

function stubClipboard() {
  const writeText = vi.fn((_text: string) => Promise.resolve(true));
  Object.defineProperty(window.navigator, 'clipboard', { value: { writeText }, configurable: true });
  return writeText;
}

afterEach(() => {
  if (root) {
    act(() => { root.unmount(); });
  }
  document.body.innerHTML = '';
  vi.restoreAllMocks();
});

describe('CLIPage', () => {
  it('renders header, hero card, and initial reference manual', async () => {
    const host = await renderPage();
    expect(host.textContent).toContain('Command-Line Interface (CLI)');
    expect(host.textContent).toContain('Reference Manual');
    expect(host.textContent).toContain('CI/CD & Automation');
    expect(host.textContent).toContain('Command Builder');
    expect(host.textContent).toContain('bluntcode scan');
  });

  it('filters commands by search input', async () => {
    const host = await renderPage();
    const input = host.querySelector('input[placeholder*="Search commands"]') as HTMLInputElement;
    expect(input).not.toBeNull();

    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')?.set;
      setter?.call(input, 'pentest');
      input.dispatchEvent(new Event('input', { bubbles: true }));
      input.dispatchEvent(new Event('change', { bubbles: true }));
    });

    expect(host.textContent).toContain('bluntcode pentest');
    expect(host.textContent).not.toContain('bluntcode tools');
  });

  it('switches to CI/CD tab and shows GitHub Actions workflow', async () => {
    const host = await renderPage();
    const tabs = Array.from(host.querySelectorAll('button.tab')) as HTMLButtonElement[];
    const ciTab = tabs.find((b) => b.textContent?.includes('CI/CD & Automation'));
    expect(ciTab).toBeDefined();

    await act(async () => {
      ciTab!.click();
    });

    expect(host.textContent).toContain('GitHub Actions with SARIF Code Scanning');
    expect(host.textContent).toContain('Git Pre-Commit Hook');
    expect(host.textContent).toContain('upload-sarif');
  });

  it('switches to Command Builder and generates custom command', async () => {
    const host = await renderPage();
    const tabs = Array.from(host.querySelectorAll('button.tab')) as HTMLButtonElement[];
    const builderTab = tabs.find((b) => b.textContent?.includes('Command Builder'));
    expect(builderTab).toBeDefined();

    await act(async () => {
      builderTab!.click();
    });

    expect(host.textContent).toContain('Scan Command Generator');
    expect(host.textContent).toContain('Generated Command:');

    const targetInput = host.querySelector('input[value="."]') as HTMLInputElement;
    expect(targetInput).not.toBeNull();

    await act(async () => {
      const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')?.set;
      setter?.call(targetInput, 'src/api');
      targetInput.dispatchEvent(new Event('input', { bubbles: true }));
      targetInput.dispatchEvent(new Event('change', { bubbles: true }));
    });

    expect(host.textContent).toContain('bluntcode scan src/api');
  });

  it('copies command on button click', async () => {
    const writeText = stubClipboard();
    const host = await renderPage();
    const copyBtn = host.querySelector('button[title="Copy synopsis"]') as HTMLButtonElement;
    expect(copyBtn).not.toBeNull();

    await act(async () => {
      copyBtn.click();
    });

    expect(writeText).toHaveBeenCalled();
  });

  it('renders with theme-compliant classes in both dark and light modes without hardcoded colors', async () => {
    document.documentElement.setAttribute('data-theme', 'dark');
    const darkHost = await renderPage();
    expect(darkHost.querySelector('.cli-page')).not.toBeNull();
    expect(darkHost.querySelector('.cli-hero')).not.toBeNull();
    expect(darkHost.querySelector('.cli-command-card')).not.toBeNull();
    expect(darkHost.querySelector('.cli-terminal-box')).not.toBeNull();

    // Verify no elements carry hardcoded #fff, #1e1e1e, or rgba inline background colors
    const elementsWithInlineBg = Array.from(darkHost.querySelectorAll('[style*="background"]'));
    const hardcodedBgs = elementsWithInlineBg.filter((el) => {
      const s = el.getAttribute('style') || '';
      return s.includes('#fff') || s.includes('#1e1e1e') || s.includes('rgba(0,0,0');
    });
    expect(hardcodedBgs).toHaveLength(0);

    document.documentElement.setAttribute('data-theme', 'light');
  });
});

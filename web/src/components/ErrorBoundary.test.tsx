import { act } from 'react';
import type { ReactNode } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ErrorBoundary } from './ErrorBoundary';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let errorSpy: ReturnType<typeof vi.spyOn>;

function Bomb(): never { throw new Error('boom in render'); }

async function render(node: ReactNode) {
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(node); });
  return host;
}

beforeEach(() => { errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {}); });
afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

describe('ErrorBoundary', () => {
  it('renders children unchanged when nothing throws', async () => {
    const host = await render(<ErrorBoundary resetKey="/">Healthy view</ErrorBoundary>);
    expect(host.textContent).toContain('Healthy view');
    expect(host.querySelector('.error-panel')).toBeNull();
  });

  it('renders a reassuring fallback with reload and diagnostics when a child throws', async () => {
    const reload = vi.fn();
    const host = await render(<ErrorBoundary resetKey="/" reload={reload}><Bomb /></ErrorBoundary>);
    expect(host.textContent).toContain('Something went wrong');
    expect(host.textContent).toContain('safe on disk');
    expect(host.querySelector('.error-panel[role="alert"]')).not.toBeNull();
    const details = host.querySelector('details');
    expect(details?.textContent).toContain('Error details');
    expect(details?.textContent).toContain('boom in render');
    const button = [...host.querySelectorAll('button')].find((b) => b.textContent === 'Reload view');
    expect(button).toBeDefined();
    await act(async () => { button!.click(); });
    expect(reload).toHaveBeenCalled();
    expect(errorSpy.mock.calls.some((call) => call[0] === 'Blunt Code view crashed')).toBe(true);
  });

  it('recovers without a reload when the resetKey changes', async () => {
    const host = await render(<ErrorBoundary resetKey="/scans/scan-1"><Bomb /></ErrorBoundary>);
    expect(host.textContent).toContain('Something went wrong');
    await act(async () => { root.render(<ErrorBoundary resetKey="/"><p>Recovered view</p></ErrorBoundary>); });
    expect(host.textContent).toContain('Recovered view');
    expect(host.textContent).not.toContain('Something went wrong');
  });
});

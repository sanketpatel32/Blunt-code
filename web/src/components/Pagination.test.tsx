import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Pagination } from './Pagination';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

afterEach(() => {
  root?.unmount();
  document.body.innerHTML = '';
});

async function render(ui: React.ReactNode) {
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(ui); });
  return host;
}

describe('Pagination', () => {
  it('renders a nav with Previous and Next buttons', async () => {
    const host = await render(
      <Pagination
        aria-label="Test pagination"
        hasPrevious={false}
        hasNext={true}
        onPrevious={() => {}}
        onNext={() => {}}
      />,
    );
    const nav = host.querySelector('nav[aria-label="Test pagination"]');
    expect(nav).toBeTruthy();
    expect(nav?.querySelectorAll('button')).toHaveLength(2);
    expect(nav?.textContent).toContain('Previous');
    expect(nav?.textContent).toContain('Next');
  });

  it('disables Previous when hasPrevious is false', async () => {
    const host = await render(
      <Pagination
        aria-label="Pages"
        hasPrevious={false}
        hasNext={true}
        onPrevious={() => {}}
        onNext={() => {}}
      />,
    );
    const prev = host.querySelector('button')!;
    expect(prev.disabled).toBe(true);
    expect(prev.getAttribute('aria-disabled')).toBe('true');
  });

  it('calls onNext when Next is clicked', async () => {
    const onNext = vi.fn();
    const host = await render(
      <Pagination
        aria-label="Pages"
        hasPrevious={true}
        hasNext={true}
        onPrevious={() => {}}
        onNext={onNext}
      />,
    );
    const buttons = host.querySelectorAll('button');
    const next = buttons[buttons.length - 1]!;
    await act(async () => { next.click(); });
    expect(onNext).toHaveBeenCalledTimes(1);
  });

  it('uses ghost variant on buttons when variant is ghost', async () => {
    const host = await render(
      <Pagination
        aria-label="Pages"
        variant="ghost"
        hasPrevious={true}
        hasNext={true}
        onPrevious={() => {}}
        onNext={() => {}}
      />,
    );
    const prev = host.querySelector('button')!;
    expect(prev.className).toContain('text-[var(--color-ink-soft)]');
  });
});

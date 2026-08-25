import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useTicker } from './useTicker';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let container: HTMLDivElement;

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

function Probe({ active }: { active: boolean }) {
  const tick = useTicker(active);
  return <output>{tick}</output>;
}

describe('useTicker', () => {
  it('does not tick while inactive', () => {
    vi.useFakeTimers();
    try {
      container = document.createElement('div');
      document.body.appendChild(container);
      root = createRoot(container);
      act(() => root.render(<Probe active={false} />));
      act(() => { vi.advanceTimersByTime(5000); });
      expect(container.querySelector('output')!.textContent).toBe('0');
    } finally { vi.useRealTimers(); }
  });

  it('ticks once per second while active and stops when deactivated', () => {
    vi.useFakeTimers();
    try {
      container = document.createElement('div');
      document.body.appendChild(container);
      root = createRoot(container);
      act(() => root.render(<Probe active />));
      act(() => { vi.advanceTimersByTime(3100); });
      expect(container.querySelector('output')!.textContent).toBe('3');
      act(() => root.render(<Probe active={false} />));
      act(() => { vi.advanceTimersByTime(10_000); });
      expect(container.querySelector('output')!.textContent).toBe('3');
    } finally { vi.useRealTimers(); }
  });
});

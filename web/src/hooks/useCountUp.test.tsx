import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useCountUp } from './useCountUp';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;
let container: HTMLDivElement;

afterEach(() => {
  act(() => root.unmount());
  container.remove();
  vi.restoreAllMocks();
});

function Probe({ target }: { target: number }) {
  const shown = useCountUp(target);
  return <output>{shown}</output>;
}

function render(target: number) {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => root.render(<Probe target={target} />));
  return () => container.querySelector('output')!.textContent;
}

describe('useCountUp', () => {
  it('shows the first value immediately', () => {
    const read = render(42);
    expect(read()).toBe('42');
  });

  it('jumps to the new value under prefers-reduced-motion', () => {
    const match = vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: true } as MediaQueryList);
    const read = render(5);
    act(() => root.render(<Probe target={99} />));
    expect(read()).toBe('99');
    expect(match).toHaveBeenCalled();
  });

  it('animates through intermediate values toward the target', () => {
    vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: false } as MediaQueryList);
    const frames: Array<(t: number) => void> = [];
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb) => { frames.push(cb); return frames.length; });
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {});
    const read = render(0);
    act(() => root.render(<Probe target={100} />));
    // Pump every scheduled frame inside act so the setStates flush.
    act(() => {
      let clock = 0;
      while (frames.length) {
        clock += 120;
        frames.shift()!(clock);
      }
    });
    expect(read()).toBe('100');
  });

  it('renders non-finite targets as 0 without wedging', () => {
    const read = render(Number.NaN);
    expect(read()).toBe('0');
  });
});

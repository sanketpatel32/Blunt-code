import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { GradeTile } from './GradeTile';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

afterEach(() => {
  root?.unmount();
  document.body.innerHTML = '';
  vi.restoreAllMocks();
});

describe('GradeTile reduced motion', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn().mockImplementation((query: string) => ({
        matches: query.includes('prefers-reduced-motion: reduce'),
        media: query,
        addEventListener: () => {},
        removeEventListener: () => {},
        addListener: () => {},
        removeListener: () => {},
        dispatchEvent: () => false,
      })),
    );
  });

  it('renders a static letter when flip is on but reduced motion is preferred', async () => {
    const host = document.createElement('div');
    document.body.append(host);
    root = createRoot(host);
    await act(async () => {
      root.render(<GradeTile grade="B" flip size="lg" />);
    });
    expect(host.querySelector('.grade-tile-letter')?.textContent).toBe('B');
    expect(host.querySelector('.grade-flap')).toBeNull();
  });
});

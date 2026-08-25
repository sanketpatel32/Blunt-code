import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { WorkspaceContextSidebar } from './WorkspaceContext';

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

function render(ui: React.ReactNode) {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => root.render(ui));
  return container;
}

describe('WorkspaceContextSidebar', () => {
  it('marks the current section active with aria-current', () => {
    const dom = render(<WorkspaceContextSidebar id="ws-1" current={{ page: 'workspace', id: 'ws-1' }} onNavigate={() => {}} />);
    const overview = [...dom.querySelectorAll('button')].find((b) => b.textContent === 'Overview')!;
    expect(overview.className).toContain('active');
    expect(overview.getAttribute('aria-current')).toBe('page');
    expect(overview.disabled).toBe(false);
  });

  it('does not mark a different workspace as active', () => {
    const dom = render(<WorkspaceContextSidebar id="ws-1" current={{ page: 'workspace', id: 'ws-other' }} onNavigate={() => {}} />);
    for (const button of [...dom.querySelectorAll('button')]) {
      expect(button.className).not.toContain('active');
      expect(button.getAttribute('aria-current')).toBeNull();
    }
  });

  it('navigates when a section is clicked', () => {
    const onNavigate = vi.fn();
    const dom = render(<WorkspaceContextSidebar id="ws-1" current={{ page: 'workspace', id: 'ws-1' }} onNavigate={onNavigate} />);
    const files = [...dom.querySelectorAll('button')].find((b) => b.textContent === 'Files & rules')!;
    act(() => files.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true })));
    expect(onNavigate).toHaveBeenCalledWith({ page: 'files', id: 'ws-1' });
  });

  it('renders an accessible landmark labelled for screen readers', () => {
    const dom = render(<WorkspaceContextSidebar id="ws-1" current={{ page: 'workspace', id: 'ws-1' }} onNavigate={() => {}} />);
    const nav = dom.querySelector('nav[aria-label="Workspace sections"]');
    expect(nav).not.toBeNull();
  });
});

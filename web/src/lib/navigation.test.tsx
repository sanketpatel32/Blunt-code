import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { navigateFromLink } from './navigation';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;
let root: Root;
let container: HTMLDivElement;

beforeEach(() => {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
});
afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

function click(init: MouseEventInit = {}, props: { target?: string; download?: string } = {}) {
  const navigate = vi.fn();
  act(() => root.render(<a {...props} href="#destination" onClick={(event) => navigateFromLink(event, navigate)}>Destination</a>));
  const event = new MouseEvent('click', { bubbles: true, cancelable: true, ...init });
  act(() => container.querySelector('a')!.dispatchEvent(event));
  return { event, navigate };
}

describe('navigateFromLink', () => {
  it('routes ordinary clicks within the app', () => {
    const { event, navigate } = click();
    expect(event.defaultPrevented).toBe(true);
    expect(navigate).toHaveBeenCalledOnce();
  });

  it.each(['ctrlKey', 'metaKey', 'shiftKey', 'altKey'])('preserves browser behavior for %s', (modifier) => {
    const { event, navigate } = click({ [modifier]: true });
    expect(event.defaultPrevented).toBe(false);
    expect(navigate).not.toHaveBeenCalled();
  });

  it('leaves middle clicks to the browser', () => {
    const { event, navigate } = click({ button: 1 });
    expect(event.defaultPrevented).toBe(false);
    expect(navigate).not.toHaveBeenCalled();
  });

  it.each([{ target: '_blank' }, { download: '' }])('respects explicit link attributes %o', (props) => {
    const { event, navigate } = click({}, props);
    expect(event.defaultPrevented).toBe(false);
    expect(navigate).not.toHaveBeenCalled();
  });
});

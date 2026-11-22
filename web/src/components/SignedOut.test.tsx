import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it } from 'vitest';
import { SignedOut } from './SignedOut';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
});

describe('SignedOut', () => {
  it('explains how to get a signed-in tab', async () => {
    const host = document.createElement('div');
    document.body.append(host);
    root = createRoot(host);
    await act(async () => { root.render(<SignedOut />); });
    expect(host.querySelector('h1')?.textContent).toBe('Open Blunt Code from its launcher');
    expect(host.textContent).toContain('Sign in:');
  });
});

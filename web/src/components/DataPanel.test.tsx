import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it } from 'vitest';
import { DataPanel } from './DataPanel';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

afterEach(() => {
  root?.unmount();
  document.body.innerHTML = '';
});

describe('DataPanel', () => {
  it('links title id for aria-labelledby and keeps board panel classes', async () => {
    const host = document.createElement('div');
    document.body.append(host);
    root = createRoot(host);
    await act(async () => {
      root.render(
        <DataPanel
          titleId="ledger-heading"
          title="Workspace assessments"
          panelClassName="board-panel board-ledger"
        >
          <p>Body</p>
        </DataPanel>,
      );
    });
    const section = host.querySelector('section.board-ledger');
    expect(section?.getAttribute('aria-labelledby')).toBe('ledger-heading');
    expect(host.querySelector('#ledger-heading')?.textContent).toBe('Workspace assessments');
  });
});

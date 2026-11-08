import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it } from 'vitest';
import { ListFooter } from './ListFooter';

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

describe('ListFooter', () => {
  it('renders range text with tabular nums wrapper', async () => {
    const host = await render(
      <ListFooter rangeText={<>Showing 1–25 of 100</>} />,
    );
    const note = host.querySelector('.board-panel-foot-note');
    expect(note?.textContent).toContain('Showing 1–25 of 100');
    expect(note?.classList.contains('tabular-nums')).toBe(true);
  });

  it('forwards pagination props to Pagination', async () => {
    const host = await render(
      <ListFooter
        rangeText="1–10"
        pagination={{
          'aria-label': 'Items',
          hasPrevious: false,
          hasNext: true,
          onPrevious: () => {},
          onNext: () => {},
        }}
      />,
    );
    expect(host.querySelector('nav[aria-label="Items"]')).toBeTruthy();
  });
});

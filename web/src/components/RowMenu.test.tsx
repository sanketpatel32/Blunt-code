import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it } from 'vitest';
import { RowMenu } from './RowMenu';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
});

function mount(node: React.ReactElement) {
  const container = document.createElement('div');
  document.body.append(container);
  root = createRoot(container);
  act(() => { root.render(node); });
  return container;
}

describe('RowMenu', () => {
  it('renders nothing when there are no items', () => {
    const container = mount(<RowMenu items={[]} />);
    expect(container.querySelector('button')).toBeNull();
  });

  it('exposes one trigger button labelled with the row subject', () => {
    const container = mount(
      <RowMenu label="Actions for claire-frontend" items={[{ label: 'Rename', onSelect: () => {} }]} />,
    );
    const trigger = container.querySelector<HTMLButtonElement>('button[aria-label="Actions for claire-frontend"]');
    expect(trigger).not.toBeNull();
    // Exactly one control: the overflow menu must not add visible actions to the row.
    expect(container.querySelectorAll('button')).toHaveLength(1);
  });
});

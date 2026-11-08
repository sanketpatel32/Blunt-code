import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { FilterBar } from './FilterBar';
import { Input } from './ui/input';

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

describe('FilterBar', () => {
  it('clusters filters and exposes meta as a status region', async () => {
    const host = await render(
      <FilterBar
        meta="2 workspaces"
        filters={(
          <>
            <Input type="search" aria-label="Search workspaces" placeholder="Search" />
            <Input type="search" aria-label="Filter by tag" placeholder="Tag" />
          </>
        )}
      />,
    );
    expect(host.querySelector('.toolbar-filters input[aria-label="Search workspaces"]')).toBeTruthy();
    expect(host.querySelector('[role="status"]')?.textContent).toBe('2 workspaces');
  });

  it('calls onClear when the clear control is clicked', async () => {
    const onClear = vi.fn();
    const host = await render(
      <FilterBar filters={<Input aria-label="Search workspaces" />} onClear={onClear} clearLabel="Clear filters" />,
    );
    await act(async () => { host.querySelector('button')!.click(); });
    expect(onClear).toHaveBeenCalledTimes(1);
    expect(host.textContent).toContain('Clear filters');
  });
});

import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it } from 'vitest';
import { DataTable, type DataTableColumn } from './DataTable';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

interface Row { id: string; name: string }

const columns: DataTableColumn<Row>[] = [
  { id: 'name', header: 'Name', accessor: (row) => row.name },
];

interface TableProps {
  data: Row[];
  columns: DataTableColumn<Row>[];
  page: number;
  pageSize: number;
  total: number;
  hasNext: boolean;
  onPageChange: (page: number) => void;
  emptyTitle?: string;
}

async function renderTable(props: TableProps) {
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<DataTable {...props} />); });
  await act(async () => { await Promise.resolve(); });
  return host;
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
});

describe('DataTable pagination', () => {
  it('renders the pager with the served range for a non-empty page', async () => {
    const host = await renderTable({ data: [{ id: 'r1', name: 'One' }], columns, page: 1, pageSize: 25, total: 1, hasNext: false, onPageChange: () => {} });
    const pager = host.querySelector('nav[aria-label="Pagination"]');
    expect(pager).not.toBeNull();
    expect(pager!.textContent).toContain('Showing 1–1 of 1');
  });

  it('hides the pager entirely when the total is zero — the empty row already says it', async () => {
    const host = await renderTable({ data: [], columns, page: 1, pageSize: 25, total: 0, hasNext: false, onPageChange: () => {}, emptyTitle: 'No results yet' });
    expect(host.querySelector('nav[aria-label="Pagination"]')).toBeNull();
    expect(host.textContent).not.toContain('1–0'); // the old "Showing 1–0 of 0"
    expect(host.textContent).toContain('No results yet');
  });
});

import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Route } from '../lib/router';
import type { Notice } from '../lib/notice';
import type { Finding, Scan } from '../types';
import { ScanPage } from './ScanPage';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

/** ScanPage opens an event stream while the scan payload is still loading, even for terminal scans. */
class FakeEventSource {
  addEventListener() {}
  close() {}
}

let root: Root;

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function scanFixture(overrides: Partial<Scan> = {}): Scan {
  return { id: 'scan-1', workspace_id: 'ws-1', state: 'completed', started_at: '2026-08-12T00:00:00Z', finished_at: '2026-08-12T00:00:02Z', total_findings: 5, analyzer_runs: [{ analyzer_id: 'biome', status: 'succeeded' }], ...overrides };
}

async function renderScanPage(scan: Scan, fixed: unknown = { fixed: [], total_fixed: 0, comparison_available: false, previous_scan_id: null }) {
  const fetchMock = vi.fn((input: string) => {
    if (input.endsWith(`/scans/${scan.id}`)) return Promise.resolve(json(scan));
    if (input.endsWith(`/scans/${scan.id}/fixed`)) return Promise.resolve(json(fixed));
    if (input.endsWith(`/scans/${scan.id}/report`)) return Promise.resolve(json({ scan, comparison: {}, warnings: [], findings: [] }));
    if (input.includes(`/scans/${scan.id}/findings`)) return Promise.resolve(json({ items: [], total: 0, limit: 25, offset: 0, has_more: false }));
    return Promise.resolve(json({}));
  });
  vi.stubGlobal('fetch', fetchMock);
  vi.stubGlobal('EventSource', FakeEventSource);
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<ScanPage id={scan.id} go={vi.fn<(route: Route) => void>()} notify={vi.fn<(notice: Notice) => void>()} />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return { host, fetchMock };
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

const fixedFinding = (index: number): Finding => ({ id: `f-${index}`, analyzer_id: index % 2 ? 'ruff' : 'biome', severity: 'medium', category: 'style', title: `Fixed issue ${index}`, message: 'Resolved', relative_path: `src/file${index}.ts`, start_line: index + 1 });

describe('ScanPage what-changed panel', () => {
  it('renders the fixed findings with counts, severity pills, and file locations', async () => {
    const fixed = {
      fixed: [
        { id: 'f-1', analyzer_id: 'biome', severity: 'high', category: 'correctness', title: 'Unused import', message: 'Remove it', relative_path: 'src/main.py', start_line: 4 },
        { id: 'f-2', analyzer_id: 'ruff', severity: 'medium', category: 'style', rule_id: 'F401', message: 'Imported but unused', relative_path: 'src/util.py', start_line: 12 },
      ],
      total_fixed: 2,
      comparison_available: true,
      previous_scan_id: 'scan-0',
    };
    const { host } = await renderScanPage(scanFixture({ fixed_count: 2 }), fixed);
    const panel = host.querySelector<HTMLElement>('.what-changed')!;
    expect(panel).not.toBeNull();
    expect(panel.textContent).toContain('What changed');
    expect(panel.textContent).toContain('Fixed since the previous scan');
    expect(panel.textContent).toContain('2 findings fixed');
    const rows = [...panel.querySelectorAll('.what-changed-row')];
    expect(rows).toHaveLength(2);
    expect(rows[0].querySelector('.severity.high')?.textContent).toBe('high');
    expect(rows[0].textContent).toContain('Unused import');
    expect(rows[0].textContent).toContain('src/main.py:4');
    expect(rows[0].textContent).toContain('Biome');
    expect(rows[1].textContent).toContain('F401');
    expect(rows[1].textContent).toContain('src/util.py:12');
    expect(panel.querySelector('.what-changed-more')).toBeNull();
  });

  it('stays hidden when the backend has no previous scan to compare against', async () => {
    const { host, fetchMock } = await renderScanPage(
      scanFixture({ fixed_count: 3 }),
      { fixed: [], total_fixed: 0, comparison_available: false, previous_scan_id: null },
    );
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/scans/scan-1/fixed', expect.objectContaining({ headers: expect.anything() }));
    expect(host.querySelector('.what-changed')).toBeNull();
    expect(host.textContent).not.toContain('What changed');
  });

  it('stays hidden, without requesting the comparison, when nothing was fixed', async () => {
    const { host, fetchMock } = await renderScanPage(scanFixture({ fixed_count: 0 }));
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes('/fixed'))).toBe(false);
    expect(host.querySelector('.what-changed')).toBeNull();
  });

  it('caps the visible rows at ten and expands the rest through the details element', async () => {
    const findings = Array.from({ length: 13 }, (_, index) => fixedFinding(index + 1));
    const { host } = await renderScanPage(
      scanFixture({ fixed_count: 13 }),
      { fixed: findings, total_fixed: 13, comparison_available: true, previous_scan_id: 'scan-0' },
    );
    const panel = host.querySelector<HTMLElement>('.what-changed')!;
    expect(panel.querySelector('.what-changed-headline')?.textContent).toContain('13 findings fixed');
    expect([...panel.querySelectorAll('.what-changed > .what-changed-list > .what-changed-row')]).toHaveLength(10);
    const details = panel.querySelector<HTMLDetailsElement>('.what-changed-more')!;
    expect(details).not.toBeNull();
    expect(details.open).toBe(false);
    expect(details.querySelector('summary')?.textContent).toBe('+3 more fixed');
    expect(details.querySelectorAll('.what-changed-row')).toHaveLength(3);
    await act(async () => { details.querySelector('summary')!.click(); });
    expect(details.open).toBe(true);
    expect([...panel.querySelectorAll('.what-changed-row')]).toHaveLength(13);
  });

  it('skips the panel silently when the comparison request fails, leaving the report intact', async () => {
    const fetchMock = vi.fn((input: string) => {
      if (input.endsWith('/scans/scan-1')) return Promise.resolve(json(scanFixture({ fixed_count: 4 })));
      if (input.endsWith('/scans/scan-1/fixed')) return Promise.resolve(json({ error: { code: 'COMPARISON_UNAVAILABLE', message: 'No baseline.' } }, 503));
      if (input.endsWith('/scans/scan-1/report')) return Promise.resolve(json({ scan: scanFixture({ fixed_count: 4 }), comparison: {}, warnings: [], findings: [] }));
      if (input.includes('/scans/scan-1/findings')) return Promise.resolve(json({ items: [], total: 0, limit: 25, offset: 0, has_more: false }));
      return Promise.resolve(json({}));
    });
    vi.stubGlobal('fetch', fetchMock);
    vi.stubGlobal('EventSource', FakeEventSource);
    const host = document.createElement('div');
    document.body.append(host);
    root = createRoot(host);
    await act(async () => { root.render(<ScanPage id="scan-1" go={vi.fn<(route: Route) => void>()} notify={vi.fn<(notice: Notice) => void>()} />); });
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    expect(host.querySelector('.what-changed')).toBeNull();
    expect(host.querySelector('.report')).not.toBeNull();
  });
});

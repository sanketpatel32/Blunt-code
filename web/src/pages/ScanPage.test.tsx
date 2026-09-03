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

/** Records every listener so tests can push server events (history replays
 *  and live updates) into the page exactly as the SSE endpoint would. */
class DispatchingEventSource {
  static instances: DispatchingEventSource[] = [];
  private listeners = new Map<string, Array<(event: { data: string }) => void>>();
  constructor(public url: string) { DispatchingEventSource.instances.push(this); }
  addEventListener(type: string, handler: (event: { data: string }) => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), handler]);
  }
  close() {}
  dispatch(type: string, data: unknown) {
    for (const handler of this.listeners.get(type) ?? []) handler({ data: JSON.stringify(data) });
  }
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

/** The scan record arrives mid-scan, so the page connects once while it is
 *  still loading and reconnects when the state resolves — both connections
 *  are replayed the same history, which used to be appended twice. */
async function renderLiveScanPage(scan: Scan) {
  const fetchMock = vi.fn((input: string) => {
    if (input.endsWith(`/scans/${scan.id}`)) return Promise.resolve(json(scan));
    return Promise.resolve(json({}));
  });
  vi.stubGlobal('fetch', fetchMock);
  vi.stubGlobal('EventSource', DispatchingEventSource);
  DispatchingEventSource.instances = [];
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<ScanPage id={scan.id} go={vi.fn<(route: Route) => void>()} notify={vi.fn<(notice: Notice) => void>()} />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return { host, fetchMock };
}

function replayHistory(source: DispatchingEventSource) {
  source.dispatch('connected', {});
  source.dispatch('scan.stage', { type: 'scan.stage', data: { stage: 'Preparing workspace' } });
  source.dispatch('analyzer.completed', { type: 'analyzer.completed', data: { analyzer_id: 'biome', findings: 3, severities: { low: 3 } } });
  source.dispatch('analyzer.completed', { type: 'analyzer.completed', data: { analyzer_id: 'gitleaks-secrets', findings: 10768, severities: { critical: 9460, high: 1308 } } });
}

const liveFixture = (): Scan => ({
  ...scanFixture({ state: 'running', total_findings: 0, started_at: '2026-09-03T00:00:00Z', finished_at: null }),
  analyzer_runs: [
    { analyzer_id: 'biome', status: 'succeeded' },
    { analyzer_id: 'gitleaks-secrets', status: 'succeeded' },
    { analyzer_id: 'semgrep', status: 'running' },
  ],
});

/** Locale number grouping varies ("10,771" vs "10.771"), so tests compare digits only. */
const digits = (text: string | null | undefined) => (text ?? '').replace(/[^\d]/g, '');

describe('ScanPage live results panel', () => {
  it('counts replayed history once even though both connections are replayed it', async () => {
    const { host } = await renderLiveScanPage(liveFixture());
    const [first, second] = DispatchingEventSource.instances;
    expect(second).toBeDefined();
    await act(async () => { replayHistory(first); });
    await act(async () => { replayHistory(second); });
    const metric = digits(host.querySelector('.scan-metric-value')?.textContent);
    expect(metric).toBe('10771');
    expect(metric).not.toBe('21542');
    const flow = [...host.querySelectorAll('.scan-flow li strong')].map((node) => node.textContent);
    expect(flow.filter((text) => text?.includes('biome finished'))).toHaveLength(1);
    expect(flow.filter((text) => text?.includes('gitleaks-secrets finished'))).toHaveLength(1);
  });

  it('shows live severity totals and per-analyzer findings from completion events', async () => {
    const { host } = await renderLiveScanPage(liveFixture());
    expect(host.querySelector('.severity-counts')?.getAttribute('data-pending')).toBe('true');
    const [first] = DispatchingEventSource.instances;
    await act(async () => { replayHistory(first); });
    expect(host.querySelector('.severity-counts')?.getAttribute('data-pending')).toBeNull();
    expect(digits(host.querySelector('.severity-counts .critical')?.textContent)).toBe('9460');
    expect(digits(host.querySelector('.severity-counts .high')?.textContent)).toBe('1308');
    expect(digits(host.querySelector('.severity-counts .low')?.textContent)).toBe('3');
    const pills = [...host.querySelectorAll('.live-analyzers li')];
    expect(digits(pills.find((li) => li.textContent?.includes('biome'))?.querySelector('.pill-count')?.textContent)).toBe('3');
    expect(digits(pills.find((li) => li.textContent?.includes('gitleaks-secrets'))?.querySelector('.pill-count')?.textContent)).toBe('10768');
    expect(pills.find((li) => li.textContent?.includes('semgrep'))?.querySelector('.pill-count')).toBeNull();
  });

  it('recovers live totals after a reconnect re-replays history', async () => {
    const { host } = await renderLiveScanPage(liveFixture());
    const [first, second] = DispatchingEventSource.instances;
    await act(async () => { replayHistory(first); });
    await act(async () => { replayHistory(second); });
    await act(async () => { second.dispatch('analyzer.completed', { type: 'analyzer.completed', data: { analyzer_id: 'semgrep', findings: 5, severities: { medium: 5 } } }); });
    expect(digits(host.querySelector('.scan-metric-value')?.textContent)).toBe('10776');
    expect(digits(host.querySelector('.severity-counts .medium')?.textContent)).toBe('5');
  });
});

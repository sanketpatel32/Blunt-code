import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { date } from '../lib/format';
import type { GlobalStats } from '../types';
import { StatsOverview } from './StatsOverview';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function anHourAgo() {
  return new Date(Date.now() - 3_600_000).toISOString();
}

/** The documented /api/v1/stats shape (internal/api/stats.go): flat counters, nested scans/findings, optional tools. */
function statsPayload(overrides: Partial<GlobalStats> = {}): GlobalStats {
  return {
    workspaces: 3,
    scans: { total: 5, completed: 3, running: 1 },
    findings: { severity: { critical: 2, high: 0, medium: 1, low: 0, info: 0 }, total: 3 },
    suppressions: 3,
    tools: { total: 4, ready: 3 },
    generated_at: anHourAgo(),
    ...overrides,
  };
}

async function render(fetchMock: ReturnType<typeof vi.fn>) {
  vi.stubGlobal('fetch', fetchMock);
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<StatsOverview />); });
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
  return host;
}

function findButton(host: HTMLElement, text: string) {
  return [...host.querySelectorAll('button')].find((button) => button.textContent === text);
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

describe('StatsOverview', () => {
  it('requests /api/v1/stats and shows the card skeleton while it loads', async () => {
    const fetchMock = vi.fn(() => new Promise<Response>(() => {}));
    const host = await render(fetchMock);
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/stats', expect.anything());
    const section = host.querySelector('.stats-overview');
    expect(section?.getAttribute('aria-busy')).toBe('true');
    // The skeleton body carries the same .stats-cards floor as the loaded grid so the swap does not shift layout.
    expect(host.querySelector('.stats-overview-loading.stats-cards')).not.toBeNull();
    expect(host.querySelectorAll('.stats-overview-loading .skeleton-card').length).toBe(5);
    expect(host.querySelector('.stats-grid')).toBeNull();
  });

  it('renders every counter, the scans split, the tools split, and the severity breakdown', async () => {
    const host = await render(vi.fn().mockResolvedValue(json(statsPayload())));
    expect(host.querySelector('.stats-grid.stats-cards')).not.toBeNull();
    const cards = [...host.querySelectorAll('.stats-grid .summary-card')];
    expect(cards).toHaveLength(5);
    // textContent includes the sr-only long description that follows each label.
    expect(cards.map((card) => card.textContent)).toEqual([
      '3WorkspacesRegistered codebases available for scanning.',
      '5ScansTotal scans run across every workspace; the split counts completed runs and runs still in progress.3 completed · 1 running',
      '3FindingsFindings reported by the latest completed scan of each workspace, summed across all severities.Latest scan per workspace',
      '3SuppressionsFinding fingerprints currently hidden from future scans, reports, and the CI gate.',
      '3 of 4Tools readyAnalyzer tools reporting ready out of everything installed.',
    ]);
    // Every counter renders in tabular numerals and every card explains itself to screen readers.
    expect(cards.every((card) => card.querySelector('strong')?.className === 'tnum')).toBe(true);
    expect(cards.every((card) => card.querySelector('.sr-only'))).toBe(true);
    expect(host.querySelector('.stats-grid .summary-card .pulse-dot')).not.toBeNull(); // running > 0 earns the live dot

    const bar = host.querySelector<HTMLElement>('.stats-distribution .severity-bar')!;
    expect(bar.getAttribute('role')).toBe('img');
    expect(bar.getAttribute('aria-label')).toBe('Current findings by severity: 2 critical, 1 medium');
    const segments = [...bar.querySelectorAll('i')];
    expect(segments.map((segment) => segment.className)).toEqual(['seg-critical', 'seg-medium']);
    expect(segments.map((segment) => segment.style.width)).toEqual(['66.7%', '33.3%']);

    // The counted legend keeps the breakdown readable without color (labels + numbers, zeros dimmed).
    const legend = [...host.querySelectorAll('.severity-legend li')];
    expect(legend.map((item) => item.textContent)).toEqual(['critical2', 'high0', 'medium1', 'low0', 'info0']);
    expect(legend.map((item) => item.className)).toEqual(['critical', 'zero', 'medium', 'zero', 'zero']);

    const heading = host.querySelector('.stats-overview .section-head p')!;
    expect(heading.textContent).toContain('updated 1 hour ago');
    expect(heading.querySelector('span')?.getAttribute('title')).toBe(date(anHourAgo()));
  });

  it('omits the tools card when the payload has no tools pair', async () => {
    const withoutTools = statsPayload();
    delete withoutTools.tools;
    const host = await render(vi.fn().mockResolvedValue(json(withoutTools)));
    const cards = [...host.querySelectorAll('.stats-grid .summary-card')];
    expect(cards).toHaveLength(4);
    expect(cards.some((card) => card.textContent?.includes('Tools ready'))).toBe(false);
  });

  it('shows zeroed cards and an empty severity track on a fresh install', async () => {
    const payload = statsPayload({
      workspaces: 0,
      scans: { total: 0, completed: 0, running: 0 },
      findings: { severity: { critical: 0, high: 0, medium: 0, low: 0, info: 0 }, total: 0 },
      suppressions: 0,
      tools: { total: 4, ready: 0 },
    });
    const host = await render(vi.fn().mockResolvedValue(json(payload)));
    const cards = [...host.querySelectorAll('.stats-grid .summary-card')];
    expect(cards).toHaveLength(5); // zeros render, nothing hides
    expect(cards.map((card) => card.textContent)).toEqual([
      '0WorkspacesRegistered codebases available for scanning.',
      '0ScansTotal scans run across every workspace; the split counts completed runs and runs still in progress.0 completed · 0 running',
      '0FindingsFindings reported by the latest completed scan of each workspace, summed across all severities.Latest scan per workspace',
      '0SuppressionsFinding fingerprints currently hidden from future scans, reports, and the CI gate.',
      '0 of 4Tools readyAnalyzer tools reporting ready out of everything installed.',
    ]);
    expect(host.querySelector('.stats-grid .summary-card .pulse-dot')).toBeNull();
    const bar = host.querySelector<HTMLElement>('.stats-distribution .severity-bar')!;
    expect(bar.getAttribute('aria-label')).toBe('Current findings by severity: none yet');
    expect(bar.querySelectorAll('i')).toHaveLength(0); // empty track, legend still counts the zeros
    expect([...host.querySelectorAll('.severity-legend li')].every((item) => item.className === 'zero')).toBe(true);
  });

  it('shows a quiet inline error with retry and recovers without blocking', async () => {
    const fetchMock = vi.fn()
      .mockRejectedValueOnce(new Error('backend offline'))
      .mockResolvedValue(json(statsPayload()));
    const host = await render(fetchMock);
    expect(host.textContent).toContain('Overview is unavailable right now.');
    expect(host.querySelector('.error-panel')).toBeNull();
    expect(host.querySelector('.stats-grid')).toBeNull();

    await act(async () => { findButton(host, 'Try again')!.click(); await Promise.resolve(); await Promise.resolve(); });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(host.querySelector('.stats-grid .summary-card')?.textContent).toBe('3WorkspacesRegistered codebases available for scanning.');
  });
});

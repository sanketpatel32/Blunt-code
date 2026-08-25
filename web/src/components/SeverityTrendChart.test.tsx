import { act, type ReactElement } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { date } from '../lib/format';
import type { SeverityTrendPoint } from '../types';
import { SeverityTrendChart, SeverityTrendSection } from './SeverityTrendChart';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function point(overrides: Partial<SeverityTrendPoint> & Pick<SeverityTrendPoint, 'scan_id'>): SeverityTrendPoint {
  return { state: 'completed', severity: { critical: 0, high: 0, medium: 0, low: 0, info: 0 }, total: 0, ...overrides };
}

async function render(element: ReactElement) {
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(element); });
  return host;
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

describe('SeverityTrendChart bars', () => {
  it('renders one bar per scan in time order, oldest to newest', async () => {
    const host = await render(<SeverityTrendChart points={[
      point({ scan_id: 'scan-old', finished_at: '2026-07-01T12:00:00Z', total: 1, severity: { critical: 0, high: 0, medium: 0, low: 1, info: 0 } }),
      point({ scan_id: 'scan-new', finished_at: '2026-07-02T12:00:00Z', total: 2, severity: { critical: 1, high: 1, medium: 0, low: 0, info: 0 } }),
    ]} />);
    const bars = [...host.querySelectorAll('.trend-bar')];
    expect(bars).toHaveLength(2);
    // The DOM order of the title tooltips is the chart's time axis.
    const titles = bars.map((bar) => bar.querySelector('title')?.textContent ?? '');
    expect(titles[0]).toContain(date('2026-07-01T12:00:00Z'));
    expect(titles[1]).toContain(date('2026-07-02T12:00:00Z'));
    expect(host.querySelectorAll('.trend-baseline')).toHaveLength(1);
  });

  it('stacks segments in the fixed severity order with proportional heights', async () => {
    const host = await render(<SeverityTrendChart points={[point({
      scan_id: 'scan-1',
      severity: { critical: 2, high: 1, medium: 1, low: 0, info: 0 },
      total: 4,
    })]} />);
    const segments = [...host.querySelectorAll('.trend-bar rect')];
    expect(segments.map((segment) => segment.getAttribute('class'))).toEqual(['seg-critical', 'seg-high', 'seg-medium']);
    // Viewbox height 150 with a 4-unit baseline and max total 4 gives 36.5 units per finding.
    expect(segments.map((segment) => Number(segment.getAttribute('height')))).toEqual([73, 36.5, 36.5]);
    expect(segments.map((segment) => Number(segment.getAttribute('y')))).toEqual([73, 36.5, 0]);
  });

  it('marks a completed scan with zero findings as a baseline sliver instead of nothing', async () => {
    const host = await render(<SeverityTrendChart points={[point({ scan_id: 'scan-clean' })]} />);
    const segments = [...host.querySelectorAll('.trend-bar rect')];
    expect(segments.map((segment) => segment.getAttribute('class'))).toEqual(['seg-zero']);
  });

  it('labels the chart for screen readers and titles every bar with date and per-severity counts', async () => {
    const finished = '2026-07-01T12:00:00Z';
    const host = await render(<SeverityTrendChart points={[point({
      scan_id: 'scan-1',
      finished_at: finished,
      profile: 'deep',
      severity: { critical: 2, high: 1, medium: 1, low: 0, info: 0 },
      total: 4,
    })]} />);
    const svg = host.querySelector('svg');
    expect(svg?.getAttribute('role')).toBe('img');
    expect(svg?.getAttribute('aria-label')).toContain('1 completed scan');
    const title = host.querySelector('.trend-bar title')?.textContent ?? '';
    expect(title).toContain(date(finished));
    expect(title).toContain('4 findings');
    expect(title).toContain('2 critical, 1 high, 1 medium, 0 low, 0 info');
    expect(title).toContain('deep');
    expect(host.querySelector('figcaption')?.className).toBe('sr-only');
  });

  it('renders nothing without completed scans', async () => {
    const host = await render(<SeverityTrendChart points={[]} />);
    expect(host.querySelector('svg')).toBeNull();
  });
});

describe('SeverityTrendChart tooltips and axes', () => {
  const points = [
    point({ scan_id: 'scan-old', finished_at: '2026-07-01T12:00:00Z', total: 1, severity: { critical: 0, high: 0, medium: 0, low: 1, info: 0 } }),
    point({ scan_id: 'scan-new', finished_at: '2026-07-02T12:00:00Z', profile: 'deep', total: 4, severity: { critical: 2, high: 1, medium: 1, low: 0, info: 0 } }),
  ];

  it('reveals a tooltip when a bar receives keyboard focus and hides it on blur', async () => {
    const host = await render(<SeverityTrendChart points={points} />);
    expect(host.querySelector('.trend-tooltip')).toBeNull();
    const bar = host.querySelector<SVGGraphicsElement>('.trend-bar');
    expect(bar).not.toBeNull();
    await act(async () => { bar?.focus(); });
    const tooltip = host.querySelector('.trend-tooltip');
    expect(tooltip?.textContent).toContain(date('2026-07-01T12:00:00Z'));
    expect(tooltip?.textContent).toContain('1 finding');
    expect(tooltip?.textContent).toContain('1 low');
    expect(bar?.getAttribute('aria-label')).toContain('1 finding');
    await act(async () => { bar?.blur(); });
    expect(host.querySelector('.trend-tooltip')).toBeNull();
  });

  it('reveals the same tooltip on hover', async () => {
    const host = await render(<SeverityTrendChart points={points} />);
    const bar = [...host.querySelectorAll('.trend-bar')][1];
    await act(async () => { bar.dispatchEvent(new MouseEvent('mouseover', { bubbles: true })); });
    const tooltip = host.querySelector('.trend-tooltip');
    expect(tooltip?.textContent).toContain(date('2026-07-02T12:00:00Z'));
    expect(tooltip?.textContent).toContain('4 findings');
    expect(tooltip?.textContent).toContain('2 critical');
    expect(host.querySelector('.trend-bar.is-active')).toBe(bar);
    // The overlay is anchored over its own slot: bar 2 of 2 centers at 75% of the viewBox.
    expect((host.querySelector('.trend-tooltip') as HTMLElement).style.left).toBe('75%');
    await act(async () => { bar.dispatchEvent(new MouseEvent('mouseout', { bubbles: true })); });
    expect(host.querySelector('.trend-tooltip')).toBeNull();
  });

  it('marks the baseline with date ticks at both ends of the timeline', async () => {
    const host = await render(<SeverityTrendChart points={points} />);
    expect(host.querySelectorAll('.trend-chart .trend-tick')).toHaveLength(2);
    const ticks = [...host.querySelectorAll('.trend-ticks span')];
    expect(ticks.map((tick) => tick.textContent)).toEqual([date('2026-07-01T12:00:00Z'), date('2026-07-02T12:00:00Z')]);
    expect(host.querySelector('.trend-ticks')?.getAttribute('aria-hidden')).toBe('true'); // dates already live in the caption
  });

  it('describes the trend direction in the accessible summary', async () => {
    const host = await render(<SeverityTrendChart points={points} />);
    expect(host.querySelector('svg')?.getAttribute('aria-label')).toContain('Up from 1 finding in the previous scan.');

    const falling = await render(<SeverityTrendChart points={[points[1], point({ scan_id: 'scan-later', finished_at: '2026-07-03T12:00:00Z', total: 0 })]} />);
    expect(falling.querySelector('svg')?.getAttribute('aria-label')).toContain('Down from 4 findings in the previous scan.');
  });

  it('keeps the tooltip quiet for screen readers while bars stay focusable', async () => {
    const host = await render(<SeverityTrendChart points={points} />);
    const bar = host.querySelector<SVGGraphicsElement>('.trend-bar');
    expect(bar).not.toBeNull();
    await act(async () => { bar?.focus(); });
    expect(host.querySelector('.trend-tooltip')?.getAttribute('aria-hidden')).toBe('true');
    expect(bar?.getAttribute('tabindex')).toBe('0');
  });
});

describe('SeverityTrendSection states', () => {
  it('hides the section entirely when the workspace has no completed scans', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ items: [] }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    const host = await render(<SeverityTrendSection workspaceId="ws-1" />);
    await act(async () => {});
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/workspaces/ws-1/trends', expect.anything());
    expect(host.innerHTML).toBe('');
  });

  it('shows a quiet inline message with a retry, never a blocking error panel', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('backend offline')));
    const host = await render(<SeverityTrendSection workspaceId="ws-1" />);
    await act(async () => {});
    expect(host.textContent).toContain('Severity trend is unavailable right now.');
    expect(host.querySelector('.error-panel')).toBeNull();
    expect(host.querySelector('button')?.textContent).toBe('Try again');
  });

  it('renders the chart, legend, and axis once points load', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ items: [
      { scan_id: 'a', finished_at: '2026-07-01T12:00:00Z', state: 'completed', severity: { critical: 0, high: 0, medium: 0, low: 0, info: 0 }, total: 0 },
      { scan_id: 'b', finished_at: '2026-07-02T12:00:00Z', state: 'completed', severity: { critical: 1, high: 0, medium: 0, low: 0, info: 0 }, total: 1 },
    ] }), { status: 200 })));
    const host = await render(<SeverityTrendSection workspaceId="ws-1" />);
    await act(async () => {});
    expect(host.querySelectorAll('.trend-bar')).toHaveLength(2);
    expect([...host.querySelectorAll('.severity-legend li')].map((li) => li.textContent)).toEqual(['critical', 'high', 'medium', 'low', 'info']);
    expect(host.querySelector('.trend-axis')?.textContent).toContain('oldest → newest');
    expect(host.querySelector('.trend-ticks')?.textContent).toContain(date('2026-07-02T12:00:00Z'));
  });
});

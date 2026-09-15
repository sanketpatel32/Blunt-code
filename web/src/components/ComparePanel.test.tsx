import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api';
import type { CompareResult, Finding, Scan } from '../types';
import { ComparePanel } from './ComparePanel';

vi.mock('../api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api')>();
  return { ...actual, api: { ...actual.api, scan: vi.fn(), compareScans: vi.fn() } };
});

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
});

function scanOf(id: string, finishedAt: string): Scan {
  return { id, workspace_id: 'ws-1', state: 'completed', started_at: finishedAt, finished_at: finishedAt };
}

function findingOf(id: string, overrides: Partial<Finding> = {}): Finding {
  return { id, analyzer_id: 'biome', severity: 'medium', category: 'maintainability', title: `Finding ${id}`, message: 'Something to fix.', relative_path: 'src/a.ts', start_line: 12, ...overrides };
}

function comparison(overrides: Partial<CompareResult> = {}): CompareResult {
  return {
    available: true,
    current_scan_id: 'newer',
    previous_scan_id: 'older',
    summary: { new: 2, fixed: 1, persistent: 12 },
    new: [findingOf('n1', { severity: 'high' }), findingOf('n2', { severity: 'low' })],
    fixed: [findingOf('f1', { severity: 'low' })],
    persistent: Array.from({ length: 12 }, (_, index) => findingOf(`p${index}`, { analyzer_id: 'sonarqube', severity: 'low' })),
    not_evaluated: [],
    ...overrides,
  };
}

async function renderPanel(aId: string, bId: string) {
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<ComparePanel aId={aId} bId={bId} />); });
  await act(async () => {});
  return host;
}

describe('ComparePanel', () => {
  const older = scanOf('older', '2026-03-01T10:00:00Z');
  const newer = scanOf('newer', '2026-03-10T10:00:00Z');

  beforeEach(() => {
    vi.mocked(api.scan).mockReset();
    vi.mocked(api.compareScans).mockReset();
  });

  function mockScans() {
    vi.mocked(api.scan).mockImplementation(async (id) => (id === 'newer' ? newer : older));
  }

  it('orders the diff request (newer, older) regardless of the order the ids arrive in', async () => {
    mockScans();
    vi.mocked(api.compareScans).mockResolvedValue(comparison());
    // Deep links can carry the pair either way round; mount deliberately reversed.
    await renderPanel('older', 'newer');
    expect(api.compareScans).toHaveBeenCalledWith('newer', 'older');
  });

  it('renders the headline counts from the summary and one row per finding', async () => {
    mockScans();
    vi.mocked(api.compareScans).mockResolvedValue(comparison());
    const host = await renderPanel('newer', 'older');
    const headline = host.querySelector('.compare-headline')!.textContent!;
    expect(headline).toContain('Since');
    expect(headline).toContain('2 new');
    expect(headline).toContain('1 fixed');
    expect(headline).toContain('12 still present');
    const sections = [...host.querySelectorAll('.compare-section')];
    expect(sections).toHaveLength(3);
    const newSection = sections[0]!;
    expect(newSection.querySelector('.what-changed-row .severity.high')?.textContent).toBe('high');
    expect(newSection.querySelector('.what-changed-rule')?.textContent).toBe('Finding n1');
    expect(newSection.querySelector('code')?.textContent).toBe('src/a.ts:12');
    expect(newSection.querySelector('.badge')?.textContent).toBe('Biome'); // analyzerName resolves display names
  });

  it('caps sections at 10 rows with a "+N more" details expansion', async () => {
    mockScans();
    vi.mocked(api.compareScans).mockResolvedValue(comparison());
    const host = await renderPanel('newer', 'older');
    const persistent = [...host.querySelectorAll('.compare-section')].find((section) => section.querySelector('.compare-section-toggle')?.textContent!.includes('Still present'))!;
    expect(persistent.querySelectorAll(':scope > .what-changed-list > li')).toHaveLength(10);
    const details = persistent.querySelector('details.what-changed-more')!;
    expect(details.querySelector('summary')!.textContent).toBe('+2 more still present');
    expect(details.querySelectorAll('.what-changed-row')).toHaveLength(2);
  });

  it('collapses a section through its aria-expanded toggle', async () => {
    mockScans();
    vi.mocked(api.compareScans).mockResolvedValue(comparison());
    const host = await renderPanel('newer', 'older');
    const toggle = host.querySelector<HTMLButtonElement>('.compare-section-toggle')!;
    expect(toggle.getAttribute('aria-expanded')).toBe('true');
    const section = toggle.closest('.compare-section')!;
    expect(section.querySelector('.what-changed-list')).not.toBeNull();
    await act(async () => { toggle.click(); });
    expect(toggle.getAttribute('aria-expanded')).toBe('false');
    expect(section.querySelector('.what-changed-list')).toBeNull();
  });

  it('hides empty sections instead of rendering zero rows', async () => {
    mockScans();
    vi.mocked(api.compareScans).mockResolvedValue(comparison({
      summary: { new: 0, fixed: 1, persistent: 12 },
      new: [],
      not_evaluated: ['ruff'],
    }));
    const host = await renderPanel('newer', 'older');
    const labels = [...host.querySelectorAll('.compare-section-toggle')].map((toggle) => toggle.textContent!);
    expect(labels).toHaveLength(2);
    expect(labels[0]).toContain('Fixed');
    expect(labels[1]).toContain('Still present');
    expect(host.querySelector('.compare-note')?.textContent).toContain('ruff');
  });

  it('explains an unavailable comparison instead of rendering the panel', async () => {
    mockScans();
    vi.mocked(api.compareScans).mockResolvedValue({ available: false, reason: 'no previous completed scan' });
    const host = await renderPanel('newer', 'older');
    expect(host.textContent).toContain("These scans can't be compared (one has no previous completed scan).");
    expect(host.querySelector('.compare-headline')).toBeNull();
  });

  it('shows the error panel when a scan cannot be loaded', async () => {
    vi.mocked(api.scan).mockRejectedValue(new Error('SCAN_NOT_FOUND: Scan was not found.'));
    const host = await renderPanel('newer', 'missing');
    expect(api.compareScans).not.toHaveBeenCalled();
    expect(host.textContent).toContain('Could not load this view');
  });
});

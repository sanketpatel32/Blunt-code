import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, expect, it, vi } from 'vitest';
import { ReportWarnings, groupAnalyzerWarnings } from './ReportView';
import { eventCopy } from '../../lib/scanEvents';

declare global { var IS_REACT_ACT_ENVIRONMENT: boolean | undefined; }
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('missing-tool warnings', () => {
  const warnings = [
    'Ruff: ruff: No pinned managed artifact is configured.',
    'Biome: Not installed on this computer. Set it up from Tools.',
    'Gitleaks: Pinned managed artifact is not installed.',
    'Semgrep: batch 1/2 produced unparseable semgrep output',
  ];
  it('folds every not-installed wording into one group', () => {
    const groups = groupAnalyzerWarnings(warnings);
    const missing = groups.filter((group) => group.missing);
    expect(missing).toHaveLength(1);
    expect(missing[0]!.tools).toEqual(['Ruff', 'Biome', 'Gitleaks']);
    expect(groups.filter((group) => !group.missing)).toHaveLength(1);
  });
  it('renders one notice with a single Set up tools action', async () => {
    const go = vi.fn();
    const host = document.createElement('div');
    document.body.append(host);
    const root = createRoot(host);
    await act(async () => { root.render(<ReportWarnings warnings={warnings} go={go} />); });
    expect(host.textContent).toContain('3 tools aren’t installed (Ruff, Biome, Gitleaks)');
    expect(host.textContent).not.toContain('managed artifact');
    const buttons = [...host.querySelectorAll('button')].filter((button) => button.textContent?.startsWith('Set up tools'));
    expect(buttons).toHaveLength(1);
    await act(async () => { buttons[0]!.click(); });
    expect(go).toHaveBeenCalledWith({ page: 'tools' });
    act(() => root.unmount());
    host.remove();
  });
  it('words a missing tool in the live timeline as not installed', () => {
    expect(eventCopy({ type: 'analyzer.failed', analyzer_id: 'ruff', error: 'Not installed on this computer. Set it up from Tools.', at: 0 })).toBe('ruff isn’t installed · skipped (set it up from Tools)');
  });
});

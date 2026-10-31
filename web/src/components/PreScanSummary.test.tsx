import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it } from 'vitest';
import type { AnalyzerStatus } from '../types';
import { PreScanSummary } from './PreScanSummary';

declare global {
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined;
}

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let root: Root;

function status(id: string, ready: boolean): AnalyzerStatus {
  return {
    id, display_name: id, category: 'lint', execution: 'external', profiles: ['quick', 'standard', 'deep'], input_kinds: [],
    network: 'none', keep_artifact_findings: false, timeout_class: 'short', description: '',
    languages: ['python'], ready, registered: true,
  };
}

async function render(props: Parameters<typeof PreScanSummary>[0]) {
  const host = document.createElement('div');
  document.body.append(host);
  root = createRoot(host);
  await act(async () => { root.render(<PreScanSummary {...props} />); });
  return host;
}

afterEach(async () => {
  await act(async () => { root?.unmount(); });
  document.body.replaceChildren();
});

describe('PreScanSummary — what this profile will run before it runs (IMP-14)', () => {
  it('predicts the engines for the profile and workspace languages, with readiness', async () => {
    const host = await render({
      profile: 'standard',
      languages: ['python', 'yaml'],
      analyzers: [status('ruff', true), status('semgrep', false), status('sonarqube', true), status('gitleaks-secrets', true)],
      exclusionCount: 2,
    });
    const text = host.textContent ?? '';
    // python + yaml at standard: ruff, semgrep, sonarqube, gitleaks, secrets,
    // pentest, todo — license-scan's languages (json/toml/markdown/text) miss.
    expect(text).toContain('3 of 4 analyzers ready');
    expect(host.querySelector('.pre-scan-groups')?.textContent).toContain('Needs setupsemgrep');
    expect(text).toContain('2 exclusions');
    expect(host.querySelector('.pre-scan-groups')?.textContent).toContain('Readyruff, sonarqube, gitleaks-secrets');
  });

  it('claims all ready when every matched engine is installed', async () => {
    const host = await render({ profile: 'quick', languages: ['python'], analyzers: [status('ruff', true), status('biome', true)], exclusionCount: 0 });
    expect(host.textContent).toContain('2 of 2 analyzers ready');
    expect(host.textContent).toContain('0 exclusions');
  });

  it('warns when no engine matches the languages — the scan would select nothing', async () => {
    // Universal engines (secrets, todo, gitleaks, pentest) match every
    // classified language, so the only real empty case is a workspace where
    // discovery detected nothing.
    const host = await render({ profile: 'deep', languages: [], analyzers: [], exclusionCount: 0 });
    expect(host.querySelector('.pre-scan-summary')?.getAttribute('data-tone')).toBe('warning');
    expect(host.textContent).toContain("No analyzers match this project's languages yet.");
  });
});

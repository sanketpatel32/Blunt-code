import { Filter, Gauge } from 'lucide-react';
import { Ring } from './viz/Ring';
import type { AnalyzerStatus } from '../types';

/** This compact inventory is advisory; the scan review discovers actual inputs.
 *  One headline, then the engines grouped by readiness — a list of names reads
 *  faster than "Name (state) · Name (state)" repeated eight times. */
export function PreScanSummary({ profile, languages, analyzers, exclusionCount }: {
  profile: string; languages?: string[]; analyzers: AnalyzerStatus[]; exclusionCount: number;
}) {
  const languageSet = new Set((languages ?? []).map((language) => language.toLowerCase()));
  const eligible = analyzers.filter((row) => row.profiles.includes(profile));
  const sourceMatches = eligible.filter((row) => row.languages.some((language) => languageSet.has(language.toLowerCase())));
  const ready = sourceMatches.filter((row) => row.ready && row.registered);
  const setup = sourceMatches.filter((row) => row.registered && !row.ready);
  const unavailable = sourceMatches.filter((row) => !row.registered);
  const allReady = ready.length === sourceMatches.length && sourceMatches.length > 0;
  return <div className="pre-scan-summary" data-tone={allReady ? 'ok' : 'warning'}>
    <div className="pre-scan-head">
      <Ring value={ready.length} max={sourceMatches.length} size={44} stroke={5} tone={allReady ? 'var(--color-success)' : 'var(--sev-fill-medium)'} label={`${ready.length} of ${sourceMatches.length} analyzers ready`} />
      <p className="pre-scan-headline"><span className="sr-only">{ready.length} of {sourceMatches.length} analyzers ready</span><strong aria-hidden="true">Analyzers</strong></p>
      <span className="pre-scan-chips">
        <span className="viz-card-chip" title="Scan profile"><Gauge className="h-3 w-3" aria-hidden="true" />{profile}</span>
        <span className="viz-card-chip" title="Active exclusion rules"><Filter className="h-3 w-3" aria-hidden="true" />{exclusionCount} exclusion{exclusionCount === 1 ? '' : 's'}</span>
      </span>
    </div>
    {sourceMatches.length === 0
      ? <p className="pre-scan-engine-list">No analyzers match this project's languages yet.</p>
      : <dl className="pre-scan-groups">
          {ready.length > 0 && <div data-state="ready"><dt>Ready</dt><dd>{ready.map((row) => row.display_name).join(', ')}</dd></div>}
          {setup.length > 0 && <div data-state="setup"><dt>Needs setup</dt><dd>{setup.map((row) => row.display_name).join(', ')}</dd></div>}
          {unavailable.length > 0 && <div data-state="off"><dt>Unavailable</dt><dd>{unavailable.map((row) => row.display_name).join(', ')}</dd></div>}
        </dl>}
  </div>;
}

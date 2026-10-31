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
    <p className="pre-scan-headline"><strong>{ready.length} of {sourceMatches.length}</strong> analyzers ready for this project <span className="pre-scan-meta">· {profile} profile · {exclusionCount} exclusion{exclusionCount === 1 ? '' : 's'}</span></p>
    {sourceMatches.length === 0
      ? <p className="pre-scan-engine-list">No analyzers match this project's languages yet.</p>
      : <dl className="pre-scan-groups">
          {ready.length > 0 && <div><dt>Ready</dt><dd>{ready.map((row) => row.display_name).join(', ')}</dd></div>}
          {setup.length > 0 && <div><dt>Needs setup</dt><dd>{setup.map((row) => row.display_name).join(', ')}</dd></div>}
          {unavailable.length > 0 && <div><dt>Unavailable</dt><dd>{unavailable.map((row) => row.display_name).join(', ')}</dd></div>}
        </dl>}
    <p className="pre-scan-engine-list">Run scan shows the exact files, dependencies, and network use before anything starts.</p>
  </div>;
}

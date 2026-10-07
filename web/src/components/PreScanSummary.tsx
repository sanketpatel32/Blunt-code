import type { AnalyzerStatus } from '../types';

/** This compact inventory is advisory; the scan review discovers actual inputs. */
export function PreScanSummary({ profile, languages, analyzers, exclusionCount }: {
  profile: string; languages?: string[]; analyzers: AnalyzerStatus[]; exclusionCount: number;
}) {
  const languageSet = new Set((languages ?? []).map((language) => language.toLowerCase()));
  const eligible = analyzers.filter((row) => row.profiles.includes(profile));
  const sourceMatches = eligible.filter((row) => row.languages.some((language) => languageSet.has(language.toLowerCase())));
  const ready = sourceMatches.filter((row) => row.ready && row.registered);
  return <div className="pre-scan-summary" data-tone={ready.length === sourceMatches.length && sourceMatches.length ? 'ok' : 'warning'}>
    <strong>{ready.length}</strong> ready of {sourceMatches.length} language-matched analyzers in the {profile} profile · {exclusionCount} exclusion{exclusionCount === 1 ? '' : 's'} active
    <span className="pre-scan-engine-list">{sourceMatches.map((row) => `${row.display_name} (${!row.registered ? 'unavailable' : row.ready ? 'ready' : 'setup required'})`).join(' · ') || 'No language-matched analyzers reported.'}</span>
    <span className="pre-scan-engine-list">Advisory inventory only. Run scan opens a review of current file selection, dependency inputs, and network use. Missing status is not counted as ready.</span>
  </div>;
}

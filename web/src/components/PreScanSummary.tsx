import { ANALYZER_CATALOG, type AnalyzerMeta } from '../lib/analyzerCatalog';
import type { AnalyzerStatus } from '../types';

/**
 * Pre-flight answer to "what will this scan actually do?" (IMP-14): given the
 * selected profile and the workspace's detected languages, name the engines
 * expected to run, which of them are installed, and any exclusions that will
 * shape the selection — before the scan starts, not after it surprises.
 *
 * The prediction is the client-side catalog (enabledByProfile × languages),
 * which mirrors the server's routing table; readiness is the server's own
 * live answer. A mismatch between the two lists is exactly the kind of
 * silent gap this card exists to surface.
 */
export function PreScanSummary({
  profile,
  languages,
  analyzers,
  exclusionCount,
}: {
  profile: string;
  languages?: string[];
  analyzers: AnalyzerStatus[];
  exclusionCount: number;
}) {
  const languageSet = new Set((languages ?? []).map((language) => language.toLowerCase()));
  const expected = ANALYZER_CATALOG.filter(
    (meta) => meta.enabledByProfile.includes(profile as AnalyzerMeta['enabledByProfile'][number]) &&
      meta.languages.some((language) => languageSet.has(language.toLowerCase())),
  );
  if (expected.length === 0) {
    return (
      <p className="pre-scan-summary" data-tone="warning">
        No engines match the {profile} profile on this workspace's languages
        {languages?.length ? ` (${languages.join(', ')})` : ' (none detected)'} — a scan would select no supported inputs.
      </p>
    );
  }
  const readyById = new Map(analyzers.map((status) => [status.id, status.ready]));
  const missing = expected.filter((meta) => readyById.get(meta.id) === false);
  const readyCount = expected.length - missing.length;
  return (
    <p className="pre-scan-summary" data-tone={missing.length ? 'warning' : 'ok'}>
      <strong>{expected.length}</strong> engine{expected.length === 1 ? '' : 's'} queued for the {profile} profile
      {missing.length > 0
        ? <> — <strong>{readyCount}</strong> ready, {missing.length} not installed ({missing.map((meta) => meta.displayName).join(', ')})</>
        : ' — all ready'}
      {exclusionCount > 0 && <> · {exclusionCount} exclusion{exclusionCount === 1 ? '' : 's'} active</>}
      <span className="pre-scan-engine-list">
        {expected.map((meta) => `${meta.displayName}${readyById.get(meta.id) === false ? ' (not installed)' : ''}`).join(' · ')}
      </span>
    </p>
  );
}

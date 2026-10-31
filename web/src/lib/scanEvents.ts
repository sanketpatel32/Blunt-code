import { analyzerName, count, isMissingToolMessage } from './format';

export type ScanEvent = { type: string; stage?: string; status?: string; message?: string; analyzer_id?: string; name?: string; findings?: number; /** Per-severity finding counts the backend attaches to analyzer.completed, so live panels can show real severity totals before the final summary lands. */ severities?: Record<string, number>; error?: string; state?: string; at: number; /** Receive-order sequence assigned by the live stream; a stable React key even when two events share a millisecond. */ seq?: number };

export const stageLabels: Record<string, string> = { preparing: 'Preparing workspace', discovering: 'Detecting languages', checking_tools: 'Checking analyzers', running: 'Running analyzers', normalizing: 'Combining results', generating_report: 'Generating report', completed: 'Complete' };

/** Wording for analyzer run states on the live pills; the CSS class and any
 *  stored value keep the raw API status ('succeeded' etc.), only the label changes. */
export const analyzerStatusLabels: Record<string, string> = { succeeded: 'done', failed: 'failed', running: 'running', skipped: 'not needed' };

export function isTerminalScanState(state: string) {
  return ['completed', 'completed_with_warnings', 'failed', 'cancelled', 'interrupted'].includes(state);
}

/** A scan still in flight: every lifecycle state that is not terminal. Screens
 *  that ask "is anything running" must use this, not a hand-picked list — the
 *  scan lifecycle passes through queued, preparing, installing_tools,
 *  discovering, normalizing, and generating_report, and a list that names only
 *  some of them goes quiet for minutes exactly when a scan is deepest in
 *  discovery. Unknown/hostile states read as active: an unrecognized state is
 *  not proof the scan finished. */
export function isActiveScanState(state: string) {
  return !isTerminalScanState(state);
}

/** Analyzer progress folded from the event log: which analyzers have started
 *  and which reached an outcome. Analyzers run concurrently, so progress is a
 *  count — never "the" analyzer that is running. */
export function analyzerProgress(events: ScanEvent[]) {
  const started = new Set<string>();
  const finished = new Set<string>();
  const failed = new Set<string>();
  for (const event of events) {
    const id = event.analyzer_id ?? event.name;
    if (!id || !event.type.startsWith('analyzer.')) continue;
    started.add(id);
    if (event.type === 'analyzer.completed' || event.type === 'analyzer.skipped') finished.add(id);
    if (event.type === 'analyzer.failed') { finished.add(id); failed.add(id); }
  }
  return { started: started.size, finished: finished.size, failed: failed.size };
}

/** The pre-analyzer lifecycle states. Once an analyzer event arrives the scan
 *  is running whatever the last persisted state said, so the badge must not
 *  read "Preparing" over a list of running analyzers. */
const SETUP_STATES = new Set(['queued', 'preparing', 'installing_tools', 'discovering', 'checking_tools']);

export function effectiveLiveState(events: ScanEvent[], state: string) {
  return SETUP_STATES.has(state) && analyzerProgress(events).started > 0 ? 'running' : state;
}

/** One headline for a live scan, consistent with the state badge and the
 *  event list: a setup stage until analyzers start, then a running count. */
export function liveHeadline(events: ScanEvent[], state: string) {
  const progress = analyzerProgress(events);
  const live = effectiveLiveState(events, state);
  if (progress.started > 0 && (live === 'running' || live === 'normalizing' || live === 'generating_report')) {
    if (live !== 'running') return stageLabels[live];
    const failed = progress.failed ? ` · ${progress.failed} need${progress.failed === 1 ? 's' : ''} attention` : '';
    return `Running analyzers · ${progress.finished} of ${progress.started} finished${failed}`;
  }
  return stageLabels[live] ?? events.at(-1)?.stage ?? 'Getting your scan ready';
}

export function eventCopy(event: ScanEvent) {
  const who = event.name ?? (event.analyzer_id ? analyzerName(event.analyzer_id) : 'Analyzer');
  if (event.type === 'analyzer.started') return `${who} started`;
  if (event.type === 'analyzer.completed') return `${who} finished${event.findings === undefined ? '' : ` · ${count(event.findings)} findings`}`;
  if (event.type === 'analyzer.failed' && isMissingToolMessage(event.error)) return `${who} isn’t installed · skipped (set it up from Tools)`;
  if (event.type === 'analyzer.failed') return `${who} needs attention${event.error ? ` · ${event.error}` : ''}`;
  if (event.type === 'analyzer.skipped') return `${who} skipped${event.message ? ` · ${event.message}` : ''}`;
  if (event.type === 'scan.cancelled') return 'Scan cancelled';
  if (event.type === 'scan.completed') return 'Report complete';
  return event.stage ?? event.message ?? 'Scan update';
}

/** Last analyzer.completed event per analyzer. Each analyzer completes at most
 *  once per scan, so keying by analyzer id collapses replayed history (the
 *  server resends it to every subscriber) and keeps "results so far" totals
 *  stable across reconnects instead of double-counting them. */
export function latestAnalyzerCompletions(events: ScanEvent[]): Map<string, ScanEvent> {
  const byAnalyzer = new Map<string, ScanEvent>();
  for (const event of events) {
    if (event.type !== 'analyzer.completed') continue;
    const id = event.analyzer_id ?? event.name;
    if (id) byAnalyzer.set(id, event);
  }
  return byAnalyzer;
}

/** Live per-severity totals summed over each analyzer's completion payload.
 *  Unknown severity keys in a payload are ignored rather than guessed. */
export function severityTotalsSoFar(events: ScanEvent[]): { critical: number; high: number; medium: number; low: number; info: number } {
  const totals = { critical: 0, high: 0, medium: 0, low: 0, info: 0 };
  for (const event of latestAnalyzerCompletions(events).values()) {
    for (const [severity, value] of Object.entries(event.severities ?? {})) {
      if (severity in totals) totals[severity as keyof typeof totals] += value;
    }
  }
  return totals;
}

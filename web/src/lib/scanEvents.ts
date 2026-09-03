import { count } from './format';

export type ScanEvent = { type: string; stage?: string; status?: string; message?: string; analyzer_id?: string; name?: string; findings?: number; /** Per-severity finding counts the backend attaches to analyzer.completed, so live panels can show real severity totals before the final summary lands. */ severities?: Record<string, number>; error?: string; state?: string; at: number; /** Receive-order sequence assigned by the live stream; a stable React key even when two events share a millisecond. */ seq?: number };

export const stageLabels: Record<string, string> = { preparing: 'Preparing workspace', discovering: 'Detecting languages', checking_tools: 'Checking analyzers', running: 'Running analyzers', normalizing: 'Normalizing findings', generating_report: 'Generating report', completed: 'Complete' };

export function isTerminalScanState(state: string) {
  return ['completed', 'completed_with_warnings', 'failed', 'cancelled', 'interrupted'].includes(state);
}

export function liveHeadline(events: ScanEvent[], state: string) {
  const latest = events.at(-1);
  if (latest?.type === 'analyzer.started') return `${latest.name ?? latest.analyzer_id ?? 'Analyzer'} is checking your code`;
  if (latest?.type === 'analyzer.completed') return `${latest.analyzer_id ?? 'Analyzer'} finished its pass`;
  if (latest?.stage) return latest.stage;
  return stageLabels[state] ?? 'Getting your scan ready';
}

export function eventCopy(event: ScanEvent) {
  if (event.type === 'analyzer.started') return `${event.name ?? event.analyzer_id ?? 'Analyzer'} started`;
  if (event.type === 'analyzer.completed') return `${event.analyzer_id ?? 'Analyzer'} finished${event.findings === undefined ? '' : ` · ${count(event.findings)} findings`}`;
  if (event.type === 'analyzer.failed') return `${event.analyzer_id ?? 'Analyzer'} needs attention${event.error ? ` · ${event.error}` : ''}`;
  if (event.type === 'analyzer.skipped') return `${event.analyzer_id ?? 'Analyzer'} skipped${event.message ? ` · ${event.message}` : ''}`;
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

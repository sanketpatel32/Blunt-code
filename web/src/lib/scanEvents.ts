export type ScanEvent = { type: string; stage?: string; status?: string; message?: string; analyzer_id?: string; name?: string; findings?: number; error?: string; state?: string; at: number; /** Receive-order sequence assigned by the live stream; a stable React key even when two events share a millisecond. */ seq?: number };

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
  if (event.type === 'analyzer.completed') return `${event.analyzer_id ?? 'Analyzer'} finished${event.findings === undefined ? '' : ` · ${event.findings} findings`}`;
  if (event.type === 'analyzer.failed') return `${event.analyzer_id ?? 'Analyzer'} needs attention${event.error ? ` · ${event.error}` : ''}`;
  if (event.type === 'analyzer.skipped') return `${event.analyzer_id ?? 'Analyzer'} skipped${event.message ? ` · ${event.message}` : ''}`;
  if (event.type === 'scan.cancelled') return 'Scan cancelled';
  if (event.type === 'scan.completed') return 'Report complete';
  return event.stage ?? event.message ?? 'Scan update';
}

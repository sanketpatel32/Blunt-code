import { useState } from 'react';
import '../css/tools.css';
import { api } from '../api';
import type { AnalyzerStatus } from '../types';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel } from '../components/ui';
import { WrenchIcon } from '../components/icons';
import { SkeletonTable } from '../components/skeletons';
import { analyzerMeta, categoryColor, ANALYZER_CATALOG, type AnalyzerCategory } from '../lib/analyzerCatalog';
import { LanguageCoverage } from '../components/LanguageCoverage';
import { ConfirmationDialog } from '../components/dialogs';
import type { Route } from '../lib/router';
import { PageHeader } from '../components/PageHeader';
import { Badge } from '../components/ui/badge';
import { Wrench, Shield, Bug, KeyRound, Boxes, Container, FileCog, Scale, Palette, Zap, Crosshair, Radar, Package, ListTodo, Gauge } from 'lucide-react';

type ToolOperation = 'install' | 'repair' | 'update';
type BusyAction = { tool: string; operation: ToolOperation };
type PendingAction = { analyzer: AnalyzerStatus; operation: ToolOperation };

const operationLabels: Record<ToolOperation, string> = { install: 'Install', repair: 'Repair', update: 'Update' };
const operationBusyLabels: Record<ToolOperation, string> = { install: 'Installing…', repair: 'Repairing…', update: 'Updating…' };
const operationVerbs: Record<ToolOperation, string> = { install: 'installed', repair: 'repaired', update: 'updated' };

/** Confirm-dialog copy naming the tool + operation, so a long re-download
 *  never starts from a single mis-click. */
const operationConfirmCopy: Record<ToolOperation, (name: string) => string> = {
  install: (name) => `Install downloads and sets up ${name} on this machine.`,
  repair: (name) => `Repair re-runs setup for ${name}, re-downloading the managed tool if needed.`,
  update: (name) => `Update fetches and applies the latest managed ${name} release.`,
};

/** The backend capability inventory's category vocabulary (internal/analyzers)
 *  with human labels; unknown values fall back to the raw string. The icon and
 *  hue stay purely visual, from the frontend catalog below. */
const API_CATEGORY_LABELS: Record<string, string> = {
  'code-quality': 'Code quality',
  compliance: 'Compliance',
  dependencies: 'Dependencies',
  infrastructure: 'Infrastructure',
  maintainability: 'Maintainability',
  secrets: 'Secrets',
  security: 'Security',
};

const categoryIcons: Record<string, React.ElementType> = {
  lint: Wrench, style: Palette, security: Shield, pentest: Zap, secrets: KeyRound, maintainability: Gauge,
  dependencies: Package, container: Container, iac: FileCog, license: Scale,
};

const NETWORK_LABELS: Record<AnalyzerStatus['network'], string> = {
  none: 'No network',
  outbound: 'Outbound',
  'loopback-only': 'Loopback only',
};

const PROFILE_LABELS: Record<string, string> = { quick: 'Quick', standard: 'Standard', deep: 'Deep', pentest: 'Pentest' };

/** Visual accent (icon + hue) for a row, looked up in the frontend catalog.
 *  The rendered label is the API's analyzer.category — see API_CATEGORY_LABELS. */
function visualCategory(analyzer: AnalyzerStatus): AnalyzerCategory {
  return analyzerMeta(analyzer.id)?.category ?? ('security' as AnalyzerCategory);
}

/** Status cell: external tools are Ready/Not installed, in-process analyzers are
 *  Built-in — or Unavailable when offline mode withheld them at startup. */
function statusLabel(analyzer: AnalyzerStatus): { text: string; state: 'ready' | 'not-ready' | 'built-in' } {
  if (analyzer.execution === 'in-process') {
    return analyzer.registered ? { text: 'Built-in', state: 'built-in' } : { text: 'Offline mode', state: 'not-ready' };
  }
  return analyzer.ready ? { text: 'Ready', state: 'ready' } : { text: 'Not installed', state: 'not-ready' };
}

function ReadinessStrip({ analyzers, busy }: { analyzers: AnalyzerStatus[]; busy?: BusyAction }) {
  const managed = analyzers.filter((a) => a.managed_tool);
  const ready = managed.filter((a) => a.ready).length;
  return <div className="tools-readiness" role="status"><span className={`badge${managed.length > 0 && ready === managed.length ? ' tools-all-ready' : ''}`}>{ready} of {managed.length} ready</span>{managed.filter((a) => !a.ready).map((a) => {
    const active = busy !== undefined && busy.tool === a.id ? busy.operation : undefined;
    return <span key={a.id} className="badge">{active ? <span className="spinner" aria-hidden="true" /> : <i className="dot not-ready" aria-hidden="true" />}{a.display_name} {active ? operationBusyLabels[active].toLowerCase() : 'not installed'}</span>;
  })}</div>;
}

export function ToolsPage({ notify, go }: { notify: (n: Notice) => void; go?: (r: Route) => void }) {
  const analyzers = useLoad(api.analyzers, []);
  const [busy, setBusy] = useState<BusyAction>();
  const [pending, setPending] = useState<PendingAction>();
  async function action(analyzer: AnalyzerStatus, operation: ToolOperation) {
    if (!analyzer.managed_tool) return;
    setBusy({ tool: analyzer.managed_tool, operation });
    try { await api.toolAction(analyzer.managed_tool, operation); await analyzers.reload(); notify({ kind: 'info', text: `${analyzer.display_name || analyzer.id}: ${operationVerbs[operation]}.` }); } catch (e) { notify({ kind: 'error', text: message(e) }); } finally { setBusy(undefined); }
  }

  const rows = analyzers.data ?? [];
  const pendingName = pending ? pending.analyzer.display_name || pending.analyzer.id : '';

  return (
    <div className="page">
      <PageHeader
        eyebrow="Analyzers"
        title="Analysis tools"
        badge={analyzers.error ? undefined : <Badge variant="secondary" className="text-xs font-mono tabular-nums">{analyzers.loading ? '… engines' : `${rows.length} engines`}</Badge>}
        description="Every analyzer registered on this machine — category, scan tiers, network use, and readiness — straight from the backend capability inventory."
      />
      {analyzers.loading ? <SkeletonTable rows={ANALYZER_CATALOG.length} cols={8} className="tool-table" /> : analyzers.error ? <ErrorPanel error={analyzers.error} retry={analyzers.reload} /> : !rows.length ? <Empty title="No analyzers" icon={<WrenchIcon />}>Analyzer capabilities appear here after the backend registers its inventory.</Empty> : <><ReadinessStrip analyzers={rows} busy={busy} />
      <div className="tool-table table-wrap border rounded-lg bg-[var(--color-surface)] overflow-hidden">
        <table>
          <thead>
            <tr>
              <th scope="col">Analyzer</th>
              <th scope="col">Category</th>
              <th scope="col">Scan tiers</th>
              <th scope="col">Network</th>
              <th scope="col">Version</th>
              <th scope="col">Status</th>
              <th scope="col">Details</th>
              <th scope="col">Actions</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((analyzer) => {
              const activeOperation = busy && busy.tool === analyzer.managed_tool ? busy.operation : undefined;
              const cat = visualCategory(analyzer);
              const status = statusLabel(analyzer);
              const Icon = categoryIcons[cat] ?? Wrench;
              const detail = analyzer.detail || analyzer.description;
              // The inventory always sends display_name, but a hostile or
              // older payload must degrade to the id, never render "undefined".
              const name = analyzer.display_name || analyzer.id;
              return (
                <tr key={analyzer.id}>
                  <td><strong className="flex items-center gap-1.5">{name}</strong></td>
                  <td><span className="badge inline-flex items-center gap-1 text-[10px]" style={{ borderColor: `color-mix(in oklch, ${categoryColor(cat)} 34%, var(--color-rule))`, color: categoryColor(cat), background: `color-mix(in oklch, ${categoryColor(cat)} 12%, var(--color-surface))` }}><Icon className="h-3 w-3" />{API_CATEGORY_LABELS[analyzer.category] ?? analyzer.category}</span></td>
                  <td className="text-xs">{(analyzer.profiles ?? []).map((p) => PROFILE_LABELS[p] ?? p).join(', ')}</td>
                  <td><span className="badge text-[10px]" title={analyzer.network_note || undefined}>{NETWORK_LABELS[analyzer.network] ?? analyzer.network}</span></td>
                  <td><span className="badge tool-version">{analyzer.version ? `v${analyzer.version}` : analyzer.execution === 'in-process' ? 'Built-in' : 'Managed version'}</span></td>
                  <td><span className={`state ${status.state === 'not-ready' ? 'not-ready' : 'ready'}`}>{status.text}</span></td>
                  <td className="text-xs"><div className="max-w-[12rem] truncate" title={detail}>{detail}</div></td>
                  <td className="table-actions" aria-busy={activeOperation ? true : undefined}>
                    {analyzer.managed_tool ? <span className="flex gap-1">{(['install', 'repair', 'update'] as const).map((operation) => <button type="button" key={operation} className="text-button" disabled={activeOperation !== undefined} onClick={() => setPending({ analyzer, operation })}>{activeOperation === operation ? <><span className="spinner" aria-hidden="true" />{operationBusyLabels[operation]}</> : operationLabels[operation]}</button>)}</span> : <span className="text-xs text-[var(--color-ink-soft)]">—</span>}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      <LanguageCoverage />
      {pending && <ConfirmationDialog
        title={`${operationLabels[pending.operation]} ${pendingName}`}
        description={operationConfirmCopy[pending.operation](pendingName)}
        confirmLabel={operationLabels[pending.operation]}
        busy={false}
        onCancel={() => setPending(undefined)}
        onConfirm={() => { const { analyzer, operation } = pending; setPending(undefined); void action(analyzer, operation); }}
      />}
    </>}
  </div>
  );
}

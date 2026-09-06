import { useRef, useState } from 'react';
import '../css/tools.css';
import { api } from '../api';
import type { AnalyzerStatus } from '../types';
import type { Notice } from '../lib/notice';
import { message } from '../lib/notice';
import { useLoad } from '../hooks/useLoad';
import { Empty, ErrorPanel } from '../components/ui';
import { WrenchIcon } from '../components/icons';
import { SkeletonTable } from '../components/skeletons';
import { analyzerMeta, categoryColor, CATEGORY_LABELS, type AnalyzerCategory } from '../lib/analyzerCatalog';
import { LanguageCoverage } from '../components/LanguageCoverage';
import type { Route } from '../lib/router';
import { PageHeader } from '../components/PageHeader';
import { Badge } from '../components/ui/badge';
import { Wrench, Shield, Bug, KeyRound, Boxes, Container, FileCog, Scale, Palette, Zap, Crosshair, Radar, Package, ListTodo, Gauge, Download, WrenchIcon as RepairIcon, RefreshCw, MoreHorizontal } from 'lucide-react';

type ToolOperation = 'install' | 'repair' | 'update';
type BusyAction = { tool: string; operation: ToolOperation };

const operationLabels: Record<ToolOperation, string> = { install: 'Install', repair: 'Repair', update: 'Update' };
const operationBusyLabels: Record<ToolOperation, string> = { install: 'Installing…', repair: 'Repairing…', update: 'Updating…' };
const operationVerbs: Record<ToolOperation, string> = { install: 'installed', repair: 'repaired', update: 'updated' };

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

function ToolActions({ analyzer, activeOperation, onAction }: { analyzer: AnalyzerStatus; activeOperation?: ToolOperation; onAction: (a: AnalyzerStatus, op: ToolOperation)=>void }) {
  const [open,setOpen]=useState(false);
  const ref=useRef<HTMLDivElement>(null);
  const ops: Array<{op: ToolOperation; icon: React.ElementType; label:string}> = [{op:'install', icon: Download, label:'Install'},{op:'repair', icon: RepairIcon, label:'Repair'},{op:'update', icon: RefreshCw, label:'Update'}];

  // close on outside
  const handleOpen=()=> setOpen(v=>!v);
  return <div ref={ref} className="relative inline-block">
    <button type="button" aria-haspopup="menu" aria-expanded={open} onClick={handleOpen} disabled={activeOperation!==undefined} className="inline-flex h-8 items-center gap-1 rounded-[var(--radius-button)] border border-[var(--color-rule)] bg-[var(--color-surface)] px-3 text-xs font-semibold shadow-[var(--shadow-card)] disabled:opacity-50"><MoreHorizontal className="h-3.5 w-3.5" />Actions</button>
    {activeOperation && <span className="ml-1 inline-flex items-center gap-1 text-xs"><span className="spinner" aria-hidden="true" />{operationBusyLabels[activeOperation]}</span>}
    {open && <div role="menu" className="absolute right-0 z-10 mt-1 grid min-w-[10rem] gap-0.5 rounded-[var(--radius-lg)] border border-[var(--color-rule)] bg-[var(--color-surface)] p-1 shadow-[var(--shadow-lg)]">{ops.map(({op,icon:Icon,label})=> <button key={op} role="menuitem" type="button" disabled={!analyzer.managed_tool} onClick={()=>{ setOpen(false); void onAction(analyzer,op); }} className="flex items-center gap-2 rounded-[var(--radius-button)] px-2 py-1.5 text-left text-sm hover:bg-[var(--color-surface-muted)] disabled:opacity-50"><Icon className="h-3.5 w-3.5" />{label}</button>)}</div>}
  </div>;
}

export function ToolsPage({ notify, go }: { notify: (n: Notice) => void; go?: (r: Route) => void }) {
  const analyzers = useLoad(api.analyzers, []);
  const [busy, setBusy] = useState<BusyAction>();
  async function action(analyzer: AnalyzerStatus, operation: ToolOperation) {
    if (!analyzer.managed_tool) return;
    setBusy({ tool: analyzer.managed_tool, operation });
    try { await api.toolAction(analyzer.managed_tool, operation); await analyzers.reload(); notify({ kind: 'info', text: `${analyzer.display_name || analyzer.id}: ${operationVerbs[operation]}.` }); } catch (e) { notify({ kind: 'error', text: message(e) }); } finally { setBusy(undefined); }
  }

  const rows = analyzers.data ?? [];

  return (
    <div className="page">
      <PageHeader
        eyebrow="Analyzers"
        title="Analysis tools"
        badge={<Badge variant="secondary" className="text-xs font-mono tabular-nums">{rows.length} engines</Badge>}
        description="Every analyzer registered on this machine — category, scan tiers, network use, and readiness — straight from the backend capability inventory."
      />
      {analyzers.loading ? <SkeletonTable rows={4} cols={7} className="tool-table" /> : analyzers.error ? <ErrorPanel error={analyzers.error} retry={analyzers.reload} /> : !rows.length ? <Empty title="No analyzers" icon={<WrenchIcon />}>Analyzer capabilities appear here after the backend registers its inventory.</Empty> : <><ReadinessStrip analyzers={rows} busy={busy} />
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
                  <td><span className="badge inline-flex items-center gap-1 text-[10px]" style={{ borderColor: `color-mix(in oklch, ${categoryColor(cat)} 34%, var(--color-rule))`, color: categoryColor(cat), background: `color-mix(in oklch, ${categoryColor(cat)} 12%, var(--color-surface))` }}><Icon className="h-3 w-3" />{CATEGORY_LABELS[cat] ?? cat}</span></td>
                  <td className="text-xs">{(analyzer.profiles ?? []).map((p) => PROFILE_LABELS[p] ?? p).join(', ')}</td>
                  <td><span className="badge text-[10px]" title={analyzer.network_note || undefined}>{NETWORK_LABELS[analyzer.network] ?? analyzer.network}</span></td>
                  <td><span className="badge tool-version">{analyzer.version ? `v${analyzer.version}` : analyzer.execution === 'in-process' ? 'Built-in' : 'Managed version'}</span></td>
                  <td><span className={`state ${status.state === 'not-ready' ? 'not-ready' : 'ready'}`}>{status.text}</span></td>
                  <td className="text-xs max-w-[18rem] truncate" title={detail}>{detail}</td>
                  <td className="table-actions" aria-busy={activeOperation ? true : undefined}>
                    {analyzer.managed_tool ? <span className="flex gap-1">{(['install', 'repair', 'update'] as const).map((operation) => <button type="button" key={operation} className="text-button" disabled={activeOperation !== undefined} onClick={() => void action(analyzer, operation)}>{activeOperation === operation ? <><span className="spinner" aria-hidden="true" />{operationBusyLabels[operation]}</> : operationLabels[operation]}</button>)}</span> : <span className="text-xs text-[var(--color-ink-soft)]">—</span>}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      <LanguageCoverage />
    </>}
  </div>
  );
}
